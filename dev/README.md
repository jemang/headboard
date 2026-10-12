# Running the dev stack

Everything here talks only to the throwaway Headscale in `compose.dev.yaml`.
Never point any of it at a production tailnet.

## Quick start

Two loops. Pick one.

**All-in-Docker** — nothing to install, rebuilds a container per change:

```sh
./dev/up.sh          # Headscale + seed data, works from an empty volume
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

## Fresh volume

Since 2026-10-12 `dev/up.sh` seeds an empty volume on its own. Headscale refuses
to store a policy whose `tests` fail, and `dev/policy.hujson` asserts on `alice@`,
`ops@`, `tag:prod:443` and `tag:ci:22`, so the script orders the work:

1. a tag-owners-only bootstrap policy, so tagging is legal;
2. the nodes: `alice-laptop`, `alice-phone`, `bob-desktop`, `ops-gw`, `prod-web`, `ci-runner`;
3. the tags: `prod-web` gets `tag:prod`, `ci-runner` gets `tag:ci`;
4. the real `dev/policy.hujson`, whose tests now resolve.

Tagging moves a node off its owner, so `ci-runner` exists only to carry `tag:ci`.
`ops-gw` stays untagged, which keeps `ops@` owning a device for the `ops@` test.
Seeded nodes never report routes, because `headscale debug create-node --route`
parses the flag and then drops it. `DebugCreateNode` rebuilds the node from
`types.RegistrationData`, which has no `Hostinfo`.

## HTTPS (for the SSH key agent)

The agent and its installer refuse plain http. Put a TLS proxy in front of
`:3000` (Nginx Proxy Manager: proxy host `headboard-dev.example.com` → `http://<mac-ip>:3000`,
Let's Encrypt, Force SSL, and in Advanced `proxy_buffering off; proxy_read_timeout 1h;`
so `/api/events` streams), then start Headboard with that address:

```sh
colima ssh -- env HOST_IP=$(ipconfig getifaddr en0) HEADBOARD_PUBLIC_URL=https://headboard-dev.example.com \
  bash -c 'cd <repo> && ./dev/app.sh up'
```

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
`up.sh` starts from empty, which it handles (see "Fresh volume"). To keep the tailnet fixture and only reset Headboard's
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
