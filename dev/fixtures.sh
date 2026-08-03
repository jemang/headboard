#!/usr/bin/env bash
#
# Capture golden fixtures for the internal/hs mapper (T2) from the throwaway
# Headscale into internal/hs/testdata/.
#
# GET only — this script never writes to Headscale. It defaults to the dev
# container and refuses to run without dev/.env.dev, so it cannot be pointed at
# production by accident.

set -euo pipefail

cd "$(dirname "$0")/.."

ENV_FILE=dev/.env.dev
OUT=internal/hs/testdata

if [[ ! -f "$ENV_FILE" ]]; then
  echo "missing $ENV_FILE — run ./dev/up.sh first" >&2
  exit 1
fi

# shellcheck disable=SC1090
set -a; source "$ENV_FILE"; set +a

case "$HEADSCALE_URL" in
  http://127.0.0.1:*|http://localhost:*) ;;
  *)
    echo "refusing to run: $ENV_FILE points at $HEADSCALE_URL, not the dev container" >&2
    exit 1
    ;;
esac

mkdir -p "$OUT"

get() {
  curl -fsS -H "Authorization: Bearer $HEADSCALE_API_KEY" \
    "$HEADSCALE_URL/api/v1/$1"
}

capture() {
  local path=$1 file=$2
  get "$path" | python3 -m json.tool >"$OUT/$file"
  printf '\033[1;36m==>\033[0m %-28s → %s\n' "/api/v1/$path" "$OUT/$file"
}

capture node          nodes.json
capture user          users.json
capture policy        policy.json
capture preauthkey    preauthkeys.json
capture apikey        apikeys.json

cat <<EOF

Fixtures written to $OUT/.

These come from the throwaway instance, so they contain no real identities and
are safe to commit. The mapper's table tests read them directly.
EOF
