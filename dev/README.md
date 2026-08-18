# Running the dev stack

Everything here talks only to the throwaway Headscale in `compose.dev.yaml`.
Never point any of it at a production tailnet.

## Quick start

Two loops. Pick one.

**All-in-Docker** — nothing to install, rebuilds a container per change:

```sh
./dev/up.sh          # Headscale + seed data (see "Fresh volume" if this fails)
./dev/app.sh up      # Headboard + dev IdP, built from this checkout
./dev/app.sh logs
./dev/app.sh down
```

Then open the URL `app.sh` prints — `http://<your-LAN-IP>:3000`. **Use that exact
host.** Accounts are keyed on (OIDC issuer, subject), and the issuer is built from
the host you browse to, so `localhost` signs you in as a brand-new member instead of
the owner.

**Host loop** — fast edit/build cycle, Headscale stays in Docker:

```sh
./dev/up.sh
./dev/mockoidc.sh                                                  # terminal 1
env $(grep -v '^#' dev/.env.dev | xargs) go run ./cmd/headboard    # terminal 2
```

Add `cd ui && pnpm dev` in a third terminal for UI hot reload (`HEADBOARD_DEV=true`
proxies unmatched routes to Vite, so the browser still sees one origin on :3000).

## Verify it is actually up

```sh
curl -s http://127.0.0.1:3000/api/health
```

Want `"headscaleState":"connected"` and `"headscaleVersionMatch":true`. Anything
else, read the logs — `docker logs headboard-headboard-1 --tail 20` (Docker loop)
or the `go run` terminal.

`"headscaleState":"unavailable"` with `acl policy not found` in the logs means
Headscale is running but never got seeded. That is the next section.

## Fresh volume: `dev/up.sh` cannot seed from empty on its own

Confirmed 2026-08-18. `up.sh` applies `dev/policy.hujson` *before* it seeds nodes,
but that policy's `tests` block asserts on `alice@`, `ops@`, `tag:prod:443` and
`tag:ci:22` — and Headscale refuses to store any policy whose tests fail. With no
nodes yet, every assertion resolves to no IP addresses, `policy set` errors, and
`set -e` kills the script before it reaches the node seeding. Re-running does not
help: it dies at the same line.

Order that works — nodes first, then a minimal policy so tagging is legal, then
tags, then the real policy:

```sh
set -euo pipefail
HS=(docker compose -f compose.dev.yaml exec -T headscale-dev headscale)

docker compose -f compose.dev.yaml up -d --wait
for u in alice bob ops; do "${HS[@]}" users create "$u" --email "$u@headboard.test" || true; done

# 1. Nodes. No real Tailscale client needed: register a pending auth request,
#    then complete it. --route is silently dropped, so routes stay empty.
seed() {
  id="hskey-authreq-$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24)"
  "${HS[@]}" debug create-node --name "$1" --user "$2" --key "$id" >/dev/null
  "${HS[@]}" auth register --user "$2" --auth-id "$id" >/dev/null
  echo "seeded $1 -> $2"
}
seed alice-laptop alice
seed alice-phone  alice
seed bob-desktop  bob
seed ops-gw       ops
seed prod-web     ops
seed ops-admin    ops   # see "why ops-admin" below

# 2. tagOwners-only policy, so the tags below are allowed to exist.
cat >/tmp/bootstrap.hujson <<'EOF'
{
  "tagOwners": { "tag:prod": ["ops@"], "tag:ci": ["ops@"] },
  "acls": [ { "action": "accept", "src": ["*"], "dst": ["*:*"] } ],
}
EOF
docker compose -f compose.dev.yaml cp /tmp/bootstrap.hujson headscale-dev:/tmp/bootstrap.hujson
"${HS[@]}" policy set --file /tmp/bootstrap.hujson

# 3. Tags. IRREVERSIBLE — Headscale will not let a tagged node go back to an owner.
nid() { "${HS[@]}" nodes list --output json | python3 -c "
import json,sys
for n in json.load(sys.stdin) or []:
    if n.get('name')==sys.argv[1]: print(n['id']); break" "$1"; }
"${HS[@]}" nodes tag --identifier "$(nid prod-web)" --tags tag:prod
"${HS[@]}" nodes tag --identifier "$(nid ops-gw)"   --tags tag:ci

# 4. The real policy. Its tests now resolve, so the write is accepted.
./dev/up.sh
```

The final `./dev/up.sh` is idempotent — it skips the existing users and nodes,
applies `dev/policy.hujson`, approves routes, and mints the API key into
`dev/.env.dev`.

### Why `ops-admin`

Tagging moves a node off its owner and onto the sentinel `tagged-devices` user.
`ops-gw` and `prod-web` are the only two `ops` nodes `up.sh` seeds, so tagging both
leaves `ops@` owning nothing — and the policy's `ops@` test then fails with
`source "ops@" resolved to no IP addresses`. `ops-admin` is an untagged `ops` device
that exists purely to keep that assertion resolvable.

`tag:ci` has to live on a node for the same reason (`ops@: accept tag:ci:22`), which
is why `ops-gw` carries it. It is a gateway in name only — seeded nodes never report
routes, because `headscale debug create-node --route` parses the flag and then drops
it (`DebugCreateNode` rebuilds the node from `types.RegistrationData`, which has no
`Hostinfo`).

## Signing in

`dev/mockoidc` hands out identities from a queue, in order, one per login:

1. `ops@headboard.test` — **the owner**
2. `alice@headboard.test`
3. `bob@headboard.test`

So the first sign-in after the IdP starts should be the one you want to be admin.
Restarting the IdP resets the queue.

There is also a local password account (`admin@headboard.local`). Mint a password by
starting Headboard once with `HEADBOARD_ADMIN_RESET=1`, then take that env var back
out — do not leave it in `compose.dev.yaml`.

## Starting over

```sh
./dev/app.sh down     # stop Headboard + IdP first, or the volumes are still in use
./dev/down.sh         # compose down --volumes: destroys BOTH volumes and dev/.env.dev
rm -f dev/headboard.db  # host loop's own store, if you used that loop
```

`down.sh` takes the Headscale data *and* Headboard's own store with it, so the next
`up.sh` starts from empty — which means the "Fresh volume" bootstrap above, not a
plain `./dev/up.sh`. To keep the tailnet fixture and only reset Headboard's
accounts, drop just its store (`rm -f dev/headboard.db`, or
`docker volume rm headboard_headboard-dev-data` for the Docker loop) and redo the
first-login-is-owner dance.

`./dev/down.sh` destroys data. Ask before running it against a stack someone is using.

## Troubleshooting

| Symptom | Cause |
|---|---|
| `acl policy not found`, polled every 5s | Headscale unseeded → "Fresh volume" above |
| `test(s) failed: ... resolved to no IP addresses` | Policy applied before nodes/tags exist → same section |
| `dst alias "tag:prod" resolved to no nodes` | No node carries the tag yet → step 3 above |
| Signed in as a new member, not the owner | Browsed a different host than the issuer (`localhost` vs LAN IP) |
| `Expired: yes` on every seeded node in `headscale nodes list` | Cosmetic. A never-set expiry serialises as `0001-01-01`; the CLI misreads it, Headboard does not |
| `dev/app.sh`: could not work out this machine's IP | Set `HOST_IP=<addr>` and re-run |
| `GET /api/v1/node: 401: Unauthorized` | `dev/.env.dev` holds a key minted against a destroyed volume. `up.sh` keeps an existing `HEADSCALE_API_KEY` line and never notices it is dead — mint a new one (`headscale apikeys create --expiration 90d`), replace that line, restart |
| `writing session: attempt to write a readonly database (8)`, logins never stick | Something wrote into the `/data` volume as the wrong owner. The container runs as uid 10001; `docker cp` stamps the host uid instead. Fix with `docker run --rm -v headboard_headboard-dev-data:/data alpine:3 chown -R 10001:10001 /data` |

Editing Headboard's store directly (approving an account, changing a role while you have
no owner password) is best done *inside* the volume, never through `docker cp` — cp both
breaks ownership and leaves the old WAL behind, which replays over your edit:

```sh
docker compose -f compose.dev.yaml --profile app stop headboard
docker run --rm -v headboard_headboard-dev-data:/data alpine:3 sh -c \
  'apk add --no-cache sqlite >/dev/null && sqlite3 /data/headboard.db "select id,email,role,admission from users;"'
HOST_IP=<addr> docker compose -f compose.dev.yaml --profile app start headboard
```

Such an edit bypasses `audit_log`, so it is a dev-loop shortcut only.
