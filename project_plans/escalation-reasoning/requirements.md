# Requirements: Escalation Reasoning on Review Queue Items

Source: backlog item `40a243b0-d4d3-4d5f-a9c5-3084def729eb` (migrated from
`TylerStaplerAtFanatics/stapler-squad#45`). Derived directly from the item's
acceptance criteria — no interactive ideation interview (none is possible in
this pipeline mode).

## Problem

When `HandlePermissionRequest` escalates a tool-use request to human review
(`session/review_queue_poller.go` reason `ReasonApprovalPending`), the
reviewer sees *what* is being requested but not *why* it wasn't auto-handled.
The classifier already computes this information — `RuleID`/`Reason` on
`classifier.ClassificationResult`, and a synthetic reason string in the
domain-age branch — but neither survives past the request handler:

- `server/services/approval_handler.go:261` — the domain-age escalation
  reason is built, then explicitly discarded (`_ = reason // will be
  surfaced when the approval is shown in review queue`).
- `server/services/approval_handler.go:283-312` — the classifier's `result`
  (containing `RuleID`, `Reason`) is scoped inside `if h.classifier != nil {
  ... }` and never reaches the `createApproval:` label where
  `PendingApproval` is constructed (`approval_handler.go:358-369`).
- `session/service.PendingApproval` (`server/services/approval_store.go:21-39`)
  and its disk-persisted twin `PersistedApproval`
  (`server/services/approval_store.go:42-53`) have no field to carry it even
  if it were threaded through.
- `session.ApprovalMetadata` (`session/review_queue_poller.go:54-61`), the
  type used to enrich `ReviewItem.Metadata` at poller-run time
  (`session/review_queue_poller.go:807-829`), likewise has no such field.

## Goals (from acceptance criteria)

Numbering matches the backlog item's AC list (1-indexed) for direct
traceability in the implementation plan and verification phase.

### AC1 — Plain-language reason, sourced from the real escalation path
A queue item with reason `ApprovalPending` shows an explanation grounded in
the actual classification outcome, not a generic string. Must be correct for:
- **no-rule-match** — `classifier.classifySingle` fallback, `RuleID == ""`
  (`pkg/classifier/classifier.go:523-527`).
- **explicit-rule escalation** — an `Escalate`-decision rule matched, e.g.
  seed rules like `seed-escalate-git-branch-safe-delete`
  (`pkg/classifier/classifier.go:1002-1017`), `RuleID != ""`.
- **new-domain-age escalation** — `DomainAgeChecker.IsNewlyRegistered`
  branch (`approval_handler.go:237-266`), synthetic `RuleID:
  "new-domain-check"`.

Secret-scan is explicitly **out of scope for AC1**: it's a terminal
`AutoDeny` (`approval_handler.go:205-233`) that returns before
`createApproval` and never creates a queue item. It still needs an analytics
bucket (AC4).

### AC2 — Persisted, not request-scoped
The escalation reason must survive a server restart / orphaned-approval
reload from disk (`ApprovalStore.persistToDiskLocked` /
`loadFromDisk`, `approval_store.go:291-394`), not just live in the HTTP
request's stack frame.

### AC3 — No-match escalations link to the suggested-rule flow
When the reason is "no rule matched," the UI offers the *existing*
`SuggestedRuleCard` / `createPortal` flow already wired in
`ReviewQueuePanel.tsx` (button + portal at roughly lines 818-838 and
1345-1420). Do **not** touch `ImportRulesModal.tsx` or
`ApprovalRulesPanel.tsx` — an earlier draft of this item incorrectly named
those. The `commandSample` input to `GenerateSuggestedRule` is already
sourced from `queueItem.metadata["tool_input_command"]`; the gap is
conditioning visibility/emphasis on the new escalation-reason data, not new
RPC plumbing.

### AC4 — Analytics breakdown by escalation-reason category
`ApprovalAnalyticsPanel` shows a breakdown of escalation counts by reason
category over a selectable time window:
- no-match
- explicit-rule
- domain-age
- secret-scan
- the classifier-internal "unclassifiable" shell-expansion sentinel
  (`RuleID: "shell-expansion-program"`, `pkg/classifier/classifier.go:485-493,
  536-544`)

Backed by a `ComputeSummary`-style aggregation
(`server/services/analytics_store.go:317-440`, extending the existing
coverage-gap pattern at lines 395-406) plus a **frontend rendering test** —
a backend `ComputeSummary` unit test alone does not satisfy this AC.

### AC5 — No regressions in existing approval flow behavior
`approval_handler_test.go` and `review_queue_determiner_test.go` continue to
pass unmodified in behavior (auto-allow/auto-deny paths, timeout,
orphaned-approval reload, hook response contract), plus new tests covering
the added fields.

### AC6 — Plain text, no new UI dependency
Escalation reason renders as plain text using the existing `itemContext`
class pattern in `ReviewQueuePanel.tsx`. Note: `itemContext` is currently
**suppressed** for pending-approval items
(`ReviewQueuePanel.tsx:718`, `queueItem.context && !queueItem.metadata?.["pending_approval_id"] && (...)`)
— the escalation-reason text needs its own render branch inside the
`pending_approval_id` block (around lines 726-743), not a change to that
suppression.

### AC7 — "Create Rule" button intent
For no-match escalations, the "Create Rule" button uses `intent="secondary"`
(currently `intent="ghost"`, `ReviewQueuePanel.tsx:820`) — visually distinct
border/background — and must never use `intent="primary"`, which is
reserved for "✓ Approve" (`ReviewQueuePanel.tsx:787`) to avoid a misclick
between two actions of unequal consequence on the same card.

### AC8 — Real end-to-end Playwright coverage
A new spec `tests/e2e/escalation-reasoning.spec.ts` drives a **real**
no-match escalation: create a session, POST a permission-request payload
directly to `POST /api/hooks/permission-request`
(`server/server.go:484-485`, handled by
`ApprovalHandler.HandlePermissionRequest`) for a command no rule matches,
poll the review queue via the existing `SessionClient` helpers in
`tests/e2e/helpers/session-client.ts` (`waitForReviewQueue`,
`getReviewQueue`), and assert the reason text is visible on the rendered
`/review-queue` page. Not satisfied by Jest tests that mock
`queueItem.metadata` directly. The hook call blocks server-side until
decision/timeout, so the test must fire it without awaiting full completion
and poll separately.

## Grounded design constraints (from exploration, to carry into research/plan)

- **No proto change needed for `ReviewItem`** — `metadata` is already a
  generic `map<string,string>` (`proto/session/v1/types.proto:575`) that
  passes through unmodified via `ReviewItemToProto`
  (`server/adapters/review_queue_adapter.go:42-65`). Add a new metadata key
  (e.g. `escalation_reason`, `escalation_category`) rather than a schema
  migration.
- **A proto change IS likely needed for the analytics summary breakdown**
  (AC4) — `AnalyticsSummaryProto` (`proto/session/v1/types.proto:1108-1134`)
  has explicit typed fields (no free-form map), so a new repeated field
  (e.g. mirroring `RuleStatProto`/`ToolStatProto` at lines 1136-1154) plus
  `make proto-gen` is the likely path. Plan phase should confirm/size this.
- **Backend plumbing gap to close**: `result classifier.ClassificationResult`
  in `HandlePermissionRequest` is block-scoped inside `if h.classifier !=
  nil { switch result.Decision { ... } }`
  (`approval_handler.go:280-312`) and never reaches the `createApproval:`
  label (`approval_handler.go:315`), which is also reachable via `goto` from
  the domain-age branch, bypassing the classifier entirely. The plan needs a
  single escalation-reason value (category + human string + optional
  `RuleID`) hoisted to function scope, set in both the domain-age and
  classifier-escalate branches, and threaded into `PendingApproval` →
  `PersistedApproval` → `ApprovalMetadata` → `ReviewItem.Metadata`.
- **Reason category taxonomy** (drives both AC1 rendering and AC4 analytics
  buckets), derivable from existing `RuleID`/`Decision` values already
  recorded via `AnalyticsEntry` (`analytics_store.go:17-43`):
  - `RuleID == ""` and `Decision == Escalate` → no-match
  - `RuleID == "new-domain-check"` → domain-age
  - `RuleID == "secret-scan"` → secret-scan (AutoDeny, analytics-only, no
    queue item)
  - `RuleID == "shell-expansion-program"` → unclassifiable
  - any other non-empty `RuleID` with `Decision == Escalate` → explicit-rule
- **`review_queue_determiner.go` is a different concept** — it decides
  *whether* a session needs attention (approval-pending / stale / error /
  etc.), not *why* a specific approval was escalated. The escalation reason
  is orthogonal and is layered on afterward via the
  `ApprovalMetadataProvider` enrichment step. AC5's reference to
  `review_queue_determiner_test.go` is a regression guard, not an
  integration point.
- No existing `tests/e2e/pages/ReviewQueuePage.ts` page object exists; AC8's
  spec will use `data-testid` locators already present in
  `ReviewQueuePanel.tsx` (`review-item-${sessionId}`,
  `approve-${sessionId}`, `create-rule-${sessionId}`, etc.).

## Out of scope

- The "3 similar commands were auto-approved in this session" nearest-match
  detail and one-click rule-add from the original GitHub issue's mockup are
  **stretch** in the issue but not present in any numbered AC — not
  required.
- Surfacing the reason in `ApprovalDrawer`/`ApprovalPanel` (the live
  in-session approval UI, distinct from the review queue) — not requested by
  any AC; `PendingApprovalProto` is not required to change.
- Rule-denied ("rule blocked it") as a *queue-item* reason — confirmed
  AutoDeny never creates a queue item, so this only exists as an analytics
  bucket (secret-scan), not a review-queue escalation reason. The original
  issue's "reasoning source #3 (rule denied)" doesn't apply to queue items in
  the current architecture.
- Stale-session detection integration (issue's "reasoning source #4") — no
  AC references it; excluded.

## Acceptance criteria traceability

| AC | Summary | Primary files touched (expected) |
|----|---------|-----------------------------------|
| 1 | Real, per-path escalation reason on queue item | `approval_handler.go`, `classifier.go` (scoped edit only — replacing 3 inline sentinel `RuleID` string literals with shared named constants during planning's architecture-review fix-up; no classification logic changes), `review_queue_poller.go` |
| 2 | Persisted across restart | `approval_store.go` (`PendingApproval`, `PersistedApproval`, persist/load) |
| 3 | No-match → SuggestedRuleCard flow | `ReviewQueuePanel.tsx` |
| 4 | Analytics breakdown by reason | `analytics_store.go`, `types.proto`, `ApprovalAnalyticsPanel.tsx` + test |
| 5 | No regressions | `approval_handler_test.go`, `review_queue_determiner_test.go`, new tests |
| 6 | Plain text via `itemContext` | `ReviewQueuePanel.tsx` |
| 7 | Button `intent="secondary"` | `ReviewQueuePanel.tsx` |
| 8 | Real e2e spec | `tests/e2e/escalation-reasoning.spec.ts` |
