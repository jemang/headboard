package config

import (
	"testing"
	"time"
)

func TestValidatePublicURLRequiresHTTPSOutsideDevelopment(t *testing.T) {
	for _, tt := range []struct {
		name      string
		publicURL string
		dev       bool
		wantErr   bool
	}{
		{"production http", "http://headboard.example", false, true},
		{"production https", "https://headboard.example", false, false},
		{"development http", "http://127.0.0.1:3000", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				HeadscaleURL:    "https://headscale.example",
				HeadscaleAPIKey: "test-key",
				PollInterval:    5 * time.Second,
				PublicURL:       tt.publicURL,
				Dev:             tt.dev,
			}
			if err := cfg.validate(); (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestDevUIProxyAllowsExplicitEmptyOverride(t *testing.T) {
	t.Setenv("HEADBOARD_DEV_UI_PROXY", "")
	if got := devUIProxy(); got != "" {
		t.Fatalf("devUIProxy() = %q, want embedded UI", got)
	}
}
