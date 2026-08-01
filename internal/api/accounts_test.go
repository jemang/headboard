package api

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

func openAccountStore(t *testing.T) *store.Store {
	t.Helper()

	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "headboard.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(st.Close)

	return st
}

func TestSetAccountAdmissionRequiresUserManagement(t *testing.T) {
	st := openAccountStore(t)
	ctx := t.Context()

	owner, err := st.UpsertLogin(ctx, store.User{OIDCIssuer: "https://idp.example", OIDCSubject: "owner", Email: "owner@example.com"})
	if err != nil {
		t.Fatalf("owner login: %v", err)
	}
	target, err := st.UpsertLogin(ctx, store.User{OIDCIssuer: "https://idp.example", OIDCSubject: "member", Email: "member@example.com"})
	if err != nil {
		t.Fatalf("member login: %v", err)
	}
	deps := Deps{Store: st, Log: slog.Default()}

	approved, err := setAccountAdmission(auth.WithPrincipal(ctx, auth.Principal{User: owner}), deps, target.ID, store.AdmissionActive)
	if err != nil {
		t.Fatalf("owner approval: %v", err)
	}
	if approved.Admission != store.AdmissionActive {
		t.Errorf("admission = %s, want active", approved.Admission)
	}

	networkAdmin := owner
	networkAdmin.Role = store.RoleNetworkAdmin
	if _, err := setAccountAdmission(auth.WithPrincipal(ctx, auth.Principal{User: networkAdmin}), deps, target.ID, store.AdmissionRejected); err == nil {
		t.Error("network admin changed account admission")
	}
}
