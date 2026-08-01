package api

import (
	"strings"
	"testing"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

func TestRequireBlocksAccountsAwaitingAdmission(t *testing.T) {
	for _, tt := range []struct {
		name      string
		admission store.AdmissionState
		want      string
	}{
		{"pending", store.AdmissionPending, "account approval is pending"},
		{"rejected", store.AdmissionRejected, "account access was rejected"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: store.User{
				Role:      store.RoleOwner,
				Admission: tt.admission,
			}})

			_, err := require(ctx, auth.CapManageUsers)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("require error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRequireAuthenticatedAllowsPendingAccountToReadItsStatus(t *testing.T) {
	ctx := auth.WithPrincipal(t.Context(), auth.Principal{User: store.User{
		Role:      store.RoleMember,
		Admission: store.AdmissionPending,
	}})

	p, err := requireAuthenticated(ctx)
	if err != nil {
		t.Fatalf("requireAuthenticated: %v", err)
	}
	if p.User.Admission != store.AdmissionPending {
		t.Errorf("admission = %s, want pending", p.User.Admission)
	}
}

func TestMeBodyForPendingAccountHasNoCapabilities(t *testing.T) {
	body := meBodyFor(auth.Principal{User: store.User{
		Role:      store.RoleOwner,
		Admission: store.AdmissionPending,
	}})

	if body.Admission != store.AdmissionPending {
		t.Errorf("admission = %s, want pending", body.Admission)
	}
	if len(body.Capabilities) != 0 {
		t.Errorf("pending capabilities = %v, want none", body.Capabilities)
	}
}
