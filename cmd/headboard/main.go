// Command headboard is a web control plane for Headscale.
//
// It drives an existing Headscale over its REST API and, because Headscale's
// policy engine is imported as a library, answers "which rules apply to this
// device" with Headscale's own evaluation rather than a re-implementation.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/jemang/headboard/internal/api"
	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/config"
	"github.com/jemang/headboard/internal/hs"
	"github.com/jemang/headboard/internal/store"
	"github.com/jemang/headboard/internal/tailnet"
	"github.com/jemang/headboard/internal/web"
	"github.com/jemang/headboard/ui"
)

// headscaleTimeout bounds every call to Headscale. The poller runs on a ticker
// shorter than this, so a hung request must not outlive the interval that
// scheduled it.
const headscaleTimeout = 10 * time.Second

// version is the Headboard build version, overridden at release time with
// -ldflags "-X main.version=…".
var version = "0.1.0-dev"

// headscaleVersion is the Headscale release whose policy engine is compiled
// into this binary. It must track the `github.com/juanfont/headscale` version
// in go.mod: the policy evaluation Headboard reports is only correct for the
// server running this version.
const headscaleVersion = "v0.29.3"

// servedUnder mounts everything below a path prefix, so Headboard can share a
// hostname with Headscale behind one proxy:
//
//	https://guard.example.com/          → Headscale
//	https://guard.example.com/manage/   → Headboard
//
// The prefix is stripped here rather than threaded through the router, so every
// handler below — the API, the auth routes, the SPA — keeps seeing the paths it
// already knows. A request for the bare prefix is redirected to the trailing
// slash, because relative asset URLs resolve against the directory otherwise.
func servedUnder(basePath string, next http.Handler) http.Handler {
	if basePath == "" {
		return next
	}

	stripped := http.StripPrefix(basePath, next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == basePath {
			http.Redirect(w, r, basePath+"/", http.StatusMovedPermanently)

			return
		}

		stripped.ServeHTTP(w, r)
	})
}

// eventStreamPath is exempt from the request timeout.
const eventStreamPath = "/api/events"

func main() {
	if err := run(); err != nil {
		slog.Error("headboard exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	if cfg.Dev {
		level = slog.LevelDebug
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	// Headscale's packages log through a global zerolog, and importing its
	// policy engine imports that logger set to trace. Left alone it emits a
	// few lines per second — every poll recompiles the filter — which buries
	// Headboard's own output, including the one-time owner password.
	zerolog.SetGlobalLevel(zerolog.WarnLevel)

	if cfg.Dev {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	headscale := hs.New(cfg.HeadscaleURL, cfg.HeadscaleAPIKey, headscaleTimeout)

	// Probing before the listener opens means the version mismatch is the
	// first thing in the log, not something discovered later from wrong
	// answers. A probe failure is not fatal: Headscale may simply be starting
	// up alongside us, and the poller retries.
	probeCtx, cancelProbe := context.WithTimeout(context.Background(), headscaleTimeout)
	probe, err := hs.CheckVersion(probeCtx, headscale, headscaleVersion, log)
	cancelProbe()

	if err != nil {
		log.Warn("could not reach headscale at startup",
			"url", cfg.HeadscaleURL,
			"err", err,
		)
	}

	st, err := store.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	log.Info("database ready", "path", store.Path(cfg.DatabaseURL), "migrations", "applied")

	if err := bootstrapOwner(context.Background(), st, cfg, log); err != nil {
		return err
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	// The event stream is exempt from the request timeout: it is meant to
	// stay open. Wrapping it would cut every browser off after 30 seconds.
	router.Use(func(next http.Handler) http.Handler {
		timed := middleware.Timeout(30 * time.Second)(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == eventStreamPath {
				next.ServeHTTP(w, r)

				return
			}

			timed.ServeHTTP(w, r)
		})
	})

	// Sessions are unconditional: password login needs them, and they wrap
	// everything including the SPA, because the login and callback handlers
	// write to the same session the API reads from.
	authn := auth.New(auth.Config{
		SecureCookies:   cfg.SecureCookies(),
		SessionLifetime: cfg.SessionLifetime,
		BasePath:        cfg.BasePath(),
	}, st)

	if cfg.OIDCConfigured() {
		authCtx, cancelAuth := context.WithTimeout(context.Background(), headscaleTimeout)

		err = authn.EnableOIDC(authCtx, auth.OIDCConfig{
			Issuer:       cfg.OIDCIssuer,
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			RedirectURL:  cfg.RedirectURL(),
		})

		cancelAuth()

		if err != nil {
			return fmt.Errorf("configuring oidc: %w", err)
		}

		log.Info("oidc login enabled",
			"issuer", cfg.OIDCIssuer,
			"redirect", cfg.RedirectURL(),
			"secureCookies", cfg.SecureCookies(),
		)
	} else {
		log.Info("password login only; set OIDC_ISSUER, OIDC_CLIENT_ID and " +
			"OIDC_CLIENT_SECRET to add an identity provider")
	}

	router.Use(authn.Sessions().LoadAndSave)
	router.Use(authn.Middleware)

	authMux := http.NewServeMux()
	authn.Routes(authMux, headscale, log)
	router.Mount("/auth/", authMux)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	watcher := tailnet.New(headscale, cfg.PollInterval, log)

	go watcher.Run(ctx)
	go st.Sessions().Cleanup(ctx, time.Hour)

	api.Mount(router, api.Deps{
		Version:               version,
		HeadscaleVersion:      headscaleVersion,
		Headscale:             headscale,
		HeadscaleURL:          cfg.HeadscaleURL,
		HeadscaleAPIKeyPrefix: hs.APIKeyPrefix(cfg.HeadscaleAPIKey),
		HeadscalePublicURL:    cfg.HeadscalePublicURL,
		Mutator:               headscale,
		Probe:                 probe,
		Store:                 st,
		Tailnet:               watcher,
		OIDCEnabled:           authn.OIDCEnabled(),
		OIDCIssuer:            cfg.OIDCIssuer,
		BasePath:              cfg.BasePath(),
		Log:                   log,
	})

	// The SPA is mounted last so it only sees paths the API did not claim.
	devProxy := ""
	if cfg.Dev {
		devProxy = cfg.DevUIProxy
	}

	dist, err := ui.Dist()
	if err != nil {
		return err
	}

	spa, err := web.Handler(dist, devProxy, cfg.BasePath(), log)
	if err != nil {
		return err
	}

	router.NotFound(spa.ServeHTTP)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           servedUnder(cfg.BasePath(), router),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)

	go func() {
		log.Info("headboard listening",
			"addr", cfg.Addr,
			"headscale", cfg.HeadscaleURL,
			"headscaleVersion", headscaleVersion,
			"dev", cfg.Dev,
		)

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}

// bootstrapOwner makes sure there is always a way in.
//
// On an empty database it mints a random password for the owner and prints it
// once — the alternative is a deployment that starts cleanly and cannot be
// signed into, which is what an OIDC-only login meant in practice. With
// HEADBOARD_ADMIN_RESET it does the same for an existing local owner, which is
// the way back in when the password is lost or the identity provider changes.
func bootstrapOwner(ctx context.Context, st *store.Store, cfg config.Config, log *slog.Logger) error {
	count, err := st.CountUsers(ctx)
	if err != nil {
		return err
	}

	switch {
	case count == 0:
		password, err := auth.GeneratePassword()
		if err != nil {
			return err
		}

		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}

		user, err := st.CreateLocalOwner(ctx, cfg.AdminEmail, hash)
		if err != nil {
			return fmt.Errorf("creating the first owner: %w", err)
		}

		announce("First run. Owner account created:", user.Email, password)

	case cfg.AdminReset:
		user, err := st.LocalOwner(ctx)
		if err != nil {
			return fmt.Errorf("HEADBOARD_ADMIN_RESET: no local account to reset: %w", err)
		}

		password, err := auth.GeneratePassword()
		if err != nil {
			return err
		}

		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}

		if err := st.SetPassword(ctx, user.ID, hash); err != nil {
			return err
		}

		announce("HEADBOARD_ADMIN_RESET. Password replaced:", user.Email, password)
		log.Warn("admin password was reset; unset HEADBOARD_ADMIN_RESET before the next restart")
	}

	return nil
}

// announce writes the one-time credential to stderr directly rather than
// through slog: this has to be readable in a scrollback, and a structured log
// line wraps it in quoting that invites transcription errors.
func announce(headline, email, password string) {
	const rule = "────────────────────────────────────────────────────"

	fmt.Fprintf(os.Stderr, "\n%s\n%s\n  email:    %s\n  password: %s\n%s\n%s\n\n",
		rule, headline, email, password,
		"Shown once — change it after signing in.", rule)
}
