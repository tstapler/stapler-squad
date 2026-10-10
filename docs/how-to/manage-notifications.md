# Manage the toast deck

Use this when a burst of notifications covers the terminal and you want the capped deck, or when you need to roll it back.

## Turn the capped deck on or off

1. Open **Settings > Features**.
2. Toggle **Notifications: capped toast deck with Move all to tray**.

The flag is `notification_tray_v2`. It is off by default and applies on the next toast render, with no reload. Off restores the legacy list that shows every toast.

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

## Roll back

Toggle the flag off. If the deck itself misbehaves on a device, the bottom-dock fallback for phones is a CSS and variant change recorded in ADR-009.
