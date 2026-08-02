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
			log.Info("password login failed", "email", body.Email, "err", err)
			writeError(w, status, err.Error())

			return
		}

		log.Info("login", "user", user.Email, "role", user.Role, "method", "password")
		w.WriteHeader(http.StatusNoContent)
	})

	// Identity-provider sign-in. Separate from POST /auth/login so that the
	// two methods can coexist on one deployment.
	mux.HandleFunc("GET /auth/oidc", func(w http.ResponseWriter, r *http.Request) {
		if !a.OIDCEnabled() {
			http.Error(w, "no identity provider is configured", http.StatusNotFound)

			return
		}

		if _, ok := a.CurrentUser(r.Context()); ok {
			http.Redirect(w, r, a.cfg.BasePath+"/", http.StatusSeeOther)

			return
		}

		url, err := a.AuthCodeURL(r.Context(), r.URL.Query().Get("return_to"))
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
			http.Redirect(w, r, a.cfg.BasePath+"/login?error=1", http.StatusSeeOther)

			return
		}

		log.Info("login", "user", user.Email, "role", user.Role,
			"linked", user.Linked(), "method", "oidc")

		http.Redirect(w, r, a.appPath(returnTo), http.StatusSeeOther)
	})

	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Logout(r.Context()); err != nil {
			log.Error("logout", "err", err)
		}

		http.Redirect(w, r, a.cfg.BasePath+"/login", http.StatusSeeOther)
	})
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
			r = r.WithContext(WithPrincipal(r.Context(), Principal{User: u}))
		}

		next.ServeHTTP(w, r)
	})
}
