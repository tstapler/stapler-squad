# Manage notifications: the toast deck and the tray

Use this when a burst of notifications covers the terminal and you want the capped deck and the tray, or when you need to roll them back.

## Turn the capped deck on or off

1. Open **Settings > Features**.
2. Toggle **Notifications: capped toast deck with Move all to tray**.

The flag is `notification_tray_v2`. It is off by default and applies on the next render, with no reload. It switches the deck **and** the tray: off restores the legacy list that shows every toast and the original modal notification panel.

## What the deck does

- Shows at most 3 toasts on desktop, 1 on a phone, and none (a one-line chip) while the soft keyboard is open. Anything over the cap collapses into a **+N more** chip that opens the notification panel.
- Puts pending decisions first. A toast is pinned exactly when the server marks it `is_pending_decision` (an unread approval, question, error, failure, or a warning that is not auto-remediating). Pinned toasts never time out.
- Docks bottom-right on desktop (top-right while it would cover the terminal cursor), and under the session tab row on a phone.

## Clear the screen without losing anything

**Move all to tray (N)** takes every toast off the screen. It does not mark anything read, delete history or call the server, so every pending approval is still there. An undo bar (**Moved N to tray - Undo**) stays for 8 seconds; hover or focus on it holds it open. The other open tab drops the same toasts.

To dismiss one toast, use its **x** (informational) or **Tray** (pinned) control, swipe it, or press Delete or Backspace while it is focused. Its history row stays.

## Per-device timing

Two settings are stored in this browser's local storage, not on the account:

| Key | Values | Default |
|---|---|---|
| `ssq.notifications.pinnedCollapse` | `8000`, `15000`, `30000`, `never` | 8 seconds |
| `ssq.notifications.undoWindow` | `5000`, `8000`, `15000`, `30000` | 8 seconds |

On a phone, a pinned card with no interaction shrinks to a one-line chip after the collapse delay; tapping it expands it again. A value that is missing, unreadable or unknown falls back to the default.

## The tray

With the flag on, the notification panel is a non-modal tray. Opening it never resizes or remounts the terminal, never dims the page and never takes focus from it when you open it with a pointer. `Esc` from inside the tray closes it and returns focus to the terminal.

| Where | Tray | Entry points |
|---|---|---|
| Desktop (900px and wider) | Right-edge overlay, `min(400px, 40vw)` wide | Edge handle at the right edge (capped "99+"), header bell |
| Phone portrait | Bottom sheet: peek (about 25% of the screen), **Expand** for about 85% | One entry chip docked at the top: bell, "+N more" or a one-line chip while the keyboard is open |
| Phone portrait, keyboard open | Top-anchored sheet, never under the keyboard | The same entry chip |
| Phone landscape | Right panel, at most `min(360px, 50vw)`, clear of the notch | The same entry chip |

Only the sheet's grabber drags it. Hardware Back closes a sheet without leaving the session. There is no edge-swipe to open.

### What is in it

- **Needs attention (N)** is pinned at the top: every unread decision (the server's `is_pending_decision`). It has no dismiss control and no swipe, and it is excluded from every bulk action. An auto-remediating warning ("PR needs attention ... An automated fix attempt will run") is informational and sits in its session group.
- One collapsible group per session, windowed so a few hundred rows stay fast. Collapse state is per tab.
- Swipe a row or press `x` while the list has focus to dismiss it; **Undo** is available for the undo window.
- While you scroll, new arrivals wait behind a **N new** pill so rows do not move under your finger.

### Bulk actions

| Action | Where | What it does |
|---|---|---|
| Mark activity read | Header | Marks unread non-decision rows read. Decisions stay unread. |
| Clear informational (N) | Overflow menu, above the divider | Hides the N non-decision rows now, shows "Cleared N - Undo", and deletes them on the server only after the undo window. |
| Clear history... | Overflow menu, below the divider | Inline confirm ("Clear N read notifications? This can't be undone. M awaiting decision kept."), no undo. |

The server guards the same rule again: a request that lists a pending decision deletes nothing for it and reports it in `kept_ids`, and the tray shows "N kept: still needs attention". If the page is hidden while an undo window is open, the clear is sent immediately (with `keepalive`) instead of being lost.

While the connection is stale or down, every server-changing control is disabled with the reason "Offline". Nothing is queued.

### Quiet mode, Pin and settings

- **Quiet mode** (tray header) sends every non-pinned toast straight to the tray on this device. A pending decision still toasts. Stored as `ssq.notifications.quietMode`.
- **Pin** (desktop only) docks the open tray as a column so the terminal narrows instead of being covered. This is the one mode that resizes the terminal, once per toggle. Stored as `ssq.notifications.trayPinned`.
- **Tray settings** (overflow menu): the undo window (5, 8, 15 or 30 seconds) and the pinned-card collapse delay (8, 15, 30 seconds or never).
- **What changed** is a one-time card on the first open after the upgrade; reopen it from the overflow menu.

### Hotkey

No tray hotkey ships. `Alt+N` was a candidate, but a chord that xterm never sees needs real macOS and terminal checks first (Option+N is a dead key on macOS and `Alt+N` reaches the shell as `ESC n` elsewhere). Until that spike is run, use the handle, the header bell or the phone entry chip.

## Where does "Claude Notification ... via tmux" come from?

The subtitle is `ssq-hook-handler`'s `source_app=tmux` for any stapler-squad-managed tmux session (`scripts/ssq-hook-handler:231-235`), rendered as `via ${sourceApp}` (`NotificationToast.tsx` and `NotificationItem.tsx`). It is not a hidden-session leak: the named session in the original report has `hidden=0`.

## Roll back

Toggle the flag off. The tray, deck, handle, entry chip, Quiet mode and Pin all return to the legacy behavior. If the deck itself misbehaves on a device, the bottom-dock fallback for phones is a CSS and variant change recorded in ADR-009.
