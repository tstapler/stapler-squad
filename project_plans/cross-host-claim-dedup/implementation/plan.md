# Implementation Plan: Cross-Host Claim/Dedup System

**Feature**: Detect and surface when two stapler-squad hosts independently claim the same
GitHub issue/PR (or any external-URL-bearing backlog item), so imports/dequeues don't silently
spawn duplicate work, and provenance is traceable via deep link regardless of which host acted.
**Date**: 2026-09-23
**Status**: Ready for implementation
**ADRs**: [ADR-001: Separate claim-gossip channel](../decisions/ADR-001-separate-claim-gossip-channel.md), [ADR-002: Record claim at single choke point](../decisions/ADR-002-record-claim-at-single-choke-point.md)

**Correction to Phase 2 research**: `research/stack.md` and `research/pitfalls.md` both state no
standalone ADR-002 document exists for the Workspace Host Registry gossip design. That's true of
`docs/adr/`, but the document does exist at
[`project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md`](../../backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md)
— confirmed by directory listing and by reading it; its content matches the design the code
comments describe verbatim. The real, verified gap is narrower than "the ADR was never written":
**nothing in `session/host_registry.go` or `session/host_advertiser.go` links to it**
(`grep -rn "ADR-002-gossip-based-host-registry" --include=*.go --include=*.md .` outside that
project's own folder returns zero hits), and it isn't at the `docs/adr/` path a reader would
check first. Story 0.1.2 below fixes discoverability, not authorship.

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `HostID` | Existing ULID newtype identifying a stapler-squad install (`session/host_identity.go:53`). | **Reused as-is** for `ClaimingHostID` — no new identifier type, per `research/architecture.md` §4. |
| `ClaimRecord` | Signed fact: `HostID` X claims external URL Y, with a deep link back to the owning item. Fields: `ExternalURL string, ClaimingHostID HostID, ItemDeepLink string, ClaimedAt time.Time, PublicKey ed25519.PublicKey, Signature []byte, Disputed bool`. `Disputed` is **not** part of `signingPayload()` — it's local-observation metadata (this host saw a same-URL conflict), not a portable signed fact; see Story 1.1.2's CB-2 AC. | New type, `session/claim_index.go`. Signing shape mirrors `AdvertisementRecord.signingPayload()`. |
| `ClaimIndex` | Sibling persisted store to `HostRegistry`: `map[externalURL]ClaimRecord`, same flock+mutex+atomic-write-then-rename persistence. Holds a `*HostRegistry` reference so `RecordClaim` can TOFU-cross-check an incoming record's `PublicKey` against the key already pinned for its `ClaimingHostID`, mirroring `HostRegistry.Advertise`'s own pin check (`session/host_registry.go:213`) — see Story 1.1.1/1.1.2's forgery-rejection AC. | New type, `session/claim_index.go`. |
| `ClaimGossiper` | Sibling to `HostAdvertiser`: broadcasts newly-recorded `ClaimRecord`s to known peers over a separate endpoint, one-hop re-gossip on first sight. `BroadcastOnce(ctx, record ClaimRecord) error` is **record-scoped** — it sends exactly the one record passed in, never the full accumulated claim set. The periodic `Run(ctx)` ticker does **not** re-broadcast previously-sent claims on every tick (that would reproduce the unbounded-payload-growth problem ADR-001 rejected, just on a new channel); it exists only to backfill claims to peers discovered after their original broadcast, scoped to the still-open per-peer backfill set, not a full-state resend. | New type, `session/claim_gossiper.go`. See ADR-001. |
| `ClaimAdvertisementEndpointPath` | HTTP path for the claim-gossip POST endpoint (`"/internal/claim-advertisement"`), sibling to `AdvertisementEndpointPath`. | Const in `session/claim_gossiper.go`. |
| `session.ClaimRecorder` | **New port, defined in package `session`** (not `server/services`): `RecordClaim(ctx context.Context, record ClaimRecord) error`. Exists so `Storage.CreateBacklogItem` (also package `session`) can call the write side of the claim system without `session` importing `server/services` back — the same shape as `ItemChangePublisher` (`session/backlog_item_change.go:84`) and `CallbackDispatcher` (`session/callback_dispatcher.go:15`). Wired via `Storage.SetClaimRecorder(...)`, mirroring `Storage.SetCallbackDispatcher` (`session/storage.go:289-292`). See Story 1.3.1. | `session/claim_recorder.go` (new). |
| `crossHostClaimChecker` | Narrow consumer-side interface, **read-only**: `CheckClaim(ctx, externalURL) (ClaimVerdict, error)`. Does **not** have a `RecordClaim` method — the write path goes through `session.ClaimRecorder`/`Storage.SetClaimRecorder` instead (Story 1.3.1), since `crossHostClaimChecker` lives in `server/services`, which `session.Storage` cannot import back without a cycle. | `server/services/claim_checker.go`, mirrors `HostResolver` in `deep_link_resolver.go`. |
| `ClaimVerdict` | Sum type for a claim check's outcome: `Unclaimed`, `HeldByOther(ClaimRecord)`, or `CheckIndeterminate` (no peer answered in time). | A struct with an exported `Kind` const, not a bare bool — per `research/ux.md` §"Error states": "couldn't verify" must never collapse into "unclaimed." Constructed only via `NewUnclaimedVerdict()`/`NewHeldByOtherVerdict(record)`/`NewIndeterminateVerdict()` (Task 2.1.1a) so `Kind`/`Record` can't be mismatched at a call site. |
| `localClaimChecker` | Production `crossHostClaimChecker`: **reads** the local `*session.ClaimIndex` for `CheckClaim`, optionally querying live peers on demand first. Does not implement any write path — recording happens via `session.ClaimRecorder` (Story 1.3.1), not through this type. | `server/services/claim_checker.go`. |
| `unimplementedClaimChecker` | Nil-safe default: `CheckClaim` always returns `Unclaimed`. | Same file, mirrors `unimplementedHostResolver`. Guarantees AC3 (no behavior change for single-host / not-yet-wired setups). |
| `ClaimConflict` | Event: two different non-empty `ClaimingHostID`s observed for the same `ExternalURL` (via gossip or on-demand query). | Logged (`claim_index.conflict_detected`) and marks the record `Disputed: true` (Task 1.1.2e) so it's surfaced to the UI (Story 4.1.1's amended AC, `design/ux.md` Surface 1's "Disputed" state) — LWW still picks which record answers a `CheckClaim` in the meantime, but which one is picked is never treated as auto-resolving the dispute (per requirements.md's Out of Scope carve-out; cross-artifact-consistency BLOCKER CB-2). |
| `PRProvenanceStamp` | Text written into a PR body/comment at `report_pr_created` time, carrying the claiming host's opaque `HostID` and `ItemDeepLink` — never a raw address. | See `research/pitfalls.md` §3's SSRF/leak warning. |
| `ClaimedByHostID` | **#475's** forthcoming field on `BacklogItemData`/ent schema (out of this item's scope to define). | Read only through one accessor (`BacklogItemData.ClaimedByHostIDOrEmpty()`, Phase 5) so a shape change in #475 touches one place. |
| `check_cross_host_claim` | New MCP tool mirroring `list_workspace_peers`'s shape: synchronously queries known peers for a given external URL. | `server/mcp/tools_claim.go`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `ClaimIndex`/`ClaimGossiper` wire format | Separate gossip channel/type (sibling to `HostRegistry`/`HostAdvertiser`) | ADR-001, `research/architecture.md` §1 | Field on `AdvertisementRecord` | Payload growth scales with claim count; different signing scope; different TTL/lifecycle semantics (host liveness vs. claim validity) |
| Claim-check/record seam at `ImportGitHubIssue`/`DequeueNextQueuedItems` | Adapter/Strategy — `crossHostClaimChecker` interface (GoF) | `research/architecture.md` §6 (Tech Debt Disposition) | Direct dependency on concrete `*session.ClaimIndex` type from `server/services` | Testable in isolation (fake checker), mirrors `deep_link_resolver.go`'s `HostResolver` precedent; keeps legacy `backlog_service_sync.go`/`backlog_service_triage.go` dedup logic untouched |
| Claim recording call site | Single choke point — inside `Storage.CreateBacklogItem`, invoked via a `session`-owned `ClaimRecorder` port (`Storage.SetClaimRecorder(...)`), **not** via `crossHostClaimChecker` | ADR-002 | Instrument `ImportGitHubIssue`, `SyncLoop`, and NL-create separately; also rejected: routing the write through `crossHostClaimChecker` (`server/services`), which would require `session` to import `server/services` — a cycle, since `server/services` already imports `session` | Structurally eliminates the "4th creation path forgets to record a claim" registry-drift risk `research/features.md` names, without the layering violation the first draft of this plan had (see Story 1.3.1) |
| GitHub label/assignee pre-check (`research/build-vs-buy.md`'s "Viable (supplement only)" option) | Rejected for v1, not silently dropped | `research/build-vs-buy.md` §"Viable (supplement only)" | A zero-gossip-latency pre-check layered ahead of the claim index for the GitHub-import case specifically | Adds a second, GitHub-specific dedup signal with its own staleness/rate-limit tradeoffs for a case the gossip-based `ClaimIndex` + on-demand peer query (Story 2.1.2) already covers with acceptable latency; revisit only if Story 2.1.2's real-world false-negative rate turns out to matter in practice |
| Conflict resolution | Plain last-write-wins by `ClaimedAt`, tie-break on `HostID` string | `research/build-vs-buy.md` §3, mirrors `HostRegistry.Advertise`'s own replace rule | CRDT library (LWW-element-set) | Problem is a single-map comparison; adopting an obscure/low-maturity dependency costs more than ~30 lines of bespoke logic already precedented in this codebase |
| Transport/library | Extend existing bespoke HTTP gossip | `research/build-vs-buy.md` (Recommended row) | `hashicorp/memberlist`/`serf` | Solves a cluster-membership/UDP-transport problem this repo's small, mostly-known peer set doesn't have; would require opening a new transport (UDP) this deployment model doesn't support |
| `ClaimVerdict` | Sum type (`Kind` enum + payload), not a bool/pointer pair | type-driven-design | `(ClaimRecord, bool, error)` triple | A third state ("couldn't check") must be structurally distinct from "unclaimed" — a bool return makes that distinction easy to drop at a call site (`research/ux.md` §"Error states") |
| `HostID` reuse for `ClaimingHostID` | Newtype reuse, no new ID type | type-driven-design, `research/architecture.md` §4 | New `ClaimantID` type | Avoids the exact primitive-obsession/duplicate-identifier problem `.claude/rules/instance-lock-free-reads.md`'s sibling research calls out for backlog item IDs |
| PR provenance write | New function via `newGHRequestForHostWithToken` (Adapter over GitHub REST) | `.claude/rules/norawghrequest.md` | Raw `http.NewRequest` | Repo-wide lint rule (`tools/lint/norawghrequest`) blocks raw GitHub HTTP calls outside the approved constructors |
| Claim-conflict UI | Reuse `DeepLinkErrorBanner.tsx`'s `role="status"`/copy-host-address pattern | `research/ux.md` §0/§1 | New bespoke banner component/vocabulary | This is structurally the same "item lives elsewhere" cross-host message the banner already solves accessibly; a new component would re-derive a11y/analytics wiring from scratch |
| Human override action | Reuse `GateVerdictBox`'s **Override** verb | `research/ux.md` §2 | New "Resolve Conflict" action | Operators already know Approve/Reopen/Override/Skip-Gate from every other review-gated flow |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `session/host_registry.go`, `session/host_advertiser.go` | None (no God-Object/hotspot finding) | **Isolate via seam** | New sibling `ClaimIndex`/`ClaimGossiper` types (ADR-001) plumbed through `crossHostClaimChecker`; existing liveness/TTL logic untouched and independently testable |
| `session/host_registry.go`'s `Prune()` | Dead code in production — zero non-test callers, never scheduled from `main.go` (verified: `grep -rn '\.Prune()'` finds only test files) | **Refactor-first**, sequenced as Story 0.1.1 before any claim-index work | A claim design that assumed pruning "just works" would inherit an unbounded-growth registry; fixing the one-line wiring gap now is cheap and removes a standing correctness landmine this item would otherwise build on top of, even though claim *expiry* itself is intentionally NOT tied to host TTL (see next row) |
| Claim staleness vs. host-registry TTL | Conflating "host still advertising" with "claim still valid" (`research/pitfalls.md` §1) | **Isolate via seam** — `ClaimIndex` has its own lifecycle, does not consult `HostRegistry.Prune()`/TTL for claim expiry | A merged/closed PR's claim must clear regardless of whether the claiming host is still reachable; a reachable host's claim on a closed issue must not linger just because the host is alive. Different signal, different code path (Story 2.1.2, on-demand liveness check reused only to answer "is this claim record's host at least alive," not "is the claim itself still valid") |
| ADR-002 discoverability | Document exists (`project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md`) but zero cross-links from the ~15 code comments citing "ADR-002," and no canonical `docs/adr/` entry | **Refactor-first** (documentation), sequenced as Story 0.1.2 | Trivial fix (add a pointer file + fix comment references); leaving it undiscoverable makes every future reader re-litigate "where is ADR-002" the way this item's own research did |
| `server/services/backlog_service_sync.go` (`ImportGitHubIssue`) | None (no hotspot finding) | **Isolate via seam** | `crossHostClaimChecker` injected; existing local-dedup (`GetBacklogItemByExternalURL`) logic stays exactly as-is, claim check is additive |
| `server/services/backlog_service_triage.go` (`DequeueNextQueuedItems`) | Not separately re-verified for this item — the file is large (3,534 lines, 70 top-level functions) and high-churn (107 commits in the last 90 days), which reads as a plausible `code-hotspot-analysis` candidate, but no specific hotspot check for this item was run against it, so "None" would overstate confidence | **Isolate via seam**, regardless | `crossHostClaimChecker.CheckClaim` (local-only, per Story 2.2.2's revised AC — see below) injected; existing CAS (`transitionWithGuard`) logic stays exactly as-is, claim check is additive. The seam choice doesn't depend on whether this file is a confirmed hotspot — isolating new logic behind an injected interface is correct either way |
| `github/` package | No PR-comment/write capability exists at all (verified: zero `AddLabel`/`assignee`/comment-write matches) | **Extend as-is** | This is a net-new capability, not an existing violation being deepened; the new function follows the package's existing `newGHRequestForHostWithToken` convention from first principles |

---

## Migration Plan
No SQL/ent schema changes in this item's own scope — `ClaimedByHostID` (or equivalent) on
`BacklogItemData`/the ent schema is **#475's** migration, not this one's (`research/architecture.md`
§4). This item introduces one new on-disk file, `claim_index.json` (JSON, flock-guarded, same
recipe as `host_registry.json`) — not a database migration:
- **Reversibility**: trivially reversible — deleting the file (or reverting the code) leaves no
  schema artifact. A missing file reads as an empty claim set (`os.IsNotExist` handled the same
  way `host_registry.go:380` already handles it for `host_registry.json`).
- **Zero-downtime strategy**: N/A — additive file, no existing readers/writers to coordinate
  with. An old binary simply never creates or reads it.
- **Rollback procedure**: revert the code change; `claim_index.json` becomes an inert, unread
  file on disk.

## Observability Plan
- **Logs**: structured lines at every state transition, mirroring `host_registry.go`'s
  `log.Info`/`log.Warn` style —
  `claim_index.recorded` (external_url, claiming_host_id), `claim_index.conflict_detected`
  (external_url, existing_host_id, incoming_host_id), `claim_gossip.sent`/`claim_gossip.received`
  (host_id, is_new), `claim_index.record_failed` (err — never fails the underlying
  `CreateBacklogItem` call, per ADR-002), `import_github_issue.blocked_by_claim` (external_url,
  claiming_host_id, item_deep_link), `dequeue.blocked_by_claim` (item_id, external_url,
  claiming_host_id).
- **Metrics**: none new — matches this codebase's existing precedent for the sibling feature
  (`server/services/deep_link_resolver.go`'s own comment: "this is a low-volume,
  non-critical-path feature"). The structured logs above are greppable per
  `research/features.md` §3's "claim conflicts logged/auditable" requirement.
- **Alerts**: no new alerts required — a claim conflict is a per-item, human-reviewed event
  (Override/redirect via existing `GateVerdictBox` vocabulary), not an operational incident.

## Risk Control
- **Feature flag**: `cross_host_claim_dedup`, default **false** at merge, following the
  `pr_event_webhooks`-style `cfg.GetFeatureFlag(...)` gate already used for
  `handlePRFixEvent` (`server/services/github_webhook_pr_fix.go:568`). Flip to default-true
  once Story 2.2.1/2.2.2's structured logs (above) show zero false-block reports over a real
  multi-host observation window. Gates the *check* (Stories 2.2.1-2.2.3); `RecordClaim`
  (Story 1.3.1 onward, wired into `Storage.CreateBacklogItem` via `session.ClaimRecorder` — see
  Phase 1) stays always-on and harmless even while the flag is off, so a later flip to "on" has
  a populated index from day one instead of a cold start.
- **Rollback procedure**: flip `cross_host_claim_dedup` off (immediate, no deploy); standard
  revert via PR close + revert commit for anything structural. `claim_index.json` and gossip
  traffic are inert with the flag off.
- **Staged rollout**: full rollout on merge for the always-on claim-recording/gossip
  infrastructure (Phases 0-1, additive and silent); flag-gated staged rollout for the
  behavior-changing check (Phase 2) per above.

## Unresolved Questions
- [ ] Exact field name/nullability of #475's `ClaimedByHostID`-shaped field on `BacklogItemData`
      — blocks Story 5.1.1 only (not Phases 0-4, which read `HostID` from the *locally running
      process's own* `HostIdentity`, never from `BacklogItemData`) — owner: whoever lands #475.
- [ ] Whether `check_cross_host_claim`'s synchronous on-demand peer query adds perceptible
      latency to a *bulk* import (multiple queued issues in one operator action), per
      `research/ux.md` §2's "any network round-trip... must not spinner-gate a fast interaction"
      — blocks Story 2.2.3's default-enabled-vs-opt-in decision for bulk flows specifically —
      owner: whoever implements Story 2.2.3, resolve via a quick timing test against a
      2-3-peer fixture before enabling by default for bulk import.
- [ ] Whether `HostRegistry.Prune()` (Story 0.1.1) should also run on an explicit CLI trigger
      (`--list-known-hosts`-adjacent) in addition to a background ticker, for operators who want
      to force-clear a decommissioned host immediately rather than wait out the TTL — not
      required for any acceptance criterion in this item; flag for a future item if requested.

Two of `research/ux.md`'s own open questions are resolved here rather than left open (Phase 4
below): the UI surface is a per-item inline banner **and** a board-level badge (not a global
`SystemBanner`) — ux.md's own JTBD framing ("catch this before spawning a session, not after")
argues board-level visibility is necessary, and a global banner would be a second, redundant
surface for a feature the operator only cares about at import/triage time. The "Copy Link" third
mode (ux.md open question 2) is **not** added — the conflict banner's own "Copy deep link to
owning host" action (mirroring `DeepLinkErrorBanner`'s `onCopyHostAddress`) already covers that
need.

## Dependency Visualization
```
Phase 0 (foundational, no #475 dependency)
  Epic 0.1 ── Story 0.1.1 (wire Prune())
           └─ Story 0.1.2 (cross-link ADR-002)
        │
        ▼
Phase 1 (ClaimIndex + ClaimGossiper core — no #475 dependency)
  Epic 1.1 ── Story 1.1.1 (ClaimRecord type + signing)
           └─ Story 1.1.2 (ClaimIndex persisted store)
        │
        ▼
  Epic 1.2 ── Story 1.2.1 (ClaimGossiper + HTTP endpoint)
           └─ Story 1.2.2 (wire into main.go)
        │
        ▼
  Epic 1.3 ── Story 1.3.1 (wire RecordClaim into Storage.CreateBacklogItem
                            via session.ClaimRecorder — the ADR-002 choke point,
                            using local HostIdentity.ID, no #475 dependency)
        │
        ▼
Phase 2 (claim check integration — no #475 dependency)
  Epic 2.1 ── Story 2.1.1 (crossHostClaimChecker interface + impls)
           └─ Story 2.1.2 (on-demand synchronous peer query)
        │
        ▼
  Epic 2.2 ── Story 2.2.1 (ImportGitHubIssue integration)
           ├─ Story 2.2.2 (DequeueNextQueuedItems integration)
           ├─ Story 2.2.3 (check_cross_host_claim MCP tool)
           └─ Story 2.2.4 (NL-create + ItemSource sync integration)
        │
        ├──────────────────────────────┐
        ▼                              ▼
Phase 3 (PR provenance —          Phase 4 (UI — depends on Phase 2's
no #475 dependency)               ClaimVerdict/ClaimRecord shape)
  Epic 3.1 ── Story 3.1.1           Epic 4.1 ── Story 4.1.1 (banner component)
           ├─ Story 3.1.2                    ├─ Story 4.1.2 (wire into detail)
           └─ Story 3.1.3                    └─ Story 4.1.3 (board badge)
        │                                       │
        └───────────────┬───────────────────────┘
                         ▼
Phase 5 (BLOCKED on #475 landing)
  Epic 5.1 ── Story 5.1.1 (populate ClaimingHostID from #475's field)
           └─ Story 5.1.2 (accessor fallback for pre-#475 items)
```
Note the load-bearing property this diagram is meant to make visible: **Phases 0-4 are fully
buildable and shippable (behind the feature flag) before #475 lands.** Only Phase 5 — using
#475's own recorded claimant on *existing* backlog items instead of the locally-running
process's live `HostIdentity` — is blocked. Everything else already has the identifier it needs
(`session.HostIdentity.ID`, minted per-install today, per `research/architecture.md` §4).
Story 1.3.1 is what makes this true in practice, not just in principle: without it, `RecordClaim`
is defined but never called against production data, and Phases 0-4 alone would be
functionally inert — compilable and flag-gateable, but the claim index would never actually
populate. Phase 5 is narrowed accordingly: it only swaps the *source* of `ClaimingHostID` from
the local process's live identity to #475's recorded claimant field; it does not introduce the
`RecordClaim` call site itself (that's Story 1.3.1's job, already done by Phase 1).

---

## Phase 0: Foundational fixes
### Epic 0.1: Registry hygiene and ADR discoverability
**Goal**: Fix the two standing gaps research surfaced in the substrate this feature builds on,
before adding new gossip traffic to it.

#### Story 0.1.1: Wire `HostRegistry.Prune()` into a running loop
**As an** operator running stapler-squad on multiple hosts, **I want** stale peer entries to
actually expire, **so that** a decommissioned host's registry entry (and, once Phase 1 lands,
its claims) don't accumulate forever.
**Acceptance Criteria**:
- `HostRegistry.Prune()` is invoked periodically by the running server, not only by unit tests.
  - *Given* a server process with `hostRegistry` and `advertiser` constructed (as in
    `main.go:1541-1559`), *When* the process runs for longer than
    `session.DefaultHostRegistryTTL` with no advertisement received from a previously-known
    peer, *Then* that peer's `RegistryEntry` is removed from `host_registry.json` without
    requiring a restart.
**Files**: `main.go`, `session/host_registry.go` (no logic change, only confirms `Prune()`'s
existing signature is call-compatible with a ticker loop).

##### Task 0.1.1a: Add a `Prune()` ticker loop (~5 min)
- In `main.go`, immediately after `go advertiser.Run(ctx)` (line 1558), start a second
  goroutine: `go func() { ticker := time.NewTicker(session.DefaultHostAdvertisementInterval); defer ticker.Stop(); for { select { case <-ctx.Done(): return; case <-ticker.C: if err := hostRegistry.Prune(); err != nil { log.Warn("host_registry.prune_failed", "err", err) } } } }()`.
- Files: `main.go`.

##### Task 0.1.1b: Add a regression test proving `main.go`'s wiring calls `Prune`  (~5 min)
- Add/extend a `main_test.go`-style integration test (or, if `main.go`'s remote-server setup
  isn't independently testable, a `session`-package test asserting the ticker-loop shape is
  exercised) confirming an entry older than TTL is gone after the loop runs — reuse
  `session/host_registry_test.go`'s injectable-`Clock` pattern so no wall-clock sleep is needed.
- Files: `session/host_registry_test.go` or a new `main_integration_test.go` per existing
  convention in this repo (check for one before adding a new file).

#### Story 0.1.2: Make the existing ADR-002 document discoverable
**As a** future contributor reading `session/host_registry.go`'s "per ADR-002" comments,
**I want** a working link to the actual decision record, **so that** I don't have to
re-derive the design from scattered comments (as this item's own Phase 2 research had to).
**Acceptance Criteria**:
- Every "ADR-002" comment reference in `session/host_registry.go` and
  `session/host_advertiser.go` resolves to a real path.
  - *Given* a reader opens `session/host_registry.go:40-44`, *When* they follow the comment's
    ADR-002 reference, *Then* it names
    `project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md`
    explicitly (not a bare "ADR-002" string).
- A canonical pointer exists at the path a contributor would check first.
  - *Given* `docs/adr/` has no file numbered 002 (confirmed: `ls docs/adr/` shows 001, 003+,
    ADR-001, ADR-022+ — 002 is unclaimed in that directory's own numbering), *When* a
    contributor looks in `docs/adr/` for ADR-002, *Then* `docs/adr/ADR-002-workspace-host-registry-gossip.md`
    exists and points to the canonical decision record.
**Files**: `session/host_registry.go`, `session/host_advertiser.go`, new
`docs/adr/ADR-002-workspace-host-registry-gossip.md`.

##### Task 0.1.2a: Add a canonical pointer file (~3 min)
- Create `docs/adr/ADR-002-workspace-host-registry-gossip.md` containing a one-paragraph
  summary and a link: "See
  [`project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md`](../../project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md)
  for the full decision record."
- Files: `docs/adr/ADR-002-workspace-host-registry-gossip.md`.

##### Task 0.1.2b: Fix the ~15 code-comment references (~5 min)
- In `session/host_registry.go` (lines ~40-44, 73, 254, 279, 313) and
  `session/host_advertiser.go` (lines ~18-24, 27-37, 58-61), change bare "ADR-002" mentions to
  reference `docs/adr/ADR-002-workspace-host-registry-gossip.md`.
- Files: `session/host_registry.go`, `session/host_advertiser.go`.

---

## Phase 1: Claim Index core (gossip substrate)
### Epic 1.1: `ClaimRecord` + `ClaimIndex` persistence
**Goal**: A durable, signed, TOFU-verified local store of claims — the write side, no network
yet.

#### Story 1.1.1: Define `ClaimRecord` and its signing shape
**As the** claim-gossip subsystem, **I want** a signed, tamper-evident claim fact type,
**so that** a peer can't forge a claim for a `HostID` it doesn't control.
**Acceptance Criteria**:
- `ClaimRecord.Verify()` rejects a record whose signature doesn't match its `signingPayload()`
  (bit-flip tampering).
  - *Given* a `ClaimRecord{ExternalURL: "https://github.com/o/r/issues/1", ClaimingHostID:
    hostA.ID, ItemDeepLink: "ssq://hostA/backlog/v1/bl_01J...", ClaimedAt: t}` signed by
    `hostA`'s private key, *When* a byte in `Signature` is flipped, *Then* `Verify()` returns
    `false`.
- **A fresh-keypair forgery is rejected, not just a tampered signature.** `ClaimRecord.Verify()`
  alone only proves internal consistency (the embedded `PublicKey` matches the `Signature`) — it
  does **not** prove the signer is actually `ClaimingHostID`. The TOFU cross-check against
  `HostRegistry`'s already-pinned key for that `HostID` (mirroring `HostRegistry.Advertise`'s
  `bytes.Equal(existing.PublicKey, record.PublicKey)` check, `session/host_registry.go:213`)
  happens one layer up, in `ClaimIndex.RecordClaim` (Story 1.1.2), not in `Verify()` itself.
  - *Given* attacker-controlled code generates a brand-new Ed25519 keypair, self-signs a
    `ClaimRecord{ClaimingHostID: hostA.ID, ...}` with the new (not hostA's) private key — a
    record that passes `Verify()` cleanly, since `Verify()` only checks internal
    consistency — *When* `ClaimIndex.RecordClaim` processes it and `HostRegistry` already has a
    pinned `PublicKey` for `hostA.ID` from a prior legitimate advertisement, *Then* `RecordClaim`
    rejects the record (same silent-reject-not-error convention as `HostRegistry.Advertise`) and
    it is never stored or re-gossiped.
**Files**: `session/claim_index.go` (new).

##### Task 1.1.1a: Add `ClaimRecord` struct + `signingPayload`/`Sign`/`Verify` (~5 min)
- Mirror `AdvertisementRecord`'s shape exactly (`session/host_registry.go:74-111`): struct
  fields, a private `signingPayload()` covering `ExternalURL, ClaimingHostID, ItemDeepLink,
  ClaimedAt`, `Sign(identity HostIdentity)`, `Verify() bool` using the existing
  `VerifyAdvertisement` helper (`session/host_identity.go:154`). Doc comment on `ClaimRecord`
  cites [ADR-001](../decisions/ADR-001-separate-claim-gossip-channel.md) for why this is a
  sibling type rather than a field on `AdvertisementRecord` — the code-facing anchor for that
  decision so a future reader doesn't have to rediscover it the way this item's own research
  had to for ADR-002 (Story 0.1.2).
- Files: `session/claim_index.go`.

##### Task 1.1.1b: Unit tests for sign/verify/tamper-rejection (~5 min)
- Table-driven test mirroring `session/host_registry_test.go`'s advertisement-record tests:
  valid signature accepted, flipped byte rejected, wrong public key rejected.
- Files: `session/claim_index_test.go` (new).

#### Story 1.1.2: `ClaimIndex` persisted store
**As the** claim subsystem, **I want** a flock-guarded, atomically-written JSON store of
`ClaimRecord`s keyed by `ExternalURL`, **so that** claims survive a restart and are safe under
concurrent process access, matching `HostRegistry`'s existing guarantees.
**Acceptance Criteria**:
- A `RecordClaim` call from one process is visible to a `CheckClaim` call from a second
  process reading the same `stateDir` after the first process's write returns.
  - *Given* an empty `claim_index.json`, *When* process A calls
    `RecordClaim(ctx, ClaimRecord{ExternalURL: "https://github.com/o/r/issues/1", ...})` and it
    returns nil error, *Then* process B constructing a fresh `NewClaimIndex(stateDir)` and
    calling `CheckClaim(ctx, "https://github.com/o/r/issues/1")` returns that same record.
- A conflicting claim (different `ClaimingHostID` for an `ExternalURL` already held) is detected,
  not silently overwritten.
  - *Given* `claim_index.json` already holds `{ExternalURL: U, ClaimingHostID: hostA}`, *When*
    `RecordClaim` is called with `{ExternalURL: U, ClaimingHostID: hostB, ClaimedAt: later}`,
    *Then* the store logs `claim_index.conflict_detected` (host_a=hostA, host_b=hostB,
    external_url=U) and applies last-write-wins by `ClaimedAt` (ties broken by `HostID` string
    comparison) rather than silently discarding either fact.
- **`RecordClaim` rejects a record whose `PublicKey` doesn't match the `ClaimingHostID`'s
  already-pinned key in `HostRegistry`** — the TOFU cross-check Story 1.1.1's forgery AC
  requires, since `ClaimRecord.Verify()` alone only proves internal self-consistency.
  - *Given* `HostRegistry` already has a pinned `PublicKey` for `hostA.ID` from a prior
    advertisement, *When* `RecordClaim` receives a `ClaimRecord{ClaimingHostID: hostA.ID, ...}`
    signed with a *different* keypair (one that isn't hostA's pinned key), *Then* `Verify()`
    passes (internally consistent) but `RecordClaim` still rejects the record — same
    silent-reject convention as `HostRegistry.Advertise` — and it is never persisted or handed
    to `ClaimGossiper.ReGossip`.
- **A genuine same-URL conflict from two different hosts is flagged for a human, never silently
  auto-arbitrated.** (Resolves cross-artifact-consistency BLOCKER CB-2, the highest-severity
  finding — `requirements.md`'s Out of Scope section explicitly excludes "automatically resolving
  a genuine simultaneous-claim race... flag it for a human, don't auto-arbitrate," but the
  original AC above only logged `claim_index.conflict_detected` and silently applied last-write-
  wins, with no human-facing state at all, confirmed absent from `design/ux.md`'s original
  Surface 1.) `RecordClaim` still needs *some* deterministic in-memory answer immediately (a
  peer's `CheckClaim` can't block on a human), so LWW continues to pick which record is served —
  but the picked record is now also marked `Disputed: true` (a new, **unsigned, local-only**
  field on the stored `ClaimRecord` — not part of `signingPayload()`, since it records this
  host's own observation of a conflict, not a portable signed fact) until a human resolves it.
  - *Given* `claim_index.json` already holds `{ExternalURL: U, ClaimingHostID: hostA, Disputed:
    false}`, *When* `RecordClaim` is called with `{ExternalURL: U, ClaimingHostID: hostB}` (a
    different, non-empty `ClaimingHostID`), *Then* the persisted entry for `U` (whichever record
    LWW selects) has `Disputed: true`, and `CheckClaim`/`crossHostClaimChecker.CheckClaim`
    surface that flag on the returned record (still `Kind: ClaimHeldByOther` — no new
    `ClaimVerdict` kind is introduced, keeping the blast radius to this field rather than the
    sum-type's call sites) so the UI can render a distinct "disputed" treatment instead of a
    plain single-claimant one (see Story 4.1.1's amended AC and `design/ux.md` Surface 1's new
    "Disputed" state).
  - *Given* an entry has `Disputed: true`, *When* a human resolves it via the same Override
    interaction Story 4.1.2 already wires up (Override in this state means "I've checked, this
    dispute is resolved in favor of the host I'm working on"), *Then* `RecordClaim`'s caller-
    facing counterpart clears `Disputed` back to `false` for that entry — the dispute is a
    transient "needs a human look" flag, not a permanent record property.
**Files**: `session/claim_index.go`.

##### Task 1.1.2a: `ClaimIndex` struct + `NewClaimIndex`/`NewClaimIndexWithClock` (~5 min)
- Copy `HostRegistry`'s shape exactly: `stateDir`, `clock Clock` (reuse the existing `Clock`
  interface from `session/host_identity.go`... actually `session/host_registry.go:52`, same
  package), `lockFile *flock.Flock`, `mu sync.Mutex`, `entries map[string]ClaimRecord` keyed by
  `ExternalURL`. **Also add a `hostRegistry *HostRegistry` field** — required by Task 1.1.2b's
  TOFU pin check — so `NewClaimIndex(stateDir string, hostRegistry *HostRegistry)` (and its
  `WithClock` sibling) both take the registry as a constructor argument, the same way
  `RegisterHostAdvertisementRoute` (`server/auth/host_advertisement.go:29`) is given a
  `*session.HostRegistry` for the equivalent check on `AdvertisementRecord`. Constants:
  `claimIndexFileName = "claim_index.json"`, `claimIndexLockFileName = "claim_index.lock"`, reuse
  `hostRegistryLockTimeout`'s value (5s).
- Files: `session/claim_index.go`.

##### Task 1.1.2b: `RecordClaim` with TOFU pin check, conflict detection + last-write-wins (~5 min)
- `func (c *ClaimIndex) RecordClaim(record ClaimRecord) (conflict bool, err error)`: verify
  signature (reject silently, mirroring `HostRegistry.Advertise`'s TOFU-reject-not-error
  pattern); **then**, using `c.hostRegistry`, look up the pinned `PublicKey` for
  `record.ClaimingHostID` and reject (same silent-reject convention) if it exists and doesn't
  `bytes.Equal` `record.PublicKey` — this is the actual forgery protection Story 1.1.1's second
  AC requires, not `Verify()` alone (mirrors `HostRegistry.Advertise`'s
  `bytes.Equal(existing.PublicKey, record.PublicKey)` check, `session/host_registry.go:213`
  exactly; if no pin exists yet for that `HostID`, accept-on-first-sight the same way
  `HostRegistry.Advertise` does for a never-before-seen host). Only after both checks pass:
  compare against existing entry for `ExternalURL` if present — different `ClaimingHostID` logs
  `claim_index.conflict_detected` and keeps whichever has the earlier `ClaimedAt` (tie-break:
  lexically smaller `HostID.String()`), same `ClaimingHostID` just updates. Persist via the same
  write-to-tmp-then-rename recipe as `HostRegistry.persistLocked()`
  (`session/host_registry.go:394-417`).
- Files: `session/claim_index.go`.

##### Task 1.1.2c: `CheckClaim` (local-only read) (~3 min)
- `func (c *ClaimIndex) CheckClaim(externalURL string) (ClaimRecord, bool)` — mirrors
  `HostRegistry.Lookup`. `ClaimRecord.Disputed` (Task 1.1.2e) is returned as part of the record
  unchanged — this method doesn't interpret it, just reads whatever's stored.
- Files: `session/claim_index.go`.

##### Task 1.1.2d: Persistence + conflict + forgery unit tests (~5 min)
- Two processes/instances sharing a `t.TempDir()` stateDir (mirroring
  `session/host_registry_test.go`'s cross-instance persistence tests); a same-`ClaimingHostID`
  update case; a different-`ClaimingHostID` conflict case with injectable `Clock` to control
  `ClaimedAt` ordering deterministically (per `deterministic-fast-tests`/
  `fix-flaky-tests-dont-defer`); **a fresh-keypair-forgery case** — a `*HostRegistry` fixture with
  a pinned key for `hostA.ID`, then `RecordClaim` called with a `ClaimRecord{ClaimingHostID:
  hostA.ID}` self-signed by a different, freshly-generated keypair — asserting rejection (per
  Story 1.1.1's second AC), distinct from the existing bit-flip-tampering case.
- Files: `session/claim_index_test.go`.

##### Task 1.1.2e: Add `ClaimRecord.Disputed` + a `ResolveDispute` clear path (~5 min)
- Add `Disputed bool` to `ClaimRecord` (excluded from `signingPayload()` — see Story 1.1.2's new
  AC above for why: it's local-observation metadata, not a signed fact). `RecordClaim` sets it
  `true` on the LWW-selected record whenever it detects a different-`ClaimingHostID` conflict
  (same branch that already logs `claim_index.conflict_detected`). Add
  `func (c *ClaimIndex) ResolveDispute(externalURL string) error` that clears `Disputed` back to
  `false` for that entry and persists — called from the human-facing Override path (Story 4.1.2),
  not auto-cleared by any background process (claims still never expire, per the Tech Debt
  Disposition table above; a dispute is cleared by a human looking at it, or not at all).
- Files: `session/claim_index.go`.

##### Task 1.1.2f: Test the dispute-flag lifecycle (~5 min)
- A different-host conflict sets `Disputed: true` on the resulting `CheckClaim` read;
  `ResolveDispute` clears it and a subsequent `CheckClaim` reflects `false`; a same-host update
  (no conflict) never sets `Disputed` at all.
- Files: `session/claim_index_test.go`.

### Epic 1.2: `ClaimGossiper` transport
**Goal**: Propagate claims between hosts, reusing the existing HTTP/TLS/TOFU machinery without
touching `AdvertisementRecord`'s wire format (ADR-001).

#### Story 1.2.1: `ClaimGossiper` + HTTP endpoint
**As** Host A, **I want** a newly-recorded claim to reach Host B without waiting for Host B to
separately ask, **so that** the common case (claim already propagated) can be a fast local
`CheckClaim` at import time.
**Acceptance Criteria**:
- A claim recorded on Host A is visible in Host B's local `ClaimIndex` after one broadcast round.
  - *Given* Host A and Host B are mutual peers in each other's `HostRegistry`, *When* Host A's
    `ClaimGossiper.BroadcastOnce(ctx)` runs after `RecordClaim` for
    `https://github.com/o/r/issues/1`, *Then* Host B's `ClaimIndex.CheckClaim` for that URL
    returns Host A's record within one HTTP round trip.
- An unsigned or badly-signed claim POST is rejected, not stored.
  - *Given* a POST to `/internal/claim-advertisement` with a `ClaimRecord` whose `Signature`
    doesn't verify, *When* the endpoint handler processes it, *Then* it responds
    `400 Bad Request` and the record is never added to the receiving host's `ClaimIndex`.
**Files**: `session/claim_gossiper.go` (new), `server/auth/claim_advertisement.go` (new).

##### Task 1.2.1a: `ClaimGossiper` type + `BroadcastOnce`/`SendClaim`/`ReGossip` (~5 min)
- Mirror `HostAdvertiser` (`session/host_advertiser.go`) structurally: same `http.Client`
  construction (`Timeout: 5s`, `InsecureSkipVerify: true` with the identical `#nosec G402`
  justification comment). **Signature is record-scoped, not a `HostAdvertiser`-style
  self-contained rebuild**: `BroadcastOnce(ctx context.Context, record ClaimRecord) error` sends
  exactly the one `record` argument to every peer — unlike `HostAdvertiser.BroadcastOnce(ctx)`,
  which takes no argument because it always (re)builds and sends its *own current* single
  advertisement. `SendClaim(ctx, addr string, record ClaimRecord) error` POSTs to `"https://" +
  addr + ClaimAdvertisementEndpointPath`. `ReGossip(ctx, record ClaimRecord) error` is the
  isNew-gated-single-hop fan-out via `registry.Snapshot()` peer addresses (takes a
  `*session.HostRegistry` to know who the peers are — the `ClaimIndex` itself has no notion of
  "who to broadcast to"), **excluding any peer whose `RegistryEntry` has already aged past
  `DefaultHostRegistryTTL`** per `HostRegistry.Prune()` (Story 0.1.1) — a peer `Prune()` would
  remove on its next tick is not worth gossiping to. `Run(ctx)`'s periodic ticker does **not**
  re-broadcast the full claim set each tick (that would reproduce the unbounded-payload-growth
  problem ADR-001 rejected for `AdvertisementRecord`, just moved to this channel) — it only
  exists to backfill claims already recorded locally to peers that have joined
  `HostRegistry.Snapshot()` since their last broadcast; a claim already gossiped to a given peer
  is not resent to that same peer on a later tick.
- Files: `session/claim_gossiper.go`.

##### Task 1.2.1b: `ClaimAdvertisementEndpointPath` const + HTTP handler (~5 min)
- `const ClaimAdvertisementEndpointPath = "/internal/claim-advertisement"` in
  `session/claim_gossiper.go`. Handler in a new `server/auth/claim_advertisement.go`,
  structurally identical to `RegisterHostAdvertisementRoute`
  (`server/auth/host_advertisement.go:29-67`) — including taking a `*session.HostRegistry`
  parameter, since `ClaimIndex.RecordClaim` (Task 1.1.2b) now requires one for its TOFU pin
  check: decode `ClaimRecord`, call `claimIndex.RecordClaim`, on success+conflict-free+new call
  `claimGossiper.ReGossip(ctx, record)`. A record rejected by `RecordClaim` (bad signature or
  failed TOFU pin check) responds `400 Bad Request` and is never gossiped further.
- Files: `session/claim_gossiper.go`, `server/auth/claim_advertisement.go`.

##### Task 1.2.1c: Endpoint + broadcast round-trip test (~5 min)
- `httptest.Server`-based test (reusing `allowImplausibleAddressesForTest`'s existing
  test-only escape hatch, `session/export_test.go`) proving one `BroadcastOnce` call makes a
  claim visible on the receiving side's `ClaimIndex`; a malformed-signature case asserting
  rejection.
- Files: `session/claim_gossiper_test.go` (new), `server/auth/claim_advertisement_test.go` (new).

#### Story 1.2.2: Wire `ClaimGossiper` into `main.go`
**As the** running server, **I want** the claim-gossip loop started alongside the existing host
advertiser, **so that** claims actually propagate in production, not just in tests.
**Acceptance Criteria**:
- Starting the server with a valid `HostIdentity` starts both the host advertiser and the claim
  gossiper.
  - *Given* `main.go`'s existing `hostIdentity`/`hostRegistry`/`advertiser` construction succeeds
    (lines 1541-1558), *When* the remote server starts, *Then* a `*session.ClaimIndex` and
    `*session.ClaimGossiper` are also constructed and `claimGossiper.Run(ctx)` is started via
    `go`, using the same `DefaultHostAdvertisementInterval` cadence.
**Files**: `main.go`.

##### Task 1.2.2a: Construct and start `ClaimIndex`/`ClaimGossiper` (~5 min)
- Immediately after line 1558 (`go advertiser.Run(ctx)`), add: construct
  `session.NewClaimIndex(configDir, hostRegistry)` (the registry argument added by Task 1.1.2a
  for the TOFU pin check), `session.NewClaimGossiper(hostIdentity, hostRegistry, claimIndex,
  selfAddresses, session.DefaultHostAdvertisementInterval)`, register
  `serverauth.RegisterClaimAdvertisementRoute(srv.Mux(), hostRegistry, claimIndex,
  claimGossiper)`, and `go claimGossiper.Run(ctx)`. This task wires the **read/gossip**
  substrate only — the constructed `*session.ClaimIndex` is threaded through to `server/services`
  construction (Phase 2, for `localClaimChecker.CheckClaim`'s reads). Wiring the **write** side
  (`Storage.SetClaimRecorder`) is Story 1.3.1's job, not this task's — see below.
- Files: `main.go`.

### Epic 1.3: Claim recording choke point (ADR-002, without the layering violation)
**Goal**: Actually wire `RecordClaim` into production data, at the single choke point ADR-002
mandates, without `session` importing back into `server/services` — closing the gap both the
architecture and adversarial reviews of this plan's first draft flagged: nothing in that draft
ever called `RecordClaim` from Phases 0-4, and the `crossHostClaimChecker`-based call site ADR-002
originally implied (`Storage.CreateBacklogItem` calling into `server/services`) is a compile-time
import cycle (`server/services` already imports `session`).

#### Story 1.3.1: Wire `RecordClaim` into `Storage.CreateBacklogItem` via `session.ClaimRecorder`
**As the** claim subsystem, **I want** every backlog item creation — regardless of which of the
three call sites created it — to record a claim automatically, **so that** ADR-002's single
choke point is real, not just documented, and the claim index has live data well before #475
lands or the feature flag flips on.

**Revision note (resolves pre-mortem P1-A, with a correction to its premise)**: P1-A's proposed
fix — skip `RecordClaim` for `ItemSource` sync-created rows because "a synced item is not a fresh
claim, it's the same item mirrored from another host" — is based on a mistaken model of what
`ItemSource` sync is. Verified against the code (`session/backlog_sync.go`'s `SyncOne`,
`session/ent/schema/item_source.go`, `session/backlog_plugin.go:76-79`): the only two registered
`ItemSourcePlugin`s are `github_issues` and `github_pr` — both call the real GitHub REST API.
`forward_sync_enabled`/`backward_sync_enabled` govern pulling GitHub issues into *this host's own*
local backlog and pushing status back to GitHub; there is no host-to-host mirroring plugin, and
`BacklogItemData` carries no origin-host field to substitute in. Two hosts each running their own
`ItemSource` against the same shared GitHub repo therefore each independently create a *genuinely
first, from that host's perspective* local row — mechanically identical to two hosts each calling
`ImportGitHubIssue` for the same issue, which `research/features.md` §1b already treated
correctly (listing the sync loop alongside NL-create as a second creation path needing the same
claim-recording, not an exclusion). **`RecordClaim` firing unconditionally on `ItemSource`-sourced
rows is correct and desired — it is exactly the race this feature exists to detect.** No skip
logic is added here. The real, separate gap P1-A's authors were reaching for — that the sync loop
has no *pre-creation check* before it creates that row, unlike `ImportGitHubIssue` — is genuine
and is fixed by new Story 2.2.4 below (also closes cross-artifact-consistency BLOCKER CB-3).
**Acceptance Criteria**:
- A `BacklogItemData` row created with a non-empty `ExternalURL`, through any of the three
  existing creation paths (`ImportGitHubIssue`, the `ItemSource` sync loop, NL/manual creation),
  results in a `ClaimRecord` for that URL appearing in the local `*session.ClaimIndex`, using the
  local process's own `session.HostIdentity.ID` as `ClaimingHostID` — no #475 dependency.
  - *Given* `Storage` has been constructed with `SetClaimRecorder` called (as Task 1.3.1b wires
    from `server/dependencies.go`), *When* `Storage.CreateBacklogItem` succeeds for
    `data.ExternalURL = "https://github.com/o/r/issues/1"`, *Then* `ClaimIndex.CheckClaim` for
    that URL returns a record with `ClaimingHostID` equal to the local `HostIdentity.ID`.
- A `RecordClaim` failure never fails the underlying item creation (ADR-002's Consequences
  section).
  - *Given* the injected `ClaimRecorder` returns an error (e.g. a lock-file timeout), *When*
    `Storage.CreateBacklogItem` is called, *Then* the backlog item is still created successfully
    and `claim_index.record_failed` is logged, with no error surfaced to the caller.
- With no `ClaimRecorder` set (nil, e.g. in existing unit tests that construct `Storage` directly
  without the full server wiring), `CreateBacklogItem` behaves exactly as it does today — no
  panic, no behavior change.
**Files**: `session/claim_recorder.go` (new), `session/storage.go`, `server/dependencies.go`.

##### Task 1.3.1a: Define the `session.ClaimRecorder` port + `Storage.SetClaimRecorder` (~5 min)
- `type ClaimRecorder interface { RecordClaim(ctx context.Context, record ClaimRecord) error }`
  in `session/claim_recorder.go`, mirroring `ItemChangePublisher`
  (`session/backlog_item_change.go:84`) exactly. Add `Storage.SetClaimRecorder(r ClaimRecorder)`
  to `session/storage.go`, mirroring `Storage.SetCallbackDispatcher`
  (`session/storage.go:289-292`) — forwards to the underlying `*EntRepository`, or is held
  directly on `Storage` if simpler (confirm which pattern `SetItemChangePublisher` vs.
  `SetCallbackDispatcher` uses at the exact call site before choosing). A small concrete
  implementation composing `*ClaimIndex` (write) and `*ClaimGossiper` (best-effort broadcast of
  the newly-recorded record) also lives in `session` — no `server/services` type is needed here,
  since both `ClaimIndex` and `ClaimGossiper` are already `session`-package types (Stories
  1.1.2, 1.2.1).
- Files: `session/claim_recorder.go`, `session/storage.go`.

##### Task 1.3.1b: Call `RecordClaim` from `Storage.CreateBacklogItem`, using local `HostIdentity.ID` (~5 min)
- In `Storage.CreateBacklogItem` (`session/storage.go:906`), after the row is durably created and
  `data.ExternalURL != ""` (the same gate `GetBacklogItemByExternalURL` uses — ADR-002),
  best-effort call `s.claimRecorder.RecordClaim(ctx, ClaimRecord{ExternalURL: data.ExternalURL,
  ClaimingHostID: <local HostIdentity.ID>, ItemDeepLink: <deep link for the new item>, ClaimedAt:
  now})`, signed with the local `HostIdentity`. Errors are logged
  (`claim_index.record_failed`), never returned. No-op if `s.claimRecorder` is nil.
- Files: `session/storage.go`.

##### Task 1.3.1c: Wire `SetClaimRecorder` from `server/dependencies.go` (~3 min)
- Alongside the existing `SetItemChangePublisher`/`SetCallbackDispatcher` calls
  (`server/dependencies.go:836,1076`), call `storage.SetClaimRecorder(...)` with the
  `ClaimIndex`/`ClaimGossiper`-backed implementation constructed in `main.go` (Task 1.2.2a).
- Files: `server/dependencies.go`.

##### Task 1.3.1d: Tests for the choke point + best-effort-failure behavior (~5 min)
- One test per creation path (or a shared helper exercised from each) asserting a claim appears
  in `ClaimIndex` after `CreateBacklogItem`; a fake `ClaimRecorder` returning an error asserting
  creation still succeeds; a nil-`ClaimRecorder` case asserting no panic/behavior change.
- Files: `session/storage_test.go`.

---

## Phase 2: Claim check integration (dedup enforcement)
### Epic 2.1: `crossHostClaimChecker` seam
**Goal**: A narrow, testable interface between the two call sites and the claim substrate, per
the Tech Debt Disposition's "Isolate via seam."

#### Story 2.1.1: Define `crossHostClaimChecker` + implementations
**As** `ImportGitHubIssue`/`DequeueNextQueuedItems`, **I want** one small, **read-only**
interface to consult, **so that** I don't need to know about `ClaimIndex`/`ClaimGossiper`/gossip
internals directly. (The write side — `RecordClaim` — is deliberately **not** on this interface;
see Story 1.3.1's `session.ClaimRecorder` port, which is what `Storage.CreateBacklogItem`
actually calls. Nothing in this plan calls `crossHostClaimChecker.RecordClaim`, so it would be
dead surface if kept here — see architecture-review Blocker 1's remediation.)
**Acceptance Criteria**:
- With no checker wired (nil), every check reports `Unclaimed` — single-host behavior is
  unchanged (AC3).
  - *Given* `BacklogService` constructed with no `crossHostClaimChecker` (or the feature flag
    off), *When* `ImportGitHubIssue` calls `CheckClaim`, *Then* it returns
    `ClaimVerdict{Kind: Unclaimed}` and import proceeds exactly as it does today.
**Files**: `server/services/claim_checker.go` (new).

##### Task 2.1.1a: `ClaimVerdict` sum type + `crossHostClaimChecker` interface (~5 min)
- `type ClaimVerdictKind int; const (ClaimUnclaimed ClaimVerdictKind = iota; ClaimHeldByOther;
  ClaimCheckIndeterminate)`; `type ClaimVerdict struct { Kind ClaimVerdictKind; Record
  session.ClaimRecord }`. Interface, **read-only**: `CheckClaim(ctx, externalURL string)
  (ClaimVerdict, error)` — no `RecordClaim` method (see Story note above). Add smart
  constructors so `Kind`/`Record` can't be mismatched at a call site (architecture-review
  concern): `func NewUnclaimedVerdict() ClaimVerdict`, `func NewHeldByOtherVerdict(record
  session.ClaimRecord) ClaimVerdict`, `func NewIndeterminateVerdict() ClaimVerdict` — every
  construction site in this plan (Task 2.1.2b's peer-fanout loop, Task 2.2.3a's MCP-result
  mapping, tests) uses these instead of a bare struct literal.
- Files: `server/services/claim_checker.go`.

##### Task 2.1.1b: `unimplementedClaimChecker` (~3 min)
- `CheckClaim` always returns `NewUnclaimedVerdict()`, mirroring `unimplementedHostResolver`
  (`server/services/deep_link_resolver.go:45-49`). No `RecordClaim` to no-op — the interface
  doesn't have one.
- Files: `server/services/claim_checker.go`.

##### Task 2.1.1c: `localClaimChecker` backed by `*session.ClaimIndex` (~5 min)
- `CheckClaim` reads the local index first (`ClaimIndex.CheckClaim`), returning
  `NewHeldByOtherVerdict`/`NewUnclaimedVerdict` accordingly (Story 2.1.2 adds the live-peer
  fan-out fallback). This type has **no write method** — recording happens exclusively via
  `session.ClaimRecorder`/`Storage.SetClaimRecorder` (Story 1.3.1), not through
  `localClaimChecker`.
- Files: `server/services/claim_checker.go`.

##### Task 2.1.1d: Unit tests with a fake `crossHostClaimChecker` (~5 min)
- Table test for nil/unimplemented-checker pass-through behavior (asserting
  `NewUnclaimedVerdict()`); a `localClaimChecker.CheckClaim` test against a real
  `*session.ClaimIndex` in a temp dir (populated directly via `ClaimIndex.RecordClaim` in the
  test, not through `localClaimChecker`, since it has no write method).
- Files: `server/services/claim_checker_test.go` (new).

#### Story 2.1.2: On-demand synchronous peer query (primary check path)
**As** Host B about to import an issue, **I want** to ask live peers directly rather than wait
for the next gossip cycle, **so that** the race window is bounded by network latency, not by
`DefaultHostAdvertisementInterval` (5 min).
**Acceptance Criteria**:
- A claim recorded on Host A one second ago (before its own gossip broadcast fires) is still
  found by Host B's synchronous check, if Host A is reachable.
  - *Given* Host A just called `RecordClaim` locally but `BroadcastOnce` hasn't run yet, *When*
    Host B's `localClaimChecker.CheckClaim` queries Host A directly via a bounded-timeout
    GET (mirroring `registryHostResolver.checkLiveness`'s pattern,
    `server/services/deep_link_resolver.go:139-155`), *Then* it returns
    `ClaimHeldByOther(Host A's record)`.
- An unreachable peer during the check reports `CheckIndeterminate`, never `Unclaimed`.
  - *Given* a known peer's advertised address times out, *When* `CheckClaim` queries it,
    *Then* the overall verdict is `ClaimCheckIndeterminate` if no other peer reported a claim
    and at least one peer was unreachable — never silently treated as `Unclaimed`.
- **The fan-out is parallel, with one shared bounded deadline for the whole check — never
  sequential.** (architecture-review concern: a sequential per-peer loop risks N×timeout
  worst-case latency, directly at odds with `research/ux.md`'s "must not spinner-gate a fast
  interaction.")
  - *Given* 3 known peers, of which 2 are unreachable (each would time out at the same
    per-peer timeout used by `deep_link_resolver.go`'s `checkLiveness`, 2s), *When*
    `CheckClaim` runs, *Then* the overall call returns in ~2s (one timeout's worth), not ~6s —
    i.e. all peer queries are issued concurrently (goroutines) against one shared
    `context.WithTimeout` for the whole fan-out, not a per-peer sequential timeout each.
- **A peer excluded by `HostRegistry.Prune()` is excluded from the fan-out set.** (ties Story
  0.1.1's `Prune()` fix to this fan-out, per the adversarial-review concern that sequencing
  alone doesn't guarantee the connection is exercised.)
  - *Given* a peer's `RegistryEntry` has aged past `DefaultHostRegistryTTL` and has been pruned
    from `HostRegistry`'s persisted state (Story 0.1.1), *When* `CheckClaim` builds its fan-out
    set from `HostRegistry.Snapshot()`, *Then* that peer is not queried — it is structurally
    absent from `Snapshot()`'s result, so no explicit skip logic is needed here beyond calling
    `Snapshot()` fresh each time (not a stale cached peer list).
**Files**: `server/services/claim_checker.go`.

##### Task 2.1.2a: Add a `GET /internal/claim-lookup` peer-query endpoint (~5 min)
- New handler alongside the claim-advertisement POST endpoint (Story 1.2.1b): given
  `?url=<externalURL>`, returns the local `ClaimIndex`'s entry (200 + JSON) or 404 if none.
- Files: `server/auth/claim_advertisement.go`.

##### Task 2.1.2b: `localClaimChecker.CheckClaim` queries live peers in parallel before falling back to local (~5 min)
- Query the local index first (fast path if already gossiped); if unclaimed locally, fan out
  bounded-timeout GETs **concurrently** (one goroutine per peer, reusing `peerTLSTransport()`
  from `deep_link_resolver.go:105-107`) to every `HostRegistry.Snapshot()` peer, all sharing one
  `context.WithTimeout` for the entire fan-out (not a per-peer sequential loop — see AC above);
  first `ClaimHeldByOther` response wins (cancel the rest via the shared context); if any queried
  peer timed out and none reported a claim, return `NewIndeterminateVerdict()`. Since
  `HostRegistry.Snapshot()` already excludes pruned entries (Story 0.1.1), no separate
  TTL-filtering logic is needed in this function.
- Files: `server/services/claim_checker.go`.

##### Task 2.1.2c: Tests for the three-way indeterminate/unclaimed/held-by-other split + parallelism (~5 min)
- `httptest.Server` fixtures for a reachable-and-claiming peer, a reachable-and-unclaimed peer,
  and a timing-out peer — assert the aggregate verdict matches the AC above. Add a
  multi-unreachable-peer case asserting total elapsed time stays near one timeout's worth (not
  N timeouts), proving the fan-out is actually concurrent and not accidentally sequential.
- Files: `server/services/claim_checker_test.go`.

### Epic 2.2: Wire the checker into call sites
**Goal**: Close every claim/check point research and Goal 4 identify — not just the one
requirements.md originally named: `ImportGitHubIssue` (2.2.1), `DequeueNextQueuedItems` (2.2.2),
the `check_cross_host_claim` MCP tool (2.2.3), and — per cross-artifact-consistency BLOCKER
CB-3 — NL-create and the `ItemSource` sync loop (2.2.4), so Goal 4's "any backlog item source"
holds for the pre-creation *check*, not only for Story 1.3.1's always-on claim *recording*.

#### Story 2.2.1: `ImportGitHubIssue` consults the claim checker
**As an** operator importing a GitHub issue on Host B, **I want** to be told Host A already
claimed it, and to have a way to proceed anyway if that turns out to be a false positive,
**so that** I don't spawn a second competing session, but also don't get permanently locked out
by a stale/incorrect claim.

**Revision note (resolves adversarial-review Blocker 1)**: the original AC made
`ClaimHeldByOther` an unconditional RPC-level block that created no item at all — with no
override path, since Phase 4's `ClaimConflictBanner`/Override action (Story 4.1.2) only attaches
to an item that already exists in this host's own storage. A false-positive claim (e.g. from a
crashed host, worsened by `HostRegistry.Prune()` having been dead code until Story 0.1.1 — see
`research/pitfalls.md` §2) would have been a **permanent** block with zero in-product recourse
short of disabling the feature flag for every item. `research/ux.md` §2 is explicit that a hard
block is the wrong default here. This revision keeps `design/ux.md`'s already-designed Surface 3
("already claimed" as its own third outcome bucket in the import picker, distinct from both the
existing same-host `alreadyExisted: true` bucket and the generic "Import failed" bucket, with a
"Copy link to \<host\>'s item" action) and *adds* an override affordance to that same bucket,
superseding Surface 3's now-stale "No Override is offered at this surface... deliberate design
constraint" note, which was written against the old hard-block AC.
**Acceptance Criteria**:
- A claim-index hit does **not** silently block — it returns a distinct, structured
  "already-claimed-elsewhere" outcome (a third bucket alongside the existing same-host
  `alreadyExisted: true` and generic-failure outcomes — `design/ux.md` Surface 3), carrying the
  owning host's ID and deep link, and **no new `BacklogItemData` row is created by default.**
  - *Given* `cross_host_claim_dedup` is enabled and Host A's `ClaimRecord` for
    `https://github.com/o/r/issues/42` is visible to Host B (locally or via on-demand query),
    *When* an operator calls `ImportGitHubIssue` with that same issue URL on Host B without
    `override: true`, *Then* the RPC returns a structured response (not a bare
    `connect.CodeAlreadyExists` error collapsed into the generic-failure bucket) identifying the
    outcome as `already_claimed_elsewhere`, naming `claiming_host_id` and `item_deep_link`
    (`ssq://hostA/backlog/v1/bl_...`), and no new row is created.
- **An explicit override creates the item anyway**, reusing `GateVerdictBox`'s Override verb
  (Pattern Decisions table) rather than inventing a new mechanism, **and captures a reason for
  audit parity with Surface 1's Override** (resolves cross-artifact-consistency CONCERN CC-2:
  Surface 1's Override already requires a ≥5-character reason, audit-logged; Surface 3's original
  "Import anyway" had no reason-capture at all, an inconsistency for what's equally a
  security/audit-relevant action in both places).
  - *Given* the same fixture, *When* an operator resubmits `ImportGitHubIssue` for the same issue
    URL with `override: true` and a `reason` string (≥5 characters, same
    `MIN_OVERRIDE_REASON_LENGTH` convention as `GateVerdictBox`) set, *Then* the item **is**
    created normally (as if unclaimed), and `import_github_issue.claim_override` is logged
    (external_url, claiming_host_id, reason) for auditability — mirroring the audit-note
    convention `GateVerdictBox.tsx`'s `onOverride` handler already uses elsewhere in this codebase
    (confirm its exact logging call before wiring Task 4.1.2b, since the web-app override button
    calls through to this same RPC path with `override: true`). A request with `override: true`
    but a missing/too-short `reason` is rejected the same way `GateVerdictBox`'s submit button is
    disabled below the minimum length — the RPC itself validates this server-side, not just the
    UI.
- A `CheckIndeterminate` verdict does not block — it proceeds optimistically but logs (per
  `research/ux.md`'s "proceed optimistically, but say so").
  - *Given* the claim check times out against every known peer, *When* `ImportGitHubIssue`
    proceeds to create the item, *Then* it logs `import_github_issue.claim_check_indeterminate`
    and creation succeeds normally.
**Files**: `server/services/backlog_service_sync.go`, `proto/session/v1/session.proto` (add
`override` **and `override_reason`** to the import request, and an `already_claimed_elsewhere`
outcome shape to the response — confirm exact message name before editing, then `make
proto-gen`).

##### Task 2.2.1a: Add the claim-check branch before `CreateBacklogItem`, with override support (~5 min)
- In `ImportGitHubIssue` (`server/services/backlog_service_sync.go:278-296`), after the existing
  local `GetBacklogItemByExternalURL` miss, add: `if s.cfg.GetFeatureFlag("cross_host_claim_dedup")
  && !req.Override { verdict, err := s.claimChecker.CheckClaim(ctx, issue.URL); ... if
  verdict.Kind == ClaimHeldByOther { return an already_claimed_elsewhere response (not a bare
  connect.CodeAlreadyExists error) naming verdict.Record.ClaimingHostID and
  verdict.Record.ItemDeepLink, and skip CreateBacklogItem } }` — when `req.Override` is true,
  validate `len(strings.TrimSpace(req.OverrideReason)) >= 5` (reject with `InvalidArgument` if
  not, per CC-2's server-side validation requirement), then skip the check entirely and create
  normally, regardless of claim state, logging `req.OverrideReason` in
  `import_github_issue.claim_override`.
- Files: `server/services/backlog_service_sync.go`.

##### Task 2.2.1b: Wire `claimChecker` into `BacklogService`'s constructor (~3 min)
- Add a `claimChecker crossHostClaimChecker` field, default to `unimplementedClaimChecker{}`
  when not supplied (mirrors `NewDeepLinkResolver`'s nil-fallback pattern).
- Files: `server/services/backlog_service.go` (or wherever `BacklogService`'s struct/constructor
  lives — confirm exact filename before editing).

##### Task 2.2.1c: Test the block, override, and indeterminate-proceeds paths (~5 min)
- Four tests: fake checker returning `ClaimHeldByOther` with `req.Override = false` asserts no
  row created + `already_claimed_elsewhere` response shape (not a generic error); the same
  fixture with `req.Override = true` and a valid `req.OverrideReason` asserts the row IS created
  and `claim_override` is logged with the reason; the same fixture with `req.Override = true` but
  `req.OverrideReason` under 5 characters asserts an `InvalidArgument` error and no row created
  (CC-2's server-side validation); fake checker returning `ClaimCheckIndeterminate` asserts row
  IS created + log line present.
- Files: `server/services/backlog_service_sync_test.go`.

##### Task 2.2.1d: Wire the `already_claimed_elsewhere`/override outcome into the import picker UI (~5 min)
- Resolves the NITPICK cross-artifact-consistency finding: `design/ux.md` Surface 3 fully designs
  this (batch-import result banner, per-issue "already claimed by \<host\>" line with a
  Copy-link action, and an "Import anyway" override with reason capture per CC-2 above), but no
  task in this story previously touched the frontend files it names. In `useBacklogService.ts`
  (`importGitHubIssue`'s result handling, `useBacklogService.ts:1303-1306`), stop collapsing the
  `already_claimed_elsewhere` outcome into `getErrorMessage`'s generic failure bucket — carry
  `claiming_host_id`/`item_deep_link` through as structured data. In
  `app/backlog/page.tsx`'s `handlePickerSelect` (`page.tsx:545-601,570-580`), add the third result
  bucket alongside the existing new-item/duplicate buckets, rendering per issue: host name, Copy
  Link button, and an "Import anyway" button that opens a short reason field (≥5 chars, mirroring
  `GateVerdictBox`'s form) and resubmits with `override: true, override_reason: <reason>` on
  confirm. Update `GitHubIssuePicker.tsx` only if the picker component itself (not the parent
  page) owns the result-list rendering — confirm which owns it before editing.
- Files: `web-app/src/lib/hooks/useBacklogService.ts`, `web-app/src/app/backlog/page.tsx`,
  `web-app/src/components/backlog/GitHubIssuePicker.tsx` (if applicable).

##### Task 2.2.1e: Test the import-picker UI's three outcome buckets (~5 min)
- Assert the cross-host-claimed line renders host name + Copy Link + Import-anyway with a reason
  form; assert Import-anyway is disabled below 5 characters; assert it stays visually distinct
  from the existing same-host "already imported" line.
- Files: `web-app/src/app/backlog/page.test.tsx` (or wherever this flow's existing
  duplicates/failures rendering is already tested — confirm before adding a new file).

#### Story 2.2.2: `DequeueNextQueuedItems` consults the claim checker (local-only, no live fan-out)
**As the** system dequeuing queued/ready items for work, **I want** to catch the case where two
hosts each independently imported the same issue before gossip caught up, **so that** the
second-invisible claim point research identified doesn't spawn a duplicate session anyway —
**without** adding network round trips inside a lock that serializes the whole method.

**Revision note (resolves adversarial-review concern)**: `DequeueNextQueuedItems` runs its
candidate loop under `s.dequeueMu.Lock()`, whose own doc comment
(`server/services/backlog_service_triage.go:719`) states this "serializes the whole method." If
this story's claim check reused Story 2.1.2's live-peer fan-out (Task 2.1.2b), N candidates × M
peers × per-peer timeout could stall every other dequeue operation on the host behind that lock —
a more severe version of the bulk-import latency concern already flagged in Unresolved Questions
for Story 2.2.3, but for a periodic *background* sweep instead of a user-initiated action. This
story therefore consults **only the local `ClaimIndex`** (gossip-propagated data already on
disk), never triggering a live peer query; the synchronous fan-out stays reserved for the
interactively-triggered paths (`ImportGitHubIssue`, `check_cross_host_claim`).
**Acceptance Criteria**:
- A candidate whose `ExternalURL` is now claimed by a different host (claim arrived after this
  host's own import) is skipped, not claimed for work.
  - *Given* Host B independently imported `https://github.com/o/r/issues/42` as its own backlog
    item (before either host's gossip converged), and a `ClaimRecord` for that URL naming Host A
    has since arrived in Host B's `ClaimIndex` (via gossip, not a live query), *When*
    `DequeueNextQueuedItems` considers that item as a candidate, *Then* it is skipped (left at
    its current status) and `dequeue.blocked_by_claim` is logged, rather than being claimed via
    `transitionWithGuard`.
- **The check never issues a live peer query.** A claim that exists on a peer but hasn't yet
  gossiped to this host is invisible to this sweep — that's an accepted false-negative, not a
  bug, per the goal above.
  - *Given* a claim for a candidate's `ExternalURL` exists only on a remote peer and has not yet
    propagated via gossip, *When* `DequeueNextQueuedItems` checks that candidate, *Then* it
    consults only the local `ClaimIndex.CheckClaim` (no HTTP call to any peer) and proceeds to
    dequeue normally if the local index has no entry — never blocking on network I/O while
    `s.dequeueMu` is held.
- **A dequeue-blocked item is surfaced to a human with a way to override it — never a silent,
  permanent skip.** (Resolves pre-mortem P1-B: claims never expire by design, and Override was
  previously wired only into `ImportGitHubIssue` (Story 2.2.1), so a stale claim from an
  abandoned/crashed host could starve a legitimately-owned queued item forever with zero
  in-product recourse.) This reuses the existing stuck-item pipeline verbatim — the same one
  `notifyBlockedByDependency` already uses for the structurally identical "dependency gate skipped
  this candidate" case (`server/services/backlog_service_triage.go:277-307`,
  `domain.StuckReasonBlockedByDependency`) — rather than inventing a new list surface.
  - *Given* `DequeueNextQueuedItems` skips a candidate because of a local claim block, *When* the
    skip occurs, *Then* it calls `s.storage.MarkStuck(ctx, itemID,
    domain.StuckReasonBlockedByClaim, currentStatus, message)` and `MarkStuckNotified` (mirroring
    `notifyBlockedByDependency`'s exact call shape), so the item appears in `ListStuckBacklogItems`
    → `/unfinished`'s `StuckItemsSection` → `BlockerChip`, the same path an operator already checks
    for every other stuck-reason today.
  - *Given* an operator sees a `BlockerChip` reading the claim-blocked reason for an item they know
    should be theirs, *When* they invoke its override action, *Then* they're prompted for a reason
    (≥5 characters, mirroring `GateVerdictBox`'s `MIN_OVERRIDE_REASON_LENGTH`/`onOverride(reason)`
    shape, per CC-2's audit-parity decision below), and on submit the item is dequeued and claimed
    for work on this host despite the stale claim, with `dequeue.claim_override` logged
    (item_id, external_url, claiming_host_id, reason) — distinct from Story 2.2.1's
    `import_github_issue.claim_override`, since this overrides a *dequeue* block on an item this
    host already owns locally, not an *import* block on an item it doesn't yet have.
**Files**: `server/services/backlog_service_triage.go`, `proto/session/v1/backlog.proto` (new
`STUCK_REASON_BLOCKED_BY_CLAIM` enum value), `session/domain/backlog.go` (new
`StuckReasonBlockedByClaim` const), `web-app/src/lib/hooks/stuckReason.ts` (icon/label for the new
reason), `web-app/src/components/backlog/BlockerChip.tsx` (override affordance).

##### Task 2.2.2a: Add a local-only claim check inside the candidate loop, before `transitionWithGuard` (~5 min)
- In `DequeueNextQueuedItems` (`server/services/backlog_service_triage.go:781-816`), for each
  `item` with a non-empty `ExternalURL`, call a **local-only** check — either a distinct method
  on `crossHostClaimChecker` (e.g. `CheckClaimLocalOnly`, added alongside `CheckClaim` in Task
  2.1.1a) or `localClaimChecker` reading `*session.ClaimIndex.CheckClaim` directly, bypassing
  Story 2.1.2's peer fan-out entirely; on `ClaimHeldByOther` with a *different* `ClaimingHostID`
  than this host's own, log and `continue` before calling `transitionWithGuard`. Skip the check
  entirely for `item.ExternalURL == ""` (nothing to check).
- Files: `server/services/backlog_service_triage.go`, `server/services/claim_checker.go` (if
  adding `CheckClaimLocalOnly`).

##### Task 2.2.2b: Test the skip-vs-claim-normally paths (~5 min)
- Fake checker returning `ClaimHeldByOther` for a different host id → item stays at its
  pre-dequeue status, not claimed; same-host-id claim (this host's own prior claim) →
  dequeues normally.
- Files: `server/services/backlog_service_triage_test.go`.

##### Task 2.2.2c: Add `StuckReasonBlockedByClaim` and mark the item stuck on skip (~5 min)
- Add `STUCK_REASON_BLOCKED_BY_CLAIM` to the `StuckReason` enum
  (`proto/session/v1/backlog.proto`, alongside the existing 19 values at line ~1888) and its Go
  const `domain.StuckReasonBlockedByClaim` (`session/domain/backlog.go`, alongside
  `StuckReasonBlockedByDependency`). In the claim-skip branch added by Task 2.2.2a, call
  `s.storage.MarkStuck(ctx, item.ID, domain.StuckReasonBlockedByClaim, item.Status, message)` then
  `s.storage.MarkStuckNotified(ctx, item.ID, domain.StuckReasonBlockedByClaim)`, mirroring
  `notifyBlockedByDependency`'s exact shape (`backlog_service_triage.go:277-307`). Add the
  icon/label mapping for the new reason to `web-app/src/lib/hooks/stuckReason.ts` so `BlockerChip`
  renders it (e.g. "Blocked: claimed by \<host\>").
- Files: `proto/session/v1/backlog.proto`, `session/domain/backlog.go`,
  `server/services/backlog_service_triage.go`, `web-app/src/lib/hooks/stuckReason.ts`.

##### Task 2.2.2d: Add the override action to `BlockerChip` for this reason (~5 min)
- New backend RPC/method (e.g. `OverrideClaimBlock(ctx, itemID, reason)`, mirroring Task 2.2.1a's
  `override: true` shape but scoped to a dequeue block on an item this host already owns) that
  re-runs `transitionWithGuard` for that item, bypassing only the claim check, and logs
  `dequeue.claim_override` with the reason. Wire it into `BlockerChip.tsx`'s existing "full"
  variant interactive-button path (`BlockerChip.tsx:110-148`, currently used for
  `onTriggerRemediationNow`) — for `StuckReasonBlockedByClaim` specifically, the button opens a
  short reason form (≥5 chars) reusing `GateVerdictBox`'s override-form shape
  (`GateVerdictBox.tsx:421-462`) rather than firing immediately like "Retry now" does for other
  reasons, since this is the security/audit-relevant action CC-2 requires reason-capture for.
- Files: `server/services/backlog_service_triage.go` (or wherever the RPC is implemented — confirm
  exact service/file before wiring), `web-app/src/components/backlog/BlockerChip.tsx`.

##### Task 2.2.2e: Test the stuck-marking and override paths (~5 min)
- Assert a claim-skip calls `MarkStuck`/`MarkStuckNotified` with the right reason; assert the
  override RPC dequeues the item despite an active claim block and logs the reason.
- Files: `server/services/backlog_service_triage_test.go`,
  `web-app/src/components/backlog/BlockerChip.test.tsx`.

#### Story 2.2.3: `check_cross_host_claim` MCP tool
**As an** operator or agent about to work on an item, **I want** to explicitly ask "has anyone
else claimed this?", **so that** I can check before starting, not only get told after the fact.
**Acceptance Criteria**:
- Given an external URL, the tool returns the same three-way verdict `ImportGitHubIssue` uses.
  - *Given* the same fixture as Story 2.2.1's block case, *When* an agent calls
    `check_cross_host_claim(external_url: "https://github.com/o/r/issues/42")`, *Then* the tool
    result includes `claimed: true`, `claiming_host_id: "host_..."`, and
    `item_deep_link: "ssq://hostA/backlog/v1/bl_..."`.
**Files**: `server/mcp/tools_claim.go` (new).

##### Task 2.2.3a: Register the MCP tool + handler (~5 min)
- Mirror `list_workspace_peers`'s registration shape (find its registration call in
  `server/mcp/`); handler calls `s.claimChecker.CheckClaim` and maps `ClaimVerdict` to the tool
  result JSON shape above (including the indeterminate case as `claimed: false, checked: false`
  so a caller can distinguish "confirmed clear" from "couldn't confirm").
- Files: `server/mcp/tools_claim.go`.

##### Task 2.2.3b: Test all three verdict mappings (~5 min)
- Files: `server/mcp/tools_claim_test.go` (new).

#### Story 2.2.4: NL-create and the `ItemSource` sync loop consult the claim checker
**As an** operator creating an item via chat, or as the background sync loop materializing a
GitHub issue into the backlog, **I want** the same pre-creation claim check `ImportGitHubIssue`
already gets, **so that** Goal 4's "works for any backlog item source" is actually true for the
*check*, not just the *record* side (Story 1.3.1 already records a claim for every source; this
story closes the matching check-side gap cross-artifact-consistency BLOCKER CB-3 identified).

**Two different check strategies, matching the two callers' concurrency shapes** — reusing the
same interactive-vs-background distinction Story 2.2.2's revision note already established for
`DequeueNextQueuedItems`:
- NL-create (`CreateBacklogItemFromChat`,
  `server/services/backlog_service_chat.go:43`→`CreateBacklogItem`,
  `server/services/backlog_service_lifecycle.go:161`) is a single, interactively-triggered
  creation — it uses the **same live-peer-fan-out** `crossHostClaimChecker.CheckClaim` and
  `already_claimed_elsewhere`/`override` outcome shape as Story 2.2.1, not a new mechanism.
- The `ItemSource` sync loop (`SyncOne`, `session/backlog_sync.go:294`, `CreateBacklogItem` call
  at line 346) processes a whole batch of fetched external items per run with no per-item
  interactive wait budget — it uses the **local-only** check (Story 2.2.2's
  `CheckClaimLocalOnly`/`localClaimChecker` local read, no HTTP fan-out), for the same reason
  `DequeueNextQueuedItems` does: N candidates × live peer queries would stall the sync loop's own
  run, and this is a background sweep an operator isn't watching a spinner for.
**Acceptance Criteria**:
- A chat-created item whose external URL is already `ClaimHeldByOther` returns the same
  structured `already_claimed_elsewhere` outcome (claiming host, deep link) Story 2.2.1 defines,
  and creates no row unless `override: true` is set.
  - *Given* the same claim fixture Story 2.2.1's tests use, *When* `CreateBacklogItemFromChat` is
    called for that same external URL without an override, *Then* no row is created and the
    caller receives `claiming_host_id`/`item_deep_link`; with `override: true` the item is created
    and `nl_create.claim_override` is logged.
- A sync-loop-fetched item whose external URL is already known locally (via gossip) as
  `ClaimHeldByOther` by a different host is skipped for this run, not created, and logged —
  without issuing any peer query.
  - *Given* `SyncOne` fetches an external item whose URL is already in this host's local
    `ClaimIndex` as claimed by a different `ClaimingHostID`, *When* `SyncOne` reaches the
    `CreateBacklogItem` call (line 346), *Then* it skips creation, logs
    `sync.blocked_by_claim` (external_id, claiming_host_id), and does not call
    `CreateBacklogItem` for that item — leaving it to be picked up (and its own claim recorded)
    correctly on a future run once the conflict resolves, or surfaced for a human to investigate
    via the sync loop's existing error/skip reporting.
- A `CheckIndeterminate` verdict does not block either path — same "proceed optimistically, log
  it" rule as Story 2.2.1/2.2.2.
**Files**: `server/services/backlog_service_chat.go`, `server/services/backlog_service_lifecycle.go`,
`session/backlog_sync.go`.

##### Task 2.2.4a: Wire the live-fan-out claim check + override into NL-create (~5 min)
- In `CreateBacklogItem` (`server/services/backlog_service_lifecycle.go:161`, called from
  `CreateBacklogItemFromChat`), before the `s.storage.CreateBacklogItem` call at line 218, add the
  same `s.claimChecker.CheckClaim`/`already_claimed_elsewhere`/`override` branch Task 2.2.1a adds
  to `ImportGitHubIssue` — only when `data.ExternalURL != ""` (most NL-created items have none).
- Files: `server/services/backlog_service_lifecycle.go`.

##### Task 2.2.4b: Wire a local-only claim check into `SyncOne`, before `CreateBacklogItem` (~5 min)
- In `SyncOne` (`session/backlog_sync.go:294`), immediately before the `CreateBacklogItem` call at
  line 346, for each fetched item with a non-empty external URL, consult the local-only check
  (`CheckClaimLocalOnly` / direct `*session.ClaimIndex.CheckClaim`, per Task 2.2.2a's local-only
  path) and skip creation + log `sync.blocked_by_claim` on a different-host hit, matching Story
  2.2.2's "never a live peer query from a background loop" rule.
- Files: `session/backlog_sync.go`.

##### Task 2.2.4c: Tests for both paths (~5 min)
- NL-create: block/override/indeterminate-proceeds cases, mirroring Task 2.2.1c. Sync loop:
  local-claim-hit skips creation and logs, no-claim proceeds normally, no HTTP call is ever made
  from the sync loop's test (assert via a checker fake that panics/fails the test if its live-query
  method is invoked).
- Files: `server/services/backlog_service_lifecycle_test.go`, `session/backlog_sync_test.go`.

---

## Phase 3: PR provenance stamping
### Epic 3.1: Trace a PR back to its originating host/item (Goal 3)
**Goal**: A PR is traceable to its origin regardless of which host's webhook handler processes
a later event on it — independent of the claim index or the originating host being reachable.

#### Story 3.1.1: Add a norawghrequest-compliant PR-comment write function
**As the** provenance-stamping code, **I want** a GitHub write call that goes through the
approved request constructors, **so that** it doesn't bypass the ETag/rate-limit plumbing the
`norawghrequest` lint rule protects (even though this is a POST, not a cached GET, the rule
requires all native GitHub calls route through the approved builders).

**Revision note (resolves architecture-review Blocker 2)**: `newGHRequestForHostWithToken`
(`github/http_client.go:274-287`) is hardcoded to build a **GET** request with a nil body — its
own doc comment says so, and method/body aren't parameters. It cannot build the `POST
.../comments` request with a JSON body this story needs, and none of the other
`norawghrequest`-approved constructors fit either (`NewConditionalRequest`/
`NewConditionalRequestNoCache` are ETag-cache-specific GETs; `newGHGraphQLRequestForHostWithToken`
is POST but hardcoded to the GraphQL endpoint/content-type). Task 3.1.1a below adds the missing
sibling constructor first, so the PR-comment task has something correct to call instead of
"fixing" this by mutating a GET request's method/body after construction — exactly the silent
workaround `.claude/rules/norawghrequest.md` exists to prevent.
**Acceptance Criteria**:
- The new function builds its request via `newGHRequestForHostWithTokenAndBody` (added by Task
  3.1.1a), not `http.NewRequest`/`http.NewRequestWithContext` directly, and not
  `newGHRequestForHostWithToken` with a post-hoc method/body mutation.
  - *Given* `make lint-custom` runs the `norawghrequest` analyzer, *When* it scans the new file,
    *Then* it reports zero findings.
**Files**: `github/http_client.go`, `github/pr_comments.go` (new).

##### Task 3.1.1a: Add `newGHRequestForHostWithTokenAndBody` (~5 min)
- Add `func newGHRequestForHostWithTokenAndBody(ctx context.Context, host, method, path, token
  string, body []byte) (*http.Request, error)` to `github/http_client.go`, following
  `newGHRequestForHostWithToken`'s same base-URL-resolution (`RestBaseURLForHost(host)`) and
  auth-header shape, but taking `method` and `body` as parameters instead of hardcoding
  `http.MethodGet`/`nil`. Add it to the `norawghrequest` analyzer's approved-constructor list
  (`tools/lint/norawghrequest`) alongside `newGHRequestForHostWithToken`,
  `NewConditionalRequest`, `NewConditionalRequestNoCache`, and
  `newGHGraphQLRequestForHostWithToken` — this is a plan-only note; the actual analyzer-code edit
  happens during implementation, not in this planning pass.
- Files: `github/http_client.go`, `tools/lint/norawghrequest` (analyzer's approved-list).

##### Task 3.1.1b: `PostPRComment(ctx, host, repo RepoRef, prNumber int, body, token string) error` (~5 min)
- Build the request via `newGHRequestForHostWithTokenAndBody(ctx, host, http.MethodPost, path,
  token, jsonBody)` against `POST /repos/{owner}/{repo}/issues/{prNumber}/comments` (GitHub's
  PR-comment endpoint is the issues-comments endpoint), with the comment body JSON-encoded.
- Files: `github/pr_comments.go`.

##### Task 3.1.1c: Unit test against a fake GitHub HTTP server (~5 min)
- Assert the request path/method/auth header shape (including that the new constructor produces
  a real `POST` with a body, not a GET); assert a non-2xx response surfaces as an error.
- Files: `github/pr_comments_test.go` (new).

#### Story 3.1.2: Stamp provenance at `report_pr_created` time
**As the** system recording a newly-created PR, **I want** the claiming host's identity and deep
link written into the PR, **so that** ownership is inspectable from GitHub directly.
**Acceptance Criteria**:
- A successful `report_pr_created` call posts a comment containing the item's deep link and the
  claiming `HostID`'s opaque string form — never a raw `AdvertisedAddress`/hostname (per
  `research/pitfalls.md` §3's leak warning).
  - *Given* `reportPRCreated` succeeds for item `bl_01J...` on host `host_01K...`, *When* the PR
    comment is posted, *Then* its body contains `ssq://<hostname>/backlog/v1/bl_01J...` and
    `host_01K...`, and contains no LAN IP or bare hostname:port string anywhere.
**Files**: `server/mcp/tools_backlog_pr.go`.

##### Task 3.1.2a: Build the deep link + call `PostPRComment` after the existing PR-pending transition (~5 min)
- In `reportPRCreated` (`server/mcp/tools_backlog_pr.go:132`), after the existing
  `SetBacklogItemPRAndTransition` success path, construct the deep link (reuse
  `session/deeplink`'s existing URL-building helper — confirm its exact function name before
  wiring) and call `github.PostPRComment`. Failure to post the comment is logged, not returned
  as an error to the caller (the PR/item transition already succeeded; provenance stamping is
  best-effort, matching this codebase's existing "don't let a secondary side-effect fail the
  primary transition" convention seen in `RecordClaim`/ADR-002).
- Files: `server/mcp/tools_backlog_pr.go`.

##### Task 3.1.2b: Test the stamped-comment content + best-effort-failure behavior (~5 min)
- Files: `server/mcp/tools_backlog_test.go` (extend the existing `reportPRCreated` test suite
  starting at line 2181).

#### Story 3.1.3: Webhook handlers read the stamped provenance back
**As** a webhook event arriving at a host that is *not* the PR's origin, **I want** to trace the
PR back to its item, **so that** `handlePRFixEvent` doesn't require the origin host to be
reachable.

**Scope note (resolves cross-artifact-consistency BLOCKER CB-1)**: `requirements.md`'s Goal 3
names both `github_webhook_pr_fix.go` and `github_webhook_handler.go`. Verified directly:
`github_webhook_handler.go`'s `Handle` (line 90) is pure event-type routing — it dispatches every
PR-related event type (`prFixEventTypes` = `check_run`, `workflow_run`, `pull_request_review`,
`issue_comment`, `github_webhook_pr_fix.go:33`) straight to `handlePRFixEvent` and does no
PR-provenance resolution itself; it never handles a raw `pull_request`/`check_suite` event
independently. Since 100% of PR-related traffic this file sees is forwarded to
`handlePRFixEvent`, wiring `parseProvenanceStamp` into `handlePRFixEvent` alone (Task 3.1.3a)
already covers every PR event `github_webhook_handler.go` routes — a second, separate wiring
into the routing file itself would have nothing left to resolve. `requirements.md`'s Goal 3 is
narrowed accordingly (see that file's own note) to name `github_webhook_pr_fix.go` as the actual
implementation site, with `github_webhook_handler.go` acknowledged only as the router that reaches
it.
**Acceptance Criteria**:
- Given a PR whose most recent comment contains a stamped deep link, the webhook handler can
  parse the item ID out of it.
  - *Given* a `check_run` webhook payload for a PR whose GitHub comments include the stamp from
    Story 3.1.2, *When* `handlePRFixEvent` processes it, *Then* it successfully resolves the
    originating item ID from the comment text, independent of any local `ClaimIndex` state.
**Files**: `server/services/github_webhook_pr_fix.go`.

##### Task 3.1.3a: Add a stamp-parsing helper + call it as a fallback (~5 min)
- Add `parseProvenanceStamp(commentBody string) (itemDeepLink string, ok bool)` and call it in
  `handlePRFixEvent` (`server/services/github_webhook_pr_fix.go:566`) only as a fallback when
  the existing local item-resolution path (whatever it is today — confirm before editing) comes
  up empty, so this is additive, not a replacement of the existing lookup.
- Files: `server/services/github_webhook_pr_fix.go`.

##### Task 3.1.3b: Test the fallback resolution path (~5 min)
- Files: `server/services/github_webhook_pr_fix_test.go`.

---

## Phase 4: UI surfaces
### Epic 4.1: Claim-conflict visibility (Goal 2, human-facing half)
**Goal**: An operator can see, without reading session UUIDs, that another host already owns an
item — reusing `DeepLinkErrorBanner`'s vetted a11y/copy patterns per `research/ux.md`.

#### Story 4.1.1: `ClaimConflictBanner` component
**As an** operator viewing a backlog item, **I want** a clearly-labeled, non-alarming banner
when another host holds the claim, **so that** I understand this is a "my other machine already
has it" situation, not a broken link.
**Acceptance Criteria**:
- Renders `role="status"`/`aria-live="polite"` (never `role="alert"`) for a claim-conflict state,
  matching `DeepLinkErrorBanner`'s cross-host-case convention.
  - *Given* `<ClaimConflictBanner hostname="hostA" itemDeepLink="ssq://hostA/backlog/v1/bl_01J..."
    lastSeenAt="2h ago" />`, *When* rendered, *Then* the root element has `role="status"` and
    `aria-live="polite"`, and its text names "hostA" explicitly (never "elsewhere").
- Offers a "Copy deep link" action whose accessible label updates on click (not a static
  `aria-label`), avoiding the exact gap `research/ux.md` §3 flags in the existing "Copy Link"
  button.
  - *Given* the banner is rendered, *When* the "Copy deep link" button is clicked, *Then* either
    its visible label or an associated `aria-live="polite"` region announces the copied state to
    assistive tech, not just a silent visual swap.
- **A `Disputed: true` record renders a visibly distinct state, not the plain single-claimant
  copy.** (Resolves CB-2's UI half — `design/ux.md`'s original Surface 1 had no "disputed" state
  at all, which is what let the backend's LWW pick silently read as an unqualified, undisputed
  claim.) See `design/ux.md` Surface 1's new "Disputed" wireframe for exact copy.
  - *Given* `<ClaimConflictBanner hostname="hostA" itemDeepLink="..." disputed={true} />`, *When*
    rendered, *Then* the banner's copy states this is a disputed claim needing reconciliation
    (not "claimed by hostA" phrased as settled fact), and its action is labeled "Resolve" rather
    than "Override — work on it here."
**Files**: `web-app/src/components/backlog/ClaimConflictBanner.tsx` (new),
`web-app/src/components/backlog/ClaimConflictBanner.css.ts` (new).

##### Task 4.1.1a: Component + CSS, structurally modeled on `DeepLinkErrorBanner.tsx` (~5 min)
- Props: `hostname: string; itemDeepLink: string; lastSeenAt?: string; disputed?: boolean;
  onCopyDeepLink?: () => void; onOverride?: () => void; onResolveDispute?: () => void`. Copy:
  "This issue is already claimed by \"{hostname}\"" / body naming staleness (`lastSeenAt`) when
  known; when `disputed` is true, copy and action swap to the "Disputed" variant per
  `design/ux.md` Surface 1 (CB-2).
- Files: `web-app/src/components/backlog/ClaimConflictBanner.tsx`,
  `web-app/src/components/backlog/ClaimConflictBanner.css.ts` (reuse
  `DeepLinkErrorBanner.css.ts`'s token values rather than inventing new ones).

##### Task 4.1.1b: Dynamic copy-confirmation a11y fix (~3 min)
- Implement the accessible-copy-confirmation via a local `copied` state driving either the
  button's own text or a visually-hidden `aria-live="polite"` span — do not repeat
  `BacklogItemDetail.tsx`'s existing static-`aria-label` gap.
- Files: `web-app/src/components/backlog/ClaimConflictBanner.tsx`.

##### Task 4.1.1c: Component test (~5 min)
- Assert `role`/`aria-live` per state, host-naming, and the copy-confirmation announcement.
- Files: `web-app/src/components/backlog/ClaimConflictBanner.test.tsx` (new).

#### Story 4.1.2: Wire the banner into `BacklogItemDetail`
**As an** operator opening an item's detail view, **I want** to see the claim-conflict banner
immediately, **so that** I don't start work only to discover the duplicate later.
**Acceptance Criteria**:
- The banner renders in the sticky header region, above/adjacent to the existing "Copy
  Link"/"Copy ID" row, when the item's `check_cross_host_claim` result is `ClaimHeldByOther`.
  - *Given* `BacklogItemDetail` loads item `bl_01J...` whose `ExternalURL` returns
    `ClaimHeldByOther` from the claim-check RPC, *When* the detail panel renders, *Then*
    `ClaimConflictBanner` appears in the sticky header region (near
    `BacklogItemDetail.tsx:1468-1473`'s existing button row).
- The **Override** action reuses `GateVerdictBox`'s existing verb/semantics, not a new one.
  - *Given* the banner's Override action is invoked, *When* it completes, *Then* the same
    audit-note/logging convention `GateVerdictBox`'s `onOverride` handler already uses is
    followed (confirm exact call by reading `GateVerdictBox.tsx`'s `onOverride` usage before
    wiring).
- When the banner's record is `Disputed: true`, its **Resolve** action calls
  `ClaimIndex.ResolveDispute` (Task 1.1.2e) via a new RPC, clearing the flag for that entry —
  resolves CB-2's UI wiring.
  - *Given* a claim record with `Disputed: true`, *When* the operator clicks "Resolve" and
    confirms, *Then* the backend clears `Disputed` for that `ExternalURL` and the banner reverts
    to (or stops rendering) the plain single-claimant treatment on next load.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`,
`server/mcp/tools_claim.go` (or wherever `check_cross_host_claim`'s sibling write RPC belongs —
confirm during implementation).

##### Task 4.1.2a: Fetch claim status + conditionally render the banner (~5 min)
- Add a query for the item's claim status (via the `check_cross_host_claim` RPC/tool from
  Story 2.2.3, exposed through whatever web-app data-fetching hook pattern
  `useBacklogService`/similar already establishes — confirm the exact hook naming convention
  before adding a new one) and render `ClaimConflictBanner` conditionally near line 1468.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`.

##### Task 4.1.2b: Wire Override to the existing conflict-acknowledgment path (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`.

##### Task 4.1.2c: Integration test for the conditional-render + Override flow (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`.

##### Task 4.1.2d: Wire the Disputed variant + Resolve action (~5 min)
- Pass `disputed`/`onResolveDispute` through from the claim-status query to `ClaimConflictBanner`;
  `onResolveDispute` calls the new resolve-dispute RPC (Task 1.1.2e's backend) and refetches the
  claim status on success.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`,
  `web-app/src/components/backlog/BacklogItemDetail.test.tsx`.

#### Story 4.1.3: Board-level claim indicator (`ClaimChip`, on `BacklogItemCard`)
**As an** operator scanning the backlog board, **I want** to see which cards are claimed
elsewhere before opening them, **so that** I catch the conflict before spawning a session, not
after (per `research/ux.md`'s JTBD: "board-level visibility is likely necessary, not optional").

**Correction (per `design/ux.md`'s "Resolving the three open questions" §3 / Surface 2)**: the
original target, `BacklogItemBadge.tsx`, is wrong — that component renders the non-board list
row, which `BacklogBoard.tsx` never uses at all (verified: `BacklogBoard.tsx` renders
`BacklogItemCard.tsx` for its cards). The correct target is a new `ClaimChip`, added as a
**sibling to `BlockerChip`** in `BacklogItemCard.tsx`'s footer row (`cardFooter`,
`BacklogItemCard.tsx:240`), matching `BlockerChip`'s compact-variant shape exactly rather than
inventing a new chip visual language — see `design/ux.md` Surface 2 for the full wireframe
(`[ 🔗 Claimed: hostA ]` / `[ ? Claim unknown ]`, positioned immediately after the existing
provenance badge at `BacklogItemCard.tsx:202-215`).
**Acceptance Criteria**:
- A card whose item is `ClaimHeldByOther` shows a compact `ClaimChip`, mirroring `BlockerChip`'s
  compact-variant shape.
  - *Given* `BacklogBoard` renders a `BacklogItemCard` for an item with `ClaimHeldByOther`
    status (from the locally-known `ClaimIndex` only — per `design/ux.md` Surface 2, the board
    does **not** fire a `check_cross_host_claim` RPC per visible card), *When* the card renders,
    *Then* `ClaimChip` appears in the card footer immediately after the provenance badge, distinct
    from `BlockerChip`, reading e.g. "Claimed: hostA", truncating gracefully within the card's
    existing width constraint; if the footer is already at capacity, the chip wraps to a second
    footer line rather than being dropped (per `design/ux.md` Surface 2's error-case table —
    unlike `BacklogItemBadge.tsx`'s own deferred-`BlockerChip` precedent, this feature has no
    equivalent secondary surface, so it may not be silently dropped).
**Files**: `web-app/src/components/backlog/BacklogBoard.tsx`,
`web-app/src/components/backlog/BacklogItemCard.tsx`,
`web-app/src/components/backlog/BacklogItemCard.css.ts`.

##### Task 4.1.3a: Add `ClaimChip` as a sibling to `BlockerChip` in `BacklogItemCard`'s footer (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemCard.tsx`,
  `web-app/src/components/backlog/BacklogItemCard.css.ts`.

##### Task 4.1.3b: Wire the claim-status data into `BacklogBoard`'s card render (~5 min)
- Files: `web-app/src/components/backlog/BacklogBoard.tsx`.

##### Task 4.1.3c: Component/board test for chip visibility + footer-wrap behavior (~5 min)
- Files: `web-app/src/components/backlog/BacklogItemCard.test.tsx`,
  `web-app/src/components/backlog/BacklogBoard.test.tsx`.

---

## Phase 5: #475 integration (BLOCKED until #475 lands)
### Epic 5.1: Consume #475's host identifier on `BacklogItemData`
**Goal**: Once #475 lands a `ClaimedByHostID`-shaped field, use it as the authoritative source
for "which host created this item" instead of relying solely on the locally-running process's
own live `HostIdentity` at claim-recording time. **Scope note**: this phase only swaps *which*
`HostID` Story 1.3.1's already-wired `RecordClaim` call uses — it does not introduce the
`RecordClaim` call site itself (that's Phase 1's job, done and always-on before #475 lands).

#### Story 5.1.1: Populate `ClaimingHostID` from `BacklogItemData`'s new field
**As the** claim-recording code in `Storage.CreateBacklogItem`, **I want** to read #475's
recorded host identifier (once it exists) rather than only ever using the current process's own
`HostIdentity.ID`, **so that** provenance is correct even when creation and claim-recording
happen in different processes (e.g. a restored/replayed create).
**Acceptance Criteria**:
- `RecordClaim`'s `ClaimingHostID` matches `BacklogItemData`'s #475-recorded claimant when
  present, falling back to the local process's own `HostIdentity.ID` when absent.
  - *Given* #475 has landed and a `BacklogItemData` row carries its new field populated with
    `host_01K...`, *When* `Storage.CreateBacklogItem` records the claim, *Then* the
    `ClaimRecord.ClaimingHostID` is `host_01K...`, not necessarily the current process's own
    identity.
**Files**: `session/storage.go`, `session/repository.go` (accessor only — do not restructure
`BacklogItemData` itself here; that's #475's change).

##### Task 5.1.1a: Add a single accessor, `BacklogItemData.ClaimedByHostIDOrEmpty() (session.HostID, bool)` (~5 min)
- Read #475's actual field name once it exists (this task cannot be written more precisely
  today — see Unresolved Questions). Centralizing behind one accessor is the point: only this
  function needs to change if #475's landed shape differs from what's assumed here.
- Files: `session/repository.go`.

##### Task 5.1.1b: Use the accessor in `Storage.CreateBacklogItem`'s `RecordClaim` call, falling back to local identity (~3 min)
- Files: `session/storage.go`.

#### Story 5.1.2: Accessor fallback for pre-#475 items
**As the** claim system, **I want** items created before #475 landed (no recorded claimant) to
degrade gracefully, **so that** this design doesn't break on the existing backlog.
**Acceptance Criteria**:
- An item with no #475 field set is treated as claimed by the local process's own identity at
  read time, not as an error.
  - *Given* a pre-#475 `BacklogItemData` row (field absent/zero), *When*
    `ClaimedByHostIDOrEmpty()` is called, *Then* it returns `(HostID{}, false)`, and the calling
    code (Task 5.1.1b) treats `false` as "use the local process's own `HostIdentity.ID`," never
    as an error or a panic.
**Files**: `session/repository.go`.

##### Task 5.1.2a: Test the absent-field fallback path (~3 min)
- Files: `session/repository_test.go` or wherever `BacklogItemData`'s existing accessor tests
  live (confirm before adding a new file).
