#!/usr/bin/env bash
#
# Run the whole stack in Docker: Headscale, Postgres, the dev identity provider
# and Headboard itself.
#
#   ./dev/up.sh     # first — seeds the tailnet and writes dev/.env.dev
#   ./dev/app.sh    # then — builds and starts Headboard + the dev IdP
#   ./dev/app.sh logs
#   ./dev/app.sh down
#
# Prefer ./dev/mockoidc.sh + `go run ./cmd/headboard` while writing code: this
# path rebuilds a container on every change.

set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f compose.dev.yaml --profile app)

# The OIDC issuer is a single string the browser and the Headboard container
# must both resolve, so it cannot be localhost — inside a container that is the
# container. A real address of this machine is the one name that works in both.
HOST_IP="${HOST_IP:-$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')}"

if [[ -z "${HOST_IP}" ]]; then
  echo "could not work out this machine's IP address; set HOST_IP=<addr> and re-run" >&2
  exit 1
fi

export HOST_IP

# Every compose command against this file needs HOST_IP interpolated, so they
# go through here rather than being copied out of the banner below.
case "${1:-up}" in
  down)
    "${COMPOSE[@]}" down
    exit 0
    ;;
  logs)
    "${COMPOSE[@]}" logs -f headboard
    exit 0
    ;;
  up) ;;
  *)
    echo "usage: ./dev/app.sh [up|logs|down]" >&2
    exit 2
    ;;
esac

if [[ ! -f dev/.env.dev ]]; then
  echo "dev/.env.dev is missing — run ./dev/up.sh first, it seeds Headscale and mints the key" >&2
  exit 1
fi

# Only the API key is taken from the seeded environment. Every URL in
# compose.dev.yaml is written for container-to-container networking, and the
# host-facing values in .env.dev would point Headboard at itself.
HEADSCALE_API_KEY="$(grep '^HEADSCALE_API_KEY=' dev/.env.dev | cut -d= -f2-)"

if [[ -z "${HEADSCALE_API_KEY}" ]]; then
  echo "no HEADSCALE_API_KEY in dev/.env.dev — re-run ./dev/up.sh" >&2
  exit 1
fi

export HEADSCALE_API_KEY

"${COMPOSE[@]}" up --build -d

# An account is keyed on (issuer, subject), so the same person arriving under a
# different issuer is a different account with the default member role. The host
# loop advertises 127.0.0.1 and this one advertises HOST_IP, so a database
# carried between them signs you in as a new member rather than the owner.
#
# The database is a file in a named volume now, so starting clean is one line:
#
#   ./dev/app.sh down && docker volume rm headboard_headboard-dev-data

cat <<EOF

  Headboard   http://${HOST_IP}:3000
  identity    http://${HOST_IP}:9998/oidc

Open the first URL and sign in. Logins are served from a queue and the first one
after the IdP starts is ops@headboard.test, the owner; then alice, then bob.

Use that exact host — a different one (localhost, another IP) is a different
OIDC issuer, and accounts are keyed on issuer + subject, so you would sign in as
a brand-new member instead of the owner.

  ./dev/app.sh logs
  ./dev/app.sh down

EOF
