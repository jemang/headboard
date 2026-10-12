package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

func resetStore(t *testing.T) (*store.Store, store.User) {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)

	hash, _ := auth.HashPassword("old password here")
	owner, err := st.CreateLocalOwner(t.Context(), "ops@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	pin, _ := auth.HashPassword("246813")
	_ = st.SetPin(t.Context(), owner.ID, pin)

	return st, owner
}

func TestResetPasswordPrintsANewOneAndSignsOut(t *testing.T) {
	st, owner := resetStore(t)
	var out bytes.Buffer

	if err := reset(t.Context(), st, "reset-password", []string{"--email", "OPS@example.com"}, &out); err != nil {
		t.Fatal(err)
	}

	got, _ := st.UserByID(t.Context(), owner.ID)
	fields := strings.Fields(out.String())
	var printed string
	for i, f := range fields {
		if f == "password:" && i+1 < len(fields) {
			printed = fields[i+1]
		}
	}
	if printed == "" || !auth.VerifyPassword(got.PasswordHash, printed) {
		t.Errorf("printed password does not sign in; output:\n%s", out.String())
	}
	if got.CredVersion != owner.CredVersion+1 {
		t.Error("existing sessions were not signed out")
	}

	entries, _ := st.ListAudit(t.Context(), store.AuditFilter{Action: "account.password_reset"})
	if len(entries) != 1 || entries[0].ActorLabel != "cli" || entries[0].TargetLabel != "ops@example.com" {
		t.Errorf("audit = %+v", entries)
	}
	if strings.Contains(string(entries[0].After), printed) {
		t.Error("the new password was written to the audit log")
	}
}

func TestResetPinClearsItSoTheGateAsksAgain(t *testing.T) {
	st, owner := resetStore(t)
	var out bytes.Buffer

	if err := reset(t.Context(), st, "reset-pin", []string{"--email", "ops@example.com"}, &out); err != nil {
		t.Fatal(err)
	}

	got, _ := st.UserByID(t.Context(), owner.ID)
	if got.PinHash != "" || got.CredVersion != owner.CredVersion+1 {
		t.Errorf("pin=%q version=%d", got.PinHash, got.CredVersion)
	}
	if e, _ := st.ListAudit(t.Context(), store.AuditFilter{Action: "account.pin_reset"}); len(e) != 1 {
		t.Errorf("pin reset not audited: %v", e)
	}
}

func TestResetRefusesWhatItCannotDoSafely(t *testing.T) {
	st, _ := resetStore(t)
	oidc, _ := st.UpsertLogin(t.Context(), store.User{OIDCIssuer: "https://idp.example", OIDCSubject: "x", Email: "ops@example.com"})
	var out bytes.Buffer

	// Two accounts share the email: a PIN reset must be told which one.
	err := reset(t.Context(), st, "reset-pin", []string{"--email", "ops@example.com"}, &out)
	if err == nil || !strings.Contains(err.Error(), "--id") {
		t.Errorf("ambiguous email: %v, want a pointer to --id", err)
	}

	// An identity-provider account has no password here to reset.
	err = reset(t.Context(), st, "reset-password", []string{"--id", itoa(oidc.ID)}, &out)
	if err == nil || !strings.Contains(err.Error(), "identity provider") {
		t.Errorf("OIDC password reset: %v, want a refusal", err)
	}

	if err := reset(t.Context(), st, "reset-pin", []string{"--email", "nobody@example.com"}, &out); err == nil {
		t.Error("reset a PIN for an email no account has")
	}
	if err := reset(t.Context(), st, "reset-pin", nil, &out); err == nil {
		t.Error("ran without --email or --id")
	}
}
