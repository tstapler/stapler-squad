# Implementation Plan: backlog-github-two-way-sync

**Feature**: Two-way linkage + status/label sync between imported backlog items and their GitHub issue counterparts (provenance display, closed-issue observation, forward sync backlog→GitHub, backward sync GitHub→backlog, loop prevention).
**Date**: 2026-08-03
**Status**: Ready for implementation
**ADRs**: `ADR-002-backward-sync-closed-issue-status-mapping.md`, `ADR-003-loop-prevention-watermark-design.md`
**Prerequisite (cited, not re-derived)**: `project_plans/backlog-github-issue-link/` — ADR-001 + `implementation/plan.md` design the `ExternalURL` field end-to-end. This project reuses that exact struct/gating shape for `Labels`, and re-implements the `ExternalURL` half itself in Phase 0 if the sibling project has not merged first (see Phase 0, Epic 0.1).

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|---|---|---|
| `ExternalURL` | The linked GitHub issue's `html_url`. Reused from ADR-001 verbatim. | Plain `string`, unconditional backfill (never local-wins gated). Implemented in Phase 0 if `backlog-github-issue-link` hasn't landed. |
| `Labels` | `[]string` of the GitHub issue's label names, persisted on `BacklogItemData`/ent/proto. | Gated by `UserModifiedFields` local-wins (per AC4's literal wording), unlike `ExternalURL`. In practice this gate is a no-op today because no UI lets a user hand-edit an item's `Labels` — see Phase 4/Unresolved Questions. |
| `State` (on `ExternalItem`/`githubIssue`) | GitHub's own `"open"`/`"closed"` issue state string, threaded from the REST response through to the sync loop. | Mirrors GitHub's own vocabulary; `GitHubPRsPlugin` leaves it at zero value (unused, not a violation — see interface-pollution-checklist rule 1). |
| `IssueUpdatedAt` (on `ExternalItem`) | The GitHub issue's own `updated_at` timestamp, newly exposed on `ExternalItem` (previously only used internally by `Fetch` to compute `newCursor`). | The comparison value for loop-prevention (ADR-003) — never local wall-clock time. |
| `GitHubSyncedIssueUpdatedAt` | New `*time.Time` field on `BacklogItem`/`BacklogItemData` — "the GitHub issue `updated_at` value this item's local state has already reacted to." Written by forward sync after a successful close, and by backward sync after reconciling (or deliberately skipping) an external state. | The single loop-prevention watermark (ADR-003) — replaces the ambiguous `ForwardSyncClosedAt` name floated in research once its actual read/write semantics were worked out (both directions write it, not just forward sync). |
| `ForwardSyncEnabled` / `BackwardSyncEnabled` | Per-`ItemSource` bools, default `false`, gating whether a `TransitionBacklogItemStatus(..., done)` event triggers an issue close (`ForwardSyncEnabled`) and whether `SyncOne` applies external state onto the local item (`BackwardSyncEnabled`). | Mirror the existing `Enabled` field's shape exactly (ent + proto + repository + service + UI). |
| `ForwardSyncCloseLabel` | Per-`ItemSource` optional string — a label applied (merged with existing labels, never replacing them) when forward sync closes an issue. | Empty string = no label applied. |
| `TriggeredByGitHubSync` | New untyped string constant (`"github_sync"`), sibling to `TriggeredByUser`/`TriggeredBySystem` (`session/backlog.go:90-93`). | Distinguishes backward-sync-driven transitions in the `BacklogStatusEvent` audit trail and in `WorkflowHistorySection.tsx`'s `triggeredBy` display, from both real users and other system automations. |
| `externalIssueCloser` | Narrow interface defined in `server/services` (the consumer package), with one method `CloseIssue`. Only `*GitHubIssuesPlugin` implements it. | Per `.claude/rules/interface-pollution-checklist.md` rules 1+2 — NOT a fourth method on the shared `ItemSourcePlugin` interface, which would force `GitHubPRsPlugin` to grow a no-op. |
| `CloseIssue` | New method on `GitHubIssuesPlugin`: `CloseIssue(ctx, config PluginConfig, externalID string, existingLabels []string, closeLabel string) error`. Issues one `PATCH /repos/{owner}/{repo}/issues/{number}` with `state=closed` and, if `closeLabel != ""`, a merged (existing ∪ closeLabel) `labels` array — never a bare replacement, since GitHub's labels field on this endpoint fully replaces the array (verify against GitHub's docs at implementation time, per ADR-001 Decision 3's lesson about not assuming API behavior). | Also posts one explanatory bot comment via a second call (`POST .../comments`) — not mergeable into the same PATCH, GitHub has no combined endpoint for state+labels+comment. |
| `StartBacklogGitHubForwardSyncSubscriber` | New package-level function in `server/services/backlog_github_forward_sync.go`, mirroring `StartAnalyticsSubscriber`/`StartPushSubscriber`/`StartSubscriber`'s exact skeleton (nil-guard → `bus.Subscribe(ctx)` → one goroutine → `select` on event chan / `ctx.Done()`). | Wired into `server/server.go` alongside the other three `Start*Subscriber` calls (not `server/dependencies.go`, which only constructs `deps.EventBus`). |
| `GuardedTransitionAllowed` | New function in `session/workflow_engine.go`: `GuardedTransitionAllowed(engine WorkflowEngine, item BacklogItemTransitionInput, to BacklogStatus) bool` — evaluates `CanTransition` + `ValidateGates` without executing the transition. | Backward sync's read-only guard check, kept in package `session` (not `server/services`) since `SyncOne` lives there and cannot import `server/services` (would create an import cycle). Distinct from — and not a replacement for — `transitionWithGuard` (`server/services/backlog_service_triage.go:484-500`), which also executes the transition; a future refactor could have `transitionWithGuard` call this same helper, flagged as an optional simplification, not required by this plan. |
| `determineBackwardSyncTarget` | New pure function: given the item's current `BacklogStatus`, returns the backward-sync target status for a closed issue (`BacklogStatusArchived`) or a sentinel meaning "no valid target, skip" for `in_progress`/`review`/`pr_pending`/already-terminal statuses. | See ADR-002 — this function embodies the "closed → archived, never done" policy decision. |
| `DecryptConfigToken` | Renamed from the unexported `decryptConfigToken` on `*SyncLoop` (`session/backlog_sync.go:129-174`) — capitalized so the new forward-sync subscriber (package `server/services`, holding a `*session.SyncLoop` handle) can call it cross-package. | Pure rename; behavior unchanged. All existing in-package call sites updated in the same commit. |
| `ParseUserModifiedFields` / `MergeUserModifiedFields` | New exported wrappers in `session/backlog_sync.go` (or a new `session/user_modified_fields.go`) around the existing unexported `parseUserModifiedFields`/`containsField` logic, plus a new `MergeUserModifiedFields(raw string, newFields ...string) (string, error)` that adds field names to the JSON set. | Needed so `server/services/backlog_service_lifecycle.go`'s `UpdateBacklogItem` RPC handler (a different package) can populate `UserModifiedFields` when a user edits Title/Description/Priority — the actual prerequisite fix for AC4's premise (pitfalls research §1: this write path does not exist today). |
| `SyncLoop.workflowEngine` | New field on `SyncLoop`, a `session.WorkflowEngine`, defaulting to `NewDefaultWorkflowEngine()` if not supplied to the constructor. | Lets `SyncOne` call `GuardedTransitionAllowed` without a new dependency-injection seam beyond what `SyncLoop` already threads through. |

---

## Pattern Decisions

### Step 0.5 — Alternatives considered

**1. Where forward sync hooks in (backlog status → GitHub issue close)**
- **Option A: inline in the RPC handler** (`server/services/backlog_service_lifecycle.go`'s `TransitionBacklogItemStatus`, next to the existing terminal-transition cleanup block at lines 586-591). *Strength*: simplest possible diff, reuses a block shape that already exists for the same trigger condition. *Weakness*: `EntRepository.TransitionBacklogItemStatus` is called directly (bypassing this RPC handler) from `autonomous_orchestration_service.go:506`, 10+ sites in `backlog_lifecycle.go`, and most of `backlog_service_triage.go` — a "done" transition driven by the autonomous orchestrator or a triage auto-transition would never reach code added only here, silently missing most real completion paths.
- **Option B: a new dedicated poller** that periodically scans for items in `done` without a corresponding closed issue and reconciles. *Strength*: fully decoupled from the transition call chain, no event-plumbing needed. *Weakness*: introduces polling latency for a state change that already has an exact, real-time signal (the transition itself); duplicates the "per-source lock, decrypt token, look up plugin" machinery `SyncLoop` already has, for no new capability; and requires its own idempotency/cursor bookkeeping distinct from `SyncLoop`'s.
- **Option C (chosen): a new `EventBus` subscriber**, filtering `events.BacklogChangeStatusTransition` with `NewStatus == "done"`. *Strength*: `EntRepository.TransitionBacklogItemStatus` already calls `publishItemChanged` unconditionally on every call regardless of caller (`session/ent_repository_backlog.go:954-958`) — this is the codebase's own existing answer to "what fires on every state change no matter who triggered it," already proven by three other subscribers (`server/analytics`, `server/push`, `server/notifications`). Zero changes needed to any of the 12+ existing `TransitionBacklogItemStatus` call sites. *Weakness*: the event payload only carries `BacklogItemData.SourceID` (a bare string), not a joined `ent.ItemSource` — the subscriber must do its own `GetItemSourceByID` lookup (one extra DB round-trip per done-transition, acceptable given done-transitions are not high-frequency).

Chosen: **Option C**, validating (not challenging) the research's recommendation.

**2. Backward sync's closed-issue → local-status mapping policy**
- **Option A: closed → `done` unconditionally** (with `OverrideReason` forced through the guard if needed). *Strength*: matches the naive mental model ("issue closed = done"). *Weakness*: `TransitionGuard`'s `to == BacklogStatusDone` branch (`session/domain/backlog.go:473-486`) requires `OverallOutcome == ReviewOutcomePass` and `!HasUnshippedCode` — computing `HasUnshippedCode` requires porting `isCodeShippedToMain`'s worktree/git-ancestry logic (currently a private method on `server/services.BacklogService`, using `session/git.IsCommitOnMain`) into package `session`, since `SyncOne` cannot import `server/services`. Using `OverrideReason` to force past the guard defeats the guard's entire purpose (someone closing an issue as "won't fix"/"duplicate" is not the same signal as "code shipped and passed review") and is exactly the failure mode the research doc calls out as the single most important open question.
- **Option B: closed → `archived`, always, for any non-done status; skip (log-only) for `in_progress`/`review`/`pr_pending`** (chosen — see ADR-002). *Strength*: `archived` has **no** `TransitionGuard` special case (the guard's `switch` only branches on `to == BacklogStatusDone`; every other target — including `archived` — hits the `default: return nil` case), so this policy needs zero `HasUnshippedCode`/`OverallOutcome` computation at all, sidestepping the entire cross-package porting problem. `archived` is reachable from `idea`/`refining`/`ready`/`queued` per `validTransitions` (`session/domain/backlog.go:332-354`) — exactly the "pre-work, not yet actively being executed" statuses where "the source tracker considers this closed" most honestly maps to "we're not pursuing this." *Weakness*: an issue closed while the item is `in_progress`/`review`/`pr_pending` has no valid backward-sync target under this policy at all (not reachable to `archived` per `validTransitions`) — a real, accepted limitation (documented in ADR-002), not silently invented.
- **Option C: a configurable per-source status-mapping table** (user picks which local status a closed issue maps to). *Strength*: maximally flexible. *Weakness*: disproportionate — requires new UI, new per-source config schema, and forces every user to make a decision the codebase's own guard rules already answer structurally for most cases; GitHub's own Projects v2 explicitly treats this mapping as a local product decision, not something to generalize away (research features.md §2).

Chosen: **Option B**, documented in ADR-002.

**3. Loop-prevention mechanism**
- **Option A: a coarse `lastSyncedAt`/`syncedByUs` boolean or timestamp compared against local wall-clock `time.Now()`**. *Strength*: trivial to implement. *Weakness*: GitHub's own read-after-write consistency is not instant (secondary read replicas, search-index lag) and local/remote clock skew makes a wall-clock comparison an unreliable ping-pong guard (pitfalls research §3-4); also can't distinguish "GitHub hasn't converged yet" from "genuinely reopened by someone else."
- **Option B (chosen): a per-item watermark storing GitHub's own `issue.updated_at` value** (`GitHubSyncedIssueUpdatedAt`), compared against the freshly-fetched `ExternalItem.IssueUpdatedAt` on every backward-sync tick — see ADR-003. *Strength*: uses GitHub's own monotonic-per-resource timestamp (the same value `Fetch` already uses for its `since` cursor), avoiding clock-skew entirely; both forward sync (after closing) and backward sync (after reconciling-or-skipping) advance the same watermark, so "have we already reacted to this exact remote state" is answered by one comparison regardless of which direction last touched it. *Weakness*: one new nillable ent field, and both sync directions must remember to advance it (a discipline risk mitigated by centralizing the write in one small helper, not duplicating the write in each caller).
- **Option C: full event-sourcing** (replay all `BacklogStatusEvent` rows to derive current state, no mutable snapshot). *Strength*: theoretically the most "correct" audit-complete model. *Weakness*: `BacklogItem.Status` is already a mutable snapshot with a parallel append-only event log for audit — full event-sourcing would be a much larger architectural change than this feature warrants (build-vs-buy research, verdict: disproportionate).

Chosen: **Option B**, documented in ADR-003.

### Pattern Decisions table

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Forward sync hook point | Observer (EventBus subscriber), GoF | `server/analytics/subscriber.go` (established in-repo pattern) | Inline in RPC handler; new dedicated poller | RPC handler is bypassed by 12+ non-RPC `TransitionBacklogItemStatus` call sites; a poller duplicates `SyncLoop` machinery for no new capability |
| `externalIssueCloser` capability | Narrow, consumer-defined interface (Dependency Inversion) | `.claude/rules/interface-pollution-checklist.md` rules 1+2 | Fourth method on shared `ItemSourcePlugin` | Would force `GitHubPRsPlugin` (two-way sync explicitly out of scope for PRs) to implement a no-op |
| Closed-issue → local-status mapping | Sum-type-style exhaustive decision table (`determineBackwardSyncTarget`), guarded by the existing `validTransitions`/`TransitionGuard` state machine | type-driven-design (illegal states unrepresentable via the existing guard, not a new one) | Unconditional closed→done; per-source configurable mapping table | `done`'s guard requires porting cross-package git/verdict logic and risks `OverrideReason` defeating the guard's purpose; a configurable table is disproportionate given the guard already answers most cases structurally |
| Loop prevention | Per-item high-water-mark (PoEAA-adjacent — same shape as the existing `PrFeedbackAddressedAt` watermark) | `session/backlog_lifecycle.go`'s `ReconcilePRPending` (precedent already in this codebase) | Wall-clock timestamp comparison; full event-sourcing | Clock-skew/read-after-write-lag makes wall-clock unreliable; event-sourcing is a disproportionate architectural change |
| Backward-sync guard evaluation | New pure function (`GuardedTransitionAllowed`) in package `session`, mirroring `transitionWithGuard`'s two-check shape without executing the transition | Existing remediation pattern from PR #199 (`server/services/backlog_service_triage.go:484-500`) | Duplicating guard logic ad hoc inside `SyncOne`; calling into `server/services` (impossible — import cycle) | `SyncOne` lives in package `session`, which cannot import `server/services`; a shared, testable helper avoids re-deriving `CanTransition`+`ValidateGates` sequencing incorrectly |
| `Labels` field type | `field.Strings("labels")` (ent's native `[]string` JSON column) | Precedent: `session/ent/schema/classificationanalytics.go:52` (`python_imports`) | A JSON-string column parsed manually (like `AcceptanceCriteria`) | No reason to introduce a second serialization convention for the same kind of data — `field.Strings` already gives a native `[]string` |
| `ExternalURL`/`Labels` write-gating | Two structurally distinct blocks in `SyncOne` (`ExternalURL` unconditional per ADR-001; `Labels`/`Status` gated by `UserModifiedFields`) | ADR-001 Decision 1 (structural independence of the unconditional block) | Folding all four fields into one gating idiom | ADR-001 already proved this exact mistake is easy to make by accident (a reorder could silently make `anyField` false for a fully-locked item) — this project must preserve, not collapse, that separation |
| Per-source sync-direction settings | First-class `ItemSource` fields (ent + proto + service + UI), mirroring `Enabled` | Existing `Enabled bool` shape, same entity | Config JSON blob | `Config` is confirmed write-only to the client (never read back) — a setting a user must be able to see/toggle cannot live there |

---

## Migration Plan

- **Migration mechanism**: ent auto-migration (`session/ent_repository.go`, `client.Schema.Create(ctx)` on every process startup), identical to ADR-001's approach — no standalone SQL migration file. New nullable/defaulted columns added via `session/ent/schema/backlog_item.go` (`labels`, `external_url` if not already landed, `github_synced_issue_updated_at`) and `session/ent/schema/item_source.go` (`forward_sync_enabled`, `backward_sync_enabled`, `forward_sync_close_label`), followed by `go generate ./session/ent` (which resolves to `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema` per `session/ent/generate.go:3` — never omit `--feature sql/upsert`).
- **Coordination with `backlog-github-issue-link`**: if that sibling project's `external_url` ent field/migration has already landed by the time this project starts implementation, Phase 0 Epic 0.1's `external_url` sub-tasks become a no-op (skip — verify via `grep external_url session/ent/schema/backlog_item.go` first) and only `labels` needs adding in that same ent-generate pass. If it has not landed, this project adds both fields (`external_url` + `labels`) in the same `Fields()` edit and single `go generate` run, so there is exactly one ent-codegen pass for this phase either way — never two separate regenerations for the same schema file.
- **Proto field-number coordination**: `BacklogItem.external_url` claims **30** (or is already claimed — recheck the live `.proto` before assigning), `BacklogItem.labels` claims **31** (confirmed next-free as of this research pass: `category = 29` is the current highest field, so 30/31 are free — recheck at implementation time regardless, per the general "never hardcode a stale field number" rule). `ItemSource.forward_sync_enabled` = **9**, `.backward_sync_enabled` = **10**, `.forward_sync_close_label` = **11** (current highest is `token_configured = 6`... confirmed highest is 8; next free is 9). `UpdateItemSourceRequest.forward_sync_enabled` = **5**, `.backward_sync_enabled` = **6**, `.forward_sync_close_label` = **7** (current highest is `token = 4`).
- **Reversibility**: irreversible via ent's auto-migration (no down-migrations, matching ADR-001's precedent) — all new columns are nullable/defaulted, so a revert leaves harmless unused columns.
- **Zero-downtime**: not applicable — single-process SQLite app, no rolling-deploy window (same as ADR-001).
- **Rollback**: revert the PR; new columns remain unused until a future schema edit removes them.

## Observability Plan

- **Logs**: extend `SyncOne`'s existing per-tick summary log (`session/backlog_sync.go:300-301`) — no new counter categories needed; backward-sync guard-rejections and skips (ADR-002's `in_progress`/`review`/`pr_pending` no-valid-target case, and ADR-003's "already-reconciled, skip" case) count toward the existing `skipped` counter, not `errored`. Add one new structured log line per backward-sync skip (`log.InfoLog.Printf("[SyncLoop] backward-sync skip item=%s from=%s reason=%s", ...)`) so a skip is diagnosable without a debugger — mirrors the existing pattern at `session/backlog_lifecycle.go`'s `ReconcilePRPending` logging.
- **Forward sync subscriber logs**: one line per attempted close (`log.InfoLog.Printf("[BacklogGitHubForwardSync] closing issue source=%s external_id=%s", ...)`) and one on failure (`log.WarningLog.Printf(...)`) — matching `analyticsSubscriber`'s existing log-and-skip-unknown-event-type convention.
- **Metrics**: no new metrics infrastructure — this repo's existing OTel instrumentation on `SyncOne`/`UpdateBacklogItem`/RPC handlers already covers the touched code paths; the new subscriber's goroutine has no new >100ms external operation beyond the GitHub HTTP calls, which inherit existing span coverage on outbound HTTP if any (verify at implementation time whether outbound GitHub calls are currently traced at all — if not, this is pre-existing gap, not introduced by this feature).
- **Alerts**: none new.
- **Per-item audit trail** (flagged as a genuine gap by features.md research, not fully closed by this plan): `BacklogStatusEvent`'s existing `triggeredBy` field, populated with the new `TriggeredByGitHubSync` constant for backward-sync-driven transitions, is the mechanism by which a team can tell "GitHub sync did this" apart from a human — already rendered generically by `WorkflowHistorySection.tsx:53` with zero frontend change required. A dedicated per-item sync-action log (distinct from the aggregate `SourceSyncEvent`) is explicitly NOT built in this plan (proportionality — the existing `BacklogStatusEvent`/`triggeredBy` mechanism is sufficient for status; label changes have no equivalent per-change audit row today and this plan does not add one, since Labels changes are lower-stakes display data, not a state-machine transition).

## Risk Control

- **Feature flags**: `ForwardSyncEnabled`/`BackwardSyncEnabled`, both default `false` per-`ItemSource` — the feature is fully inert until a user explicitly opts in per source, per direction (AC3/AC4). No separate build-time flag needed.
- **Staged rollout**: Phase 0 (prerequisites) and Phase 4 (UI) are safe to ship independently and are additive/inert. Phase 1 (forward sync) and Phase 2 (backward sync) are each gated by their own settings bool and can ship independently of each other — a user could enable forward sync alone with zero backward-sync code risk, and vice versa.
- **Blast-radius warning**: per pitfalls research §9, the Settings UI (Phase 4) must warn when both directions are enabled on the same source, and first-enable of backward sync should not silently bulk-transition every already-imported item in one tick — see Phase 4, Story 4.3 and Unresolved Questions #3.
- **Rollback**: standard revert via PR; both new booleans defaulting `false` means a rollback of the UI toggle alone (leaving the backend merged) is also safe — no user could have had a way to enable the setting without the corresponding UI.
- **Rate limiting**: forward sync's writes get their own `Retry-After`-aware check (Phase 1, Story 1.2) distinct from the existing read-path's primary-only check — a rate-limited write is retried on the next event (naturally, since a future done-transition or a manual "resync" — no automatic replay queue is built; see Unresolved Questions #5), not treated as fatal to the whole `SyncOne` batch (backward sync's per-item write failures already don't abort the batch — only `Fetch` failing does, unchanged by this project).

## Unresolved Questions

1. **Should a failed forward-sync write get an automatic retry queue?** This plan logs the failure and relies on the next real trigger (another done-transition can't happen twice for the same item; a manual re-open→re-done cycle, or a future "resync this item" affordance) rather than building a dedicated retry queue — flagged as a possible follow-up if failed-write visibility (Phase 4's persistent row-level warning) proves insufficient in practice. Not a blocker for any story in this plan.
2. **`CreateItemSource`'s pre-existing bug** (`server/services/backlog_service_lifecycle.go:663-694` hardcodes `Enabled: true`, never reading a request field) is confirmed but out of scope — this plan only adds fields to `UpdateItemSourceRequest` (sync-direction toggles are set post-creation, matching the existing UX where sources are configured incrementally after being added), so this pre-existing bug is not touched. Flagged for a separate, unrelated fix.
3. **First-enable-of-backward-sync blast radius**: pitfalls research §9 recommends the first enable of backward sync should not silently bulk-transition the entire existing backlog for that source in one tick. This plan's Phase 4 adds an inline warning (static UI copy) but does NOT implement a preview/dry-run or a "only sync forward from now on" cutoff — a full bounded-reconciliation UX is out of scope for this plan's ACs (none of AC0-AC7 require it) and is flagged here as a genuine, not-fully-resolved product question rather than silently assumed safe.
4. **Labels local-wins gate is currently unreachable** (no UI lets a user edit an item's `Labels` today) — implemented per AC4's literal wording anyway, for forward-compatibility, but has no observable effect until/unless a future feature adds label editing. Not a blocker; documented so a future reader doesn't mistake the dead gate for a bug.
5. **No automatic retry queue for a rate-limited forward-sync write** (see #1) — an item that fails to forward-sync (e.g. secondary rate limit) shows a warning (Phase 4) but nothing re-attempts the write automatically. A user could manually re-trigger by transitioning the item out of and back into `done` (which re-fires the event) as a manual workaround; this plan does not build a cleaner one.
6. **Whether outbound GitHub HTTP calls have existing OTel span coverage** — not verified by this research pass; flagged for the implementer to check before assuming Observability Plan's tracing claim holds.

### Known limitations (documented, not fixed by this plan)

Carried forward from research (features.md §3.2-3.3) — accepted as out of scope, not silently discovered later:

- **Two `ItemSource`s pointed at the same GitHub repo** produce independent, mutually-unaware duplicate imports; both would independently forward-sync writes to the same issue. No uniqueness constraint or save-time warning is added by this plan.
- **A transferred GitHub issue** (moved to a different repo, potentially assigned a new number by GitHub) breaks the `(sourceID, ExternalID)` lookup key outright — the source's `Fetch` (scoped to one fixed `owner/repo`) never sees it again under its original number. Detecting this would require polling by GitHub's immutable `node_id`, which neither plugin fetches today; out of scope.
- **A genuinely deleted GitHub issue** (rare — usually repo-admin-only) simply disappears from `Fetch`'s result set; the backlog item's provenance link goes stale-but-harmless, same symptom as the pre-existing "closed before ever synced" limitation.

## Dependency Visualization

```
Phase 0: Prerequisites (schema + core primitives — required by every later phase)
  Epic 0.1 (Labels + ExternalURL ent/proto/repository/plugin-mapping)
  Epic 0.2 (state=all fetch + State/IssueUpdatedAt on ExternalItem)
  Epic 0.3 (UserModifiedFields write-path: UpdateBacklogItem RPC handler)
  Epic 0.4 (TriggeredByGitHubSync + GuardedTransitionAllowed + SyncLoop.workflowEngine)
  Epic 0.5 (Per-source settings fields: ent/proto/repository/service — Forward/BackwardSyncEnabled, CloseLabel)
  Epic 0.6 (GitHubSyncedIssueUpdatedAt watermark field + DecryptConfigToken export)
        │
        ├──────────────────────────────┬───────────────────────────────┐
        ▼                              ▼                               ▼
Phase 1: Forward Sync            Phase 2: Backward Sync         Phase 4: UI (provenance display —
  Epic 1.1 (CloseIssue on          Epic 2.1 (closed→status          card/detail; independent of 1-3)
   GitHubIssuesPlugin)              via determineBackwardSyncTarget)   Epic 4.1 (card badge)
  Epic 1.2 (externalIssueCloser +  Epic 2.2 (reopened no-op-log)      Epic 4.2 (detail Source section)
   EventBus subscriber)            Epic 2.3 (Labels backward sync)    Epic 4.3 (Settings toggles +
  Epic 1.3 (bot comment)           Epic 2.4 (backfill ExternalURL/     both-directions warning)
        │                           Labels for pre-existing items,
        │                           AC6)
        │                              │
        └──────────────┬───────────────┘
                        ▼
        Phase 3: Loop Prevention (integration — both directions already
                  write the watermark from Phase 1/2; this phase adds
                  the read-and-skip check + Risk A/B regression tests)
                        │
                        ▼
        Phase 5: Final Verification (full build/test, registry update)
```
Phase 4 (UI) has no code dependency on Phases 1-3 (it only reads already-added `ExternalURL`/`Labels`/`ForwardSyncEnabled`/`BackwardSyncEnabled` fields from Phase 0) and can be implemented in parallel with them. Phase 3 depends on both Phase 1 (which writes the watermark on close) and Phase 2 (which reads it) — it cannot start meaningfully before both exist, though its regression-test-writing work can be drafted earlier. Phase 5 depends on all prior phases.

---

## Phase 0: Prerequisites

**Goal**: every piece of schema, cross-cutting infrastructure, and guard machinery that both forward and backward sync need, landed and tested before either sync direction is built. This phase can be implemented even if `backlog-github-issue-link` never ships (Epic 0.1 covers `ExternalURL` defensively).

### Epic 0.1: `Labels` (+ `ExternalURL` if not already landed) persistence

**Goal**: `BacklogItemData`/`BacklogItemUpdate`/ent schema/proto persist `Labels []string` (and `ExternalURL string`, reusing ADR-001's exact design, if the sibling project's field doesn't already exist), and `GitHubIssuesPlugin.MapToBacklogItem` populates both instead of dropping them.

#### Story 0.1.1: Add `labels` (and `external_url` if absent) to the ent schema and regenerate
**Acceptance Criteria** (AC1, part 1):
- *Given* `session/ent/schema/backlog_item.go` has no `labels` field, *When* `field.Strings("labels").Optional()` is added to `Fields()` (mirroring `classificationanalytics.go:52`'s `python_imports` precedent) immediately after the existing `external_id` field (line 74-75) and `go generate ./session/ent` is run, *Then* `session/ent/backlogitem.go` contains a `Labels []string` struct field and `session/ent/migrate/schema.go` gains a matching JSON-typed nullable column.
- *Given* a pre-existing row with `labels` `NULL` after migration, *When* fetched via `r.client.BacklogItem.Get(ctx, id)`, *Then* `item.Labels` is `nil` or an empty slice (not a panic) — verify ent's generated scan behavior for `field.Strings` on a NULL column matches the `Optional()` no-nillable convention (differs slightly from a plain string field; confirm via a quick read of the generated scan code before writing the test).
**Files**: `session/ent/schema/backlog_item.go`, `session/ent/backlogitem.go` (generated), `session/ent/migrate/schema.go` (generated), `session/ent/backlogitem_create.go` (generated), `session/ent/backlogitem_update.go` (generated), `session/ent/mutation.go` (generated)

##### Task 0.1.1a: Check whether `external_url` already exists (~1 min)
- Run `grep -n "external_url" /home/tstapler/Programming/stapler-squad/session/ent/schema/backlog_item.go`. If a match exists, `backlog-github-issue-link` has landed — skip Task 0.1.1c entirely. If no match, proceed with both 0.1.1b and 0.1.1c.
- Files: none (verification only)

##### Task 0.1.1b: Add `labels` field to ent schema (~2 min)
- In `session/ent/schema/backlog_item.go`, in `Fields()`, add immediately after the existing `field.String("external_id").Optional(),` block:
  ```go
  field.Strings("labels").
      Optional(),
  ```
- Files: `session/ent/schema/backlog_item.go`

##### Task 0.1.1c: Add `external_url` field to ent schema, ONLY if Task 0.1.1a found it missing (~2 min)
- Add `field.String("external_url").Optional(),` immediately after `external_id`, per ADR-001's exact shape (plain `Optional()`, no `.Nillable()`, no new index — see `project_plans/backlog-github-issue-link/decisions/ADR-001-external-url-backfill-and-prompt-boundary.md`).
- Files: `session/ent/schema/backlog_item.go`

##### Task 0.1.1d: Regenerate ent code and verify compilation (~3 min)
- Run `go generate ./session/ent` (resolves to `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema` — do not omit the flag).
- Run `go build ./session/...` — expected to pass (purely additive schema change).
- Files: `session/ent/backlogitem.go`, `session/ent/migrate/schema.go`, `session/ent/backlogitem_create.go`, `session/ent/backlogitem_update.go`, `session/ent/mutation.go` (all regenerated)

##### Task 0.1.1e: Add the NULL-safety test for `Labels` (~4 min)
- In `session/ent_repository_backlog_test.go` (create if it doesn't exist yet, matching the sibling project's naming), add `TestGetBacklogItem_Labels_ReadsEmptyForPreExistingRow`: create a `BacklogItem` via `repo.client.BacklogItem.Create()` without setting `labels`, fetch it back, assert no panic and `len(item.Labels) == 0`.
- Files: `session/ent_repository_backlog_test.go`

#### Story 0.1.2: Thread `Labels`/`ExternalURL` through `BacklogItemData`/`BacklogItemUpdate` and the ent converter
**Acceptance Criteria** (AC1, part 2):
- *Given* `session/repository.go`'s `BacklogItemData` struct (line 346-446) has no `Labels` field, *When* `Labels []string` is added immediately after `ExternalID string` (line 404), *Then* `BacklogItemData{Labels: []string{"bug","p1"}}` compiles.
- *Given* `storage.CreateBacklogItem(ctx, BacklogItemData{Title: "t", Labels: []string{"bug"}, ...})`, *When* refetched via `storage.GetBacklogItem`, *Then* `refetched.Labels == []string{"bug"}`.
**Files**: `session/repository.go`, `session/ent_repository_backlog.go`, `session/backlog_lifecycle_test.go`

##### Task 0.1.2a: Add `Labels`/`ExternalURL` to `BacklogItemData` (~2 min)
- In `session/repository.go`, add `Labels []string` after `ExternalID string` (line 404). If Task 0.1.1a found `external_url` absent, also add `ExternalURL string` on the following line (matching ADR-001's exact struct shape).
- Files: `session/repository.go`

##### Task 0.1.2b: Add `Labels`/`ExternalURL` to `BacklogItemUpdate` (~2 min)
- In `session/repository.go`, add `Labels *[]string` near line 524 (after `Notes *string`). If applicable, also add `ExternalURL *string`.
- Files: `session/repository.go`

##### Task 0.1.2c: Wire `Labels`/`ExternalURL` through `backlogItemToData`, `CreateBacklogItem`, `UpdateBacklogItem` (~4 min)
- In `session/ent_repository_backlog.go`:
  - `backlogItemToData` (lines 171-242): add `Labels: item.Labels,` (and `ExternalURL: item.ExternalURL,` if applicable) inside the struct literal (~line 205).
  - `CreateBacklogItem` (lines 263-309): add `.SetLabels(data.Labels).` (and `.SetNillableExternalURL(&data.ExternalURL).` if applicable) to the builder chain (~line 294).
  - `UpdateBacklogItem` (lines 533-656): add
    ```go
    if update.Labels != nil {
        u.SetLabels(*update.Labels)
    }
    ```
    immediately after the existing `ReworkCapOverride` block (lines 638-640) — and the matching `ExternalURL` block if applicable, following ADR-001 Decision 1's exact independence requirement (this block must NOT be nested inside any `UserModifiedFields`-gated `if`).
  - Also extend `updatedFieldsFromBacklogItemUpdate` (lines 664-748) with `Labels`/`ExternalURL` so they appear in `ChangeItemUpdated`'s `UpdatedFields`.
- Files: `session/ent_repository_backlog.go`

##### Task 0.1.2d: Add the Create/Get round-trip test (~3 min)
- In `session/backlog_lifecycle_test.go`, add `TestCreateBacklogItem_Labels_RoundTripsThroughGetBacklogItem`, mirroring the sibling project's `ExternalURL` round-trip test pattern.
- Files: `session/backlog_lifecycle_test.go`

#### Story 0.1.3: `GitHubIssuesPlugin.MapToBacklogItem` populates `Labels` (+ `ExternalURL` if applicable)
**Acceptance Criteria** (AC1, part 3):
- *Given* `ExternalItem{ExternalID: "42", Labels: []string{"bug","p1"}}` (already computed in `Fetch`, `session/backlog_plugin_github.go:151`, currently dropped by `MapToBacklogItem`), *When* `GitHubIssuesPlugin.MapToBacklogItem(item, sourceID)` is called, *Then* the returned `BacklogItemData.Labels == []string{"bug","p1"}`.
**Files**: `session/backlog_plugin_github.go`, `session/backlog_plugin_github_test.go`

##### Task 0.1.3a: Add `Labels: item.Labels,` to `MapToBacklogItem` (~2 min)
- In `session/backlog_plugin_github.go`, in `MapToBacklogItem` (lines 166-185), add `Labels: item.Labels,` to the returned literal (no truncation needed — label names are already bounded by GitHub's own ~50-char limit).
- Files: `session/backlog_plugin_github.go`

##### Task 0.1.3b: Add a `Labels` round-trip test (~3 min)
- Extend `TestGitHubIssuesPlugin_MapToBacklogItem_...` in `session/backlog_plugin_github_test.go` to assert `Labels` passes through unchanged.
- Files: `session/backlog_plugin_github_test.go`

##### Task 0.1.4: Run package tests for Epic 0.1 (~2 min)
- Run `go build ./session/... && go test ./session/... -run 'TestGitHubIssuesPlugin|TestCreateBacklogItem_Labels|TestGetBacklogItem_Labels'`
- Files: none (verification only)

---

### Epic 0.2: Observe closed/reopened issues (`state=all`) and expose `IssueUpdatedAt`

**Goal**: `GitHubIssuesPlugin.Fetch` queries `state=all`, decodes the issue's `state` field, and threads both `State` and `IssueUpdatedAt` onto `ExternalItem` — satisfying AC0 and providing the raw material Phase 3's loop-prevention watermark comparison needs.

#### Story 0.2.1: `Fetch` queries `state=all` and decodes `State`
**Acceptance Criteria** (AC0):
- *Given* `GitHubIssuesPlugin.Fetch`'s query string currently hardcodes `state=open` (`session/backlog_plugin_github.go:90`), *When* changed to `state=all`, *Then* a closed issue returned by GitHub's API (previously invisible to `Fetch`) is now included in the result set — verified via a test that mocks a closed-issue JSON response and asserts it appears in the returned `[]ExternalItem`.
- *Given* the `githubIssue` struct (line 46-55) has no `State` field, *When* `State string \`json:"state"\`` is added, *Then* `json.Unmarshal` populates it from GitHub's `"state": "open"|"closed"` response field.
- *Given* `ExternalItem` (`session/backlog_plugin.go:21-28`) has no `State` field, *When* `State string` is added and `Fetch`'s loop (lines 132-160) sets `State: issue.State,` on each `ExternalItem{}` literal, *Then* a fetched closed issue's `ExternalItem.State == "closed"`.
**Files**: `session/backlog_plugin_github.go`, `session/backlog_plugin.go`, `session/backlog_plugin_github_test.go`

##### Task 0.2.1a: Change `state=open` to `state=all` in the Fetch query (~2 min)
- In `session/backlog_plugin_github.go` (~line 90), change the query string from `issues?state=open&per_page=%d` to `issues?state=all&per_page=%d`.
- Files: `session/backlog_plugin_github.go`

##### Task 0.2.1b: Add `State` to `githubIssue` and `ExternalItem` (~3 min)
- In `session/backlog_plugin_github.go`, add `State string \`json:"state"\`` to the `githubIssue` struct (line 46-55).
- In `session/backlog_plugin.go`, add `State string` to `ExternalItem` (line 21-28, after `Labels []string`).
- In `Fetch`'s loop (lines 132-160), add `State: issue.State,` to the `ExternalItem{}` literal.
- Note (per interface-pollution-checklist rule 1): `GitHubPRsPlugin` does not need to populate this field — leaving it at zero value there is correct, not a defect, since two-way sync is explicitly out of scope for PRs.
- Files: `session/backlog_plugin_github.go`, `session/backlog_plugin.go`

##### Task 0.2.1c: Add a closed-issue Fetch test (~4 min)
- In `session/backlog_plugin_github_test.go`, add `TestGitHubIssuesPlugin_Fetch_IncludesClosedIssues`: mock a response with one open and one closed issue (`"state":"closed"`), assert both appear in the returned `[]ExternalItem` with correct `State` values.
- Files: `session/backlog_plugin_github_test.go`

#### Story 0.2.2: Expose `IssueUpdatedAt` on `ExternalItem`
**Acceptance Criteria** (prerequisite for Phase 3's ADR-003 watermark comparison — not a numbered AC on its own, but required by AC7):
- *Given* `githubIssue.UpdatedAt` is already parsed (used internally to compute `newCursor`, `session/backlog_plugin_github.go:130,157-158`) but not exposed on `ExternalItem`, *When* `IssueUpdatedAt time.Time` is added to `ExternalItem` and set from `issue.UpdatedAt` in `Fetch`'s loop, *Then* a fetched item's `ExternalItem.IssueUpdatedAt` equals the issue's `updated_at` value, independently checkable in a test from the same fixture used for the cursor computation.
**Files**: `session/backlog_plugin.go`, `session/backlog_plugin_github.go`, `session/backlog_plugin_github_test.go`

##### Task 0.2.2a: Add `IssueUpdatedAt` to `ExternalItem` and populate it (~3 min)
- In `session/backlog_plugin.go`, add `IssueUpdatedAt time.Time` to `ExternalItem`.
- In `session/backlog_plugin_github.go`'s `Fetch` loop, add `IssueUpdatedAt: issue.UpdatedAt,` to the literal (reusing the already-parsed value, not re-parsing).
- Files: `session/backlog_plugin.go`, `session/backlog_plugin_github.go`

##### Task 0.2.2b: Add a test asserting `IssueUpdatedAt` matches the cursor-computation value (~3 min)
- In `session/backlog_plugin_github_test.go`, extend the existing cursor test (or add a new one) asserting `ExternalItem.IssueUpdatedAt` equals the same timestamp used to compute `newCursor` for that same issue.
- Files: `session/backlog_plugin_github_test.go`

##### Task 0.2.3: Run package tests for Epic 0.2 (~2 min)
- Run `go build ./session/... && go test ./session/... -run TestGitHubIssuesPlugin`
- Files: none (verification only)

---

### Epic 0.3: Wire up `UserModifiedFields` from the user-edit RPC path

**Goal**: close the gap confirmed by pitfalls research §1 — `UserModifiedFields` is currently write-never in production, so AC4's "respects the existing guard" premise is false today. This is a genuine prerequisite task, not an assumed dependency.

#### Story 0.3.1: Export `ParseUserModifiedFields`/add `MergeUserModifiedFields` in package `session`
**Acceptance Criteria** (no numbered AC — pure prerequisite; pins the exact merge semantics a later test can rely on):
- *Given* an existing `UserModifiedFields` JSON string `'["title"]'`, *When* `session.MergeUserModifiedFields(raw, "priority")` is called, *Then* it returns a JSON string that `session.ParseUserModifiedFields` parses back into a set containing exactly `{"title", "priority"}` (order-independent, no duplicates if called twice with the same field).
**Files**: `session/backlog_sync.go` (or new `session/user_modified_fields.go`)

##### Task 0.3.1a: Export the parse function and add a merge function (~4 min)
- In `session/backlog_sync.go`, rename the unexported `parseUserModifiedFields` to exported `ParseUserModifiedFields` (update the 4-5 in-package call sites in the same commit) and the unexported `containsField` to exported `ContainsModifiedField` (update in-package call sites).
- Add a new function:
  ```go
  // MergeUserModifiedFields adds newFields to the existing JSON-encoded set of
  // user-modified field names, deduplicating, and returns the re-serialized JSON.
  func MergeUserModifiedFields(raw string, newFields ...string) (string, error) {
      existing := ParseUserModifiedFields(raw)
      set := make(map[string]bool, len(existing)+len(newFields))
      for _, f := range existing {
          set[f] = true
      }
      for _, f := range newFields {
          set[f] = true
      }
      merged := make([]string, 0, len(set))
      for f := range set {
          merged = append(merged, f)
      }
      sort.Strings(merged) // deterministic output for tests
      out, err := json.Marshal(merged)
      return string(out), err
  }
  ```
- Files: `session/backlog_sync.go`

##### Task 0.3.1b: Add a unit test for `MergeUserModifiedFields` (~3 min)
- In `session/backlog_sync_test.go`, add a table-driven test: empty raw + one new field; existing fields + a duplicate of an existing field (asserts no duplication); existing fields + a genuinely new field.
- Files: `session/backlog_sync_test.go`

#### Story 0.3.2: `UpdateBacklogItem` RPC handler populates `UserModifiedFields` when Title/Description/Priority are touched
**Acceptance Criteria** (prerequisite closing pitfalls research §1's gap — required for AC4 to have real meaning):
- *Given* a backlog item with `UserModifiedFields: ""`, *When* a user calls `UpdateBacklogItem` with a non-empty `Title` in the request, *Then* the item's stored `UserModifiedFields` afterward parses to a set containing `"title"`.
- *Given* the same item now has `UserModifiedFields` containing `"title"`, *When* the next `SyncOne` backward-fetch tick runs with a plugin-fetched title differing from the local one, *Then* the local title is NOT overwritten (exercising the pre-existing `containsField(modifiedFields, "title")` gate in `SyncOne`, `session/backlog_sync.go:265-268`, now actually reachable in production for the first time).
**Files**: `server/services/backlog_service_lifecycle.go`, `session/repository.go`, `session/ent_repository_backlog.go`, `server/services/backlog_service_test.go`

##### Task 0.3.2a: Add `UserModifiedFields *string` to `BacklogItemUpdate` and wire it into `UpdateBacklogItem` (repo layer) (~3 min)
- In `session/repository.go`, add `UserModifiedFields *string` to `BacklogItemUpdate` (line ~524).
- In `session/ent_repository_backlog.go`'s `UpdateBacklogItem` (lines 533-656), add:
  ```go
  if update.UserModifiedFields != nil {
      u.SetUserModifiedFields(*update.UserModifiedFields)
  }
  ```
- Files: `session/repository.go`, `session/ent_repository_backlog.go`

##### Task 0.3.2b: Populate `UserModifiedFields` in the RPC handler (~5 min)
- In `server/services/backlog_service_lifecycle.go`'s `UpdateBacklogItem` (lines 235-334): first, confirm whether the handler already loads the existing item before building `update` — if not, add `existing, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)` near the top (needed to read the current `UserModifiedFields` raw string to merge into).
- After the existing `if req.Msg.Title != "" { ... }` / `Description` / `Priority` presence checks (lines 249-263), collect a `var touchedFields []string` appending `"title"`/`"description"`/`"priority"` for each field the request actually set (mirrors the existing presence-based semantics already used for those three fields — this is a coarse "did this request touch the field" signal, not a value-diff, consistent with how the rest of this handler already works).
- If `len(touchedFields) > 0`, call `merged, err := session.MergeUserModifiedFields(existing.UserModifiedFields, touchedFields...)` and set `update.UserModifiedFields = &merged`.
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 0.3.2c: Add the RPC-level integration test (~5 min)
- In `server/services/backlog_service_test.go`, add `TestUpdateBacklogItem_PopulatesUserModifiedFieldsOnTitleEdit`: create an item, call `svc.UpdateBacklogItem` with a new `Title`, refetch, assert `UserModifiedFields` contains `"title"`.
- Add `TestSyncOne_UserEditedTitleSurvivesSubsequentBackwardSync` (the actual regression pitfalls research §1 asks for — not just that the plumbing compiles): create an item, edit its title via the RPC path (populating `UserModifiedFields` for real, not via a test-only `SetUserModifiedFields` call), then run `sl.SyncOne` with a plugin fetch returning a different title, assert the user's title survives.
- Files: `server/services/backlog_service_test.go`, `session/backlog_sync_test.go` (whichever file the second test naturally belongs in, given `SyncOne` is package `session` — likely needs to be a `session`-package test seeding `UserModifiedFields` via the same merge helper rather than a live RPC call, since `session` cannot import `server/services`)

##### Task 0.3.3: Run package tests for Epic 0.3 (~2 min)
- Run `go build ./... && go test ./session/... ./server/services/... -run 'UserModifiedFields'`
- Files: none (verification only)

---

### Epic 0.4: `TriggeredByGitHubSync` + `GuardedTransitionAllowed` + `SyncLoop.workflowEngine`

**Goal**: the audit-trail marker and the read-only guard-evaluation helper backward sync needs, without creating an import cycle (`session` cannot import `server/services`).

#### Story 0.4.1: Add `TriggeredByGitHubSync` constant
**Acceptance Criteria** (prerequisite — pins the exact string value used in later ADR-002/ADR-003 tests):
- *Given* `session/backlog.go:90-93` defines `TriggeredByUser`/`TriggeredBySystem`, *When* `TriggeredByGitHubSync = "github_sync"` is added alongside them, *Then* it's usable anywhere `triggeredBy string` is accepted (e.g. `TransitionBacklogItemStatus`) with no type changes needed.
**Files**: `session/backlog.go`

##### Task 0.4.1a: Add the constant (~1 min)
- In `session/backlog.go`, add `TriggeredByGitHubSync = "github_sync"` next to `TriggeredByUser`/`TriggeredBySystem` (lines 90-93).
- Files: `session/backlog.go`

#### Story 0.4.2: `GuardedTransitionAllowed` — read-only guard evaluation
**Acceptance Criteria** (prerequisite — enables ADR-002's `archived`-target policy without executing anything):
- *Given* an item with `Status: "idea"`, *When* `GuardedTransitionAllowed(engine, BacklogItemTransitionInput{Status: BacklogStatusIdea}, BacklogStatusArchived)` is called, *Then* it returns `true` (idea→archived is a valid edge per `validTransitions`, and `archived` hits `TransitionGuard`'s `default: return nil` case).
- *Given* an item with `Status: "in_progress"`, *When* called with `to: BacklogStatusArchived`, *Then* it returns `false` (no such edge in `validTransitions`).
**Files**: `session/workflow_engine.go`, `session/workflow_engine_test.go`

##### Task 0.4.2a: Implement `GuardedTransitionAllowed` (~3 min)
- In `session/workflow_engine.go`, add:
  ```go
  // GuardedTransitionAllowed evaluates whether a transition is both structurally
  // valid (CanTransition) and passes business-rule gates (ValidateGates),
  // WITHOUT executing it — the read-only counterpart to transitionWithGuard
  // (server/services/backlog_service_triage.go), for callers in package
  // session (like SyncOne) that cannot import server/services.
  func GuardedTransitionAllowed(engine WorkflowEngine, item BacklogItemTransitionInput, to BacklogStatus) bool {
      if !engine.CanTransition(item.Status, to) {
          return false
      }
      return engine.ValidateGates(item, to) == nil
  }
  ```
- Files: `session/workflow_engine.go`

##### Task 0.4.2b: Add unit tests for `GuardedTransitionAllowed` (~3 min)
- In `session/workflow_engine_test.go`, add table-driven tests: `idea→archived` (true), `in_progress→archived` (false, no edge), `review→done` with a passing `OverallOutcome` (true), `review→done` with no verdict (false, `ErrVerdictRequired`).
- Files: `session/workflow_engine_test.go`

#### Story 0.4.3: Add `workflowEngine` field to `SyncLoop`
**Acceptance Criteria** (prerequisite — no observable behavior change yet, just plumbing):
- *Given* `SyncLoop` is constructed today with no workflow-engine parameter, *When* a `workflowEngine WorkflowEngine` field is added (defaulting to `NewDefaultWorkflowEngine()` if the constructor is called with `nil`), *Then* existing `NewSyncLoop(...)` call sites compile unchanged (using a variadic-safe default, or by adding a new constructor parameter with all existing call sites updated to pass `nil`/the shared instance — pick whichever keeps the diff smallest given the actual current constructor signature at implementation time).
**Files**: `session/backlog_sync.go`, and any file constructing `SyncLoop` (verify exact call sites via `grep -rn "NewSyncLoop(" session/ server/` before editing)

##### Task 0.4.3a: Add the field and thread it through the constructor (~4 min)
- In `session/backlog_sync.go`, add `workflowEngine WorkflowEngine` to the `SyncLoop` struct (lines 34-40) and thread it through the constructor, defaulting to `NewDefaultWorkflowEngine()` when not supplied.
- Update all existing `NewSyncLoop(...)` call sites (locate via grep first) to pass the shared engine instance if one is already constructed nearby (e.g. `server/dependencies.go:469`'s `workflowEngine`), or `nil` to accept the default.
- Files: `session/backlog_sync.go`, whichever file(s) construct `SyncLoop`

##### Task 0.4.4: Run package tests for Epic 0.4 (~2 min)
- Run `go build ./... && go test ./session/... -run 'TestGuardedTransitionAllowed|TestSyncLoop'`
- Files: none (verification only)

---

### Epic 0.5: Per-source sync-direction settings (ent + proto + repository + service)

**Goal**: `ForwardSyncEnabled`, `BackwardSyncEnabled`, `ForwardSyncCloseLabel` become first-class, independently-toggleable, readable `ItemSource` fields — never buried in the write-only `Config` JSON blob.

#### Story 0.5.1: Ent schema + regenerate
**Acceptance Criteria** (prerequisite for AC3/AC4/AC5):
- *Given* `session/ent/schema/item_source.go` has no sync-direction fields, *When* `field.Bool("forward_sync_enabled").Default(false)`, `field.Bool("backward_sync_enabled").Default(false)`, and `field.String("forward_sync_close_label").Optional()` are added (next to `enabled`, line 29-30) and regenerated, *Then* `ent.ItemSource` gains matching struct fields, all defaulting to inert values on existing rows (both bools `false`, string `""`).
**Files**: `session/ent/schema/item_source.go`, generated ent files

##### Task 0.5.1a: Add the three fields to ent schema (~3 min)
- In `session/ent/schema/item_source.go`, add immediately after `field.Bool("enabled").Default(true),` (line 29-30):
  ```go
  field.Bool("forward_sync_enabled").
      Default(false),
  field.Bool("backward_sync_enabled").
      Default(false),
  field.String("forward_sync_close_label").
      Optional(),
  ```
- Files: `session/ent/schema/item_source.go`

##### Task 0.5.1b: Regenerate and verify (~3 min)
- Run `go generate ./session/ent && go build ./session/...`
- Files: generated ent files

#### Story 0.5.2: Proto fields
**Acceptance Criteria** (AC5 — configurable in Settings, requires a wire format):
- *Given* `proto/session/v1/backlog.proto`'s `ItemSource` message has fields 1-8, *When* `forward_sync_enabled = 9`, `backward_sync_enabled = 10`, `forward_sync_close_label = 11` are added, and the same three added to `UpdateItemSourceRequest` (currently fields 1-4) as `5`, `6`, `7`, *Then* `make proto-gen` produces matching Go and TypeScript bindings.
**Files**: `proto/session/v1/backlog.proto`

##### Task 0.5.2a: Recheck live field numbers before assigning (~1 min)
- Run `grep -n "message ItemSource\|message UpdateItemSourceRequest" -A 15 /home/tstapler/Programming/stapler-squad/proto/session/v1/backlog.proto` to confirm the highest field number in each message is still 8 and 4 respectively (this research pass confirmed it, but proto changes may have landed since) — do not blindly reuse 9/10/11 and 5/6/7 without this check.
- Files: none (verification only)

##### Task 0.5.2b: Add the three fields to `ItemSource` and `UpdateItemSourceRequest` (~3 min)
- Add `bool forward_sync_enabled = 9;`, `bool backward_sync_enabled = 10;`, `string forward_sync_close_label = 11;` to `ItemSource` (after `updated_at = 8`).
- Add `bool forward_sync_enabled = 5;`, `bool backward_sync_enabled = 6;`, `string forward_sync_close_label = 7;` to `UpdateItemSourceRequest` (after `token = 4`).
- Run `make proto-gen`.
- Files: `proto/session/v1/backlog.proto`, `session/gen/session/v1/*.go` (regenerated), `web-app/src/gen/session/v1/*_pb.ts` (regenerated)

#### Story 0.5.3: Repository + service plumbing
**Acceptance Criteria** (AC3/AC4/AC5, part 2 — the fields are readable/writable end-to-end):
- *Given* `session.ItemSourceData`/`ItemSourceUpdate` (`session/repository.go:577-594`) have no sync-direction fields, *When* `ForwardSyncEnabled bool`, `BackwardSyncEnabled bool`, `ForwardSyncCloseLabel string` are added to `ItemSourceData` (after `Enabled bool`, line 582) and `*bool`/`*string` equivalents to `ItemSourceUpdate`, *Then* `UpdateItemSource`'s handler (`server/services/backlog_service_lifecycle.go:698-733`) can thread `req.Msg.ForwardSyncEnabled`/etc. through to storage exactly as `Enabled` already is (line 711-712).
**Files**: `session/repository.go`, `session/ent_repository_backlog.go` (ItemSource CRUD methods — locate exact lines via `grep -n "func.*ItemSource" session/ent_repository_backlog.go` at implementation time), `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service.go` (`itemSourceToProto`)

##### Task 0.5.3a: Add fields to `ItemSourceData`/`ItemSourceUpdate` (~2 min)
- In `session/repository.go`, add `ForwardSyncEnabled bool`, `BackwardSyncEnabled bool`, `ForwardSyncCloseLabel string` to `ItemSourceData` (after `Enabled bool`, line 582); add `ForwardSyncEnabled *bool`, `BackwardSyncEnabled *bool`, `ForwardSyncCloseLabel *string` to `ItemSourceUpdate`.
- Files: `session/repository.go`

##### Task 0.5.3b: Wire into the ent `ItemSource` create/update methods (~4 min)
- Locate `EntRepository.CreateItemSource`/`UpdateItemSource` in `session/ent_repository_backlog.go` (grep for `func.*ItemSource` — not captured with exact line numbers by this plan's research; confirm before editing). Add `.SetForwardSyncEnabled(data.ForwardSyncEnabled).SetBackwardSyncEnabled(...).SetNillableForwardSyncCloseLabel(&data.ForwardSyncCloseLabel)` to create, and matching `if update.X != nil { u.SetX(*update.X) }` blocks to update.
- Files: `session/ent_repository_backlog.go`

##### Task 0.5.3c: Thread the fields through `UpdateItemSource`'s RPC handler (~3 min)
- In `server/services/backlog_service_lifecycle.go`'s `UpdateItemSource` (lines 698-733), add, mirroring the existing `Enabled` handling (line 711-712):
  ```go
  fwd := req.Msg.ForwardSyncEnabled
  update.ForwardSyncEnabled = &fwd
  bwd := req.Msg.BackwardSyncEnabled
  update.BackwardSyncEnabled = &bwd
  if req.Msg.ForwardSyncCloseLabel != "" {
      label := req.Msg.ForwardSyncCloseLabel
      update.ForwardSyncCloseLabel = &label
  }
  ```
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 0.5.3d: Add the three fields to `itemSourceToProto` so the UI can read current state back (~2 min)
- Locate `itemSourceToProto` (likely near `backlogItemToProto` in `server/services/backlog_service.go`, or a dedicated file — grep for `func itemSourceToProto`), add `ForwardSyncEnabled: item.ForwardSyncEnabled,` etc.
- Files: `server/services/backlog_service.go` (or wherever `itemSourceToProto` lives)

##### Task 0.5.3e: Add a round-trip test (~4 min)
- In `server/services/backlog_service_test.go`, add `TestUpdateItemSource_RoundTripsForwardBackwardSyncEnabled`: create a source, call `UpdateItemSource` setting both bools true and a close label, refetch via `ListItemSources` or `GetItemSource`, assert all three round-trip.
- Files: `server/services/backlog_service_test.go`

##### Task 0.5.4: Run package tests for Epic 0.5 (~2 min)
- Run `go build ./... && go test ./session/... ./server/services/... -run ItemSource`
- Files: none (verification only)

---

### Epic 0.6: `GitHubSyncedIssueUpdatedAt` watermark field + `DecryptConfigToken` export

**Goal**: the loop-prevention field itself (written in Phase 1/2, read in Phase 3) and the token-decryption method the forward-sync subscriber needs cross-package.

#### Story 0.6.1: `GitHubSyncedIssueUpdatedAt` ent field
**Acceptance Criteria** (prerequisite for AC7 — the field exists and round-trips; the loop-prevention logic itself is Phase 3):
- *Given* `session/ent/schema/backlog_item.go` has no such field, *When* `field.Time("github_synced_issue_updated_at").Optional().Nillable()` is added (mirroring `pr_feedback_addressed_at`'s exact shape, line 102-105) and regenerated, *Then* `BacklogItemData.GitHubSyncedIssueUpdatedAt *time.Time` round-trips through `CreateBacklogItem`/`UpdateBacklogItem` identically to `PrFeedbackAddressedAt`.
**Files**: `session/ent/schema/backlog_item.go`, `session/repository.go`, `session/ent_repository_backlog.go`, generated ent files

##### Task 0.6.1a: Add the ent field (~2 min)
- In `session/ent/schema/backlog_item.go`, add immediately after `pr_feedback_addressed_at` (line 102-105):
  ```go
  field.Time("github_synced_issue_updated_at").
      Optional().
      Nillable(),
  ```
- Files: `session/ent/schema/backlog_item.go`

##### Task 0.6.1b: Regenerate (~2 min)
- Run `go generate ./session/ent && go build ./session/...`
- Files: generated ent files

##### Task 0.6.1c: Add `GitHubSyncedIssueUpdatedAt` to `BacklogItemData`/`BacklogItemUpdate` and wire the converter (~4 min)
- Mirror `PrFeedbackAddressedAt`'s exact three-place pattern: `BacklogItemData` (session/repository.go, next to `PrFeedbackAddressedAt` line 426), `BacklogItemUpdate` (+ a `ClearGitHubSyncedIssueUpdatedAt bool`, mirroring `ClearPrFeedbackAddressedAt`, line 554), and `backlogItemToData`/`UpdateBacklogItem` in `session/ent_repository_backlog.go` (mirroring lines ~627-631's `if update.ClearPrFeedbackAddressedAt { u.ClearPrFeedbackAddressedAt() } else if update.PrFeedbackAddressedAt != nil { ... }` shape).
- Files: `session/repository.go`, `session/ent_repository_backlog.go`

##### Task 0.6.1d: Add a round-trip test (~3 min)
- Mirror the existing `PrFeedbackAddressedAt` round-trip test pattern (find it via grep in `session/backlog_lifecycle_test.go` or `session/ent_repository_backlog_test.go`) for `GitHubSyncedIssueUpdatedAt`.
- Files: whichever test file the `PrFeedbackAddressedAt` precedent test lives in

#### Story 0.6.2: Export `DecryptConfigToken`
**Acceptance Criteria** (prerequisite — needed by Phase 1's cross-package subscriber):
- *Given* `SyncLoop.decryptConfigToken` (`session/backlog_sync.go:129-174`) is unexported, *When* renamed to `DecryptConfigToken`, *Then* all existing in-package call sites (including the test wrapper at lines 125-127, which can now be deleted since the method itself is exported) still compile, and `server/services` can call `sl.DecryptConfigToken(raw)` on a `*session.SyncLoop` value.
**Files**: `session/backlog_sync.go`, `session/backlog_sync_test.go`

##### Task 0.6.2a: Rename and update call sites (~3 min)
- In `session/backlog_sync.go`, rename `decryptConfigToken` → `DecryptConfigToken` (all receiver-method call sites in the same file, e.g. inside `SyncOne`).
- In `session/backlog_sync_test.go`, delete the now-redundant exported test-only wrapper (lines 125-127) since the method itself is now exported and directly callable from tests.
- Files: `session/backlog_sync.go`, `session/backlog_sync_test.go`

##### Task 0.6.3: Run package tests for Epic 0.6 (~2 min)
- Run `go build ./... && go test ./session/... -run 'TestSyncOne|TestDecryptConfigToken'`
- Files: none (verification only)

---

## Phase 1: Forward Sync (AC3)

**Goal**: transitioning a backlog item to `done` closes the linked GitHub issue (optionally applying a configured label), via a new `EventBus` subscriber, gated by `ForwardSyncEnabled`, using the same per-source token `Fetch` already has.

### Epic 1.1: `CloseIssue` on `GitHubIssuesPlugin`

#### Story 1.1.1: Implement `CloseIssue`
**Acceptance Criteria** (AC3, part 1):
- *Given* an issue with existing labels `["bug"]` and a configured `closeLabel = "shipped"`, *When* `CloseIssue(ctx, config, "42", []string{"bug"}, "shipped")` is called, *Then* exactly one `PATCH /repos/{owner}/{repo}/issues/42` request is issued with body `{"state":"closed","labels":["bug","shipped"]}` (merged, not replaced) — verified against a mock HTTP server asserting the request body.
- *Given* `closeLabel == ""`, *When* `CloseIssue` is called, *Then* the PATCH body omits `labels` entirely (`{"state":"closed"}`), leaving the issue's existing labels untouched (since omitting the field is different from sending an empty array, which GitHub would interpret as "clear all labels" — verify this exact semantic against GitHub's docs at implementation time per ADR-001 Decision 3's lesson about not assuming API behavior).
**Files**: `session/backlog_plugin_github.go`, `session/backlog_plugin_github_test.go`

##### Task 1.1.1a: Implement `CloseIssue` (~5 min)
- In `session/backlog_plugin_github.go`, add:
  ```go
  // CloseIssue closes a GitHub issue and, if closeLabel is non-empty, merges it
  // into the issue's existing labels (never replacing them — GitHub's labels
  // field on this endpoint fully replaces the array, so existingLabels must be
  // passed in and merged locally; verify this replace-vs-merge semantic against
  // GitHub's REST API docs before relying on it, per ADR-001 Decision 3's lesson).
  func (p *GitHubIssuesPlugin) CloseIssue(ctx context.Context, config PluginConfig, externalID string, existingLabels []string, closeLabel string) error {
      cfg, err := decodeGithubPluginConfig(config)
      if err != nil {
          return err
      }
      body := map[string]interface{}{"state": "closed"}
      if closeLabel != "" {
          body["labels"] = mergeLabels(existingLabels, closeLabel)
      }
      payload, err := json.Marshal(body)
      if err != nil {
          return err
      }
      url := githubAPIURL(cfg.Host, fmt.Sprintf("repos/%s/%s/issues/%s", cfg.Owner, cfg.Repo, externalID))
      req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(payload))
      if err != nil {
          return err
      }
      req.Header.Set("Authorization", "token "+cfg.Token)
      req.Header.Set("Content-Type", "application/json")
      resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
      if err != nil {
          return err
      }
      defer resp.Body.Close()
      if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
          return fmt.Errorf("github: rate limited closing issue %s (retry-after=%s)", externalID, resp.Header.Get("Retry-After"))
      }
      if resp.StatusCode >= 300 {
          return fmt.Errorf("github: close issue %s failed: status %d", externalID, resp.StatusCode)
      }
      return nil
  }

  func mergeLabels(existing []string, add string) []string {
      for _, l := range existing {
          if l == add {
              return existing
          }
      }
      return append(append([]string{}, existing...), add)
  }
  ```
- Note: reuse whatever the existing `Fetch` method already uses to decode `config` into `githubPluginConfig` (locate the exact helper — likely inline in `Fetch`, not a separate `decodeGithubPluginConfig` function; adjust to match the real decode call, don't invent a new one if `Fetch` already has this logic factored differently).
- Files: `session/backlog_plugin_github.go`

##### Task 1.1.1b: Add `CloseIssue` tests (~5 min)
- In `session/backlog_plugin_github_test.go`, add `TestGitHubIssuesPlugin_CloseIssue_MergesLabels` (asserts merged body), `TestGitHubIssuesPlugin_CloseIssue_NoLabelOmitsLabelsField`, `TestGitHubIssuesPlugin_CloseIssue_RateLimitedReturnsError` (mock a 403 + `X-RateLimit-Remaining: 0` response, assert a descriptive error, not a panic).
- Files: `session/backlog_plugin_github_test.go`

#### Story 1.1.2: Bot comment on close
**Acceptance Criteria** (AC3, part 1b — per pitfalls research §7's "no silent automated action" convention):
- *Given* `CloseIssue` succeeds, *When* the forward-sync subscriber (Epic 1.2) calls a follow-up `PostIssueComment`, *Then* a `POST /repos/{owner}/{repo}/issues/{number}/comments` request is issued with a body explaining the automated close (e.g. `"Closed automatically — the linked backlog item was marked done in stapler-squad."`).
**Files**: `session/backlog_plugin_github.go`, `session/backlog_plugin_github_test.go`

##### Task 1.1.2a: Implement `PostIssueComment` (~4 min)
- In `session/backlog_plugin_github.go`, add a small sibling method `PostIssueComment(ctx context.Context, config PluginConfig, externalID string, body string) error`, following the identical request-construction shape as `CloseIssue` but `POST .../issues/{n}/comments` with `{"body": body}`.
- Files: `session/backlog_plugin_github.go`

##### Task 1.1.2b: Add a test (~3 min)
- In `session/backlog_plugin_github_test.go`, add `TestGitHubIssuesPlugin_PostIssueComment_SendsExpectedBody`.
- Files: `session/backlog_plugin_github_test.go`

##### Task 1.1.3: Run package tests for Epic 1.1 (~2 min)
- Run `go build ./session/... && go test ./session/... -run 'TestGitHubIssuesPlugin_CloseIssue|TestGitHubIssuesPlugin_PostIssueComment'`
- Files: none (verification only)

---

### Epic 1.2: `externalIssueCloser` interface + `EventBus` subscriber

#### Story 1.2.1: Define `externalIssueCloser` and the subscriber skeleton
**Acceptance Criteria** (AC3, part 2 — the subscriber fires on the right event and is a structural no-op for non-issue plugins):
- *Given* a `BacklogChangeStatusTransition` event with `NewStatus == "done"` for an item whose source's plugin is `github_issues` with `ForwardSyncEnabled == true`, *When* `StartBacklogGitHubForwardSyncSubscriber` is running, *Then* it calls `CloseIssue` on that source's plugin instance.
- *Given* the same event but the item's source plugin is `github_prs` (which does not implement `externalIssueCloser`), *When* the subscriber processes the event, *Then* the type assertion `plugin.(externalIssueCloser)` fails, `ok == false`, and the subscriber logs a debug line and returns — no panic, no error.
- *Given* `ForwardSyncEnabled == false` on the source, *When* the event fires, *Then* no GitHub call is made at all.
**Files**: `server/services/backlog_github_forward_sync.go` (new), `server/services/backlog_github_forward_sync_test.go` (new)

##### Task 1.2.1a: Define the interface and `Start...Subscriber` skeleton (~5 min)
- Create `server/services/backlog_github_forward_sync.go`:
  ```go
  package services

  type externalIssueCloser interface {
      CloseIssue(ctx context.Context, config session.PluginConfig, externalID string, existingLabels []string, closeLabel string) error
      PostIssueComment(ctx context.Context, config session.PluginConfig, externalID string, body string) error
  }

  // StartBacklogGitHubForwardSyncSubscriber closes the GitHub issue linked to a
  // backlog item when that item transitions to done, if the item's source has
  // ForwardSyncEnabled. Mirrors StartAnalyticsSubscriber's skeleton exactly.
  func StartBacklogGitHubForwardSyncSubscriber(ctx context.Context, bus *events.EventBus, syncLoop *session.SyncLoop, storage *session.Storage) {
      if bus == nil || syncLoop == nil || storage == nil {
          log.WarningLog.Println("[BacklogGitHubForwardSync] missing dependency, subscriber not started")
          return
      }
      ch, _ := bus.Subscribe(ctx)
      go func() {
          for {
              select {
              case event, ok := <-ch:
                  if !ok {
                      return
                  }
                  if event == nil || event.Type != events.EventBacklogItemChanged {
                      continue
                  }
                  payload := event.BacklogItemPayload
                  if payload == nil || payload.Kind != events.BacklogChangeStatusTransition || payload.NewStatus != string(session.BacklogStatusDone) {
                      continue
                  }
                  handleForwardSyncClose(ctx, syncLoop, storage, payload.Item)
              case <-ctx.Done():
                  return
              }
          }
      }()
  }
  ```
- Files: `server/services/backlog_github_forward_sync.go`

##### Task 1.2.1b: Implement `handleForwardSyncClose` (~5 min)
- In the same file:
  ```go
  func handleForwardSyncClose(ctx context.Context, syncLoop *session.SyncLoop, storage *session.Storage, item *session.BacklogItemData) {
      if item == nil || item.SourceID == "" || item.ExternalID == "" {
          return // locally-created item, nothing to sync
      }
      entRepo, ok := storage.Repo().(*session.EntRepository) // adjust to actual accessor
      if !ok {
          return
      }
      source, err := entRepo.GetItemSourceByID(ctx, item.SourceID)
      if err != nil || !source.ForwardSyncEnabled {
          return
      }
      plugin, ok := syncLoop.Registry().Get(source.PluginID) // adjust to actual accessor
      if !ok {
          return
      }
      closer, ok := plugin.(externalIssueCloser)
      if !ok {
          log.InfoLog.Printf("[BacklogGitHubForwardSync] plugin=%s does not support closing issues, skip item=%s", source.PluginID, item.ID)
          return
      }
      token, err := syncLoop.DecryptConfigToken(source.Config)
      if err != nil {
          log.WarningLog.Printf("[BacklogGitHubForwardSync] decrypt token failed source=%s: %v", source.ID, err)
          return
      }
      config := session.PluginConfig{ /* populate from source.Config + token, matching Fetch's own config-building shape */ }
      if err := closer.CloseIssue(ctx, config, item.ExternalID, item.Labels, source.ForwardSyncCloseLabel); err != nil {
          log.WarningLog.Printf("[BacklogGitHubForwardSync] close issue failed item=%s: %v", item.ID, err)
          return
      }
      _ = closer.PostIssueComment(ctx, config, item.ExternalID, "Closed automatically — the linked backlog item was marked done in stapler-squad.")
      // Advance the loop-prevention watermark — see Phase 3 for the read side.
      now := time.Now().UTC()
      if _, err := storage.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &now}, nil); err != nil {
          log.WarningLog.Printf("[BacklogGitHubForwardSync] failed to persist watermark item=%s: %v", item.ID, err)
      }
  }
  ```
- **Note for the implementer**: the exact accessor method names (`storage.Repo()`, `syncLoop.Registry()`, `session.PluginConfig{}`'s actual field names) were not captured with full precision by this plan's research pass — verify each against the live code before writing this file; the shape above is correct, the exact method/field names may need adjusting.
- Files: `server/services/backlog_github_forward_sync.go`

##### Task 1.2.1c: Wire the subscriber into `server/server.go` (~2 min)
- Add one line alongside the three existing `Start*Subscriber` calls (near line 595, after `analytics.StartAnalyticsSubscriber(...)`):
  ```go
  services.StartBacklogGitHubForwardSyncSubscriber(serverCtx, deps.EventBus, deps.SyncLoop, deps.Storage)
  ```
  (adjust `deps.SyncLoop`/`deps.Storage` to whatever the actual `Dependencies` struct field names are — verify via `server/dependencies.go` at implementation time).
- Files: `server/server.go`

##### Task 1.2.2: Add subscriber tests (~5 min)
- In `server/services/backlog_github_forward_sync_test.go`, add: `TestForwardSyncSubscriber_ClosesIssueOnDoneTransition_WhenEnabled`, `TestForwardSyncSubscriber_NoOpWhenForwardSyncDisabled`, `TestForwardSyncSubscriber_NoOpWhenPluginDoesNotImplementCloser` (using a fake `github_prs`-like plugin registered without `CloseIssue`).
- Files: `server/services/backlog_github_forward_sync_test.go`

##### Task 1.2.3: Run package tests for Epic 1.2 (~2 min)
- Run `go build ./... && go test ./server/services/... -run ForwardSync`
- Files: none (verification only)

---

## Phase 2: Backward Sync (AC4)

**Goal**: `SyncOne` applies the source issue's closed/open state and label changes onto the backlog item, respecting `UserModifiedFields` local-wins for labels, and routing status changes through the guarded `archived`-only policy (ADR-002).

### Epic 2.1: Closed-issue → status mapping

#### Story 2.1.1: `determineBackwardSyncTarget` and the guarded transition attempt in `SyncOne`
**Acceptance Criteria** (AC4, part 1 — see ADR-002 for the full policy rationale):
- *Given* an item with `Status: "ready"` and its linked issue now `State: "closed"`, *When* `SyncOne` processes it with `BackwardSyncEnabled == true`, *Then* `determineBackwardSyncTarget(BacklogStatusReady)` returns `BacklogStatusArchived`, `GuardedTransitionAllowed` returns `true` (ready→archived is a valid edge, no special gate), and the item transitions to `archived` with `triggeredBy == TriggeredByGitHubSync`.
- *Given* an item with `Status: "in_progress"` and its linked issue now closed, *When* `SyncOne` processes it, *Then* `determineBackwardSyncTarget` returns a sentinel meaning "no valid target" (in_progress cannot reach archived), the transition is skipped, logged, and counted toward `skipped` (not `errored`).
- *Given* an item already `Status: "done"` and its linked issue closed, *When* `SyncOne` processes it, *Then* `GuardedTransitionAllowed(engine, ..., done)`... actually no target is attempted at all — `done` is already terminal for this policy, so the item is left untouched (`archived` is technically reachable from `done` per `validTransitions`, but ADR-002 explicitly does NOT auto-archive an already-`done` item just because its issue is closed — see ADR-002 for why).
**Files**: `session/backlog_sync.go`, `session/backlog_sync_test.go`

##### Task 2.1.1a: Implement `determineBackwardSyncTarget` (~4 min)
- In `session/backlog_sync.go`, add:
  ```go
  // determineBackwardSyncTarget implements ADR-002's policy: a closed GitHub
  // issue maps to BacklogStatusArchived for pre-work statuses only. It never
  // targets "done" (would require porting HasUnshippedCode/OverallOutcome
  // computation cross-package and risks conflating "closed" with "shipped").
  // Returns ok=false when there is no valid target under this policy (item is
  // already done/archived, or mid-flight in_progress/review/pr_pending).
  func determineBackwardSyncTarget(current BacklogStatus) (target BacklogStatus, ok bool) {
      switch current {
      case BacklogStatusIdea, BacklogStatusRefining, BacklogStatusReady, BacklogStatusQueued:
          return BacklogStatusArchived, true
      default:
          return "", false
      }
  }
  ```
- Files: `session/backlog_sync.go`

##### Task 2.1.1b: Add the closed-issue branch to `SyncOne`'s existing-item path (~5 min)
- In `session/backlog_sync.go`, in the existing-item branch (after the `anyField`/label-gating additions from Epic 2.3, before the `if !anyField { skipped++; continue }` check), add a structurally separate block (status transitions bypass `BacklogItemUpdate`/`anyField` entirely — they use `TransitionBacklogItemStatus`, a different call, per the pre-existing "Status is always local-wins" comment this block now supersedes with real logic):
  ```go
  if source.BackwardSyncEnabled && data.State == "closed" {
      alreadyReconciled := existing.GitHubSyncedIssueUpdatedAt != nil && !data.IssueUpdatedAt.After(*existing.GitHubSyncedIssueUpdatedAt)
      if !alreadyReconciled {
          if target, ok := determineBackwardSyncTarget(BacklogStatus(existing.Status)); ok {
              guardInput := BacklogItemTransitionInput{
                  Status:            BacklogStatus(existing.Status),
                  AcCriteria:        existing.AcceptanceCriteria,
                  PlanApproved:      existing.PlanApproved,
                  SkipPlanning:      existing.SkipPlanning,
                  PlanArtifactsPath: existing.PlanArtifactsPath,
              }
              if GuardedTransitionAllowed(sl.workflowEngine, guardInput, target) {
                  if _, err := sl.storage.TransitionBacklogItemStatus(ctx, existing.ID, target, nil, TriggeredByGitHubSync); err != nil {
                      log.WarningLog.Printf("[SyncLoop] backward-sync transition failed item=%s: %v", existing.ID, err)
                      errored++
                  } else {
                      updated++
                  }
              } else {
                  log.InfoLog.Printf("[SyncLoop] backward-sync skip item=%s status=%s (no valid target for closed issue)", existing.ID, existing.Status)
                  skipped++
              }
          } else {
              log.InfoLog.Printf("[SyncLoop] backward-sync skip item=%s status=%s (mid-flight or terminal, no auto-archive)", existing.ID, existing.Status)
              skipped++
          }
          watermark := data.IssueUpdatedAt
          _, _ = sl.storage.UpdateBacklogItem(ctx, existing.ID, BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &watermark}, nil)
      }
  }
  ```
  Note: this block does its own `updated++`/`skipped++`/`errored++` accounting alongside the existing field-update counters — verify at implementation time that `SyncOne`'s counters aren't double-incremented if both this block and the field-update block fire for the same item in the same tick (they're independent counts of different kinds of change, both legitimate — confirm the final per-tick summary line's semantics still read sensibly).
- Files: `session/backlog_sync.go`

##### Task 2.1.1c: Add regression tests (~6 min)
- In `session/backlog_sync_test.go`, add: `TestSyncOne_BackwardSync_ClosedIssueArchivesReadyItem`, `TestSyncOne_BackwardSync_ClosedIssueSkipsInProgressItem` (asserts no transition, `skipped` incremented), `TestSyncOne_BackwardSync_NoOpWhenBackwardSyncDisabled`, `TestSyncOne_BackwardSync_DoesNotReArchiveAlreadyDoneItem`.
- Files: `session/backlog_sync_test.go`

##### Task 2.1.2: Run package tests for Epic 2.1 (~2 min)
- Run `go build ./session/... && go test ./session/... -run 'TestSyncOne_BackwardSync'`
- Files: none (verification only)

---

### Epic 2.2: Reopened-issue handling (log-only, no forced transition)

#### Story 2.2.1: Reopened issue on an `archived`/`done` item is a documented no-op
**Acceptance Criteria** (AC4, part 2 — see ADR-002):
- *Given* an item `Status: "archived"` whose linked issue's `State` flips to `"open"`, *When* `SyncOne` processes it with `BackwardSyncEnabled == true`, *Then* no transition is attempted (archived's only valid edge is to `idea`, which this plan deliberately does not auto-fire — see ADR-002), a log line is emitted (`"GitHub issue reopened; backlog item is archived — reopen manually to re-triage from idea"`), and the watermark still advances (so this doesn't re-log every tick).
**Files**: `session/backlog_sync.go`, `session/backlog_sync_test.go`

##### Task 2.2.1a: Add the reopened-issue log-only branch (~3 min)
- In `session/backlog_sync.go`, alongside the closed-issue block from Task 2.1.1b, add:
  ```go
  if source.BackwardSyncEnabled && data.State == "open" && (existing.Status == string(BacklogStatusArchived) || existing.Status == string(BacklogStatusDone)) {
      alreadyLogged := existing.GitHubSyncedIssueUpdatedAt != nil && !data.IssueUpdatedAt.After(*existing.GitHubSyncedIssueUpdatedAt)
      if !alreadyLogged {
          log.InfoLog.Printf("[SyncLoop] GitHub issue reopened; backlog item=%s is %s — reopen manually to re-triage (no automatic action taken)", existing.ID, existing.Status)
          watermark := data.IssueUpdatedAt
          _, _ = sl.storage.UpdateBacklogItem(ctx, existing.ID, BacklogItemUpdate{GitHubSyncedIssueUpdatedAt: &watermark}, nil)
      }
  }
  ```
- Files: `session/backlog_sync.go`

##### Task 2.2.1b: Add a test (~3 min)
- In `session/backlog_sync_test.go`, add `TestSyncOne_BackwardSync_ReopenedIssueOnArchivedItemLogsNoOp` and asserts no `TransitionBacklogItemStatus`-driven status change occurred (item's `Status` unchanged after `SyncOne`).
- Files: `session/backlog_sync_test.go`

##### Task 2.2.2: Run package tests for Epic 2.2 (~2 min)
- Run `go build ./session/... && go test ./session/... -run 'TestSyncOne_BackwardSync_Reopened'`
- Files: none (verification only)

---

### Epic 2.3: Labels backward sync (local-wins gated)

#### Story 2.3.1: Add the `labels` gated block to `SyncOne`
**Acceptance Criteria** (AC4, part 3):
- *Given* an existing item with `Labels: []string{"bug"}`, no `"labels"` entry in `UserModifiedFields`, and the plugin now returns `Labels: []string{"bug","p1"}`, *When* `SyncOne` runs, *Then* the item's `Labels` updates to `["bug","p1"]`.
- *Given* the same setup but `UserModifiedFields` contains `"labels"` (per Epic 0.3's gate — currently unreachable via any existing UI, but the mechanism must still honor it if ever set, e.g. via a future feature or direct test seeding), *When* `SyncOne` runs, *Then* `Labels` is NOT overwritten.
**Files**: `session/backlog_sync.go`, `session/backlog_sync_test.go`

##### Task 2.3.1a: Add the labels gating block (~2 min)
- In `session/backlog_sync.go`'s existing-item branch, immediately after the existing `priority` gated block (lines 273-276), add:
  ```go
  if !ContainsModifiedField(modifiedFields, "labels") {
      update.Labels = &data.Labels
      anyField = true
  }
  ```
- Files: `session/backlog_sync.go`

##### Task 2.3.1b: Add tests (~4 min)
- In `session/backlog_sync_test.go`, add `TestSyncOne_BackwardSync_UpdatesLabelsWhenNotUserLocked` and `TestSyncOne_BackwardSync_SkipsLabelsWhenUserLocked` (seeding `UserModifiedFields` directly, since no production path sets `"labels"` today — documented as expected in the Domain Glossary).
- Files: `session/backlog_sync_test.go`

##### Task 2.3.2: Run package tests for Epic 2.3 (~2 min)
- Run `go build ./session/... && go test ./session/... -run 'TestSyncOne_BackwardSync_.*Labels'`
- Files: none (verification only)

---

### Epic 2.4: Backfill `ExternalURL`/`Labels` for pre-existing items (AC6)

#### Story 2.4.1: `SyncOne` unconditionally backfills `Labels` (and `ExternalURL` if Phase 0 implemented it)
**Acceptance Criteria** (AC6):
- *Given* an existing item with `Labels: nil`, `UserModifiedFields: '["title","description","priority"]'` (all three gated fields locked), and the plugin returns `Labels: []string{"bug"}`, *When* `SyncOne` runs, *Then* `Labels` is backfilled to `["bug"]` — **but** per Epic 2.3's gating decision, if `"labels"` is ALSO in `UserModifiedFields`, it is NOT backfilled (unlike `ExternalURL`, which per ADR-001 is unconditional regardless of any gate). This is a deliberate asymmetry: `ExternalURL` is unconditional-backfill (AC6's original intent, per ADR-001), `Labels` is gated (per AC4's literal wording, per architecture research §3's resolution) — do not conflate the two.
- *Given* the sibling project's `ExternalURL` backfill logic (if not already landed via Phase 0), *When* an existing item has `ExternalURL: ""` and the plugin returns a URL, *Then* it backfills unconditionally exactly per ADR-001's Decision 1 (independent `anyField = true`, never nested inside a gated block).
**Files**: `session/backlog_sync.go`, `session/backlog_sync_test.go`

##### Task 2.4.1a: Add the `ExternalURL` backfill block, ONLY if not already present from a landed sibling PR (~3 min)
- Check via `grep -n "ExternalURL" session/backlog_sync.go` first. If absent, add (verbatim from ADR-001):
  ```go
  if existing.ExternalURL == "" && data.ExternalURL != "" {
      update.ExternalURL = &data.ExternalURL
      anyField = true
  }
  ```
  immediately after the three `UserModifiedFields`-gated blocks and structurally separate from them (ADR-001 Decision 1).
- Files: `session/backlog_sync.go`

##### Task 2.4.1b: Add backfill regression tests (~5 min)
- In `session/backlog_sync_test.go`, add `TestSyncOne_BackfillsLabelsOnExistingItemWithNoLabels` and, if Task 2.4.1a applied, `TestSyncOne_BackfillsExternalURLEvenWhenAllOtherFieldsAreUserModified` (mirroring the sibling project's own test of the same name, if not already present).
- Files: `session/backlog_sync_test.go`

##### Task 2.4.2: Run package tests for Epic 2.4 (~2 min)
- Run `go build ./session/... && go test ./session/... -run 'TestSyncOne_Backfills'`
- Files: none (verification only)

---

## Phase 3: Loop Prevention (AC7)

**Goal**: pin, with regression tests, that neither Risk A (forward-sync-closes → next tick's backward sync re-observes "closed") nor Risk B (a `done` item manually reopened locally, without touching GitHub, gets silently re-closed by backward sync) causes thrash. The actual watermark read/write logic already landed in Phase 1 (write, on close) and Phase 2 (read+skip, on the closed-issue branch) and Epic 2.2 (read+skip, on the reopened branch) — this phase is integration verification, not new production code, plus one additional safety net.

### Epic 3.1: Risk A regression test (structural — already covered by "no self-edges")

#### Story 3.1.1: Pin that `done → done` is impossible via `CanTransition`
**Acceptance Criteria** (AC7, part 1):
- *Given* an item already `Status: "done"` (forward-sync closed its issue on the transition INTO done), *When* the next `SyncOne` tick observes the same closed issue, *Then* `determineBackwardSyncTarget(BacklogStatusDone)` returns `ok=false` (done is not in the switch's matched cases) — so no transition is even attempted, structurally, with zero reliance on the watermark for this specific case (the watermark is what prevents Risk B, not Risk A — Risk A is prevented for free by the state machine).
**Files**: `session/backlog_sync_test.go`

##### Task 3.1.1a: Add `TestSyncOne_BackwardSync_DoneItemClosedIssueIsNoOpEvenWithoutWatermark` (~4 min)
- Construct an item at `Status: "done"` with `GitHubSyncedIssueUpdatedAt: nil` (deliberately NOT set, to prove the no-op holds even without the watermark's help) and a plugin fetch returning `State: "closed"`. Assert `SyncOne` makes zero `TransitionBacklogItemStatus` calls and the item's status is unchanged.
- Files: `session/backlog_sync_test.go`

### Epic 3.2: Risk B regression test (the watermark's actual job)

#### Story 3.2.1: Manual reopen after forward-sync-close does not get re-closed
**Acceptance Criteria** (AC7, part 2 — this is the actual infinite-loop risk AC7 is worried about):
- *Given* forward sync closed an issue and set `GitHubSyncedIssueUpdatedAt = T1` (the issue's `updated_at` at close time), the item is then manually transitioned locally from `done` back to `in_progress` (a valid edge, no GitHub interaction), and the GitHub issue remains closed with `updated_at` still `T1` (nobody touched it again), *When* the next `SyncOne` tick fetches the issue (`State: "closed"`, `IssueUpdatedAt: T1`), *Then* `!data.IssueUpdatedAt.After(*existing.GitHubSyncedIssueUpdatedAt)` is true (T1 is not after T1), the item is treated as "already reconciled," and NO transition is attempted — the item stays `in_progress`, not silently pushed back to `done`/`archived`.
- *Given* the same setup, but this time a *different* person genuinely re-closes-then-reopens the issue on GitHub AFTER the manual local reopen (so the issue's `updated_at` advances to `T2 > T1`), *When* the next tick fetches `IssueUpdatedAt: T2`, *Then* the watermark check now sees `T2.After(T1) == true`, and (since `data.State` on this second observation would be whatever the issue's *current* state is — closed or open — the branch corresponding to that state fires normally, proving the watermark only suppresses the exact-echo case, not genuinely newer external changes.
**Files**: `session/backlog_sync_test.go`

##### Task 3.2.1a: Add `TestSyncOne_BackwardSync_ManualReopenAfterForwardSyncCloseIsNotReClosed` (~5 min)
- Seed an item at `Status: "in_progress"` (simulating the post-manual-reopen state) with `GitHubSyncedIssueUpdatedAt: &t1`. Configure the fake plugin to return `ExternalItem{State: "closed", IssueUpdatedAt: t1}` (same timestamp — the echo). Run `SyncOne`. Assert zero `TransitionBacklogItemStatus` calls occurred and `Status` is still `"in_progress"`.
- Files: `session/backlog_sync_test.go`

##### Task 3.2.1b: Add `TestSyncOne_BackwardSync_GenuinelyNewerExternalCloseIsProcessed` (~4 min)
- Same setup, but the fake plugin returns `IssueUpdatedAt: t2` where `t2.After(t1)`. Assert the closed-issue branch DOES fire this time (transition attempted per `determineBackwardSyncTarget`).
- Files: `session/backlog_sync_test.go`

##### Task 3.2.2: Run package tests for Epic 3.2 and the full `session` package (~3 min)
- Run `go build ./session/... && go test ./session/... -run 'TestSyncOne'`
- Files: none (verification only)

---

## Phase 4: UI (AC2, AC5)

**Goal**: provenance display on the card/detail view, and per-source Settings toggles — following this repo's existing conventions exactly (`lucide-react` + `aria-hidden`, not the emoji pattern; `role="switch"` toggle; `CollapsibleSection` for the detail view).

### Epic 4.1: Card badge (AC2, part 1)

#### Story 4.1.1: Provenance badge on `BacklogItemCard`
**Acceptance Criteria** (AC2):
- *Given* an item with `ExternalURL: "https://github.com/acme/widget/issues/42"`, *When* `BacklogItemCard` renders, *Then* a small badge renders with a `Github` (lucide-react) icon (`aria-hidden="true"`) and text `#42`, wrapped in a real `<a href=... target="_blank" rel="noopener noreferrer">`, with `aria-label="Imported from GitHub issue #42"` on the wrapping element.
- *Given* the same badge is clicked, *When* the click fires, *Then* it opens the GitHub URL and does NOT also trigger `onClick(item.id)` (the card's own detail-open handler) — verified via the same `e.stopPropagation()` + `data-action-button`-style guard pattern the existing action button uses (`BacklogItemCard.tsx:192-197`).
- *Given* an item with `ExternalURL == ""` (locally-created, no source), *When* the card renders, *Then* no badge renders at all (nothing to show).
**Files**: `web-app/src/components/backlog/BacklogItemCard.tsx`, `web-app/src/components/backlog/BacklogItemCard.css.ts`, `web-app/src/components/backlog/BacklogItemCard.test.tsx`

##### Task 4.1.1a: Add the badge markup (~4 min)
- In `web-app/src/components/backlog/BacklogItemCard.tsx`, import `Github` from `lucide-react` (matching `VcsWidgetGithubRow.tsx:3`'s import style). Add, guarded by `item.externalUrl`:
  ```tsx
  {item.externalUrl && (
    <a
      href={item.externalUrl}
      target="_blank"
      rel="noopener noreferrer"
      className={styles.provenanceBadge}
      aria-label={`Imported from GitHub issue #${item.externalId}`}
      data-action-button="true"
      onClick={(e) => e.stopPropagation()}
    >
      <Github aria-hidden="true" size={12} />
      #{item.externalId}
    </a>
  )}
  ```
  placed near the existing `data-action-button` action button so it's covered by the same `handleCardClick` guard (`data-action-button` selector, `BacklogItemCard.tsx:133-135`) with no changes needed to `handleCardClick` itself.
- Files: `web-app/src/components/backlog/BacklogItemCard.tsx`

##### Task 4.1.1b: Add the badge CSS token (~3 min)
- In `BacklogItemCard.css.ts`, add `provenanceBadge` following `BacklogItemBadge.css.ts`'s 3-token shape (`background`/`color`/`border` via `vars.color.*`, not a hardcoded GitHub-brand color — per UX research's accessibility note about unaudited brand colors).
- Files: `web-app/src/components/backlog/BacklogItemCard.css.ts`

##### Task 4.1.1c: Add tests (~4 min)
- In `BacklogItemCard.test.tsx`, add: badge renders when `externalUrl` present; badge absent when not; clicking the badge does not fire the card's `onClick`.
- Files: `web-app/src/components/backlog/BacklogItemCard.test.tsx`

##### Task 4.1.2: Run frontend tests for Epic 4.1 (~2 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItemCard"`
- Files: none (verification only)

---

### Epic 4.2: Detail-view "Source" section (AC2, part 2)

#### Story 4.2.1: New collapsible "Source" section showing the issue link + label chips
**Acceptance Criteria** (AC2):
- *Given* an item with `externalUrl` and `labels: ["bug","p1"]`, *When* `BacklogItemDetail.tsx` renders, *Then* a new `<SourceSection>` (wrapped in `CollapsibleSection sectionKey="source" title="Source"`) renders showing the full issue title/link and label chips styled per `GitHubIssuePicker.css.ts`'s `labelBadge`.
- *Given* an item with `externalUrl == ""`, *When* the detail view renders, *Then* the section does not render at all (matching `PullRequestSection`'s guard-at-call-site pattern — the parent decides whether to render, not the section itself).
**Files**: `web-app/src/components/backlog/detail/SourceSection.tsx` (new), `web-app/src/components/backlog/detail/SourceSection.css.ts` (new), `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/components/backlog/detail/SourceSection.test.tsx` (new)

##### Task 4.2.1a: Create `SourceSection.tsx` (~5 min)
- New file, modeled on `PullRequestSection.tsx`'s structure:
  ```tsx
  import { Github } from "lucide-react";
  import { CollapsibleSection } from "../../ui/Collapsible";
  import * as styles from "./SourceSection.css";

  interface SourceSectionProps {
    externalUrl: string;
    externalId: string;
    labels: string[];
  }

  export function SourceSection({ externalUrl, externalId, labels }: SourceSectionProps) {
    return (
      <CollapsibleSection sectionKey="source" title="Source" defaultExpanded={false}>
        <div className={styles.section}>
          <a href={externalUrl} target="_blank" rel="noopener noreferrer" className={styles.link}>
            <Github aria-hidden="true" size={14} />
            Issue #{externalId}
          </a>
          {labels.length > 0 && (
            <div className={styles.labels}>
              {labels.map((label) => (
                <span key={label} className={styles.labelBadge} title={label}>
                  {label}
                </span>
              ))}
            </div>
          )}
        </div>
      </CollapsibleSection>
    );
  }
  ```
- Files: `web-app/src/components/backlog/detail/SourceSection.tsx`

##### Task 4.2.1b: Add `SourceSection.css.ts` (~3 min)
- Mirror `GitHubIssuePicker.css.ts`'s `labelBadge` token-for-token (same `vars.*` references), plus a simple `link`/`section` style following `detailShared.css.ts`'s existing conventions.
- Files: `web-app/src/components/backlog/detail/SourceSection.css.ts`

##### Task 4.2.1c: Wire the guard into `BacklogItemDetail.tsx` (~2 min)
- Add, near the existing `{item.status === "pr_pending" && <PullRequestSection ... />}` block:
  ```tsx
  {item.externalUrl && (
    <SourceSection externalUrl={item.externalUrl} externalId={item.externalId} labels={item.labels ?? []} />
  )}
  ```
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 4.2.1d: Add tests (~4 min)
- New `SourceSection.test.tsx`: renders link + labels when props present; a `BacklogItemDetail.tsx` test (or extend an existing one) asserting the section is absent when `externalUrl` is empty.
- Files: `web-app/src/components/backlog/detail/SourceSection.test.tsx`, `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 4.2.2: Run frontend tests for Epic 4.2 (~2 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="SourceSection|BacklogItemDetail"`
- Files: none (verification only)

---

### Epic 4.3: Settings toggles + both-directions warning (AC5)

#### Story 4.3.1: Two new `role="switch"` toggles + close-label input
**Acceptance Criteria** (AC5):
- *Given* a source row in `BacklogSourcesSettings.tsx`, *When* rendered, *Then* it shows two new toggles under a "Sync with GitHub" sub-heading: "Close GitHub issues when I finish here" (forward) and "Reflect GitHub status back here" (backward), both `role="switch" aria-checked={...}`, both defaulting to the fetched `source.forwardSyncEnabled`/`backwardSyncEnabled` values, plus a text input for the close label (visible/enabled only when the forward toggle is on).
- *Given* both toggles are enabled simultaneously for the same source, *When* rendered, *Then* an inline warning appears: "Both directions are enabled — closing this item's issue may be observed and re-applied by backward sync. Verify this doesn't create a loop for items you also edit manually."
**Files**: `web-app/src/components/settings/BacklogSourcesSettings.tsx`, `web-app/src/lib/hooks/useBacklogSourcesService.ts`, `web-app/src/components/settings/BacklogSourcesSettings.test.tsx`

##### Task 4.3.1a: Add `setForwardSyncEnabled`/`setBackwardSyncEnabled`/`setForwardSyncCloseLabel` hook functions (~5 min)
- In `useBacklogSourcesService.ts`, add three functions mirroring `setItemSourceEnabled` (lines 125-144) exactly, each calling `updateItemSource` with the corresponding field set and the other three (`displayName`, `enabled`, `token: ""`) passed through unchanged from the current source state (since `UpdateItemSource`'s handler treats `Enabled` as unconditionally-overwritten — passing the CURRENT value, not a stale default, is required to avoid accidentally flipping `enabled` off, per the pre-existing inconsistency flagged in this plan's research).
- Add `forwardSyncEnabled`/`backwardSyncEnabled`/`forwardSyncCloseLabel` to the `ItemSource` interface and `mapItemSource` (lines 38-47).
- Files: `web-app/src/lib/hooks/useBacklogSourcesService.ts`

##### Task 4.3.1b: Add the two toggles + label input + warning to the source row (~5 min)
- In `BacklogSourcesSettings.tsx`, inside the existing per-source row (near the `enabled` toggle, lines 143-149), add a "Sync with GitHub" sub-section:
  ```tsx
  <div className={styles.syncDirectionGroup}>
    <span className={styles.subHeading}>Sync with GitHub</span>
    <button role="switch" aria-checked={source.forwardSyncEnabled} ... aria-label={`${source.forwardSyncEnabled ? "Disable" : "Enable"} closing GitHub issues when done`} />
    <span>Close GitHub issues when I finish here</span>
    {source.forwardSyncEnabled && (
      <input type="text" placeholder="Label to apply on close (optional)" value={source.forwardSyncCloseLabel} onChange={...} />
    )}
    <button role="switch" aria-checked={source.backwardSyncEnabled} ... aria-label={`${source.backwardSyncEnabled ? "Disable" : "Enable"} reflecting GitHub status back`} />
    <span>Reflect GitHub status back here</span>
    {source.forwardSyncEnabled && source.backwardSyncEnabled && (
      <div className={styles.bothDirectionsWarning}>
        Both directions are enabled — closing this item's issue may be observed and re-applied by backward sync. Verify this doesn't create a loop for items you also edit manually.
      </div>
    )}
  </div>
  ```
- Files: `web-app/src/components/settings/BacklogSourcesSettings.tsx`

##### Task 4.3.1c: Add CSS tokens for the new elements (~2 min)
- Add `syncDirectionGroup`/`subHeading`/`bothDirectionsWarning` to `BacklogSourcesSettings.css.ts`, following `errorMessage`'s existing token pattern for the warning (same visual weight, per pitfalls/UX research recommendation — reuse the warning color token, don't invent a new one).
- Files: `web-app/src/components/settings/BacklogSourcesSettings.css.ts`

##### Task 4.3.1d: Add tests (~5 min)
- In `BacklogSourcesSettings.test.tsx`, add: toggles render with correct `aria-checked` from fetched state; clicking each toggle calls the corresponding hook function; the both-directions warning appears only when both are `true`.
- Files: `web-app/src/components/settings/BacklogSourcesSettings.test.tsx`

##### Task 4.3.2: Run frontend tests for Epic 4.3 (~2 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogSourcesSettings"`
- Files: none (verification only)

#### Story 4.3.2: Persistent row-level warning for non-transient sync failures
**Acceptance Criteria** (per UX research §4 — extends AC5's error-visibility expectations):
- *Given* a source's most recent sync history entry has an `errorMessage` indicating an auth failure (e.g. containing "401"/"403"/"revoked"), *When* the source row renders (without needing to expand history), *Then* a persistent warning affordance appears next to `source.displayName` (same visual weight as the existing top-level `lastError` banner, scoped to the row).
**Files**: `web-app/src/components/settings/BacklogSourcesSettings.tsx`, `web-app/src/components/settings/BacklogSourcesSettings.test.tsx`

##### Task 4.3.2a: Add the row-level warning affordance (~4 min)
- In `BacklogSourcesSettings.tsx`'s `listItemHeader` (lines 140-157), add a conditional warning icon/text next to `source.displayName` when the most recent sync event (already fetched into `historyBySource`, or a lightweight "has recent auth-type error" flag threaded from the backend — verify whether this requires a new field on the `ItemSource` proto response or can be derived client-side from already-fetched history) indicates a non-transient failure.
- Files: `web-app/src/components/settings/BacklogSourcesSettings.tsx`

##### Task 4.3.2b: Add a test (~3 min)
- Add a test asserting the warning renders when the fixture's sync history contains an auth-type `errorMessage`.
- Files: `web-app/src/components/settings/BacklogSourcesSettings.test.tsx`

##### Task 4.3.3: Run frontend tests for Story 4.3.2 (~2 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogSourcesSettings"`
- Files: none (verification only)

---

## Phase 5: Final Verification

### Epic 5.1: Full build/test pass + feature registry

#### Story 5.1.1: `make build && make test`, `make ci`, registry update
**Acceptance Criteria**:
- *Given* the complete implementation, *When* `make build && make test` runs from repo root, *Then* it exits 0, with no compile errors in `session/ent/*` (confirming `--feature sql/upsert` was preserved through every regeneration in Phases 0-2).
- *Given* two new RPCs' worth of proto fields and several new/changed frontend components, *When* `make registry-generate` runs, *Then* the per-feature JSON files under `docs/registry/features/` reflect the new `ForwardSyncEnabled`/`BackwardSyncEnabled` settings and the new `SourceSection`/badge UI, and `docs/registry/coverage-gaps.json`'s count does not grow without justification (per `.claude/rules/feature-registry.md`).
**Files**: none (verification), `docs/registry/features/**` (generated diffs to review and commit)

##### Task 5.1.1a: Run the full build and test suite (~5 min)
- Run `make build && make test`. Diagnose and fix any fallout at its root cause (per CLAUDE.md engineering discipline), not by guessing broadly.
- Files: whichever file(s) a failure implicates

##### Task 5.1.1b: Run `make lint` and `make quick-check` (~4 min)
- Run `make quick-check` (build + test + lint) as the fast pre-push gate.
- Files: whichever file(s) a lint failure implicates

##### Task 5.1.1c: Run `make registry-generate` and commit changed registry files (~3 min)
- Run `make registry-generate`, review the diff, commit changed per-feature JSON files.
- Files: `docs/registry/features/backend/*.json`, `docs/registry/features/frontend/*.json`

##### Task 5.1.1d: Run the e2e suite for touched surfaces (~5 min)
- Run `cd tests/e2e && npx playwright test` (or a targeted subset covering `BacklogSourcesSettings`/card/detail provenance, if a dedicated spec is added — note: no numbered AC in requirements.md mandates a new e2e spec; existing Jest component tests plus Go integration tests are the primary coverage for this plan. Adding an e2e spec for the two new Settings toggles is a reasonable proportional addition — flagged as optional, not blocking, given none of AC0-AC7 explicitly require e2e coverage).
- Files: none (verification only), or a new `tests/e2e/backlog-github-sync-settings.spec.ts` if added

##### Task 5.1.2: Run `make ci` as the definitive pre-push check (~5 min)
- Run `make ci` from repo root; fix any remaining fallout.
- Files: whichever file(s) a failure implicates
