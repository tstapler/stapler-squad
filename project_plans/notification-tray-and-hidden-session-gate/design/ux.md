# UX Design: notification-tray-and-hidden-session-gate

**Date**: 2026-10-07
**Status**: **Review-repair iteration 4 (2026-10-09): one criterion was added, RP-17 (177 criteria): a hidden question that is not replyable because its session's hook has no valid proof says so ("Reply unavailable for this session (started before reply support was enabled; restart the session)") instead of the silent "Open the terminal to answer"; Reply works only for sessions started after hook proof ships (an accepted limit, default applied); not yet re-reviewed.** **Review-repair iteration 3 (2026-10-09): no criterion was added or removed (still 176); the Reply card shows the pane-derived question text when it differs from the hook's text, the indeterminate-send card also covers an Enter that was withheld because the pane changed during the settle wait, and Reply is sequenced last in Epic 5 (until it ships, every hidden `INPUT_REQUIRED` surface shows "View output" only).** **Review-repair iteration 2 (2026-10-09): the indeterminate-send copy now says the text may be typed but not submitted, a superseded outcome without a newer question has its own copy and RP-16 was added (176 criteria; a question with no identifying text is not replyable, no `IN_PROGRESS` state exists because the outcome was removed, a busy write lease shows the existing "Could not send - Retry"); not yet re-reviewed.** Review-repair iteration 1 (2026-10-09): the Reply card states were rewritten for the focused re-review (indeterminate send with no Retry, superseded and stale questions, no question text, multi-question degradation, refused device); RP-13..RP-15 added (175 criteria); not yet re-reviewed.** Reconciled with `implementation/plan.md` in Phase 3 iteration 2 and updated in iteration 3 (operator decisions O2 audited Reply, O6 fixed handle; single mobile tray affordance; pinned = server field). Section 21 records resolutions, not open conflicts. Phase 4 patch (2026-10-08): Reply length is in bytes; Reply Retry and the `ClearNotificationHistory` `kept` response are specified (TM-10, RP-7); FG-7 added for the shadow-stats status line. **Triad iteration 1 patch (2026-10-09)**: one `Announcer` owns every live region (TD-8, XA-7, XA-15); coarse-pointer focus rules and a non-trapping expanded sheet on touch (D6, TS-4, TS-9, TK-4, XA-12); tray empty/loading/offline states have owning tasks with one connectivity source and one offline policy (Surface 14, TE-1..TE-7); the deck has a single bulk control "Move all to tray" (Surface 3); the pinned group is "Needs attention" and an auto-remediating WARNING is not pinned (D4, TD-5, TR-10, TR-11); undo placement and a configurable, pausable undo window (TB-4, TB-10, TM-3, TM-12); pinned phone card auto-collapse (TD-14); desktop deck overlap guard (TD-15); in-drag swipe feedback and a visible tray control (TC-1, TC-11); Reply prompt length and gated quick-picks (RP-11, RP-12) and banner copy (RO-12); undoable versus irreversible menu separation (TM-11); RO-4 quantified; the former Stretch items (Quiet mode, hotkey, `top-sheet`, `landscape-panel`, push handoff) are in scope and sequenced last (operator decision O8). Placement is decided in `decisions/ADR-009-mobile-toast-stack-docks-at-top-of-session-page.md`; the Reply write path is `decisions/ADR-010-audited-reply-to-pending-question.md`.
**Inputs**: `requirements.md`, `research/ux.md`, `implementation/plan.md` (Epics 3, 4, 5), ADR-005/006/007/008, `project_plans/notification-revamp/design/ux.md` (shipped surfaces 1-11), the two operator mobile screenshots (`uploads/1791432550193-...-10386.png`, `uploads/1791432574187-...-10387.png`).
**Builds on shipped behavior**: verb discipline (Skip = temporary, Dismiss = permanent for one row, Mark read = visual only), the shipped label **"Mark activity read"** (not "Mark all read"), shipped "Clear history" with the server-side "never delete an unread actionable record" guarantee, success-state empty copy, counts capped at `99+`.

Evidence labels: VERIFIED = seen in the screenshots or in a file opened this session; INFERRED = my reasoning; DECISION = a choice this document makes (flagged in section 14 if it deviates from plan.md).

## 0. What the screenshots show (VERIFIED, by eye)

- Screenshot 1 (portrait, ~390 CSS px wide): two toasts, about 170px tall each, fill the lower ~30% of the screen and cover the terminal input line and the "Window / Terminal" chrome below it. The second toast ("Claude Notification ... via tmux", from a `worktrees/triage-...` path, so very likely a hidden triage session) is clipped by the first. It carries three buttons, including **Focus Window**, which is a desktop-window concept with no meaning on a phone. The "1 keystroke dropped - reconnecting" status pill sits on top of the second toast, so a system status and a notification collide.
- Screenshot 2 (portrait, soft keyboard open): the visible terminal is only ~35% of the screen; the "Page keys" row is already clipped. A Chrome web push ("Session 'review:ee1b4be0:18dc7...'") is the hidden-session routine completion this project removes. Anything stacked at the bottom here lands on the keyboard accessory row.
- Top of both screens: an orange memory banner, a window tab strip, a pane header, and the `Terminal | Diff | VCS | Files | Logs | Info | Browser | Artifacts` tab row. There is no app Header (so no header bell) and no BottomNav on the session page (INFERRED from the screenshot; plan Spike 1.1 should confirm).

Design consequence (decided by the operator, ADR-009): on a phone session page, the only free regions are (a) the strip directly under the pane tab row (over the top of the terminal output, which is read-only content) and (b) nothing at the bottom. Bottom-anchored stacks always fight the input line, the page-keys row, the keyboard, and the connection pill. See DECISION D1.

## 1. Surface inventory

| # | Surface | Interactive? | Epic / Story |
|---|---------|--------------|--------------|
| 1 | Toast deck (stack, cap 3, "+N more" chip, deck header, pinned vs routine, pause) | yes | 3.3, 3.5, 3.7 |
| 2 | Toast card (anatomy, actions, close, swipe, hidden-session variant) | yes | 3.8, 5.3 |
| 3 | Deck bulk action: "Move all to tray" with undo | yes | 3.4, 3.9 |
| 4 | Tray entry points: desktop edge handle, header bell, mobile chip, hotkey | yes | 4.2 |
| 5 | Tray, desktop right-edge overlay (non-modal) | yes | 4.1, 4.2 |
| 6 | Tray, mobile portrait bottom sheet (peek / expanded) | yes | 4.2 |
| 7 | Tray, soft-keyboard top-anchored sheet variant (the capped bottom sheet of Surface 6 serves until it lands; sequenced last in Epic 4) | yes | 4.2 (Task 4.2e) |
| 8 | Tray, landscape panel variant (the capped bottom sheet serves in landscape until it lands; sequenced last in Epic 4) | yes | 4.2 (Task 4.2e) |
| 9 | Tray header and bulk actions (Mark activity read, overflow menu, Clear informational, Clear history) | yes | 4.4 |
| 10 | Tray list: needs-decision pin, per-session groups, rows, swipe-to-dismiss and button alternative | yes | 4.3 |
| 11 | Background activity section (failure and needs-human rows; a pending question row carries Reply) | yes | 5.4, 5.6 |
| 12 | Read-only hidden-session view (banner, input removed, deleted state, **Reply card for a pending question, Surface 12b**) | yes | 5.3, 5.6 |
| 13 | Quiet mode (in scope, sequenced last in Epic 4; operator decision O8) | yes | 4.5 (Tasks 4.5a, 4.5b) |
| 14 | Tray (and deck) empty / loading / error / offline states | yes | 4.3 (Tasks 4.3e-4.3g), 3.3 (Task 3.3e) |
| 15 | Push-notification click flow for a hidden session | flow | 5.3, 5.5 (5.5 sequenced last in Epic 5) |
| 16 | Delivery-gate suppression log line (non-interactive) | no | 2.8 |
| 17 | Feature flags `notification_tray_v2`, `hidden_session_gate` in Settings > Features (non-interactive here; existing panel) | no | 2.1, 3.10 |

Breakpoints and variants (from `trayVariant.ts`, plan 4.2b). **First wave:** `side-overlay` (>= 900px wide) and `bottom-sheet` (< 900px: portrait, landscape and keyboard open, always capped to `--viewport-height` with its bottom edge at `var(--keyboard-height)`). **Sequenced last in Epic 4 (Task 4.2e, in scope):** `top-sheet` (< 900px, keyboard open) and `landscape-panel` (< 900px wide and landscape or height < 500px).

Shared design tokens/vars used below: `--viewport-height` (visual viewport, already maintained by `ViewportProvider`), `--bottom-nav-height`, `--mobile-pane-tab-strip-height`, `env(safe-area-inset-*)`. One new var, `--mobile-stack-top-offset` (C3, plan Task 3.7c), is the y coordinate of the bottom edge of the session tab row, **measured** (`getBoundingClientRect().bottom` via `ResizeObserver`) and published by the session layout next to `--mobile-pane-tab-strip-height` (`web-app/src/components/pane/PaneSplitRenderer.tsx:442-449`). Measuring absorbs the variable-height, dismissible memory/fork-pressure banners; the stack top is `max(var(--mobile-stack-top-offset), env(safe-area-inset-top)) + 8px`.

Plan scoping of the variants (operator decision O8: all four are in scope): `side-overlay` and `bottom-sheet` land first. `top-sheet` (Surface 7) and `landscape-panel` (Surface 8) land last in Epic 4 (Task 4.2e, after Task 4.2g and Spike 1.2). Until they land, the bottom sheet is capped to `--viewport-height` with its bottom edge at `var(--keyboard-height)`, so it never extends under the keyboard (TK-2 holds), and in landscape (< 900px) it is the same capped bottom sheet. Criteria TK-1..TK-5, TL-3 and TL-4 are asserted against the capped bottom sheet and, once Task 4.2e lands, additionally against the new variants; TL-1 (landscape-panel width) applies when the variant lands; TL-2 (toast landscape cap) is a toast criterion owned by Story 3.7.

## 2. Design decisions (summary)

- **D1. (DECIDED, ADR-009) Mobile toasts dock at the top of the session page, never the bottom.** Portrait: at most 1 toast visible, directly under the session tab row, overlaying the first lines of terminal output, below the memory banner. Non-session pages (no terminal) may use the plan's bottom offset. Soft keyboard open: 1-line chip only. Reason: screenshots 1 and 2; INFERRED that the plan's "112px above bottom" still covers the input line on a short terminal. Plan Story 3.7 now matches (C1 resolved); the bottom offset applies only to desktop and mobile pages with no terminal. Fallback to a bottom dock above the page-keys row only if device check DV-1 shows the top card hides needed terminal lines.
- **D2. Desktop keeps the shipped bottom-right deck**, but narrower (360px), compact, max 3, and it shifts left of the tray when the tray is open.
- **D3. The tray is a notification shade, not a second inbox.** Same read/dismissed state as the Notifications page; the tray adds quick triage plus a "Review all" link to the page.
- **D4. Needs-attention rows are pinned at the top of the tray** and never swept by a bulk action (ADR-008). The group is named "Needs attention" (not "Needs a decision": the set includes errors and failures) and the header shows "N need attention" first. A WARNING whose producer stamped `auto_remediating=true` (for example the screenshot's "PR needs attention ... An automated fix attempt will run") is informational: not pinned, not counted, dismissible.
- **D5. No gesture is the only path.** Every swipe has a >= 44px button; every drag has a tap equivalent. Edge-drag-to-open is not offered on mobile (ADR-006).
- **D6. Focus never moves on toast arrival, and `.focus()` on the xterm textarea is restricted on touch.** Pointer-opened mobile sheets never move focus out of the xterm textarea unless the user taps an input inside the tray (so the soft keyboard does not collapse and trigger a resize vote). On `pointer: coarse`, closing a tray or sheet by tap or Back never moves focus into xterm when the keyboard was closed at open time (that would summon the keyboard and vote a resize); when the keyboard was open at open time the textarea keeps focus throughout and nothing is restored. Keyboard- or hotkey-initiated opens move focus into the tray and a keyboard-initiated close restores it to the stored element. The expanded mobile sheet is non-trapping on touch (Surface 6).
- **D7. The tray is overlay-only**: `position: fixed`, `transform` animation, no layout participation, no `backdrop-filter`, no `body` scroll lock. The expanded mobile sheet is the only modal variant (ADR-006 item 2).

## 3. Surface 1: Toast deck

**Purpose**: bounded, interrupting layer for things that need attention now. Everything shown here also lives in the tray, so expiry or dismissal never loses information.

**Rules**
- Cap 3 visible on desktop; portrait phone cap 1; landscape cap 1; keyboard open 0 (chip only).
- Order: pinned first (pinned = the server-sent `is_pending_decision` field on the live event: approval_needed, input_required, confirmation_needed, error, failure, and warning types **except** a WARNING whose producer stamped `auto_remediating=true`; the client keeps no type list), then newest. Pinned toasts never auto-close and never demote on their own, with one phone-only exception: after `PINNED_COLLAPSE_MS` (8s) without interaction a pinned card collapses to a one-line 44px chip in the same position (the notification stays pinned and unread in the tray; a tap expands it; TD-14).
- Routine types (task_complete, info, idle, and the like) do not toast; they go to the tray and increment the handle badge (existing `HISTORY_ONLY_TYPES`). A toast for the session currently in view is suppressed unless it is pinned.
- Overflow (including pinned beyond the cap) collapses into the "+N more" chip.
- Timers pause on hover, focus-within, while the tray is open, and while a swipe is in progress. Remaining time is preserved on resume.
- Dedupe by (session, type): the existing toast shows "x2" instead of stacking a twin.

**Desktop wireframe (>= 900px, tray closed)**
```
 +----------------------------------------------------------------------+
 | Header                                                   [bell 3]    |
 +------------------------------------------------------------+---------+
 |                                                            | [tray   |
 |   terminal / session content                               | handle  |
 |                                                            |  3  ]   |
 |                                                            |         |
 |                         +----------------------------------+--+      |
 |                         | Move all to tray (3)             (x)|      |
 |                         +-------------------------------------+      |
 |                         | ! Approval needed      WARNING   (x)|      |
 |                         |   sess-a1b2 - Bash: rm -rf /tmp/x   |      |
 |                         |   [Approve] [Deny] [View Session]   |      |
 |                         +-------------------------------------+      |
 |                         | x Task failed - review:abc  Background    |
 |                         |   [View output]                     |      |
 |                         +-------------------------------------+      |
 |                         | +4 more - open tray                 |      |
 +--------------------------------------------------------------------+
```

**Mobile portrait wireframe (session page, keyboard closed, D1).** The stack is `position: fixed` and takes no layout space: it **overlays** what is under it. On the Terminal tab that is the first terminal lines and the "Connected / Redraw / Hist" status row (the row is drawn below the stack only to show where it sits when no toast is up); on Diff, VCS, Files, Logs and Info it overlays the top of that tab's content, with the same one-card cap. DV-1 records whether the covered status row is needed while a toast is up; the pinned auto-collapse (TD-14) bounds how long a card covers it.
```
 +------------------------------+
 | ! Memory banner          (x) |   existing, above the stack
 | [Window 3]               [+] |
 | stapler-squad    ... [] [] x |
 | Terminal Diff VCS Files ...  |  <- --mobile-stack-top-offset = bottom of this row
 | +--------------------------+ |
 | | x Task failed   Background| |  one compact toast, <= 96px
 | | review:abc  just now  [x] | |
 | | [View output]  [Dismiss]  | |
 | +--------------------------+ |
 | [ +6 more  -  open tray ]    |  44px chip, full width of the stack
 | Connected   [Redraw] [Hist]  |
 | terminal output ...          |
 | > input line                 |  never covered
 | [Esc][/][-][Home][^][End]    |  never covered
 |   (connection status pill)   |  never covered
 +------------------------------+
```

**Mobile, soft keyboard open**
```
 +------------------------------+
 | Terminal Diff VCS ...        |
 | [ 3 notifications - 1 needs you ]  <- 1-line chip, 44px, tap opens the capped bottom sheet (Peek); the top-anchored sheet (Surface 7) replaces it when Task 4.2e lands
 | terminal output              |
 | > input line                 |
 | [Esc][/][-][Home][^][End]    |
 | [   soft keyboard          ] |
 +------------------------------+
```

**Desktop overlap rule (TD-15).** The bottom-right deck (360px) can sit over the terminal's last rows, where Claude Code prints its input line, and over the xterm scrollbar. The deck's right offset is the scrollbar width plus 16px, and when the xterm cursor cell would be covered the deck switches to a top-right anchor (same width, below the header) until the cursor moves away; if the cursor rectangle cannot be read, the top-right anchor is used (plan Task 3.7f).

**Interaction flow**
1. Event arrives. Entry: server push to the client. System: partition into visible / overflow; announce through the `Announcer` (one polite message such as "1 new notification" or "5 new notifications" for a burst; a lone pinned toast is announced assertively by title, a pinned burst once as "5 notifications need attention, newest: <title>"); no focus change.
2. User taps a toast action (View Session / View output / Approve / Deny). System: runs the action; on success the toast leaves the deck, history row is marked handled per the existing rule.
3. User taps the "+N more" chip. System: opens the tray (variant by breakpoint), pausing all toast timers.
4. User does nothing. System: non-pinned toasts auto-close after `toastAutoCloseMs(type)`; their tray rows remain. Pinned toasts stay.
5. Exit: dismiss (x on an informational toast), move to tray (tray control or swipe on a pinned one), auto-close, "Move all to tray", action completion, or open tray.

**Error and edge states**

| Case | Behavior |
|------|----------|
| Action fails (approve RPC error, network) | Toast stays; inline red line "Could not approve - Retry" with a Retry button (existing `NotificationItem` pattern); toast becomes pinned until resolved. |
| WebSocket disconnected (`selectConnectionState` not `connected`) | No new toasts can arrive; the deck shows no banner of its own and nothing misleading. Existing toasts stay; Approve and Deny are disabled with the visible reason "Offline"; the client-only controls (close or Move to tray, Move all to tray) stay enabled. The existing `ConnectionIndicator` and the tray offline banner (Surface 14) carry the status. |
| Burst of 50 events | Cap holds; chip count updates in place ("+47 more"); one announcement "50 new notifications" (one assertive "50 notifications need attention, newest: <title>" if all are pinned). |
| Orientation or keyboard state change while toasts present | Layout re-derives from variant; no toast is lost (overflow count adjusts); no focus change. |
| Toast would overlap connection status pill | Stack never occupies the bottom region on session pages (D1); on desktop the pill region is outside the deck footprint. |
| Tray flag off (`notification_tray_v2` false) | Legacy uncapped list with `clearAll()` (now filtering pinned) renders; none of surfaces 1-3 caps apply. |

**UX acceptance criteria (Surface 1)**
- **TD-1** With 10 simultaneous pinned-or-informational toasts on a 1280x800 viewport, exactly 3 toasts are visible and the chip reads "+7 more" with `aria-label="7 more notifications, open tray"`.
- **TD-2** On a 390x844 portrait session page, at most 1 toast is visible, its bounding box top is >= the session tab row bottom and below the memory banner, and its bounding box does not intersect the terminal input row, the page-keys row or the connection status pill (Playwright bounding-box assertions).
- **TD-3** With `isVirtualKeyboardOpen` true and 3 toasts present, no toast card renders; one chip of >= 44px height with text "3 notifications" (plus "- N needs you" when pinned exist) is shown at the top, and the soft keyboard remains open (no blur event on the xterm textarea). *Automated at V4 through the keyboard CSS var; the real soft-keyboard leg rests on device checks DV-1..DV-6 (DV-3), which Playwright cannot run.*
- **TD-4** In landscape (844x390), at most 1 toast is visible and the stack max-height is <= 40% of `--viewport-height`.
- **TD-5** A pinned toast (a live event with `is_pending_decision` true: approval_needed, input_required, confirmation_needed, error, failure, or a warning that is not auto-remediating) still exists after 10 minutes of fake time on desktop; an informational toast, including an auto-remediating WARNING, is removed after its configured timeout.
- **TD-6** Hovering, focusing within, or opening the tray pauses the timer; leaving resumes with the remaining time (not a reset).
- **TD-7** Routine completions (task_complete, info) never create a toast; they increment the handle badge. A non-pinned toast for the session currently in view is not shown.
- **TD-8** Exactly one `Announcer` owns the notification system's live regions: one always-mounted `role="status"` region and one always-mounted `role="alert"` region, and no toast, deck, tray, banner, receipt or kept-line element renders its own `role="status"` or `role="alert"`; no element has both a live role and `aria-live="polite"`. Coalescing wins inside a 500ms window per channel: 5 toasts within 500ms announce once ("5 new notifications"); one pinned toast alone is announced assertively by title; 5 pinned toasts within 500ms announce one assertive summary, not 5 interruptions. (Pre-existing unrelated live regions such as `ConnectionIndicator` and `InputDropBadge` are out of scope.)
- **TD-9** Arrival of a toast never changes `document.activeElement`.
- **TD-10** Opening and closing the deck states in Surface 1 never changes xterm `cols x rows` and never remounts the terminal root node.
- **TD-11** With `prefers-reduced-motion: reduce`, enter/exit use opacity only or no animation.
- **TD-12** Desktop: when the tray is open the deck's right edge is >= 16px left of the tray's left edge (no overlap).
- **TD-13** `--mobile-stack-top-offset` equals the session tab row's bottom edge within 1px with the memory banner shown, dismissed and wrapped to two lines, and the stack top is never above `env(safe-area-inset-top)`.
- **TD-14** On a 390x844 session page a pinned toast with no interaction collapses to a one-line chip (>= 44px, "1 needs you - <title>") after `PINNED_COLLAPSE_MS` (8s, fake timers); the notification stays pinned and unread in the tray; tapping the chip expands the card and it is not re-collapsed until the user collapses or acts; touch, hover and focus pause the timer; the expanded compact card is at most 96px tall; desktop cards never auto-collapse.
- **TD-15** At 1280x800 with the xterm cursor on the last row inside the deck's footprint, the bottom-right deck's bounding box intersects neither the cursor cell nor the xterm scrollbar (the deck switches to the top-right anchor, or uses the scrollbar-clear right offset), at V1 with Playwright bounding boxes.

## 4. Surface 2: Toast card

**Anatomy** (all variants)
```
 +------------------------------------------------+
 | [icon]  Title                  [BADGE]   [ x ] |   x >= 44x44, aria-label "Dismiss notification"
 |         subtitle: session - time               |
 | body text, max 3 lines, ellipsis               |
 | [Primary]  [Secondary]             [... more]  |   buttons >= 44px high
 +------------------------------------------------+
```
- Badge text always accompanies color ("WARNING", "FAILED", "NEEDS YOU", "Background"). Icon + text, not color alone.
- Hidden-session cards carry a "Background" text chip beside the badge. The primary action reads **"View output"** (not "View Session") to set the read-only expectation (DECISION D8). It carries `data-testid="notification-view-output"` (visible sessions: `notification-view-session`); e2e clicks by testid, never by label (C6 resolved in plan Task 5.3f).
- **Focus Window** is a desktop-terminal action; on touch (`pointer: coarse`) it is hidden, on desktop it moves to the "..." overflow of the card when more than 2 actions exist (plan Task 3.8d, C13 resolved). Screenshot 1 shows three buttons crowding a phone card; max 2 visible actions on mobile.
- Mobile compact variant: title, one-line body, one primary action, close. Full text is in the tray.
- A hidden session's `INPUT_REQUIRED` toast ("Claude has a question") with a **replyable** pending question (header-attributed to this session, single-answer prompt) shows two actions: **Reply** (`data-testid="notification-reply"`) and **View output**; Reply opens the read-only view with the Reply card (Surface 12b). Without a replyable question it shows **View output** only. Visible sessions never show Reply (they answer in the terminal).

**Gestures (touch)**
- Horizontal swipe past ~40% of the card width (or fling) on an informational card dismisses it with an undo toast.
- The same swipe on a pinned card demotes it to the tray ("Moved to tray" announced); the history row stays unread.
- `touch-action: pan-y` on the card; vertical drift > 10px cancels; the card stops propagation so the terminal gesture machine never sees the touch.
- Non-gesture alternatives: the close control (an x that dismisses an informational toast; on a pinned toast a **tray icon plus the visible text "Tray"**, `aria-label="Move to tray"`, so a touch user can see it is not a dismissal and no tooltip is needed), keyboard `Delete`/`Backspace` when the card is focused.
- In-drag feedback (TC-11): dragging a card past 24px reveals a label behind it, the icon and text "Dismiss" for an informational toast and the tray icon and "Move to tray" for a pinned one; releasing before 40% of the card width springs back and does nothing.

**Flow**
1. Entry: toast arrives.
2. Tap primary: action runs. Exit: toast leaves.
3. Swipe or x: Exit with undo (informational) or demotion (pinned).
4. Error: action RPC fails: inline "Could not ... Retry" and the card pins.
5. Error: "View output" target is deleted: lands on Surface 12 deleted state, not a 404.

**UX acceptance criteria (Surface 2)**
- **TC-1** Every toast has a visible close control with a hit area >= 44x44 CSS px, keyboard reachable in DOM order, with a text label per type: an informational toast shows an x (`aria-label` "Dismiss notification"); a pinned toast shows a tray icon and the visible text "Tray" (`aria-label` "Move to tray").
- **TC-2** A horizontal swipe of 90px in <= 200ms with < 10px vertical drift dismisses an informational toast once; a swipe with 60px vertical drift does nothing and the page/list scrolls.
- **TC-3** The same swipe on a pinned toast removes it from the deck while its tray row remains unread.
- **TC-4** A document-level `touchmove` spy is not called for a swipe that starts on a toast.
- **TC-5** On 390px width a toast shows at most 2 action buttons, none of which is "Focus Window".
- **TC-6** A hidden-session toast shows a "Background" text chip and a "View output" action; tapping it opens the read-only view (Surface 12) in <= 1 tap.
- **TC-7** Text contrast >= 4.5:1 and non-text (icon, border, focus ring) >= 3:1 in both light and dark themes (Axe on the card's own surfaces). Axe cannot compute contrast for a card composited over the xterm canvas, so the composite case is the manual check MC-1 (plan Task 3.7g): toast, chip and handle over bright and dark xterm themes with coloured ANSI output behind.
- **TC-8** A failed action shows the exact line "Could not <verb> - Retry" with a working Retry button, and the toast stays.
- **TC-9** The card is never swipeable without a visible button path (assert by hiding gestures: all actions reachable with keyboard only).
- **TC-10** A hidden-session `INPUT_REQUIRED` toast with a pending question shows exactly two actions, "Reply" and "View output" (no "Focus Window"), each >= 44px; a visible-session toast shows no Reply.
- **TC-11** During a swipe an informational toast reveals the text "Dismiss" with an icon and a pinned toast reveals "Move to tray" with the tray icon (reveal text >= 4.5:1); a release under 40% springs back with no action.

## 5. Surface 3: Deck bulk action

**Header rules** (DECISION D9, revised in triad iteration 1; C2 resolved by plan Story 3.4, Tasks 3.4a-3.4e)
- The deck header appears when the deck contains at least 2 toasts total (visible + overflow). It holds **one** control, **"Move all to tray (N)"**, whatever the deck contains. (The earlier header slot flipped between "Dismiss all" and "Move all to tray (N)" depending on deck contents, which is a mode error, and "Dismiss all" was almost a dead path: routine types never toast and nearly every toast is pinned, so the dismissable set was usually empty.)
- It only **moves**: toasts leave the deck, every history row stays unread and visible in the tray; nothing is dismissed, deleted or marked read; no RPC is sent.
- Dismissal is per toast: the x on an informational toast, or a swipe. A pinned toast shows a tray control instead (Surface 2), so it can never be dismissed from the deck. The only bulk deletions live in the tray (Surface 9).
- After the action: an undo control "Moved N to tray - Undo", for the `UndoWindow` (default 8s, per-device 5/8/15/30s, paused while hovered or focused), Undo >= 44px.
- **Undo placement.** On a phone (deck cap 1, or 0 with the keyboard open) the undo control **replaces the chip row in place**: same docked position, same >= 44px height, visible and operable with the keyboard open. On desktop it occupies the deck's header slot. An undo that follows a tray action is a sticky bar inside the tray (Surface 9).
- Cross-tab: the other tab removes exactly the same ids (`NOTIFICATIONS_BULK_DISMISSED` with `kind:"moved"`).

```
 desktop deck header                              mobile (1 visible toast + chip row)
 +--------------------------------------+         +-----------------------------+
 | Notifications (3)  [Move all to tray (3)]       | [x Task failed ...]         |
 +--------------------------------------+         | [+6 more]  [Move all (7)]   |
                                                   +-----------------------------+
 mobile, after Move all: the undo control replaces the chip row (also with the keyboard open)
                                                   | [ Moved 7 to tray - Undo ]  |
```
On mobile "Move all to tray" is a secondary button on the chip row (>= 44px), because the single visible toast has no room for a header.

**Flow**: Entry: tap "Move all to tray (N)". System: all toasts leave the deck, undo control shown in place, live region announces "7 moved to tray" through the `Announcer`. Exit: the UndoWindow ends (state final), Undo tapped (toasts restored in their previous order), or the tray is opened. Errors: none are server-side (the toast layer is client state; history is untouched), so the only failure is the undo expiring, stated in the undo text and announced.

**UX acceptance criteria (Surface 3)**
- **TB-1** Given toasts `[approval_needed, error, info, info]`, "Move all to tray (4)" removes all 4 toasts, all 4 history rows remain unread in the tray, and 0 `MarkNotificationRead`/`ClearNotificationHistory` calls are made.
- **TB-2** The control's label is "Move all to tray (N)" for every deck content (informational only, pinned only, or mixed); there is no "Dismiss all" control on the deck.
- **TB-3** The header is absent when the deck holds 0 or 1 toast; the button never appears as a no-op.
- **TB-4** Undo restores exactly the moved toasts in their previous order within the `UndoWindow` (default 8s); hovering or focusing the undo control pauses the countdown; after the window ends no Undo control exists.
- **TB-5** `clearAll()` called on a flag-off deck cannot remove a pinned toast.
- **TB-6** The move-all and undo controls have hit areas >= 44px and are reachable by keyboard (Tab) and by screen reader with the label "Move all notifications to tray".
- **TB-7** The other open tab drops exactly the broadcast ids; replaying the message changes nothing; a toast that arrived after the click is unaffected.
- **TB-8** From a deck of 10 notifications, a user reaches an empty deck in <= 1 tap.
- **TB-9** "Move all to tray (N)" sends 0 `MarkNotificationRead`/`ClearNotificationHistory` calls, leaves all N rows unread with their `isPendingDecision` values unchanged, and its undo restores the same N toasts in order.
- **TB-10** On a 390x844 phone (deck cap 1) and with the keyboard open (cap 0) the undo control replaces the chip row in place at the docked position, is >= 44px, and is visible and operable at V2 and V4.

## 6. Surface 4: Tray entry points

| Entry | Where | Behavior |
|-------|-------|----------|
| Desktop edge handle | Right edge, ~35% from top, 44px wide x 88px tall, text rotated or bell icon + count | `button`, `aria-expanded`, `aria-controls="notification-tray"`, label "Notifications, 3 unread, 1 needs attention". Count capped `99+`; a second line shows an attention dot + number when pinned > 0. |
| Header bell | Existing | Same toggle semantics; `aria-expanded` added. |
| Mobile chip | Floating, >= 44x44, bottom-right, 16px above `--bottom-nav-height` or above safe-area when no BottomNav; on a session page with the page-keys row it sits above that row only when the keyboard is closed. **Exactly one tray affordance is visible on mobile at any time, never zero** (plan Task 4.2i): while the deck has overflow (N > 0) the top-dock "+N more" chip is the entry and this floating chip is hidden; with the keyboard open and notifications present the 1-line top chip is the entry; otherwise this chip shows, **including with exactly 1 toast and 0 overflow** (the top dock then holds only the toast), so a phone is never left without a tray entry. Accessible names differ ("Notifications, 7 unread" vs "6 more notifications, open tray") | Tap opens the bottom sheet. With the keyboard open the chip is replaced by the top-docked chip (Surface 1). |
| "+N more" chip | Toast deck | Opens the tray. |
| Hotkey (in scope, sequenced last in Epic 4, plan Task 4.2f) | Chord **not decided**: `Alt+N` is a candidate only and is NOT verified free (macOS `Option+N` types a dead-key character; Linux/Windows terminals send `Alt+N` as `ESC n`, which readline/emacs treat as word-motion keys). A spike picks a chord, or the hotkey is dropped. Capture phase, IME guard | Toggles. Never delivers a byte to xterm. Listed in the shortcuts dialog. Until the hotkey lands (or the spike finds no safe chord) the entry points are the handle, bell, chip and "+N more". C4 resolved: the chord stays unverified until the spike runs. |
| BottomNav "More" sheet | Mobile | Row "Notifications (N)" as a secondary path. |

```
 desktop handle                     mobile chip
        +----+                        (   [bell] 7  )   <- 44x44 min, 99+ cap
        | [] |  <- right edge         with a small solid dot when a decision is pending
        | 7  |
        +----+
```

**Accepted trade-off, decided (operator decision O6: fixed desktop handle)** (INFERRED, plan Task 4.2h is the eyeball check; if it shows lost terminal columns the plan stops and escalates to the operator): the desktop handle overlays up to 44px at the right edge of the terminal in a ~88px band. It is overlay-only, so no reflow, but it can hide those columns. The handle is fixed at 35% from the top in core; no draggable-handle or Settings-hide work is planned. If the check fails, the operator chooses between a draggable handle and "bell only" (the latter would amend requirements.md's "persistent handle").

**Flow**: Entry: tap/click/hotkey. Exit: the tray closes (Surfaces 5-8). Errors: if the tray fails to render (render error), an error boundary shows a 44px "Notifications unavailable - Open Notifications page" link and the handle stays usable; hotkey conflict (event already `defaultPrevented` by xterm composition) does nothing and the user can use the handle.

**UX acceptance criteria (Surface 4)**
- **TH-1** At 1280x800 with 120 unread, the handle is visible, shows "99+", has `aria-expanded="false"` and `aria-controls="notification-tray"`, and its accessible name includes unread and decision counts.
- **TH-2** At 390x844 on a session page without a header bell, a chip with hit area >= 44x44 is visible and reachable, and does not cover the page-keys row or input line (bounding-box assertion with keyboard closed).
- **TH-3** (lands with the hotkey, Task 4.2f; if the chord spike finds no safe chord this records "no safe chord") With xterm focused, pressing the hotkey toggles the tray and the terminal receives 0 input bytes (echo/drop probe).
- **TH-4** The handle and chip are reachable by Tab, activate with Enter and Space, and show a visible focus ring with >= 3:1 contrast.
- **TH-5** The unread count never exceeds "99+" visually; the screen-reader name carries the exact number.
- **TH-6** No edge-drag-to-open gesture exists on mobile (assert: a touchstart within 30px of the screen edge does not open the tray).
- **TH-7** On a 390x844 session page exactly one tray affordance is visible at any time (the top-dock "+N more" chip while the deck has overflow, the 1-line top chip with the keyboard open and notifications present, otherwise the floating chip), their accessible names differ, and all open the same tray.
- **TH-8** On a 390x844 session page with exactly 1 toast and 0 overflow, the floating chip is visible and operable and is the only tray entry (never zero entries); when overflow appears it hides and the top "+N more" chip takes over, with no frame where both or neither is visible.

## 7. Surface 5: Desktop tray (side overlay)

**Layout**: right-edge panel, 400px wide (`min(400px, 40vw)`), full height below the Header, `position: fixed`, `transform: translateX`, no backdrop, no dim, no focus trap, no `aria-modal`. Landmark: `<aside id="notification-tray" aria-label="Notifications">`. The terminal stays interactive underneath.

```
 +------------------------------------------+---------------------------------+
 | Header                          [bell 3] | Notifications       [Q][...][x] |
 +------------------------------------------+ 1 needs attention - 2 unread    |
 |                                          | [Notifications | Background (1)]|
 |   terminal (no resize, no remount)       | [Search...]  [All][Appr][Err].. |
 |                                          |---------------------------------|
 |                                          | NEEDS ATTENTION (1)             |
 |                                          | ! Approval - sess-a1b2   2m     |
 |                                          |   Bash: rm -rf /tmp/x           |
 |                                          |   [Approve] [Deny] [Open]       |
 |                                          |---------------------------------|
 |                                          | v sess-j1k2 (12)                |
 |                                          |   ok  task_complete       6m [x]|
 |                                          |   i   info                9m [x]|
 |                                          | > sess-m3n4 (3)                 |
 |                                          |---------------------------------|
 |                                          | Review all notifications ->     |
 +------------------------------------------+---------------------------------+
```
Header controls left to right: title, **Q** Quiet mode toggle (in scope, lands last in Epic 4; the wireframes show where it sits), "Mark activity read" icon button, overflow "..." (Clear informational, a divider, Clear history, a divider, Collapse all groups, Expand all groups), close (x). All >= 44x44.

**Flow**
1. Open (handle / bell / chip / hotkey / "+N more"). System: slides in via `transform` (reduced motion: instant); if the open was by keyboard/hotkey, focus moves to the tray heading; if by pointer, focus stays where it was (xterm textarea) and the panel is operable by pointer. `document.activeElement` is stored at open.
2. Interact (Surfaces 9, 10, 11, 13, 14).
3. Close via x, handle, bell, hotkey, `Esc` (when focus is inside the tray, or when the tray was keyboard-opened). System: slides out; if focus was inside the tray it returns to the stored element (xterm textarea, `preventScroll`); if focus was outside, it is left alone.
4. `Esc` pressed while focus is in xterm does not close the tray (the key belongs to the terminal); use the hotkey or x.

**Error / edge**: see Surface 14 for offline, loading and RPC failure. Terminal resize or dev-tools opening while open: the tray stays fixed; no reflow. Window narrower than 900px while open: variant switches to the mobile sheet without losing scroll position or search text.

**UX acceptance criteria (Surface 5)**
- **TY-1** Open then close the tray with the terminal at 80x24: `cols x rows` identical, terminal root DOM node identity unchanged, scrollback marker text present, `document.body.style.overflow` unchanged.
- **TY-2** Tray open via handle with xterm focused, then Esc from inside the tray: `document.activeElement` is the xterm textarea.
- **TY-3** The desktop tray has no `aria-modal`, no backdrop element, no `inert` on the rest of the page; the terminal accepts typed input while the tray is open.
- **TY-4** (the hotkey half lands with Task 4.2f; if the chord spike finds no safe chord it records "no safe chord") Hotkey while xterm is focused toggles the tray; `Esc` while xterm is focused does not close it.
- **TY-5** The tray uses only `transform` (and `opacity` for reduced motion) for animation; no `backdrop-filter`; z-index from the `zIndex` token (no 9998/9999 literals).
- **TY-6** All header controls have hit areas >= 44x44 and an accessible name; the overflow menu is operable with arrow keys, `Esc` closes it and returns focus to the "..." button.
- **TY-7** Tab order is: tray heading, header controls, segment tabs, search, list, "Review all" link; focus is never trapped; `Shift+Tab` from the first control leaves the tray.
- **TY-8** Single-key shortcuts (`j`, `k`, `x`) are active only while focus is inside the list (WCAG 2.1.4); they do nothing when xterm or any other element has focus.
- **TY-9** At 200% zoom and at 320 CSS px content width nothing is clipped or requires two-axis scrolling (WCAG 1.4.10).
- **TY-10** (manual, rests on plan Task 4.2h) On a real session at 1280x800, the fixed 44px handle band at 35% from the top does not stop the operator from grabbing the xterm scrollbar thumb and does not hide needed last columns; if it does, the plan stops and reports to the operator (decision O6) instead of choosing a different handle.

## 8. Surface 6: Mobile portrait bottom sheet

**Snap points**: **Peek** (~25% of `--viewport-height`, min 160px) and **Expanded** (85% of `--viewport-height`). Sized from `--viewport-height`, honors `env(safe-area-inset-bottom)`.

```
 PEEK                                          EXPANDED (modal behavior, background inert)
 +------------------------------+              +------------------------------+
 |  terminal visible above      |              | (status/tab area visible 15%)|
 |                              |              +------------------------------+
 +------------------------------+              |         ====  grabber        |
 |         ====  grabber        |              | Notifications  [Q][...][ x ] |
 | 1 needs attention - 4 unread |              | [Notifications | Background] |
 | ! Approval  sess-a1b2   2m   |              | NEEDS ATTENTION (1)          |
 | [Approve] [Deny]   [Expand v]|              |  ! Approval ...  [Approve].. |
 +------------------------------+              |  v sess-j1k2 (12)            |
 | [bell 7]  (chip hidden)      |              |  ...                         |
 +------------------------------+              | Review all ->                |
                                               +------------------------------+
```
- Open: tap the chip or "+N more": opens at **Peek** (non-modal; terminal visible and interactive above the sheet; peek covers only the lower quarter, which on a session page includes the input line (accepted: the user explicitly opened it), but no terminal resize occurs).
- Expand: tap "Expand" (>= 44px) or drag the **grabber** only. The list content never drags the sheet (so list scroll and row swipes are unambiguous).
- Collapse/close: tap x, tap the chip/handle, drag the grabber down, hardware Back, or `Esc` with a hardware keyboard.
- Expanded, by pointer type (triad UX B2). On `pointer: coarse` the expanded sheet is **non-trapping**: no focus trap, no `inert`, no `aria-modal`, no focus move on open or close; closing by tap or Back never moves focus into xterm when the keyboard was closed at open time (that would summon the keyboard and vote a resize), and when the keyboard was open at open time the textarea keeps focus through Expand and close (no blur, keyboard stays up, no resize). On `pointer: fine` (a narrow desktop window) the expanded sheet is modal with a focus trap, `inert` background and a defined **trap exit**: Esc, the close button, a scrim click or Back each close it and focus returns to the opener (the xterm textarea only when it was focused at open, else the chip).
- Android Back closes the sheet before navigating (history entry pushed on open, popped on close, no remount of the page).

**Error / edge**: sheet open while a hidden-session toast arrives: no toast is shown (timers paused, toasts demoted to rows); the live region announces "1 new notification". Rotation: the capped bottom sheet keeps the same state until the `landscape-panel` (Surface 8, Task 4.2e) lands. Keyboard opens (e.g. user taps terminal): the bottom sheet is re-capped so its bottom edge is at `var(--keyboard-height)` and the keyboard never covers it; the top-anchored variant (Surface 7, Task 4.2e) replaces it when it lands.

**UX acceptance criteria (Surface 6)**
- **TS-1** The peek sheet height is within 22-28% of `--viewport-height`; expanded is 84-86%; neither exceeds `--viewport-height` nor the safe-area-adjusted bottom.
- **TS-2** Open and close at 390x844: terminal `cols x rows` unchanged; terminal node identity unchanged; no `resize` message sent for the terminal during the open/close (resize-vote spy). *Automated with the keyboard CSS var; the real soft-keyboard and `visualViewport` ordering leg rests on device checks DV-1..DV-6 (DV-3).*
- **TS-3** Dragging the list content does not move the sheet; dragging the grabber 80px down from expanded goes to peek; tapping "Expand" reaches expanded without any drag.
- **TS-4** Expanded sheet on `pointer: fine`: focus is trapped inside, background has `inert` (or `aria-hidden`), and Esc, the close button, a scrim click and Back each close it with focus returned to the opener. On `pointer: coarse` it is non-trapping: no `inert`, no `aria-modal`, no focus move on open or close, and a tap or Back close never calls `.focus()` on the xterm textarea when the keyboard was closed at open time.
- **TS-5** Peek sheet: background remains interactive; no focus trap; no `aria-modal`.
- **TS-6** Every tappable control in the sheet has a hit area >= 44x44 with >= 8px spacing between adjacent targets.
- **TS-7** Sheet content respects `env(safe-area-inset-bottom)`; the last row and "Review all" link are fully visible on a device with a 34px inset.
- **TS-8** Hardware Back with the sheet open closes the sheet and does not change the URL or session.
- **TS-9** At V4 (keyboard var set) with the sheet at Peek, tapping Expand leaves the xterm textarea focused (no `blur`), sends 0 terminal resize messages, and grows the sheet only to the cap (`--viewport-height`, bottom edge at `var(--keyboard-height)`); closing it afterwards leaves focus and the keyboard unchanged.

## 9. Surface 7 (sequenced last in Epic 4): Soft-keyboard top-anchored sheet (`top-sheet`)

**In scope, sequenced last in Epic 4 (plan Task 4.2e, operator decision O8; after Task 4.2g and Spike 1.2).** Until it lands the capped bottom sheet of Surface 6 (plan.md Story 4.2: bottom edge at `var(--keyboard-height)`) serves. Everything below describes the `top-sheet` variant. The criteria TK-1..TK-5 are asserted against the capped bottom sheet and, once Task 4.2e lands, additionally against the `top-sheet`. Owners in plan.md: TK-1, TK-3, TK-4 Story 4.2 (Task 4.2g); TK-2 Story 4.2 (sheet cap); TK-5 Story 4.2 (variant-switch AC, Task 4.2b).

Trigger: the user taps the top-docked "N notifications" chip (Surface 1) or the floating chip while the keyboard is open.

```
 +------------------------------+
 | [ Notifications  [Q][..][x]] |  top-anchored, height calc(var(--viewport-height) - header)
 | 1 needs attention            |
 | ! Approval  [Approve][Deny]  |
 | v sess-j1k2 (12)  ...        |
 | ...                          |
 | Review all ->                |
 +------------------------------+  <- bottom stays above the keyboard
 | [        keyboard         ]  |
```
Rules: sized from the visual viewport (`--viewport-height`), never extends under the keyboard; opening does not blur the xterm textarea (pointer-opened; D6) so the keyboard stays up and no resize vote fires; the search field is not auto-focused; if the user taps the search field the keyboard follows that field (accepted, they asked for it), and closing the tray restores focus to the xterm textarea.

**Error / edge**: keyboard closes while open: variant becomes the Peek bottom sheet at the same scroll position. Keyboard height changes (floating, split): layout re-derives from `--viewport-height`.

**UX acceptance criteria (Surface 7)**
- **TK-1** With the keyboard open, opening the tray leaves the xterm textarea focused (no `blur`), the keyboard remains visible, and no terminal resize is sent. *Automated at V4 through the keyboard CSS var; the real-device leg rests on DV-1..DV-6 (DV-3).*
- **TK-2** The tray bottom edge is >= the keyboard top edge (visual viewport bottom) at all times; the bottom action row is not covered.
- **TK-3** The tray never auto-focuses the search input on touch.
- **TK-4** A keyboard-initiated close (Esc, hotkey) returns focus to the xterm textarea if the tray was the only thing that took focus, otherwise to the previously focused element. On `pointer: coarse`, a tap or Back close never calls `.focus()` on the textarea when the keyboard was closed at open time, and when the keyboard was open at open time the textarea kept focus throughout, so nothing is restored and no resize is voted.
- **TK-5** Rotating or dismissing the keyboard while open switches variant without losing scroll position, search text, or collapse state.

## 10. Surface 8 (sequenced last in Epic 4): Landscape panel (`landscape-panel`)

**In scope, sequenced last in Epic 4 (plan Task 4.2e, operator decision O8).** Until it lands, landscape (< 900px) renders the capped bottom sheet of Surface 6. TL-1 (panel width and safe-area insets) applies when the variant lands; TL-2 (toast landscape cap, visible close) is owned by plan Story 3.7; TL-3 and TL-4 are asserted against the capped bottom sheet first and against the panel once it lands.

Viewport height may be ~300px. Panel on the right edge, width `min(360px, 50vw)`, full height of the visual viewport, honors `env(safe-area-inset-left/right)`. Header is a single 44px row (title + Q + "..." + x). Rows are single-line compact (title, session, time, one action). Non-modal. Entry: the chip at the right edge (> 44px from the bottom edge so it never overlaps the gesture bar), "+N more", or hardware hotkey. Exit: x, chip, Back, `Esc`.

**UX acceptance criteria (Surface 8)**
- **TL-1** At 844x390 the variant is `landscape-panel` with width <= 360px and <= 50vw, and its content clears left/right safe-area insets.
- **TL-2** At most 1 toast is visible in landscape; the panel never leaves a state where the only visible control is clipped (the close button is visible without scrolling).
- **TL-3** Opening and closing the panel does not reflow or resize the terminal (cols/rows constant).
- **TL-4** All controls >= 44px hit area; list scrolls internally.

## 11. Surface 9: Tray header and bulk actions

**Verb set** (ADR-008, shipped labels preserved)

| Action | Location | Scope | Safety |
|--------|----------|-------|--------|
| Mark activity read | Header icon button, label "Mark activity read" | Marks read all unread rows that are not unread decisions | Visual only, reversible; disabled when nothing to mark |
| Clear informational (N) | Overflow menu, above a divider, captioned "Undo available" | Dismisses informational rows (not pinned, not server-protected) | Confirm line + `UndoWindow` undo (default 8s), deferred RPC |
| Clear history | Overflow menu, below the divider, destructive styling (icon + the text "cannot be undone", not colour alone) | Same server guard as shipped: never deletes unread pending decisions | Inline confirm (not native `confirm()`), no undo (irreversible); lists what is kept |
| Collapse / Expand all groups | Overflow menu | UI only, per viewer | none |
| Quiet mode | Header toggle (Surface 13; lands last in Epic 4) | Per device | none |
| Review all notifications | Footer link | Navigates to Notifications page | leaves tray; terminal pool preserved by router |

```
 overflow menu                       Clear informational confirm (inline, in the tray)
 +--------------------------+        +------------------------------------------+
 | Clear informational (20) |        | Clear 20 informational notifications?    |
 |   Undo available         |        | 2 awaiting attention kept.               |
 |--------------------------|        | [Cancel]            [Clear 20]           |
 | (!) Clear history...     |        +------------------------------------------+
 |   cannot be undone       |        after Clear: sticky bar inside the tray
 |--------------------------|        "Cleared 20 - Undo" for the UndoWindow
 | Collapse all groups      |
 | Expand all groups        |
 +--------------------------+
```

**Flow: Clear informational**
1. Entry: overflow menu > "Clear informational (20)". If N = 0 the item is disabled with the hint "Nothing to clear".
2. System: shows the inline confirm with the "kept" line ("2 awaiting attention kept"; "3 unresolved failures kept" when applicable).
3. User confirms: rows are removed optimistically from view; a sticky undo bar inside the tray reads "Cleared 20 - Undo" for the `UndoWindow` (default 8s, per-device 5/8/15/30s, paused while the control is hovered or focused, announced through the `Announcer`); the RPC `ClearNotificationHistory{notification_ids}` is sent only after the window; broadcast `NOTIFICATIONS_BULK_DISMISSED` to other tabs at the same time as the RPC. If the page is hidden or closed during the window, the RPC is sent at once with `fetch` `keepalive` (Undo can no longer be offered; if the transport cannot honor `keepalive` the pending clear is cancelled and the rows return on the next load); no row is lost or half-deleted (TM-12).
4. Exit paths: Cancel (nothing changes), Undo (no RPC, rows reappear), window elapses (RPC sent).
5. Error: RPC fails: rows reappear, error toast "Could not clear notifications - Retry" (Retry re-issues); other tabs are re-synced from the server on the next history refresh. Offline: the action is disabled with the visible reason "Offline" (Surface 14); nothing is queued.

**Flow: Clear history** (irreversible)
1. Overflow > "Clear history..." opens inline confirm: "Clear N read notifications? This can't be undone. M awaiting decision kept." Buttons [Cancel] [Clear N] (focus starts on Cancel).
2. Confirm: RPC; success toast "Cleared N notifications". Error: dialog stays open with "Could not clear - Retry" and the list unchanged (single atomic store operation, no partial state).

**Flow: a `ClearNotificationHistory` response with `kept` ids** (validation G-5; the same rule on the tray and on the Notifications page): the rows the server refused to delete (unread pending decisions the client had already removed optimistically) are restored in their original positions and a line "N kept: still needs attention" stays until the next user action and is announced once through the `Announcer` (the line itself has no `role`); no further RPC is sent; an empty `kept` shows nothing. Both surfaces call one shared helper (plan Task 4.4f).

**Flow: Mark activity read**: tap; unread non-decision rows flip to read; the badge drops to the count of unread decisions; announcement "4 marked read, 1 still needs attention" (through the `Announcer`). Error: RPC fails: rows revert, inline line "Could not mark read - Retry".

**UX acceptance criteria (Surface 9)**
- **TM-1** Given 5 unread rows including 1 pending approval, "Mark activity read" marks the 4 others read, keeps the approval unread, and the badge shows 1.
- **TM-2** Given 20 informational and 2 pending-decision rows, Clear informational shows the exact confirm line "2 awaiting decision kept", and the RPC contains exactly the 20 ids; the server deletes 20, never 22.
- **TM-3** Undo within the `UndoWindow` (default 8s) issues no RPC and restores the rows in original order; hovering or focusing the undo control pauses the countdown.
- **TM-4** RPC failure restores the rows and shows the exact text "Could not clear notifications" with a Retry button.
- **TM-5** "Clear history" opens an inline confirm that names how many items are kept; the native `window.confirm` is not used; focus starts on Cancel and `Esc` cancels.
- **TM-6** The overflow menu, confirm and undo toast all have hit targets >= 44px and are reachable by keyboard; `role="menu"` items arrow-navigable.
- **TM-7** No bulk action can remove an unread approval_needed/input_required/confirmation_needed/error/failure/warning row (every row with `is_pending_decision`; server-side guard test plus UI test with seeded decisions).
- **TM-8** A user clears 50 informational rows in <= 3 taps (overflow, "Clear informational", confirm).
- **TM-9** While offline (`selectConnectionState` not `connected`), every server-mutating action (row Dismiss, swipe dismiss, Mark activity read, Clear informational, Clear history, Approve, Deny, Reply) is `aria-disabled` with the visible reason "Offline"; nothing is queued, replayed or shown as "pending sync", so destructive operations are never silently queued and "dismiss is permanent" stays true.
- **TM-10** When a `ClearNotificationHistory` response lists `kept` ids, the tray and the Notifications page both restore those rows and show "N kept: still needs attention", announced once through the `Announcer` (the line has no live role); an empty `kept` shows no line.
- **TM-11** The overflow menu separates the undoable from the irreversible: "Clear informational (N)" with the caption "Undo available" sits above a divider; "Clear history..." sits below it with destructive styling that is not colour alone (an icon and the text "cannot be undone") and is followed by a second divider before the view-only items.
- **TM-12** The `UndoWindow` setting (5/8/15/30s, default 8s) persists per device and falls back to the default when `localStorage` throws; when the page is hidden or closed during a pending Clear informational, the deferred RPC is sent immediately with `keepalive` (or, if the transport cannot honor it, cancelled with the rows returning on next load); in neither case is a row lost or half-deleted.

## 12. Surface 10: Tray list (groups, rows, swipe)

**Structure** (top to bottom): a pinned **NEEDS ATTENTION (N)** group (D4; the rows with `is_pending_decision`, never an auto-remediating WARNING) with actionable rows (Approve/Deny/Open) that never has a dismiss control while unread; then **per-session groups** from `groupNotifications()` sorted by latest activity; each group header shows session name, a "Background" chip when hidden, row count, and a collapse chevron (>= 44px). Groups beyond the first 3 start collapsed. The list is virtualized (< 60 DOM rows for 500 items), `role="list"`/`listitem`, `aria-setsize`/`aria-posinset`, roving tabindex.

```
 row (informational), swipe right to dismiss
 +----------------------------------------------------+
 | [icon] task_complete - sess-j1k2           6m  [x] |  x = Dismiss row, >= 44px
 | Finished: refactor notification service            |
 | [Open]                                             |
 +----------------------------------------------------+
   <-- swipe: row translates, red "Dismiss" reveal, release > 40% commits -->

 group header (contains a pending decision)
 | v sess-a1b2 (4)  Background        [Dismiss 3 informational] |
```

**Rules**
- Row dismiss (x or swipe) is permanent for that row, with an undo control for the `UndoWindow` (default 8s; a sticky bar inside the tray); offline the dismiss is disabled, not queued (Surface 14).
- A group dismiss is offered only as "Dismiss N informational" when the group contains an unread decision; the decision stays.
- Rows do not reorder under a finger: a duplicate increments the existing group's count in place; if the new item would change group order, a "N new - show" pill appears at the top of the list instead of moving rows.
- Opening a row ("Open" / row tap): navigates in-app (router, terminal pool preserved), marks the row read (decisions: marks read only after the decision is made, shipped rule), and closes the mobile sheet; the desktop tray stays open.
- Hidden-session rows open the read-only view (Surface 12).
- Swipe regions: the row surface only. Not the grabber, not the terminal, not the tab strip, not within 30px of the screen edge. `touch-action: pan-y`.
- Keyboard: arrows move, `Enter` opens, `x`/`Delete` dismisses (when permitted), `Space` expands a group.

**Flow and errors**: Entry: tray open. Row action succeeds: row updates/removes. Dismiss RPC fails: row reappears, "Could not dismiss - Retry" inline. Open fails (session deleted/unknown): Surface 12 deleted state. Exit: close the tray or navigate.

**UX acceptance criteria (Surface 10)**
- **TR-1** With 500 history rows, fewer than 60 row nodes are in the DOM, and opening the tray issues 0 `GetNotificationHistory` calls beyond the hydrated slice; scrolling to the end loads the next page.
- **TR-2** Unread rows with `is_pending_decision` render in the pinned top "Needs attention" group and expose no dismiss button and no swipe action; other rows (including an auto-remediating WARNING) expose both a >= 44px dismiss button and swipe.
- **TR-3** A swipe of 90px on an informational row dismisses it with an undo toast; 60px of vertical drift scrolls the list instead.
- **TR-4** Group dismiss on a group containing a decision shows "Dismiss N informational" and leaves the decision row in place.
- **TR-5** A duplicate arriving while the tray is open increments the group count without moving any row's bounding box (+-1px) of rows currently in the viewport.
- **TR-6** Rows have visible text for type and status, not color alone; the "Background" chip is text.
- **TR-7** The group collapse control has hit area >= 44px, `aria-expanded`, and collapse state is not shared across tabs or devices.
- **TR-8** Keyboard-only user can open a row, dismiss a row, and collapse a group without a pointer.
- **TR-9** The tray never calls `.focus()` on a row on arrival of a new notification.
- **TR-10** The tray header leads with "N need attention" (hidden at 0) and the first group is "NEEDS ATTENTION (N)"; the same N appears on the handle's dot; a pinned row appears in no other group (plan Task 4.3d, C9).
- **TR-11** An unread WARNING stamped `auto_remediating=true` (for example "PR needs attention ... An automated fix attempt will run") appears in its session group, is dismissible, auto-closes as a toast, and is excluded from "N need attention" and the handle's dot; an unstamped unread WARNING, error or failure is pinned.

## 13. Surface 11: Background activity section

**Placement**: a "Background" segment beside "Notifications" in the tray (not a nav page, not in the session list). Badge counts only failures and needs-human items (never routine completions).

```
 +------------------------------------------+
 | [Notifications] [Background (2)]         |
 |------------------------------------------|
 | NEEDS ATTENTION (2)                      |
 | x review - PR #912 fix           FAILED  |
 |   review:ee1b4be0  failed 4m ago         |
 |   [View output]                          |
 | ! triage - issue #355     NEEDS INPUT    |
 |   "Claude is waiting for your input"     |
 |   [View output]                          |
 |------------------------------------------|
 | 12 completed OK today                    |
 | 3 running                                |
 | Updated 30s ago   [Refresh]              |
 +------------------------------------------+
```
- Rows: role label (review / triage / diagnose / headless), linked item or PR when known, status text, relative time. Failures first, then needs-human, newest first within each.
- Routine completions are one summary line (no rows), which keeps the Hidden policy's quiet promise. A running count line is allowed.
- Row tap or "View output" opens Surface 12.
- Data: `ListSessions{hidden_only:true}`, polled every 30s only while the segment is visible; 0 polls while collapsed or the tray is closed. Manual "Refresh" button >= 44px.
- Join and lifecycle (C7 resolved in plan Story 5.4, Task 5.4e): rows are a filtered view over notification history (failure-class or needs-human, unread, session_id/session_name equal to a hidden session's id/title), enriched by `ListSessions{hidden_only}`. A row leaves when its record is read (opening it marks it read), dismissed or cleared. It does **not** leave because the session was deleted or archived: it stays as "Session no longer available" until read or dismissed. "N completed OK today" and "N running" come from the session list. Rows add nothing to the bell count (the records are already counted there).

**States**
| State | Content |
|-------|---------|
| Loading (first fetch) | 3 skeleton rows, `aria-busy="true"`, announce "Loading background activity" once |
| Empty, healthy | "Nothing needs attention in the background. 12 completed OK today." with a check icon |
| Empty, no background sessions | "No background sessions have run recently." |
| Error | "Could not load background activity." + [Retry]; last good data stays visible under a "Showing data from 2m ago" line |
| Offline | Same banner as Surface 14; no polling; last data shown with timestamp |
| Failure but session deleted | Row stays until its record is read or dismissed, showing "Session no longer available" and the captured message; open leads to Surface 12 deleted state |

**UX acceptance criteria (Surface 11)**
- **BA-1** The segment badge equals the number of failure + needs-human rows; routine completions never change the bell count or the badge.
- **BA-2** With the segment collapsed or the tray closed, 0 `ListSessions` polls are made; expanded, 1 immediately then every 30s (fake timers).
- **BA-3** Routine completions render as exactly one summary line, never as rows.
- **BA-4** Tapping a row or "View output" opens the read-only view in <= 1 tap and the tray state (scroll, segment) is preserved on return.
- **BA-5** Error state shows "Could not load background activity." and a working Retry button; stale data remains with its age.
- **BA-6** The empty-healthy state is presented as a success state (text + icon), not an error, only when the last fetch succeeded; it is never shown while loading or offline.
- **BA-7** Controls >= 44px; status is conveyed by text label plus icon, contrast >= 4.5:1.
- **BA-8** Main `ListSessions` calls never set `include_hidden`; the main session list never contains hidden sessions.
- **BA-9** A failure row disappears when its record is read or dismissed, stays (as "Session no longer available") when its session is deleted while the record is unread, and a hidden session with no failure/needs-human record produces no row.
- **BA-10** A Background row for a pending `INPUT_REQUIRED` question shows a "Reply" action beside "View output"; a row whose question is no longer pending, and every other row, shows none.

## 14. Surface 12: Read-only hidden-session view

**Entry points**: toast "View output", tray row Open, Background row, push click, deep link `/?session=<uuid>&tab=terminal`. **Exit**: the normal navigation (back, tab strip, tray, BottomNav); no "unlock" path (ADR-005).

```
 desktop and mobile (same structure)
 +--------------------------------------------------+
 | stapler-squad > review:ee1b4be0     [...]   [ x ] |
 | +----------------------------------------------+ |
 | | (eye) Background session - read-only         | |  data-testid="readonly-banner"
 | | You can read output but not type.   [Back]   | |  sticky, not dismissible
 | +----------------------------------------------+ |
 | Terminal  Diff  VCS  Files  Logs  Info           |
 | terminal output (selectable, copyable, scrollable)|
 |                                                  |
 | [no input bar] [no soft-keyboard button] [no Page keys / send-keys toolbar]
 +--------------------------------------------------+
```
- Banner text exactly: "Background session - read-only". Secondary text: "You can read output but not type." when no Reply card is shown, and "You can read output. Reply to Claude's question below; nothing else can be typed." when the Reply card is present (RO-12), so the banner never contradicts the card. Persistent and sticky below the pane header; it is announced once on mount through the `Announcer` (polite) and the banner element itself carries no live role.
- Removed on read-only: input bar, soft-keyboard trigger, send-keys/page-keys toolbar, Redraw if it writes, any steer/nudge control. Kept: scroll, text selection, Copy all, Terminal history, Diff/VCS/Files/Logs/Info tabs (read RPCs), and the Reply card (Surface 12b) while a question is pending: the only control that writes, narrowly scoped and audited (ADR-010).
- Resize frames are not sent (server also drops them) so a viewer cannot change the pane size.
- If the session finishes while viewing: status chip updates to "Finished"; banner remains.

**Deleted / unavailable state**
```
 +--------------------------------------------------+
 | (info) Session no longer available               |
 | review:ee1b4be0 was removed before you opened it |
 | Last notification: "Task failed - tests failing" |  captured title and message (when available)
 | 4m ago                                           |
 | [Open notifications]   [Go to sessions]          |
 +--------------------------------------------------+
```
No 404 and no blank screen. The deep link carries `notification=<id>` (toast, tray row, Background row and the inline push payload; plan Task 5.3e, C5 resolved) so the card can look up the captured title and message. If no notification is available (cold link without the param, a status-change push that has no record, history pruned) the card shows only the first two lines plus the two buttons.

**Error / edge states**

| Case | Behavior |
|------|----------|
| `getSession` network error | Inline "Could not load session." + [Retry] + [Go to sessions]; the retry does not reload the page. |
| Stream disconnects | Existing "Disconnected - reconnecting" status chip; output resumes; banner stays. |
| User tries to type (hardware keyboard) | Nothing is sent; a transient hint "Read-only session" is announced (polite) at most once per 10s. |
| Server rejects a write (guard) | Never surfaced as a stack trace; toast "This session is read-only" if any control still attempted a write. |
| Cold load with an empty live list | The session still opens (fallback runs after the list settles empty). |

**UX acceptance criteria (Surface 12)**
- **RO-1** Opening a hidden session shows `[data-testid="readonly-banner"]` with the text "Background session - read-only" before any terminal output is interactive; xterm has `disableStdin` and the input bar and soft-keyboard trigger are not in the DOM.
- **RO-2** Typing into the terminal with a physical keyboard produces 0 bytes on the server stream; resize produces 0 resize calls (server-verified).
- **RO-3** A deleted or unknown session id from a notification shows "Session no longer available" with the notification's title and message and two buttons >= 44px; the page never shows a 404.
- **RO-4** A cold deep link `/?session=<hidden-uuid>&tab=terminal` with an empty list opens the read-only view, with the banner visible, within 2000ms of navigation start at V1 and within 4000ms at V2 under 4x CPU throttling on the isolated server (initial thresholds, recalibrated once from the first e2e run); "View output" from the toast, tray row, and Background row all land on the same view.
- **RO-5** The banner is not dismissible, is not hidden by scrolling, and has contrast >= 4.5:1.
- **RO-6** Text selection and Copy all still work; Diff/VCS/Files/Logs/Info tabs load.
- **RO-7** A getSession failure shows "Could not load session." with Retry; Retry succeeds without a page reload.
- **RO-8** The banner and deleted card are announced once on mount through the `Announcer` (neither element carries its own `role="status"`).
- **RO-10** Opening `/?session=<deleted>&tab=terminal&notification=<id>` with the record in history shows its title and message; the same link without `notification` or with a pruned id shows the card without them; neither shows a 404.
- **RO-9** A user reaches the read-only output from a failure toast in <= 1 tap and from a push in <= 2 taps (tap push, app opens).
- **RO-12** When the Reply card is present the banner's secondary text reads "You can read output. Reply to Claude's question below; nothing else can be typed."; without it, "You can read output but not type."; the banner's primary text is unchanged.
- **RO-11** While a question is pending the banner stays and the terminal keeps `disableStdin` and no input bar; the Reply card is the only writable control in the view, and it disappears when the question is no longer pending.

### Surface 12b: Reply card (audited reply to a pending question, operator decision O2)

**Purpose**: a hidden session that asks "Claude has a question" is a needs-human event; the read-only view must not be a dead end. The Reply card is the one writable control in an otherwise read-only view. It answers one outstanding question, once, and every reply is logged (ADR-010, plan Story 5.6). It never appears for a visible session (they answer in the terminal) and never when no question is pending.

**Entry points**: "Reply" on the hidden `INPUT_REQUIRED` toast, tray row and Background row (`data-testid="notification-reply"`, beside "View output", never more than 2 actions on touch); each opens `/?session=<id>&tab=terminal&notification=<id>&reply=1`. Desktop focuses the composer; mobile scrolls the card into view and does not focus it (no surprise soft keyboard). Opening the view by "View output" shows the same card. **Question text (review-repair iteration 3, ADV-N15):** the card shows the question text that the pane capture parsed from the dialog when it differs from the hook's text, so the operator reads what is on screen and a forged hook cannot dress another dialog as a question.

```
 +--------------------------------------------------+
 | (eye) Background session - read-only             |  banner, unchanged, non-dismissible
 +--------------------------------------------------+
 | (?) Claude has a question                        |  data-testid="reply-card", role="group"
 | "Which approach should I take? 1) A  2) B"       |  captured prompt, first 280 chars + "Show full prompt"
 | [1) A] [2) B]                                    |  numbered quick-picks ONLY if Spike 1.3g allows; fills the field, never auto-sends
 | [ Your reply (one line, up to 500 bytes) ] [Send] |  data-testid="reply-input" / "reply-send", >= 44px
 | 12 / 500 bytes                                   |  visible byte counter (UTF-8 bytes via TextEncoder)
 | Sent to the background session's terminal once.  |
 | This reply is logged.                            |
 +--------------------------------------------------+
 | Terminal  Diff  VCS  Files  Logs  Info           |
 | terminal output (read-only)                      |
```

**States**

| State | Content | Exit |
|-------|---------|------|
| Composing | Card as drawn; Send disabled while the field is empty | Type, Send |
| Sending | Send disabled, `aria-busy="true"`, field text preserved | Success or error |
| Sent | Receipt "Reply sent 10:42 - waiting for the session to continue" (announced politely through the `Announcer`; the element has no live role); composer removed; focus moves to the receipt; the notification is marked read | Receipt stays until the question's state changes or the view closes |
| No longer pending (`NO_PENDING`) | "This question is no longer waiting. You may have answered it elsewhere." (not an error toast); composer removed; no write happened | Card disappears on next state refresh |
| Rate limited | "Please wait N seconds" with the text preserved; Send re-enables when the wait ends | Retry |
| Invalid input | "Replies are one line, up to 500 bytes". The unit is **UTF-8 bytes** on both client and server (plan Story 5.6, ADR-010): the client measures with `TextEncoder`, shows the live counter "N / 500 bytes", disables Send above 500 and strips newlines and control characters on paste. HTML `maxlength` counts UTF-16 code units, so it is at most a coarse extra guard, never the check. The server also rejects C1 controls (U+0080-U+009F), U+2028/U+2029 and every Unicode format character except U+200C and U+200D (so invisible tag characters, the soft hyphen and bidi controls are refused while emoji joiners and Persian text are accepted); the client sanitizer strips the same set on paste | Edit |
| Not sent, or network or server error with no outcome | "Could not send - Retry" (announced assertively through the `Announcer`; the element has no `role`); text preserved; Retry re-sends with the same `reply_id`. A `NOT_SENT` caused by a busy write lease (the session driver or a nudge was writing, or an earlier blocked write is still running) uses this same state and copy: there is no separate "in progress" state because the enum has no `IN_PROGRESS`. Server rule (review-repair iteration 1, replacing the Phase 4 G-3 rule that let any failed write be retried): only a failure **strictly before the first byte** (`NOT_SENT`) releases the claim, so Retry is safe there; an audit-log failure (`Internal`) wrote nothing; a lost response is answered from the server's in-flight or cached result | Retry |
| **Send indeterminate** (`SEND_INDETERMINATE`) | "Sent? It may have been typed but not submitted. Check the terminal output" with a **View output** action and **no Retry button**; the typed text stays visible read-only; announced assertively. The content or the Enter may have reached the terminal (a timed-out content write stops before the Enter, so the text can be typed but not submitted; the same card is shown when the pre-Enter pane check withheld the Enter because the dialog changed, in which case the text is typed and not submitted), and a second send could type the answer twice (or into the next dialog), so the claim is consumed and the result is cached under the `reply_id` | View output, or leave the view |
| Superseded (`SUPERSEDED`) | "Claude asked a newer question." with an "Open the latest question" action when a newer registered question exists; nothing was written. When it was superseded by a **permission dialog or another hook** (no newer question exists): "Claude's prompt changed. Check the terminal and answer there." with **View output** only (RP-16) | Open latest, or View output |
| Prompt changed (`STALE_PROMPT`) | "The question on screen changed. Check the terminal and answer there." No Retry; nothing was written | View output |
| Not showing yet (`NOT_WAITING`) | "The question isn't showing yet - try again in a moment" with Retry; nothing was written (also the state for a shell prompt or a session that is not waiting) | Retry |
| No question text | "Open the terminal output to read the question" above the composer (the hook payload had no prompt text but did have an option label, so the question can still be identified on screen); the composer still works when the question is a single prompt. With **neither** text nor an option label the question cannot be identified, so it is treated as not answerable here (next row) | Reply |
| Not answerable here (multi-question or unknown shape, a payload with nothing that identifies the question, or a question that could not be attributed to this session) | "Open the terminal to answer" with View output; **no composer** and no Reply action on the toast, tray row or Background row | View output |
| **Reply unavailable for this session** (notification metadata `reply_unavailable=no_proof` or `bad_proof`: the session's hook carries no valid sender proof, which is the case for every session that was already running when hook proof shipped, and for the senders that are not proofed) | "Reply unavailable for this session (started before reply support was enabled; restart the session)" with **View output** only; no composer, no Reply action on the toast, tray row or Background row, no Retry; announced once politely; nothing was written. The other causes (`no_token`, `boilerplate`, `shape`, `path_only`) keep the "Open the terminal to answer" copy (RP-17) | View output |
| Refused (`PermissionDenied`) | "This device is not allowed to reply" with no Retry (authentication is off and the connection is not local, or the Host or Origin was rejected) | Leave |
| Offline | Send disabled with the reason "Offline - reconnect to reply" | Reconnect |
| Session deleted or finished | Card replaced by the existing deleted/finished states (Surface 12) | Navigate |

**Mobile (390x844 and 844x390)**: the card sits directly below the banner, above the terminal; it is sized from `--viewport-height`, so with the keyboard open it stays above the keyboard; targets >= 44px; the composer uses `enterkeyhint="send"`; opening the view from a toast never auto-focuses the field. The read-only terminal sends no resize while the card is used.

**Accessibility**: `role="group"` labelled by the "Claude has a question" heading; the input has a visible label "Your reply to Claude's question"; errors are announced once assertively and the receipt once politely, both through the `Announcer` (no `role="alert"` or `role="status"` on the card); status is text plus icon, not color alone; reduced motion removes the card's enter animation; Enter sends, Escape clears nothing and never leaves the view.

**Why this is safe to show** (for the reviewer): the server accepts the reply only for the question that is on screen (a newer question or any other dialog-producing hook such as a permission request supersedes older ones, the live pane fingerprint, which contains the question text, is re-checked right before the first byte, and the session driver cannot write inside the reply), once, one line, rate-limited, after an audit line is written and flushed, with one content write and one Enter and no retry, and the Story 5.1d guard checks (typed capabilities, the RPC-descriptor classification and the pinned-caller table) keep any other UI path from gaining a write unnoticed, within the limits ADR-005 decision 3 states (ADR-005, ADR-010). It is a safety rail, not a security boundary. **Accepted limit (default applied in review-repair iteration 4; the operator may override)**: Reply works only for sessions started after hook proof ships, because Claude Code most likely snapshots its hooks at start (INFERRED); every session alive at upgrade shows the "Reply unavailable for this session" state until it is restarted, which is fail-safe and visible rather than silent.

**UX acceptance criteria (Surface 12b)**
- **RP-1** With a pending question the Reply card is directly below the banner; with none it is absent, and a visible session never shows it.
- **RP-2** The input is single-line, limited to 500 UTF-8 **bytes** with a visible "N / 500 bytes" counter (a 200-emoji reply shows 800 / 500 and Send is disabled), labelled, Enter sends, Send is >= 44px, and the helper text states that the reply is sent once and logged.
- **RP-3** Sending disables Send and sets `aria-busy`; the text is preserved; a double tap issues one reply (same `reply_id`).
- **RP-4** Success shows a receipt with the send time (announced once politely through the `Announcer`, no live role on the element), moves focus to it, marks the notification read, and offers no second reply for the same question.
- **RP-5** A reply to a question that is no longer pending (`NO_PENDING`) shows the exact text "This question is no longer waiting. You may have answered it elsewhere." and writes nothing.
- **RP-6** A rate-limited reply shows "Please wait N seconds", preserves the text and re-enables Send after the wait.
- **RP-7** A network failure with no response, an `Internal` (audit) error or a `NOT_SENT` outcome shows "Could not send - Retry" (announced once assertively through the `Announcer`) with the text preserved; Retry reuses the same `reply_id` and re-attempts the write (safe: the claim is released only before the first byte, or nothing was written); invalid input shows "Replies are one line, up to 500 bytes". A `SEND_INDETERMINATE` outcome never offers Retry (RP-13).
- **RP-8** The Reply action appears on the hidden `INPUT_REQUIRED` toast, tray row and Background row only while a question is pending, with at most 2 actions on touch and no "Focus Window".
- **RP-9** At 390x844 with the keyboard var set the card stays above the keyboard, all targets are >= 44px, no terminal resize is sent, and a deep link with `reply=1` does not auto-focus the field on mobile. *Automated with the keyboard CSS var; the real soft-keyboard leg rests on DV-1..DV-6 (DV-3).*
- **RP-10** Axe reports 0 WCAG 2.2 AA violations on the card in composing, sending, sent, indeterminate, superseded, no-longer-pending and error states, and each error is announced once.
- **RP-11** A captured prompt longer than 280 characters shows the first 280 and a "Show full prompt" disclosure (>= 44px) that reveals the rest, so a multi-option question can be read in full before it is answered.
- **RP-12** Numbered quick-pick buttons (>= 44px) appear only when Spike 1.3g recorded that the dialog is answered by option number and the options parse reliably from the captured prompt; tapping one fills the composer with the option number and does not auto-send (the same audited, rate-limited Send applies); otherwise the card is free text only.
- **RP-13** A `SEND_INDETERMINATE` outcome shows "Sent? It may have been typed but not submitted. Check the terminal output" with a View output action and **no Retry button** (no control re-sends that `reply_id`), keeps the typed text visible read-only, is announced once assertively, and a reload of the view shows the same state (the result is cached server-side under the `reply_id`).
- **RP-14** A `SUPERSEDED` outcome shows "Claude asked a newer question." with an action that opens the newest question's card; a `STALE_PROMPT` outcome shows "The question on screen changed. Check the terminal and answer there." with no Retry; a `NOT_WAITING` outcome shows "The question isn't showing yet - try again in a moment" with Retry; none of them writes anything, and each is announced once.
- **RP-15** With no question text but an option label in the notification the card says "Open the terminal output to read the question" and the composer still works for a single prompt; with a multi-question, unknown-shape or unattributed question, or a payload with neither text nor an option label, the card says "Open the terminal to answer" (or, for the no-identifier case, "Open the terminal output to read the question" with View output only), has no composer, and the toast, tray row and Background row show only "View output" (no Reply action).
- **RP-16** A `SUPERSEDED` outcome with no newer registered question (the older question was superseded by a permission dialog or another hook) shows "Claude's prompt changed. Check the terminal and answer there." with View output only, no "Open the latest question" action and no Retry, writes nothing and is announced once; with a newer registered question it shows the RP-14 copy and the Open-latest action.
- **RP-17** A question that is not replyable because the session's hook has no valid proof (`reply_unavailable=no_proof` or `bad_proof` in the notification metadata) shows "Reply unavailable for this session (started before reply support was enabled; restart the session)" with View output only: no composer, no Reply action on the toast, tray row or Background row, no Retry; it is announced once politely and nothing is written; the other causes keep the existing "Open the terminal to answer" copy.

## 15. Surface 13 (sequenced last in Epic 4): Quiet mode

**In scope, sequenced last in Epic 4 (plan Story 4.5, Tasks 4.5a and 4.5b; beyond the original requirements.md text, kept by operator decision O8).** TQ-1..TQ-5 are authored with Quiet mode. Pinned types (the server-sent `is_pending_decision`: warning (except an auto-remediating one), error, failure, approval_needed, input_required, confirmation_needed) always toast under Quiet mode, because the pinned set is server-pinned (TD-5).

- Toggle (Q icon button with `aria-pressed`, label "Quiet mode: on/off") in the tray header. When on: every non-pinned toast is demoted to the tray; pinned toasts still show; the handle shows a "quiet" crescent and the badge still counts.
- Persisted per device in `localStorage` (try/catch). If storage throws, Quiet mode renders off and the toggle still works for the session, with an inline note "Preference could not be saved".
- Scope: client toasts only. Web push and OS notifications are separate settings and are not changed; the tray hint says so ("Quiet mode hides toasts on this device. Push notifications are unchanged.").
- Flow: Entry: tap Q. Exit: tap Q again. Error: storage failure as above. Visible state: a persistent "Quiet" label on the handle and a line in the tray header so the user is never unaware that toasts are off (`feedback_document_ai_decisions_in_edge_cases`: no silent behavior).

**UX acceptance criteria (Surface 13)**
- **TQ-1** With Quiet on, a non-pinned notification that would normally toast (a `custom`, type 100) creates no toast and increments the tray count; every pinned type (a warning that is not auto-remediating, error, failure, approval_needed, input_required, confirmation_needed) still creates a toast. (`info` and `task_complete` are history-only and never toast, so they cannot demonstrate demotion; WARNING is pinned, so the earlier "warning creates no toast" wording contradicted TD-5.)
- **TQ-2** Reload keeps the setting; when `localStorage` throws, the app renders with Quiet off, no crash, and the inline note is shown.
- **TQ-3** The toggle has `aria-pressed`, a hit area >= 44px, and state text that is not color-only.
- **TQ-4** When Quiet is on, the handle/chip shows a visible text/icon indicator and the tray header shows "Quiet mode on".
- **TQ-5** The hint text states that push notifications are unchanged.

## 16. Surface 14: Tray (and deck) empty / loading / error / offline

**One connectivity source (decided).** "Offline" means `selectConnectionState` (`web-app/src/lib/store/sessionsSlice`, the store behind the existing `ConnectionIndicator`) is `stale` or `disconnected`. The tray's offline banner reads that same store; it is not a second implementation, and it agrees with the indicator the operator already sees. A `GetNotificationHistory` failure while connected is a different state, **load error**: the two have different remedies (wait or reconnect versus Retry), so they are never merged into one flag. (The terminal pane's own "Connected / Redraw / Hist" status is the per-terminal stream, a separate concern; the notification surfaces do not read it.) Plan Tasks 3.3e, 4.3e, 4.3f, 4.3g own this surface; it is no longer owned "by reference".

**One offline policy (decided).** While offline, **every server-mutating action is disabled with the visible reason "Offline" and nothing is queued**: row Dismiss and swipe-dismiss, Mark activity read, Clear informational, Clear history, Approve, Deny and Reply. There is no "pending sync" state and no replay on reconnect. This resolves the earlier conflict between a queued offline dismiss and TM-9 ("never silently queue destructive operations") and "dismiss is permanent". Client-only controls (collapse, search, filters, the deck's close, Move to tray and Move all to tray) stay enabled. On reconnect the controls re-enable and the banner clears.

| State | Trigger | UI | Exit |
|-------|---------|----|------|
| Initial loading | History slice not yet hydrated | 4 skeleton rows, `aria-busy="true"`, not announced row by row; the list is not blocked by a full-history fetch | Data arrives |
| Empty, healthy | 0 rows, connected, last fetch OK | check icon + "All caught up. Nothing needs your attention." + "Review all notifications" link | New row arrives |
| Empty, filtered | Search/type filter yields 0 | "No matching notifications" + [Clear filters] (>= 44px) | Clear filters |
| Needs-attention empty, others present | 0 pinned rows, informational present | the pinned group shows a one-line success "Nothing needs attention" | n/a |
| Load error | History fetch failed while connected | "Could not load notifications." + [Retry]; cached rows (if any) remain under a "Showing cached - updated 3m ago" line | Retry |
| Offline | `selectConnectionState` is `stale` or `disconnected` | Thin banner on top: "Offline - showing cached, updated <time>"; every server-mutating control disabled with the reason "Offline"; no empty-state success | Reconnect: controls re-enable, banner clears |
| Load more | Scroll to end of virtual list | 1 spinner row "Loading earlier notifications"; on error "Could not load more - Retry" | Retry |
| RPC failure (any bulk) | Surface 9 | Optimistic rollback + visible message | Retry |

Rule: "All caught up" never appears while offline or after a load error (that would be a false success). The toast deck has no loading or empty state by design; offline it shows no banner of its own, keeps existing toasts, disables Approve and Deny with the reason "Offline", and leaves its client-only controls enabled, while the existing `ConnectionIndicator` and the tray banner carry the status. (With the keyboard open the tray is closed, so the status is carried by `ConnectionIndicator`, not by a tray the user cannot see.)

**UX acceptance criteria (Surface 14)**
- **TE-1** The "All caught up" success state shows only when connected, the last fetch succeeded, and there are 0 rows.
- **TE-2** While offline the banner "Offline - showing cached, updated <time>" is visible, every server-mutating control is disabled with the accessible reason "Offline", and no empty-state success is shown.
- **TE-3** A failed history load while connected shows "Could not load notifications." with a Retry that works without closing the tray, and is not presented as offline.
- **TE-4** While offline no dismiss is queued, replayed on reconnect or marked "pending sync": row Dismiss, swipe-dismiss and the bulk actions are disabled; on reconnect they re-enable and the banner clears.
- **TE-5** Loading skeletons use `aria-busy` and do not announce each row; the filtered-empty state has a Clear filters button >= 44px.
- **TE-6** Every state in this table has at least one visible control or an automatic exit (no dead ends).
- **TE-7** The offline banner, the deck's disabled Approve/Deny and the Reply card's offline state all derive from `selectConnectionState` through one hook; no notification code reads `navigator.onLine` or opens its own connection-state reader (a grep test), and `historyError` never sets the offline state.

## 17. Surface 15: Push click flow (hidden failure)

```
 push (hidden failure) -> tap -> app opens -> read-only view
        |                                |-- session deleted --> deleted card
        |                                |-- list empty/cold --> fallback getSession
        +--> app already open --> (Story 5.5) focus window, route in-app, no reload
```
- Notification text: "<role> session failed: <title>" (not "Session Completed"). Routine completions never push (gate).
- Actions on the OS notification: Open (default tap) and Dismiss. No "Approve" on push for hidden items.
- Entry: OS tap. Exit: read-only view (Surface 12) or deleted card. Errors: getSession network failure shows Retry; app cannot reach the server shows the existing offline screen with the tray offline banner.
- The inline push payload URL is `/?session=<id>&tab=terminal&notification=<notification id>` (plan Task 5.3e, `server/push/subscriber.go:193-212`); the status-change push has no record and keeps the two-param URL (C5 resolved).

**UX acceptance criteria (Surface 15)**
- **TP-1** Tapping a hidden-failure push with the app closed lands on the read-only view (not the main list, not an error).
- **TP-2** Tapping it when the session no longer exists lands on the "Session no longer available" card with Open notifications and Go to sessions buttons.
- **TP-3** With the app already open (Story 5.5 shipped), the click focuses the existing window and routes in-app with no full page load.
- **TP-4** The push title for a hidden failure names the failure ("Review failed: ..."), not "Session Completed".

## 18. Surfaces 16-17 (non-interactive, condensed)

**Surface 16: gate suppression log line**
```
{"level":"INFO","msg":"delivery_suppressed","channel":"push_status","session_id":"ee1b4be0-...","notification_type":4,"priority":3,"reason":"routine_for_hidden","suppressed_since_last":165}
```
- **GL-1** One log line per (session, type, reason) per 60s, with `suppressed_since_last`.
- **GL-2** Fields `channel`, `session_id`, `notification_type`, `priority`, `reason` always present; channel values only from `bus`, `push_status`, `auto_approved`, `slack`.
- **GL-3** An allowed hidden delivery logs `hidden_delivery_allowed` with `class`; nothing is silently dropped.
- **GL-4** Shadow mode logs `delivery_would_suppress` and delivers.
- **GL-5** No log line includes message bodies (only ids and types).

**Surface 17: flags (Settings > Features, existing panel)**
- **FG-1** `hidden_session_gate` and `notification_tray_v2` appear as toggles with a one-line description each and the text "Takes effect without reload".
- **FG-2** Toggling `notification_tray_v2` off restores the legacy list and modal panel on the next render, without a page reload and without losing existing notifications.
- **FG-3** Flipping either flag shows the live state read back from the server after save (not an optimistic checkmark).
- **FG-4** When `hidden_session_gate` is off before the PR 2b default flip, the panel states "Shadow mode: hidden sessions still notify; would-suppress counts are logged"; after the flip, an explicit off shows "Gate is OFF: hidden sessions deliver everything" (the flag's status line).
- **FG-5** For a scopable flag (`hidden_session_gate`) the panel shows an "Overrides by session kind" disclosure with Inherit / On / Off for review, triage, diagnose and other (each control >= 44px and labelled); every change displays the server read-back (FG-3), and `notification_tray_v2` shows no override controls.
- **FG-7** Under `hidden_session_gate` the panel shows the shadow-stats line from `GetDeliveryGateStats` ("Shadow: N hidden events would have been suppressed in the last 24h; M unresolved fail-open; K unversioned ssq-notify"), readable with telemetry uninitialized; it is the standing reminder that a flag that ships dark is not yet shipped (plan Stage 1b).
- **FG-6** Choosing Inherit clears the override (the effective value follows the global setting again) and the row text states the effective value and where it comes from ("On - from global" or "Off - override").

## 19. Cross-cutting accessibility and layout criteria

- **XA-1** Axe reports 0 WCAG 2.2 AA violations on the toast stack story, the tray in each variant (`side-overlay`, `bottom-sheet` and, once plan Task 4.2e lands, `top-sheet` and `landscape-panel`), Background activity, the read-only view, the deleted card and the Reply card. (Axe cannot compute contrast for toast, chip or handle composited over the xterm canvas: that is the manual check MC-1, plan Task 3.7g.)
- **XA-2** All interactive controls on touch variants have hit areas >= 44x44 CSS px and >= 8px spacing; desktop >= 44px for toast and tray controls too (requirements), never below the WCAG 2.5.8 24px floor.
- **XA-3** Text contrast >= 4.5:1, UI/graphic contrast >= 3:1, in light and dark; type and status never conveyed by color alone.
- **XA-4** No toast or tray surface fully hides the currently focused element (WCAG 2.4.11); the deck never overlaps the terminal input row on phones.
- **XA-5** Every gesture (swipe row, swipe toast, sheet drag) has a single-pointer non-gesture alternative (WCAG 2.5.1).
- **XA-6** Auto-closing toasts pause on hover/focus/tray-open and are always recoverable from the tray (WCAG 2.2.1); pinned toasts never auto-close.
- **XA-7** New toasts, banners, receipts and status lines are announced through the single `Announcer` (one polite and one assertive region, priority queue, 500ms coalescing) and do not take focus (WCAG 4.1.3).
- **XA-8** Character-key shortcuts are scoped to focus inside the tray list (WCAG 2.1.4); the global hotkey uses a modifier chord.
- **XA-9** `prefers-reduced-motion: reduce` removes slide/scale/fling animation across all surfaces.
- **XA-10** Opening or closing any toast/tray surface never changes xterm `cols x rows`, never remounts the terminal root node, never sends a terminal resize, and never writes `body` overflow.
- **XA-11** On mobile, no surface is placed under the soft keyboard; `--viewport-height` governs all sheet heights; safe-area insets are honored on all four sides.
- **XA-12** Focus return: closing a keyboard- or hotkey-opened tray returns focus to the previously focused element, and to the xterm textarea when that was the opener. On `pointer: coarse`, closing by tap or Back never calls `.focus()` on the xterm textarea when the keyboard was closed at open time (it would summon the keyboard and vote a resize), and when the keyboard was open at open time the textarea keeps focus throughout so nothing is restored.
- **XA-13** Both form factors are covered by Playwright runs at the four profiles of `tests/e2e/helpers/viewport-profiles.ts` (plan Task 3.7e): `V1-desktop` 1280x800, `V2-portrait` 390x844, `V3-landscape` 844x390 and `V4-portrait-keyboard` 390x844 with the keyboard var set. All four profiles apply to the toast stack, the tray (the capped bottom sheet at 844x390 until Task 4.2e adds the `landscape-panel`, which is then added to the matrix), the read-only view and the Reply card. A meta-test fails if a layout spec lacks a profile. Real soft-keyboard and device behavior (TD-3, TK-1, TS-2, RP-9) rests on DV-1..DV-6, which Playwright cannot run.
- **XA-14** Pinch-zoom is not disabled; content reflows at 320px (WCAG 1.4.10).
- **XA-15** Single live-region owner: on every notification surface (deck, tray, Background, read-only banner, Reply card) the only `role="status"` and `role="alert"` elements are the `Announcer`'s two regions (`data-testid` `announcer-polite` and `announcer-assertive`); no surface renders its own.

## 20. Flow matrix (entry, exit, error)

| Flow | Entry | Exit | Error state | Has exit? | Has error state? |
|------|-------|------|-------------|-----------|------------------|
| Toast arrives and is acted on | Push event | action done / x / swipe / auto-close | Retry inline (TC-8) | yes | yes |
| Move all to tray | Deck header | undo expiry or Undo | undo expiry is the terminal state; no network call | yes | n/a (client only), stated |
| Open tray (4 variants) | handle, bell, chip, hotkey, chip "+N" | x, Esc, handle, Back, hotkey | render error boundary (Surface 4) | yes | yes |
| Mark activity read | header | immediate | revert + Retry | yes | yes |
| Clear informational | overflow | Cancel / Undo / elapse | rollback + "Could not clear notifications" | yes | yes |
| Clear history | overflow | Cancel / confirm | dialog stays with Retry | yes | yes |
| Row dismiss / swipe | row | undo / elapse | rollback + Retry | yes | yes |
| Group dismiss (has a pinned row) | group header | n/a | refuses, offers "Dismiss N informational" | yes | yes |
| Background activity | segment tab | switch segment / close | Retry + stale data (BA-5) | yes | yes |
| Read-only hidden view | toast, tray, Background, push, link | back/nav | Retry; deleted card (RO-3, RO-7) | yes | yes |
| Reply to a pending question (hidden) | Reply on toast, tray row, Background row, or the card in the read-only view | receipt, or leave the view | inline error with preserved text (RP-5..RP-7); no_pending, rate_limited, offline | yes | yes |
| Quiet mode | toggle | toggle | storage failure note | yes | yes |
| Offline (any server-mutating action) | disabled control with the reason "Offline" | reconnect re-enables | nothing is queued; banner explains | yes | yes |
| Undo (deck move-all, tray clear, row dismiss) | undo control (chip row on phone, header slot on desktop, bar in the tray) | Undo, window end, or page hide | page hide flushes with `keepalive` or cancels | yes | yes |
| Push click | OS tap | read-only view / deleted card | getSession failure Retry | yes | yes |

No flow lacks an exit path or an error state. Two rows state the error case is "not applicable" (client-only deck bulk action) rather than missing.

## 21. Conflict resolutions against `implementation/plan.md` (Phase 3 iteration 2)

All conflicts below are reconciled in both documents. "Plan" references are to `implementation/plan.md`.

- **C1. Mobile toast anchor. RESOLVED (operator decision, ADR-009).** Mobile session pages dock at the top under the tab row; desktop stays bottom-right. Plan Story 3.7 rewritten; bottom offsets apply only to desktop and mobile pages with no terminal. DV-1..DV-6 device checks are plan Task 3.7d.
- **C2. "Dismiss all" can be a no-op. RESOLVED.** Plan Story 3.4 adds "Move all to tray (N)" (demote-only, no RPC, undo) with Tasks 3.4d-3.4e; Story 3.9 gets a `kind:"moved"` message. TB-2, TB-8, TB-9.
- **C3. No top-offset var. RESOLVED.** `--mobile-stack-top-offset`, measured and published by the session layout; plan Task 3.7c; TD-13.
- **C4. `Alt+N` status. RESOLVED as "unverified Stretch".** No "confirmed free" claim remains. Option+N on macOS is a dead key; Alt+N in terminals is `ESC n`. Plan Task 4.2f spikes a chord or drops the hotkey; the core tray has no hotkey.
- **C5. Deleted-session card lacks data. RESOLVED.** `notification=<id>` on toast, tray, Background and inline push links (plan Task 5.3e); message-less fallback when absent. RO-3, RO-10.
- **C6. Button label. RESOLVED.** "View output" with `data-testid="notification-view-output"`; plan Task 5.3f changes e2e to click by testid.
- **C7. Background join undefined. RESOLVED.** Defined in Surface 11 and plan Story 5.4 / Task 5.4e; BA-9.
- **C8. Label drift. RESOLVED.** "Mark activity read" in the plan (Story 4.4), matching the shipped label at `NotificationsPage.tsx:304`.
- **C9. Needs-decision pin. RESOLVED.** Plan Story 4.3 / Task 4.3d adds the pinned group and the "N need a decision" header; TR-2, TR-10.
- **C10. Clear history confirm. RESOLVED.** Inline confirm replaces native `window.confirm` (shipped at `NotificationPanel.tsx:124`, `NotificationsPage.tsx:206`); plan Story 4.4 / Task 4.4e; TM-5.
- **C11. Mobile focus vs soft keyboard. RESOLVED.** Pointer-opened sheets never take focus; plan Spike 1.2, Story 4.1 AC, Story 4.2 AC, Task 4.2g; TK-1, TK-3.
- **C12. Row swipe vs panel swipe. RESOLVED.** Only the grabber drags the bottom sheet; side panels have no swipe-to-close; no edge-drag-to-open; row swipe is row-scoped; plan Stories 4.2 and 4.3, Task 4.2g.
- **C13. "Focus Window" on touch. RESOLVED.** Hidden on `pointer: coarse`, max 2 visible actions, desktop overflow; plan Story 3.8 / Task 3.8d; TC-5.
- **C14. Desktop handle overlays terminal columns. RESOLVED as a decision (O6), visual check still pending.** The operator chose the fixed handle. Plan Task 4.2h is still a manual visual check on a real terminal; the plan escalates to the operator only if the handle hides needed columns.
- **C15. Hidden `INPUT_REQUIRED` toast. RESOLVED, extended by O2.** The toast follows the hidden variant (Background chip, "View output") and, when a question is pending, also offers "Reply" (Surface 12b, plan Story 5.6); plan Story 5.3 AC.

### Iteration 3 changes (operator decisions and re-review findings)

- **C16. Hidden `INPUT_REQUIRED` was a dead end (operator decision O2). RESOLVED.** Surface 12b adds the audited Reply card; the Background row, tray row and toast carry a Reply action while a question is pending (TC-10, BA-10, RO-11, RP-1..RP-10). The read-only promise is kept for everything else (ADR-005, ADR-010).
- **C17. Two mobile tray chips could read as duplicates (adversarial M6). RESOLVED.** One affordance at a time (TH-7, plan Task 4.2i).
- **C18. Pinned toast predicate (adversarial N5). RESOLVED.** Pinned is the server-sent `is_pending_decision` on the live event; this document no longer lists types as a client rule (Surface 1 rules, TD-5, TM-7).
- **C19. Stale `top-sheet` wording (adversarial M1). RESOLVED.** The keyboard-open chip opens the capped bottom sheet in core; `top-sheet` stays Stretch.
- **C20. Per-kind flag override (operator decision O1). RESOLVED.** FG-5, FG-6.

### Triad iteration 1 changes (2026-10-09)

- **C27. Live-region ownership (triad UX B1). RESOLVED.** One `Announcer` owns one polite and one assertive region; no surface renders its own `role=status`/`role=alert`; coalescing wins inside 500ms per channel and the assertive channel is reserved for the single highest-priority pinned item (TD-8, XA-7, XA-15, RO-8, RP-4, RP-7, TM-10; plan Story 3.6, Tasks 3.6a-3.6c).
- **C28. Focus restore re-summons the soft keyboard (triad UX B2). RESOLVED.** Coarse-pointer close never moves focus into xterm when the keyboard was closed; with the keyboard open nothing is restored; the expanded sheet is non-trapping on touch and trap-with-exit on fine pointers; TS-9 adds the keyboard-open Expand test (D6, TS-4, TK-4, XA-12; plan Story 4.1, 4.2, Task 4.2g).
- **C29. TE-1..TE-6 had no owner and a conflicting offline queue (triad UX B3). RESOLVED.** One connectivity source (`selectConnectionState`), one offline policy (server-mutating actions disabled, nothing queued), tasks 3.3e and 4.3e-4.3g, TE-7 (Surface 14).
- **C30. "Needs a decision" included an auto-remediating WARNING. RESOLVED.** The group is "Needs attention"; a WARNING stamped `auto_remediating=true` is informational (D4, TD-5, TR-10, TR-11; plan Story 3.2, Task 3.2d, ADR-008).
- **C31. "Dismiss all" / "Move all to tray" mode flip. RESOLVED.** A single "Move all to tray (N)" control; dismissal is per toast (Surface 3, TB-1..TB-10).
- **C32. Undo placement, window and tab close. RESOLVED.** Undo replaces the chip row on phones, the header slot on desktop, a sticky bar in the tray; default 8s, per-device 5/8/15/30s, paused on hover/focus; page hide flushes with `keepalive` or cancels (TB-4, TB-10, TM-3, TM-12; plan Tasks 4.4g, 4.4h).
- **C33. Mobile with 1 toast and 0 overflow had no tray entry. RESOLVED.** The floating chip hides only while the top "+N more" chip is visible (TH-7, TH-8; plan Task 4.2i).
- **C34. Desktop deck and handle overlap with the terminal. RESOLVED for the deck, checked for the handle.** TD-15 and plan Task 3.7f for the deck; TY-10 and Task 4.2h for the handle (decision O6 unchanged).
- **C35. Pinned phone card could cover the terminal indefinitely. RESOLVED.** Auto-collapse to a chip after `PINNED_COLLAPSE_MS` (TD-14; plan Task 3.5c).
- **C36. Overlay versus layout. RESOLVED.** The mobile stack overlays; the status row is covered while a toast is up; DV-1 records whether that matters (Surface 1).
- **C37. Swipe feedback and the hover-only tooltip. RESOLVED.** In-drag reveal labels and a visible tray control on pinned cards (TC-1, TC-11; plan Task 3.8e).
- **C38. Reply prompt truncation, quick-picks and banner copy. RESOLVED.** 280 characters with a disclosure, quick-picks gated by Spike 1.3g, banner secondary text follows the card (RP-11, RP-12, RO-12).
- **C39. Undoable and irreversible actions adjacent in one menu. RESOLVED.** Divider, caption and destructive styling (TM-11).
- **C40. Device-only criteria and the Axe composite-contrast gap. NOTED.** TD-3, TK-1, TS-2, RP-9 are marked as resting on DV-1..DV-6; toast-over-xterm contrast is the manual check MC-1.
- **C41. Stretch labels. RESOLVED by operator decision O8.** Quiet mode, the hotkey, `top-sheet`, `landscape-panel` and the push handoff are in scope, sequenced last.

### Phase 4 changes (validation, consistency and pre-mortem patches)

- **C21. Quiet-mode criterion contradicted the pinned set. RESOLVED.** TQ-1 used `warning` as the demoted example, but WARNING is server-pinned (`IsActionableType`, TD-5). TQ-1 now uses a non-pinned toasting type (`custom`) and states that every pinned type still toasts; Quiet mode is labelled Stretch throughout (inventory row 13, Surface 13, TQ-1..TQ-5, flow row).
- **C22. Surface 7 and 8 described Stretch variants as core. RESOLVED.** Both headings are labelled Stretch; the core path is the capped bottom sheet; TK-1..TK-5, TL-3 and TL-4 are core criteria asserted against it, TL-1 is Stretch, TL-2 belongs to the toast story; XA-1 and XA-13 state which variants and profiles are core.
- **C23. Reply length unit. RESOLVED.** Bytes everywhere (client `TextEncoder` counter, server 500 bytes); RP-2 and RP-7 updated.
- **C24. Reply Retry after a failed write (validation G-3). RESOLVED.** Failed write releases the claim; only `sent` is cached by `reply_id`.
- **C25. `kept` response handling (validation G-5). RESOLVED.** TM-10.
- **C26. Shadow-stats status line. ADDED.** FG-7.

## 22. Acceptance-criteria index

Criteria IDs by surface: TD (15), TC (11), TB (10), TH (8), TY (10), TS (9), TK (5), TL (4), TM (12), TR (11), BA (10), RO (12), RP (17), TQ (5), TE (7), TP (4), GL (5), FG (7), XA (15) = 177. Recounted by script in review-repair iteration 4 (176 before; RP-17 added), in review-repair iteration 2 (175 before; RP-16 added), in review-repair iteration 1 (172 before; RP-13, RP-14 and RP-15 added) and in triad iteration 1 (157 before; 15 added: TD-14, TD-15, TC-11, TB-10, TH-8, TY-10, TS-9, TM-11, TM-12, TR-11, RO-12, RP-11, RP-12, TE-7, XA-15); the story-to-criterion mapping is `implementation/plan.md` "UX criterion coverage by story".
