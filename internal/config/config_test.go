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

func TestLoadPreservesOIDCIssuerTrailingSlash(t *testing.T) {
	t.Setenv("HEADSCALE_URL", "https://headscale.example")
	t.Setenv("HEADSCALE_API_KEY", "test-key")
	t.Setenv("HEADBOARD_PUBLIC_URL", "https://headboard.example")
	t.Setenv("OIDC_ISSUER", "https://auth.example/application/o/headscale/")
	t.Setenv("OIDC_CLIENT_ID", "headboard")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.OIDCIssuer, "https://auth.example/application/o/headscale/"; got != want {
		t.Fatalf("OIDCIssuer = %q, want %q", got, want)
	}
}

// A deployment behind a proxy at /manage has to say so in HEADBOARD_PUBLIC_URL
// anyway, because the OIDC redirect is derived from it. BasePath reads it back
// out rather than adding a second variable that could disagree.
func TestBasePathComesFromThePublicURL(t *testing.T) {
	for _, tc := range []struct{ publicURL, want string }{
		{"https://guard.example.com", ""},
		{"https://guard.example.com/", ""},
		{"https://guard.example.com/manage", "/manage"},
		{"https://guard.example.com/manage/", "/manage"},
		{"http://127.0.0.1:3000/a/b/", "/a/b"},
		{"://nonsense", ""},
	} {
		c := Config{PublicURL: tc.publicURL}
		if got := c.BasePath(); got != tc.want {
			t.Errorf("BasePath(%q) = %q, want %q", tc.publicURL, got, tc.want)
		}
	}
}
