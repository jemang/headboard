package api

import (
	"testing"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

func TestRegistrationInfoRequiresDeviceManagement(t *testing.T) {
	deps := Deps{HeadscalePublicURL: "https://hs.example"}
	owner := auth.Principal{User: store.User{Role: store.RoleOwner, Admission: store.AdmissionActive}}

	info, err := registrationInfo(auth.WithPrincipal(t.Context(), owner), deps)
	if err != nil {
		t.Fatalf("owner registration info: %v", err)
	}
	if info.HeadscalePublicURL != "https://hs.example" {
		t.Errorf("headscale public URL = %q, want %q", info.HeadscalePublicURL, "https://hs.example")
	}

	member := auth.Principal{User: store.User{Role: store.RoleMember, Admission: store.AdmissionActive}}
	if _, err := registrationInfo(auth.WithPrincipal(t.Context(), member), deps); err == nil {
		t.Error("member received registration info")
	}
}
