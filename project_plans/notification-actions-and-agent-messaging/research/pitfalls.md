# Pitfalls: notification-actions-and-agent-messaging

Labels: VERIFIED = source/commit opened this session. INFERRED = reasoning from the cited material. All paths are repo-relative. Prior art mined: `project_plans/notification-tray-and-hidden-session-gate/` (ADR-010, research/pitfalls.md), `project_plans/durable-guidance-request/research/pitfalls.md`, `project_plans/backlog-event-subscribe/research/pitfalls.md`, `project_plans/pr-fix-steering/research/pitfalls.md`, `project_plans/backlog-agent-communication/research/pitfalls.md`, `docs/explanation/*`, `.claude/rules/*`, repo memory notes.

Headline: a hidden-session Reply path already exists and has been through five review-repair iterations (ADR-010, `server/services/hidden_question_reply.go`). The new feature should generalize that machinery (claim, `DialogMatch`, write lease, `SubmitReplyOnce`), not build a second keystroke path.

## 1. Answering a stale or already-resolved prompt

| Pitfall | Evidence | Design rule |
|---|---|---|
| Wrong dialog on screen. A card for question A gets answered while the pane shows question B or a permission dialog; a digit then approves something. | ADR-010 decision 2d-2e (`project_plans/notification-tray-and-hidden-session-gate/decisions/ADR-010-audited-reply-to-pending-question.md`). Two parallel `AskUserQuestion` hooks fire before either is answered and no hook fires when the second appears, so "newest hook wins" is wrong. | Identity is a positive structural match of a fresh `tmux capture-pane` (`DialogMatch`), never the cached status (blank 6-9 s after a keypress) or the hook stream. Mismatch returns `STALE_PROMPT` with zero writes. |
| Forged or agent-controlled text inside a permission dialog reproduces header, question and option rows. | ADR-010 2d "bottom anchor": a numbered row or `Do you want` after the last footer line is no-match. | Reuse the bottom anchor. Layout is pinned to Claude Code v2.1.296 rendering; any other layout fails closed. Other harnesses (Aider, Codex, Gemini) need their own measured fixture; until then they are not replyable (INFERRED). |
| Double answer: two tabs, or UI plus terminal plus MCP. | Guidance pitfalls section 1 (`project_plans/durable-guidance-request/research/pitfalls.md`): answer must be `UPDATE ... SET answered_at=now WHERE answered_at IS NULL`, loser is a no-op. ADR-010 decision 3: singleflight by `reply_id`, result cache keyed to SHA-256 of (session UUID, question_id, digit). | Every answer path (notification pane, session view, MCP, Slack) funnels through one idempotent resolve. Same `reply_id` with a different payload is `InvalidArgument`. A cached result is returned on a double tap, never `RATE_LIMITED`. |
| Stale cards in other views keep showing an open form. | Guidance pitfalls section 5; `ssq#428` polling baseline. | Treat the server as truth; every card re-fetches on focus and handles `NO_PENDING`/`STALE_PROMPT` as "answered elsewhere" UI, not an error toast. |
| Wrong-session attribution. `resolveSessionID` falls back to longest cwd-prefix, and a hidden review session can share a path with a visible one. | ADR-010 2a (`server/services/approval_handler.go:841-870`). | Register a replyable question only when attribution came from `X-CS-Session-ID` or a verified hook proof; `cwd`/`unknown` is view-only. Claim requires `inst.UUID == pending.SessionUUID`; `MatchesID` also accepts titles and tmux names (`session/instance_terminal.go:72-80`), so resolve by UUID. |
| Re-asked identical question answered by an old card. | ADR-010 2e (accepted residual: wrong answer, never a permission approval). | Accept and document; do not add supersede logic. |
| Answering a dead session's question. | Guidance pitfalls section 2 and 6. | Dead session: write the durable record only, mark delivery `undeliverable`, surface it on the item (`feedback_document_ai_decisions_in_edge_cases`). Cancel pending requests when the owning scope is archived; an unanswerable `pending` row is indistinguishable from a live one. |

Gap: ADR-010 decision 5 limits Reply to single-select, 1-7 options, one digit. Free text, `multiSelect` and multi-question payloads are explicitly "answer in the terminal". Requirements success metric 3 ("structured question with option list") is satisfiable; free-text and multi-select inline answering would need a new measured spike per harness, not an extension of this ADR.

## 2. Keystroke injection races into tmux

- Single-write `content+"\r"` is folded into Ink's paste-detection window and never submits (BUG-031). Fixed by two-write submit in #832 (`e2085dec4`, `4d2fab668`); structural guard `session/sendkeysguard/sendkeysguard.go` fails the build on the old shape in `server/mcp`, `server/services`, `session/tymux`. Any new inline-answer code must go through `session.SubmitContentWithEnter`/`SubmitReplyOnce`, never raw `SendKeys` with an Enter byte.
- `SubmitDriverContent` writes a blind second Enter on no pane change (`session/pane_submit.go:126`). Fine for free text, harmful for selection dialogs (the extra Enter picks the default of the next dialog). Option answers must use one digit and no Enter (ADR-010 decision 3, "one digit, one write"). Note #832's own commit message says numbered-selection delivery was left as a separate follow-up (backlog item `e08ad709-6ade-48cf-81f9-f577b53c1b10`).
- "It returned an error" does not mean "nothing was typed": the wrapper returns `DeadlineExceeded` at 8 s while the inner goroutine may already have written (ADR-010 context). Abandoned submit goroutines kept firing retry Enters after the caller moved on until #832 added ctx checks before each write. Classify outcomes as `NOT_SENT` (strictly before first byte, safe to retry) vs `SEND_INDETERMINATE` (never auto-retry).
- Concurrent writers to one pane: the autonomous driver (`sendAnswerKey` sends `"1\n"` into `NeedsApproval` dialogs, `session/session_driver.go`), idle nudges, `performNudge` (`server/mcp/tools_diagnose.go`), and the in-process stall nudger. `docs/explanation/autonomous-stall-causes.md` records that the write lease serializes writes but not intent. Use the per-instance `TerminalWriteLease` (`Try`, no wait); a human answering from the pane while a nudge fires still collides, so an inline answer must also re-check the dialog under the lease.
- Controller not wired: MCP-created sessions never opened in the UI fail `send keys` with "instance that has not been started or is paused" (`project_plans/pr-fix-steering/research/pitfalls.md` section 1, `docs/tasks/backlog-feature-improvement.md:480`). An inline answer to such a session must report `NOT_SENT`, not silently succeed.
- Bounded limiter state: per-session 1 reply/5 s, global 10/min, LRU 1024 (hidden_question_reply.go constants). Reuse; do not add a second limiter set.
- Test with a fault-injecting `paneSubmitter` per stage and a real-capture fixture, not mocks of "dialog matched" (ADR-010 decision 9).

## 3. Approval-gate bypass and caller identity on MCP tools

- `--allowedTools` is client-side only. #880 shipped Diagnose & Nudge with no server-side gate; #894 (`b1df034f2`) added `denyIfDiagnoseCaller`/`withDiagnoseGate` around `write_to_session`, `send_control`, `run_command`, `steer_session`, `resume_session` (`server/mcp/diagnose_role_gate.go`, `tools_terminal.go`, `tools_lifecycle.go`). Memory: `instinct_mcp_dispatched_agent_tool_surface`.
  - Implication: any new MCP tool ("answer peer's question", "wake session X", "schedule wake") is reachable by every dispatched role (triage, review, diagnose). Each needs a server-side caller-identity check at the handler, written at plan time. A peer-help answer tool is a write into another session's context: restrict to the named asker/answerer pair, reject self-answer and cross-item answers, and have the gate wrapper registered with a test that fails if the handler is unwrapped (the #894 test used `MCPServer.GetTool`, not JSON-RPC).
- Local listener `:8543` has no auth at all (`SetupAuth` has no non-test caller; `localChain` runs `ProbeGuard` only, ADR-010 context). Any local process can POST hook payloads and call RPCs. ADR-010 2g: hook HMAC proof is provenance, not the guard; `DialogMatch` is the guard. Do not claim inline actions are "authenticated" for the operator on `:8543`; ADR-010 had a review blocker (ARCH-B2) for exactly that false claim. The `:8444` remote chain has auth and a DNS-rebinding Host allowlist (ADR-010 decision 7).
- Approval cards: `clearAll()` clears approvals too (VERIFIED in tray pitfalls section 6). Bulk actions and "mark all read" must exclude unresolved approvals (ADR-008 in the tray plan, `pending_approvals.json` reconciliation).
- Server-side read-only for hidden sessions: UI-only read-only is insufficient; `WriteToSession` exists on `SessionService`, `TerminalService` and websocket input (tray pitfalls section 2, ADR-005). Reply and the backlog steer are the only sanctioned exceptions; a new inline action that writes to a hidden session needs its own ADR amendment, not a quiet reuse of `UpdateSession`'s steer branch.
- Session identity for MCP callers: wake and message tools must derive the caller from `callerSessionUUID`, not a caller-supplied `session_id` argument (INFERRED from the #894 pattern).
- Prompt injection: peer messages and guidance answers land in another agent's context. Escape/delimit them (precedent: `lastNudgeText` angle-bracket escaping in `bdfd9479b`), and label provenance ("from session X", "from operator").

## 4. Wake storms and loops

Real incidents:
- Idle-wait loop: a session re-read its entire cached history on every trivial "still waiting" wake turn; one headless triage call cost $106.69 (242 Monitor + 293 ListAgents calls for 262 completions). Mitigated by `IdleWaitTurnCeiling` (default 15) plus `/compact` and escalation in #885 (`59bbcf33f`); prompt-level batching in #883. Prompt text is a mitigation, not a cap. Wake triggers must carry a hard per-session rate cap and a batch/coalesce window; "wake on every event of type X" needs a debounce.
- Fixed-cadence re-nudging: autonomous driver re-steered every idle-settle interval after the agent had said it was waiting. Fixed with exact/near-exact dedup, a WAIT directive and a cancellable cooldown (`bdfd9479b`, `dab799362`). A wake trigger that fires into a session that already answered "waiting" re-creates this.
- PR-pending churn: `AutoReopenForPRFix` flipped `pr_pending`<->`in_progress` every 60 s for a live 4-hour session (`af426f27`, `f8f788ab`); fix was to check liveness/dedup before any side effect (`project_plans/pr-fix-steering/research/pitfalls.md` section 1). The dedup bound "one redundant message per reason change" only holds if the reason signal is stable (gh `mergeable` is not, section 6 there).
- Retry without backoff: BUG-041 (backlog nudge retry never backs off); reuse `RemediationDue`, do not invent a parallel schedule (`project_plans/backlog-agent-communication/research/pitfalls.md`).
- Chained wakes: a wake whose handler schedules another wake (agent self-scheduling in its woken turn) is unbounded unless the trigger carries a generation count or a per-source cap. INFERRED; no repo incident, but #885 is the cost shape.
- Cron / delay triggers firing into a busy session: a wake is a prompt injected through the write lease; queue it for the next idle point rather than typing over a working agent (INFERRED from the lease/intent split).

Also log: wake scheduled/fired/skipped/missed per trigger type (requirements Observability); "skipped, session archived" must be a visible state.

## 5. Missed wakes across service restart

- In-memory notify-once maps were the root cause of repeated/lost notifications on restart (15+ restarts/day on the dev instance): ADR-001 of `project_plans/backlog-stuck-item-visibility` (`session/backlog_lifecycle.go:119-133`). Triggers must be an ent row (one unique non-null key, resolve-in-place, `OnConflictColumns` upsert), with `fired_at`/`notified_at` as separate idempotent nullable timestamps; plus a self-heal sweep as the correctness backstop (guidance pitfalls first section).
- Restart-recovery sweep must check the durable `fired_at` before re-firing, or every boot re-wakes every pending trigger (guidance pitfalls section 2, "double notification on replay").
- Subscribe-before-read for event triggers: evaluate "condition already true?" after subscribing, else an event published in the gap is lost; `after_seq` does not help a first-call waiter (`project_plans/backlog-event-subscribe/research/pitfalls.md` section 2). The EventBus is in-memory only, so on restart the sweep must evaluate current state, not rely on replayed events.
- MCP stdio fallback path passes `eventBus=nil` (`main.go:104`); a wait/schedule tool must return a distinct "event stream unavailable" error, not a timeout that looks like "no event" (backlog-event-subscribe section 5). Same goes for the window while the daemon restarts.
- Deploy hazard: `make install-service` restarts the service and kills the tmux server unless `--tmux-keep-server` is set (`docs/explanation/tmux-keep-server-on-restart.md`); a wake aimed at a session whose pane died must degrade to revive-or-notify per policy, not write into a stale target.
- `ScheduleWakeup`/`scheduled_tasks` is a Claude Code harness feature, not a Go scheduler in this repo. I found no `.go` references to it (grep for `ScheduleWakeup|scheduled_tasks` in `*.go` returned nothing); `scheduled_tasks` appears only as a session-process lock file in commit `e2085dec4`. GAP: whether the harness's own wakeups survive a stapler-squad restart or a Claude Code restart was not verified here; the design must not depend on it.
- Service-restart orphan processes can race over tmux/session state (`docs/explanation/service-restart-orphan-process.md`); a scheduler goroutine must be joined on shutdown (`5fd2a25df`, "join every background goroutine wireDepsIntoServer starts") and use a single-leader guard on restart overlap.

## 6. Reviving archived sessions

- Rule: every automated `Start(false)`, `RecoverFromStopped()`, `restartForRetry` or `transitionToLocked(..., Active)` in the lifecycle files needs an archival check first (`.claude/rules/noarchivedrevival.md`; analyzer `tools/lint/noarchivedrevival`; scope files `session/health.go`, `review_queue_poller.go`, `session_driver.go`, `instance_claude.go`, `instance_serialization.go`, `server/dependencies.go`). A wake path that revives a stopped target is a new site; either put it in a guarded function or use `//nolint:noarchivedrevival <reason>` only for an explicit operator action.
- PR #810: seven auto-revival sites needed guards, found over four review rounds.
- One-shot respawn loop: a Stopped `backlog:triage`/`backlog:review` session respawned 230+ times (~60 s cadence) because `fromInstanceData` revived it; guard needs `ArchivedAt != nil || isOneShot(instance)` (`docs/explanation/oneshot-session-respawn-loop.md`). Wake targets must exclude one-shot and terminal-item sessions, not just archived ones. Its evidence also shows `archived_at IS NULL` on a session whose item was archived, so checking archival alone is insufficient; check item terminal state too.
- Archive-on-terminal did not kill the tmux pane (2026-07-29 OOM, `project_2026_07_29_oom_session_leak_fix`): do not assume "archived" implies "pane gone" or vice versa.
- Do not treat "delivery failed because session archived" as retry-forever; resolve as undeliverable and visibly (guidance pitfalls section 2).
- Reads of `*Instance` fields go through `Snapshot()` (`.claude/rules/instance-lock-free-reads.md`); use `Workspace().ActiveDir` to compare sessions (false collisions in `WorkspacePeersPanel`, PR #801).

## 7. Check-then-reserve races

- A per-item lock does not protect a global cap. Memory `instinct_check_then_reserve_needs_process_wide_lock`: #880's diagnose nudge counter was read-increment-write with no lock (fixed in #894 with `diagnoseNudgeGuardMu`, `nudge_cap_store.go`); same class as the Jules spend-cap race (`5a187795b`). Applies to: per-session wake caps, global wake budget, peer-help request caps, pending-question caps (guidance pitfalls section 4: use COUNT in the same transaction or post-commit revalidation).
- Add `-race` tests asserting exactly N successes out of M concurrent attempts at cap N. Also test the boundary (`>` vs `>=`).
- Backlog rework cap was only enforced per caller until `1ef546463` ("enforce the rework cap in the shared spawn gate, not just per-caller"). Wake-triggered spawns of backlog sessions must go through the same shared spawn gate, not a second spawn path.
- A cap that silently refuses a new help request makes agents resume guessing; the refusal must be a visible notified state (guidance pitfalls section 4).
- Unique indexes on SQLite: NULL is distinct, so never include a nullable column (e.g. `resolved_at`) in the dedup key (ADR-001 stuck-item-visibility).

## 8. Notification dedup interplay

- Store dedups by `(sessionID, notificationType)`, and `APPROVAL_NEEDED` collapse updates the record ID to the incoming UUID so outcome-stamping still finds the record (`server/notifications/store.go:24-29`, via guidance pitfalls section 2). A structured question card that collapses into an existing unread record can leave its action bound to a stale ID. Stamp `question_id` in metadata and key actions on it, not the notification ID.
- `project_plans/smart-notification-dedup/requirements.md` NFR-1: `APPROVAL_NEEDED` and `INPUT_REQUIRED` must never be suppressed by dedup; FR-4 closes a previous native notification for the same `(sessionId, type)`. Two distinct questions from one session are both actionable; a naive `(session,type)` collapse drops the first card while its claim is still live. Dedup by `question_id`; collapse only identical question text and labels (ADR-010 2e).
- FR-3 gives actionable native notifications a 5 min lifetime; the claim TTL is 30 min (ADR-010 2f). An action on a card that outlived its claim must resolve to `NO_PENDING`/`STALE_PROMPT` gracefully.
- Cross-tab: sync only idempotent, id-set based payloads ("these ids resolved"), never "clear everything now"; BroadcastChannel does not echo to the sender (tray pitfalls section 7). Optimistic UI needs rollback on RPC failure.
- Toasts and native pushes must follow the same rule (FR-6); the second push path `push/subscriber.go` has no hidden handling today (tray pitfalls section 1).
- Dedup timers use `showUndoToast`/`showActionToast` with uncancellable `setTimeout`; an inline action toast auto-dismissing while the user reads it violates WCAG 2.2.1 (tray pitfalls sections 7-8).

## 9. Delivery-gate suppression of hidden sessions

- Why the Background tab is empty is partly the gate: 283 hidden occurrences were in history before the gate (166 from one spamming review producer, `capacity_monitor.go:420`); the gate (`a9b19d863`, #974) now suppresses routine classes for hidden sessions with counters `notification_delivery_suppressed_total` / `_would_suppress_total` (`docs/reference/notification-delivery-gate.md`). Requirements Open Question 1 can be answered cheaply: query those counters and the `delivery_suppressed` log lines before assuming a producer bug.
- Gate keys on typed class (failure | needs-human | routine), not priority; ceiling events arrived at both priority 1 and 3. Any new notification type (wake fired, peer help requested, question created) needs a class mapping or it fails open and floods, or is misclassified routine and is dropped for hidden sessions. Unresolved events fail open by design.
- Hidden-session "needs human" must reach the operator or the hidden review question is a dead end (ADR-010 context). A hidden session's peer-help request and a wake-failure are needs-human/failure class, not routine.
- Gate placement: one shared classifier at bus publish (ADR-001 of the tray plan). New producers calling the store directly bypass it. Add the enumerating test (every event type x channel with a Hidden session) before adding event types.
- Log volume: "log, not suppress" for hidden failures floods logs for a spamming producer; the gate rate-limits its own log to once per 60 s with `suppressed_since_last`.
- Flag consulted inside the gate so one live flip restores prior behavior across all channels. Rollout flags must be live-settable (`feedback_rollout_flags_live_settable_no_env_vars`); the guidance pitfalls note that triage reads flags at decision time, not cached at pass start, and that no per-scope override precedence exists in `server/features/flags.go`: global+per-scope precedence for the four requirements flags needs its own explicit rule and test.

## 10. WIP cap and resource limits

- Memory `feedback_backlog_wip_limit`: cap concurrent backlog work sessions at 2; the 2026-07-12 kernel OOM (57/61 GB, swap exhausted) came from concurrent agent processes. A wake trigger that spawns/revives sessions is a dispatch path and must check in-progress count first, through the shared spawn gate (section 7).
- Idle sessions kept alive for wake targets consume memory; the OOM of 2026-07-29 was a session leak. Wakes for sessions with no pane should queue as "needs revive" under the cap, not revive in bulk after a restart (a restart with N pending wakes would otherwise spawn N sessions at once; INFERRED).
- Parallel workers in one worktree swept each other's hunks and filled the disk (`feedback_parallel_workers_one_worktree_sweep_each_other`); implementation waves need separate worktrees.

## 11. CI gates: jscpd, proto/ent, Playwright

- jscpd: `web-app/.jscpd.json` threshold 0.14%, minLines 20/minTokens 200 (CLAUDE.md). Heavily-mocked test files add irreducible `jest.mock` baseline; memory `project_plan_feedback_ship_learnings` put the margin at 0.10% after #815. Inline-action card variants (question, approval, backlog decision) risk forked components and copy-pasted hooks; extract one action-row component and one resolve hook up front (tray pitfalls section 10 listed the same three duplication sources). Run `pnpm run lint:duplicates` before pushing. Reconcile CLAUDE.md's stated figures (0.12 in history text, 0.14 current) with the file when budgeting.
- Go `dupl` gate is new-code-only vs `origin/main`; gocognit/funlen apply to modified existing functions, and `NotificationService.SendNotification` is already ~110 lines (tray pitfalls section 10).
- Generated code: `gen/` proto output and `session/ent/*.go` are gitignored; never `git add -f` (incident: missing generated ent package left main broken). Ent generate must use `--feature sql/upsert` or `UpsertRule`-style methods break. Fresh worktrees need `make build` first. A new `StuckReason`-like enum needs proto wiring as well as the domain enum (`instinct_alias_session`). Run `make registry-generate` for every new RPC/component/marker; CI "registry out of date" is usually a stale base (`instinct_ci_tests_against_live_base_not_stale_merge`; #905).
- New RPC enum values need `*_UNSPECIFIED = 0` and the type prefix under buf `STANDARD` lint (ADR-010 decision 1).
- Playwright (skill `e2e-test-conventions`, `tests/e2e/`): header `// @feature ...`, no `waitForTimeout` (toast and tray animations: `expect(...).toBeHidden()` or `page.clock`), `data-testid`/ARIA locators only, helpers in `tests/e2e/pages/` (extend existing `NotificationPanel.ts`). The harness spawns an isolated server on an ephemeral port; answering a question end to end needs a fake-pane fixture or a real private-tmux-socket session in that server, not the live `:8543` one. Swipe/touch tests need `hasTouch`.
- Manual testing: only the isolated instance (ports 62871/62872, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1`); never `make install-service` (CLAUDE.md WARNING).
- Review-repair loops: ADR-010 took five iterations and per memory `feedback_review_repair_loops_overbuilding` repeated new blockers in one subsystem mean simplify; set a stop rule up front. Also `feedback_comment_proportionality`: keep rationale in commit messages.

## Top risks, ranked

1. A new answer or write path that bypasses the claim + `DialogMatch` + lease + audit chain (wrong-dialog keystroke, including approving a permission prompt).
2. New MCP tools (peer help, wake, schedule) with no server-side caller-identity gate, reachable by dispatched triage/review/diagnose roles.
3. In-memory wake state or non-idempotent restart sweep: missed or doubled wakes on every deploy.
4. Wake loops and revive storms: no hard per-session cap, no archived/one-shot/terminal-item exclusion, spawn outside the shared gate/WIP cap.
5. Notification dedup collapsing two live questions or binding an action to a stale record ID; new notification types unclassified in the delivery gate.
6. Check-then-reserve caps with per-item locks only.
7. CI: jscpd margin, committed generated code, missing registry regeneration, e2e convention violations.

## Gaps (not verified)

- Whether Claude Code's native `ScheduleWakeup` persists across restarts; no Go-side scheduler exists to inspect.
- Native answer channels for non-Claude harnesses (requirements Open Question 2): ADR-010's `DialogMatch` is pinned to one Claude Code rendering; others are unmeasured.
- Whether the gate or missing producers explain the empty Background tab was not queried live here; use the gate counters first.
- Ownership-check precedent for "session X owns scope Y" does not exist in the ent schemas (guidance pitfalls section 6); peer-help authorization needs a new explicit model.
