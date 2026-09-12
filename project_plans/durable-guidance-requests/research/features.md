# Feature Landscape: Durable Guidance Requests

## 1. Existing analogs for "ask something, get an answer later"

Four existing mechanisms overlap with what a durable Guidance Request needs. None
of them is durable+structured+cross-session at the same time — each nails one or
two of those properties and drops the others. That gap is exactly what this
feature has to fill.

### 1a. Tool-use approval flow (`PendingApproval` / `ApprovalStore`) — closest UX, but NOT durable

- Proto: `PendingApprovalProto` (`proto/session/v1/types.proto:1248`), RPCs
  `ResolveApproval`/`ListPendingApprovals` (`proto/session/v1/session.proto:139,143`).
- Server: `server/services/approval_handler.go`'s `HandlePermissionRequest`
  (`approval_handler.go:240`) is invoked synchronously by Claude Code's
  `PermissionRequest` HTTP hook. It creates a `PendingApproval` in
  `ApprovalStore` (`server/services/approval_store.go:76`, an in-process
  `map[string]*PendingApproval` guarded by `sync.RWMutex`), then **blocks the
  HTTP handler goroutine** on a per-approval `decisionCh` channel for up to
  `h.timeout` (default 4 minutes, `approval_handler.go:95`) — strictly less
  than Claude Code's 5-minute hook timeout. A human resolves it via
  `ResolveApproval` RPC (`server/services/approval_service.go:97`,
  `resolveApproval`) or a Slack interactive button
  (`server/services/slack_interactive_handler.go:41,120`), which pushes onto
  `decisionCh` and the blocked hook handler returns the decision to Claude
  Code's live process.
- **Durability is explicitly out of scope by design**: `ApprovalStore`
  persists pending approvals to a JSON file
  (`NewApprovalStore`, `approval_store.go:91`) purely so a server restart can
  reload them for *display*, but every reloaded approval is marked
  `Orphaned: true` (`approval_store.go:46-48`) and has `decisionCh == nil` —
  there is no live HTTP connection left to unblock, so `ResolveApproval` on an
  orphaned approval just deletes the record (`approval_store.go:213,228`)
  rather than delivering an answer anywhere. Orphaned approvals are
  garbage-collected after `orphanedCleanupThreshold = 4 * time.Hour`
  (`approval_store.go:72-73`).
- **Takeaway**: this is the wrong template to copy structurally (blocking a
  live HTTP connection is the opposite of "survive session being
  paused/restarted"), but its UI treatment (toast → dashboard panel → Slack
  button) and its `MarkHumanResolving`/reconciliation-race handling
  (`approval_service.go:79-96`) are worth reusing patterns from.

### 1b. `AskUserQuestion` passthrough — same UX slot, currently has NO durable answer path at all

- When Claude Code's own built-in `AskUserQuestion` tool fires, stapler-squad's
  hook handler calls `broadcastQuestionNotification`
  (`server/services/approval_handler.go:695-711`), which publishes an
  `NOTIFICATION_TYPE_INPUT_REQUIRED` toast with **no `approval_id`** in its
  metadata (deliberately, per the comment at `approval_handler.go:692`: "so no
  Approve/Deny buttons are shown") and a message that just says "Check the
  terminal to respond." There is no persisted question record, no structured
  answer capture, and no cross-session delivery — the user must locate the
  live tmux terminal and type directly into it. If the session is paused or
  the terminal isn't visible, the question is effectively stranded.
- This is the single most direct precedent for the feature's trigger surface
  (an LLM asking a question mid-session) and simultaneously the sharpest
  illustration of the gap this feature must close.

### 1c. Auto-approval rules (`ListApprovalRules`/`UpsertApprovalRule`) — durable config, not a request/response construct

- `ApprovalRuleProto` (`proto/session/v1/types.proto:1294`), backed by the
  `session/ent/schema/approvalrule.go` ent schema and
  `server/services/rules_service.go` (1573 lines). This is durable, ent-backed
  storage, but it's a standing declarative rule set ("always allow `npm test`"),
  not an ad hoc question-and-answer exchange scoped to one item/session. Useful
  precedent for the ent-backed CRUD + proto + React panel wiring pattern
  (`web-app/src/components/sessions/ApprovalRulesPanel.tsx`), not for the
  async-answer semantics.

### 1d. Backlog review-queue flow (`request_review`/`submit_review_verdict`) — durable, but a workflow *gate*, not a question

- `session/review_queue.go`, `session/review_state.go`,
  `session/review_queue_determiner.go`, `server/services/review_queue_service.go`.
  `request_review`/`submit_review_verdict` are MCP tools in
  `server/mcp/tools_backlog.go:1221` and around line 2676 (triage's sibling,
  `submit_triage_result`) that drive **backlog item status transitions**
  (`in_progress` → `review` → `done`/back to `in_progress`), gated by
  `validateSelfResolveSource` (`tools_backlog.go:286`). The "answer" here is a
  verdict on a whole body of work, not a scoped question with a typed answer
  (yes/no/choice/text), and it's inherently 1:1 with the backlog item's
  lifecycle rather than a general-purpose construct usable standalone or
  per-session.
- Its durability comes from `BacklogItemPrecondition`-gated ent transitions
  (`tools_backlog.go:1332`) — a good template for how a guidance request's
  answer should durably mutate item/session state without racing a stale
  read, mirroring the compare-and-swap precondition pattern already used
  here.

### 1e. Backlog activity notes (`post_backlog_update`) — closest durability/any-session-can-post precedent

- `session/ent/schema/backlog_activity_note.go`: an **append-only, ent-backed,
  per-item log** any session can write to "with or without
  `STAPLER_SESSION_UUID`, regardless of whether that session is linked to the
  item" (doc comment, `backlog_activity_note.go:13-23`). Fields: `item_id`,
  `message`, `author_session_uuid` (optional), `author_session_title`
  (optional), `created_at` (immutable). Indexed by `(item_id, created_at)` for
  "all notes for this item, in order" queries
  (`backlog_activity_note.go:57-61`).
  Deliberately kept as a **sibling table**, not an extension of the
  role-gated `BacklogProgressNote`/`ProgressNoteData` used by
  `report_progress` — see
  `project_plans/backlog-item-activity-log/decisions/ADR-001-sibling-table-not-extend-progress-note.md`
  for the "ungated write must never be confused with role-gated data"
  rationale.
- **Takeaway**: this ADR's reasoning applies almost verbatim to Guidance
  Requests — they should probably be their own ent schema/table (or a small
  family of tables: request + answer), not bolted onto
  `BacklogActivityNote` or `BacklogProgressNote`, because a question has
  structured fields (type, choices, answer, answered_by, expiry) that a
  freeform note does not, and mixing gated/ungated or structured/freeform
  semantics into one table has already been explicitly rejected once in this
  codebase.

### 1f. Notification history (`GetNotificationHistory`) — the audit-trail precedent

- `proto/session/v1/session.proto:1660` `NotificationHistoryRecord` +
  `GetNotificationHistoryRequest/Response` (`session.proto:1683-1697`),
  served by `server/services/notification_service.go` (369 lines). Supports
  pagination (`limit`/`offset`), filtering by type/session/unread, dedup via
  `occurrence_count`/`last_occurred_at`, and a `Clear()` that is
  **unconditionally forbidden from deleting unread actionable records**
  (`approval_needed`/`question`/`error`/`task_failed`/`warning` — see
  `ClearNotificationHistoryRequest`'s doc comment, `session.proto:1712-1719`).
  Note the enum already special-cases `"question"` as an actionable
  notification kind that must never be silently GC'd — direct textual
  evidence the codebase's own notification model already treats "question"
  as a first-class, must-survive-cleanup category, even though nothing today
  populates it with a real answerable question object.

### 1g. Multi-user / workspace-peer awareness

- `session/workspace_peers.go`'s `ListWorkspacePeers` (`workspace_peers.go:58`)
  and `WorkspacePeer` struct (`workspace_peers.go:25-40`) already model "other
  sessions sharing this workspace" with a `Lifecycle()` of `gone`/`stuck`/`active`
  derived from `InstanceLive` + `StaleGoal`. There is no equivalent notion of
  *human* multi-user/multi-workspace-peer identity anywhere in the codebase —
  `list_workspace_peers` is about sibling **sessions**, not sibling **people**.
  Every write path inspected (`post_backlog_update`, approvals, review
  verdicts) identifies the actor only via `author_session_uuid`/session
  identity, never a human user ID. This is a real gap for the "answering user
  differs from asker" edge case (see §2) — the codebase currently has no
  human-identity concept to check against at all, only session identity.

## 2. Edge cases and failure modes to design for

- **Unanswered question / TTL**: No existing construct has a "waiting
  forever" story except the approval flow, whose answer is "hard-timeout after
  4 minutes, then treat as denied/expired" (`approval_handler.go:95`,
  `stampResolved`, `approval_handler.go:124-142`) — appropriate for a live
  blocking tool call, wrong for a guidance request meant to survive session
  restarts over hours/days. The `StuckReason` enum (see §3) is the right
  precedent to extend instead: a `StuckReasonAwaitingGuidance` (or similar)
  driven by the existing stale-item sweep infrastructure
  (`session/backlog_lifecycle_stale.go`, `MarkStuckNotified`/`ResolveStuck` in
  `session/storage.go:1059-1062`) rather than a hard channel timeout. Needs an
  explicit product decision: does an unanswered question ever auto-expire
  (and with what fallback — proceed with a default? re-escalate?), or does it
  stay open indefinitely and only surface via staleness sweeps/notifications?
- **Scope object deleted/archived**: `BacklogActivityNote`'s
  `edge.From("item", ...).Required()` (`backlog_activity_note.go:49-54`) means
  notes are hard-tied to a `BacklogItem` via a required FK edge — ent's
  default edge-delete-behavior questions apply directly (does deleting the
  item cascade-delete open guidance requests, or orphan them the way
  `ApprovalStore` orphans approvals?). Recommend explicitly deciding: a
  standalone guidance request (per the requirement's "or standalone" scoping
  option) must survive its scoping session/item disappearing, so unlike
  `BacklogActivityNote` the FK edge to item/session should probably be
  **optional**, not required, with the request retaining enough denormalized
  context (title/summary) to remain readable after the scope object is gone —
  mirroring how `PendingApproval`'s persisted JSON keeps `SessionID` as a
  plain string rather than a live reference.
- **Multiple simultaneous open questions per item/session**: No existing
  precedent enforces a cap — `BacklogActivityNote` and notification history are
  both unbounded append logs. `session/backlog_triage.go`'s chat-retriage
  prompt (`BuildHeadlessChatRetriagePrompt`, `backlog_triage.go:176-184`) is
  the one place that *does* impose a limit, but by prompt instruction only
  ("include AT MOST ONE... never more than one question in a single
  response") — not enforced in code/schema. Worth deciding whether the
  storage layer should enforce "at most N open guidance requests per
  item/session" the way `blockedCycleThreshold = 3`
  (`server/mcp/tools_backlog.go:1387`) caps `report_blocked` cycles, versus
  leaving it to UI-level queuing.
- **Answering user ≠ asking session's expected user**: No existing mechanism
  restricts *who* may resolve an approval, verdict, or note — `ResolveApproval`
  takes any caller's decision, `submit_review_verdict` is gated by session
  *role* (work vs review) not by human identity. Given §1g's finding (no human
  identity model exists at all), a first version likely can't meaningfully
  restrict "which human should answer" — it can only track *which session*
  asked and let any UI-connected human answer, recording who (if anything
  beyond "the UI") answered. Flag this as a genuine open design question
  rather than something existing patterns already solve.
- **Editing/withdrawing a question before it's answered**: No precedent exists.
  `ApprovalStore` supports cancellation
  (`approval_store.go` — approvals removed on session-stop, "Cancel" path
  distinct from resolve) but nothing supports an *asker*-initiated edit of
  the question text after creation. Needs explicit scoping: is a
  guidance request immutable once created (simplest, and matches
  `BacklogActivityNote`'s immutable `created_at`/append-only philosophy), or
  editable/withdrawable by the original asker only?
- **Asker session already moved on when the answer arrives**: This is the
  crux of "durable." The approval flow's model (block the asker's live
  process until answered) is unusable here by definition — the asker may be
  paused/dead by the time an answer lands. The two live options in this
  codebase's existing vocabulary are: (a) **poll on next check-in** — mirrors
  `wait_for_backlog_event(item_id, event_type="verdict_recorded")`
  (referenced in `server/mcp/tools_backlog.go:531`, an existing MCP tool that
  blocks until a specific event fires *or times out*, used today for
  review verdicts) — a `wait_for_guidance_answer` tool built the same way
  would let a *live* session block-with-timeout, while a resumed/restarted
  session would just re-check via a `get_...` read; or (b) **active
  resumption** — actually calling something like `resume_session`
  (`mcp__stapler-squad__resume_session` is a real tool in this server) when an
  answer lands for a paused session. (a) requires no new session-lifecycle
  code; (b) is a stronger guarantee but a materially bigger scope increase.
  Recommend (a) as the default with (b) as an explicit stretch/non-goal to
  call out in the plan.

## 3. Unstated needs beyond the explicit requirement

- **Audit trail**: Almost certainly wanted, and cheap to justify by precedent
  — `GetNotificationHistory` (§1f) already gives this codebase a fully
  worked pattern (paginated, filterable, occurrence-deduped, ent/store-backed)
  for exactly this shape of "history of past X." A guidance-request history
  view (per item, and probably a global one alongside notification history)
  should follow the same RPC shape rather than inventing a new one.
- **A distinct "waiting for guidance" status/badge**: Strong existing
  extension point already exists and should be reused rather than a new bolt-on
  concept: `domain.StuckReason*` is a real, actively-extended enum (grep
  turned up `StuckReasonStaleWork`, `StuckReasonBounceCapExhausted`,
  `StuckReasonAbandonedReview`, `StuckReasonGateTimeout`,
  `StuckReasonOrphanedTriage`, `StuckReasonPushFailed`,
  `StuckReasonPRPendingNoPR`, `StuckReasonPRNeedsFix`,
  `StuckReasonPRReadyUnmerged`, `StuckReasonPlanNotApproved`,
  `StuckReasonBouncing`, `StuckReasonMultipleReasons` across
  `session/backlog_lifecycle*.go`), each driven through the shared
  `MarkStuckNotified`/`ResolveStuck` pair and — per this repo's own
  MEMORY.md note on PR #769 — a single shared `StuckReason` priority order
  between item detail and the board view. Adding
  `StuckReasonAwaitingGuidance` (or similar) and wiring it through that same
  priority order is very likely the lowest-friction way to get a
  human-visible "blocked on guidance" badge in both the backlog board and
  item detail for free, rather than inventing a parallel status concept.
- **Batch-answer UX**: No direct precedent, but the closest sibling is
  `MarkNotificationReadRequest`'s "empty `notification_ids` means mark all"
  convenience (`session.proto:1699-1703`) and the review-queue's
  priority-sorted, statistics-bearing `ReviewQueue` message
  (`proto/session/v1/types.proto:911-932`, with `by_priority`/`by_reason`
  breakdowns). If many items end up waiting on structurally similar
  questions, a triage/review-queue-style aggregate view (grouped by
  question similarity or by asking-session) is a natural v2, but nothing in
  the current codebase suggests this was scoped for v1 — flag as an explicit
  non-goal unless the plan phase decides otherwise.
- **Structured-answer type reuse for automated triage**: `backlog_triage.go`'s
  suggestion objects already carry a `rationale` field that is set to the
  *string* `"question"` (`TriageSuggestion.Rationale`, `backlog_triage.go:13`)
  — free text only, no answer type/choices. If Guidance Requests ship,
  `submit_triage_result`'s suggestion pipeline is the natural first caller to
  migrate onto the new construct (turning a triage "question" suggestion into
  a real structured `GuidanceRequest` instead of a text blob a human must
  notice and manually retype into the chat-retriage feedback field) — this is
  explicitly named in the requirement text ("Automated triage... should be
  able to use this instead of guessing") and the code path
  (`BuildHeadlessChatRetriagePrompt`, `backlog_triage.go:176-184`) is the exact
  integration point.

## 4. Industry patterns and their fit

- **Workflow-engine "human task"/approval step** (Temporal signals, Airflow
  sensors, Camunda user tasks): a durable task object that blocks a workflow
  until a signal arrives, decoupled from any specific worker process being
  alive. **Maps cleanly** onto this codebase's ent storage + ConnectRPC — the
  guidance request *is* this pattern, and `BacklogItemPrecondition`-gated
  transitions (§1d) already give a compare-and-swap primitive for "resume the
  workflow exactly once when answered." No net-new infrastructure needed for
  the durability core; it's squarely what ent + the existing lifecycle
  sweepers (`backlog_lifecycle*.go`) already do for other paused states.
- **Slack interactive messages with buttons**: a message carries interactive
  elements; a webhook callback resolves the pending action server-side.
  **Already partially built**: `server/services/slack_interactive_handler.go`
  and `SlackNotifier.NotifyApprovalPending` (`approval_handler.go:685`) do
  exactly this for tool-use approvals today. Extending Slack buttons to
  guidance requests (yes/no/multiple-choice map naturally to Slack's
  `block_kit` interactive elements) is incremental work on an existing
  integration, not new infrastructure — short-answer/free-text questions
  would need a Slack modal or a "reply in dashboard" fallback link, which is
  a smaller net-new piece.
- **GitHub "request changes" review**: a durable, threaded, per-PR review
  state machine with required-reviewer semantics. Maps loosely onto
  `request_review`/`submit_review_verdict` (§1d) already, but that pathway
  is hardwired to backlog-item lifecycle transitions, not reusable as a
  generic primitive — a guidance request needs a lighter-weight, workflow-
  agnostic version of the same "durable pending decision" idea, closer to
  `BacklogActivityNote`'s generality than to the review queue's specificity.
- **What's genuinely net-new for this codebase**: (1) a *structured answer
  type* (yes/no, multiple-choice, short-answer) — every existing "answer"
  in this codebase is either a two-value decision (`allow`/`deny`) or
  freeform text; nothing here models an enum-of-choices answer schema
  today, so the proto/ent modeling for that is new ground, not an extension.
  (2) A **standalone** (not backlog-item-scoped, not session-scoped) question
  object — every durable, ent-backed construct found (`BacklogActivityNote`,
  `BacklogProgressNote`, `ApprovalRule`) is anchored to something; the
  requirement's "or standalone" option has no existing analog to lean on and
  will need its own top-level ent entity and its own UI surface (the
  requirement's ">=3 views" bullet already anticipates this).
