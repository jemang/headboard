package auth

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"github.com/jemang/headboard/internal/store"
)

// Sign-ins are part of "what happened": a burst of failures, or a success
// from an unfamiliar address, is exactly what an admin looks for afterwards.
func TestSignInsAndSignOutsAreAudited(t *testing.T) {
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateLocalOwner(t.Context(), "ops@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}

	a := New(Config{}, st)
	mux := http.NewServeMux()
	a.Routes(mux, nil, slog.Default())
	h := a.Sessions().LoadAndSave(mux)

	post := func(path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.RemoteAddr = "203.0.113.5:4000"
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		return w
	}

	post("/auth/login", `{"email":"ops@example.com","password":"wrong"}`)
	// A password typed into the email box must not be kept.
	post("/auth/login", `{"email":"correct horse battery","password":"x"}`)
	ok := post("/auth/login", `{"email":"ops@example.com","password":"correct horse battery"}`)
	if ok.Code != http.StatusNoContent {
		t.Fatalf("login: %d", ok.Code)
	}
	post("/auth/logout", "", ok.Result().Cookies()...)

	entries, err := st.ListAudit(t.Context(), store.AuditFilter{Prefixes: []string{"auth."}})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]store.AuditEntry{}
	for _, e := range entries {
		got[e.Action] = e
	}

	failed := got["auth.login_failed"]
	if failed.ActorUserID != nil || failed.ActorLabel != "ops@example.com" || failed.IP != "203.0.113.5" {
		t.Errorf("failed sign-in = %+v, want the attempted email, no account and the address", failed)
	}
	if strings.Contains(string(failed.After), "wrong") {
		t.Errorf("failed sign-in recorded the password: %s", failed.After)
	}

	for _, e := range entries {
		if strings.Contains(e.ActorLabel, "horse") {
			t.Errorf("a non-email sign-in name was stored: %q", e.ActorLabel)
		}
	}

	in := got["auth.login"]
	if in.ActorUserID == nil || *in.ActorUserID != owner.ID || !strings.Contains(string(in.After), `"password"`) {
		t.Errorf("sign-in = %+v, want the account and the method", in)
	}

	out := got["auth.logout"]
	if out.ActorUserID == nil || *out.ActorUserID != owner.ID {
		t.Errorf("sign-out = %+v, want the account that left", out)
	}
}

// Sudo belongs to one sign-in: whoever signs in next on the same browser
// starts without it, and the middleware hands handlers only a live grant.
func TestSudoIsClearedBySignInAndReadByMiddleware(t *testing.T) {
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	hash, _ := HashPassword("correct horse battery")
	owner, err := st.CreateLocalOwner(t.Context(), "ops@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}

	a := New(Config{}, st)
	var seen Principal
	mux := http.NewServeMux()
	a.Routes(mux, nil, slog.Default())
	mux.HandleFunc("POST /grant", func(w http.ResponseWriter, r *http.Request) {
		_ = a.GrantSudo(r.Context(), owner.ID, time.Now().Add(5*time.Minute))
	})
	mux.HandleFunc("GET /who", func(w http.ResponseWriter, r *http.Request) {
		seen, _ = PrincipalFrom(r.Context())
	})
	h := a.Sessions().LoadAndSave(a.Middleware(mux))

	do := func(method, path, body string, c []*http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, k := range c {
			r.AddCookie(k)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		return w
	}

	login := do(http.MethodPost, "/auth/login", `{"email":"ops@example.com","password":"correct horse battery"}`, nil)
	cookies := login.Result().Cookies()
	do(http.MethodPost, "/grant", "", cookies)
	do(http.MethodGet, "/who", "", cookies)
	if !seen.SudoUntil.After(time.Now()) {
		t.Fatalf("middleware did not carry the grant: %v", seen.SudoUntil)
	}

	again := do(http.MethodPost, "/auth/login", `{"email":"ops@example.com","password":"correct horse battery"}`, cookies)
	do(http.MethodGet, "/who", "", again.Result().Cookies())
	if !seen.SudoUntil.IsZero() {
		t.Errorf("sudo survived a new sign-in: %v", seen.SudoUntil)
	}
}

// A reset from the command line signs that person out everywhere: the session
// remembers the credential version it signed in with.
func TestCredentialResetEndsExistingSessions(t *testing.T) {
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	hash, _ := HashPassword("correct horse battery")
	owner, _ := st.CreateLocalOwner(t.Context(), "ops@example.com", hash)

	a := New(Config{}, st)
	signedIn := false
	mux := http.NewServeMux()
	a.Routes(mux, nil, slog.Default())
	mux.HandleFunc("GET /who", func(w http.ResponseWriter, r *http.Request) {
		_, signedIn = PrincipalFrom(r.Context())
	})
	h := a.Sessions().LoadAndSave(a.Middleware(mux))

	r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"ops@example.com","password":"correct horse battery"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	cookies := w.Result().Cookies()

	who := func() bool {
		r := httptest.NewRequest(http.MethodGet, "/who", nil)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)

		return signedIn
	}

	if !who() {
		t.Fatal("not signed in after login")
	}
	if err := st.BumpCredVersion(t.Context(), owner.ID); err != nil {
		t.Fatal(err)
	}
	if who() {
		t.Error("session still valid after a credential reset")
	}
}

// Granting sudo must not write the session back: a grant that lands after a
// logout would otherwise bring the logged-out session back to life.
func TestGrantingSudoLeavesTheSessionUntouched(t *testing.T) {
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	hash, _ := HashPassword("correct horse battery")
	owner, _ := st.CreateLocalOwner(t.Context(), "ops@example.com", hash)

	a := New(Config{}, st)
	var status scs.Status
	mux := http.NewServeMux()
	a.Routes(mux, nil, slog.Default())
	mux.HandleFunc("POST /grant", func(w http.ResponseWriter, r *http.Request) {
		_ = a.GrantSudo(r.Context(), owner.ID, time.Now().Add(5*time.Minute))
		status = a.Sessions().Status(r.Context())
	})
	h := a.Sessions().LoadAndSave(a.Middleware(mux))

	r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"ops@example.com","password":"correct horse battery"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	g := httptest.NewRequest(http.MethodPost, "/grant", nil)
	for _, c := range w.Result().Cookies() {
		g.AddCookie(c)
	}
	h.ServeHTTP(httptest.NewRecorder(), g)

	if status != scs.Unmodified {
		t.Errorf("granting sudo left the session %v, want unmodified", status)
	}
}
