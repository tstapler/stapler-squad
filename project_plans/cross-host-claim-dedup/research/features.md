# Feature Research: Cross-Host Claim/Dedup System

## 1. Existing dedup/claim mechanisms (single-host only)

All of these solve the *single-host* version of this problem. None cross a
host boundary — that's the gap this project fills.

### 1a. Import-time dedup — `GetBacklogItemByExternalURL`
`server/services/backlog_service_sync.go:251-306` (`ImportGitHubIssue`):
checks `GetBacklogItemByExternalURL(ctx, issue.URL)` before creating a new
backlog item. Doc comment at `session/ent_repository_backlog.go:2640-2646`
explains why URL, not ID: `external_id` needs `ItemSource` scoping to be
unique (two different sources could reuse the same numeric ID), but
`external_url` is globally unique on its own. It also explicitly uses
`.First()` (oldest match) instead of `.Only()`, because rows created before
this check existed may already violate uniqueness — a real precedent for
"the dedup check must not hard-fail on pre-existing duplicate data."

This is the exact function the requirements doc names as the extension
point, and it is genuinely a single `WHERE external_url = ?` query against
this host's own SQLite/ent DB — a cross-host claim check has no analogous
query to piggyback on; it has to be a new network call (gossip lookup or
RPC), not a schema extension of this table.

### 1b. Sync-loop dedup — `GetBacklogItemByExternalID` + ItemSource scoping
`session/backlog_sync.go:337-360`: the periodic `ItemSource` sync loop dedups
new items against existing ones scoped by `(source.ID, external_id)`, then
applies "local-wins" merge — only overwrites fields NOT in
`UserModifiedFields` (`ParseUserModifiedFields`). This is the second,
independent code path requirements.md's Goal 4 calls out ("works for any
backlog item source... forward/backward-synced via ItemSource") — a claim
index has to be fed from *both* `ImportGitHubIssue` and `SyncLoop`'s create
path, which currently share no helper function; whatever "record a claim"
call gets added must be inserted at both of these two call sites (plus
NL-created items, which go through neither and would need a third call
site — see `session/backlog_plugin.go`'s `ItemSourcePlugin` interface as
the abstraction boundary if the maintainers want one call site instead of
three).

### 1c. `report_duplicate` MCP tool — same-host cross-*item* duplicate, not cross-host
`server/mcp/tools_backlog.go:2112-2442` (`reportDuplicate` +
`reportDuplicateUnclaimed`). This lets a **work session** flag that its own
backlog item is a duplicate of an already-shipped GitHub PR/issue/commit,
verified live via `verifyGitHubRefExists` (`GetPR`/`GetIssue`/`GetCommit`).
It is not a "two hosts claimed the same issue" tool — it's "this item
turned out to already be done elsewhere (a human did it, or another PR
shipped it)," reported by the agent doing the work, then routed to human
review (never auto-archived, per ADR-001 cited in the comment).

Two design precedents worth reusing:
- **CAS-based race handling**: `reportDuplicate` reads the item via an
  overridable `getBacklogItemFor` seam specifically so tests
  (`TestReportDuplicate_ReportsDistinctMessage_WhenCASPreconditionFails`)
  can force two racing calls' reads to land before either write, then
  asserts the loser gets a distinct, honest error message rather than
  silently overwriting the winner's write. Any cross-host claim
  "compare-and-swap" (two hosts racing to claim the same issue within one
  gossip interval) should follow this same test pattern: force the race
  deterministically, assert the loser's error is informative, not silent.
- **Unclaimed vs. claimed caller branching**: `reportDuplicate` branches on
  whether the calling session is linked to the item at all
  (`resolveItemLink`) — an unlinked caller can still act on an *unclaimed*
  item (`reportDuplicateUnclaimed`), but not a claimed one. This maps
  directly onto "host B tries to import an issue host A already claimed":
  the check needs to know not just "does a claim exist" but "is it still
  live/claimed, or has it lapsed."

### 1d. Same-host duplicate-work-session guard (`backlog_service_triage.go`) — closest existing analogue to the claim/staleness problem
`server/services/backlog_service_triage.go:919-947` (`spawnSessionAfterGates`
step 8b/8b2/8b-Jules): before spawning a new work session for an item, three
independent checks run in sequence — `findActiveWorkSession` (DB
bookkeeping), `findConfirmedLiveWorkSession` (re-asks tmux/OS truth via
`sessionStopper.IsSessionLive`, specifically because DB-only trust already
caused a real bug — a wrongly-tombstoned live session), and
`findActiveJulesSession` (a *remote* infra — Google's — the local host has
no liveness signal for at all, so it just trusts the DB record). Blocked
callers get `CodeAlreadyExists` with a staleness-aware message via
`activeWorkSessionBlockedError` → `respawnBlockedActiveProgressSignal` →
`workSessionStaleness`, all sharing one constant, `maxReworkBlockStaleness =
15 * time.Minute` (see `docs/adr/ADR-001-staleness-threshold-recalibration.md`
referenced in the comment at
`server/services/backlog_service_triage.go:1367-1378`).

This is the most directly transferable precedent in the codebase for the
cross-host case: the **Jules case is structurally identical to a remote
host's claim** — a resource this process cannot directly observe liveness
for, tracked only via a DB record with no built-in expiry check beyond "is
it still marked active." The requirements doc's stale-claim/force-release
concern (see below) should reuse `workSessionStaleness`'s three-way
live/idle/stale shape and the "same threshold shared by the mark and
resolve sides so they can't drift" discipline, rather than inventing a
new one.

### 1e. Gossip registry TTL precedent (`session/host_registry.go`)
`DefaultHostAdvertisementInterval = 5 * time.Minute`,
`DefaultHostRegistryTTL = 3 * DefaultHostAdvertisementInterval` (15 min) —
comment explicitly frames this as "N missed cycles, not immediately, to
tolerate transient network blips" (ADR-002). If a claim index rides the
same gossip channel, a claim's natural staleness/expiry unit is already
established as "N missed re-advertisement cycles," and 15 minutes already
exists twice in this codebase (host TTL and `maxReworkBlockStaleness`) as
the house standard for "how long before we stop trusting a liveness claim
we can't directly verify." A third, unrelated number here would be a
smell.

Important structural note: `AdvertisementRecord`
(`session/host_registry.go:73-79`) is a small, ed25519-signed, fixed-shape
struct (hostname, addresses, timestamp, pubkey, signature) — it carries no
application payload today. Piggybacking a claim index (URL → owning
host+item) onto it means either (a) growing the signed payload with the
size of the local claim set (scales with backlog size, re-signed and
re-broadcast every 5 min to every peer), or (b) a second, separate gossip
message type alongside the identity advertisement. The requirements doc's
"narrower v1" alternative — an on-demand `check_cross_host_claim`-style
RPC/MCP tool mirroring `session/workspace_peers.go`'s `ListWorkspacePeers`
pattern but querying peers over the network instead of the local DB — avoids
that payload-growth problem entirely and is the lower-risk starting point.

### 1f. Deep-link resolver's existing "local vs. remote-handoff" branch
`server/services/deep_link_resolver.go:162-268` (`DeepLinkResolver`,
`resolveLocal`, `deepLinkHandoffResponse`, `isOwnHostname`). Given an
`ssq://<hostname>/<type>/<version>/<id>` link, this already distinguishes
"this host owns the item, resolve it locally" from "hand off to
`hostname`" via `HostResolver.ResolveHost` (backed by the same
`host_registry.json`, `registryHostResolver` at line 74). This is *half* of
Goal 2 (deep link back to the owning host) already built — but it requires
already knowing the target `hostname` and item ID. The genuinely missing
piece is the reverse index: given only a GitHub issue/PR URL, find which
hostname+item ID to build that deep link *to* in the first place. No
existing code answers that reverse lookup.

## 2. Edge cases and failure modes to design for

- **Simultaneous claims within one gossip interval**: two hosts import the
  same issue before either's advertisement propagates. No amount of
  faster gossip fully closes this window (CAP-theorem-flavored: an
  eventually-consistent index cannot also be linearizable without a
  synchronous round-trip on every claim). The `report_duplicate` CAS
  precedent (1c) suggests the right *response* isn't "prevent," it's
  "detect after the fact and force a clear, non-silent human-visible
  reconciliation" — matching the explicit "Out of scope: automatically
  resolving a genuine simultaneous-claim race" in requirements.md.
- **Host goes offline mid-claim, claim never expires**: directly the
  `findActiveJulesSession`/`workSessionStaleness` problem (1d) at
  cross-host scope. A claim with no TTL is a claim that can never be
  reclaimed if the claiming host is permanently gone (laptop wiped,
  reinstalled). Needs the same three-way shape: is-the-owner-live signal
  (from `HostRegistry`'s own liveness/TTL, 1e) distinct from
  is-the-claim-itself-stale (item hasn't progressed in N min/hours) —
  these are two different clocks and conflating them repeats the exact bug
  `findConfirmedLiveWorkSession`'s comment (1d) describes fixing once
  already (DB-only trust missed a real liveness signal).
- **Claim index entries surviving item deletion**: nothing today deletes a
  `HostRegistry` entry when a session ends normally (only TTL-based
  pruning, 1e) — a claim index would need the same question answered
  explicitly: does deleting/archiving a backlog item locally proactively
  retract its claim broadcast, or rely purely on TTL expiry? Given gossip
  is fire-and-forget/best-effort (`ReGossip`, `BroadcastOnce` in
  `session/host_advertiser.go`), an explicit retraction message is not
  guaranteed delivered either — TTL is the only mechanism that's actually
  reliable here, which argues for keeping claims short-TTL-and-renewed
  rather than relying on explicit retraction as the primary mechanism.
- **Renamed/transferred/closed GitHub issues**: `external_url` (the current
  single-host dedup key, 1a) is a URL that GitHub does *not* guarantee
  stability of forever — an issue can transfer to a different repo, which
  changes its URL, or a PR can be converted from/to a draft/issue in some
  GitHub Enterprise configurations. `GetBacklogItemByExternalURL`'s comment
  already flags `external_url` has no DB uniqueness constraint and
  pre-existing dupes exist — a claim index keyed the same way inherits the
  same weakness. A closed issue does not clear its claim today (nothing in
  `ImportGitHubIssue` or the sync loop reacts to `issue.State`), so a
  claim can outlive the thing it claims.
- **PRs opened against forks**: `payloadRepoHost` /
  `payloadRepoFullName` in `server/services/github_webhook_pr_fix.go`
  extract the *base* repo's identity from the webhook payload — a fork PR
  still reports the same base `repository.full_name`, so this specific
  edge case is likely already handled correctly by existing webhook
  parsing; worth confirming during planning rather than assuming it's a
  new gap, but flagging since requirements.md calls it out explicitly.
- **Manual import vs. NL-created vs. ItemSource-synced all needing the same
  claim index**: three distinct code paths today (1a, 1b, and NL-created
  items which go through neither, presumably a third `CreateBacklogItem`
  call site) — confirmed no single choke point exists in
  `session/storage.go`/`EntRepository` that all three funnel through before
  hitting `CreateBacklogItem`. Whatever "publish this claim" call gets
  added either needs three call sites kept in sync (the registry-drift risk
  the repo's own `docs/reference/session-creation-registry.md` /
  `docs/reference/feature-testing-registry.md` pattern was built to guard
  against for other multi-touchpoint features) or should be pushed down
  into `CreateBacklogItem` itself so it's structurally impossible to miss.
- **Network partition, each host thinks it's sole owner**: this is the
  gossip system's normal operating assumption already (ADR-002's "TOFU-pinned,
  TTL-pruned" design, 1e/host_registry.go:133) — a partition just means both
  sides' registries independently prune each other after
  `DefaultHostRegistryTTL` and then genuinely believe they're alone. No
  existing code detects "I used to know about host X and don't anymore" as
  a distinguishable event from "I never knew about host X" — worth deciding
  during planning whether that distinction matters for claim conflict
  resolution (e.g. surfacing "this claim may be stale due to a partition,
  not resolved" vs. treating a pruned-but-once-known host the same as an
  unknown one).

## 3. Unstated user needs (distributed backlog systems generally)

- **Force-release a stale claim.** Every existing "is this thing still
  active" gate in this codebase that blocks a human action
  (`activeWorkSessionBlockedError`, 1d; the Jules active-session guard)
  pairs the block with an escape hatch — the error message text always
  points at "wait for it to finish or kill it first." A claim with no
  force-release path is a permanent footgun the moment a host is
  decommissioned; users will want a manual "I know host A is dead, release
  this claim" action, ideally visible from wherever "Copy Link"
  (`BacklogItemDetail.tsx`, cited in requirements.md) already surfaces the
  deep link.
- **"Who else is working on related work" visibility before starting.**
  `session/workspace_peers.go`'s `ListWorkspacePeers`/`WorkspacePeer`
  (1e-adjacent) already answers this question at *local, same-workspace*
  granularity — showing peer sessions' title/branch/status/goal before a
  user starts overlapping work in the same repo. Users of a genuinely
  cross-host system will expect the same visibility widened to "which
  *other host* has claimed this GitHub issue," not just same-host peers —
  i.e., the UI affordance already exists and sets the expected bar
  (title, status, staleness/"stuck" lifecycle badge via
  `WorkspacePeer.Lifecycle()`) for what a cross-host claim panel should
  look like.
- **Claim conflicts logged/auditable, not just silently blocked.** Every
  precedent found here logs structured detail at the moment of a block or
  conflict (`log.InfoLog().Printf("[mcp:report_duplicate] ...")`,
  `log.Warn("host_registry.entry_expired", ...)`) — a claim-conflict event
  (two hosts racing) should follow the same convention: a structured,
  greppable log line naming both hosts and the shared external URL, not
  just a silently-swallowed no-op or a bare error string, so an operator
  debugging "why did two PRs get opened for the same issue" has something
  to search logs for.
- **A way to distinguish "not claimed" from "claim unknown because I can't
  reach any peer right now."** The MCP tool design direction in
  requirements.md ("query known peers' advertised addresses on demand")
  needs a third response state beyond claimed/unclaimed — "couldn't
  verify" — mirroring `deep_link_resolver.go`'s existing
  `deepLinkUnreachableResponse` (1f) distinguishing "not found" from
  "target host unreachable." Silently treating "couldn't reach any peer"
  as "unclaimed" would race straight back into the exact duplicate-import
  bug this project exists to close.

## 4. Industry patterns for distributed work-claiming (reasoned from known general patterns, no web search)

- **GitHub issue assignment**: purely last-writer-wins, no distributed
  claim protocol at all — GitHub is the single source of truth (one
  central DB), so "someone else is already assigned" is just a normal
  read-before-write UI affordance, not a consensus problem. This maps
  cleanly onto the "narrower v1" direction in requirements.md: treat one
  designated authority (whichever host currently holds the claim, or GitHub
  itself via an issue comment/label) as the tie-breaker read, rather than
  building true multi-master consensus. stapler-squad's actual hard part is
  that *there is no single central DB* — every host has its own — so this
  pattern only partially transfers.
- **Jira / ticket-tracker "in progress + assignee" model**: a claim is a
  mutable field, not a lease — nothing expires it automatically, which is
  exactly the "stale claim never expiring" failure mode requirements.md
  calls out, and matches this codebase's own `report_duplicate`/duplicate
  backlog items: humans, not the system, resolve genuine conflicts, so the
  system's job is to make the conflict *visible* (label, comment, "already
  assigned" warning), not to prevent it outright.
- **Distributed lease/lock patterns (Chubby/etcd-style, reasoned generally)**:
  a claim is a time-bounded lease that must be actively renewed, and
  ownership transfers only after the lease lapses — this is the pattern
  `session/host_registry.go`'s TTL-based advertisement (1e) already
  implements at the host-identity level, and is the natural fit to extend
  to claims: "claim TTL = N missed re-advertisement cycles," reusing the
  same 15-minute constant already established twice in this codebase (1d,
  1e) rather than inventing a third.
- **Optimistic concurrency / CAS with idempotent conflict resolution
  (common in eventually-consistent systems)**: accept that two claims can
  race, detect the collision after the fact via a version/timestamp
  compare, and pick a deterministic tie-breaker (e.g., earliest
  `AdvertisedAt`/claim timestamp wins, matching `GetBacklogItemByExternalURL`'s
  own tie-breaker choice of `.First()` ordered by `CreatedAt`, 1a) — cheap,
  requires no synchronous cross-host round trip, and is consistent with
  requirements.md's explicit exclusion of "automatically resolving a
  genuine simultaneous-claim race" (i.e., detect + log + flag for human
  resolution, don't try to auto-merge).

## Summary of the single biggest structural gap

Every "is this already claimed/in-progress/duplicate" check that exists
today (1a-1d) is a local SQL query or a local tmux/OS liveness check against
state this process fully owns. None of them cross a process boundary. The
one piece of cross-host machinery that already exists and is genuinely
reusable is the gossip `HostRegistry`/`HostAdvertiser` (1e) and the deep-link
handoff resolver (1f) — but the registry carries no application payload
today, and the resolver requires already knowing the target host. The net
new primitive this project needs is a reverse index (external GitHub
URL → owning host + item ID) that doesn't exist in any form yet, plus a
staleness/expiry policy for it that should reuse, not reinvent, the
`workSessionStaleness`/`maxReworkBlockStaleness` (15 min) and
`DefaultHostRegistryTTL` (15 min) precedents already in the codebase.
