# Requirements: Cross-Host Claim/Dedup System

Backlog item: `5fba8e5f-2810-42aa-a32f-58684a768e43` — "Cross-host claim/dedup system for
backlog items, GitHub issues, and PRs"

## Problem

Every stapler-squad host keeps its own independent backlog DB. `ImportGitHubIssue`
([`server/services/backlog_service_sync.go:251`](server/services/backlog_service_sync.go#L251))
dedupes only against its own storage via `GetBacklogItemByExternalURL`
([`server/services/backlog_service_sync.go:278`](server/services/backlog_service_sync.go#L278)) —
there is no check against any other host. Once backlog import/triage runs on more than one
host against the same repos, two hosts can independently import the same GitHub issue, each
spawn a work session, and each open a PR for it, with neither aware of the other.

The gossip-based Workspace Host Registry (`session/host_registry.go`, ADR-002) and the
`ssq://<hostname>/<type>/<version>/<id>` deep-link scheme (`session/deeplink/url.go`,
`server/services/deep_link_resolver.go`, `/resolve` page, "Copy Link" in
`BacklogItemDetail.tsx`) are fully wired end-to-end — confirmed via
`tests/e2e/backlog-cross-host-handoff.spec.ts` and `backlog-deep-link-resolve.spec.ts`. So
resolving *a specific link a human already has* to its owning host works today. What's
missing is the other direction: given a GitHub issue or PR, nothing lets a host (or a human)
discover whether some other host has already claimed it, or find a deep link back to
whichever host/item/session actually owns it.

## Prerequisite

**#475** ("Add a stable per-instance identifier to backlog item claims for cross-host
provenance") — confirmed still `OPEN` via `gh issue view 475`. It is explicitly scoped to
*recording* which host made a claim, and explicitly excludes implementing remote/multi-host
claiming or a distributed claim protocol, and claim expiry/reclaim logic. This item is the
follow-on that consumes #475's host identifier to build the actual cross-host
coordination/dedup layer. **Implementation here cannot start until #475 lands or its
identifier shape is settled** — research/plan work can proceed in parallel.

## Goals

1. Importing a GitHub issue (or picking up a queued item) on host B checks whether host A
   already claimed the same issue/item, and blocks or flags it instead of silently creating
   a second competing backlog item.
2. A claim record carries (or can produce on demand) a deep link back to the owning
   host/item, so any host — or a human via the GitHub issue/PR itself — can answer "who
   already owns this?" without guessing from bare session UUIDs.
3. PR provenance is derivable regardless of which host's session actually pushed it —
   `report_pr_created` / the GitHub webhook handlers
   (`server/services/github_webhook_pr_fix.go`, `server/services/github_webhook_handler.go`)
   should be able to trace a PR back to its originating host/item. **Narrowed per plan.md Story
   3.1.3's verification**: `github_webhook_handler.go` is pure event-type routing that forwards
   every PR-related event type to `github_webhook_pr_fix.go`'s `handlePRFixEvent` and does no
   provenance resolution of its own — wiring the fallback lookup into `handlePRFixEvent` alone
   satisfies this goal for both files.
4. Works for any backlog item source (manual import, NL-created, forward/backward-synced via
   `ItemSource`), not just the GitHub-issue-import path.

## Design directions (not prescriptive — input to the research/plan phases)

- Extend the existing gossip channel (`HostAdvertiser`/`HostRegistry`, ADR-002,
  `DefaultHostAdvertisementInterval` = 5 min, `DefaultHostRegistryTTL`) to also carry a
  lightweight claim index (external issue/PR URL → owning host + item deep link),
  consistent with its existing eventual-consistency/TTL tolerance.
- Or a narrower v1: a `check_cross_host_claim`-style MCP tool/RPC that queries known peers'
  advertised addresses on demand — mirroring `list_workspace_peers`
  (`session/workspace_peers.go`) but across hosts instead of within one — used by
  triage/import flows and by a human before importing.
- PR-side visibility: stamp the deep link (or #475's host identifier) into the PR body/comment
  so ownership is inspectable from GitHub directly, not only from inside stapler-squad.

## Kano classification

Performance/one-dimensional for anyone running stapler-squad across multiple hosts against
shared repos; invisible/irrelevant for single-host setups.

## Priority signal (qualitative RICE)

- **Reach:** low today, grows directly with multi-host adoption.
- **Impact:** moderate-high where it applies — prevents genuinely wasted duplicate
  sessions/PRs, not a cosmetic annoyance.
- **Confidence:** high that the gap is real (verified directly against `ImportGitHubIssue`
  and the host registry code); lower on exact protocol shape.
- **Effort:** moderate-large — builds on #475, extends an existing gossip subsystem rather
  than inventing one.

## Evidence basis and success metrics

**Evidence basis (UNVERIFIED — preventive, not reactive):** no real-world incident of this
duplicate-claim failure has been logged — this repo does not yet run stapler-squad's backlog
triage on more than one host against shared repos in practice. The gap is confirmed real by
direct code inspection (`ImportGitHubIssue` only checks local storage, per the Problem section
above), but the "Reach: low today" signal in Priority signal above should be read as "the code
path is provably reachable," not "this has already cost anyone real time." This is a
preventive investment against a multi-host adoption plan, not a fix for an observed incident —
size the priority accordingly rather than treating it as an active fire.

**Outcome-level success metrics** (beyond the functional ACs below):
- Zero duplicate cross-host PRs opened against the same GitHub issue, measured over the first
  90 days after ≥2 hosts run backlog triage against a shared repo (the earliest point this
  failure mode can actually occur).
- Time from a second host's claim attempt to that host seeing "already claimed by \<host\>" ≤
  one gossip interval (`DefaultHostAdvertisementInterval`, 5 min) in the fallback/gossip path,
  or immediate in the primary on-demand-check path (per Open Questions' resolved cadence).
- Zero reports of a false "already claimed" block that couldn't be resolved via the in-product
  Override path (plan.md Story 2.2.1/2.2.2's override affordances) within the same operator
  session.

**Scope reconciliation — why the full gossip-index design over the narrower v1:** the "Design
directions" section above named a narrower on-demand-only `check_cross_host_claim` RPC (no
persisted claim index) as an alternative. `research/build-vs-buy.md` and `research/architecture.md`
evaluated it directly: an on-demand-only check satisfies Goal 1 (block/flag at import time) but
not Goal 2 (a claim record must be discoverable *after the fact*, e.g. from a GitHub issue
found weeks later, when the claiming host might be offline and unable to answer an on-demand
query) or Goal 3 (PR-provenance stamping needs something to stamp *from*, not just a live
query). The chosen design keeps the on-demand check as the primary path (per the resolved Open
Question on cadence) and adds the persisted, gossiped index specifically to cover Goals 2–3 —
it is not scope creep, but the two goals the narrower v1 admittedly can't reach. Phases 0–4 of
plan.md are independently shippable and individually deliver a shrinking scope if priorities
change before Phase 5 (gated on external issue #475).

## Success criteria (acceptance criteria)

1. Importing/claiming the same GitHub issue from two hosts sharing a workspace either blocks
   with a clear "already claimed by \<host\>, see \<deep link\>" message, or is surfaced for
   manual reconciliation — never silently produces two independent items/sessions/PRs.
2. Given a GitHub issue or PR, it's possible to find which stapler-squad host/item/session is
   or was responsible for it via a deep link or equivalent, without reading session UUIDs.
3. No behavior change for single-host setups.

## Out of scope

- #475's own scope (the host identifier itself) — this item consumes it, doesn't redo it.
- Full distributed consensus/locking; best-effort, eventually-consistent dedup (matching
  ADR-002's existing tolerance for stale peer data) is acceptable for v1.
- Automatically resolving a genuine simultaneous-claim race (both hosts claim within the same
  gossip interval) — flag it for a human, don't auto-arbitrate.

## Open questions — resolved by Phase 2 research

- **Cadence:** a *separate, parallel* `ClaimIndex`/claim-gossip channel alongside
  `AdvertisementRecord` — not a field bolted onto it (payload-growth and differing
  TTL/lifecycle semantics; see `research/architecture.md`). A synchronous on-demand peer
  query (mirroring `deep_link_resolver.go`'s `checkLiveness` bounded-timeout pattern) is
  the primary check path; periodic gossip is the fallback/propagation mechanism, not the
  sole source of truth.
- **Claim index key:** the external issue/PR URL directly (`ClaimRecord{ExternalURL,
  ClaimingHostID, ItemDeepLink, ClaimedAt}`), so it works pre-import too — confirmed by
  both `research/architecture.md` and `research/features.md`.
- **Human-facing surface:** not a hard block — reuse `DeepLinkErrorBanner.tsx`'s
  `role="alert"`/`role="status"` split and `GateVerdictBox`'s existing Override verb
  rather than inventing new UI vocabulary. No dedicated Omnibar/`report_duplicate`-style
  UI exists to mirror (that flow is backend/agent-only); see `research/ux.md`.
- **#475 timing:** the identifier should be `session.HostID` (existing ULID newtype,
  reused as-is, not a new type). Claim-index/gossip plumbing itself doesn't depend on
  #475's schema and can be designed now; only wiring host identity onto `BacklogItemData`
  needs #475's field to land. Treat the identifier as opaque in the interim (read through
  one accessor) so this design degrades gracefully if #475's final shape shifts — see
  `research/architecture.md` and `research/pitfalls.md`.

## New risks surfaced by research (not in original scope, feed into plan.md)

- `HostRegistry.Prune()` is currently dead code — wired up in tests only, never called
  from `main.go`. A claim index does **not** inherit working staleness-pruning for free;
  this needs fixing as a prerequisite or companion change (`research/pitfalls.md`).
- No `docs/adr/ADR-002-*.md` file actually exists — "ADR-002" is only referenced in code
  comments, and the number is already taken by an unrelated ADR
  (`docs/architecture/decisions/002-external-session-streaming-mode.md`, different
  numbering namespace). Worth writing a real ADR before extending this design further.
- `DequeueNextQueuedItems`'s `dequeueMu` (`backlog_service_triage.go:736`) is a second,
  currently-invisible claim point beyond `ImportGitHubIssue` — two hosts that already
  imported the same issue before gossip catches up will each independently dequeue and
  spawn a session (`research/architecture.md`).
