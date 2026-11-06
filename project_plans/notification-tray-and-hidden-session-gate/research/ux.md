# UX Research: notification-tray-and-hidden-session-gate

Scope: requirements.md items 3 (toast stack), 4 (side tray), and the user-facing half of 2 (hidden-session reachability). Builds on `project_plans/notification-revamp/research/ux.md` (verb discipline: Skip = temporary, Dismiss = permanent for a row, Mark read = visual only; "needs a decision" empty state is a success state; counts capped at 99+; auto actions must be visible). Not re-derived here.

Confidence labels: VERIFIED = opened in this repo or in the screenshots; PRIOR-ART = established product/standard behavior from my knowledge, no live link fetched this session (WebSearch not used); INFERRED = my reasoning.

## 0. Current UI, as built (VERIFIED)

- Toast container (`web-app/src/lib/contexts/NotificationContext.tsx` ~L491-509): `position: fixed; bottom:0; right:0`, renders **every** toast in `notifications` with no cap.
- `NotificationToast.css.ts`: each toast is itself `position: fixed` (L11-12), so they overlap via `:nth-child(2)`/`(3)` hard-coded `bottom: 24px+120px / +240px` (L38-43). On mobile the media rule (L27-34) clears `--bottom-nav-height`, `--mobile-pane-tab-strip-height` and safe-area, but the `:nth-child` selectors have higher specificity than the media-query rule, so toasts 2 and 3 likely ignore that clearance on mobile (INFERRED; confirm in a browser). Toasts 4+ have no rule at all and sit at the base offset, i.e. on top of toast 1.
- Neither mobile rule accounts for the soft keyboard: `--viewport-height` is used only for `maxHeight`, and `bottom` is relative to the layout viewport. Screenshot 2 (VERIFIED, by eye) shows the keyboard open with the terminal's "Page keys" row clipped; any toast would be stacked on the same strip.
- Screenshot 1 (VERIFIED): two ~170px toasts ("PR needs attention" with View Session / Dismiss; "Claude Notification ... via tmux" with Focus Window / View Session / Dismiss) cover roughly the bottom 30% of the viewport, including the terminal input line, and the second is clipped behind the first. The "keystroke dropped - reconnecting" status pill is also overlapped, so the toast hides a system status.
- Toast a11y: every toast is `role="alert"` with `aria-live` polite or assertive (`NotificationToast.tsx:166-167`). `role=alert` already implies assertive, so the polite override is contradictory, and N toasts = N interrupting announcements.
- Auto-close exists (`toastAutoCloseMs(type)`, `NotificationToast.tsx:73`); pause-on-hover/focus is not verified.
- Panel (`NotificationPanel.tsx`): modal `role="dialog" aria-modal="true"` with a click-to-close backdrop overlay (L145-153). It has search, type pills, "Mark activity as read", "Clear history", grouped list via `groupNotifications()`, and a separate auto_approved section. It is opened only by the header bell (`Header.tsx:133`, `aria-label="Open notifications"`, unread badge, uncapped `{unreadCount}` at L141). No `Escape` handling or focus handling was found by grep in the panel file (UNVERIFIED: may live in a shared hook).
- Mobile reach: BottomNav items are page links plus a "More" sheet (`BottomNav.tsx`). Whether the Header (and so the bell) is visible on the mobile terminal view was not verified; screenshot 1/2 show no header, only the session chrome, so on a session page the bell is plausibly unreachable (INFERRED, matches the requirement text "hard to reach on mobile").
- Touch conflicts (VERIFIED): `useWindowSwipe.ts` does horizontal swipe to change window with 30px edge avoidance, 60px / 300ms thresholds; it is attached to the window tab strip. Terminal touch is a gesture machine (`lib/terminal/gestureMachine.ts`, `XtermTerminal.tsx`) handling vertical scroll/long-press; `TerminalOutput.css.ts:466` sets `touchAction: manipulation`.

## 1. Comparable patterns (PRIOR-ART)

| Product | What works | Why / lesson for us |
|---|---|---|
| Android notification shade | Heads-up (transient, top) then collapses to a grouped shade; groups per app with summary row and expand; swipe horizontally to dismiss one, "Clear all" pinned at bottom; ongoing/needs-action items are not swipeable | Two-tier model: a transient interruption plus a durable home. "Clear all" does not clear ongoing items, which is exactly our "never clear a pending decision" rule. |
| iOS Notification Center | Stacks per app with count, tap to expand; Time Sensitive vs Passive delivery levels; Clear (x) is per group with confirm tap | Per-group clear with two-step confirm prevents accidental mass loss. Delivery level (our Hidden policy) decides interruption, not storage. |
| macOS Notification Center | Group by app/thread; banner auto-hides, alert stays until acted on | Banner vs Alert maps to routine vs pinned-decision toasts. |
| GitHub inbox | Done vs Unsubscribe; Save; select-all then bulk Done; filters Participating/Watching; keyboard `e`, `j/k` | Bulk "Done" is safe because inbox contains only things already seen. Our bulk-clear must be scoped to informational rows. Keyboard triage is a power-user must for a single operator. |
| Linear inbox | Single-key actions, snooze-until-activity, "mark all read" under a menu, not a primary button | Destructive bulk actions live in a menu, frequent safe ones (mark read) are one tap. |
| Slack Activity | Tabs (All / Mentions / Threads); unread dots; no popups when you are looking at that surface | Suppress toast when the user is already looking at the thing it announces (e.g. the session in view). |
| VS Code notification center | Toasts bottom-right, max few visible, bell in status bar with count, "Do Not Disturb" mode that sends everything to the center, error toasts sticky, info auto-hide; progress notifications | Closest analogue: toast -> bell -> center, with errors sticky and routine info demoted. Our tray should adopt this "DND demotes all toasts to the tray" idea as a toggle ("Quiet mode"). |
| Sonner / react-hot-toast | Sonner: stacked collapsed deck (default visibleToasts=3), expands on hover/focus (and on touch tap), timers pause while expanded, swipe to dismiss, `expand` and `closeButton` options, offset props incl. mobile offset. react-hot-toast: simple list, no deck | Deck-with-expand solves "cap + still reachable" without a "+N" row; we still want an explicit "+N more" because on touch there is no hover. |

Net: every mature product separates (a) a bounded, ephemeral interruption layer from (b) an unbounded durable home, and makes "clear all" skip items that are still pending. Our design matches that if the toast layer is capped and the tray becomes the home.

## 2. Mental models and gesture conventions

- **Toast** = "something just happened, glance and move on"; implicitly disposable. **Tray/shade** = "things I may have missed, temporary pile". **Inbox** = "a to-do list I intend to drive to zero" (GitHub/Linear). The existing Notifications page is the inbox (needs a decision / grouped / auto-handled). The tray should be the shade: quick access, bulk triage, link out to the page for depth. Do not make the tray a second inbox with different state; read/dismissed state must be shared (requirements rabbit hole).
- Verb discipline carried over: tray row actions = **Dismiss** (permanent for that row), **Mark all read** (visual only), **Clear informational** (bulk Dismiss scoped to non-decision types). Never label a bulk action just "Clear all" if it excludes decisions; say "Clear informational (N)" so the count shows what will go and the label shows what will stay.
- Swipe-to-dismiss conventions: horizontal swipe on a row dismisses; Android and iOS both exclude items needing action and show an undo affordance; movement threshold ~35-50% of width or fling velocity; vertical movement cancels it so list scroll still works. Sonner follows this.
- Conflicts here (VERIFIED/INFERRED):
  - Window swipe lives on the tab strip only, so toast/tray swipe does not collide with it; keep tray rows out of the strip's region and keep the 30px edge-avoidance zone free (the tray's edge handle must not start inside the 30px OS back-swipe zone on iOS; on Android gesture nav, right-edge drag from the screen edge can trigger Back).
  - Terminal vertical scroll/long-press (`gestureMachine`): toasts are siblings above it in z-order, so touches on a toast never reach xterm. Safe regions for swipe: toast surface and tray rows. Use `touch-action: pan-y` on swipeable rows so the browser keeps vertical scroll and only horizontal motion is claimed. Do not add any handler to the terminal element.
  - Do not use swipe-to-dismiss as the only path: always keep the visible close button (WCAG 2.5.1 pointer gestures: single-pointer alternative).
- Tray open/close gesture: avoid an edge-drag-to-open handle on mobile (collides with OS back gesture and `useWindowSwipe` edge zone). Use a tap on a visible handle/chip instead; allow drag-to-close (swipe toward the edge) on the open sheet.

## 3. Accessibility

- **4.1.3 Status Messages (AA)**: toasts are status messages; they must be announced without taking focus. Use a persistent, always-mounted live region (as prior research concluded) instead of per-toast `role=alert` with N interrupts. Recommendation: one container `role="region" aria-label="Notifications"` holding a polite `role="status"` live region for routine/info; pinned approval/error toasts may use `role="alert"` (assertive), but only for those types, and remove the contradictory `aria-live="polite"` override. When toasts are capped, announce the summary once ("3 notifications, 5 more in tray"), not each collapsed one.
- **2.2.1 Timing Adjustable (A)**: auto-dismissing toasts need a way to turn off/extend. Meet it by: timers pause on hover, focus-within, and while the tray is open; decision/error toasts never auto-close; every toast's content is recoverable from the tray, so expiry loses nothing. A "Quiet mode"/persist setting is a bonus. Also 2.2.2 (pause/stop/hide) for any moving animation.
- **2.5.8 Target Size (Minimum, AA 2.2)**: >=24x24 CSS px is the floor; the requirement's 44px is stricter and the right call for the toast close button (currently an x icon; size unverified) and per-row dismiss. 2.5.1 gesture alternative covered above. Also 2.4.11 Focus Not Obscured (AA 2.2): a toast must not cover the focused terminal input or a focused control; this is a real failure today (screenshot 1).
- **Tray semantics**: current `role="dialog" aria-modal="true"` is wrong for the new intent. A modal dialog traps focus and makes the page inert, which contradicts "no navigation/remount, keep working with the terminal". Recommend a non-modal disclosure: bell/handle is a `<button aria-expanded aria-controls="notification-tray">`; tray is `<aside aria-label="Notifications">` (landmark `complementary`) or `role="dialog"` WITHOUT `aria-modal` on mobile where it covers most of the screen. On mobile full-height sheet, modal behavior is acceptable (focus trap, `inert` background, Escape, return focus to the trigger) because the terminal is occluded anyway; on desktop keep it non-modal and non-trapping.
- **Focus management that does not steal xterm focus**: never call `.focus()` on toast arrival. On tray open via pointer, move focus to the tray container (or heading) only if the open was user-initiated; on close, restore focus to the previously focused element, which for terminal users is xterm's textarea (store `document.activeElement` at open; restore with `preventScroll`). Opening via the hotkey must not type the hotkey char into xterm: bind at capture phase and `preventDefault`, and ignore when xterm has an IME composition open.
- **Keyboard**: a global shortcut to toggle the tray (suggest `g n` or `Alt+N`; must not collide with terminal keys, since xterm swallows most keys, a modified chord handled in capture phase is safest; add to the existing keyboard shortcut dialog); `Esc` closes the tray; within tray `j/k` or arrows, `Enter` open, `Delete`/`x` dismiss, `Shift+Delete` or menu for bulk. Roving tabindex for the list.
- **Reduced motion**: `prefers-reduced-motion: reduce` replaces the 300ms slide/scale transitions (`NotificationToast.css.ts` L26) with opacity-only or instant; no deck-expansion animation; disable swipe fling animation.
- **Color not alone**: priority/type must keep icon + text (prior research); applies to the new "failure" vs "needs-human" vs "info" row badges for hidden-session items.

## 4. Edge cases needing graceful UX

| Case | Recommended behavior |
|---|---|
| 50+ (hundreds) notifications | Tray header shows capped badge (`99+`, matching `NavBadge`). List paginated/virtualized, grouped per session with collapsed groups by default beyond the top few; the header shows "N need a decision" first, counts of informational secondary. No synchronous full-history fetch on open: render from the in-memory slice, fetch next page on scroll (`historyHasMore`/`loadMoreHistory` already exist). Bulk "Clear informational" is a server-side filtered op, not N RPCs, with an undo toast (existing `showUndoToast` pattern). |
| Offline / disconnected | Tray shows a thin banner "Offline - showing cached; last updated <time>" using `historyLastUpdatedAt`/`historyError`. Dismiss actions queue optimistically, show a "pending sync" mark, and roll back with a visible message if the RPC fails (not silent). Toasts for new events cannot arrive; do not show a misleading empty-state "All caught up" while disconnected. |
| Hidden-session failure notification | Row/toast carries a "Background" chip (text, not color only). Primary action "View output" opens a **read-only** view: no terminal input, no send-keys toolbar, a persistent "Read-only (background session)" banner, and the page still loads if the session is later deleted (show "Session no longer available" with the notification's captured message, not a 404). The notification must never fail with "session not found" (current dead end). Provide "Open in Background activity" fallback. Push click deep-links to the same read-only view. |
| Soft keyboard open (mobile) | Toasts and tray anchor to `--viewport-height` (visual viewport), not the layout viewport; when the keyboard is up, collapse the toast deck to a single-line chip ("N notifications") docked at the **top** (below status bar/banner), never over the input row or the keyboard accessory row ("Page keys"). Tray as a bottom sheet is dangerous with the keyboard open: use a top-anchored or full-height sheet sized to `--viewport-height`. Do not trigger keyboard dismissal or terminal resize votes on open. |
| Landscape (mobile) | Viewport height may be ~300px: toast max one visible, compact one-line variant with actions in overflow; tray becomes a right-edge panel (like desktop) with width ~min(360px, 50vw) rather than a bottom sheet; honor `env(safe-area-inset-left/right)` notch insets. |
| Tray open while terminal streams | Overlay only (`position: fixed`, `transform` animation), no layout change to the terminal container: no resize, no remount (this is the success metric). On desktop non-modal: terminal keeps rendering and keyboard focus stays reachable by Esc/click; avoid backdrop dimming that suggests pause. On mobile full-height sheet the terminal keeps streaming underneath; add `contain: layout` on the tray and avoid `backdrop-filter` blur over the streaming canvas (WebGL repaint cost; recent atlas work #944). |
| Needs-decision item in a bulk action | Excluded and listed in the confirm text ("2 awaiting decision kept"). Row stays pinned at top of tray. |
| Toast arrives for the session the user is viewing | Demote straight to the tray (Slack pattern), with at most a badge pulse, unless it is a decision/failure. |
| Duplicate while tray is open | Increment the group's count in place; do not reorder rows under the user's finger (jumping targets); show "N new - tap to refresh" if order would change. |
| Tray empty | Success state copy per prior research; if filtered, "No matching notifications" (already exists). |

## 5. Jobs to be done

- **Functional**: (1) "Know right now if anything needs me, from wherever I am, without losing my place in a terminal." (2) "Triage a backlog of events in one pass: clear the noise, act on the few decisions." (3) "When something in the background broke, get to its output in one tap."
- **Emotional**: confidence that background automation is quiet when healthy (zero routine pings, matching the Hidden policy) so that a ping means something; relief from toasts physically covering work; no fear that "clear" destroyed a pending decision.
- **Social**: none, single operator (consistent with the prior research's explicit finding); no invented social job.

## 6. Recommendations

### 6.1 Tray form factor per breakpoint

| Breakpoint | Form | Trigger | Notes |
|---|---|---|---|
| Desktop >=900px | Right-edge **overlay** panel, 380-420px wide, **non-modal** (no backdrop dim, no focus trap, page stays interactive), `Esc` closes. Overlay not push-aside (resolves Open Question 6: push-aside resizes the terminal) | Header bell (existing, make it `aria-expanded`), a persistent edge handle with capped unread count on the right edge, hotkey | Resolves Open Q6 as overlay. Edge handle click-to-toggle; drag optional. |
| Mobile portrait <900px | **Bottom sheet with two snap points**: peek (one-line summary + top item) and ~85% height; sized to `--viewport-height`; modal behavior only at the expanded snap | A floating notification chip/button (>=44px, with capped count) placed above the BottomNav, plus the toast stack's "+N more" chip; also reachable from the BottomNav "More" sheet | Gives mobile a reachable trigger on session pages where the header bell is absent. With soft keyboard open, use a top-anchored full-height sheet instead (see 4). |
| Mobile landscape | Right-edge panel as desktop but ~50vw max | Same chip | |

Keep one `NotificationPanel` implementation with a layout variant, not two components (jscpd gate, requirements "evolve NotificationPanel").

### 6.2 Toast cap and stack behavior

- Max **3 visible** (matches the success metric), rendered as one container (`role=region`), not N `position: fixed` siblings; remove the `:nth-child` offset hacks. Order: pinned decisions first, then newest.
- Beyond 3: a "+N more" chip (button, opens tray, `aria-label="N more notifications, open tray"`). Collapsed deck look is optional; on touch there is no hover, so the chip is the contract.
- Pinned (never auto-close, never demoted): approval_needed / needs-human, and failures. Routine info/completions demote straight to the tray (badge increments, no toast), with a Quiet-mode toggle that demotes everything non-decision.
- Mobile: single compact toast (title + one action + close) anchored **top** of the viewport below the status bar and any memory/warning banner, or above the BottomNav at bottom only when no soft keyboard and not on a terminal tab; never over the terminal input row. If the stack has more than one item on mobile, show 1 toast + "+N more".
- "Dismiss all" appears in the deck header whenever >=2 toasts; it dismisses toasts only (history/tray rows remain, so nothing is lost) and never discards a pending decision from the tray. Wire to existing `clearAll()` but scope it to non-pinned toasts and leave the tray copies intact.
- Timers pause on hover/focus-within/while tray open; dedupe by (session, type) with a count.
- Swipe: horizontal swipe dismisses a non-pinned toast on touch (with close button as the non-gesture path); pinned toasts only swipe-to-tray (demote), not dismiss.

### 6.3 Bulk action set and where each lives

| Action | Location | Scope / safety |
|---|---|---|
| Dismiss all (toasts) | Toast deck header | Toasts only; decisions stay pinned in the tray |
| Mark all read | Tray header, primary icon button | Visual only; reversible |
| Clear informational (N) | Tray header overflow menu (not a primary button) | Excludes needs-decision and unresolved failures; confirm text lists what is kept; undo toast |
| Collapse / expand all groups | Tray header overflow menu | UI only |
| Dismiss group | Group row swipe or menu | Per-session; refuses if the group contains a pending decision (offers "Dismiss N informational") |
| Clear history | Move to overflow menu with confirm (currently a top-level header button, `NotificationPanel.tsx:178`) | Destructive; excludes pending decisions |
| Quiet mode toggle | Tray header | Persisted per device |
| Open Notifications page ("Review all") | Tray footer link | Escape hatch for depth |

### 6.4 Background activity placement

Put it **inside the tray as a collapsed section/tab** ("Background" segment next to "Notifications"), not as a main-nav page and not in the session list (requirements reject un-hiding). Reasons: it answers "is anything running/broken in the background?", which is the same glance-and-triage context as the tray; hidden sessions are infrastructure for the operator (review/triage/diagnose), not destinations. Contents: one row per hidden session (role label such as review/triage/diagnose, linked item/PR, status, last event time), failures and needs-human on top, routine completions summarized as a count line ("12 completed OK today") rather than rows, which keeps the Hidden policy's quiet promise. Row tap opens the read-only view. The section badge counts only failures/needs-human, so Background activity never inflates the bell count with routine items. Consider also a link from the Notifications page for desktop discoverability (INFERRED, optional).

### 6.5 Open questions answered or sharpened for planning

- Open Q5 (swipe regions): safe on toast surface and tray rows with `touch-action: pan-y`; unsafe on the terminal, the tab strip (owned by `useWindowSwipe`), and the 30px screen edges.
- Open Q6: overlay, non-modal on desktop.
- New decision for planning: pick one live region owner (the toast container) and fix the `role=alert` + `aria-live=polite` contradiction as part of the stack rewrite.
- Verify early in a browser (not done here): bell visibility on the mobile session page, whether `:nth-child` specificity really defeats the mobile `bottom` rule, and focus/Escape handling in `NotificationPanel`.
