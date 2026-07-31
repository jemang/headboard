# Golden fixtures for `internal/hs`

Real `/api/v1` responses from the throwaway Headscale v0.29.3 in
`compose.dev.yaml`. No real identities — safe to commit.

Regenerate with `./dev/fixtures.sh` after `./dev/up.sh`.

| File | Source | Covers |
|---|---|---|
| `nodes.json` | captured | 5 nodes: two owned by one user, one tagged, one with approved routes |
| `users.json` | captured | 3 users, all with empty `providerId` (the CLI-created case) |
| `policy.json` | captured | HuJSON with comments, wrapped in `{"policy": "…"}` |
| `preauthkeys.json` | captured | empty list |
| `apikeys.json` | captured | one key, hash only |
| `nodes_subnet_router.json` | **hand-authored** | `availableRoutes` / `subnetRoutes` / future expiry / `online: true` |

## Why one fixture is hand-authored

`headscale debug create-node --route` accepts routes and echoes them back in
its response, but never persists them: `DebugCreateNode` (`hscontrol/grpcv1.go`)
puts the `Hostinfo` only on the echoed `types.Node`, while the node that
actually gets stored is built later from `types.RegistrationData`, which has no
`Hostinfo` field. So a seeded node always reports `availableRoutes: []`.

Only a real Tailscale client sending `Hostinfo.RoutableIPs` in a MapRequest can
populate it — and that is exactly the field `model.go` must synthesise into
`Hostinfo`, or `NodeView.SubnetRoutes()` and `IsExitNode()` silently return
nothing. `nodes_subnet_router.json` is `ops-gw` from `nodes.json` with those
fields filled in by hand so the mapper's hardest path still has a test.

Everything else here is byte-for-byte what the server returned.
