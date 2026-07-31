package hs

import (
	"context"
	"log/slog"
	"strings"
)

// Probe checks the server Headboard is pointed at before serving traffic.
//
// It is not a health check. Headboard compiles Headscale's policy engine in, so
// the effective-rules views it reports are only correct for the version in
// go.mod. A server on a different version can answer every request perfectly
// while Headboard quietly describes rules that server would not apply.
//
// The mismatch is a loud warning rather than a fatal error: refusing to start
// would lock an operator out of the UI at exactly the moment they need it to
// see what changed. The banner surfaces the same fact in the browser.
type Probe struct {
	// Server is the version Headscale reports.
	Server string

	// CompiledAgainst is the version Headboard was built against.
	CompiledAgainst string

	// Match is true when the two agree on major and minor. Patch releases
	// do not change policy semantics.
	Match bool
}

// CheckVersion probes the server and logs the outcome.
func CheckVersion(ctx context.Context, c Client, compiledAgainst string, log *slog.Logger) (Probe, error) {
	v, err := c.Version(ctx)
	if err != nil {
		return Probe{}, err
	}

	p := Probe{
		Server:          v.Version,
		CompiledAgainst: compiledAgainst,
		Match:           sameMinor(v.Version, compiledAgainst),
	}

	if p.Match {
		log.Info("headscale version verified",
			"server", p.Server,
			"compiledAgainst", p.CompiledAgainst,
		)

		return p, nil
	}

	log.Warn("headscale version mismatch — effective rules may not match what this server enforces",
		"server", p.Server,
		"compiledAgainst", p.CompiledAgainst,
		"fix", "rebuild Headboard against "+p.Server+", or run Headscale "+p.CompiledAgainst,
	)

	return p, nil
}

// sameMinor compares vMAJOR.MINOR, ignoring the patch component and any
// pre-release suffix. v0.29.3 and v0.29.5 agree; v0.29.3 and v0.30.0 do not.
func sameMinor(a, b string) bool {
	return minorOf(a) == minorOf(b)
}

func minorOf(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")

	// Drop any pre-release or build suffix: 0.30.0-beta.1 → 0.30.0.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}

	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return v
	}

	return parts[0] + "." + parts[1]
}
