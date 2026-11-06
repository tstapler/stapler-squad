# Stack Research: notification-tray-and-hidden-session-gate

Builds on `project_plans/notification-revamp/research/stack.md` (not re-derived). VERIFIED = source opened this session; INFERRED = deduced.

## Verdict

**No new dependency is required.** Everything needed is already in `web-app/package.json` or the Go tree. The one optional addition is `@use-gesture/react` for swipe-to-dismiss, which is NOT recommended: the repo has hand-rolled touch/gesture hooks and a pointer-event pattern to copy (below).

## Web-app stack (VERIFIED, `web-app/package.json`)

| Concern | In repo | Line |
|---|---|---|
| Framework | next 15.3.2, react ^19.0.0, react-dom ^19.0.0, typescript ^5.9.3 | 98, 99, 101, 167 |
| RPC | @connectrpc/connect ^2.1.1, connect-web ^2.1.1, @bufbuild/protobuf ^2.11.0 | 44, 59-60 |
| Styling | @vanilla-extract/css ^1.20.1, recipes ^0.5.7, next-plugin ^2.5.1 | 83, 147-148 |
| Dialog primitive | @radix-ui/react-dialog ^1.1.15 (focus trap, portal, aria-modal built in) | 75 |
| Other radix | accordion ^1.2.17, tabs, tooltip, slot | 74-78 |
| Virtualization | @tanstack/react-virtual ^3.13.25 and react-virtuoso ^4.18.7 (both present) | 82, 104 |
| DnD | @dnd-kit/core ^6.3.1 | 61 |
| Tests | jest ^30.2.0, @playwright/test ^1.57.0 | 156, 131 |

No toast library (sonner etc.), no gesture library (`@use-gesture`, `react-swipeable`, `framer-motion` absent from the grep of package.json).

### Existing building blocks to reuse

- **Toasts**: custom. `NotificationContext.tsx` renders the stack at ~L483-501 in a `position: fixed; bottom:0; right:0; zIndex: zIndex.toast; pointerEvents:none` wrapper, one `<NotificationToast>` per `notifications` entry, no cap. `clearAll` is defined at L263 and exposed in context (L75, L466) but nothing in the UI calls it. Cap/"+N more"/"Dismiss all" is a change to this block plus `NotificationToast.tsx`/`NotificationToast.css.ts`, not a library swap.
- **Panel**: `web-app/src/components/ui/NotificationPanel.tsx` (+ `NotificationPanel.css.ts`, `NotificationItem.tsx`, `NotificationsNavBadge.tsx`) already has `TypeFilter`, search, history. Evolve it (matches requirements' "evolve, don't replace").
- **Dialog/overlay**: `web-app/src/components/ui/Modal.tsx` and `OnboardingModal.tsx`/`BacklogTourModal.tsx` import `@radix-ui/react-dialog`. For a non-modal edge tray that must not steal xterm focus, use `Dialog` with `modal={false}` (INFERRED from Radix API; verify in a spike), or a plain `role="complementary"` fixed panel. A modal Dialog would trap focus away from the terminal (requirements rabbit hole).
- **Z-index tokens**: `web-app/src/styles/theme-contract.css.ts:202+` `zIndex` scale: header 100, dropdown 500, slideOver 700, modal 1000, bottomNav 1050, mobilePickerSheet 1065, dialog 1070, toast above dialog but below Radix overlay 1100. `slideOver: 700` is the natural slot for the tray on desktop; on mobile it must exceed bottomNav (1050) if it overlays the nav (INFERRED).
- **Viewport/soft keyboard**: `components/providers/ViewportProvider.tsx` sets `--viewport-height` from `window.visualViewport` (L35-52) and exposes `isVirtualKeyboardOpen` (L10). Reuse for tray height and toast offset.
- **Virtualization**: `components/history/VirtualHistoryList.tsx` (react-virtuoso/tanstack pattern already in repo) for hundreds of tray rows.
- **Grouping**: `lib/utils/notificationGrouping.ts` (from prior research) and `useShowMore.ts`, `ui/Collapsible.tsx` (radix accordion) for per-session collapse.
- **Gesture prior art for swipe-to-dismiss**: `lib/window/useWindowSwipe.ts` (edge avoidance `EDGE_AVOIDANCE_PX = 30`, `SWIPE_DISTANCE_THRESHOLD_PX = 60`, `SWIPE_TIME_THRESHOLD_MS = 300`, axis classification, scroll-moved guard, L10-30); `lib/hooks/useTerminalGestures.ts`, `lib/terminal/gestureMachine.ts`, `lib/terminal/touchDrag.ts`. Pane swipe lives on the window tab strip (`components/window/WindowTabStrip.tsx`), so toast/tray swipe zones are naturally disjoint from the xterm surface as long as handlers attach to the toast/row element, not `document` (INFERRED; confirm no document-level listener in `useWindowSwipe`). Reuse its threshold constants/axis helper; do not add a dependency.
- **Feature flags (client)**: `lib/contexts/FeatureFlagsContext.tsx` exposes `useFeatureFlags()` (`{flags, isLoading}`), used at `app/backlog/layout.tsx:5-8` and `app/settings/features/page.tsx`. The new-UI rollout flag reads from here.
- **Hidden-session deep link already half-works (answers Open Question 3, partly)**: `app/page.tsx:222-250` — when `?session=<id>` is not in the (hidden-filtered) list, `fetchHiddenSessionFallback` calls `getSession(id)` and `routeToResolvedSession`. So the failure mode is likely not the router but either (a) the stableID/ID form in the push URL, (b) read-only enforcement absent, or (c) `getSession` hidden handling. Needs targeted verification in the Architecture/Pitfalls phase; `ListSessions` filtering is at `server/services/session_service_crud.go:60-61` and `:100-101`. UNVERIFIED which of these actually breaks "View Session".

## Web Push click -> deep link (VERIFIED, `web-app/public/push-sw.js`)

- Registered from `lib/hooks/usePushNotifications.ts:89,147` via `navigator.serviceWorker.register("/push-sw.js")`. Cache SW is separate (`cache-sw.js`, per header comment L1-4).
- `notificationclick` (L51-80): closes the notification; `dismiss`/`later` return; `open`/`review`/default read `event.notification.data?.url || '/'`, then `clients.matchAll({type:'window', includeUncontrolled:true})` and `client.focus()` if `client.url.includes(urlToOpen)`, else `clients.openWindow(urlToOpen)`.
- **Gap**: an already-open tab at a different URL is only focused if its URL contains the target; otherwise a new window opens. For "open without reload/remount" the SW should instead `client.focus()` the existing window and `client.postMessage({type:'notification-click', url})`, and the page navigates via the Next router (INFERRED improvement; a new `message` listener on the client is needed — the SW's existing `message` handler L82-86 only handles `SKIP_WAITING`). This is a no-new-dependency change.
- Server payload `url` is `buildSessionURL(id)` = `/?session=<id>&tab=terminal` (`server/push/subscriber.go:301-303`), set in `data` at L211 and L297. The `tab=terminal` hint is write-capable; a hidden-session link needs a read-only tab/param (e.g. `tab=output` or `&readonly=1`) — design decision for the architecture phase.

## Go side (VERIFIED unless noted)

- **EventBus**: `pkg/events/bus.go:25` `type EventBus`, `Subscribe(ctx) (<-chan *Event, string)` L51, `Publish` L72. Event types `EventSessionUpdated` (`pkg/events/types.go:18`) and `EventNotification` (`:36`). It is a fan-out bus: each subscriber gets every event, so a gate cannot be a single bus filter without either a gating publisher wrapper or per-subscriber checks.
- **Subscribers to the bus** (the channel inventory for Open Question 1, via `bus.Subscribe(`): `server/push/subscriber.go:32` (web push), `server/notifications/subscriber.go:43` (history store), `server/analytics/subscriber.go:44`, `server/services/backlog_github_forward_sync.go:54`, `server/services/backlog_service_events.go:107`, `server/services/workflow_service_events.go:60`, `server/mcp/tools_backlog.go` (~L755). Only the first two deliver user-facing notifications; the push and notifications subscribers are the two that need the gate. Producers: `server/review_queue_manager.go:444` publishes `EventNotification`; the full producer list for `EventNotification`/`EventSessionUpdated` still needs a pass (INFERRED incomplete).
- **Choke-point option (INFERRED)**: a shared pure function `ShouldDeliver(evt, sessionSnapshot) (bool, reason)` called at the top of both `push.buildStatusChangeNotification` path and `notifications` subscriber (+ `NotificationService.SendNotification`), or — stronger — resolve Hidden once at publish time and stamp it onto the `Event` (new bool field) so every subscriber reads one field. Stamping at publish keeps the gate in one place and avoids stale-snapshot lookups (requirements' feasibility risk); `Snapshot()` per `.claude/rules/instance-lock-free-reads.md`.
- **Push delivery**: `server/push/notifier.go:23-28` `Notifier` interface (`Send(ctx, DeliveryNotification) error`, `Name() string`); `WebPushNotifier` (`Name()` = `"web-push"`, L40). Test mocks in `subscriber_test.go:356,368` show the pattern for gate tests. Metrics/logging for suppression: slog already used (`log.Info("DeliverySubscriber started"...)` L44).
- **Feature flags (server)**: `config/config.go:356-359` `FeatureFlags map[string]bool`; flag keys are consts like `StreamHubFeatureFlag = "stream_hub"` (L485), `TymuxFeatureFlag` (L492) read via `cfg.GetFeatureFlagWithDefault(key, default)` (e.g. L509, L528, L537). RPCs `GetFeatureFlags`/`UpdateFeatureFlag` at `proto/session/v1/session.proto:478,481`. New flags (`hidden_session_gate`, `notification_tray_v2`) follow this pattern; default-on gate uses `GetFeatureFlagWithDefault(key, true)`. Live-settable and "per-scope override" — whether the existing store supports per-scope (not just global) is UNVERIFIED; check `UpdateFeatureFlagRequest` fields before promising it.
- **"via tmux" toast (Open Question 2)**: `grep 'via tmux'` across Go/TS/JSON/sh in the repo found no literal producer. Likely generated by Claude Code's own notification hook payload relayed through the hook path (INFERRED). Needs runtime/log investigation by the Pitfalls/Architecture agent.

## New dependencies

None required. If the team wants a gesture lib anyway, `@use-gesture/react` would be the candidate; rejected here because `useWindowSwipe.ts` + `gestureMachine.ts` already encode the repo's edge-avoidance and threshold conventions, and a second gesture system risks conflicting with xterm touch handling.

## Open items for later phases

1. Why "View Session" fails today despite `fetchHiddenSessionFallback` (ID form vs `GetSession` hidden handling vs `tab=terminal`).
2. Per-scope support in `UpdateFeatureFlag`.
3. Exhaustive `EventNotification`/`EventSessionUpdated` producer list.
4. Non-modal Radix Dialog vs plain landmark for the tray (focus behavior with xterm).
5. SW `postMessage` handoff vs `openWindow` for in-place navigation.
