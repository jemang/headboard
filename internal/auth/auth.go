// Package auth handles who is logged in and what they may do.
//
// There are two ways in, and both are first class:
//
//   - A local account with a password, so Headboard can be run against a
//     Headscale with nothing else deployed, and so there is always a way back
//     in when the identity provider is misconfigured.
//   - An OIDC identity shared with Headscale. A Headscale user carries
//     provider_identifier — the `iss` and `sub` concatenated — so an identity
//     that logs into Headboard can be matched to the Headscale user that owns
//     the devices, without either system trusting an email address.
//
// OIDC is optional. Sessions are not: they are built whether or not a provider
// is configured, because the password path needs them too.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/jemang/headboard/internal/store"
)

// sessionKeyUserID is the only thing kept in the session. Everything else is
// re-read from the database per request, so a role change takes effect on the
// next request rather than on the next login.
const sessionKeyUserID = "user_id"

// Config is what auth needs from the environment.
type Config struct {
	// SecureCookies marks the session cookie Secure. Off only for plain
	// HTTP development.
	SecureCookies bool

	// SessionLifetime is how long a login lasts.
	SessionLifetime time.Duration

	// BasePath scopes the session cookie when Headboard is served under a
	// path rather than at a site root. It matters most when something else
	// shares the hostname — Headscale itself, typically — because a cookie
	// on "/" is sent to that neighbour on every request.
	BasePath string
}

// OIDCConfig describes the optional identity provider.
type OIDCConfig struct {
	// Issuer is the OIDC issuer URL, the same one Headscale is configured
	// with. Sharing it is what makes provider_identifier matching work.
	Issuer string

	ClientID     string
	ClientSecret string

	// RedirectURL is Headboard's callback, e.g.
	// https://headboard.example.com/auth/callback.
	RedirectURL string

	// Scopes beyond the mandatory openid. Defaults to profile and email.
	Scopes []string
}

// Auth is the assembled authentication layer.
type Auth struct {
	cfg      Config
	store    *store.Store
	sessions *scs.SessionManager
	logins   *limiter

	// Set by EnableOIDC. Nil when no provider is configured, which is the
	// supported default rather than a broken state.
	oidcCfg  *OIDCConfig
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
}

// ErrDisabled is returned when a request needs OIDC but it is not configured.
var ErrDisabled = errors.New("oidc is not configured")

// New wires up sessions. It never fails on a missing identity provider,
// because password login does not need one.
func New(cfg Config, st *store.Store) *Auth {
	if cfg.SessionLifetime <= 0 {
		cfg.SessionLifetime = 12 * time.Hour
	}

	sessions := scs.New()
	sessions.Store = st.Sessions()
	sessions.Lifetime = cfg.SessionLifetime
	sessions.Cookie.Name = "headboard_session"
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.Path = cfg.BasePath + "/"
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Secure = cfg.SecureCookies

	return &Auth{
		cfg:      cfg,
		store:    st,
		sessions: sessions,
		logins:   newLimiter(5, time.Minute),
	}
}

// EnableOIDC discovers the provider and turns on the identity-provider path.
//
// Discovery happens once at startup rather than per login: it is a network call
// to the IdP, and failing it at boot is a clearer error than failing it for the
// first person who tries to log in.
func (a *Auth) EnableOIDC(ctx context.Context, cfg OIDCConfig) error {
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.RedirectURL == "" {
		return ErrDisabled
	}

	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{oidc.ScopeOpenID, "profile", "email"}
	}

	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return fmt.Errorf("oidc discovery for %s: %w", cfg.Issuer, err)
	}

	a.oidcCfg = &cfg
	a.provider = provider
	a.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	a.oauth = oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       cfg.Scopes,
	}

	return nil
}

// OIDCEnabled reports whether an identity provider is configured.
func (a *Auth) OIDCEnabled() bool { return a.provider != nil }

// LoginWithPassword checks credentials and starts a session.
//
// key scopes the rate limiter — the caller passes something derived from the
// client address, so that neither an address nor a source can be attacked
// without tripping it.
func (a *Auth) LoginWithPassword(ctx context.Context, key, email, password string) (store.User, error) {
	bucket := key + "|" + strings.ToLower(strings.TrimSpace(email))

	if wait, blocked := a.logins.blocked(bucket); blocked {
		return store.User{}, fmt.Errorf("%w: try again in %s", ErrTooManyAttempts, wait.Round(time.Second))
	}

	u, err := a.store.UserByLocalEmail(ctx, email)
	if err != nil || u.PasswordHash == "" || !VerifyPassword(u.PasswordHash, password) {
		a.logins.fail(bucket)

		return store.User{}, ErrBadCredentials
	}

	a.logins.succeed(bucket)

	if err := a.login(ctx, u.ID); err != nil {
		return store.User{}, err
	}

	if err := a.store.TouchLogin(ctx, u.ID); err != nil {
		// The login worked; failing to stamp it is not worth refusing.
		_ = err
	}

	return u, nil
}

// ErrTooManyAttempts is returned when the rate limiter trips.
var ErrTooManyAttempts = errors.New("too many attempts")

// Sessions exposes the manager so the router can wrap every request in
// LoadAndSave.
func (a *Auth) Sessions() *scs.SessionManager { return a.sessions }

// Issuer is the configured OIDC issuer, empty when there is none.
func (a *Auth) Issuer() string {
	if a.oidcCfg == nil {
		return ""
	}

	return a.oidcCfg.Issuer
}

// login records a successful authentication in the session.
func (a *Auth) login(ctx context.Context, userID int64) error {
	// A fresh token on login prevents a pre-set cookie from being promoted
	// to an authenticated session.
	if err := a.sessions.RenewToken(ctx); err != nil {
		return fmt.Errorf("renewing session: %w", err)
	}

	a.sessions.Put(ctx, sessionKeyUserID, userID)

	return nil
}

// Logout ends the session.
func (a *Auth) Logout(ctx context.Context) error {
	if err := a.sessions.Destroy(ctx); err != nil {
		return fmt.Errorf("destroying session: %w", err)
	}

	return nil
}

// CurrentUser reads the logged-in account, fresh from the database.
func (a *Auth) CurrentUser(ctx context.Context) (store.User, bool) {
	id, ok := a.sessions.Get(ctx, sessionKeyUserID).(int64)
	if !ok || id == 0 {
		return store.User{}, false
	}

	u, err := a.store.UserByID(ctx, id)
	if err != nil {
		// The account was deleted while the session lived on.
		return store.User{}, false
	}

	return u, true
}
