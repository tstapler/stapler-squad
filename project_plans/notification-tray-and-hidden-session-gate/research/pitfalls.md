# Pitfalls: notification-tray-and-hidden-session-gate

Labels: VERIFIED = opened the source / ran the query this session (read-only). INFERRED = reasoning from code, not exercised.
Base: origin/main 013856269. Prior art: `project_plans/notification-revamp/research/pitfalls.md` (dedup/retention/rule-reconciliation; not repeated here except where it intersects).

## 0. OQ2 resolved: the "Claude Notification ... via tmux" toast

**Verdict (VERIFIED): not a hidden-session leak and not an ungated channel. It is a legitimate hook notification from a visible Backlog work session.** It is, however, a low-value "idle" toast that the new toast cap/demotion should handle.

Evidence chain:
- Title: `scripts/ssq-hook-handler:432` falls back to `"Claude Notification"` when the hook payload has no title. Body "Claude is waiting for your input" is Claude Code's own Notification-hook text (same string appears in `server/services/notification_service_test.go:146`). The hook is sent with `--type info`, default priority `medium` (`ssq-hook-handler:431-447`).
- Subtitle: `web-app/src/components/ui/NotificationItem.tsx:74-85` `getContextString` builds `"<sourceProject> via <sourceApp>"`. `source_app=tmux` comes from `ssq-hook-handler:231-235` (`[[ -n $TMUX ]]` -> `APP_NAME="tmux"`, project = `tmux display-message '#S'`). So "via tmux" is just "hook ran inside any tmux session", i.e. every ssq-managed session. `NotificationItem.tsx:73` `GENERIC_TITLES` demotes "Claude Notification" to subtitle-only.
- Record (read-only from `~/.stapler-squad/workspaces/d685c4b1a423cca3/notifications.json`): session_id `0db5dfef-...`, name `stapler-squad-background-llm-model-pinning-r2`, type 1, priority 2, source_project `staplersquad_stapler-squad-background-llm-model-pinning-r2`, occurrence_count 3.
- Hidden? `sessions.db` (opened `mode=ro`): `hidden=0`, `category=Backlog`, `session_type=existing_worktree`, status 2. The "background" in the name is the backlog item slug ("background-llm-model-pinning"), not the Hidden flag. Same for sibling `...-pinning` (hidden=0).
- Gate: `server/services/notification_service.go:132-146` would not suppress it even if hidden, because it only suppresses `PRIORITY_LOW`; this notification is priority 2 (medium). So that gate would have let a *hidden* session's idle "waiting for input" through too (INFERRED for the hypothetical; the code path is VERIFIED).

Design implication: the "waiting for your input" idle toast is the recurring-noise class for visible sessions (routine, demote to tray), distinct from the hidden-session problem.

## 1. Gate placement: quantified evidence the bug class is still live

**Production data (VERIFIED, read-only query of notifications.json joined to sessions.db.hidden):** 319 stored rows; 55 of 240 sessions are Hidden; 84 rows unresolvable to a session (deleted). Of resolvable rows, 10 rows / 283 occurrences are from Hidden sessions:

| Hidden session | type/priority | Title | occurrences |
|---|---|---|---|
| `review:ce71ad1a` | 8 / 1 (low) | Session Token Ceiling Exceeded | 166 |
| `review:ec5e3618` | 8 / 1 | Session Token Ceiling Exceeded | 48 |
| 7 other `review:*:<id>` | 8 / 3 | Session Token Ceiling Exceeded | 1-2 each |
| `review:c42de545` | 2 / 3 | Claude has a question | 61 |

Producer of the ceiling rows: `server/services/capacity_monitor.go:420` (publishes straight to the EventBus). So:
1. The history store ingests every `EventNotification` unconditionally (`server/notifications/subscriber.go:80-90`, `eventToRecord` -> buffer; no session lookup). The existing hook-path gate in `NotificationService.SendNotification` is irrelevant to it.
2. The `Hidden` check at `notification_service.go:132` is priority-LOW-only and lives inside one RPC. It is the *third* per-path gate (after #227, #854); `review_queue_manager.go:436-449` `suppressForHidden` is a fourth, differently-shaped one.
3. Ceiling events for hidden sessions are *failure-like* but published at priority 1 (low) on some rows and 3 on others; a gate keyed on priority would suppress or pass them inconsistently (VERIFIED: same title, both priorities in data). Key the policy on a typed **reason/class** (failure | needs-human | routine), not on priority.
4. 166 occurrences from one review session shows a spamming producer; the gate must be paired with the existing dedup, and "log, not suppress, hidden failures" (requirements) will itself flood logs/counters for this producer unless counters are aggregated.

**Producer inventory (VERIFIED by grep `NewNotificationEvent(`; 50+ call sites):** `notification_service.go:158`, `review_queue_manager.go:460`, `server.go:1874,1892`, `mcp/tools_backlog.go:851,2746`, `autonomous_orchestration_service.go` (x6), `approval_handler.go:726,765`, `backlog_notifier.go`, `backlog_service*.go` (triage x10, stuck, verdict_steer, pr_fix_steer x2), `memory_pressure_notifier.go`, `quota_gate.go`, `capacity_monitor.go` (x5), `session_service*.go` (x6), `stale_session_notifier.go`. Subscribers of EventNotification: `notifications/subscriber.go`, `push/subscriber.go:129,150`, `services/event_converter.go:70` (toast/WebSocket stream), plus analytics. Most producers carry a session ID but none besides the two above look it up for Hidden.

**Second push path (VERIFIED):** `push/subscriber.go:buildStatusChangeNotification` (~L152-185) pushes "Session Completed" on any Stopped status transition, uses `event.Session` (a `*Instance`), and has zero Hidden handling. Tag is `"session-completed-"+stableID` with `Renotify:false`, so repeat completions of the same session silently replace each other (fine), but a gate here must read `sess.Snapshot().Hidden`, not `sess.Hidden` (`.claude/rules/instance-lock-free-reads.md`). `buildInlineNotification` (~L186+) has only `shouldNotify` (urgent or approval) and also no Hidden check; hidden `needs-human` approvals (type approval) pass through it today, which is the desired behaviour and must be preserved.

**Must design against:**
- Choke point: a single `HiddenGate` evaluated in each *subscriber* (store, push, event_converter/toast, Slack) over one shared classifier `(hidden, class) -> deliver?`, or at publish time via a wrapper around `eventBus.Publish`. Producer-side is fragile (50+ sites). Subscriber-side is robust but four copies of the call; prefer one decorator on the bus that drops/annotates before fan-out. INFERRED recommendation; the 50+ producer count is VERIFIED.
- Lookup failures: event for a deleted/unresolved session (84 of 319 stored rows have no live session) must fail *open* for failure/needs-human and fail *closed*... decide explicitly; a closed default would reintroduce the "dead-end View Session" with no session to open. INFERRED.
- Race: `NotificationService.SendNotification` falls back poller -> storage (`notification_service.go:96-125`); a poller miss with empty storage leaves `hidden=false`. Gate must resolve Hidden by the same path for every channel or hidden sessions leak right after restart (VERIFIED code; leak scenario INFERRED).
- Regression test: table test that enumerates every `EventNotification` type x channel with a Hidden session and asserts zero delivery for routine and delivery for failure/needs-human. Without an enumerating test the bug class will regress a fourth time. Consider a lint/ast-grep check that `eventBus.Publish(NewNotificationEvent` is only called via the gated wrapper.
- Feature flag must be consulted *inside* the gate so flipping it restores prior behaviour for all channels at once.

## 2. Hidden-session reachability

- `ListSessions` drops hidden unless `IncludeHidden` (`session_service_crud.go:61,101`); the web UI never sets it. A client-side fix that sets `IncludeHidden=true` globally floods the list/board/search (Rabbit Hole). Add a dedicated `GetSession`-by-id path or a "background" list RPC instead (INFERRED).
- Read-only view: `WriteToSession` exists on both `SessionService` (`session_service_delegates.go:931`) and `TerminalService` (`terminal_service.go:102`), plus websocket terminal input (`connectrpc_websocket.go`). A UI-only "read-only" flag is insufficient; guard server-side by session `Hidden` + a caller-identity/ view-mode check at each write entry (same lesson as `instinct_mcp_dispatched_agent_tool_surface`). Note that MCP dispatchers legitimately write to hidden review sessions, so the guard must distinguish operator-UI callers from dispatchers. INFERRED; entry points VERIFIED by name.
- Push-click deep link: `buildSessionURL(event.SessionID)` (`push/subscriber.go`) must route hidden sessions to the read-only route, else the click lands on "session not found".

## 3. Swipe gestures vs terminal/pane touch handling

VERIFIED existing handlers, all of which a tray swipe must coexist with:
- `web-app/src/lib/window/useWindowSwipe.ts`: pane/window swipe; ignores touches within `EDGE_AVOIDANCE_PX = 30` of either viewport edge (L14-19), thresholds 60px / 300ms, passive listeners, requires horizontal axis and no strip scroll. **A right-edge tray handle and edge-swipe-to-open live exactly in the 30px zone window swipe deliberately skips, so edge-open does not conflict with it, but also collides with iOS/Android system back gesture** (INFERRED; the file's comment cites iOS Safari back/forward).
- `lib/hooks/useTerminalGestures.ts:~755-792`: non-passive `touchstart` on the container, non-passive `touchmove`/`touchend` on `document` (so it receives drags that leave the container), cancels on `visualViewport` resize and `orientationchange`. A tray with its own document-level touch listeners, or one that opens mid-gesture and shifts visual viewport, will trigger the `interrupt` path or double-handle events.
- `XtermTerminal.css.ts:31,60,133,188,199,223`: `touchAction: manipulation | none` toggled by `data-gesture-scroll`; `overscrollBehavior: contain` at L62.
- `components/sessions/ScrollModeChip.tsx`, `BoardCard.css.ts:47` (`touchAction:none` drag handle), `CDPViewer.tsx`, `WindowTabStrip.tsx` also bind touch.

Design against:
- Swipe-to-dismiss only on toast/tray **rows** (own element, `touch-action: pan-y`, horizontal axis lock like `classifyAxis`), never on a document-level listener. Toasts are over-terminal, so a swipe starting on a toast must `stopPropagation` or the terminal gesture machine at `document` level will also see `touchmove` (INFERRED from the document-level registration).
- Tray open must not call `preventDefault` on terminal-targeted events; reuse `GESTURE_MOVEMENT_THRESHOLD_PX` (`lib/window/windowGestureConstants`) so thresholds are consistent.
- Add unit tests mirroring `lib/window/__tests__/useWindowSwipe.test.ts` and `useTerminalGestures.test.ts` patterns (jsdom touch synth). Real-device behaviour (Android Chrome over Tailscale) cannot be proven by jsdom; label such claims UNVERIFIED until a manual check.

## 4. Tray vs xterm focus and resize/reflow

- Terminal fit is driven by a ResizeObserver plus dead-band logic (`XtermTerminal.tsx:89`, `:1169` ResizeObserver, `fit()` at `:635,853`), with resize votes reworked in PRs #728/#731 (cited in requirements; not re-read here). Any tray layout that changes the terminal container's width (docked push-aside, `100vw` + scrollbar gutter toggle, `body` overflow lock changing scrollbar width) will emit a resize -> `fit()` -> PTY resize vote -> reflow and scrollback repaint. **Use a `position: fixed` overlay with `transform: translateX`, no layout participation, and do not set `overflow:hidden` on `body`** (INFERRED; that scrollbar-width change is a classic trigger). Add an e2e that records terminal `cols`/`rows` (see `tests/e2e/terminal-stress/tmux-roundtrip.spec.ts:440` pattern) before/after tray open/close and asserts equality.
- Focus: `NotificationPanel` is already `role="dialog" aria-modal="true"` (`NotificationPanel.tsx:151-153`). A modal dialog traps focus and on close restores focus to the opener; if the opener is the bell and the user was typing in xterm, focus will not return to the terminal. Capture `document.activeElement` on open and restore it on close; xterm's hidden textarea needs `.focus()` restored explicitly. Do not autofocus the search input on open on mobile (opens soft keyboard -> `visualViewport` resize -> terminal fit + gesture interrupt). INFERRED.
- A *non-modal* "persistent edge handle" must not be `aria-modal`; the modal semantics apply only while open. Currently the overlay is `aria-hidden` click-catcher (`NotificationPanel.tsx:145`).
- Remount: the "never remounts terminal" requirement fails if the tray is rendered inside a route component or conditionally in a parent that wraps the terminal. Mount it at the layout/provider level, outside the page tree, and keep the open/closed state in `NotificationContext` (already holds `isPanelOpen`, `togglePanel`).

## 5. Mobile soft keyboard and safe areas

- `--viewport-height` and `--keyboard-height` are set at runtime by `ViewportProvider` from `visualViewport` (`web-app/src/app/globals.css:98-103` comment; the setter was not located by my grep of `setProperty("--viewport-height"`, so UNVERIFIED which file owns it). `--bottom-nav-height` is set by a ResizeObserver in `components/layout/BottomNav.tsx:55-66`, with a static default of 72px in `globals.css:88`.
- Toasts at the bottom must offset by `max(var(--bottom-nav-height), var(--keyboard-height)) + env(safe-area-inset-bottom)` and the pane tab strip; when the keyboard is open they should move to the **top** or be suppressed (requirement: never cover the terminal input). Using `bottom: 0` + `100dvh` alone is wrong because `dvh` does not track the Android keyboard when `interactive-widget` is the default `resizes-visual` (INFERRED; hence the existing JS vars).
- Tray panel height: `calc(var(--viewport-height) - header)`; otherwise the bottom action row (Dismiss all) is hidden behind the keyboard when the search input is focused.
- Landscape phone: 3 stacked toasts may exceed viewport height; cap by height as well as count.
- Touch targets >=44px for Dismiss all, +N chip and row actions (requirement); the current `aria-label="Mark activity as read"` buttons at `NotificationPanel.tsx:170-187` should be audited for size.

## 6. Bulk actions and needs-decision items

- `clearAll()` (`NotificationContext.tsx:263-265`) is `setNotifications([])`: **it clears everything including approval and question toasts** (VERIFIED). Wiring a "Dismiss all" button straight to it violates the #738 requirement. It must filter to informational types only (reuse the actionable classification from #738 / `isActionable`-style helper; verify the name before use).
- Staleness sweep (`NotificationContext.tsx:330-345`) already treats actionable toasts differently (6 min vs 5 min); the tray's "clear informational" must use the same predicate, not a second copy (drift risk; also a jscpd hit).
- Pinned approval toasts count against the visible cap of 3? If 3 approvals are pinned, a 4th arrives: decide that approvals never get demoted, but the cap then breaks; spec an overflow rule (stack the rest in "+N more" but keep pinned badge/count on the handle).
- Server side: "mark all read" via history RPC must exclude items with unresolved approvals (`pending_approvals.json`), or resolved-state reconciliation from #738 will flip them back and the user sees a flicker. INFERRED.
- Undo: bulk dismiss should offer `showUndoToast` (exists, `NotificationContext.tsx:~285`) given the "document AI decisions" feedback.

## 7. Stale closures, duplicated timers, cross-tab races

- `showUndoToast` and `showActionToast` call `setTimeout` with **no handle and no cleanup** (`NotificationContext.tsx:~296,~322`). Consequences (VERIFIED code, consequences INFERRED): timers fire after unmount (harmless set-state on filter), but a `clearAll()` followed by a re-add with the same `actionToastKey` is fine only because ids are unique; a *hover-to-pause* or *collapse* feature added on top cannot cancel these timers, so toasts vanish while the user is reading/swiping them. Move auto-dismiss into a per-toast timer registry (`Map<id, timeoutId>`) with pause/resume, cleared on dismiss/clearAll/unmount. Dismiss-all then has one place to cancel.
- A second timer population: the 60s staleness `setInterval` (L330-345) and per-toast timers can both fire; keep one source for expiry.
- Id generation `notification-${Date.now()}-${Math.random()}` is fine but not stable across hydration reconciliation (`backendByDedupKey`; prior pitfalls doc section 1): a toast added locally then re-delivered via server stream can duplicate. Ensure dedup key spans both sources before capping at 3, or "+N more" will count duplicates.
- Cross-tab: `createNotificationSyncChannel` handles only `NOTIFICATION_DISMISSED` and marks history read (`NotificationContext.tsx:~347-362`). New bulk actions (mark all read, clear informational, per-session collapse) need new message types, and **idempotent, id-set based** payloads (not "clear everything now", which would clear items that arrived in the other tab after the click). `collapse per session` state is per-viewer; do not sync it. BroadcastChannel does not deliver to the sending tab and is not available in every webview; fall back to `storage` events (INFERRED).
- Server is the source of truth for read/dismissed; client optimistic updates need rollback on RPC failure, else the tray count and the push badge disagree.
- Stale closure risk: handlers passed into memoized rows (`onView`, `onAcknowledge`, swipe callbacks) capturing `notifications` array; use functional `setNotifications(prev => ...)` as the current code does, and `useRef` for gesture callbacks (same pattern as `optionsRef` in `useTerminalGestures.ts`).

## 8. Accessibility

- Toast container currently: I did not find `aria-live`/`role="status"` on the toast list (grep over `NotificationContext.tsx` and component files returned nothing; INFERRED absent). Adding `aria-live="assertive"` to a stack that re-renders on every update re-announces the whole region; use a single polite live region that announces **only the newly added toast**, rate-limited, and `role="alert"` solely for approvals/failures. With 10+ simultaneous arrivals, announce a summary ("5 new notifications") not each.
- Auto-dismissing toasts violate WCAG 2.2.1 (timing) unless pausable/extendable; pause on hover/focus and when the tray is open. Approval toasts must not auto-expire visually.
- Swipe-only dismiss violates WCAG 2.5.1 (pointer gestures); every swipe action needs a visible button with the 44px target and keyboard path.
- Tray as dialog: focus trap, Esc to close, restore focus (see section 4); `aria-hidden` overlay click-catcher is OK. The Axe CI gate blocks on WCAG AA violations (CLAUDE.md, "UX analysis CI"), so: colour contrast of unread badge on the edge handle, `aria-label` on the icon-only handle, no nested interactive elements inside swipeable rows, list semantics for the virtualized list (`VirtualHistoryList.tsx` exists; a virtualized list needs `aria-setsize`/`aria-posinset` or `role=list` handling).
- Reduced motion: slide animations behind `prefers-reduced-motion`.

## 9. Push notification tag collisions / renotify

- `session-completed-<stableID>` tag + `Renotify:false` (VERIFIED `push/subscriber.go`): same-session completions replace one another silently, appropriate. `notification-<NotificationID>` tag for inline pushes is unique per event, so they never collapse (a burst of 10 produces 10 pushes); `Renotify` is true only for approvals. If the gate downgrades hidden failures to a push, use a per-session+class tag (e.g. `hidden-failure-<id>`) so a spamming producer (166 ceiling events for one review session, VERIFIED above) yields one OS notification, and set `Renotify:false`.
- Web Push `renotify:true` requires a non-empty `tag` or the browser throws; verify the service worker handles that for approvals (not read this session; UNVERIFIED).
- The click handler must be updated for hidden-session deep links (section 2); `data.url` is built from `buildSessionURL`.

## 10. CI gates and e2e conventions

- **jscpd**: `web-app/.jscpd.json` threshold is **0.14** (VERIFIED), whereas requirements.md says 0.12 and CLAUDE.md says 0.1% and ~0.09% baseline; reconcile before budgeting, and memory notes the margin was 0.10% after PR #815. minLines 20 / minTokens 200: expect hits from (a) a second copy of the actionable-vs-informational predicate, (b) toast and tray row components forked from `NotificationItem`, (c) copy-pasted swipe hooks. Extract shared row + one predicate + one gesture hook up front. Go side: `dupl` is new-code-only, so four copies of the gate logic across subscribers will fail; one shared function avoids it.
- **Complexity gates** (gocognit/funlen etc. apply to modified existing functions, per memory `instinct_go_git_native_plumbing_gotchas`): `NotificationService.SendNotification` is already long (~110 lines); extracting the hidden lookup into the shared gate also pays down that gate.
- **Feature registry**: run `make registry-generate` when adding RPCs/components/markers; CI "registry out of date" failures are usually stale base (memory `instinct_ci_tests_against_live_base_not_stale_merge`).
- **e2e** (CLAUDE.md "E2E Tests", skill `e2e-test-conventions`): header `// @feature ...`; no `waitForTimeout` (toast auto-dismiss and tray animation tests must use `expect(...).toBeHidden()` / `toHaveCount`, or clock-controlled `page.clock`); `data-testid`/ARIA only; page helpers in `tests/e2e/pages/` (`NotificationPanel.ts` already exists there, extend it rather than adding a new one). Isolated server on ephemeral port; swipe gestures in Playwright need `hasTouch` context + `touchscreen`/CDP `Input.dispatchTouchEvent`; Chromium-emulated touch does not prove Android behaviour.
- New Go tests: follow `deterministic-fast-tests` (no real sleeps for the dedup/coalesce windows; `notifications/subscriber.go` has a flush ticker, inject a clock).
- Rollout flags: two live-settable flags via the feature-flag RPC (memory `feedback_rollout_flags_live_settable_no_env_vars`); the fallback to the old toast list must keep `clearAll` semantics unchanged.

## Top risks, ranked

1. Gate bypass through another channel/producer (store ingests everything today: 283 hidden occurrences already in history). Mitigate with one shared classifier + enumerating test.
2. `clearAll()` clears approvals (VERIFIED) once wired to a button.
3. Tray layout causing terminal resize/focus loss (reflow, soft keyboard).
4. Policy keyed on priority rather than class (ceiling events arrive at both priorities).
5. Timer leaks/no pause breaking swipe + hover UX and a11y timing.
6. Read-only hidden view without server-side write guard.
