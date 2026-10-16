# Architecture Research: cross-host-claim-dedup

Agent 3 (Architecture), SDD Phase 2 research. Companion to `requirements.md`.
Read first, per Phase-2 tasking: `project_plans/network-hostname-redetect/research/architecture.md`
(`HostnameDetector`/`atomic.Pointer` publish pattern) and
`project_plans/backlog-deep-linking/research/architecture.md` (deep-link scheme + Workspace
Host Registry precedent, and the same "peer vs. host" bounded-context split this item inherits).

## 0. What's already built, confirmed by reading the code (not just the two prior docs)

The deep-linking project's design is no longer speculative — it has landed:

- `session/host_registry.go`: `HostRegistry` persists `RegistryEntry{HostID, AdvertisedAddress
  []string, PublicKey ed25519.PublicKey, AdvertisedAt, LastSeenAt}` to a JSON file, keyed by
  `HostID` (a ULID newtype, `session/host_identity.go:53`, `"host_<26-char ULID>"`, parsed via
  `ParseHostID`/`IsHostIDShape`). `DefaultHostAdvertisementInterval = 5m`,
  `DefaultHostRegistryTTL = 3 * interval` (`host_registry.go:37,44`) — ADR-002's "N missed
  cycles, not one" tolerance.
- `session/host_advertiser.go`: `HostAdvertiser.Run(ctx)` is a `time.NewTicker` loop
  (`host_advertiser.go:75-86`) that POSTs a signed `AdvertisementRecord{HostIdentity,
  AdvertisedAddress, AdvertisedAt, PublicKey, Signature}` (`host_registry.go:74-80`) to every
  peer in `registry.Snapshot()`, over the existing `--remote-port` HTTPS listener at
  `/internal/host-advertisement`. Identity is Ed25519/TOFU-verified at the application layer;
  TLS is transport-only (`InsecureSkipVerify: true`, deliberate, commented). One-hop bounded
  re-gossip (`ReGossip`) fans out newly-learned records without an explicit hop counter,
  relying on `HostRegistry.Advertise`'s `isNew` bool to prevent floods.
- `server/services/deep_link_resolver.go`: `DeepLinkResolver.HandleResolve` (`GET
  /api/deep-link/resolve`) already does exactly the "is this mine or a peer's" dispatch this
  item needs generalized: parses `ssq://` (`session/deeplink/url.go`), and if
  `!isOwnHostname(link.Hostname)`, calls `HostResolver.ResolveHost(ctx, hostname)` →
  `registryHostResolver` looks the hostname up in `HostRegistry.LookupByHostname`, then does a
  bounded-timeout **live** `GET https://<addr>/health` liveness probe
  (`deep_link_resolver.go:139-155`) before answering `reachable`. Response kinds are `local`,
  `handoff` (with `advertisedAddress`), `unreachable` (`not-registered` or `unreachable`
  reasons, `LastSeenAt` included), `not-found`, `invalid`.

**Implication for this item**: the hard infrastructure — signed gossip, a registry with TTL
semantics, and a resolver that already distinguishes "mine" vs. "known peer, live" vs. "known
peer, stale" vs. "unknown" — exists and works today for *hosts*. This item's job is narrower
than it might look: add a **claim fact** (issue/PR URL → owning host + item) to that already-
gossiped, already-gap-tolerant substrate, and wire two call sites (`ImportGitHubIssue`,
`DequeueNextQueuedItems`) to consult it. It is not a new distributed system.

## 1. Claim index shape: field on `AdvertisementRecord`, or a separate gossip channel?

**Recommendation: a separate, parallel gossip channel/endpoint, not a field bolted onto
`AdvertisementRecord`.** Both were considered against the same tradeoffs prior art already
names for a structurally identical decision — `network-hostname-redetect`'s research rejected
a new pub/sub layer for hostname changes *because* the existing consumers were pull-based; here
the situation reverses, and gossip is the right fit, but the payload should stay separate:

- **Payload size / interval coupling.** `AdvertisementRecord` is broadcast every
  `DefaultHostAdvertisementInterval` (5 min) to *every* known peer, unconditionally, forever —
  that's the right cadence for "is this host still alive," which changes rarely and must be
  cheap to re-send. A claim index entry is created once per imported issue and essentially
  never changes after that (claims are not "renewed" the way liveness is) — for it to ride on
  the every-5-minute heartbeat, every host would need to retransmit its *entire accumulated
  claim history* on every tick, an unbounded and wasteful payload growth curve as the number of
  imported issues grows, whereas `AdvertisementRecord`'s payload today is O(1)
  (`HostIdentity` + a short `AdvertisedAddress` list). Piggybacking would force choosing between
  (a) bloating every heartbeat with the full claim set, or (b) inventing a delta/diff protocol
  on top of the heartbeat channel — which is just reinventing a separate channel with extra
  steps.
- **Signing scope.** `AdvertisementRecord.Sign` signs exactly `{HostIdentity, AdvertisedAddress,
  AdvertisedAt}` (`host_registry.go:87-97`) — a narrow, intentional signing payload. Extending it
  to cover an open-ended claim list means either re-signing a growing struct on every broadcast
  (cost grows with claim count) or introducing a second signature scope inside the same record,
  which is really the "separate channel" design wearing the same struct.
- **TTL/staleness semantics reuse, not duplication.** A claim doesn't expire the way a host
  registry entry does (`Prune()` drops an entry that stops being re-advertised) — a claimed
  GitHub issue stays claimed until the backlog item resolves (merged/closed/abandoned), which is
  an application-state transition, not a liveness heartbeat gap. Reusing `HostRegistry`'s TTL
  machinery for claims would incorrectly expire (and re-permit a duplicate claim on) a real,
  still-open item just because gossip missed a few cycles. The right lifecycle for a claim
  record is closer to "one-shot fact plus later state-change events" (claimed → PR opened →
  resolved) than to a periodically-refreshed liveness ping — which is a strong signal it's a
  different data shape, not a variant of `RegistryEntry`.
- **Backward compatibility.** A host running old code that doesn't know about a new field on
  `AdvertisementRecord` will `json.Unmarshal` and silently drop the unknown field (Go's default
  behavior) — survivable, but it also means an old host can never *learn about* a claim, so
  dedup silently fails against any peer not yet upgraded, with no visible error. A separate
  endpoint (e.g. `POST /internal/claim-advertisement`, mirroring
  `AdvertisementEndpointPath`'s precedent) degrades the same way in effect (an old peer 404s or
  ignores it) but doesn't risk **corrupting or bloating** the liveness channel's already-battle-
  tested wire format — the failure is additive-absence, not payload confusion, which is easier to
  reason about and roll out incrementally (new hosts can start claim-gossiping without any
  coordination with un-upgraded peers, exactly as `HostAdvertiser`'s own one-hop re-gossip design
  already tolerates partial rollout).

**Concrete shape**: a `ClaimRecord{ExternalURL string, ClaimingHostID HostID, ItemDeepLink
string, ClaimedAt time.Time, PublicKey, Signature}` gossiped the same way
`AdvertisementRecord` is (signed, TOFU-verified, POSTed to known peers, one-hop re-gossiped on
first sight), stored in a sibling `ClaimIndex` type next to `HostRegistry` (same
`stateDir`-scoped JSON-file-plus-mutex pattern, see `withReadLock`/`withWriteLock`/
`persistLocked` in `host_registry.go:349-417`) keyed by `ExternalURL` (already the app's
existing dedup key, see §2). Reuse `HostAdvertiser`'s `SendAdvertisement`/`ReGossip` request
plumbing (HTTP client, TLS config, retry-free best-effort semantics) via a small shared helper
or a second thin type built the same way — don't duplicate the TOFU/Ed25519 verification logic.

## 2. Integration points

- **`ImportGitHubIssue`** (`server/services/backlog_service_sync.go:251-317`) — today dedups
  *only* locally: `s.storage.GetBacklogItemByExternalURL(ctx, issue.URL)`
  (`backlog_service_sync.go:278`), `errors.Is(lookupErr, session.ErrNotFound)` gate. This is the
  layer to add a cross-host consult: after the local lookup misses, query the local `ClaimIndex`
  (already-gossiped, no network round-trip needed at *this* call site — see §3 on why an
  on-demand RPC is a fallback, not the primary path) for `issue.URL`. A hit means another host
  already claimed it — requirement's "blocks or flags" behavior branches here (§4 for which of
  those two this data can actually support).
- **`DequeueNextQueuedItems`** (`server/services/backlog_service_triage.go:736-`) — this is the
  **second**, currently-invisible claim point the requirements' framing (centered on
  `ImportGitHubIssue`) under-weights: even if import-time dedup is airtight, two hosts that
  *each* independently imported the item *before* either started gossiping (a real race window
  given the 5-minute advertisement interval) will each hold their own local backlog item row and
  each independently dequeue-and-spawn a work session for it via this function's
  `s.dequeueMu`-guarded loop — `dequeueMu` is an in-process `sync.Mutex`
  (`backlog_service_triage.go:740`), giving zero cross-host protection. `transitionWithGuard`
  (`backlog_service_triage.go:653`) is the CAS precondition machinery already used to prevent
  same-host double-transition races (see report_pr_created's racing-call tests,
  `server/mcp/tools_backlog_test.go:2510` `TestReportPRCreated_LoserPRNeverPersists_WhenCASPreconditionFails`)
  — a cross-host claim check belongs as a precondition *alongside* this existing local CAS check,
  at the point a queued item is about to be claimed for local work, not only at import time.
- **`report_pr_created`** (`server/mcp/tools_backlog_pr.go`) and the GitHub webhook handlers
  (`server/services/github_webhook_pr_fix.go`'s `handlePRFixEvent`,
  `server/services/github_webhook_handler.go`) — these already operate purely in terms of the
  *local* backlog item and PR metadata; there is no host-identity field anywhere in
  `BacklogItemData` (`session/repository.go:308-`) today to trace a PR back to its origin host.
  This is exactly what #475 is scoped to add (see §4). Once that field exists, the natural
  integration point is stamping the claiming `HostID` (or its deep link) into the PR body/comment
  at PR-creation time (`report_pr_created`'s existing write path) — a passive, always-available
  provenance signal that survives even if the claim index itself is stale or the originating host
  is offline when a *different* host's webhook handler later processes an event on that PR.
- **`GetBacklogItemByExternalURL`** — stays exactly as it is (local-only); it is not the seam to
  extend. The `ClaimIndex` is a new, separate lookup consulted *in addition to* it, not a
  replacement — conflating them would mean every local read pays a cross-host semantics cost it
  doesn't need.
- **`deep_link_resolver.go`** and `session/deeplink/url.go` — reused as-is for producing/parsing
  the deep link embedded in a `ClaimRecord.ItemDeepLink`; no changes needed here, this item is a
  consumer of that scheme, not a modifier of it.
- **`list_workspace_peers` / `session/workspace_peers.go`** — confirmed (per the
  backlog-deep-linking research, §2 of that doc, re-verified here) to be same-instance
  session/worktree peering with no host/URL concept at all; it is precedent for *shape*
  (a read-only "what does the system currently know about other concurrent claimants" query
  tool) for a new cross-host equivalent (the requirements' "narrower v1:
  `check_cross_host_claim`-style MCP tool/RPC"), not a mechanism to extend.

## 3. Data flow / consistency requirements

ADR-002 already commits this whole subsystem to **eventual consistency with a multi-cycle grace
period**, not strong consistency: `DefaultHostRegistryTTL = 3 * DefaultHostAdvertisementInterval`
= 15 minutes before a *host* is considered gone, and gossip itself has no ordering/consensus
guarantee — a claim broadcast can arrive at different peers in different orders, or not arrive at
all if a peer is down during the broadcast window and only catches up via re-gossip from a third
host. This is a real, unavoidable **window in which a genuine double-claim race is possible**:
two hosts import the same issue within the same gossip propagation delay, each believing itself
first. The requirements explicitly place "automatically resolving a genuine simultaneous-claim
race" out of scope — the architecture should treat that as correct given the substrate, not as a
gap to engineer around.

What this implies concretely for "blocks or flags for manual reconciliation":

- **A claim-index hit at `ImportGitHubIssue` time can safely *block*** (refuse to create a
  second backlog item, return the existing claim's deep link instead) — this is the common case
  (claim already propagated) and matches `ImportGitHubIssue`'s existing `AlreadyExisted` response
  shape (`backlog_service_sync.go:279-282`), just sourced from a remote claim instead of a local
  row.
- **A claim-index *miss* at import time cannot be trusted as proof of no-claim** — it only means
  "no claim has propagated to this host yet," which is indistinguishable from "no claim exists."
  The design must not claim to fully prevent the race; it can only shrink the window from
  "unbounded, silent, forever" (today) to "bounded by gossip propagation delay, typically well
  under the 5-minute interval, usually near-instant if the on-demand `check_cross_host_claim`
  fallback queries live peers directly rather than waiting for the next heartbeat." This is why
  the "flag for manual reconciliation" branch exists in the requirements at all — it's the
  necessary complement to a system that admits it cannot fully block: after both hosts create
  competing items and it's later discovered (e.g. both PRs land against the same issue, or a
  delayed claim gossip arrives showing a conflicting `ClaimedAt`), the system's job is to *detect
  and surface* the conflict, not silently reconcile it.
- **The on-demand `check_cross_host_claim` RPC (requirements' narrower-v1 direction) reduces but
  doesn't eliminate the window** — querying live peers synchronously at import time is strictly
  stronger than waiting for the next gossip tick, but it still only covers peers currently
  reachable and known to the registry; a peer that just imported the same issue one second ago,
  before its own gossip broadcast fires, is invisible to a synchronous query too. This is a
  latency-of-propagation problem, not a protocol bug, and no design short of true distributed
  consensus (explicitly out of scope) removes it entirely.
- **Recommendation**: implement the on-demand on-demand check as the primary read path (query
  known-live peers synchronously at `ImportGitHubIssue` time, mirroring
  `registryHostResolver.checkLiveness`'s bounded-timeout HTTP pattern), with the periodic gossip
  channel from §1 as the write/propagation path and the fallback for peers unreachable at
  import time. This gets the best available freshness without inventing new consistency
  machinery, and it directly reuses `deep_link_resolver.go`'s already-proven "ask a live peer,
  bounded timeout, treat failure as unknown-not-false" pattern instead of a new one.

## 4. Integration seam with #475 (open, blocking prerequisite)

#475 is scoped narrowly: "a stable per-instance identifier to backlog item claims for cross-host
provenance" — recording *which host* made a claim, explicitly excluding the remote/multi-host
protocol and claim expiry/reclaim this item adds. The seam:

- **Shape #475's identifier must have**: it should be `session.HostID` (the existing ULID
  newtype at `session/host_identity.go:53`, `"host_<ULID>"` string form), not a new ad hoc type.
  This host already has a durable, ULID-based, string-serializable, TOFU-key-paired identity
  concept purpose-built for exactly "which instance is this" — inventing a second identifier
  for the same concept (e.g. a raw hostname string, which can change per
  `network-hostname-redetect`'s own subject matter, or a new UUID) would duplicate
  `HostIdentity`'s job and reopen the primitive-obsession problem the `backlog-deep-linking`
  research explicitly called out for backlog item IDs. Concretely, #475 most likely adds a
  `ClaimedByHostID session.HostID` (or equivalent nullable field) to `BacklogItemData`
  (`session/repository.go:308-`) and the `backlog_item` ent schema, populated from the local
  process's own `HostIdentity.ID` at creation time.
- **What this item's design should assume**: that a `ClaimedByHostID`-shaped field exists on
  `BacklogItemData` and is populated at creation for every item, regardless of `ItemSource`
  (manual import, NL-created, forward/backward-synced) — goal 4 in the requirements. The
  `ClaimRecord` this item's gossip channel carries (§1) should carry that same `HostID` value
  as `ClaimingHostID`, not re-derive it independently, so the two features' provenance data
  never disagree about "who claimed this."
- **What this item's design should leave flexible until #475 lands**: the exact
  field name/nullability semantics on `BacklogItemData` and ent schema, and whether #475 stamps
  the ID at *all* creation paths or only some (the requirements flag "works for any backlog item
  source" as *this* item's goal, implying #475 may not yet cover every `ItemSource` path itself)
  — this item's gossip/claim-index code should read the host identifier through a single
  accessor (mirroring `Instance.Snapshot()`'s "one authoritative read path" convention from
  `.claude/rules/instance-lock-free-reads.md`) so that if #475 lands with a narrower field
  population than expected, only that one accessor needs a fallback (e.g. "no claimant recorded
  → treat as unclaimed-but-locally-owned"), not every call site in this item's code.

## 5. Event–Command–Policy table (EventStorming)

Actors: **User/Operator** (triage decisions, manual reconciliation), **Host A** (imports/claims
first), **Host B** (a second host, potentially racing), **GitHub** (issue/PR state, webhooks),
**Claim Index** (the new gossiped store, §1), **Workspace Host Registry** (existing,
`session/host_registry.go`).

| Command | Actor | Event | Policy (triggers next command) |
|---|---|---|---|
| `ImportGitHubIssue` | User/Operator on Host A | `IssueImportAttempted` | **Policy**: local `GetBacklogItemByExternalURL` miss + local `ClaimIndex` miss → triggers `CreateBacklogItem` + `RecordClaim`. Local hit → returns existing item (`AlreadyExisted`, unchanged today's behavior). Claim-index hit → triggers `RejectImportWithHandoffLink` (blocks, per §3). |
| `RecordClaim` | Host A (server-side, on successful create) | `ClaimRecorded` (ExternalURL, ClaimingHostID, ItemDeepLink, ClaimedAt) | **Policy**: triggers `GossipClaim` (broadcast to known peers, mirroring `HostAdvertiser.BroadcastOnce`) and, independently, `CheckCrossHostClaimOnDemand` if a synchronous peer query is enabled (§3). |
| `GossipClaim` | Host A's claim-broadcaster (new, sibling to `HostAdvertiser`) | `ClaimGossiped` | **Policy**: receiving host's endpoint verifies signature (TOFU) → triggers `UpsertClaimIndexEntry` locally, then `ReGossipClaim` (one-hop fan-out, mirroring `ReGossip`). |
| `CheckCrossHostClaimOnDemand` | Host B (about to import the same issue) | `PeerClaimQueryAnswered` — either `NoClaimFound` or `ClaimFound` | **Policy** (`ClaimFound`): triggers `RejectImportWithHandoffLink` on Host B instead of `CreateBacklogItem`. **Policy** (`NoClaimFound`): triggers `CreateBacklogItem` (Host B proceeds — this is the residual race window named in §3). |
| `RejectImportWithHandoffLink` | Origin host's `ImportGitHubIssue` handler | `ImportBlockedByExistingClaim` | Terminal (for this call) — surfaces the owning host's deep link to the operator, matching goal 2. |
| `CompetingClaimDetected` | Claim Index (either host, on receiving a `ClaimGossiped` event for an `ExternalURL` it already holds a *different* `ClaimingHostID` for) | `ClaimConflictFlagged` | **Policy**: since automatic resolution is out of scope, triggers `SurfaceConflictForManualReconciliation` (e.g. a backlog-UI banner/notification on both items) rather than silently picking a winner. |
| `SurfaceConflictForManualReconciliation` | System (both hosts) | `ConflictAcknowledged` or left open | **Policy**: User/Operator manually resolves (e.g. closes the duplicate item, per today's existing manual-dedup workflow) — no new automated command needed; this is intentionally a human decision point. |
| `DequeueNextQueuedItems` | Host A's/B's triage loop (`backlog_service_triage.go:736`) | `ItemDequeuedForWork` | **Policy**: before `transitionWithGuard` fires locally, consult local `ClaimIndex`/on-demand check for this item's `ExternalURL` — if a *different* host's claim is now visible (arrived after this host's own import), triggers `ClaimConflictFlagged` instead of proceeding to spawn a work session. |
| `SpawnWorkSession` | Host (via existing dequeue path) | `WorkSessionStarted` | Unchanged from today — no new command needed once the pre-check above gates it. |
| `report_pr_created` | Work session (MCP tool call) | `PRCreated` (PR URL, item ID) | **Policy**: triggers `StampProvenanceOnPR` — write the claiming host's deep link (or #475's `HostID`) into the PR body/comment, satisfying goal 3. |
| `StampProvenanceOnPR` | Origin host | `PRProvenanceRecorded` | Terminal for this flow — any later `github_webhook_pr_fix.go`/`github_webhook_handler.go` event on this PR can now read the stamped provenance directly from GitHub, without depending on the claim index or the originating host being reachable. |
| GitHub webhook event (`check_run`, `pull_request_review`, etc.) | GitHub | `PRFixEventReceived` | **Policy**: `handlePRFixEvent` traces the PR to its origin item via the stamped provenance (goal 3) — this works even if the receiving host is *not* the origin host, since the provenance lives on GitHub's own PR body, not in this host's local state. |

**Bounded-context note carried forward from the deep-linking research**: `Workspace Host
Registry` (host-scoped liveness/addressing) and the new `Claim Index` (issue/PR-scoped ownership
facts) are two distinct gossiped stores that happen to share transport and TOFU-verification
machinery — keep them as separate types/files (§1), not one growing struct, so a future reader
doesn't conflate "is this host alive" with "who owns this issue," the same distinction the prior
research insisted on for "peer" vs. "host."

## 6. Tech Debt Disposition: **Isolate via seam**

Neither `session/host_registry.go` nor `server/services/backlog_service_sync.go` needs a
refactor before this feature can land, and "extend as-is" is too weak a characterization —
this feature's cleanest shape is a **new sibling type** (a `ClaimIndex`/claim-gossip pair
mirroring `HostRegistry`/`HostAdvertiser`'s existing file-per-concern split) plumbed into the two
call sites (`ImportGitHubIssue`, `DequeueNextQueuedItems`) through a single new interface
(something like `crossHostClaimChecker` with `CheckClaim(externalURL string) (ClaimRecord, bool)`
and `RecordClaim(...)`), the same seam-insertion pattern `deep_link_resolver.go`'s `HostResolver`
interface already demonstrates for cross-host lookups (`unimplementedHostResolver` as a safe
default, a real `registryHostResolver` wired in production, both satisfying one small interface).
This keeps `backlog_service_sync.go`'s existing dedup logic and `host_registry.go`'s existing
liveness/TTL logic untouched and independently testable, while the new claim behavior is
additive and unit-testable in isolation — a refactor-first approach would be solving a problem
neither file actually has (no God-Object or hotspot findings turned up for either file in the
Step-2.75 check), and "extend as-is" (bolting claim fields directly onto `AdvertisementRecord`/
`RegistryEntry`) is the option §1 already rejected on payload-size and TTL-semantics grounds.
