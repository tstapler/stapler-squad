# Requirements: Session Completion Summary (Proof-of-Work Document)

Source: backlog item `59bbff11-ee8b-418c-8484-64307cb14244` (migrated from
`TylerStaplerAtFanatics/stapler-squad#43`). Pipeline mode `sdd`; this document
was written directly from the item description + the 8 acceptance criteria
(no interactive ideation interview) plus grounding reads of the current
session lifecycle, notification, and diff-stats code.

## Problem Statement

When a stapler-squad session ends, the only record of what the agent did is
raw terminal scrollback and the notification history log. Answering "what did
this session actually accomplish?" requires reading through that output by
hand. Symphony (a competing agent-orchestration tool) auto-generates a
proof-of-work document at session end; none of the 122 tools surveyed for
this project do so automatically. This feature closes that gap: a markdown
completion summary generated when a session ends, covering the diff, approval
decisions, timeline, and cost.

**Target user**: stapler-squad is a solo-maintainer/small-team self-hosted
tool, not a multi-tenant product; the user this feature is for is a developer
running stapler-squad sessions who wants to know what an agent did without
reading through raw scrollback.

## Goals

- Generate a markdown completion summary automatically, without manual
  action, when a session reaches a genuine terminal state.
- Make the summary durable and independently retrievable — it must outlive
  the `Session` row and survive a server restart.
- Make it exportable as-is for reuse as a PR body or issue comment.
- Never let generation slow down or block session teardown.
- Produce a valid document even for trivial/near-empty sessions, and even if
  the narrative (LLM) step fails.

## Non-Goals

- CI status, PR review feedback, complexity analysis, or walkthrough videos
  (Symphony has these; out of scope for this item — future work).
- A UI for editing/annotating the generated summary after creation (export
  only, per AC-4).
- Generating summaries for sessions killed by process crash/OOM outside the
  normal lifecycle-event path (still covered if `EventExited`/`EventStopped`
  fires; not covered if the process is force-killed such that no lifecycle
  event fires at all — same limitation as existing `BacklogLifecycleListener`
  bookkeeping).

## Grounding: Relevant Existing Code

- **Lifecycle events** — `session/instance.go` defines `EventExited` (fires
  on unexpected/natural process exit, e.g. reason `"pty-eof"` from
  `session/instance_controller.go:69`) and `EventStopped` (fires from
  `Destroy()`, reason `"operator-destroy"`, `session/instance.go:1247` — the
  explicit stop/delete path referenced in AC-7). The reconciler
  (`session/review_queue_poller.go:451`) also fires `EventExited` with reason
  `"reconcile-session-missing"` when it detects a session is gone without a
  real exit — AC-1 explicitly excludes this reason.
- **Existing listener pattern** — `session/backlog_lifecycle.go`'s
  `instanceBacklogListener.OnLifecycleEvent` subscribes to both
  `EventExited` and `EventStopped` and treats them identically ("ends the
  session exactly as much as an unexpected exit does... must not depend on
  which one happened", see BUG-027 reference at
  `session/backlog_lifecycle.go:804-808`). The new summary generator should
  follow the same dual-subscription pattern, with a reason-string filter for
  `reconcile-session-missing`.
- **Independent persistence precedent** — `AnalyticsEvent`
  (`session/ent/schema/analytics_event.go`) and the notification history
  store (`server/notifications/store.go`) both key off a plain
  `session_id` string field rather than a required ent edge to `Session`.
  This is the pattern needed for AC-3 ("survives... Session-row deletion"):
  an edge with `.Required()` (e.g. `DiffStats`'s `edge.From("session",
  Session.Type)` in `session/ent/schema/diffstats.go`) is the wrong shape to
  copy, since ent edges of that kind are tied to the parent row's lifecycle.
- **Approval decision data** — `ApprovalHandler`
  (`server/services/approval_handler.go`) resolves each approval as
  auto-allowed, auto-denied (rule match or plaintext-secret detection), or
  human-resolved, and stamps `approval_decision` metadata on the
  notification record (`stampResolved`, line 96) or writes a silent
  `NOTIFICATION_TYPE_AUTO_APPROVED` record via `autoApprovalLog` (interface
  at line 56, implemented by `NotificationHistoryStore.AppendAutoApproved` in
  `server/notifications/store.go:181`). This is the source for the
  auto-approved/manually-approved/denied breakdown in AC-2. Review-queue
  resolution counts come from the review-queue tables already used by
  `server/review_queue_manager.go` / `server/services/review_queue_service.go`.
- **Diff stats** — `session/ent/schema/diffstats.go` already models
  added/removed line counts and diff content per session;
  `SessionService.GetSessionDiff` (`server/services/session_service.go:2586`)
  is the existing RPC for full diff retrieval, and the `get_session_diff` MCP
  tool exposes the same data. The summary's "Changes" section should reuse
  this rather than recomputing the diff independently.
- **Token usage / cost** — `session/tokens` (`parser.go`, `pricing.go`)
  already parses JSONL transcripts into token counts and estimated cost, and
  `InsightsService` / the token-usage RPCs
  (`server/features/analytics.go`, `insights.go`) already surface per-session
  token and cost data. Reuse this rather than re-deriving cost math.
- **Web UI session tabs** — `web-app/src/components/sessions/SessionDetailView.tsx`
  renders a tab strip (`tabs` array, line ~283: info/diff/vcs/files/logs/...)
  driven by `SessionDetailTab` from `SessionDetail.tsx`. AC-3 requires a new
  "Summary" tab following this existing pattern. There is currently no
  session-summary tab or component in the web app.

## Functional Requirements

### FR-1: Trigger — natural termination only (AC-1, AC-7)

A new lifecycle listener (or extension of an existing one, per the dual
`EventExited`/`EventStopped` pattern above) triggers summary generation on:
- `EventExited` with any reason **except** `"reconcile-session-missing"`.
- `EventStopped` (the explicit stop/delete path — `stop_session` MCP tool,
  `DeleteSession` RPC, backlog stale-work remediation).

No manual action is required to produce the document.

### FR-2: Document content (AC-2)

The generated markdown includes, at minimum:
1. **What Was Done** — an LLM-generated narrative grounded in the diff, the
   approval-decision breakdown, and the session's goal/title (not raw
   tool-use/tool-output history — see `implementation/plan.md`'s Pattern
   Decisions table for the "Narrative input scope" decision). Grounding in
   the goal/title alongside the diff and decisions gives the model real
   signal for low-diff/high-effort sessions (e.g. exploration or
   investigation work with little or no diff) without the hallucination
   risk and cost of ingesting full tool-call transcripts; the deterministic
   sections (Changes, Decisions, Timeline, Token usage) already cover the
   factual "what changed," so the narrative's job is framing/context rather
   than an independent factual record. Must have a deterministic, non-LLM
   fallback line if the narrative generation step fails or times out (see
   FR-5).
2. **Changes** — files modified, added/removed line counts (from
   `DiffStats`/`GetSessionDiff`), and a link to the full diff.
3. **Decisions** — counts and percentages for: auto-approved, manually
   approved, denied, review-queue-resolved, and still-open, derived from
   notification history / review-queue state as of session end.
4. **Timeline** — started, stopped, duration.
5. **Token usage** — total tokens and estimated cost (from `session/tokens`).

### FR-3: Independent, durable persistence (AC-3)

The document (and the data needed to regenerate/redisplay it) is persisted
in storage that does **not** depend on the `Session` row still existing —
i.e., not a required ent edge to `Session` (see `DiffStats` counter-example
above). It must be retrievable after:
- A server restart.
- Deletion of the originating `Session` row.

Retrieval is exposed via a new "Summary" tab on the session detail view,
scoped to sessions that ended via a natural/explicit-stop exit (FR-1). The
tab/data must also be reachable by some stable session identifier even once
the live `Session` row is gone (e.g. from session history/search), per AC-3's
"survives... Session-row deletion" language — this implies the retrieval
path itself can't assume a live `Session` lookup.

### FR-4: Export (AC-4)

A one-click "Copy as Markdown" action copies the full document to the
clipboard. The exported markdown must be usable verbatim as a GitHub PR body
or issue-comment attachment (i.e., no app-specific markup, valid GFM).

**Verification note**: the "reusable as a PR body/issue comment" claim is
verified manually, not by an automated GFM-linter assertion — paste the
exported markdown into an actual GitHub PR description field (or issue
comment box) and confirm it renders correctly with no raw markdown artifacts
(literal `#`/`-`/`[]()` characters showing instead of rendered headings,
lists, and links). See `implementation/validation.md`'s Requirement → Test
Mapping table (the "FR-4: 'Reusable as PR body' GFM-rendering claim" row)
for where this is tracked as a manual verification step.

### FR-5: Asynchronous generation, non-blocking, graceful degradation (AC-5)

- Generation is kicked off from the lifecycle listener but runs off the
  teardown path — it must not delay session stop/destroy completion.
- The pipeline has (at least) two stages with different failure semantics:
  - **Narrative stage** (LLM call): on failure/timeout, substitute a
    deterministic fallback line (e.g. derived from tool-use counts/diff
    stats) and continue. The document still reaches a `READY` state.
  - **Deterministic snapshot/persist stage** (diff stats, approval counts,
    timeline, token usage, and the write to durable storage): on failure,
    the document/session-summary record must surface a visible **error**
    state in the UI with a **Regenerate** action, rather than silently
    producing nothing or a corrupt partial record.

### FR-6: Minimal-activity sessions (AC-6)

A session that ends with little or no activity (e.g. started and stopped
immediately, no tool calls, no diff) still produces a valid document in the
`READY` state — not an error state, not a blank/omitted-sections document.
Empty sections render explicit empty-state text (e.g. "No files changed",
"No approval requests during this session") rather than being silently
dropped.

### FR-7: Idempotency / concurrency safety (AC-8)

Duplicate or concurrent triggers must not produce overlapping generation
work or nondeterministic persisted output:
- Concurrent or duplicate `EventExited` fires for the same session (already
  a known scenario in this codebase — see the reconciler's synthetic
  `EventExited` and real process-exit `EventExited` potentially racing).
- Repeated manual "Regenerate" clicks from the UI (FR-5's error-state
  action).

The mechanism must ensure at most one generation pipeline runs per session
at a time (e.g. a per-session in-flight guard / dedup key), so repeated
triggers neither spawn redundant LLM calls (cost) nor leave the persisted
document in a state determined by whichever pipeline happened to finish
last.

## Non-Functional Requirements

- **No teardown latency impact**: session stop/destroy RPCs must not
  wait on summary generation (FR-5).
- **Cost control**: the narrative LLM call happens at most once per
  successful generation per session (no automatic retries beyond what
  FR-7's dedup + explicit Regenerate allow).
- **Consistency with existing patterns**: reuse `DiffStats`/`GetSessionDiff`
  for changes, `session/tokens` for cost, and the notification-history /
  review-queue data for the decision breakdown, rather than re-deriving any
  of these independently.

## Out of Scope

- CI status, PR review feedback, complexity analysis, walkthrough videos
  (Symphony-specific features called out in the item as future whitespace,
  not requested by any AC here).
- Editing the generated document's content in the UI beyond regeneration.
- Summaries for processes that terminate without firing any lifecycle event.

## Acceptance Criteria (verbatim, from backlog item)

1. When a session transitions to its terminal (stopped) state via natural
   process exit, a completion-summary markdown document is generated
   automatically without manual action, excluding reconciler-driven
   spurious exits (reason=reconcile-session-missing).
2. The document includes: a 'what was done' narrative (with deterministic
   fallback on LLM failure), changed files with diff stat and a link to the
   full diff, an approval-decision breakdown (auto-approved / manually
   approved / denied / review-queue-resolved / still-open), a timeline
   (started/stopped/duration), and token usage + estimated cost.
3. The document is persisted independently of the Session row (survives
   server restart and Session-row deletion) and is retrievable from the
   session's Summary tab in the web UI for sessions ended via natural exit.
4. A user can export the document via one-click copy-to-clipboard as
   markdown, reusable as a PR body or issue-comment attachment.
5. Generation runs asynchronously and never blocks or delays session
   teardown; narrative-only failures still produce a READY document with a
   deterministic fallback line, while deterministic snapshot/persist
   failures surface a visible error state with a Regenerate action.
6. Sessions that end with little/no activity still produce a valid,
   non-error minimal document with explicit empty-state text rather than
   blank or omitted sections.
7. The explicit stop/delete path (the common real-world session-termination
   path) produces a completion summary the user can actually retrieve, not
   an orphaned write-only row.
8. Concurrent or duplicate EventExited fires, and repeated manual
   Regenerate clicks, do not spawn overlapping generation pipelines (no
   wasted LLM cost, no nondeterministic persisted result).

## Success Metrics

- 100% of sessions ending via natural exit or explicit stop/delete (i.e.
  every `EventExited` with reason != `reconcile-session-missing`, and every
  `EventStopped`) produce a retrievable `READY` or `ERROR`-with-Regenerate
  summary — never silently nothing — **for any session whose Summary tab or
  standalone route is subsequently visited at least once** (which triggers
  the lazy read-time staleness check, `session/session_summary_service.go`'s
  `reconcileStaleness`). A session whose summary is never revisited after a
  crash mid-generation stays stuck in `GENERATING` indefinitely — this is a
  known, accepted v1 limitation of the lazy-reconciliation design (no
  background sweep in v1); see `implementation/plan.md`'s Pattern Decisions
  table, "FR-7 restart-survival dedup" row, for the trade-off rationale.
- Zero measurable added latency to `stop_session`/`DeleteSession` RPC
  response time attributable to summary generation.
- A generated document can be pasted as a PR body with no manual editing
  required to be coherent GFM markdown.
