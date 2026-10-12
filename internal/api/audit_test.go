package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

func auditAPI(t *testing.T) (humatest.TestAPI, *store.Store) {
	t.Helper()

	st := openAccountStore(t)
	_, api := humatest.New(t)
	for _, fn := range registrations {
		fn(api, Deps{Store: st, Log: slog.Default()})
	}

	return api, st
}

// The client address is part of "who did it": a stolen session shows up as a
// familiar name from an unfamiliar place.
func TestAuditRecordsTheCallersAddress(t *testing.T) {
	st := openAccountStore(t)
	deps := Deps{Store: st, Log: slog.Default()}
	u, err := st.UpsertLogin(t.Context(), store.User{OIDCIssuer: "https://idp.example", OIDCSubject: "ops", Email: "ops@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	p := auth.Principal{User: u, IP: "203.0.113.7"}

	audit(t.Context(), deps, p, "device.rename", "device", 4, "web-1", nil, nil)

	entries, err := st.ListAudit(t.Context(), store.AuditFilter{})
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %v, %v", entries, err)
	}
	if entries[0].IP != "203.0.113.7" {
		t.Errorf("ip = %q, want the caller's address", entries[0].IP)
	}
}

func TestActivityListsByCategoryForViewAudit(t *testing.T) {
	api, st := auditAPI(t)
	for _, a := range []string{"device.rename", "apikey.create", "preauthkey.expire", "policy.save"} {
		if err := st.Audit(t.Context(), store.AuditEntry{ActorLabel: "ops@example.com", Action: a, TargetType: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	auditor := auth.WithPrincipal(t.Context(), auth.Principal{User: store.User{ID: 2, Role: store.RoleAuditor, Admission: store.AdmissionActive}})
	resp := api.DoCtx(auditor, http.MethodGet, "/api/audit?category=keys&limit=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("auditor: %d %s", resp.Code, resp.Body)
	}

	var body struct {
		Entries []store.AuditEntry `json:"entries"`
		Next    int64              `json:"next"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 1 || body.Entries[0].Action != "preauthkey.expire" || body.Next != body.Entries[0].ID {
		t.Errorf("first keys page = %+v next=%d, want preauthkey.expire with a cursor", body.Entries, body.Next)
	}

	resp = api.DoCtx(auditor, http.MethodGet, "/api/audit?category=keys&before="+itoa64(body.Next))
	_ = json.Unmarshal(resp.Body.Bytes(), &body)
	if len(body.Entries) != 1 || body.Entries[0].Action != "apikey.create" || body.Next != 0 {
		t.Errorf("last keys page = %+v next=%d, want apikey.create and no cursor", body.Entries, body.Next)
	}

	if resp := api.DoCtx(auditor, http.MethodGet, "/api/audit?category=nope"); resp.Code != http.StatusUnprocessableEntity {
		t.Errorf("unknown category: %d, want 422", resp.Code)
	}

	member := auth.WithPrincipal(t.Context(), auth.Principal{User: store.User{ID: 3, Role: store.RoleMember, Admission: store.AdmissionActive}})
	if resp := api.DoCtx(member, http.MethodGet, "/api/audit"); resp.Code != http.StatusForbidden {
		t.Errorf("member: %d, want 403", resp.Code)
	}
}

// A failed sign-in carries an email someone typed, which may be a real
// person's address or a password pasted into the wrong box. Only people who
// manage accounts see it whole.
func TestFailedSignInEmailIsMaskedWithoutManageUsers(t *testing.T) {
	api, st := auditAPI(t)
	if err := st.Audit(t.Context(), store.AuditEntry{ActorLabel: "alice@example.com", Action: "auth.login_failed", TargetType: "session"}); err != nil {
		t.Fatal(err)
	}

	label := func(role store.Role) string {
		ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: store.User{ID: 2, Role: role, Admission: store.AdmissionActive, PinHash: "set"}})
		resp := api.DoCtx(ctx, http.MethodGet, "/api/audit")
		var body struct {
			Entries []store.AuditEntry `json:"entries"`
		}
		_ = json.Unmarshal(resp.Body.Bytes(), &body)
		if len(body.Entries) != 1 {
			t.Fatalf("%s: %s", role, resp.Body)
		}

		return body.Entries[0].ActorLabel
	}

	if got := label(store.RoleAuditor); got != "a***@example.com" {
		t.Errorf("auditor sees %q, want a***@example.com", got)
	}
	if got := label(store.RoleAdmin); got != "alice@example.com" {
		t.Errorf("admin sees %q, want the full address", got)
	}
}

func TestMaskEmail(t *testing.T) {
	for in, want := range map[string]string{
		"alice@example.com": "a***@example.com",
		"x":                 "***",
		"":                  "",
		"not-an-email":      "***",
	} {
		if got := maskEmail(in); got != want {
			t.Errorf("maskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
