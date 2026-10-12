package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

type fakeSessions struct {
	granted time.Time
	ended   bool
	fail    error
}

func (f *fakeSessions) GrantSudo(_ context.Context, _ int64, until time.Time) error {
	if f.fail != nil {
		return f.fail
	}
	f.granted = until

	return nil
}

func (f *fakeSessions) EndSudo(context.Context) error {
	if f.fail != nil {
		return f.fail
	}
	f.ended = true

	return nil
}

func sudoAPI(t *testing.T) (humatest.TestAPI, *store.Store, *fakeSessions) {
	t.Helper()

	st := openAccountStore(t)
	sessions := &fakeSessions{}
	_, api := humatest.New(t)
	for _, fn := range registrations {
		fn(api, Deps{Store: st, Log: slog.Default(), Sessions: sessions})
	}

	return api, st, sessions
}

func localAdmin(t *testing.T, st *store.Store, pin string) store.User {
	t.Helper()

	hash, _ := auth.HashPassword("correct horse battery")
	u, err := st.CreateLocalOwner(t.Context(), "ops@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	if pin != "" {
		ph, _ := auth.HashPassword(pin)
		if err := st.SetPin(t.Context(), u.ID, ph); err != nil {
			t.Fatal(err)
		}
	}
	u, _ = st.UserByID(t.Context(), u.ID)

	return u
}

func code(t *testing.T, body []byte) string {
	t.Helper()

	var b struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &b)

	return b.Code
}

// The PIN is mandatory for anyone who can change things, and the server holds
// that line: skipping the setup screen with a direct request gets nothing.
func TestAdminWithoutPinIsHeldAtSetup(t *testing.T) {
	api, st, _ := sudoAPI(t)
	u := localAdmin(t, st, "")
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: u})

	resp := api.DoCtx(ctx, http.MethodGet, "/api/audit")
	if resp.Code != http.StatusForbidden || code(t, resp.Body.Bytes()) != "pin_required" {
		t.Errorf("audit without a PIN: %d %s, want 403 pin_required", resp.Code, resp.Body)
	}

	resp = api.DoCtx(ctx, http.MethodGet, "/api/me")
	var me MeBody
	_ = json.Unmarshal(resp.Body.Bytes(), &me)
	if resp.Code != http.StatusOK || !me.NeedsPin {
		t.Errorf("/api/me: %d needsPin=%v, want 200 and needsPin", resp.Code, me.NeedsPin)
	}
}

// Every dangerous action refuses before it does anything until sudo is on.
func TestDangerousActionsNeedSudo(t *testing.T) {
	api, st, _ := sudoAPI(t)
	u := localAdmin(t, st, "246813")
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: u})

	calls := []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "/api/devices/1/tags", map[string]any{"tags": []string{"tag:a"}}},
		{http.MethodPut, "/api/devices/1/routes", map[string]any{"routes": []string{}}},
		{http.MethodPost, "/api/devices/1/expire", nil},
		{http.MethodDelete, "/api/devices/1", nil},
		{http.MethodDelete, "/api/users/1", nil},
		{http.MethodPut, "/api/accounts/1/headscale-user", map[string]any{"headscaleUserId": 1}},
		{http.MethodPut, "/api/accounts/1/role", map[string]any{"role": "admin"}},
		{http.MethodPut, "/api/accounts/1/admission", map[string]any{"admission": "active"}},
		{http.MethodPut, "/api/policy", map[string]any{"hujson": "{}", "sha256": "x"}},
		{http.MethodPost, "/api/preauth-keys/revoke-active", nil},
		{http.MethodPost, "/api/preauth-keys/expire", map[string]any{"id": "1"}},
		{http.MethodPost, "/api/headscale-keys", map[string]any{}},
		{http.MethodPost, "/api/headscale-keys/abcdefghijkl/expire", nil},
		{http.MethodDelete, "/api/headscale-keys/abcdefghijkl", nil},
		{http.MethodPost, "/api/registrations/approve", map[string]any{"authId": "abcdefghijklmnop", "user": "alice"}},
		{http.MethodPut, "/api/ssh/access/app/deploy", map[string]any{"userIds": []int{1}}},
		{http.MethodPost, "/api/ssh/targets/app", map[string]any{"agentId": 1, "targetRef": "105"}},
		{http.MethodDelete, "/api/ssh/targets/app", nil},
		{http.MethodDelete, "/api/ssh/access/app", nil},
		{http.MethodPost, "/api/users/1/ssh-keys", map[string]any{"publicKey": "ssh-ed25519 AAAA", "label": "x"}},
		{http.MethodDelete, "/api/ssh-keys/1", nil},
		{http.MethodPost, "/api/ssh/agents", map[string]any{"name": "pve9"}},
		{http.MethodPost, "/api/ssh/agents/1/rotate", nil},
		{http.MethodDelete, "/api/ssh/agents/1", nil},
		{http.MethodPost, "/api/me/password", map[string]any{"current": "correct horse battery", "new": "another long password"}},
		{http.MethodPost, "/api/registrations/reject", map[string]any{"authId": "abcdefghijklmnop"}},
		{http.MethodPost, "/api/users/1/rename", map[string]any{"name": "carol"}},
	}

	for _, c := range calls {
		args := []any{}
		if c.body != nil {
			args = append(args, c.body)
		}
		resp := api.DoCtx(ctx, c.method, c.path, args...)
		if resp.Code != http.StatusForbidden || code(t, resp.Body.Bytes()) != "sudo_required" {
			t.Errorf("%s %s: %d %s, want 403 sudo_required", c.method, c.path, resp.Code, resp.Body)
		}
	}

	// With sudo on, the same caller gets through to the action itself.
	on := auth.WithPrincipal(t.Context(), auth.Principal{User: u, SudoUntil: time.Now().Add(time.Minute)})
	if resp := api.DoCtx(on, http.MethodPut, "/api/accounts/1/role", map[string]any{"role": "owner"}); resp.Code != http.StatusOK {
		t.Errorf("role change with sudo: %d %s", resp.Code, resp.Body)
	}
}

func TestSudoOpensForFiveMinutesAndLocksAfterFiveMisses(t *testing.T) {
	api, st, sessions := sudoAPI(t)
	u := localAdmin(t, st, "246813")
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: u, IP: "203.0.113.9"})

	resp := api.DoCtx(ctx, http.MethodPost, "/api/me/sudo", map[string]any{"pin": "246813"})
	if resp.Code != http.StatusOK {
		t.Fatalf("right PIN: %d %s", resp.Code, resp.Body)
	}
	if d := time.Until(sessions.granted); d < 4*time.Minute || d > 5*time.Minute {
		t.Errorf("granted for %v, want 5 minutes", d)
	}

	// Four misses say how many tries are left; the fifth says it is locked.
	for i := 1; i <= 5; i++ {
		resp = api.DoCtx(ctx, http.MethodPost, "/api/me/sudo", map[string]any{"pin": "135792"})
		want, wantCode := http.StatusUnauthorized, "pin_wrong"
		if i == 5 {
			want, wantCode = http.StatusTooManyRequests, "pin_locked"
		}
		if resp.Code != want || code(t, resp.Body.Bytes()) != wantCode {
			t.Fatalf("miss %d: %d %s, want %d %s", i, resp.Code, resp.Body, want, wantCode)
		}
	}

	sessions.granted = time.Time{}
	resp = api.DoCtx(ctx, http.MethodPost, "/api/me/sudo", map[string]any{"pin": "246813"})
	if resp.Code != http.StatusTooManyRequests || !sessions.granted.IsZero() {
		t.Errorf("right PIN while locked: %d granted=%v, want 429 and no grant", resp.Code, sessions.granted)
	}

	entries, _ := st.ListAudit(t.Context(), store.AuditFilter{Prefixes: []string{"sudo."}})
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Action]++
	}
	if counts["sudo.grant"] != 1 || counts["sudo.fail"] != 5 || counts["sudo.locked"] != 1 {
		t.Errorf("audit = %v, want 1 grant, 5 fails, 1 locked", counts)
	}
}

func TestSettingThePin(t *testing.T) {
	api, st, _ := sudoAPI(t)
	u := localAdmin(t, st, "")
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: u})

	for _, c := range []struct {
		body map[string]any
		want int
	}{
		{map[string]any{"pin": "246813", "password": "wrong"}, http.StatusUnauthorized},
		{map[string]any{"pin": "123456", "password": "correct horse battery"}, http.StatusUnprocessableEntity},
		{map[string]any{"pin": "111111", "password": "correct horse battery"}, http.StatusUnprocessableEntity},
		{map[string]any{"pin": "12345", "password": "correct horse battery"}, http.StatusUnprocessableEntity},
		{map[string]any{"pin": "246813", "password": "correct horse battery"}, http.StatusOK},
	} {
		if resp := api.DoCtx(ctx, http.MethodPost, "/api/me/pin", c.body); resp.Code != c.want {
			t.Errorf("set PIN %v: %d %s, want %d", c.body, resp.Code, resp.Body, c.want)
		}
	}

	// Changing it later needs sudo, i.e. the current PIN within five minutes.
	u, _ = st.UserByID(t.Context(), u.ID)
	ctx = auth.WithPrincipal(t.Context(), auth.Principal{User: u})
	if resp := api.DoCtx(ctx, http.MethodPost, "/api/me/pin", map[string]any{"pin": "975318", "password": "correct horse battery"}); code(t, resp.Body.Bytes()) != "sudo_required" {
		t.Errorf("change without sudo: %d %s, want sudo_required", resp.Code, resp.Body)
	}

	// An identity-provider account proves itself by signing in again.
	oidc, _ := st.UpsertLogin(t.Context(), store.User{OIDCIssuer: "https://idp.example", OIDCSubject: "net", Email: "net@example.com"})
	oidc, _ = st.SetRole(t.Context(), oidc.ID, store.RoleNetworkAdmin)
	oidc, _ = st.SetAdmission(t.Context(), oidc.ID, store.AdmissionActive)
	stale := auth.WithPrincipal(t.Context(), auth.Principal{User: oidc})
	if resp := api.DoCtx(stale, http.MethodPost, "/api/me/pin", map[string]any{"pin": "246813"}); resp.Code != http.StatusForbidden || code(t, resp.Body.Bytes()) != "reauth_required" {
		t.Errorf("OIDC without re-auth: %d %s, want 403 reauth_required", resp.Code, resp.Body)
	}
	fresh := auth.WithPrincipal(t.Context(), auth.Principal{User: oidc, ReauthAt: time.Now().Add(-time.Minute)})
	if resp := api.DoCtx(fresh, http.MethodPost, "/api/me/pin", map[string]any{"pin": "246813"}); resp.Code != http.StatusOK {
		t.Errorf("OIDC after re-auth: %d %s", resp.Code, resp.Body)
	}
}

// Guesses sent at once must not each get a try before any of them is counted:
// the five-try lock holds however the requests are timed.
func TestConcurrentWrongPinsStillLockAtFive(t *testing.T) {
	api, st, _ := sudoAPI(t)
	u := localAdmin(t, st, "246813")
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: u, IP: "203.0.113.9"})

	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			api.DoCtx(ctx, http.MethodPost, "/api/me/sudo", map[string]any{"pin": "135792"})
		}()
	}
	wg.Wait()

	entries, _ := st.ListAudit(t.Context(), store.AuditFilter{Action: "sudo.fail"})
	if len(entries) != 5 {
		t.Errorf("%d guesses were checked, want exactly 5 before the lock", len(entries))
	}
}

// Setting the first PIN checks the password; wrong passwords count toward the
// same lock, or a stolen session could guess the password forever.
func TestFirstPinPasswordGuessesLock(t *testing.T) {
	api, st, _ := sudoAPI(t)
	u := localAdmin(t, st, "")
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: u})

	var last int
	for range 6 {
		last = api.DoCtx(ctx, http.MethodPost, "/api/me/pin", map[string]any{"pin": "246813", "password": "guess"}).Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("sixth wrong password: %d, want 429", last)
	}
	if resp := api.DoCtx(ctx, http.MethodPost, "/api/me/pin", map[string]any{"pin": "246813", "password": "correct horse battery"}); resp.Code != http.StatusTooManyRequests {
		t.Errorf("right password while locked: %d, want 429", resp.Code)
	}
}

// If the grant cannot be stored, the caller must not be told sudo is on; if
// it cannot be removed, they must not be told it is off.
func TestSudoStorageFailuresAreReported(t *testing.T) {
	api, st, sessions := sudoAPI(t)
	u := localAdmin(t, st, "246813")
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: u})
	sessions.fail = errors.New("disk full")

	if resp := api.DoCtx(ctx, http.MethodPost, "/api/me/sudo", map[string]any{"pin": "246813"}); resp.Code != http.StatusInternalServerError {
		t.Errorf("grant failure: %d, want 500", resp.Code)
	}
	if resp := api.DoCtx(ctx, http.MethodDelete, "/api/me/sudo"); resp.Code != http.StatusInternalServerError {
		t.Errorf("end failure: %d, want 500", resp.Code)
	}
}
