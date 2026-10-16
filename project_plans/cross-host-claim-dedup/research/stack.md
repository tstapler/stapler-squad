# Research: Stack — Cross-Host Claim/Dedup System

## Existing gossip transport (ADR-002), what a claim index must reuse

`session/host_identity.go`, `session/host_registry.go`, `session/host_advertiser.go` are the
whole implementation. No standalone `docs/adr/*ADR-002*.md` file exists in this checkout despite
~15 code comments citing "ADR-002" (e.g. `session/host_registry.go:41`,
`session/host_advertiser.go:18-24`, `docs/reference/bundling-tymuxd.md:55,67`) — the design intent
has to be reconstructed from those comments and the code itself; flag this as a gap for Phase 3
planning (either the ADR was never checked in, or it's misnamed/missing from `docs/adr/`).

**Identity & crypto** (`session/host_identity.go`):
- `HostID` = a type-safe wrapper over `oklog/ulid/v2` (`github.com/oklog/ulid/v2 v2.1.2`, already
  a direct dependency), prefixed `"host_"`, generated via a package-level mutex-guarded
  `ulid.Monotonic(rand.Reader, 0)` entropy source (same pattern as `backlogItemIDMu`/
  `backlogItemIDEntropy` in `session/backlog_item_id.go`).
- Keypair is stdlib `crypto/ed25519`, minted once, immutable for the life of the install — no
  rotation path.
- `AdvertisementRecord.Sign`/`.Verify` (`session/host_registry.go:99-111`) sign only a canonical
  JSON-marshaled subset of fields (`signingPayload()`), never the raw struct — a claim record's
  signature should follow the identical shape: a private `signingPayload()` method covering just
  the fields that must be tamper-evident, with `PublicKey`/`Signature` themselves excluded.
- Trust model is **TOFU** (trust-on-first-use): `HostRegistry.Advertise` (`session/host_registry.go:193-235`)
  accepts a first-seen `HostIdentity` unconditionally and pins its `PublicKey`; every later record
  claiming that identity is accepted only if the key matches the pin AND the signature verifies.
  A pin mismatch is silently rejected (not an error) — "the expected outcome for a misbehaving or
  buggy peer" per that function's doc comment.

**Persistence** (`session/host_registry.go:127-178, 345-417`):
- JSON file (`host_registry.json`) alongside a JSON identity file, both guarded by
  `github.com/gofrs/flock` (`v0.12.1`, already a direct dep) for cross-process safety plus an
  in-process `sync.Mutex` "in addition to (not instead of) the flock" (comment at line 142-144).
- Read/write via `TryRLockContext`/`TryLockContext` with a 5s timeout and 100ms poll interval
  (`hostRegistryLockTimeout`), not a blocking `Lock()`.
- Writes use atomic write-then-rename (`persistLocked`, lines 394-417): write to `<path>.tmp`,
  `os.Rename` over the real path, clean up the tmp file on failure. A claim-index file should copy
  this exact write path rather than inventing a new one.
- In-memory representation is a `map[string]RegistryEntry` keyed by `HostID.String()`, loaded once
  at construction (`NewHostRegistryWithClock`) and flushed to the map+file together under `r.mu`.

**Transport & wire protocol** (`session/host_advertiser.go`):
- Not a generic gossip library — it's a bespoke HTTP/TLS POST exchange on the **existing**
  `--remote-port` HTTPS server (`server/server.go`'s `StartRemote`), explicitly chosen over a new
  listener or piggybacking on the local-only `WorkspacePeer` mechanism (comment at
  `host_advertiser.go:13-24`). Deliberate reuse of "the only cross-host-reachable HTTP surface in
  this codebase."
- `http.Client{Timeout: 5s, TLSClientConfig: InsecureSkipVerify: true}` — TLS is transport-only
  (self-signed CA per host, no shared CA), identity/authenticity comes from the Ed25519/TOFU layer
  above, not from certificate verification. `#nosec G402` is pre-justified inline.
- Fan-out/anti-flood: no explicit hop counter. `HostRegistry.Advertise` returns `isNew` bool; the
  HTTP handler (`server/auth/host_advertisement.go`, not read in this pass) only calls
  `HostAdvertiser.ReGossip` when `isNew==true`, bounding propagation to one extra hop per genuinely
  new fact rather than a TTL/hop-count field on the message. **A claim-index extension riding this
  channel should follow the same isNew-gated-single-hop pattern**, not add a hop counter to the
  wire format.
- SSRF hardening: `isPlausiblePeerAddress` (`host_registry.go:257-275`) rejects loopback/link-local/
  unspecified advertised addresses before they're trusted as a liveness-check target — relevant if
  a claim record ever embeds a deep-link URL back to "the owning host," since that URL will
  eventually be dereferenced by another host's liveness/resolution code
  (`server/services/deep_link_resolver.go`'s `registryHostResolver.checkLiveness`, referenced at
  `host_registry.go:249`). Any new claim-fetch codepath must apply the same address-plausibility
  gate before dialing a peer-supplied address.

**Concurrency model**: `Clock` interface + `realClock`/injectable fake clock for TTL/prune logic
(mirrors `executor/circuit_breaker.go`'s `Clock`), explicitly to keep tests deterministic per the
`fix-flaky-tests-dont-defer` skill and `deterministic-fast-tests` — no wall-clock sleeps in tests.
`DefaultHostRegistryTTL = 3 * DefaultHostAdvertisementInterval` (15 min): an entry survives several
missed cycles before `Prune()` drops it, not one — a claim index's own staleness/TTL policy should
default to the same multiple-of-interval shape for consistency, not a fresh single-cycle TTL.

## Other repo patterns worth reusing

- **`atomic.Pointer[T]`-published snapshot pattern**: `session/instance_snapshot.go` (see also
  `.claude/rules/instance-lock-free-reads.md`) is the codebase's lock-free-read convention for
  hot-path reads of mutable state — republish an immutable struct via `atomic.Pointer.Store` on
  every mutation, read via `.Load()`. `HostRegistry` itself does *not* use this pattern (it uses
  `sync.Mutex` + a plain map, since registry reads aren't identified as a hot path); a claim index
  queried from request-handling code (e.g. the `ImportGitHubIssue` path) at higher frequency is a
  better candidate for the snapshot pattern than for copying `HostRegistry`'s mutex-only approach.
- **`session/workspace_peers.go`** is explicitly *not* a distributed/gossip mechanism — it's local-only:
  DB query (`ListWorkspacePeers`) + local `tmux list-sessions` liveness (`LiveTmuxSessionUUIDs`,
  line 162) joined via `ApplyTmuxLiveness` (line 277). It answers "which sessions on *this* host
  share a workspace," not a cross-host question. It's a useful *shape* reference for the
  requirements doc's "narrower v1" (`list_workspace_peers`-style RPC), but has zero distributed
  state or CRDT logic to borrow — the actual distributed piece must come from the gossip layer
  above.
- No CRDT types, vector clocks, or last-write-wins merge logic exist anywhere in the repo today
  (`grep -ri "crdt\|vector.clock\|lww" --include=*.go` returns nothing). `HostRegistry.Advertise`'s
  merge rule is the closest analog: unconditional overwrite of an entry keyed by `HostID`, gated
  only by signature/TOFU validity, not by a logical clock — i.e. "last advertisement wins" per
  peer-identity key, no conflict resolution across concurrent claims. A claim index built the same
  way inherits the same weakness the requirements doc calls out of scope: "Automatically resolving
  a genuine simultaneous-claim race."
- No standalone ADR-002 file was found (see above) — treat `session/host_registry.go` and
  `session/host_advertiser.go`'s doc comments as the authoritative design record until/unless the
  missing doc surfaces elsewhere.

## Community-recommended eventually-consistent gossip-KV approaches (for comparison, not adoption)

Given the requirements' explicit steer toward extending the existing gossip channel rather than
adopting new infrastructure, these are offered as a sanity check that "extend the bespoke HTTP
gossip" is the right call versus reaching for a library — not a recommendation to switch:

- **`hashicorp/memberlist`** (SWIM protocol) — the standard Go gossip-membership library, UDP+TCP
  based, with pluggable `Delegate`/`EventDelegate` hooks for piggybacking arbitrary application
  state (exactly a claim index) onto membership gossip messages. Used by Consul/Nomad/Serf. Would
  be a legitimate from-scratch alternative, but it's a new heavyweight dependency (its own failure
  detector, UDP transport, message queue) that duplicates work this codebase already built
  (TOFU/Ed25519 identity, TTL pruning, HTTP-based transport reusing the existing TLS listener).
  Given the fixed, small, mostly-known peer set (workstation-scale, not a large fleet), the
  cost/benefit favors extending the existing bespoke system.
- **`hashicorp/serf`** — built on memberlist, adds a distributed event/query layer closer to what
  "query known peers on demand" (the requirements' narrower v1 direction) would need, but same
  new-dependency cost as above.
- **CRDT libraries** (e.g. `automerge`, or hand-rolled OR-Set/LWW-Register) — overkill for a
  key→(owning host, deep link) map with no concurrent-edit merge requirement beyond "last claim
  wins" or "first claim wins" (the requirements explicitly exclude automatic race resolution).
  A plain last-write-wins map (mirroring `HostRegistry`'s existing merge rule) is sufficient and
  consistent with the rest of the codebase's simplicity bias.
- **General pattern recommendation**: for a claim index this size (bounded by GitHub
  issue/PR count per repo, refreshed on the same `DefaultHostAdvertisementInterval` cadence as host
  advertisements), piggybacking a `map[externalURL]ClaimRecord` onto the existing
  `AdvertisementRecord` POST/reply exchange (or a sibling endpoint reusing
  `newGHRequestForHostWithToken`-style conventions — n/a here, this is intra-fleet not GitHub API
  traffic) is simpler and lower-risk than introducing memberlist/serf. The "narrower v1" pull-based
  RPC (`check_cross_host_claim`, mirroring `list_workspace_peers`) is the lower-risk starting point
  per the requirements doc's own framing, and fits the codebase's existing preference for
  synchronous on-demand RPCs (`server/services/`) over new background daemons.

## Summary of concrete reuse recommendations

1. Follow `session/host_registry.go`'s exact persistence recipe: JSON file + `gofrs/flock` +
   in-process `sync.Mutex` + atomic write-then-rename, TTL expressed as a multiple of the
   advertisement interval, injectable `Clock` for deterministic tests.
2. Reuse `AdvertisementRecord`'s sign/verify shape (`signingPayload()` covering only
   tamper-relevant fields, Ed25519 signature, TOFU-pinned `PublicKey`) for any claim record that
   travels over the wire — don't invent a second crypto scheme.
3. Ride the existing `--remote-port` HTTPS surface and `isNew`-gated single-hop re-gossip; don't
   add a new listener, a hop-count field, or a new external dependency (memberlist/serf) given the
   small fixed peer-set scale this system operates at.
