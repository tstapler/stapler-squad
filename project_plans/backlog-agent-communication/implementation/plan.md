# Implementation Plan: backlog-agent-communication

**Feature**: Enrich backlog MCP tooling for structured agent-to-agent handoff, distinct
infra-bug reporting, agent-initiated "ask for help" escalation, and a review-verdict
dispute path — all composing with the existing `StuckReason`/`RemediationDue`/
`Notifier`/`/unfinished` machinery rather than duplicating it.
**Date**: 2026-07-23
**Status**: Ready for implementation
**ADRs**: ADR-001 (agent-initiated StuckReason rows), ADR-002 (global InfraIssueReport
entity), ADR-003 (defer persistent Master agent; use headless.Pool as extension seam)

---

## Dependency Visualization

```
Epic 1 (forward handoff)  ─┐
Epic 2 (structured findings)├──► Epic 6 (verdict dispute) ──► Epic 7 (UI surfacing)
Epic 3 (PR metadata fix)  ─┘                                        ▲
Epic 4 (infra-bug reporting) ───────────────────────────────────────┤
Epic 5 (ask-for-help escalation, reuses StuckReason like Epic 6) ────┘

Epic 1, 2, 3, 4 are independent of each other — can implement/ship in any order.
Epic 5 and Epic 6 both extend StuckReason with agent-initiated rows (same new
pattern, ADR-001) — implement Epic 5 first since it's the simpler case, reuse its
review-tested plumbing for Epic 6.
Epic 7 depends on whichever of Epics 4/5/6 introduce new human-visible state
(all three) — do UI work per-epic as each lands, not as one big deferred pass.
```

---

## Phase 1: Structured Agent-to-Agent Handoff

### Epic 1.1: Forward context from work session to review session
**Goal**: Give the work-session agent a structured (not just freeform-prose) way to
flag known limitations and review focus areas, so the reviewer's prompt is built
from typed data instead of parsing prose out of `verification_notes`.

#### Story 1.1.1: Extend `request_review` with structured handoff fields
**As a** work-session agent, **I want** to report known limitations and specific
files/areas needing extra reviewer scrutiny as structured fields, **so that** the
review session's prompt surfaces them distinctly instead of buried in freeform text.
**Acceptance Criteria**:
- `request_review`'s MCP schema gains two new optional fields:
  `known_limitations` (string, ≤2000 chars) and `review_focus_areas` (array of
  strings, each ≤200 chars, max 10 entries).
- Both fields are persisted on the calling session's `ItemSession` row (new
  `handoff_context` JSON column, typed per the `AcCriteriaJSON` pattern:
  `HandoffContextJSON` + `Parse()`/`Serialize()`).
- `BuildReviewPrompt` (in `server/mcp/tools_backlog.go`) renders `known_limitations`
  and `review_focus_areas` as distinct, labeled sections in the review session's
  system prompt (not merged into `verification_notes`'s existing block).
- Persisting `handoff_context` is best-effort (log-and-continue on failure) —
  matches the existing `verification_notes`/`AppendProgressNote` secondary-data
  discipline; it must never block or fail the `request_review` call itself.
**Files**: `session/ent/schema/item_session.go`, `session/domain/backlog.go`
(new `HandoffContextJSON` type), `session/storage_backlog.go` (persist call),
`server/mcp/tools_backlog.go` (`request_review` schema + `BuildReviewPrompt`).

##### Task 1.1.1a: Add `HandoffContextJSON` domain type (~3 min)
- Add `HandoffContextJSON` string type + `HandoffContext` struct
  (`KnownLimitations string`, `ReviewFocusAreas []string`) + `Parse()`/
  `SerializeHandoffContext()` functions to `session/domain/backlog.go`, mirroring
  `AcCriteriaJSON`/`ParseAcCriteria` exactly.
- Files: `session/domain/backlog.go`

##### Task 1.1.1b: Add `handoff_context` ent column + regenerate (~3 min)
- Add `field.String("handoff_context").Optional().Comment("JSON HandoffContext — known limitations and reviewer focus areas reported via request_review")` to `ItemSession` schema.
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`.
- Files: `session/ent/schema/item_session.go`, generated `session/ent/*` files.

##### Task 1.1.1c: Wire `request_review` schema + persist call (~5 min)
- Add `known_limitations`/`review_focus_areas` to the `request_review` tool schema
  in `registerBacklogTools`; parse in `requestReview` handler; serialize via
  `SerializeHandoffContext`; add a `Storage.UpdateItemSessionHandoffContext`
  passthrough (mirrors `UpdateItemSessionVerificationNotes`); call it best-effort
  right after the existing `verification_notes` persist call.
- Files: `server/mcp/tools_backlog.go`, `session/storage_backlog.go`

##### Task 1.1.1d: Render in `BuildReviewPrompt` (~4 min)
- Parse `handoff_context` off the work `ItemSession` and add a labeled
  "Known limitations (reported by implementer)" / "Areas flagged for extra
  scrutiny" section, distinct from the existing verification-notes block.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.1.1e: Unit tests (~5 min)
- `TestRequestReview_should_PersistHandoffContext_When_FieldsProvided`,
  `TestBuildReviewPrompt_should_RenderKnownLimitations_When_HandoffContextSet`,
  `TestHandoffContextJSON_should_RoundTrip_When_Serialized`.
- Files: `server/mcp/tools_backlog_test.go`, `session/domain/backlog_test.go`

##### Task 1.1.1f: Surface handoff context on the item detail page for human reviewers too (~4 min)
- The original brief frames this as making it "easier for their reviewers to
  review" — reviewers are not always AI review sessions; the operator reviews
  items directly on the item detail page too. Render `known_limitations`/
  `review_focus_areas` there (read-only, alongside existing verification-notes
  display) so a human loses nothing this feature only gave to AI reviewers.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx` (or its nearest
  existing verification-notes rendering subcomponent — confirm exact file at
  implementation time)

---

## Phase 2: Structured Review Findings (backward channel)

### Epic 2.1: Typed findings on review verdicts
**Goal**: Let a review session report findings with a severity/category taxonomy
(reusing this repo's own `code-review` skill vocabulary) instead of only a
per-criterion evidence string, so a reworking agent — or a future dispute
adjudicator — gets machine-parseable findings, not just prose.

#### Story 2.1.1: Extend `submit_review_verdict` with structured findings
**As a** review-session agent, **I want** to attach severity-tagged, located
findings to my verdict, **so that** the next agent (work session doing rework, or a
human/re-reviewer adjudicating a dispute) can act on structured data.
**Acceptance Criteria**:
- `submit_review_verdict`'s `verdicts` array items gain an optional `findings`
  array: each finding has `severity` (`BLOCKER|CRITICAL|MAJOR|NIT` — the exact
  vocabulary this repo's `code-review` skill already uses), `file` (optional
  string), `line` (optional number), `description` (required string, ≤1000
  chars), `suggested_fix` (optional string, ≤1000 chars).
- Findings are serialized into a new `StructuredFindings` JSON field alongside the
  existing `PerCriterion` field on `ReviewVerdictData`, using the same
  `AcCriteriaJSON`-style typed-JSON pattern.
- `get_backlog_item`'s response includes the latest verdict's structured findings
  (not just the `summary` string) so a work session picking up rework sees them
  without re-deriving from prose.
- Empty `findings` is valid (backward compatible — existing callers unaffected).
**Files**: `session/domain/backlog.go` (new `ReviewFinding`/`FindingsJSON` type),
`server/mcp/tools_backlog.go` (`submit_review_verdict` schema + `submitReviewVerdict`
handler + `getBacklogItem` response), wherever `ReviewVerdictData` is persisted
(confirm exact ent location during implementation — not fully traced in research;
first implementation task must locate it before the rest proceeds).

##### Task 2.1.1a: Locate `ReviewVerdictData` persistence + add `StructuredFindings` field (~5 min)
- Find `SaveReviewVerdict`'s ent-backed implementation (likely
  `session/ent_repository_backlog.go` or a `reviewverdict.go` ent schema file —
  confirm exact file first); add a `structured_findings` column following the
  same JSON-string pattern as `handoff_context` (Task 1.1.1b).
- Files: locate via `grep -rn "SaveReviewVerdict" session/` as this task's first
  sub-step (confirmed in research to be called from
  `server/mcp/tools_backlog.go:505`, but its ent-backed implementation file was
  not traced to completion during planning — see `adversarial-review.md`'s
  Concerns section for the explicit risk this carries for this task's time
  estimate).

##### Task 2.1.1b: Add `ReviewFinding`/`FindingsJSON` domain type (~3 min)
- Mirrors `HandoffContextJSON`: `ReviewFinding{Severity, File, Line, Description,
  SuggestedFix}`, `FindingsJSON` string type, `Parse()`/`Serialize()`.
- Add `FindingSeverity` validated enum (`BLOCKER|CRITICAL|MAJOR|NIT`) with
  `IsValid()`, mirroring `ReviewOutcome`'s existing pattern.
- Files: `session/domain/backlog.go`

##### Task 2.1.1c: Wire `submit_review_verdict` schema + persist (~5 min)
- Extend the `verdicts` array item schema with `findings`; parse per-verdict
  findings in `submitReviewVerdict`, flatten to one `FindingsJSON` blob on
  `ReviewVerdictData`, persist via the column from Task 2.1.1a.
- Files: `server/mcp/tools_backlog.go`

##### Task 2.1.1d: Surface findings in `get_backlog_item` (~4 min)
- Extend `latestReviewVerdict`'s consumption / `getBacklogItem`'s response
  rendering to include parsed findings, grouped by severity.
- Files: `server/mcp/tools_backlog.go`

##### Task 2.1.1e: Unit tests (~5 min)
- `TestSubmitReviewVerdict_should_PersistStructuredFindings_When_FindingsProvided`,
  `TestGetBacklogItem_should_IncludeFindings_When_LatestVerdictHasThem`,
  `TestFindingsJSON_should_RoundTrip_When_Serialized`,
  `TestFindingSeverity_should_RejectUnknownValue_When_Validated`.
- Files: `server/mcp/tools_backlog_test.go`, `session/domain/backlog_test.go`

##### Task 2.1.1f: Surface structured findings on the item detail page (~4 min)
- Same rationale as Task 1.1.1f: render severity-grouped findings (with
  file/line/suggested_fix where present) on the item detail page, not only via
  `get_backlog_item` for agent consumption — a human deciding whether to trust an
  auto-shipped PASS, or reviewing a FAIL before it reworks, benefits from the
  same structure an agent gets.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx` (or nearest
  existing review-verdict rendering subcomponent — confirm exact file at
  implementation time)

---

## Phase 3: PR Metadata Capture Fix (pain point A)

### Epic 3.1: Agent-driven PR creation reports back to the item record
**Goal**: Close the confirmed gap — today only the *system-driven* mechanical push
path (`pushAndCreatePR`) writes `pr_url`/`pr_number` onto the item; when a live work
session drives shipping itself via `/backlog:ship` → `gh pr create`, there is no MCP
tool call that reports the resulting PR back, so the item can sit in `review` with a
real, unlinked PR until some other path notices. This is the direct root cause class
behind BUG-040/BUG-045's downstream symptoms.

#### Story 3.1.1: New `report_pr_created` MCP tool
**As a** work-session agent that just created a PR via `gh pr create` (or the
`github:pr-ship`/`/backlog:ship` flow), **I want** to report the PR's URL and number
back to the backlog item, **so that** the item record is never out of sync with a
PR that genuinely exists.
**Acceptance Criteria**:
- New tool `report_pr_created(item_id, pr_url, pr_number, summary)`, role: work
  only, following the exact caller-identity/link/role-guard pattern every existing
  backlog tool uses.
- The write of `pr_url`/`pr_number` onto the `BacklogItem` row is **primary, not
  best-effort** — a persistence failure returns an error to the caller (unlike
  `AppendProgressNote`'s secondary-data discipline) so the agent knows to retry,
  directly avoiding BUG-040 root cause #1's shape.
- On success, transitions the item `review → pr_pending` using the existing
  `TransitionBacklogItemStatus` + precondition machinery (same guard shape
  `pushAndCreatePR` already uses) — idempotent: if the item is already
  `pr_pending` with the same `pr_number`, returns success as a no-op rather than
  erroring.
- `summary` (required, ≤1000 chars: what changed and why) is persisted onto the
  item alongside the PR fields, so a reviewer (human or the mechanical PR-pending
  reconciler surfacing it) sees *why*, not just a bare link — this directly answers
  the "our tooling doesn't include the right points for the AI to report data
  about what they did" framing from the original brief.
- **The reported PR is verified against GitHub before being trusted, not persisted
  on the agent's word alone.** Unlike `pushAndCreatePR`'s mechanical path (which
  only ever writes PR data *it itself just created*, so it's inherently trustworthy),
  `report_pr_created` accepts a self-report from the agent — a hallucinated,
  stale, or mistyped `pr_number`/`pr_url` would otherwise silently poison the item
  record with a wrong reference, a *new* class of bad data BUG-040 never had to
  guard against (BUG-040 was about a *real* PR's reference being lost, not a fake
  one being trusted). The handler calls the GitHub API (reusing `tools_github.go`'s
  existing PR-lookup plumbing, same credential path `list_github_prs` already
  uses) to confirm: the PR exists, its number matches the reported `pr_url`, and
  its head branch matches the item's own branch (from the calling session's
  worktree/branch record) before writing anything. On mismatch or lookup failure,
  the tool returns an error (does not persist) and the agent must correct itself
  or retry — this verification is itself best-effort-tolerant of transient GitHub
  API errors (return an error asking the agent to retry) but never tolerant of a
  confirmed mismatch.
- `server/mcp/tools_backlog.go`'s role-instruction text (`sb.WriteString(...)` in
  the work-role branch) is updated to instruct agents driving their own shipping to
  call this tool as the final step, alongside the existing `/backlog:ship` skill
  guidance in `session/backlog_commands.go`.
**Files**: `server/mcp/tools_backlog.go`, `session/storage_backlog.go`,
`session/backlog_commands.go` (skill file guidance), `docs/registry/features/backend/`
(new registry entry per `.claude/rules/feature-registry.md`).

##### Task 3.1.1a: Add `reportPRCreated` handler + primary-write storage method (~5 min)
- New `Storage.SetBacklogItemPRAndTransition(ctx, itemID, prURL, prNumber, summary)`
  — blocking write, returns error on failure (no `log.Warning`-and-continue).
- Files: `session/storage_backlog.go`

##### Task 3.1.1b: Register `report_pr_created` tool + handler (~5 min)
- Follow the exact `requestReview` handler shape (caller UUID, item link, role
  check, arg validation, idempotency check against current item state).
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1c: Update role-instruction text + `/backlog:ship` skill guidance (~3 min)
- Files: `server/mcp/tools_backlog.go` (role instructions), `session/backlog_commands.go`

##### Task 3.1.1d: Feature registry entry (~2 min)
- Create `docs/registry/features/backend/report-pr-created.json` per
  `.claude/rules/feature-registry.md`; run `make registry-generate`.
- Files: `docs/registry/features/backend/report-pr-created.json`

##### Task 3.1.1e: GitHub verification step before persisting (~5 min)
- Extract a plain-Go `VerifyPRMatchesBranch(ctx, owner, repo, prNumber, expectedBranch)
  (bool, error)` helper from `tools_github.go`'s existing lookup plumbing; call it
  from `reportPRCreated` before Task 3.1.1a's write. Mismatch or a definitive
  "PR not found" → `ErrInvalidArgument` (no persist). A transient API error
  (rate limit, network) → `ErrInternalError` asking the agent to retry, never a
  silent skip-verification fallback.
- Files: `server/mcp/tools_github.go`, `server/mcp/tools_backlog.go`

##### Task 3.1.1f: Unit tests (~5 min)
- `TestReportPRCreated_should_TransitionToPRPending_When_ValidPR`,
  `TestReportPRCreated_should_ReturnError_When_PersistFails` (mirrors BUG-040's
  own `TestPushAndCreatePR_PRFieldsPersistFails_StaysInReview_AndNotifies` test
  shape, but asserting the tool call itself errors rather than silently
  succeeding), `TestReportPRCreated_should_NoOp_When_AlreadyPRPendingSamePR`,
  `TestReportPRCreated_should_RejectCall_When_CallerRoleNotWork`,
  `TestReportPRCreated_should_RejectCall_When_BranchMismatch` (new: the
  GitHub-verification regression test for this review's Task 3.1.1e),
  `TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails`.
- Files: `server/mcp/tools_backlog_test.go`

### Epic 3.2: Reconciliation safety net for agent-driven PRs that never got reported
**Goal**: Even with Epic 3.1, an agent can still crash/be killed after `gh pr
create` but before calling `report_pr_created` — provide a backstop, composing
with the existing `StuckReason`/reconciler pattern rather than a new one.

#### Story 3.2.1: Detect "review status, no live session, but a GitHub PR exists for this branch"
**As the** system, **I want** to detect items that are stuck in `review` with a
real unreported PR, **so that** they self-heal instead of silently stalling until
`StuckReasonAbandonedReview`'s much coarser detector eventually catches them.
**Acceptance Criteria**:
- New periodic detector (added to `ReconcileStuck`'s detector list, same pattern
  as `reconcilePRPendingWithoutPRItems`): for `review`-status items with no live
  work session, query GitHub for an open PR matching the item's branch (reuses
  `list_github_prs`'s underlying lookup, exposed as a plain Go function rather
  than only the MCP tool wrapper).
- On a match, calls the same `SetBacklogItemPRAndTransition` primary-write path
  from Epic 3.1 — no duplicate PR-field-writing logic.
- On no match, no action (this is a backstop, not a new stuck reason — an item in
  `review` with no PR and no live session is already covered by
  `StuckReasonAbandonedReview`).
**Files**: `session/backlog_lifecycle.go` (new detector function + `ReconcileStuck`
wiring), `server/mcp/tools_github.go` (extract reusable lookup function if not
already plain-Go-callable).

##### Task 3.2.1a: Extract reusable branch→PR lookup (~4 min)
- Files: `server/mcp/tools_github.go`

##### Task 3.2.1b: New detector + `ReconcileStuck` wiring (~5 min)
- Files: `session/backlog_lifecycle.go`

##### Task 3.2.1c: Regression test mirroring BUG-040's test shape (~5 min)
- `TestReconcileOrphanedAgentPRs_should_LinkPR_When_ReviewStatusNoLiveSessionPRExists`,
  `TestReconcileOrphanedAgentPRs_should_NoOp_When_NoMatchingPR`.
- Files: `session/backlog_lifecycle_test.go`

---

## Phase 4: Infra/Orchestrator Bug Reporting (dimension 2)

### Epic 4.1: A distinct, low-friction path for "the tooling is broken," not item-scoped
**Goal**: `StuckReason`/`BacklogStuckState` is structurally item-anchored
(`item_id` required) — not a fit for systemic reports. New, deliberately separate
(but equally durable and equally visible) machinery per ADR-002.

#### Story 4.1.1: New `report_infra_issue` MCP tool + `InfraIssueReport` entity
**As any** pipeline session (triage/work/review), **I want** to report that the
orchestrator/tooling itself is broken — distinct from my assigned item failing —
**so that** it gets triaged with appropriate urgency and doesn't get lost as a
footnote on whichever item happened to be running.
**Acceptance Criteria**:
- New tool `report_infra_issue(category, severity, description, related_item_id?,
  suggested_fix?)`. `category` enum:
  `orchestrator_bug|mcp_tool_error|reconciler_stuck|confusing_tool_behavior|other`.
  `severity` enum: `low|medium|high|critical`. Available to any session role (no
  role guard) — infra problems aren't role-scoped.
- New ent entity `InfraIssueReport` (new schema file
  `session/ent/schema/infraissuereport.go`), mirroring `BacklogProgressNote`'s
  shape: `id`, `category`, `severity`, `description`, `suggested_fix` (optional),
  `related_item_id` (optional, no required FK — a systemic report may have no
  single owning item), `reporter_session_uuid`, `status`
  (`open|acknowledged|resolved`), `occurrence_count` (default 1, incremented by
  the dedup path below), `last_occurred_at`, `created_at` immutable. Indexed on
  `(status, created_at)` for the "open reports, newest first" query and on
  `(category, created_at)` for the dedup lookup.
- On `severity=high|critical`, fires `Notifier.Notify` immediately at HIGH
  priority; `low|medium` do not push-notify (surfaces only in the durable list —
  see Story 4.1.2), per the pitfalls research's alert-fatigue guidance.
- **Deduplication cooldown**: before creating a new row (and before any
  notification), checks for an existing `open` report with the same `category`
  and a `description` similarity match (simple exact-match or truncated-prefix
  match is sufficient for MVP — no need for fuzzy matching) created within the
  last hour; if found, increments an `occurrence_count` on the existing row
  instead of creating a new one and does not re-notify. Directly addresses the
  "an agent hitting the same broken tool repeatedly floods the list/notifications"
  pitfall named in `research/pitfalls.md` — a crash-looping session that reports
  the same infra problem every retry must not degrade the signal for genuinely
  distinct reports.
- Tool description explicitly distinguishes this from `report_progress`
  (item-specific work status) and `request_help` (Epic 5, "I personally am stuck
  on my assigned task") — three-way disambiguation in each tool's own description
  text so an agent picks the right one without needing external docs.
**Files**: `session/ent/schema/infraissuereport.go`, `session/storage_infra.go`
(new file, mirrors `storage_backlog.go`'s CRUD shape), `server/mcp/tools_backlog.go`
or a new `server/mcp/tools_infra.go` (884-line `tools_backlog.go` is already the
largest tool file — split this into its own file per the stack research's file-size
observation).

##### Task 4.1.1a: `InfraIssueReport` ent schema (~4 min)
- Files: `session/ent/schema/infraissuereport.go`, regenerate.

##### Task 4.1.1b: `session/storage_infra.go` CRUD (Create, ListOpen, Acknowledge, Resolve) (~5 min)
- Files: `session/storage_infra.go`

##### Task 4.1.1c: `server/mcp/tools_infra.go` — register `report_infra_issue` (~5 min)
- Files: `server/mcp/tools_infra.go` (new)

##### Task 4.1.1d: Notifier wiring for high/critical severity (~3 min)
- Files: `server/mcp/tools_infra.go`

##### Task 4.1.1e: Dedup-cooldown check before create/notify (~4 min)
- Files: `session/storage_infra.go`, `server/mcp/tools_infra.go`

##### Task 4.1.1f: Unit tests (~5 min)
- `TestReportInfraIssue_should_CreateOpenReport_When_ValidInput`,
  `TestReportInfraIssue_should_Notify_When_SeverityHighOrCritical`,
  `TestReportInfraIssue_should_NotNotify_When_SeverityLowOrMedium`,
  `TestReportInfraIssue_should_IncrementOccurrenceCount_When_DuplicateWithinCooldown`,
  `TestReportInfraIssue_should_CreateNewRow_When_SameCategoryPastCooldown`.
- Files: `server/mcp/tools_infra_test.go`

#### Story 4.1.2: Human-visible surface for open infra issue reports
**As the** operator, **I want** to see open infra/tooling reports in the same
triage flow as stuck backlog items, **so that** I don't need a separate place to
check.
**Acceptance Criteria**:
- New ConnectRPC methods `ListInfraIssueReports`, `AcknowledgeInfraIssueReport`,
  `ResolveInfraIssueReport` (proto in `proto/session/v1/backlog.proto`, following
  the existing `ListStuckBacklogItems`/`SnoozeStuckItem` pattern).
- New section on the `/unfinished` page (`web-app/src/components/backlog-stuck/`)
  — "Infra & Tooling Reports" — listing open reports with category/severity
  badges and Acknowledge/Resolve actions, visually distinct from (not merged
  into) the existing per-item `StuckReason` cards, since these are not
  per-item.
- E2e test per `.claude/rules/e2e-test-conventions.md` (feature annotation,
  `data-testid` locators, no `waitForTimeout`).
**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_infra.go`
(new), `web-app/src/components/backlog-stuck/InfraIssueReports.tsx` (new, with
colocated `InfraIssueReports.css.ts` per `.claude/rules/css-architecture.md`),
`tests/e2e/infra-issue-reports.spec.ts` (new).

##### Task 4.1.2a: Proto additions + `make proto-gen` (~4 min)
##### Task 4.1.2b: `server/services/backlog_service_infra.go` RPC handlers (~5 min)
##### Task 4.1.2c: `InfraIssueReports.tsx` + `.css.ts` component (~5 min)
##### Task 4.1.2d: Wire into `/unfinished` page (~3 min)
##### Task 4.1.2e: E2e test + feature registry entries (~5 min)
(File paths as listed in Story 4.1.2's Files line; each task touches a subset.)

---

## Phase 5: Ask-for-Help Escalation (dimension 3)

### Epic 5.1: Agent-initiated "I am genuinely stuck" signal
**Goal**: The existing `StuckReason` system is entirely reconciler-detected; this
epic adds the *first* agent-initiated stuck row, composing with — not duplicating —
`MarkStuck`/`RemediationDue`/`/unfinished` (ADR-001). Deliberately does **not**
build a persistent "Master agent" process (ADR-003) — reuses `headless.Pool` as a
documented future extension seam instead.

#### Story 5.1.1: New `request_help` MCP tool + `StuckReasonHelpRequested`
**As a** pipeline session that is genuinely blocked (not facing a normal retryable
failure), **I want** to explicitly signal that I need human help, with what I've
already tried, **so that** a human sees it promptly instead of the item silently
bouncing through automated retries for hours.
**Acceptance Criteria**:
- New tool `request_help(item_id, reason, attempted_remediation, urgency)`.
  `reason` (required, ≤1000 chars: what's blocking you) and
  `attempted_remediation` (required, ≤1000 chars: what you already tried) are both
  **required, not optional** — directly addresses the "escalation without content"
  and "escalation as a crutch" pitfalls by making the tool schema itself enforce
  evidence of a prior attempt. `urgency` enum `normal|high`.
- Calling it invokes `Storage.MarkStuck(ctx, itemID, domain.StuckReasonHelpRequested,
  ...)` — the exact same durable-row primitive every reconciler-detected reason
  already uses — with `reason`/`attempted_remediation` stored as the row's note
  field (reuses `BacklogStuckState`'s existing note column, no new column needed).
- Fires `Notifier.Notify` immediately at HIGH priority (bypasses the periodic
  sweep entirely — this is voluntary and time-sensitive, unlike reconciler-detected
  reasons that wait for the next tick by design).
- A second `request_help` call for the same item while a `StuckReasonHelpRequested`
  row is already open is rejected with a clear error (`"help already requested for
  this item — see /unfinished"`) — prevents notification spam from repeated calls
  in a retry loop.
- Explicitly does **not** go through `RemediationDue`'s backoff gate — that gate
  is for *automated remediation attempts*, and this is a one-shot human signal, not
  a retry action. Row resolution requires a human action (`RespondToHelpRequest`,
  Story 5.1.2), not automatic self-heal on item-status-change, since the point is a
  human must actually see and respond to `reason`/`attempted_remediation` before
  the signal should clear.
**Files**: `session/domain/backlog.go` (`StuckReasonHelpRequested` constant +
`AllStuckReasons`/`IsValid`), `server/mcp/tools_backlog.go` or `tools_infra.go`,
`proto/session/v1/backlog.proto` (new stuck-reason enum value),
`server/services/backlog_service_stuck.go` (proto mapping),
`web-app/src/components/backlog-stuck/stuckReason.ts` (exhaustive map entry).

##### Task 5.1.1a: `StuckReasonHelpRequested` domain constant (~2 min)
- Files: `session/domain/backlog.go`

##### Task 5.1.1b: Proto enum value + regen + backend/frontend mapping (~5 min)
- Follow BUG-040's exact checklist (new enum value in
  `proto/session/v1/backlog.proto`, `make proto-gen`,
  `toProtoStuckReason`/`fromProtoStuckReason` in
  `server/services/backlog_service_stuck.go`, `stuckReason.ts`'s exhaustive
  `Record<StuckReason, T>` maps + `stuckReason.css.ts` icon/label/class entries).
- Files: `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`,
  `web-app/src/components/backlog-stuck/stuckReason.ts`,
  `web-app/src/components/backlog-stuck/stuckReason.css.ts`

##### Task 5.1.1c: `request_help` tool handler (~5 min)
- Files: `server/mcp/tools_backlog.go`

##### Task 5.1.1d: Duplicate-call rejection + Notifier wiring (~3 min)
- Files: `server/mcp/tools_backlog.go`

##### Task 5.1.1e: Unit tests (~5 min)
- `TestRequestHelp_should_MarkItemStuck_When_ValidCall`,
  `TestRequestHelp_should_RejectDuplicateCall_When_RowAlreadyOpen`,
  `TestRequestHelp_should_NotifyImmediately_When_Called` (unlike reconciler
  reasons — asserts no dependency on a periodic sweep tick),
  `TestToProtoStuckReason_should_MapHelpRequested_When_RoundTripped` (mirrors
  BUG-040's exhaustiveness test extension pattern).
- Files: `server/mcp/tools_backlog_test.go`, `server/services/backlog_stuck_rpc_test.go`

#### Story 5.1.2: Human "respond" action — distinct from retry/reset/snooze
**As the** operator, **I want** to see the agent's stated reason and attempted
remediation and respond with guidance, **so that** the agent can act on my answer
instead of the row just closing itself.
**Acceptance Criteria**:
- New ConnectRPC `RespondToHelpRequest(item_id, response_text, resume_session)` —
  writes the response onto the `BacklogStuckState` row (or a small linked note),
  resolves the row via the existing `selfHealStuck`-style path, and — critically —
  guarantees `response_text` actually reaches a live agent, not just a database
  column a session might never read again:
  - If a work session is still live for the item (`SessionLivenessChecker`, the
    same primitive `BacklogLifecycleListener` already uses elsewhere), the
    response is delivered directly into that session via the existing
    `write_to_session`/steer-session mechanism (`mcp__stapler-squad__
    write_to_session` — already used for operator-to-agent injection elsewhere in
    this codebase) rather than only being polled for.
  - If no work session is live (the common case — the agent that filed
    `request_help` may have exited or been genuinely blocked with nothing left to
    poll), `RespondToHelpRequest` accepts a `resume_session: bool` flag: when true,
    it spawns a fresh work session for the item (reusing the same
    `ReviewGateSpawner`/`headless.Pool` session-creation path used elsewhere),
    seeding its initial prompt with `response_text` plus the original
    `reason`/`attempted_remediation` context. This closes the gap a purely
    passive "surfaced via next `get_backlog_item` call" design would leave open:
    without an explicit resume trigger, a response could sit persisted and
    correct but never actually delivered to any agent, defeating the entire
    point of the escalation.
  - Regardless of delivery path, `response_text` is *also* persisted and surfaced
    via `get_backlog_item`'s role-instruction block (same channel
    `HandoffContext`/findings use) as a durable fallback, in case delivery
    timing races a session's own exit.
- `/unfinished` renders `StuckReasonHelpRequested` cards with a text-input
  "Respond" action (with a "resume session" checkbox, default on) — distinct from
  the existing Retry/Reset/Snooze buttons other reasons show (this reason has no
  automated remediation action to retry).
- E2e test per `.claude/rules/e2e-test-conventions.md`.
**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`,
`session/storage_backlog.go` (persist response, expose to next session context),
`web-app/src/components/backlog-stuck/HelpRequestCard.tsx` (new),
`tests/e2e/help-request-response.spec.ts` (new).

##### Task 5.1.2a: `RespondToHelpRequest` RPC (~5 min)
##### Task 5.1.2b: Live-session delivery via `write_to_session` when a work session is still live (~5 min)
##### Task 5.1.2c: Session-resume/respawn path when no live session exists (~5 min)
##### Task 5.1.2d: Persist response + surface via `get_backlog_item` as durable fallback (~4 min)
##### Task 5.1.2e: `HelpRequestCard.tsx` UI with resume-session option (~5 min)
##### Task 5.1.2f: Unit tests — `TestRespondToHelpRequest_should_DeliverToLiveSession_When_SessionStillRunning`,
`TestRespondToHelpRequest_should_SpawnFreshSession_When_NoLiveSessionAndResumeTrue`,
`TestRespondToHelpRequest_should_OnlyPersist_When_NoLiveSessionAndResumeFalse` (~5 min)
##### Task 5.1.2g: E2e test + registry entries (~5 min)

### Epic 5.2 (explicitly deferred — documented, not built): "Master agent" triage spawn
Per ADR-003: do **not** build a persistent always-on "Master agent" process in this
plan. If future need justifies it, the extension seam is a `headless.Pool`-spawned
short-lived "triage-the-escalation" session, invoked from `request_help`'s handler
the same way review sessions are spawned today — structurally straightforward, but
out of scope for this MVP per the low-operational-overhead constraint. No tasks
in this phase; this is a documented decision, not a story.

---

## Phase 6: Verdict Dispute Path (pain point B)

### Epic 6.1: Formal dispute of a FAIL/PARTIAL/UNVERIFIABLE verdict
**Goal**: Give the implementer agent a real alternative to silently reworking on a
verdict it believes is wrong, using the same agent-initiated-`StuckReason` pattern
Epic 5 establishes (ADR-001), and explicitly accounting for BUG-045's live risk to
any re-review-based adjudication path.

#### Story 6.1.1: New `dispute_review_verdict` MCP tool + `VerdictDispute` entity
**As a** work-session agent that has just discovered (via polling
`get_backlog_item`) a FAIL/PARTIAL/UNVERIFIABLE verdict it believes is incorrect,
**I want** to formally dispute it with reasoning, **so that** a human — or a
carefully-scoped fresh re-review — adjudicates it instead of the item silently
entering `autoReopenWithBackoffGate`'s rework loop on a possibly-false premise.
**Acceptance Criteria**:
- New tool `dispute_review_verdict(item_id, disputed_criteria[], reasoning)`, role:
  work only, callable only when the item's latest verdict is
  FAIL/PARTIAL/UNVERIFIABLE and no dispute is already open for that specific
  verdict (tracked by `item_session_id` of the disputed review).
- `reasoning` required (≤2000 chars: why the agent believes the verdict is wrong,
  citing specific evidence). `disputed_criteria` (array of criterion indices) —
  a dispute may target a subset of criteria, not necessarily the whole verdict.
- New `VerdictDispute` ent entity (mirrors `BacklogProgressNote`'s append-only
  shape): `id`, `item_id`, `disputed_item_session_id` (the review session's
  `ItemSession.id`), `disputed_criteria` (JSON int array), `reasoning`,
  `status` (`open|upheld|overturned|reassigned_for_rereview`), `adjudicator_note`
  (optional), `created_at`, `resolved_at`.
- Calling this tool **pauses** `autoReopenWithBackoffGate`'s effect: on a
  successful dispute call, marks the item stuck via `domain.StuckReasonVerdictDisputed`
  (same `MarkStuck` primitive as Epic 5) — the existing FAIL-verdict rework path
  becomes conditional: `handleReviewSessionExited`'s FAIL/PARTIAL/UNVERIFIABLE
  branch must check for an open, unresolved `VerdictDispute` on the just-exited
  review session **before** calling `autoReopenWithBackoffGate`, and skip the
  auto-reopen if one exists (the dispute IS the "what happens next," not a
  parallel track).

  **Design note**: because a `VerdictDispute` can only be filed *after* a verdict
  already exists (i.e., after `handleReviewSessionExited` has already run its
  FAIL branch once), the practical sequence is: review exits → FAIL branch fires
  → (if work session was live) the live work session discovers FAIL via polling,
  and — instead of immediately reworking, per updated `taskProtocolBlock`
  guidance in `session/backlog_context.go` — calls `dispute_review_verdict`
  first if it disagrees. `autoReopenWithBackoffGate` has already run by then in
  the has-no-live-work-session case; the "pause auto-reopen" acceptance criterion
  above therefore applies specifically to the **respawned rework session** that
  `AutoReopenAfterFailedReview` would otherwise create — this plan adds a check in
  `AutoReopenAfterFailedReview` itself (not `handleReviewSessionExited`) to no-op
  if an open `VerdictDispute` already exists for the item, mirroring the existing
  `hasActiveWorkSession` guard already in that function.
- Hard cap: max 2 disputes total per item (lifetime counter on `BacklogItem`, new
  optional field) — after the cap, `dispute_review_verdict` returns an error
  instructing the agent to proceed with the human-adjudicated path only (mirrors
  `MaxRemediationAttempts`'s discipline, addresses the dispute-loop-abuse pitfall).
**Files**: `session/ent/schema/verdictdispute.go` (new), `session/domain/backlog.go`
(`StuckReasonVerdictDisputed` constant), `session/backlog_lifecycle.go`
(`AutoReopenAfterFailedReview`'s new guard), `server/mcp/tools_backlog.go`
(new tool), `session/backlog_context.go` (`taskProtocolBlock` guidance update),
proto + frontend mapping (same checklist as Story 5.1.1's Task b).

##### Task 6.1.1a: `VerdictDispute` ent schema + `BacklogItem.dispute_count` field (~5 min)
##### Task 6.1.1b: `StuckReasonVerdictDisputed` + proto/frontend mapping (~5 min, same checklist as 5.1.1b)
##### Task 6.1.1c: `dispute_review_verdict` tool handler + cap enforcement (~5 min)
##### Task 6.1.1d: `AutoReopenAfterFailedReview` open-dispute guard (~4 min)
##### Task 6.1.1e: `taskProtocolBlock` guidance update (~3 min)
##### Task 6.1.1f: Unit tests (~5 min)
- `TestDisputeReviewVerdict_should_CreateOpenDispute_When_LatestVerdictIsFail`,
  `TestDisputeReviewVerdict_should_Reject_When_VerdictIsPass`,
  `TestDisputeReviewVerdict_should_Reject_When_CapExceeded`,
  `TestAutoReopenAfterFailedReview_should_NoOp_When_OpenDisputeExists` (mirrors
  the existing `hasActiveWorkSession` guard's test shape).
- Files: `server/mcp/tools_backlog_test.go`, `session/backlog_lifecycle_test.go`

#### Story 6.1.2: Human adjudication of disputes
**As the** operator, **I want** to see disputed verdicts with both the reviewer's
and implementer's framing side by side, and choose an outcome, **so that** the
implementer's side is never silently discarded (the exact gap named in the brief).
**Acceptance Criteria**:
- New ConnectRPC `AdjudicateDispute(item_id, dispute_id, outcome, adjudicator_note)`
  — `outcome` one of `uphold` (proceed to normal rework — resolves the dispute row
  and lets `AutoReopenAfterFailedReview` run), `overturn` (marks the disputed
  criteria PASS and re-evaluates whether the item can proceed toward `pr_pending`,
  reusing `applyVerdictsToACs`'s existing machinery), `request_rereview` (spawns a
  fresh review session **only if the item's worktree still exists** — if it
  doesn't, the RPC returns a specific error explaining the worktree is gone and
  only `uphold`/`overturn` are available, a direct, explicit mitigation for
  BUG-045's live risk rather than silently running a fresh review against the
  wrong codebase state).
- `/unfinished` and the item detail page render `StuckReasonVerdictDisputed`
  cards showing both the original verdict's `summary`/findings (Epic 2.1) and the
  dispute's `reasoning` together, with the three adjudication actions.
- E2e test per `.claude/rules/e2e-test-conventions.md`.
**Files**: `proto/session/v1/backlog.proto`,
`server/services/backlog_service_stuck.go` (or new `backlog_service_dispute.go`),
`web-app/src/components/backlog-stuck/DisputeCard.tsx` (new),
`tests/e2e/verdict-dispute.spec.ts` (new).

##### Task 6.1.2a: `AdjudicateDispute` RPC + `uphold`/`overturn` outcomes (~5 min)
##### Task 6.1.2b: `request_rereview` outcome + BUG-045 worktree-existence guard (~5 min)
- Explicitly checks worktree existence via the same `GetWorktreeDataBySessionUUID`
  + `os.Stat` pattern BUG-045's own root-cause code uses, refusing (not silently
  falling back) when absent — the opposite of BUG-045's current bug shape.
##### Task 6.1.2c: `DisputeCard.tsx` UI (~5 min)
##### Task 6.1.2d: E2e test + registry entries (~5 min)

---

## Phase 7: Cross-Cutting Human-in-the-Loop Verification

### Epic 7.1: Notification-volume review
**Goal**: Verify the four new signal sources (infra reports, help requests,
disputes, plus existing ones) don't collectively degrade the existing notification
system's usefulness for a solo operator.

#### Story 7.1.1: Audit new `Notifier.Notify` call sites against existing volume
**As the** operator, **I want** confidence the new signals won't flood
notifications, **so that** I don't start ignoring them the way alert fatigue
degrades any paging system.
**Acceptance Criteria**:
- A short written audit (added to this plan's implementation notes, not a
  separate artifact) listing every new `Notifier.Notify` call site introduced by
  Epics 4–6, its priority, and its expected frequency ceiling (e.g.
  `request_help` is capped at once-per-item via the duplicate-rejection guard in
  Story 5.1.1; `report_infra_issue` low/medium severities never push-notify).
- Confirms no new call site fires on every reconciler tick (all are one-shot,
  event-triggered, matching the existing `notify-once` discipline
  `reconcilePlanNotApprovedItems`/`reconcilePRPendingWithoutPRItems` already
  established).
**Files**: none (verification task, folded into code review for Epics 4–6).

##### Task 7.1.1a: Write the audit as part of Epic 4/5/6 code review (~5 min)

---

## Cross-Reference: Requirements → Plan Coverage

| Requirement dimension | Epic(s) |
|---|---|
| 1. Structured forward/backward handoff | Epic 1.1, Epic 2.1 |
| 2. Infra/orchestrator bug reporting | Epic 4.1 |
| 3. Ask-for-help escalation | Epic 5.1 (5.2 explicitly deferred) |
| 4. Human-in-the-loop preservation | Epic 7.1 + every Story's explicit human-visibility acceptance criterion |
| Pain point A (PR visibility) | Epic 3.1, Epic 3.2 |
| Pain point B (verdict dispute) | Epic 6.1 |
| Compose-not-duplicate constraint | ADR-001 (Epics 5, 6 reuse `StuckReason`/`MarkStuck`); ADR-002 explains why Epic 4 deliberately does *not* reuse `StuckReason` (item-id-required schema mismatch) |
