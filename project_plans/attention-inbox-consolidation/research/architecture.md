# Architecture research: attention-inbox-consolidation

Date: 2026-10-23. Labels: VERIFIED = source opened or command run this session; INFERRED = reasoned from code, not executed; GAP = not checked.
Paths are relative to the repo root. Prior plans consulted by listing only (not re-derived): `review-queue-state-detection`, `review-queue-event-driven` (ADR-001 coalescing status channel), `notification-tray-and-hidden-session-gate` (ADR-001..010), `smart-notification-dedup`, `backlog-item-activity-log` (ADR-001 sibling ent table).

## 1. Pipeline map (as built)

```
 pane text ──> detection.TerminalDetector (per-binary patterns) ─┐
 controller (idle state, ClaudeStatus, PendingApprovals) ────────┤
                                                                 v
 ReviewQueuePoller.checkSession ──> DefaultStatusDeterminer.Determine (pure)
        │  shouldSkipSession, ack snooze, grace, re-add limiter
        v
 session/queue ReviewQueue ──observer──> ReactiveQueueManager.OnItemAdded
        │                                   ├─ gRPC/Connect WatchReviewQueue ──> web useReviewQueue (hides IDLE) + useReviewQueueNotifications (tiers)
        │                                   ├─ EventBus.Publish(EventNotification) ──[delivery gate]──> NotificationHistoryStore (JSON file, 7d) ──> tray/push
        │                                   ├─ Slack (gate + cfg.NotifyOnQueueItem) / webhook callback (gate)
 Claude hooks (separate path):
   Stop ─> scripts/ssq-hook-handler stop ─> ssq-notify ─> "Task Complete" LOW task_complete ──> same EventBus
   Notification ─> ssq-hook-handler notification ─> INFO/medium ("Claude is waiting for your input") ──> same EventBus
   PreToolUse(AskUserQuestion) ─> INPUT_REQUIRED URGENT "Claude has a question"
   HTTP /api/hooks/stop ─> HookReceiver.HandleStop ─> drains body, logs, returns 200 (no-op)
```

## 2. Drop points

"Finished" = agent turn ended, idle at prompt, awaiting next instruction. "Prose" = last assistant text is a question, no AskUserQuestion dialog.

| # | Where | What it does | Finished/waiting | Prose question | Label |
|---|---|---|---|---|---|
| D1 | `scripts/ssq-hook-handler:448-463` (Stop) | Always emits `Task Complete`, `-p low --type task_complete` unless stop_reason matches max_tokens/error. Does not read transcript or last message. | Emitted, but generic text "Task completed" and LOW; routine class | Same event as a normal finish; the question is not in the message, so it is indistinguishable | VERIFIED (script) + real data: history shows many `TASK_COMPLETE / LOW / "Task completed"` rows |
| D2 | `server/services/hook_receivers.go:64-70` | `HandleStop` only drains the body and logs `[hook/stop]`. No state change, no event. | Nothing | Nothing | VERIFIED; `grep -c "hook/stop" ~/.stapler-squad/logs/service.log` = 0, so this endpoint is also not being hit in practice |
| D3 | `scripts/ssq-hook-handler:430-444` (Notification) | Maps Claude's Notification hook to `--type info`, priority medium. "Claude is waiting for your input" (the idle prompt Claude emits ~60s after a turn ends) becomes INFO = routine. | Delivered as INFO to visible sessions only (`policy.go:136`-style routine class is suppressed for hidden); no needs-human class | Same | VERIFIED script; history rows `INFO/MEDIUM "Claude is waiting for your input"` (4f8f96e2, 2026-10-23T19:40Z) |
| D4 | `session/detection/binaries/claude.go:421-460` InputRequired | Only numbered-option selector (`[❯●]\s*\d+\.`), opencode bar options, opencode allow-once buttons. Prose never matches. | n/a | Never StatusInputRequired | VERIFIED (patterns in that range) |
| D5 | `session/detection/binaries/claude.go:198-215` Idle, `:315-` Success | `—INSERT—` and `^>\s*▌?\s*$` give StatusIdle; cost line / "verb + duration" gives StatusSuccess. Scan is bottom-up (`review_queue_determiner.go:219-224`), so a trailing prose paragraph (no pattern) falls through to whatever marker is above it, or Unknown. | Success marker => TaskComplete; Idle prompt alone => Idle (no queue add) | Classified as the finished/idle state of the turn, never as a question | INFERRED (bottom-up semantics from comment; not executed against a fixture) |
| D6 | `review_queue_determiner.go:166-171, 253-258` | StatusSuccess => `ReasonTaskComplete`, `PriorityLow`. | Item created, LOW | Same | VERIFIED |
| D7 | `review_queue_determiner.go:185-205` controller branch | `IdleStateWaiting` => `shouldAdd=false` ("normal idle"). Only `IdleStateTimeout` adds `ReasonIdle` LOW, and not when `suppressedByAck`. | Finished-but-no-Success-marker sessions add nothing until timeout | Same | VERIFIED |
| D8 | `review_queue_determiner.go:276-288` no-controller | `time.Since(inst.UpdatedAt) > 5s && !ack` => `ReasonIdle` LOW. The idle clock is `UpdatedAt`, which is bumped by non-output events (D12). | Mis-timed in both directions | Same | VERIFIED |
| D9 | `review_queue_determiner.go:172-181, 261-272` | `StatusWaitingForAgent` (bg shells/monitors in the auto-mode footer, `detector.go:70-82` footerAgentCount) => Remove while `UpdatedAt` < 30 min (`:19`). | Suppressed up to 30 min after any `UpdatedAt` bump (note save `instance_actor_setters.go:616`, rename `instance_terminal.go:116`, restart `instance.go:2700`) | Same | VERIFIED. Only fires when the footer reports >0 background tasks |
| D10 | `review_queue_poller.go:902-905` + `review_queue_determiner.go:80` | Ack snooze: item removed when `LastAcknowledged > LastMeaningfulOutput`. Bypass is `priority <= PriorityHigh` (`:901`); comment at `:896-900` says input is bypassed, but INPUT_REQUIRED is Medium (Urgent=1,High=2,Medium=3,Low=4 per `:931`), so it is not. | Low items snoozed until new output | INPUT_REQUIRED snoozed too (code/comment mismatch) | VERIFIED |
| D11 | `review_queue_poller.go:912-921` | After ack, LOW or no-controller items are not re-added for 5 min. | Suppressed 5 min | Same | VERIFIED |
| D12 | `review_queue_poller.go:927-942` | Re-add limiter 2 min, only when item already in queue; escalation bypasses. | Minor | Minor | VERIFIED |
| D13 | `review_queue_poller.go:819-829` `shouldSkipSession` | Skips Hidden, suspended (Paused/Hibernated/Stopped/Crashed/PermanentlyFailed), archived, `!Started()` with no log line. | Silent drop | Silent drop | VERIFIED (matches requirement: observability gap) |
| D14 | `review_queue_determiner.go:344-348` | Hidden + TaskComplete/Idle/Stale => Skip. Redundant with D13 for the poller; matters for `StartupScanner`. | Dropped | Dropped | VERIFIED |
| D15 | `web-app/src/lib/utils/reviewQueueVisibility.ts:9` | `isReviewQueueVisible` hides `AttentionReason.IDLE` from page, badge, nav. | The only reason the fallback path can produce for "finished" is invisible | Prose question lands (at best) as IDLE => invisible | VERIFIED. Strongest single cause for requirement gaps 1 and 2 |
| D16 | `web-app/src/lib/hooks/useReviewQueueNotifications.ts:25-47` | Tier 3 (TASK_COMPLETE, IDLE, UNCOMMITTED, IDLE_TIMEOUT) = history only, no toast, no OS notification. | No interrupt | No interrupt | VERIFIED |
| D17 | `server/review_queue_manager.go:963-982`, `server/deliverygate/policy.go:136` | TaskComplete => NOTIFICATION_TYPE_TASK_COMPLETE; Idle/Stale/IdleTimeout => INFO; both class "routine". Hidden sessions get nothing on the bus. Priority is not a policy input (ADR-002). | Visible: goes to history/tray; Hidden: dropped | Same | VERIFIED |
| D18 | `server/notifications/store.go:181-230, mergeOccurrence` | Append collapses by (session, type[, question]); only count, last message and unread flag survive. 7-day retention (`:20`), JSON file. | A second finish replaces the first; no per-completion trail | Prose-question text is lost if it shares a key with a later "Task completed" (different types, so no collision, but nothing carries the question) | VERIFIED |
| D19 | `docs/explanation/askuserquestion-answer-routing.md` ladder | Covers only the `AskUserQuestion` tool dialog (`hook_reply` rung closed by compile-time `spikeS1HookReplyPassed=false`). Prose has no row, no ANSWER_QUESTION action. | n/a | Out of design scope | VERIFIED (doc) |
| D20 | `review_queue_poller.go:1048-1049` / `:709-727` | Item add depends on the poll/activity-signal loop (ADR-001 event-driven coalescing channel). Not re-verified here. | | | GAP |

### Real data (read-only)
- `get_notification_history(limit=30)`: total 500, unread 290. Per-session pattern for a normal finish (c6dde3e9, 2026-10-23): `STATUS_CHANGE/LOW "Uncommitted Changes"` 18:41:24, `INFO "Claude is waiting for your input"` 18:41:31, `TASK_COMPLETE/LOW "Task completed"` 19:03:58. So a finish does fire, but as three LOW/INFO rows with no content, no priority above LOW, and no "waiting on you" class.
- A real AskUserQuestion this session produced `INPUT_REQUIRED/URGENT "Claude has a question"` with the question text (87d3ec0d, 21:25 and 21:30). The tool path works; the prose path has no equivalent.
- Stale detection arrives at ~28-30 min as `INFO "stale: ..."` plus `WARNING "Session went stale"` (e.g. tymux 2026-10-18T00:29Z / 01:18Z), i.e. an idle-waiting session is only noticed half an hour late.
- `~/.stapler-squad/logs/staplersquad.log` last written 2026-09-05 (stale); `service.log` is current but queue-add lines are Debug level and absent. 0 `[hook/stop]` lines. Could not reconstruct per-session queue decisions from logs (GAP; needs debug level on an isolated instance).
- GAP: whether web push / OS notification fires for TASK_COMPLETE on a visible session (not traced past the bus).

### Root-cause statement (hypothesis, supported by D1-D17)
A finished-and-waiting or prose-question session has no first-class signal at any of three layers: (1) screen detection has no "turn ended with a question" class (D4-D5); (2) the hook channel that does know the turn ended (Stop) discards the last message and always emits LOW "Task completed" (D1-D2); (3) whatever fallback survives is `ReasonIdle`/`TaskComplete` LOW, which is snoozed by ack (D10-D11), clocked by the wrong timestamp (D8), hidden by the UI filter (D15) and kept out of the interrupt tier (D16-D17). Fixing detection alone would still be invisible because of D15/D16.

### Secondary check (suspended sessions)
`shouldSkipSession` (D13) skips Paused/Crashed etc. by design; the operator clarified "paused" is not that. Autonomous runs already emit `FAILURE "Autonomous fix stuck"` for exited sessions (history), so crashed ones are partly covered elsewhere. Defer; add only the skip-reason counter.

## 3. Proposals

### (a) Better detection
1. **Hook-first turn-end signal.** Make `HookReceiver.HandleStop` (D2) real: parse `transcript_path`/last assistant message (field names INFERRED from Claude Code hook docs; verify in a spike), classify the last assistant text (question heuristics: trailing `?`, "which/should I/do you want", options list, no tool_use) and write a per-session `TurnEnded{kind: question|finished|error, excerpt}` fact to the Instance (via actor setter, snapshot field per `.claude/rules/instance-lock-free-reads.md`). Route the installed Stop hook to the HTTP endpoint (or have `ssq-hook-handler stop` also POST to it) so one hook produces the fact. Works only for Claude; other programs fall back to screen heuristics (aider, agy, pi, gemini, opencode patterns in `session/detection/binaries/`), measured per program with a fixture corpus.
2. **Two new reasons** in `AttentionReason` (+ proto wiring per project memory: domain enum is not enough): `ReasonProseQuestion` (Medium, needs-human class) and `ReasonAwaitingDirection` (finished, Low-Medium). Determine consumes the `TurnEnded` fact as a new stage before the idle-time fallback. Flag `attention_prose_question_detection`, live-settable, default off.
3. **Clock fix.** Idle/WaitingForAgent grace use `LastMeaningfulOutput` (already atomic-shadowed), not `UpdatedAt` (D8, D9).
4. **Ack semantics.** Bypass ack for needs-human reasons (fix D10 mismatch: `priority <= PriorityMedium` or reason-based); re-surface when `TurnEnded` is newer than the ack.
5. **Make skips visible:** one Debug/counter line per `shouldSkipSession` branch (observability requirement).
6. Delivery: add `ClassNeedsHuman` mapping for the new types in `deliverygate/policy.go` (new NotificationType or `delivery_class` metadata hint; the latter needs no proto enum change, honors ADR-002 type-keyed purity only for hidden sessions). Hidden stays suppressed unless needs-human.

### (b) Durable per-session event log
- **Store: new ent table `SessionEvent`** (sibling-table precedent: `BacklogActivityNote` ADR-001, `backlog_status_event.go`, `stage_transition.go`; no edge to Session so rows survive deletion like `SessionSummary`). Not the notification JSON store: it collapses duplicates (D18), caps at 7 days, and is keyed to delivery rather than state. Schema commit only; never commit generated ent (CLAUDE.md).
- Fields: `id`, `session_id` (uuid string, indexed with `occurred_at`), `session_title`, `kind` (enum string), `from_state`, `to_state`, `reason`, `excerpt` (<=500 chars, secret-scrubbed), `source` (poller|hook|lifecycle|user), `occurred_at`, `payload` JSON (small).
- Events: StatusChanged (Running/Idle/Executing transitions from `transitionTo`), TurnEnded(kind), AttentionRaised(reason)/AttentionCleared(reason, by=user|state-change), Acknowledged, NotificationEmitted/Suppressed(gate reason), LifecycleStarted/Exited/Stopped (reuse `LifecycleEvent` hook, `session/instance.go:147-165`), ApprovalResolved. Write only on change (dedup by last event kind+reason per session) to bound volume.
- Retention: per-session cap 200 rows and 30 days, global cap ~50k, pruned by the existing orphan-prune/retention sweeper pattern (`store.go enforceRetention`, `orphanPruneInterval`). Count prunes in a metric.
- Timer fix rides on this: `last_meaningful_change_at` = max(occurred_at of non-touch events), exposed on Session proto; SessionCard shows it instead of `UpdatedAt` (`SessionCard.tsx:1272`). `UpdatedAt` keeps its upsertSession-dedup job (`instance_state.go:32-38`). Persistence of UpdatedAt across restart/hydration is GAP; the reload reset in requirement 5 needs a repro (INFERRED cause: `instance.go:2700` restart and hydration bump it, not the browser).
- RPC: `ListSessionEvents(session_id | all, since, kinds, limit)`; flag `session_event_log`. New RPC => `make proto-gen`, `make registry-generate`, route tables, sinkguard classification (read).

### (c) Single attention surface
- Backend: one read model `AttentionItem` = current open ReviewItem + last N SessionEvents + notification link, produced from the queue (state) and event log (history). `WatchReviewQueue` stays as the live stream; add `ListAttention(include_recent_finished, include_quiet_since)` for "recently finished" and "unchanged for a while" views, both computed from `SessionEvent`.
- Notifications become derived views of the same events, not a parallel truth: emit tray/push from `AttentionRaised`; tray row links to `/attention?session=`.
- Durability: item persists until Cleared event; a dismissed toast no longer loses it.

## 4. Page-consolidation options (architecture only; ADR is separate)

Facts: `/review-queue` is referenced in 69 files under web-app/src, tests/e2e, docs/registry (grep count), including `routes.tsx`, `route-meta.ts:46`, `nav-pages.ts:58` (bottomNavPrimary), `CockpitShell.tsx`, `ReviewQueueNavBadge`, `lib/nav/paths.ts`, `features/review-queue.ts` catalog, `CommandDetector` omnibar, Slack/callback settings deep links, e2e `review-queue.spec.ts`, `nav-navigation.spec.ts`, `route-titles.spec.ts`. `/unfinished` ("Up Next", `route-meta.ts:70`, `nav-pages.ts:57`, also bottomNavPrimary) hosts PR + backlog tabs via `UpNextTabs`/`useUpNextTab`. `/notifications` is custom-rendered in BottomNav.

| Option | Routes / nav | RPC | Deep links | e2e / registry | Verdict |
|---|---|---|---|---|---|
| A keep `/review-queue`, fix pipeline, add log + timer | none | add `ListSessionEvents` | none break | add specs only | Lowest risk; leaves three overlapping surfaces; bottom nav already at 2 primary slots for work |
| B tray-only | delete route, nav item, badge | tray needs persistent grouped rows from `AttentionItem` | all `/review-queue` links (Slack, callbacks, omnibar, shortcuts, onboarding modal) need redirect | rewrite 3+ specs, registry feature removal | Tray is ephemeral by ADR-006 (non-modal overlay, no layout participation); using it as the durable surface contradicts that. Highest regression risk |
| C merge into Up Next | add "Sessions" tab (default when count>0) to `UpNextTabs`; `/review-queue` => redirect to `/unfinished?tab=sessions` | same `ListAttention` | keep `/review-queue` as a redirect permanently (cheap) | specs move to unfinished; registry `review-queue` feature re-pointed | Good fit: one "what next" home, frees a bottom-nav slot. Cost: Up Next is PR/backlog-centric and its tab model must handle live-stream state and mobile width (open question in requirements) |
| D hybrid: Up Next home + tray toasts + event drawer | as C, plus `SessionEventDrawer` mounted in SessionCard and attention rows | same | same as C | same as C plus drawer spec | Recommended architecture: matches the pipeline split (state => surface, transient => tray, history => drawer), and the redirect can be flag-gated (`attention_inbox_consolidation`) with `/review-queue` kept until soak ends |

Implementation notes for C/D: new rows in `route-meta.ts` only if a new `page.tsx` is added (a tab does not need one); keep `ReviewQueuePanel` as the tab body to avoid a rewrite; navigation via `@/lib/nav` only; `FeatureGuard` for the redirect flag; mobile: tab strip scroll + sticky filter chips (`needs me`, `finished`, `quiet`).

## 5. Tech debt disposition: determiner / poller

Measured (VERIFIED, `gocyclo`/`gocognit` run on both files): `checkSession` cyclomatic 49, cognitive 100; `Determine` 48 / 57; `reconcileSessions` 31 / 110; `pollLoop` cognitive 39; `GetContent` 21 / 29. Gate in `.golangci.yml` is gocyclo/gocognit min-complexity 25/40 plus funlen 150 lines, applied new-from-rev, so any edit to `Determine` or `checkSession` as-is fails `make ready-complexity-gate`. 20 commits touched these two files in 90 days (`git log --since=90.days`): high churn x high complexity = hotspot.

**Disposition: Refactor-first for `Determine` (small, mechanical), Isolate via seam for `checkSession`/poller.**
1. Refactor-first, `Determine`: the controller and no-controller branches duplicate the same status-to-(reason, priority, ctx) switch (`:144-181` vs `:228-273`). Extract `reasonForStatus(DetectedStatus, ctx) (reason, priority, ok)` as a table; behavior-preserving, covered by existing determiner tests. Then insert the new `TurnEnded` stage as one function call.
2. Seam, poller: do not add logic to `checkSession`. Extract the post-Determine gate chain (ack snooze, grace, re-add limiter, `:895-942`) into a `suppressionPolicy` function returning a typed decision + reason (also gives the "log why it did not surface" requirement), and put event-log writes behind a `SessionEventRecorder` interface injected into the poller. New code lives in new files with tests; the old body only shrinks.
3. Extend as-is is rejected: it would add branches to a function already 2x over the gate.

Characterization first: golden tests over a fixture corpus of real terminal tails per program (prose question, finished, approval, executing) before touching either function.

## 6. Event-Command-Policy table

| Domain Event | Policy trigger | Command | Actor/System |
|---|---|---|---|
| ClaudeStopHookReceived | Stop hook arrives with transcript | ClassifyTurnEnd, RecordTurnEnded(kind, excerpt) | System (HookReceiver) |
| TurnEnded(question) | `attention_prose_question_detection` on, session visible or needs-human | RaiseAttention(ProseQuestion, Medium) | System (Determine) |
| TurnEnded(finished) | flag on, no open higher item | RaiseAttention(AwaitingDirection) | System |
| ScreenDetectedInputRequired | numbered selector pattern | RaiseAttention(InputRequired) | System (poller) |
| AttentionRaised | reason in needs-human class | EmitNotification(type INPUT_REQUIRED, class needs_human) | System (ReactiveQueueManager) |
| AttentionRaised | reason routine and session hidden | SuppressNotification (log gate reason) | System (delivery gate) |
| AttentionRaised | any | AppendSessionEvent(AttentionRaised) | System (recorder) |
| NewOutputObserved | LastMeaningfulOutput > LastAcknowledged | ReopenAttention | System |
| OperatorAcknowledged | skip/snooze click | RecordAck, SnoozeUntilNewOutput (not for needs-human unless TurnEnded older) | Operator |
| OperatorReplied / SessionExecuting | state returns to Executing | ClearAttention(by=state-change), AppendSessionEvent | System |
| SessionQuietFor(N min) | LastMeaningfulOutput age >= threshold, no open item | RaiseAttention(Stale) once per quiet period | System (poller) |
| SessionExited/Stopped | LifecycleEvent | AppendSessionEvent; no revival (`noarchivedrevival`) | System |
| SkipEvaluated | `shouldSkipSession` true for a running-looking session | IncrementSkipCounter, log reason | System |
| EventLogRetentionExceeded | per-session/global cap or age | PruneSessionEvents, count drops | System |

## 7. Risks and open items
- Prose classification false positives re-flood the queue; mitigate with hook-first (explicit turn end), `?`-ending requirement, per-session rate limit, and soak on an isolated instance (private tmux socket).
- Non-Claude programs have no Stop hook; coverage there is screen-only and must be measured per program.
- GAP: web push path for TASK_COMPLETE; debug-level queue decisions; exact Claude Stop/Notification payload fields; reload-reset repro for the timer.
- Changing the D10 bypass affects ack UX for Medium items; needs a test and a flag.
