package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/juanfont/headscale/hscontrol/types"
	"golang.org/x/oauth2"

	"github.com/jemang/headboard/internal/store"
)

// Session keys for the in-flight authorisation request. They live in the
// session rather than in a cookie of their own so they inherit its SameSite and
// Secure settings.
const (
	sessionKeyState    = "oidc_state"
	sessionKeyNonce    = "oidc_nonce"
	sessionKeyVerifier = "oidc_verifier"
	sessionKeyReturnTo = "oidc_return_to"
)

// claims is the subset of the ID token Headboard reads.
type claims struct {
	Subject           string `json:"sub"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Picture           string `json:"picture"`
}

// AuthCodeURL starts a login and returns where to send the browser.
//
// State, nonce and a PKCE verifier are all generated per attempt and stored in
// the session: state defends the callback against cross-site forgery, nonce
// binds the ID token to this attempt, and PKCE means an intercepted code cannot
// be redeemed without the verifier.
func (a *Auth) AuthCodeURL(ctx context.Context, returnTo string) (string, error) {
	state, err := randomToken()
	if err != nil {
		return "", err
	}

	nonce, err := randomToken()
	if err != nil {
		return "", err
	}

	verifier := oauth2.GenerateVerifier()

	a.sessions.Put(ctx, sessionKeyState, state)
	a.sessions.Put(ctx, sessionKeyNonce, nonce)
	a.sessions.Put(ctx, sessionKeyVerifier, verifier)
	a.sessions.Put(ctx, sessionKeyReturnTo, safeReturnTo(returnTo))

	return a.oauth.AuthCodeURL(state,
		oidcNonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	), nil
}

// ErrStateMismatch is returned when the callback's state does not match the one
// this session started with.
var ErrStateMismatch = errors.New("oidc state mismatch")

// Callback completes the login. It returns the account and where to send the
// browser next.
//
// headscaleUsers is the live user list; the identity is matched against it on
// provider_identifier. Matching on email would be wrong — an IdP can let a
// display email change, and two Headscale users can share one.
func (a *Auth) Callback(
	ctx context.Context,
	r *http.Request,
	headscaleUsers []types.User,
	log *slog.Logger,
) (store.User, string, error) {
	q := r.URL.Query()

	if errCode := q.Get("error"); errCode != "" {
		return store.User{}, "", fmt.Errorf("identity provider returned %s: %s",
			errCode, q.Get("error_description"))
	}

	wantState, _ := a.sessions.Get(ctx, sessionKeyState).(string)
	if wantState == "" || q.Get("state") != wantState {
		return store.User{}, "", ErrStateMismatch
	}

	verifier, _ := a.sessions.Get(ctx, sessionKeyVerifier).(string)
	wantNonce, _ := a.sessions.Get(ctx, sessionKeyNonce).(string)
	returnTo, _ := a.sessions.Get(ctx, sessionKeyReturnTo).(string)

	// Single-use: clear them before any early return below.
	a.sessions.Remove(ctx, sessionKeyState)
	a.sessions.Remove(ctx, sessionKeyNonce)
	a.sessions.Remove(ctx, sessionKeyVerifier)
	a.sessions.Remove(ctx, sessionKeyReturnTo)

	token, err := a.oauth.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		return store.User{}, "", fmt.Errorf("exchanging authorization code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return store.User{}, "", errors.New("identity provider returned no id_token")
	}

	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return store.User{}, "", fmt.Errorf("verifying id_token: %w", err)
	}

	if idToken.Nonce != wantNonce {
		return store.User{}, "", errors.New("oidc nonce mismatch")
	}

	var c claims
	if err := idToken.Claims(&c); err != nil {
		return store.User{}, "", fmt.Errorf("reading id_token claims: %w", err)
	}

	if c.Subject == "" {
		return store.User{}, "", errors.New("id_token has no subject")
	}

	u := store.User{
		OIDCIssuer:  idToken.Issuer,
		OIDCSubject: c.Subject,
		Email:       c.Email,
		DisplayName: displayName(c),
		AvatarURL:   c.Picture,
	}

	if hsUser, ok := matchHeadscaleUser(idToken.Issuer, c.Subject, headscaleUsers); ok {
		id := int64(hsUser.ID)
		u.HeadscaleUserID = &id
	}

	// Preserve an existing link: matching only happens for identities
	// Headscale already knows, and an admin may have linked this account by
	// hand precisely because Headscale did not.
	if existing, err := a.store.UserByOIDC(ctx, u.OIDCIssuer, u.OIDCSubject); err == nil {
		if u.HeadscaleUserID == nil {
			u.HeadscaleUserID = existing.HeadscaleUserID
		}
	}

	saved, err := a.store.UpsertLogin(ctx, u)
	if err != nil {
		return store.User{}, "", fmt.Errorf("recording login: %w", err)
	}

	if err := a.login(ctx, saved.ID); err != nil {
		return store.User{}, "", err
	}

	if !saved.Linked() {
		log.Warn("logged-in identity is not linked to a headscale user",
			"user", saved.Email,
			"sub", c.Subject,
			"hint", "headscale users created with the CLI have no provider_identifier; link the account from the admin console",
		)
	}

	if returnTo == "" {
		returnTo = "/"
	}

	return saved, returnTo, nil
}

// matchHeadscaleUser finds the Headscale user for an OIDC identity.
//
// Headscale stores iss and sub concatenated in provider_identifier. It is
// written by Headscale itself at OIDC registration, which is why users created
// with `headscale users create` have it empty and never match here.
func matchHeadscaleUser(iss, sub string, users []types.User) (types.User, bool) {
	want := providerIdentifier(iss, sub)

	for _, u := range users {
		if !u.ProviderIdentifier.Valid || u.ProviderIdentifier.String == "" {
			continue
		}

		if u.ProviderIdentifier.String == want {
			return u, true
		}
	}

	return types.User{}, false
}

// providerIdentifier builds the value Headscale stores.
//
// It calls Headscale's own OIDCClaims.Identifier rather than concatenating the
// two claims here. The rules are fiddlier than they look — trailing and leading
// slashes are trimmed, the result goes through CleanIdentifier, and empty
// issuer or subject are special-cased — and a mismatch would silently fail to
// link an account rather than error.
func providerIdentifier(iss, sub string) string {
	c := types.OIDCClaims{Iss: iss, Sub: sub}

	return c.Identifier()
}

func displayName(c claims) string {
	switch {
	case c.Name != "":
		return c.Name
	case c.PreferredUsername != "":
		return c.PreferredUsername
	case c.Email != "":
		return c.Email
	default:
		return c.Subject
	}
}

// safeReturnTo keeps post-login redirects inside Headboard. An absolute URL
// here would turn the login into an open redirect.
func safeReturnTo(raw string) string {
	if raw == "" {
		return "/"
	}

	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return "/"
	}

	// "//evil.example" parses with an empty host in some cases but is a
	// protocol-relative URL to the browser.
	if strings.HasPrefix(raw, "//") {
		return "/"
	}

	return u.RequestURI()
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func oidcNonce(nonce string) oauth2.AuthCodeOption {
	return oauth2.SetAuthURLParam("nonce", nonce)
}

// FingerprintIssuer is used in logs to identify which IdP is configured without
// printing a URL that may embed a tenant id.
func FingerprintIssuer(iss string) string {
	sum := sha256.Sum256([]byte(iss))

	return base64.RawURLEncoding.EncodeToString(sum[:6])
}
