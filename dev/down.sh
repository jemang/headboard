#!/usr/bin/env bash
#
# Destroy the throwaway Headscale, including its volume and the generated
# dev/.env.dev. Only ever touches compose.dev.yaml resources.

set -euo pipefail

cd "$(dirname "$0")/.."

docker compose -f compose.dev.yaml down --volumes
rm -f dev/.env.dev

printf '\033[1;36m==>\033[0m dev Headscale destroyed (volume + dev/.env.dev removed)\n'
