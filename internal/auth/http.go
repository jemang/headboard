package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/juanfont/headscale/hscontrol/types"
	"golang.org/x/oauth2"

	"github.com/jemang/headboard/internal/store"
)

// UserLister supplies the live Headscale user list for identity matching. It is
// an interface so auth does not depend on the whole Headscale client.
type UserLister interface {
	ListUsers(ctx context.Context) ([]types.User, error)
}

// Routes mounts the browser-facing authentication endpoints.
//
// These are plain http handlers rather than Huma operations: they redirect and
// set cookies rather than returning JSON, so an OpenAPI description of them
// would describe nothing useful.
func (a *Auth) Routes(mux *http.ServeMux, users UserLister, log *slog.Logger) {
	// Password sign-in. Posted by the login form as JSON; it answers with a
	// status rather than a redirect so the SPA can show the error in place.
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}

		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "could not read the request")

			return
		}

		user, err := a.LoginWithPassword(r.Context(), clientKey(r), body.Email, body.Password)
		if err != nil {
			status := http.StatusUnauthorized
			if errors.Is(err, ErrTooManyAttempts) {
				status = http.StatusTooManyRequests
			}

			// Logged at info, not warn: a mistyped password is normal, and
			// a log that cries wolf gets ignored when it matters.
			log.Info("password login failed", "email", attemptedLabel(body.Email), "err", err)
			reason := "bad credentials"
			if errors.Is(err, ErrTooManyAttempts) {
				reason = "too many attempts"
			}
			a.record(r, nil, attemptedLabel(body.Email), "auth.login_failed", map[string]string{"method": "password", "reason": reason}, log)
			writeError(w, status, err.Error())

			return
		}

		log.Info("login", "user", user.Email, "role", user.Role, "method", "password")
		a.record(r, &user.ID, user.Email, "auth.login", map[string]string{"method": "password"}, log)
		w.WriteHeader(http.StatusNoContent)
	})

	// Identity-provider sign-in. Separate from POST /auth/login so that the
	// two methods can coexist on one deployment.
	mux.HandleFunc("GET /auth/oidc", func(w http.ResponseWriter, r *http.Request) {
		if !a.OIDCEnabled() {
			http.Error(w, "no identity provider is configured", http.StatusNotFound)

			return
		}

		// ?reauth=1 is a signed-in person confirming who they are before
		// setting a sudo PIN: force a real sign-in at the provider.
		var opts []oauth2.AuthCodeOption
		if u, ok := a.CurrentUser(r.Context()); ok {
			if r.URL.Query().Get("reauth") != "1" {
				http.Redirect(w, r, a.cfg.BasePath+"/", http.StatusSeeOther)

				return
			}

			a.sessions.Put(r.Context(), sessionKeyReauthFor, u.ID)
			opts = append(opts, oauth2.SetAuthURLParam("prompt", "login"), oauth2.SetAuthURLParam("max_age", "0"))
		}

		url, err := a.AuthCodeURL(r.Context(), r.URL.Query().Get("return_to"), opts...)
		if err != nil {
			log.Error("starting login", "err", err)
			http.Error(w, "could not start login", http.StatusInternalServerError)

			return
		}

		http.Redirect(w, r, url, http.StatusSeeOther)
	})

	mux.HandleFunc("GET /auth/callback", func(w http.ResponseWriter, r *http.Request) {
		if !a.OIDCEnabled() {
			http.Error(w, "no identity provider is configured", http.StatusNotFound)

			return
		}

		list, err := users.ListUsers(r.Context())
		if err != nil {
			// Without the user list an identity cannot be linked, but it
			// can still log in — the account simply arrives unlinked and
			// an admin fixes it. Refusing the login instead would make a
			// Headscale outage lock everyone out of the console that
			// tells them Headscale is down.
			log.Warn("could not read headscale users during login; account will be unlinked", "err", err)

			list = nil
		}

		user, returnTo, err := a.Callback(r.Context(), r, list, log)
		if err != nil {
			log.Warn("login failed", "err", err)
			a.record(r, nil, "", "auth.login_failed", map[string]string{"method": "oidc", "reason": "identity provider sign-in failed"}, log)
			http.Redirect(w, r, a.cfg.BasePath+"/login?error=1", http.StatusSeeOther)

			return
		}

		log.Info("login", "user", user.Email, "role", user.Role,
			"linked", user.Linked(), "method", "oidc")
		a.record(r, &user.ID, user.Email, "auth.login", map[string]string{"method": "oidc"}, log)

		http.Redirect(w, r, a.appPath(returnTo), http.StatusSeeOther)
	})

	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if u, ok := a.CurrentUser(r.Context()); ok {
			a.record(r, &u.ID, u.Email, "auth.logout", nil, log)
		}

		if err := a.Logout(r.Context()); err != nil {
			log.Error("logout", "err", err)
		}

		http.Redirect(w, r, a.cfg.BasePath+"/login", http.StatusSeeOther)
	})
}

// record writes a sign-in event to the audit log. Best effort, like every
// audit write: a failed write must not turn a good sign-in into an error.
// A failed attempt has no account, so the label is the email that was tried.
func (a *Auth) record(r *http.Request, userID *int64, label, action string, detail any, log *slog.Logger) {
	var after json.RawMessage
	if detail != nil {
		after, _ = json.Marshal(detail)
	}

	if err := a.store.Audit(r.Context(), store.AuditEntry{
		ActorUserID: userID, ActorLabel: label, Action: action,
		TargetType: "session", After: after, IP: clientKey(r),
	}); err != nil {
		log.Error("audit write failed", "action", action, "err", err)
	}
}

// attemptedLabel is what a failed sign-in records as "who": the email that
// was tried, but only if it looks like one. Anything else may be a password
// typed into the wrong box, and is not kept.
func attemptedLabel(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if len(email) > 254 || at < 1 || at == len(email)-1 || strings.ContainsAny(email, " \t\r\n") || !strings.Contains(email[at:], ".") {
		return ""
	}

	return email
}

// clientKey identifies the caller for rate limiting. RealIP has already
// normalised proxy headers by the time this runs.
func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": message})
}

// appPath prefixes a redirect target with the deployment's base path.
//
// return_to comes off the query string, so it may already carry the prefix —
// a person who copies /manage/acl out of the address bar into a login link.
// Prefixing blindly would send them to /manage/manage/acl, which the SPA
// renders as an empty page rather than an error.
func (a *Auth) appPath(p string) string {
	base := a.cfg.BasePath
	if base == "" || p == base || strings.HasPrefix(p, base+"/") {
		return p
	}

	return base + p
}

// Middleware attaches the caller to the request context when a session exists.
// It never rejects: authorisation is each handler's decision, expressed as a
// capability.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := a.CurrentUser(r.Context()); ok {
			p := Principal{User: u, IP: clientKey(r)}
			// Only a live grant reaches handlers; an expired one reads as none.
			p.SudoUntil = a.sudoUntil(r.Context(), u.ID)
			p.ReauthAt = a.sessionTime(r.Context(), sessionKeyReauthAt)
			r = r.WithContext(WithPrincipal(r.Context(), p))
		}

		next.ServeHTTP(w, r)
	})
}
