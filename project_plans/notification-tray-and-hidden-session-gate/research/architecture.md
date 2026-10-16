# Architecture Research: notification-tray-and-hidden-session-gate

Labels: **VERIFIED** = file opened / command run this session; **INFERRED** = reasoned from verified code, not executed.
All paths are repo-relative at origin/main `013856269`.

Builds on (not re-derived):
- `project_plans/notification-revamp/research/architecture.md` — dedup/collapse in `server/notifications/store.go` (§1, L15-101), `AppendAutoApproved` pre-read contract (§1 point 2, L76-87), Auto-handled section keyed on `notificationType === "auto_approved"` (§4, L379-393). ADR-001 (collapse semantics), ADR-003 (audit trail = metadata stamp, not new record type).
- `project_plans/review-session-notification-cleanup/decisions/ADR-001-notification-record-session-scoped-field.md` — `NotificationRecord.SessionID` is overloaded (real session UUID *or* backlog item ID); an explicit `SessionScoped` field (`pkg/events/notification_metadata.go:10`, `SessionScopedMetadata` L21) distinguishes them. **Consequence for the gate:** a resolver that looks up `event.SessionID` must treat "not a session" (item IDs) as pass-through, keyed on `metadata["item_id"]`/`session_scoped`, never on "lookup failed".

---

## 1. Channel inventory (OQ1)

### 1.1 Delivery channels (consumers)

| # | Channel | Entry point | Fed by | Hidden check? |
|---|---|---|---|---|
| C1 | Web push (+ OS notif on device) | `server/push/subscriber.go:21` `StartDeliverySubscriber` -> `deliverEvent` (L60) -> `WebPushNotifier` (`server/push/notifier.go:52`) -> `PushService.SendNotification` (`server/services/push_service.go:197`). Wired `server/server.go:331` | `bus.Subscribe` (L32): `EventSessionUpdated` via `buildStatusChangeNotification` (L148), `EventNotification` via `buildInlineNotification` (L150) | **None** (`rg -i hidden server/push` = no match, VERIFIED) |
| C2 | Notification history store (JSON) -> Notifications page, panel, `get_notification_history` MCP | `server/notifications/subscriber.go:31` `StartSubscriber` (wired `server/server.go:300`) -> `store.Append` (L59). Second writer: `AppendAutoApproved` (`store.go:267`), called directly from `server/services/approval_handler.go:477,488`, bypassing the bus | `bus.Subscribe` (L43), `EventNotification` only | **None** in subscriber/store. `AppendAutoApproved` takes `sessionID` but has no Hidden input |
| C3 | Live toast + history-only items + native OS notification in browser | `WatchSessions` stream (`server/services/session_service_watch.go:21`) -> `convertEventToProto` `case events.EventNotification` (`event_converter.go:70`) -> web `useSessionService.ts:1100` -> `useSessionNotifications.ts:83-228` -> `NotificationContext.addNotification/addToHistoryOnly` | `s.eventBus.Subscribe` (watch.go:31) | Hidden filter at watch.go:38,79,127 is `event.Session != nil && event.Session.Hidden`. **`EventNotification` events carry `Session == nil`** (`NewNotificationEvent`, `pkg/events/types.go:344-367` sets SessionID/Context only) so they are never filtered. The filter only protects `session.updated/created` snapshots. |
| C4 | Slack | `services.SlackNotifier` (`slack_notifier.go:253 NotifyReviewQueueItem`, `:297 NotifyApprovalPending`, `:369 MaybeNotifyQueueDepthThreshold`). Called directly (not via bus) from `review_queue_manager.go:476-486` and `approval_handler.go:748` | direct call | RQM path: inside the `!suppressForHidden` block (L449). Approval path: none (not needed: approval = needs-human) |
| C5 | Review-queue stream `WatchReviewQueue` (`ReviewQueueEvent`) | `review_queue_manager.go:397,618` `publishToClients` | queue observer | Upstream: `session/review_queue_poller.go:805-822` excludes `snap.Hidden` sessions from the queue; `review_queue_determiner.go:336-340` lets ErrorState/TestsFailing through for Hidden. (Poller exclusion makes RQM's `suppressForHidden` mostly redundant — INFERRED, needs a test to confirm which wins.) |
| C6 | Other bus subscribers (not user-facing): analytics `server/analytics/subscriber.go:44`, workflow, backlog events, unfinished-work, `waitForEvent`, MCP `wait_for_backlog_event`, backlog forward-sync | `bus.Subscribe` | — | N/A (no user delivery) — but **they see every event**, so a Publish-time drop must not break them (see §2) |
| C7 | PWA push click -> deep link | `web-app/public/push-sw.js:51` `notificationclick` opens `data.url` = `/?session=<id>&tab=terminal` (`push/subscriber.go:buildSessionURL`) | — | N/A; resolution is client-side (§3) |

There is **no** `WatchNotifications` RPC (`rg "rpc .*Notif" proto/session/v1/session.proto` -> only Send/GetHistory/MarkRead/Clear). Toasts ride `WatchSessions`. VERIFIED.

### 1.2 Producers -> channels -> Hidden check

All `EventNotification` producers call `events.NewNotificationEvent` then `Publish` on the one `*events.EventBus` (`pkg/events/bus.go:72`), so they reach **C1+C2+C3** together (C1 only if URGENT or APPROVAL type, `push/subscriber.go:126-136`). `derivePriority(urgent, important)` (`server/services/notification_priority.go:11`): urgent+important=URGENT(4), important=HIGH(3), urgent=MEDIUM(2), neither=LOW(1).

| Producer (file:line) | Event / type / priority | Channels | Hidden check? |
|---|---|---|---|
| `NotificationService.SendNotification` `server/services/notification_service.go:76`, publish L158. Called by `ssq-hook-handler`/`ssq-notify` RPC (`scripts/ssq-hook-handler:432-528`) | per hook: Notification hook = INFO, `medium` (L432-445, title default `"Claude Notification"`); Stop = `task_complete` `low` (L461) / `task_failed` `high` (L455-458); SubagentStop low/medium; PostToolUseFailure `high` `error` (L528) | C1(only if urgent) C2 C3 | **Partial, priority-based**: L142 suppresses only `priority == LOW`. Hidden resolved at L101-127 (poller `inst.Snapshot().Hidden`, storage fallback `d.Hidden`) — good pattern. **Gap:** the `Notification` hook (medium, INFO) and `Subagent: Context Limit` (medium) from a hidden session pass. |
| `buildStatusChangeNotification` `push/subscriber.go:148,L168-195` | `EventSessionUpdated` with `status` in UpdatedFields and `Status==Stopped` -> "Session Completed" push (tag `session-completed-<id>`) | **C1 only** (does not touch C2/C3) | **None. This is the reported screenshot bug.** Event has `Session *Instance`, so `sess.Snapshot().Hidden` is available. Fed by `session_service_events.go:115` (exit publisher), `session_service_lifecycle.go:66,109,153,652,707`, `dependencies.go:949,980` |
| `ReactiveQueueManager.OnItemAdded` `review_queue_manager.go:460` | review reasons, mapped by `mapReviewItemToNotification` | C1 C2 C3 C4 | **Yes, reason-based** L442: `hidden && reason in {TaskComplete, Idle, Stale}` suppressed; ErrorState/TestsFailing published. Uses raw `inst.Hidden` (L413) — violates `.claude/rules/instance-lock-free-reads.md`. APPROVAL_PENDING skipped on purpose (dup of approval handler) |
| `ApprovalHandler.broadcastApprovalNotification` `approval_handler.go:726` (+Slack L748) | APPROVAL_NEEDED, URGENT | C1 C2 C3 C4 | None — correct under policy (needs-human) but must stay *allowed* by any new gate |
| `broadcastQuestionNotification` `approval_handler.go:765` | INPUT_REQUIRED, URGENT | C1 C2 C3 | None — needs-human, allowed |
| `approval_handler.go:477,488` `AppendAutoApproved` | AUTO_APPROVED, pre-read, **bypasses bus** | C2 only (appears in "Auto-handled" section) | None. Routine by definition; for hidden sessions it is pure noise in history |
| `StaleSessionNotifier.notify` `stale_session_notifier.go:147`, loop L101 over `poller.GetInstances()` | WARNING, MEDIUM ("Session went stale") | C2 C3 | **None — ungated.** Iterates every instance incl. Hidden (`rg -i hidden` on the file = no match) |
| `SessionService.notifySteerSent` `session_service_update.go:420` | INFO, LOW "Steering input sent" | C2 C3 | None; user-initiated, but fires for steers of hidden sessions too |
| `onColdRestoreLostHistory` `session_service_events.go:81` | WARNING, HIGH | C2 C3 | Yes (`inst.Hidden` L76) — **suppresses unconditionally**, raw field |
| `onRateLimitDetected` `session_service_events.go:202` | WARNING, URGENT | C1 C2 C3 | Yes (L193) unconditional; note state sync L213 and quota gate L219 intentionally ungated |
| `onRateLimitRecoverySucceeded` L236 | INFO, LOW | C2 C3 | Yes (L234) — correct (routine) |
| `onRateLimitRecoveryFailed` L254 | FAILURE, URGENT | C1 C2 C3 | Yes (L252) — **contradicts the decided policy** (failures must notify); needs to move to the gate |
| `AutonomousOrchestrationService` generic completion `autonomous_orchestration_service.go:620` | task complete/failed | C1 C2 C3 | Yes (`!inst.Hidden` L619) — suppresses *including* the L610 "status update failed" FAILURE override. Other publishes in this file (L184 turn callback, L344, L420, L649, L674) are item-scoped; L184 `buildTurnCallback(inst)` not checked — INFERRED ungated |
| `CapacityMonitor` `capacity_monitor.go:385,416,488,697,721` | WARNING, HIGH (guardrail stop, token ceiling, compaction/escalation) | C2 C3 (HIGH is not push-eligible) | None. Uses `snap.UUID` — `snap.Hidden` available on the same snapshot |
| `session.Notifier` -> `EventBusNotifier` `backlog_notifier.go:30,47`, from `session/session_driver.go:1004` (ERROR, URGENT "gave up after repeated failures"), `session/review_gate.go:338,375,602,660`, `session/backlog_lifecycle.go:626`, `session/worktree_consistency_sweep.go:490-493`, `guidance_request_service.go:75,79` | mixed; `Notify` threads **backlog item ID** as SessionID (ADR-001), `NotifySession` threads a session UUID | C1 C2 C3 | None. `session_driver.go:1004` is a hidden-session *failure* (should pass); review_gate notifications are item-scoped |
| Backlog service notifications `backlog_service_triage.go:190,225,334,364,386,1529,1644,2865,2960`, `backlog_service_pr_fix_steer.go:378,407`, `backlog_service_verdict_steer.go:172`, `backlog_service_trigger_triage.go:49`, `backlog_service_stuck.go:315`, `backlog_service.go:1301`, `autonomous...:649,674`, `mcp/tools_backlog.go:851,2746` | item-scoped (SessionID = item ID, `item_id` metadata) | C2 C3 (C1 if URGENT) | N/A — no session; gate must pass them through |
| Process-level health: `server/server.go:1874` fork-pressure, `:1892` tmux recovered; `memory_pressure_notifier.go:139,158`; `quota_gate.go:386,405`; `session_service.go:601` claude-settings | system alerts, empty/no session | C2 C3 (+C1 if URGENT) | N/A — pass-through |

`tmux` "via tmux" toast (Success-metric baseline, OQ2): **VERIFIED origin.** `scripts/ssq-hook-handler:231-235` sets `APP_NAME="tmux"` when `$TMUX` is set, passed as `source_app` (L367-368); title falls back to `"Claude Notification"` (L432) for Claude's own `Notification` hook, priority `medium`, type `info` (L434, L445). `NotificationItem.tsx:73,149-153` treats that title as generic and shows the session name. So it is the **hook path** of `SendNotification`. **Is `…-r2` hidden? INFERRED not hidden**: `search_sessions` returned it (tags `backlog:work, backlog:revision, autonomous`), it is a work session (`-r2` revision), and the MCP search path shows both `…model-pinning` (triage) and `-r2`; I could not read the `Hidden` flag from that response, so confirm with `GetSession`. If it is visible, the toast is legitimate (Claude asked for input / idle prompt) and the real defect is only that the *hidden* variant of the same hook is not suppressed because the existing check is priority-based (L142) and this hook is `medium`.

### 1.3 Ungated paths (summary)

1. **U1 (reported bug)**: `buildStatusChangeNotification` push (C1) — no check.
2. **U2**: `SendNotification` hook path for non-LOW priority (Notification hook = medium) — C1/C2/C3.
3. **U3**: `StaleSessionNotifier` — C2/C3.
4. **U4**: `CapacityMonitor` (5 sites), `notifySteerSent`, `AppendAutoApproved`, `EventBusNotifier` session-scoped (`NotifySession`) producers — C2/C3.
5. **U5**: the C3 stream filter (`session_service_watch.go:38,79,127`) is structurally unable to filter `EventNotification` (Session==nil).
6. **Over-gated (policy violation, not leak)**: `onRateLimitRecoveryFailed`, `onRateLimitDetected`, `onColdRestoreLostHistory`, autonomous completion-failure override — hidden *failures* are swallowed today. Per-site checks have drifted in **three different shapes** (priority==LOW, reason-set, boolean). That is the bug class the requirements want to retire.

---

## 2. Recommended choke point (one gate)

### Options

| Option | Covers | Misses / cost |
|---|---|---|
| A. Producer-side (at `NewNotificationEvent`) | any producer that remembers | status-change push (C1 derives from `EventSessionUpdated`, no notification event exists); needs session lookup inside a pure constructor; this is the status quo that regressed |
| B. **EventBus `Publish` filter for `EventNotification`** (hook set at wiring) | every `NewNotificationEvent` producer (47 call sites, recounted by `grep` at `013856269`; the earlier "~60" was an estimate; VERIFIED list §1.2) with **zero producer edits**; all of C1(inline) C2 C3 at once; dropped before `Seq` assignment so `EventsSince` replay can't resurrect it | does not see `AppendAutoApproved` (direct store write), Slack direct calls, nor `EventSessionUpdated`-derived push. Needs a session-visibility resolver on the bus |
| C. Each subscriber | covers anything | N copies of the check; exactly the per-channel regrowth the requirements reject |
| D. `store.Append` only | C2 | C1/C3 untouched |

### Recommendation: B + one shared pure policy function, called from the 3 non-bus points

- New pure function (e.g. `server/notifications` or `pkg/events`): `ShouldDeliver(hidden bool, notifType, priority int32, kind) Decision{Allow|Suppress, Reason}`. Policy (decided in requirements): hidden -> allow only **needs-human** = `APPROVAL_NEEDED(1)`, `INPUT_REQUIRED(2)`, `CONFIRMATION_NEEDED(3)`; **failure** = `ERROR(7)`, `FAILURE(9)`; everything else suppressed. This answers OQ4: key on **type**, not priority — the current priority==LOW test (`notification_service.go:142`) is why `medium` INFO hooks leak. `WARNING(8)` is the judgment call (rate limit, stale, capacity guardrails are WARNING): recommend allow WARNING only when `priority >= HIGH` (guardrail stops, rate limit) and suppress MEDIUM warnings (stale). Flag for plan.
- `EventBus.SetPublishFilter(func(*Event) bool)` (or a thin `GatedPublisher`) installed in `server/dependencies.go` where the bus is built. Filter body: type != `EventNotification` -> pass; resolve visibility from `event.SessionID` via a `SessionVisibilityResolver` (poller `FindInstance(...).Snapshot().Hidden`, then `InstanceStore.ListInstanceData` fallback — copy of `notification_service.go:101-129`, which should be *moved* into the resolver and deleted from `SendNotification`); not-a-session (`item_id` metadata / `session_scoped != "true"` per ADR-001 / empty ID) or unresolved -> **pass** (fail-open; log at WARN so a deleted-hidden-session leak is observable); else apply `ShouldDeliver`. On suppress: `slog` + counter (`channel=bus`, session id, type, reason) per Observability Requirements; on a hidden failure that passes: log-only.
- The three non-bus sites call the same function: (1) `buildStatusChangeNotification` — add `ShouldDeliver(sess.Snapshot().Hidden, STATUS_CHANGE/TASK_COMPLETE, …)` (a "Session Completed" is routine, so hidden => always suppress; hidden *failed* status could map to FAILURE and pass); (2) `AppendAutoApproved` caller — resolve visibility, suppress AUTO_APPROVED for hidden; (3) Slack — RQM already inside the shared block; keep but route through the same function and delete `suppressForHidden`.
- Trade-offs: B couples the bus to a lookup (hot-path cost only for notification events, a map/RW lookup; the poller already holds instances — INFERRED cheap). Filtering inside `Publish` also hides the event from analytics/MCP waiters (C6); acceptable because only *notification* events are filtered, but `wait_for_backlog_event`-style consumers of `EventNotification` for hidden sessions should be checked in plan (grep `EventNotification` consumers: only `notifications/subscriber.go:80`, `push/subscriber.go`, `event_converter.go`, VERIFIED). Live-settable flag: filter reads the feature-flag per call (no env var).
- Alternative if bus-level coupling is rejected: keep Publish unfiltered but make **every** subscriber go through `ShouldDeliver` via a shared `deliverable(event)` helper; strictly weaker (adding a 4th subscriber silently bypasses).

### How to prove no channel bypasses it

1. **Matrix test** (table-driven, deterministic, no sleeps): fake resolver marks session `S` hidden; publish one event per `(NotificationType x Priority)` plus a `session.updated{status=Stopped}`; attach *real* `notifications.StartSubscriberWithInterval` (store fake `Appender`), a recording `push.Notifier` via `StartDeliverySubscriber`, and a `WatchSessions`-style `bus.Subscribe`. Assert: routine -> zero records in all three; failure/needs-human -> exactly one in each. The recording notifiers and the store are the only terminal sinks, so passing means no channel leaks.
2. **Sink-enumeration guard test**: a Go test (AST/`rg`-style over non-test files, pattern: the `go vet`-ish allowlist test used elsewhere, e.g. `terminal_service_test.go:19`) that fails if anything outside an allowlist calls `PushService.SendNotification`, `store.Append*`, `SlackNotifier.Notify*`, or `webpush.SendNotification`, or if a new `bus.Subscribe(` appears outside a reviewed list. New channel => test fails until its author routes it through the gate.
3. **Producer lint**: forbid direct `inst.Hidden` reads inside `server/` notification code (project rule already requires `Snapshot()`).
4. e2e (existing pattern, `tests/e2e`): spawn a hidden session via test-mode, assert Notifications page/panel/toast stay empty after completion.

---

## 3. Read-only hidden-session view (OQ3)

**What already exists (VERIFIED):**
- `GetSession` has **no** Hidden filter (`session_service_crud.go:125-151`); `ListSessions` filters `inst.Hidden && !IncludeHidden` (L61, external L101). `WatchSessions` always drops hidden (watch.go:38,79,127).
- The web app already resolves hidden sessions for deep links: `web-app/src/app/page.tsx:222-250` `fetchHiddenSessionFallback` calls `getSession` when `?session=<id>` isn't in the list (added for diagnose sessions, commit `67fb03d4c`). The push URL `/?session=<id>&tab=terminal` therefore **may already open** the hidden session via a push click once `sessions.length > 0` (L241 early-returns on an empty list — a race to test). Requirement text "View Session fails" is thus at least partly the toast/panel `onView` path, not the deep link — INFERRED; needs an in-browser repro before building anything. Reuse this; do not add `includeHidden: true` to the list call.
- No read-only terminal mode exists: `rg -i "readonly|view_only"` over `connectrpc_websocket.go`, `terminal_service.go`, `*.proto` = no match.

**Write paths reachable once a hidden session is opened in the normal terminal pane (VERIFIED sites):**
- `TerminalData.Input` handlers: `session_service_stream_terminal.go:405`, `connectrpc_websocket.go:2701` (control-mode read loop, `dispatchInputReadLoopFrame`), `:3337` (capture-pane), `connectrpc_websocket_shell.go:257`; `Resize` at the same sites (resize votes affect the tmux pane, so read-only should also not vote — #728/#731).
- Unary: `TerminalService.WriteToSession` (`terminal_service.go:102`, proto L459), `UpdateSession` steer branch (`session_service_update.go:313,406`), pause/resume/delete lifecycle RPCs.
- MCP: `write_to_session`, `send_control`, `run_command`, `steer_session`, `resume_session` registered through `withDiagnoseGate` (`server/mcp/diagnose_role_gate.go:62`, `tools_terminal.go:106,122,158`) — an existing caller-identity gate precedent.

**Recommended guard (server-side, mirrors `instinct_mcp_dispatched_agent_tool_surface`):**
1. Mode is a property of the **stream**, not trusted from the client: on `StreamTerminal`/WebSocket attach, the server decides `readOnly := inst.Snapshot().Hidden` (plus optional explicit `read_only` field in the handshake that can only *tighten*). Store on the stream params struct (`inputReadLoopParams`, `connectrpc_websocket.go:2695`) and drop `GetInput()`/`GetResize()` frames in `dispatchInputReadLoopFrame` and the three sibling handlers with a debug log. One struct field + 4 early returns; the dupl gate suggests a shared `p.acceptsInput()` helper.
2. Unary writes: add a single `rejectIfHidden(inst)` in `findInstance` callers that mutate (`WriteToSession`, steer branch, lifecycle RPCs) returning `FailedPrecondition`; keep read RPCs (`GetSession`, `GetTerminalSnapshot` `terminal_service.go:61`, `GetSessionDiff`) open.
3. Escape hatch: operators sometimes *do* need to nudge a stuck hidden session; that already exists via `diagnose_nudge_session`/MCP and is out of scope. Do not add a UI "unlock".
4. Frontend: pass `readOnly` to the terminal component (xterm `disableStdin`, hide the input bar/soft-keyboard trigger) and show a "Background session — read-only" banner. Request flow stays `GetSession` -> `StreamTerminal`.
5. "Background activity" view: needs a **list** of hidden sessions -> either `ListSessions{IncludeHidden:true}` filtered client-side to `hidden`, or (preferred, cheaper payload) a dedicated field/RPC; do not merge them into the main store `sessions` slice (keeps the main list clean, the stated rabbit-hole cap).

---

## 4. Frontend state architecture

**Provider tree (VERIFIED, `web-app/src/app/layout.tsx:61-77`):** `ViewportProvider > ErrorBoundary > AuthProvider > Providers (…NotificationProvider, GlobalSessionServiceProvider, TerminalPoolProvider, … `app/Providers.tsx`) > CockpitShell > { skip-link, <main>{children + banners}</main>, <NotificationPanel/> }`.
- `NotificationPanel` is a **sibling of `<main>`**, not inside page content, and is `position: fixed; right:0; transform: translateX(100%)` -> `translateX(0)` with a full-viewport `overlay` (`NotificationPanel.css.ts:9-46`; render `NotificationPanel.tsx:144-153`, `role="dialog" aria-modal="true"`). So opening it **already** does not navigate or remount terminals (INFERRED from structure; the e2e in Success Metrics must still assert terminal DOM identity). The tray work is therefore evolve-in-place, not a new surface.
- Toasts: rendered by `NotificationProvider` itself in a `position: fixed; bottom:0; right:0; zIndex: zIndex.toast` div, unbounded `notifications.map` (`NotificationContext.tsx:484-501`); `clearAll()` exists (L263) but is unused by UI.
- Gaps vs requirements: no toast cap/grouping; the panel is a modal (backdrop dims and `aria-modal` traps focus -> conflicts with "tray without stealing terminal focus"); width `100%` below `breakpoints.md` (full-screen on mobile) — a persistent edge handle + non-modal overlay needs a different shape on mobile; `zIndex` 9998/9999 literals are hard-coded instead of the `zIndex` tokens.
- State: `NotificationContext` holds `notifications` (toasts, local state), `notificationHistory` (hydrated from server `useNotificationHistory`, server wins on isRead, L141-175), `isPanelOpen` (L144). Cross-tab: `createNotificationSyncChannel()` BroadcastChannel handles only `NOTIFICATION_DISMISSED` (L346-361); acknowledgement deliberately goes through the server stream. Stale toasts auto-expire at 5 / 6 min (L328-343); `HISTORY_ONLY_TYPES` (`useSessionNotifications.ts:19-26`) already demote routine types (TASK_COMPLETE, INFO, STATUS_CHANGE…) straight to history, so "routine toasts demote to tray" is partly done; it keys on **type** only, so a hidden-session event the server now lets through (ERROR/FAILURE/approval) correctly toasts.
- Single source of truth recommendation: keep server history as truth for read/dismissed (already so); add toast *presentation* state (collapsed, cap overflow count) as derived/local only; extend the BroadcastChannel message union with `NOTIFICATIONS_CLEARED{scope}` so bulk "Dismiss all"/"mark all read" mirrors across tabs (per-id messages would flood). Bulk clear must filter on `isActionable(type)` (exists, used L328-343) so approvals/INPUT_REQUIRED are never cleared (#738 requirement).
- Terminals mount in page content under `<main>` with a shared `TerminalPoolProvider` in `Providers` (above everything), so remounts only occur on route/`key` change; a fixed overlay sibling cannot trigger them. Risks to test, not assume: focus return to xterm on tray close, `--viewport-height` soft-keyboard handling for a bottom-anchored toast stack, and touch gestures — `lib/hooks/useTerminalGestures.ts` already owns touch handling over the terminal region (INFERRED conflict site for swipe-to-dismiss; restrict swipe handlers to toast/tray elements only, never ancestors of the terminal).

---

## 5. Hotspot dispositions (90-day commit counts via `git log --since=90.days --oneline -- <file> | wc -l`, VERIFIED)

| File | Commits/90d | Disposition | Why |
|---|---|---|---|
| `server/review_queue_manager.go` | 9 | **Isolate via seam** | Delete `suppressForHidden` (L436-442) in favour of the shared `ShouldDeliver`; do not otherwise refactor |
| `server/notifications/store.go` | 8 | **Extend as-is** | Gate sits upstream (bus); only `AppendAutoApproved` caller changes. Don't add Hidden to the store schema |
| `web-app/src/lib/contexts/NotificationContext.tsx` (532 lines) | 5 | **Refactor-first (small)** | Provider owns toast list, history merge, panel state and renders the toast DOM inline (L484-501). Extract `ToastStack` component + `useToastStack` selector first, then the cap/"+N more"/"Dismiss all" lands in the new unit |
| `server/services/notification_service.go` | 3 | **Isolate via seam** | Move the L101-142 hidden-resolution into the resolver; the RPC keeps validation + rate limit |
| `server/push/subscriber.go` | 3 | **Extend as-is** | One call to `ShouldDeliver` in `buildStatusChangeNotification` |
| `web-app/src/components/ui/NotificationPanel.tsx` (286 lines) / `.css.ts` | 3 | **Extend as-is** (mobile modal behaviour change is a style/aria edit) | Evolve into tray; add handle + sections |
| `server/services/session_service_events.go` | 1 | **Extend as-is** | Remove four `inst.Hidden` branches (use gate) |

(`rg` found no `gocognit`-exempt markers on these; run `make ready-complexity-gate` since `dispatchInputReadLoopFrame`/RQM edits touch modified existing functions.)

---

## 6. Event-Command-Policy table

| Domain Event | Policy trigger | Command | Actor / System |
|---|---|---|---|
| `HookNotificationReceived` (Claude Notification/Stop/StopFailure hook) | none (direct) | `SendNotification` RPC | Claude process -> `ssq-hook-handler` -> `NotificationService` |
| `SessionStopped` (`session.updated`, `status` field) | "a completion is routine unless it failed" | `BuildStatusChangeNotification` -> push | `PushSubscriber` |
| `NotificationRaised` (any producer, `EventNotification`) | **"whenever a notification targets a Hidden session, only failure/needs-human may pass"** | `GateNotification(sessionID, type, priority)` -> Allow/Suppress | `EventBus` publish filter + `SessionVisibilityResolver` (new) |
| `NotificationSuppressed` | "a suppression must be observable, never silent" | `LogAndCount(channel, session, type, reason)` | gate |
| `NotificationAllowed` (hidden failure/needs-human) | "log hidden deliveries with channel" | `LogDelivery` then fan-out | gate |
| `NotificationPublished` (allowed) | "persist every delivered notification" | `AppendToHistory` (collapse per ADR-001) | `notifications.Subscriber` -> `NotificationHistoryStore` |
| `NotificationPublished` | "urgent or approval pushes" | `SendWebPush` (dedup window) | `push.Subscriber` -> `PushService` |
| `NotificationPublished` | "stream to open UIs" | `StreamToClients` | `WatchSessions` |
| `NotificationReceivedByClient` | "routine types go history-only; approvals never dedup" | `AddToHistoryOnly` / `AddToast` | `useSessionNotifications` -> `NotificationContext` |
| `ToastCountExceededCap` (>3 visible) | "overflow collapses into tray" | `CollapseOverflow` -> "+N more" chip | `ToastStack` (new) |
| `UserTapsDismissAll` | **"bulk actions never clear items needing a decision"** | `ClearInformational` (filter `!isActionable`) + broadcast `NOTIFICATIONS_CLEARED` | user -> `NotificationContext` -> `BroadcastChannel` |
| `UserOpensNotificationForHiddenSession` ("View Session") | "hidden sessions open read-only" | `GetSession` (no hidden filter) -> `StreamTerminal(readOnly)` | user -> `page.tsx fetchHiddenSessionFallback` -> terminal service |
| `TerminalInputFrameReceived` on a hidden-session stream | **"hidden streams accept no input or resize"** | `DropInputFrame` | `dispatchInputReadLoopFrame` guard (`p.acceptsInput()`) |
| `ApprovalRaised` for hidden session | "needs-human always notifies" | gate returns Allow -> Slack + push | `ApprovalHandler` |
| `RuleAutoResolved` (`AppendAutoApproved`) | "auto-handled records are silent history" | gate via shared policy; suppress when hidden | `ApprovalHandler` -> store |

---

## 7. Open items handed to the plan

1. Confirm in a browser whether push-click / toast "View Session" already resolves hidden sessions through `fetchHiddenSessionFallback` (page.tsx:222), and the `sessions.length === 0` race at L241. This may shrink the reachability epic.
2. Confirm `…-r2` Hidden flag via `GetSession` (§1.2 INFERRED).
3. Decide WARNING handling for hidden sessions (§2).
4. Decide fail-open vs fail-closed for unresolvable session IDs (recommend fail-open + WARN log; ADR-001 `session_scoped` flag as the discriminator).
5. Verify RQM `suppressForHidden` is redundant with `review_queue_poller.go:822` before deleting.
6. Swipe-to-dismiss safe regions (OQ5) not researched here; `useTerminalGestures.ts` is the file to read. Tray overlay-vs-dock (OQ6): existing panel is already a fixed overlay; the only change needed is non-modal behaviour.
