# ADR-002: Workspace Host Registry via gossip

**Status**: Accepted

Each stapler-squad install mints a durable `HostIdentity` (a `host_<ULID>` ID plus an
Ed25519 keypair) and learns about peers through signed, TOFU-pinned advertisements
exchanged over the `--remote-port` HTTPS server. Peers are held in a local, TTL-pruned
`HostRegistry` (`session/host_registry.go`, `session/host_advertiser.go`).

See
[`project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md`](../../project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md)
for the full decision record.
