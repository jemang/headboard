# Headboard

A web control plane for [Headscale](https://headscale.net) — a modern UI with a real ACL
editor, logins for *every* user rather than one shared admin, and a self-service portal where
people can see their own devices and the rules that actually apply to them.

Runs as a single container against your existing Headscale. No database server, and no identity
provider unless you want one.

## Why not Headplane

[Headplane](https://github.com/tale/headplane) covers admin basics, but its ACL editor is a raw
HuJSON textarea (disabled entirely outside `policy.mode = database`), one OIDC identity gets full
admin, and a normal user cannot log in at all. Headboard targets exactly those gaps.

## The idea

Headscale's policy engine is an importable Go package. Headboard reads policy, users and nodes
over the Headscale REST API, feeds them to Headscale's own `PolicyManager`, and reads the answers
back:

| Question | Answered by |
| --- | --- |
| Who can reach this device? | `FilterForNode(node)` |
| What can this device reach? | `FilterForNode` over `BuildPeerMap` peers |
| Which peers does it see? | `BuildPeerMap(nodes)` |
| Can A reach B on port N? | the destination's own filter, ports checked against `DstPorts` |
| Does the policy still hold? | its own `tests` / `sshTests` blocks |

Nothing about ACL semantics is re-implemented, so alias expansion, `autogroup:self`, tag
resolution and rule reduction stay correct on the day Headscale changes them.

**The trade:** Headboard is compiled against one Headscale version (`v0.29.3`). Upgrade the server
and the `go.mod` pin together.

## Requirements

- Headscale **v0.29.x** with `policy.mode = database` (ACL writes over the API need it)
- An admin API key: `headscale apikeys create --expiration 90d`
- Docker, or Go 1.26 + Node 22 + pnpm to build from source

Nothing else. Headboard's own store is a SQLite file, and it creates its first administrator
itself — an identity provider is optional and can be added later.

## Run

```sh
cp .env.example .env   # HEADSCALE_URL and HEADSCALE_API_KEY are the only required values
docker compose up --build
```

On the first start Headboard creates an owner account and prints its password **once**:

```
$ docker compose logs headboard
────────────────────────────────────────────────────
First run. Owner account created:
  email:    admin@headboard.local
  password: 3qow-i3wv-ufqx-hmdz-vx7v
Shown once — change it after signing in.
────────────────────────────────────────────────────
```

Open <http://localhost:3000>, sign in with that, and change it under *Account*.

Lost it? Start once with `HEADBOARD_ADMIN_RESET=1` and a fresh password is printed the same way.
Unset it afterwards.

> The database lives in the `headboard-data` volume. Delete it and the next start is a first run
> again: new password, empty audit log. Your tailnet is untouched — Headscale remains the source of
> truth for devices, users and the policy.

### Configuration

Everything is environment variables. The ones without a default must be set.

| Variable | Default | What it does |
| --- | --- | --- |
| `HEADSCALE_URL` | — | Base URL of your Headscale, no trailing slash |
| `HEADSCALE_API_KEY` | — | Admin key. Stays in the server process; the browser never sees it |
| `DATABASE_URL` | `headboard.db` | SQLite file for Headboard's own store. The image points it at `/data/headboard.db` |
| `HEADBOARD_ADMIN_EMAIL` | `admin@headboard.local` | The owner created on first run |
| `HEADBOARD_ADMIN_RESET` | `false` | Mint and print a new owner password at startup |
| `HEADBOARD_PUBLIC_URL` | `http://127.0.0.1:3000` | Where browsers reach Headboard. The OIDC redirect is derived from it |
| `OIDC_ISSUER` | — | Optional. Same issuer as Headscale — see below |
| `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | — | Client registered with that issuer |
| `HEADBOARD_POLL_INTERVAL` | `5s` | Headscale has no event stream; this is the staleness bound |
| `HEADBOARD_SESSION_LIFETIME` | `12h` | How long a login lasts |
| `HEADBOARD_ADDR` | `:3000` | Listen address |

### Adding an identity provider

Password sign-in keeps working after this; the two sit side by side on the login screen.

Register `<HEADBOARD_PUBLIC_URL>/auth/callback` as a redirect URI, then set `OIDC_ISSUER`,
`OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET`.

**Use the same issuer as Headscale.** Headboard matches an account to its Headscale user on
`(iss, sub)` — the pair Headscale stores as `provider_identifier`. Different issuers on the two
sides means no automatic match, and every account has to be linked by hand under *People*.

With **Authentik** that is easy to get wrong, because its issuer is per-provider by default and
shaped `https://authentik.example.com/application/o/<app-slug>/`. Two applications — one for
Headscale, one for Headboard — are two different issuers. Either point both at one provider (a
provider accepts several redirect URIs), or set both providers to the same *issuer mode* and
*subject mode*. Check those names against your Authentik version; they have moved between releases.

> **Pick `OIDC_ISSUER` once and do not change it.** An account is identified by issuer + subject,
> not by email, so changing the issuer URL signs everyone in as a *new* member account — owner
> included. The old rows remain but can no longer be reached. Recovery is
> `HEADBOARD_ADMIN_RESET=1`, which is exactly why the local owner exists.

### Roles

| Role | Can |
| --- | --- |
| `owner` | Everything, including changing roles |
| `admin` | Everything except roles |
| `network-admin` | Devices and the policy, not people or keys |
| `auditor` | Read everything, change nothing |
| `member` | Their own devices, and enrol new ones |

A member sees only machines they own — probing another device's id returns 404, not 403, so the
tailnet's shape is not leaked by the error.

## What it does

- **ACL builder.** Rules, groups, tags, hosts, SSH, auto-approvers and tests as forms, plus the raw
  HuJSON. Edits are applied as RFC-6902 patches to the document's syntax tree, so **comments and
  layout outside the field you changed stay byte-identical**. Nothing saves without showing the
  real diff and Headscale's own validation first, and a write is refused if the document changed
  underneath you.
- **Tests.** The policy's `tests` / `sshTests` run on demand, per assertion, against the live
  tailnet or a pending change. Worth knowing: Headscale *refuses to store* a policy whose tests
  fail, so a failing assertion is a blocked save, not a warning.
- **Effective rules.** For any device: who can reach it, what it can reach, and which peers it
  sees — computed by Headscale's engine, not an approximation.
- **Simulator.** "Can A reach B on port N", answered against the destination's own filter (so
  `autogroup:self` rules count), with a link to the policy line responsible.
- **The usual admin.** Devices, routes, users, pre-auth keys with an install command, API keys,
  and an audit log of every change.

Press <kbd>⌘K</kbd> for pages and devices.

## Develop

The UI is embedded into the binary at compile time, so a source build needs the UI built first:

```sh
cd ui && pnpm install && pnpm build && cd ..
go build ./cmd/headboard
go test ./...
```

Toolchain versions are pinned in `.tool-versions`; run through `mise exec --`.

### A Headscale to develop against

Don't point the dev loop at a real tailnet — a Headscale API key is all-access,
with no read-only scope. `compose.dev.yaml` runs a throwaway instance instead:

```sh
./dev/up.sh          # start, seed users/nodes/tags/routes/policy, write dev/.env.dev
./dev/mockoidc.sh    # a dev identity provider, in its own terminal
./dev/fixtures.sh    # GET-only capture of /api/v1 responses into internal/hs/testdata/
./dev/down.sh        # destroy it, volume and all
```

`dev/up.sh` seeds three users, five nodes (one tagged, one gateway with an approved subnet route
and exit node) and a deliberately comment-heavy policy with a `tests` block, then prints an API key
into `dev/.env.dev`. That file is gitignored and worthless outside the container. Headboard's own
store is `dev/headboard.db` — delete the file to start over.
`dev/fixtures.sh` refuses to run against anything but loopback, so it cannot be aimed at production
by accident.

### Hot reload

Run the two halves separately — `HEADBOARD_DEV=true` makes the Go server proxy unmatched routes to
Vite, keeping one origin in the browser:

```sh
cd ui && pnpm dev                                          # terminal 1, :5173
env $(grep -v '^#' dev/.env.dev | xargs) go run ./cmd/headboard   # terminal 2, :3000
```

API docs are served from the running binary at `/api/docs`.

## Status

All planned milestones are implemented and exercised against a local Headscale v0.29.3, both from
source and from the release container. Not yet run against a production tailnet. See [`.doc/`](.doc/) for the plan and
[`.claude/skills/project-conventions/`](.claude/skills/project-conventions/SKILL.md) for the
gotchas worth reading before changing `internal/`.
