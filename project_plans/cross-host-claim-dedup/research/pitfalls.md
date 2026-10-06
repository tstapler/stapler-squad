# Research: Pitfalls & Risks — Cross-Host Claim/Dedup System

Agent 4 (Pitfalls) — SDD Phase 2 research for backlog item `5fba8e5f-2810-42aa-a32f-58684a768e43`.

## 1. Existing gossip registry: what safeguards actually exist vs. what's wired up

`session/host_registry.go` and `session/host_advertiser.go` implement the "Workspace Host
Registry" gossip described (only) in inline code comments as "ADR-002" — **there is no
standalone ADR-002 document for this design.** `docs/architecture/decisions/002-external-session-streaming-mode.md`
is a different, unrelated ADR-002 (different numbering namespace: `docs/adr/ADR-0NN-*.md` vs.
`docs/architecture/decisions/0NN-*.md`). Anyone citing "ADR-002" for this feature is citing a
design that lives only as scattered doc-comments in `session/host_registry.go` (lines 40-44,
73, 254, 279, 313) and `session/host_advertiser.go` (lines 18-24, 27-37, 58-61) — worth writing
up as a real ADR before extending it, not just referencing the comments further.

**Existing safeguards, verified by reading the code:**
- `DefaultHostAdvertisementInterval` = 5 min, `DefaultHostRegistryTTL` = 3× that = 15 min
  (`session/host_registry.go:33-44`). An entry survives several missed advertisement cycles
  before `Prune()` would remove it — a deliberate blip-tolerance design.
- TOFU pinning: `HostRegistry.Advertise` (`session/host_registry.go:207-234`) rejects a record
  whose public key doesn't match a previously-seen identity, preventing a peer from
  impersonating a `HostID` it doesn't hold the private key for.
- `isPlausiblePeerAddress` (`session/host_registry.go:245-275`) blocks loopback/link-local/
  unspecified advertised addresses — an SSRF guard against a malicious peer redirecting this
  instance's liveness probe (`server/services/deep_link_resolver.go`'s `checkLiveness`) at
  itself or a cloud metadata endpoint.
- Fan-out for re-gossip is bounded by construction, not a hop counter: `HostAdvertiser.ReGossip`
  only fires once per newly-learned identity (`Advertise` returns `isNew=false` on repeats), so
  there's no unbounded flood (`session/host_advertiser.go:26-37`).

**Verified gap — `Prune()` is never called in production.** `grep -rn '\.Prune()' --include='*.go' .`
finds only unrelated git-worktree/gogit-store `Prune()` calls; `session/host_registry.go`'s own
`Prune()` (line 281) has zero callers outside its four unit tests in `session/host_registry_test.go`.
`main.go`'s wiring (`main.go:1545-1558`) constructs `NewHostRegistry` + `NewHostAdvertiser` and
starts `advertiser.Run(ctx)`, but never schedules `hostRegistry.Prune()` anywhere (no ticker, no
cron, no call in `Run`). **In the deployed system today, stale peer entries are never pruned —
the TTL constant and the `Prune` logic exist and are tested in isolation but are dead code in
production.** This is the single most important existing-system fact for this feature:

- Extending the gossip payload with a claim index does **not** inherit working pruning "for
  free" — it inherits a registry that already never prunes. Any claim index piggybacked on
  `AdvertisementRecord` would grow unboundedly *right now*, with or without new claim-specific
  lifecycle logic, unless this item also wires `Prune()` into a running loop (or the claim
  design deliberately avoids depending on host-entry pruning at all).
- Even if `Prune()` were wired up, host-entry TTL and claim-record lifecycle are different
  concerns: a host's advertisement going stale (it crashed, or is just slow to re-advertise)
  says nothing about whether the *claim* it made is stale. A completed/merged PR's claim should
  be released promptly regardless of whether the *host* that made it is still reachable; a
  perfectly reachable, actively-advertising host can still hold a claim on an issue that was
  closed by a human three weeks ago. Claim expiry needs its own signal (issue/PR state, not
  peer liveness) — conflating the two means a live host's claim on a dead issue lingers forever,
  or a merged-PR's claim gets prematurely dropped just because its host missed one advertisement
  cycle.

## 2. Split-brain / stale-data / TOCTOU risks specific to this feature

- **No cluster-wide compare-and-swap.** Gossip is eventually consistent by design; there is no
  mechanism (and none is proposed by the design directions) for "check nobody else has claimed
  this" and "claim it" to be atomic across hosts. Two hosts polling the same GitHub repo can both
  run `ImportGitHubIssue` (`server/services/backlog_service_sync.go:251`) within the same
  gossip propagation window (up to one `DefaultHostAdvertisementInterval` = 5 min, or longer if
  the claim index is only exchanged on the existing advertisement cadence rather than pushed
  immediately) and both see "unclaimed," both claim, both spawn sessions, both open PRs — the
  exact race the requirements doc explicitly puts **out of scope** to fully resolve, but the
  research should flag that even a "check before act" implementation only *narrows* the window,
  it doesn't close it, and the narrower-v1 (`check_cross_host_claim`-style on-demand RPC mirroring
  `list_workspace_peers`) has the same fundamental gap — it just makes the check synchronous
  instead of relying on propagated gossip state, which shrinks the window but doesn't remove it.
- **False-positive blocking from stale peer data.** If host A crashed hours ago while holding a
  claim, and its `HostRegistry` entries at every other host are (per the dead-`Prune()` finding
  above) *never* expired in the current codebase, "Host A already claimed this" becomes a
  permanent false block with no automatic recovery path. Any design must define an explicit
  claim-expiry/reclaim signal independent of host-registry TTL (e.g. claim age, or liveness-check
  against the claiming host via the existing `checkLiveness` pattern in
  `server/services/deep_link_resolver.go:139-155`, before treating a claim as authoritative) —
  otherwise this feature would trade "duplicate work" for "permanently stuck work," which is
  arguably worse since duplicate work is at least visible (two open PRs) while a false claim
  block is a silent no-op.
- **Stale human-facing message.** A UI/CLI surface saying "already claimed by host X" is itself
  gossip-derived and can be wrong in either direction: stale-stale (claim was released but this
  host hasn't heard yet) or stale-fresh (claim is real but this host's registry entry for host X
  aged out — again moot today since it never ages out, but relevant once `Prune()` is fixed).
  The message should carry its own "as of last seen at TIME" caveat (the registry already tracks
  `LastSeenAt`/`AdvertisedAt` per entry, so this data exists — `RegistryEntry` at
  `session/host_registry.go:64-70`) rather than presenting claim state as certain.
- **Thundering herd on rejoin** is a lesser risk here than in typical gossip systems because
  fan-out is already bounded (one hop per new fact, `ReGossip`'s doc comment,
  `session/host_advertiser.go:26-37`) and advertisement is push-based on a fixed 5-minute
  interval, not triggered in bulk by a rejoin event. A claim index piggybacked on the existing
  `AdvertisementRecord` inherits this same bounded behavior. A *separate* claim-specific gossip
  channel (if the design splits it out) would need to re-derive the same bounded-fanout
  discipline rather than assume it comes free.

## 3. Design-direction-specific risks

- **Extending `AdvertisementRecord` (Option A, gossip payload).** `AdvertisementRecord`
  (`session/host_registry.go:74-97`) is a *signed* struct — `signingPayload()` explicitly signs
  only `HostIdentity`, `AdvertisedAddress`, `AdvertisedAt`. Adding a claim index to this struct
  means either (a) it rides along unsigned (then a MITM-free but honesty-optional peer could lie
  about claims without detection, weaker than the existing TOFU/Ed25519 identity guarantee), or
  (b) `signingPayload()` must be extended and every existing signature-verification call site
  and test that constructs/verifies an `AdvertisementRecord` needs updating — a wire-format and
  security-surface change to a struct whose whole job today is proving *identity*, not carrying
  arbitrary mutable application data. Mixing "who are you" (rarely changes) with "what do you
  claim" (changes per-import) in one signed, periodically-rebroadcast record also means every
  5-minute advertisement cycle now re-transmits the *entire* claim index to every peer, growing
  with the number of open claims — a cost the current identity-only record doesn't pay.
- **Narrower v1 (on-demand RPC, Option B).** Mirrors `list_workspace_peers`
  (`session/workspace_peers.go:58` `ListWorkspacePeers`), but that function is purely local (DB
  query + `tmux list-sessions`, no network) — the cross-host version has no local precedent to
  copy fault-handling from. It needs its own answer for: which peers to query (all
  `HostRegistry.Snapshot()` entries, unbounded as that registry never shrinks per the pruning
  gap above?), timeout/partial-failure handling (some peers unreachable — return "unknown" or
  "not claimed"? the latter is exactly the false-negative that lets duplicate imports through),
  and whether it blocks `ImportGitHubIssue` synchronously (adds multi-host network latency to a
  previously-local call) or runs async/best-effort.
- **PR-body stamping.** Stamping a deep link or #475's host identifier into a PR body/comment
  risks leaking internal hostnames, LAN addresses, or file paths into a **public** GitHub
  issue/PR if the target repo is public — `AdvertisedAddress` values are exactly the kind of
  internal-network detail (`isPlausiblePeerAddress`'s own doc comment calls out LAN/link-local
  addresses) that shouldn't appear in issue/PR text visible to external contributors. If #475's
  identifier is an opaque ID (not a raw hostname/address) this is safer; if it's a raw
  `HostIdentity`/hostname string, this needs an explicit "never stamp a raw address, only an
  opaque ID or the `ssq://` deep link's hostname component if that's already meant to be
  public-safe" rule under this item's design, not left implicit.
- **Coupling to #475.** #475 is still open and explicitly scoped narrowly (records *which* host
  claimed something; excludes remote claiming protocol and expiry/reclaim — see
  `project_plans/cross-host-claim-dedup/requirements.md`'s Prerequisite section). This item
  should treat #475's identifier as an opaque value behind an interface/accessor rather than
  hard-coding assumptions about its storage shape (field name, type, location on the backlog
  item), so a shape change in #475 doesn't force a rewrite here. No code for #475 exists yet in
  this repo (`grep -rn '#475' --include='*.go' --include='*.md' .` finds only this project's own
  `requirements.md`), so there's nothing to verify compatibility against today — this is a
  forward-looking risk, not a currently-observed one.

## 4. What to explicitly design against

1. **Don't let claim-index staleness silently block real work forever.** Require every
   "already claimed" verdict to carry an age/last-seen timestamp and a way to override or
   re-check liveness (reuse `checkLiveness`'s bounded-timeout `/health` GET pattern from
   `server/services/deep_link_resolver.go:139-155`) before treating a stale claim as gospel.
2. **Don't assume extending the gossip payload inherits pruning.** It doesn't — `Prune()` is
   currently dead code in production (verified: zero non-test callers). Either wire it up as
   part of this item, or make the claim index's own lifecycle independent of host-entry TTL
   entirely (e.g. claims expire based on the underlying GitHub issue/PR state via existing
   webhook/sync paths, not via `HostRegistry`'s prune cycle).
3. **Don't leak internal network topology into public GitHub content.** PR-body/comment
   stamping must go through a value explicitly designed to be public-safe (opaque ID or
   sanitized deep-link form), never a raw `AdvertisedAddress`/hostname.
4. **Don't let a TOCTOU narrowing be sold as a fix.** Both design directions (gossip-carried
   index or on-demand RPC) reduce the race window but cannot close it without cluster-wide
   consensus, which is explicitly out of scope. The requirements/plan should say "reduces
   the race window to ~X" rather than "prevents duplicate claims," and the out-of-scope section
   already correctly excludes "automatically resolving a genuine simultaneous-claim race" — this
   should stay a hard line, not soften during planning.
5. **Don't hard-code #475's shape.** Access its identifier through a narrow interface so #475
   landing differently doesn't cascade into this feature's code.
6. **Write ADR-002 as an actual document** if this item is going to extend that design — right
   now "ADR-002" for the host registry/gossip system exists only as recurring doc-comment prose
   scattered across two files, not a reviewable, linkable decision record, and the ADR-002
   *number* is already taken by an unrelated decision in `docs/architecture/decisions/`.

## Key file references

- `session/host_registry.go` — `RegistryEntry`, `AdvertisementRecord`, `Advertise`, `Prune`,
  `Lookup`, `LookupByHostname`, TOFU pinning, SSRF address filtering.
- `session/host_advertiser.go` — `HostAdvertiser`, `BroadcastOnce`, `SendAdvertisement`,
  `ReGossip`, bounded one-hop fan-out.
- `session/host_registry_test.go:85-` — the four `Prune` unit tests (isolated coverage, no
  production wiring).
- `main.go:1545-1558` — production wiring of `HostRegistry`/`HostAdvertiser` (confirms no
  `Prune()` call).
- `server/services/deep_link_resolver.go:105-155` — `peerTLSTransport`, `ResolveHost`,
  `checkLiveness` (bounded-timeout liveness probe pattern to reuse for claim staleness).
- `session/workspace_peers.go:58` — `ListWorkspacePeers` (local-only precedent, no network
  fault-handling to borrow from).
- `server/services/backlog_service_sync.go:251,278` — `ImportGitHubIssue`,
  `GetBacklogItemByExternalURL` (today's single-host-only dedup check).
- `docs/architecture/decisions/002-external-session-streaming-mode.md` — the *other*, unrelated
  ADR-002 (numbering collision to be aware of).
