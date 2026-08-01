// Package api exposes Headboard's own HTTP API.
//
// It is deliberately separate from the Headscale API: the browser talks only to
// these routes, and the Headscale admin key never leaves the server. Handlers
// are declared with Huma so the OpenAPI document at /api/openapi is generated
// from the same types the handlers use, rather than maintained by hand.
package api

import (
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/jemang/headboard/internal/hs"
	"github.com/jemang/headboard/internal/store"
	"github.com/jemang/headboard/internal/tailnet"
)

// Deps is the dependency surface the API needs. It grows as milestones land
// (T3 adds the policy manager, T4 auth and store).
type Deps struct {
	// Version is Headboard's build version, reported by /api/health.
	Version string

	// HeadscaleVersion is the version Headboard was compiled against. The
	// policy engine is imported from Headscale as a library, so a server
	// running a different version is a real correctness risk, not a
	// cosmetic mismatch.
	HeadscaleVersion string

	// Headscale is the read client for the server this process drives.
	Headscale hs.Client

	// HeadscaleURL is how this process reaches Headscale.
	HeadscaleURL string

	// HeadscalePublicURL is how *devices* reach it, which is not always the
	// same address. It is what goes into the generated tailscale up command;
	// never the API key.
	HeadscalePublicURL string

	// Mutator is the write client. Separate from Headscale so a handler
	// that only reads cannot accidentally write.
	Mutator hs.Mutator

	// Probe is the result of the startup version check.
	Probe hs.Probe

	// Store is Headboard's own database.
	Store *store.Store

	// Tailnet is the polled view of Headscale. Handlers read from its
	// current snapshot rather than calling Headscale per request, so a
	// device list and the rules beside it describe the same instant.
	Tailnet *tailnet.Watcher

	// OIDCEnabled reports whether login is configured, so the login screen
	// can say what is wrong instead of failing at the redirect.
	OIDCEnabled bool

	// OIDCIssuer is shown on the login screen to confirm which identity
	// provider a person is about to be sent to.
	OIDCIssuer string

	Log *slog.Logger
}

// registrations is populated by each resource file's init, so adding a resource
// means adding a file rather than editing a shared list.
var registrations []func(huma.API, Deps)

// register is called from each resource file's init.
func register(fn func(huma.API, Deps)) {
	registrations = append(registrations, fn)
}

// Config returns the Huma configuration shared by the server and tests.
func Config(version string) huma.Config {
	cfg := huma.DefaultConfig("Headboard API", version)
	cfg.Info.Description = "Headboard's own API. The browser talks to this; " +
		"Headscale is reached server-side only."
	cfg.OpenAPIPath = "/api/openapi"
	cfg.DocsPath = "/api/docs"

	// Keep "$schema" out of response bodies: the UI is the only consumer and
	// the OpenAPI document already describes every shape.
	cfg.SchemasPath = ""
	cfg.CreateHooks = nil

	return cfg
}

// Mount builds the Huma API on router and registers every operation.
func Mount(router chi.Router, deps Deps) huma.API {
	api := humachi.New(router, Config(deps.Version))

	for _, fn := range registrations {
		fn(api, deps)
	}

	// SSE is mounted directly rather than through Huma: the response is an
	// open stream, not a body, and OpenAPI has nothing useful to say about
	// it. The chi timeout middleware is skipped for the same reason.
	if deps.Tailnet != nil {
		router.Get("/api/events", EventsHandler(deps.Tailnet))
	}

	return api
}
