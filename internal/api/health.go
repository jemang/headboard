package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// HealthBody is the payload of GET /api/health.
type HealthBody struct {
	Status string `json:"status" doc:"Always \"ok\" when the process is serving." example:"ok"`

	Version string `json:"version" doc:"Headboard build version." example:"0.1.0-dev"`

	// HeadscaleVersion is the version of Headscale that Headboard's policy
	// engine was compiled against, not the version of the server it is
	// pointed at.
	HeadscaleVersion string `json:"headscaleVersion" doc:"Headscale version Headboard was built against." example:"v0.29.3"`

	// HeadscaleServerVersion is what the server actually reports. It can be
	// empty when the probe could not reach Headscale at startup.
	HeadscaleServerVersion string `json:"headscaleServerVersion" doc:"Headscale version reported by the server." example:"v0.29.3"`

	// HeadscaleVersionMatch is false when the two disagree on minor version.
	// Headboard still serves, but its effective-rules views are computed by
	// a different policy engine than the one enforcing traffic, so the UI
	// shows a banner rather than presenting them as authoritative.
	HeadscaleVersionMatch bool `json:"headscaleVersionMatch" doc:"Whether the server's version matches the compiled-in policy engine."`
}

type healthOutput struct {
	Body HealthBody
}

func init() {
	register(func(api huma.API, deps Deps) {
		huma.Register(api, huma.Operation{
			OperationID: "health",
			Method:      http.MethodGet,
			Path:        "/api/health",
			Summary:     "Liveness and build information",
			Tags:        []string{"meta"},
		}, func(ctx context.Context, _ *struct{}) (*healthOutput, error) {
			return &healthOutput{Body: HealthBody{
				Status:                 "ok",
				Version:                deps.Version,
				HeadscaleVersion:       deps.HeadscaleVersion,
				HeadscaleServerVersion: deps.Probe.Server,
				HeadscaleVersionMatch:  deps.Probe.Match,
			}}, nil
		})
	})
}
