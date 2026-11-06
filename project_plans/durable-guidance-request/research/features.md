# Research: Feature Landscape — durable-guidance-request

Agent 2 (Features), SDD Phase 2. Scope: prior art in-repo, industry "ask/pause for
input" patterns, edge cases, and unstated needs.

## 1. Prior art in this repo

### 1.1 `project_plans/backlog-agent-communication/` (2026-07-23, never implemented)

Same root problem from the *infra/escalation* angle rather than *structured user
Q&A*. Read `requirements.md`, `decisions/ADR-001-agent-initiated-stuck-reason-rows.md`,
`decisions/ADR-002-global-infra-issue-report-entity.md`.

- **Dimension 3 ("ask for help")** is the closest overlap. ADR-001's decision: reuse
  `StuckReason`/`BacklogStuckState`/`MarkStuck` by adding a new
  `StuckReasonHelpRequested` enum value, called directly from an MCP tool handler
  instead of a reconciler tick — "an agent-initiated call to `MarkStuck` is not
  fundamentally different from a reconciler-initiated one from the data model's
  perspective." That reasoning **does not carry forward** to this feature: a
  `GuidanceRequest` is a structured Q&A record (question text, type, options, answer
  value) with its own lifecycle, not a one-bit "something's wrong, look at this" flag.
  Forcing it into `BacklogStuckState`'s shape (one row per `(item_id, reason)`, a
  `context` free-text field, no answer/options/type fields) would be the same
  "misrepresents scope" anti-pattern ADR-002 explicitly rejected for infra reports —
  see ADR-002's reasoning for why a structurally different shape got its own entity
  instead of being wedged into `BacklogStuckState`.
- ADR-001's negative consequence is directly relevant to design: "`RemediationDue`'s
  backoff-and-cap semantics are designed for *automated retry actions* — they do not
  fit an agent-initiated one-shot signal that must wait for a *human* response, not
  an automated respawn... they call `MarkStuck` directly and resolve via an explicit
  new human-driven RPC instead of `selfHealStuck`'s automatic
  status-change-triggered resolution." This is the same shape `GuidanceRequest` needs:
  a durable row whose *resolution* is a deliberate human write (an answer), never a
  reconciler side effect of the item reaching some other status. Requirements.md's
  ADR-001 precedent constraint ("BacklogStuckState — one row per scope with a unique
  key, atomic upsert, resolve-in-place") should be read together with *this* lesson —
  reuse the **pattern** (durable row, unique index, resolve-in-place, `notified_at`),
  not the **entity**.
- **Dimension 2 (`InfraIssueReport`, ADR-002)** is a useful precedent for "give it its
  own minimal entity rather than force-fitting an existing one," and for the
  `related_item_id`-as-optional-field idea — directly applicable to this feature's
  `scope: backlog-item | session | standalone` design, where `standalone` is the
  guidance-request analogue of a report with no owning item.
- **What does NOT carry forward**: dimensions 1 (structured stage handoff) and the
  PR-visibility/verdict-dispute pain points (A/B) are unrelated to this feature and
  should not be pulled in scope.
- **Never implemented** — no `InfraIssueReport`, no `StuckReasonHelpRequested`, no
  `RespondToHelpRequest` RPC exist in the codebase today (verified: `grep -rl
  "InfraIssueReport\|StuckReasonHelpRequested" --include="*.go" .` returns nothing).
  Any future implementation of that project's dimension 3 would now collide
  conceptually with `GuidanceRequest` — worth a forward-pointing note in this
  project's ADR that a future "ask for help escalation" should be evaluated as a
  `GuidanceRequest` variant (e.g. a `scope: standalone` + a "flag for human, no
  specific question" type) rather than a third parallel mechanism.

### 1.2 `PendingApproval` (`server/services/approval_store.go`)

The closest *existing, shipped* mechanism to "pause and wait for a human answer" —
but explicitly named in requirements.md's Alternatives Considered as "closer
conceptually... but in-memory only, not durable." Confirmed: `ApprovalStore` is a
`sync.RWMutex`-guarded in-memory map (`pending map[string]*PendingApproval`), with a
`decisionCh chan ApprovalDecision` per approval for live delivery, and a
`Orphaned bool` field marking approvals reloaded from disk after a restart that
"have no live HTTP connection, so they cannot be resolved via the decision channel."
That `Orphaned` field is itself evidence of the exact failure mode this feature must
solve durably: `PendingApproval` *does* persist to `pending_approvals.json` for
crash-survival of the record, but resolution requires a live channel that a restart
destroys — a structural half-measure `GuidanceRequest` should not repeat. Take from
it: the shape of a form (`ToolName`, `ToolInput`-equivalent → `question`/`type`/
`options`), the `EscalationReason`/`RiskLevel` capture-once-at-creation idiom
(applicable to `GuidanceRequest`'s question/options — set once, never re-derived),
and the `orphanedCleanupThreshold` idea (does a pending `GuidanceRequest` need a
similar staleness/expiry policy? Not in the ACs — flag as an open question, not a
default "yes").

### 1.3 Event/notification plumbing

- `pkg/events/bus.go`'s `EventBus` is **purely in-memory, live-subscriber pub/sub**
  (`Subscribe(ctx) (<-chan *Event, string)`, `Publish(event *Event)`,
  `SubscriberCount()`) with no persistence layer. `wait_for_backlog_event`
  (`server/mcp/tools_backlog.go:775`) blocks on this channel with a
  `currentStateWaitResult` fallback that re-reads current DB state if nothing arrives
  in time — i.e. the *tool* is resilient to a missed event by re-checking the
  authoritative row, but the **bus itself buffers nothing across a subscriber
  disconnect or a server restart**. This directly answers the Rabbit Hole question:
  `BacklogItemEventPublisher`/`EventBus` covers "a live blocking RPC call," **not**
  "notify a session across a restart." `GuidanceRequest`'s cross-restart notification
  must follow `BacklogStuckState`'s `notified_at` idiom (a durable timestamp column
  the row's own state proves) plus the same defense `wait_for_backlog_event` uses:
  the *consumer* (a resumed session, or MCP tool call) re-reads the authoritative
  `GuidanceRequest.status`/`answer` row rather than trusting it received a live event.
  EventBus can still be the *live* notification path (a session that happens to be
  running gets pushed immediately), but it cannot be the *only* path.
- `session.Notifier` (`session/backlog_lifecycle.go:29`) is the separate,
  already-proven transport for human-facing push notifications (used throughout
  `backlog_service_triage.go`'s `notify*` family and `BacklogStuckState`'s
  `notified_at`-gated notify-once semantics). This is the transport for "tell the
  *human*"; a `GuidanceRequest` created by triage/a session needs this to alert the
  human a question is pending — separate from whatever tells the *originating agent*
  its answer arrived.

### 1.4 `resolveItemLink` (`server/mcp/tools_backlog.go:585`)

Confirmed exact ownership-check shape requirements.md asks to reuse: looks up
`GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)`, and on
`session.ErrNotFound` disambiguates "item doesn't exist" (`ErrItemNotFound`,
non-retryable per `itemNotFoundRemediation`) from "item exists but this session has
no link to it" (`PERMISSION_DENIED`) — a distinction the underlying ent join
predicate can't make on its own. A `GuidanceRequest` creation RPC/MCP tool with
`scope: backlog-item` should call this verbatim rather than re-deriving link logic;
`scope: session` and `scope: standalone` have no equivalent existing check and need
new (likely much simpler — "does the caller session ID match") authorization, which
the plan phase should specify explicitly since `resolveItemLink` doesn't cover them.

### 1.5 Feature flags (`server/features/flags.go`, `server/interceptors/feature_flag_interceptor.go`)

Confirmed live-settable rollout pattern requirements.md's constraint points at:
`GetFeatureFlags`/`UpdateFeatureFlag` RPCs registered in `featureregistry`, read at
call time via an interceptor rather than an env var or build flag — matches the
user's global memory note "rollout flags: live-settable, no env vars." AC3's triage
halt-and-wait flag should be a new `featureregistry.Feature` entry following this
exact pattern (see `server/features/flags.go` for the two-line registration idiom),
not a new ad hoc config mechanism.

### 1.6 Triage halt/resume mechanics

Triage is **not** a single long-lived stateful process — `backlog_service_triage.go`
is built from discrete, re-entrant operations (`SpawnSessionFromItem`,
`DequeueNextQueuedItems`, `spawnSessionAfterGates`, WIP-cap gating at line ~690)
invoked per item/tick, with state living in the `BacklogItem`'s durable status field
(`BacklogStatus`) and `BacklogItemPrecondition` guards, not in in-memory process
state. This is good news for "halt and wait" cheapness: a triage run that creates a
`GuidanceRequest` and halts does not need to keep a process alive — it can simply
leave the item in a "waiting on guidance" status (a new precondition/status value,
or an existing one reused — plan phase to decide) and let the *next* triage tick (or
the guidance-answered event) resume it, exactly like `DequeueNextQueuedItems`
already resumes queued items when a WIP slot frees up. This confirms Rabbit Hole #4's
premise was right to flag but the answer is favorable: pause/resume is cheap here
because triage already works this way for an unrelated reason (WIP capacity).

Separately, `pause_session`/`resume_session` (`server/mcp/tools_lifecycle.go:68-79`)
is a *session*-level (tmux/worktree) pause, not a *triage-logic* pause — it commits
uncommitted changes, tears down the worktree, and stops the tmux process. This is
the wrong primitive for "triage halts on ambiguity" (that's a same-tick early-return,
not a session pause) but is the right existing primitive to note for a related,
adjacent case: a live interactive session that asks a `GuidanceRequest` and then
sits idle waiting could plausibly also be `pause_session`'d to free tmux/worktree
resources while waiting — worth flagging to the plan phase as an optional
optimization, not a v1 requirement (nothing in the ACs asks for it, and it adds
resume-timing complexity: `resume_session` "recreates the git worktree," which is a
non-trivial cost per guidance round-trip).

### 1.7 No existing per-scope pending cap pattern

Confirmed via search: the only existing cap is `maxConcurrentBacklogWorkItems`
(board-wide WIP, `backlog_service_triage.go`), gated with a live count query
(`countLiveBacklogWorkSessions`) before spawn, with a documented failure-open
behavior on query error ("WIP count query failed... allowing spawn"). This
fail-open-with-a-warning-log idiom is the one existing precedent for how a
`GuidanceRequest` per-scope pending cap should behave on a count-query error —
reject-on-cap-hit but don't turn a transient DB hiccup into a hard block, logged
loudly. No per-*item* (as opposed to per-board) cap precedent exists, confirming
requirements.md's Rabbit Hole note; the plan phase is inventing this pattern, not
copying one.

### 1.8 `AskUserQuestion` — not implemented in this repo

`AskUserQuestion` appears in this codebase only as a *string literal detected in
Claude Code's own output* (`session/detection/binaries/claude.go:386`, a comment
about matching Claude Code's prompt format for PTY-output detection purposes) and
in a comment listing example tool names in `pkg/classifier/classifier.go:111`. It is
not a stapler-squad-implemented mechanism — it's the upstream Claude Code CLI's own
interactive tool-use turn, which stapler-squad's PTY-scraping detection layer merely
recognizes the on-screen shape of. This confirms requirements.md's Baseline
statement precisely and rules out any reuse: there is no local `AskUserQuestion`
handler code to extend — the "interactive session" baseline really is just "a human
watches the terminal and answers whatever autocomplete-style prompt the CLI itself
draws," entirely outside stapler-squad's data model.

## 2. Industry patterns ("ask the user / pause for input")

- **Temporal signals** (durable-workflow HITL): a workflow calls `wait_condition()`
  on a signal; the worker returns the task to the server and goes idle — consuming
  no compute while state is durably persisted server-side — until a signal arrives
  or a timer fires, at which point the server replays event history and resumes
  exactly where it left off, surviving worker restarts/deployments/months-long waits
  natively. [Human-in-the-loop AI agent — Temporal docs](https://docs.temporal.io/ai-cookbook/human-in-the-loop-python),
  [Human-in-the-Loop Approval Workflows — Temporal blog](https://temporal.io/blog/human-in-the-loop-approvals).
  **Relevance**: stapler-squad has no workflow engine with that replay guarantee —
  the closest analogue is exactly what section 1.6 found (state lives in a durable
  DB row + status field, and resumption is a fresh read-and-continue on the next
  tick/event, not in-process suspension). The `GuidanceRequest` design should treat
  itself as **the durability substrate that plays Temporal's server-side event-history
  role** — the row *is* the checkpoint. No new "wait_condition"-style blocking
  primitive needs to be invented; `wait_for_backlog_event`'s existing block-with-
  DB-fallback shape already covers the *live* case, and simply re-reading
  `GuidanceRequest.status` covers the *restarted* case.
- **GitHub Actions environment required reviewers**: a `workflow_dispatch` run can
  reference a protected `environment`, and a job referencing it goes to a "Waiting"
  status until one of up to 6 named reviewers approves — auto-fails if unapproved
  after 30 days. [GitHub Docs: Deployments and environments](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments).
  **Relevance**: two transferable ideas — (a) an explicit **expiry/auto-resolve
  policy** for a pending request that nobody ever answers (out of scope per
  requirements.md, "Editing/retracting a pending question" is excluded, but *silent
  permanent limbo* is a different failure mode worth flagging — see Edge Cases
  below), and (b) a visible "Waiting" state distinct from both pending-unseen and
  answered, which maps to `notified_at` set but `answered_at` still null.
- **Slack workflow approval steps** (Workflow Builder "Approval" step / interactive
  message buttons): posts a message with actionable buttons directly in the channel
  where the ask originated; the response updates that same message in place (no
  separate "go check a dashboard" step) and is durable against the approver being
  offline arbitrarily long — Slack's own message store is the durable record, not
  the requesting app's memory. **Relevance** (general knowledge, not doc-verified
  this pass): reinforces the requirement that the **answer surface must be co-located
  with wherever the human already is** — this feature's three views (backlog item
  detail, triage panel, session view) are the stapler-squad equivalent of "the
  channel where the ask originated," which is exactly why requirements.md calls out
  a single shared component across all three as the crux of the UI work.
- **Airflow human-in-the-loop** (`ExternalTaskSensor`/deferrable operators,
  Airflow 2.7+ HITL operators): a task suspends into a "deferred" state, releasing
  its worker slot entirely (via a trigger process), and resumes only when an
  external signal (an XCom push, a sensor poke, or a dedicated HITL operator
  response) arrives — explicitly built so a stuck-waiting task costs zero worker
  capacity. **Relevance** (general knowledge): the resource-non-consumption property
  matters for stapler-squad's triage case specifically — AC3's "halt and wait" must
  not mean "leave a Claude Code session/tmux pane alive burning cost while idle."
  Section 1.6's finding that triage is already re-entrant/tick-based means this
  property comes for free if the halt is modeled as "set status, exit the tick,"
  not "keep the triage session's process alive in a poll loop" — which is also
  exactly what requirements.md's Baseline explicitly forbids ("must not add another
  poll loop").

## 3. Edge cases and failure modes to design against

1. **Answering session is gone forever** (requirements.md's own framing). The
   consumer of the answer is "the originating session (resumed or fresh)" — so the
   design must not assume the *same* session process reads the answer. The
   `GuidanceRequest` row plus event-bus notify-if-live is necessary but not
   sufficient: whatever creates a *new* session to continue the work (a backlog
   reconciler, a human manually resuming) must know to check for
   answered-but-unconsumed `GuidanceRequest` rows scoped to that item/session as
   part of its own startup context-gathering (the same place `initialPromptFor`
   already assembles prior-session context, per `backlog_service_triage.go:42`).
2. **Item archived/deleted while a question is pending.** `resolveItemLink`'s own
   `itemNotFoundRemediation` string already establishes the existing idiom for "the
   item might be gone out from under you" — a pending `GuidanceRequest` scoped to a
   now-archived item should not surface in the "pending" UI list as if still
   actionable, and should not silently vanish either (a human who already saw it
   asked may wonder what happened to it). Recommend: keep the row, but the UI badges
   it "item archived" rather than "pending answer," and creation-time validation
   (mirroring `resolveItemLink`'s existence check) should refuse to *create* a new
   request against an already-archived item.
3. **Scope references a deleted/never-existed session** (`scope: session`, no
   `resolveItemLink`-equivalent check exists per section 1.4). A `GuidanceRequest`
   whose `scope` is a session ID must validate that session ID exists at creation
   time using whatever the session-lookup equivalent is, and the notify-on-answer
   path must handle "target session no longer exists" the same non-fatal way
   `BacklogItemEventPublisher.PublishItemChanged`'s recover()-wrapped best-effort
   send already treats a vanished/unmapped target — log and move on, never block the
   answer-write itself on a live listener existing.
4. **Concurrent double-creation for the same effective question** — requirements.md
   already scopes this ("Unique-index + status-guard duplicate handling for
   concurrent creation/double-answer"); `BacklogStuckState`'s
   `index.Fields("item_id", "reason").Unique()` + resolve-in-place upsert is the
   direct template, but note `GuidanceRequest` doesn't have a natural "reason" enum
   to dedupe on the way `BacklogStuckState` does — the plan phase must define what
   the unique key *is* (likely `(scope_type, scope_id, status=pending)` as a partial
   index, since unlike stuck-state, two *different* pending questions against the
   same item are legitimate and must not collide).
5. **Double-answer race** — two near-simultaneous answer submissions (e.g. a
   double-click, or a stale browser tab resubmitting) must not both "win." A
   status-guard compare-and-swap (`UPDATE ... WHERE status = 'pending'`, checking
   rows-affected) is the standard defense — ent's optimistic concurrency or an
   explicit `WHERE` predicate on the update, not an app-level read-then-write.
6. **Never-answered request accumulates forever** — no expiry/auto-resolve is in
   the ACs, but the per-scope pending cap (in scope) interacts with this: if nothing
   ever ages out or gets manually resolved, a permanently-ignored request could
   itself become the reason a *later*, more urgent request against the same scope
   is rejected by the cap. Flag as a plan-phase decision point (a TTL is explicitly
   avoidable per Out of Scope's "Editing/retracting," but a cap-exhaustion deadlock
   from an ignored request is a different, in-scope concern worth naming).
7. **Answer arrives while triage is mid-tick on the same item** (a race between the
   human answering and a reconciler sweep touching the same `BacklogItem` row) —
   `transitionWithGuard`'s precondition-checked status transitions
   (`backlog_service_triage.go:741`) are the existing pattern for "don't let two
   writers stomp the same item's status concurrently"; resuming a halted triage item
   on answer should reuse that guard rather than an unconditional status write.
8. **Multiple-choice options changing meaning is impossible by construction** (good
   — "Editing/retracting a pending question" is out of scope, so options are
   immutable once created; no edge case to design against here, just confirms the
   scope boundary is coherent).
9. **Standalone-scope requests have no natural owner for the per-scope cap or for
   archival cascade** — sections 2 and 6 above (cap, archive-cascade) both implicitly
   assume a scope with a lifecycle (item archived, session deleted). A `standalone`
   request has neither: define its cap bucket explicitly (global? per-creating-tool?)
   rather than leaving it as a fall-through of the item/session logic.

## 4. Unstated needs beyond the explicit ACs

- **Answer must be visible in context to whatever reads it next**, not just
  queryable — echoing edge case 1, the real user need behind "durable notification"
  isn't just "ping something," it's "the next agent that picks up this work
  automatically sees the Q&A in its prompt/context," the same way
  `initialPromptFor`/`recentReviewHadVerdict` already assemble prior-session history
  into a fresh session's starting prompt. If a plan only wires up the MCP
  read-back tool and the event-bus ping but doesn't touch the prompt-assembly code
  path, the feature will technically satisfy the CRUD ACs while still failing the
  actual "resumed session picks up where it left off" user story.
- **The human needs one place to see all pending questions across all three
  scopes**, not just per-item/per-session panels — requirements.md scopes the
  shared *component* to 3 views but says nothing about a cross-cutting "all my
  pending guidance requests" list (the `/unfinished`-style aggregate view
  `BacklogStuckState` already gets). Given the operator is solo and this is meant to
  reduce "guess or block forever," a request that's easy to miss because it's
  buried on one specific item's detail page defeats the purpose. Worth raising to
  the plan phase as a candidate scope addition (or explicit, justified exclusion)
  rather than an implicit gap.
- **Distinguishing "urgent, blocking real work" from "low-stakes, answer whenever"**
  — triage's halt-and-wait (AC3) is inherently higher-urgency than an interactive
  session politely asking a side question. The single shared entity/component
  doesn't preclude a priority/urgency signal, but nothing in requirements.md asks
  for one; the existing `Notifier` priority parameter
  (`fakeNotifier.Notify(itemID, title, message string, notificationType, priority
  int32)`) already supports differentiated urgency — using a flat priority for every
  `GuidanceRequest` notify would under-signal the triage-halt case relative to what
  the codebase's own notification transport already supports.
- **Auditability of "why was this asked"** — a `GuidanceRequest` created by
  automated triage under a feature flag needs enough context in the row (or a link
  back to the triage run/session) that a human answering it six hours later, out of
  context, can tell *why* the ambiguity arose — otherwise the human is asked to
  answer a yes/no/multiple-choice question with no memory of the item's state at
  question-creation time. `BacklogStuckState.context` (free-text "why" string) is
  the direct precedent to carry forward here, not just the question text itself.
