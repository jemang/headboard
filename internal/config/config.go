// Package config loads Headboard's runtime configuration from the environment.
//
// Everything Headboard needs to reach Headscale and its own database is
// supplied by env vars so the container stays the only artifact; there is no
// config file to mount.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully-resolved configuration for one Headboard process.
type Config struct {
	// Addr is the listen address for the HTTP server.
	Addr string

	// HeadscaleURL is the base URL of the Headscale instance, without a
	// trailing slash (e.g. https://headscale.example.com).
	HeadscaleURL string

	// HeadscalePublicURL is the address *devices* use to reach Headscale, as
	// opposed to the address Headboard uses. They differ whenever Headboard
	// talks to Headscale over an internal name — a Docker network, a
	// Kubernetes service, localhost — and the enrolment command Headboard
	// prints has to carry the one a laptop can actually resolve.
	//
	// Empty means "same as HeadscaleURL", which is correct for a single-address
	// deployment and wrong in a way the Keys screen points out.
	HeadscalePublicURL string

	// HeadscaleAPIKey is an admin API key minted with
	// `headscale apikeys create`. It never leaves this process.
	HeadscaleAPIKey string

	// PollInterval is how often the tailnet snapshot is refreshed from
	// Headscale. Headscale exposes no event stream, so this is the staleness
	// bound for node online/offline state.
	PollInterval time.Duration

	// DatabaseURL is the SQLite file holding Headboard's own store (accounts,
	// sessions, audit log, policy revisions). A bare name is relative to the
	// working directory; the container image points it at the mounted volume.
	DatabaseURL string

	// PublicURL is where browsers reach Headboard, without a trailing
	// slash. The OIDC redirect is derived from it, so it must match what is
	// registered with the identity provider.
	PublicURL string

	// OIDCIssuer is the identity provider, ideally the same one Headscale
	// uses: identities are matched to Headscale users on issuer + subject,
	// which only lines up when both sides trust the same IdP.
	OIDCIssuer string

	OIDCClientID     string
	OIDCClientSecret string

	// SessionLifetime is how long a login lasts.
	SessionLifetime time.Duration

	// Dev enables development behaviour: verbose logging, and proxying
	// unmatched routes to the Vite dev server instead of serving the
	// embedded SPA.
	Dev bool

	// DevUIProxy is the optional Vite dev server to proxy to when Dev is set.
	// An explicitly empty value keeps development behaviour but serves the
	// embedded UI, which is what the all-in-Docker development stack needs.
	DevUIProxy string

	// AdminEmail names the local owner created on first run.
	AdminEmail string

	// AdminReset sets a fresh random password on the local owner at startup
	// and prints it. The way back in when the password is lost or the
	// identity provider is misconfigured.
	AdminReset bool
}

// OIDCConfigured reports whether login is available. Headboard starts without
// it so an operator can reach the console and read the configuration error,
// rather than facing a process that exits.
func (c Config) OIDCConfigured() bool {
	return c.OIDCIssuer != "" && c.OIDCClientID != ""
}

// RedirectURL is the OIDC callback Headboard serves.
func (c Config) RedirectURL() string {
	return c.PublicURL + "/auth/callback"
}

// SecureCookies reports whether the session cookie may be marked Secure, which
// depends on Headboard being reached over HTTPS.
func (c Config) SecureCookies() bool {
	return strings.HasPrefix(c.PublicURL, "https://")
}

// Load reads configuration from the environment and validates it.
func Load() (Config, error) {
	c := Config{
		Addr:             env("HEADBOARD_ADDR", ":3000"),
		HeadscaleURL:     strings.TrimRight(os.Getenv("HEADSCALE_URL"), "/"),
		HeadscaleAPIKey:  os.Getenv("HEADSCALE_API_KEY"),
		DatabaseURL:      env("DATABASE_URL", "headboard.db"),
		AdminEmail:       env("HEADBOARD_ADMIN_EMAIL", "admin@headboard.local"),
		AdminReset:       envBool("HEADBOARD_ADMIN_RESET", false),
		PublicURL:        strings.TrimRight(env("HEADBOARD_PUBLIC_URL", "http://127.0.0.1:3000"), "/"),
		OIDCIssuer:       strings.TrimRight(os.Getenv("OIDC_ISSUER"), "/"),
		OIDCClientID:     os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		Dev:              envBool("HEADBOARD_DEV", false),
		DevUIProxy:       devUIProxy(),
	}

	poll, err := time.ParseDuration(env("HEADBOARD_POLL_INTERVAL", "5s"))
	if err != nil {
		return Config{}, fmt.Errorf("HEADBOARD_POLL_INTERVAL: %w", err)
	}
	c.PollInterval = poll

	life, err := time.ParseDuration(env("HEADBOARD_SESSION_LIFETIME", "12h"))
	if err != nil {
		return Config{}, fmt.Errorf("HEADBOARD_SESSION_LIFETIME: %w", err)
	}
	c.SessionLifetime = life

	// Devices reach Headscale at the same address Headboard does unless told
	// otherwise, which is the common single-address case.
	c.HeadscalePublicURL = strings.TrimRight(os.Getenv("HEADSCALE_PUBLIC_URL"), "/")
	if c.HeadscalePublicURL == "" {
		c.HeadscalePublicURL = c.HeadscaleURL
	}

	return c, c.validate()
}

func devUIProxy() string {
	if value, ok := os.LookupEnv("HEADBOARD_DEV_UI_PROXY"); ok {
		return strings.TrimRight(value, "/")
	}

	return "http://127.0.0.1:5173"
}

func (c Config) validate() error {
	var errs []error

	if c.HeadscaleURL == "" {
		errs = append(errs, errors.New("HEADSCALE_URL is required"))
	} else if u, err := url.Parse(c.HeadscaleURL); err != nil {
		errs = append(errs, fmt.Errorf("HEADSCALE_URL: %w", err))
	} else if u.Scheme != "http" && u.Scheme != "https" {
		errs = append(errs, fmt.Errorf("HEADSCALE_URL: want http or https scheme, got %q", u.Scheme))
	}

	if c.HeadscaleAPIKey == "" {
		errs = append(errs, errors.New("HEADSCALE_API_KEY is required (headscale apikeys create)"))
	}

	if c.PollInterval < time.Second {
		errs = append(errs, fmt.Errorf("HEADBOARD_POLL_INTERVAL: %s is too aggressive, minimum 1s", c.PollInterval))
	}

	if u, err := url.Parse(c.PublicURL); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("HEADBOARD_PUBLIC_URL: %q is not an absolute URL", c.PublicURL))
	} else if !c.Dev && u.Scheme != "https" {
		errs = append(errs, errors.New("HEADBOARD_PUBLIC_URL must use https outside HEADBOARD_DEV=true"))
	}

	// Half-configured OIDC is worse than none: it looks like login should
	// work and fails at the redirect.
	if c.OIDCIssuer != "" || c.OIDCClientID != "" || c.OIDCClientSecret != "" {
		if c.OIDCIssuer == "" {
			errs = append(errs, errors.New("OIDC_ISSUER is required when any OIDC_* variable is set"))
		}

		if c.OIDCClientID == "" {
			errs = append(errs, errors.New("OIDC_CLIENT_ID is required when any OIDC_* variable is set"))
		}
	}

	return errors.Join(errs...)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}

	return b
}
