#!/usr/bin/env bash
#
# Dev identity provider, so the OIDC login flow can be exercised without any
# external IdP.
#
# Runs on the host rather than in a container: mockoidc builds its issuer from
# the address it binds to, and a container binding 0.0.0.0 would advertise
# http://0.0.0.0:9998/oidc — unreachable from a browser and invalid for
# discovery.
#
# Lives in its own Go module (dev/mockoidc) so Headboard's dependency graph
# stays clean. Run in its own terminal; it stays in the foreground.

set -euo pipefail

cd "$(dirname "$0")/mockoidc"

export MOCKOIDC_ADDR="${MOCKOIDC_ADDR:-127.0.0.1:9998}"
export MOCKOIDC_CLIENT_ID="${MOCKOIDC_CLIENT_ID:-headboard-dev}"
export MOCKOIDC_CLIENT_SECRET="${MOCKOIDC_CLIENT_SECRET:-headboard-dev-secret}"

exec go run .
