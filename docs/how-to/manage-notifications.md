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

### Background activity

The **Background** tab beside **Notifications** shows what hidden sessions (review, triage, diagnose, headless) are doing without un-hiding them.

- A hidden session gets one row while its notification history holds an unread failure, error, approval or input record. Each row carries a "Background" chip, a status word (FAILED, NEEDS INPUT, NEEDS APPROVAL) and **View output**, which opens the read-only view and marks those records read, so the row leaves.
- Routine completions are never rows: they are one line, "N completed OK today", with "N running" beneath it.
- The tab badge counts rows only and never adds to the bell count; the same records are already counted there.
- A row stays, labelled "Session no longer available", when its session was deleted while the failure was unread. This only works for sessions this tab has seen hidden: history records carry no hidden flag, so after a reload a deleted hidden session's failure stays an ordinary notification.
- The list comes from `ListSessions{hidden_only: true}`: once when the tab is shown, then every 30 seconds, and never while the tab is collapsed, the tray is closed or the connection is down. **Refresh** polls now.
- The join reads the history page the tray has loaded (50 rows), so a failure older than that does not appear until **Load more** brings it in.

### Hotkey

No tray hotkey ships. `Alt+N` was a candidate, but a chord that xterm never sees needs real macOS and terminal checks first (Option+N is a dead key on macOS and `Alt+N` reaches the shell as `ESC n` elsewhere). Until that spike is run, use the handle, the header bell or the phone entry chip.

## Where does "Claude Notification ... via tmux" come from?

The subtitle is `ssq-hook-handler`'s `source_app=tmux` for any stapler-squad-managed tmux session (`scripts/ssq-hook-handler:231-235`), rendered as `via ${sourceApp}` (`NotificationToast.tsx` and `NotificationItem.tsx`). It is not a hidden-session leak: the named session in the original report has `hidden=0`.

## "A write to this session is stuck"

Every automated or unary write to a session's terminal (the driver's prompt and answer keys, a steer, a nudge, the MCP write tools, a rate-limit recovery) holds one per-session write lease, so two writers never interleave bytes. A write that never returns (a wedged pane) keeps the lease held; there is deliberately no "unstick" control, because a forced release would allow exactly the interleave the lease exists to stop.

- **What you see**: after 30 seconds one tray warning per stuck write, "A write to this session is stuck". It reaches the toast deck, the tray and history; it is never pushed. The server log has `terminal_write_lease_wedged` (at most once a minute) and the gauge `hidden_session_write_lease_held_seconds{writer}` rises.
- **What stops meanwhile**: the driver's prompt and answer key, steers, nudges and (later) Reply for that session get a retryable "a write to this session is in progress".
- **Remedy**: Delete the session, or restart stapler-squad. Pause and Delete both complete while the write is stuck (they do not take the lease) and the pane header's actions menu stays available in the read-only hidden view. Closing the PTY does not wake a blocked write (verified against a real PTY pair; only a reader on the slave side does), so a session that is Paused and then Resumed stays blocked until the service restarts.
- **Escape hatch**: the `terminal_write_lease` flag (Settings > Features, global only, default on). Off makes every lease non-exclusive, which is the behavior before the lease existed. Every flip is a `flag_change` line in the audit file; turning it off is persisted even when the audit sink is down.

## Hidden sessions stop notifying for routine events (and how to roll it back)

The hidden-session delivery gate is **on by default**. A hidden session (review, diagnose, triage, headless) notifies only for failures and needs-human events (errors, crashes, approvals, questions); routine completions, idle and rate-limit advisories are dropped on every channel (history, toasts, push, Slack, webhook callbacks). The per-site hidden checks that used to do this have been removed, so the gate is the only mechanism: see `docs/reference/notification-delivery-gate.md`.

To roll back, open **Settings > Features** and turn **Notifications: hidden-session delivery gate** off (globally, or per kind under the kind overrides). It applies within seconds. Off means hidden sessions deliver everything, including the routine events the removed checks used to swallow; the status line under the flag still counts what would have been suppressed. If **Stats writer not running** is shown, enabling is refused until the stats writer is up; disabling is never refused.

## A hidden session refuses a UI action ("read-only")

A hidden (background) session is read-only in the UI. The server refuses, with "this session is a background session and is read-only": raw terminal input (`WriteToSession`), restart (`RestartSession`, `RestartShell`), workspace switches of every type, an `UpdateSession` that changes `program` or `auto_approve`, a steer that is not the backlog Steer, and the manual PR nudge. Read RPCs, pause, resume and delete are unchanged, and MCP tools and `SteerActiveSession` (the internal steers) behave as before.

- **The one steer that still works**: the backlog "Steer" of a live review session. It is allowed only when the session's newest backlog row is an un-ended `review` row and the session is live (a tag never matters), the request passes the Host and Origin check, and the audit line can be written first. A steer of a triage, diagnose or unlinked session is refused with "no live backlog review link". The steer message may not contain control characters (LF and TAB are fine) or exceed 10000 bytes, and must be sent with no other field.
- **"Audit log unavailable, steer not sent"**: the steer's audit line could not be appended, so nothing was typed. Fix the audit directory (`<config dir>/audit/`, mode 0700) or use the MCP `steer_session` tool or the terminal.
- **Audit lines**: `kind=backlog_steer` with a `requested` line before the write and a `result` line after (`outcome` is `sent`, `failed` or `aborted_before_write`), recording the session, the item, the message length, its SHA-256 and the first 80 characters, and the Host, Origin and peer. The counter `hidden_session_backlog_steer_total{outcome}` counts `sent`, `refused`, `no_link`, `audit_failed`, `aborted_before_write`, `failed` and `guard_bypass`.
- **Escape hatch**: the `hidden_session_readonly_guards` flag (Settings > Features, global only, default on, applies at once). While it is off those RPCs accept a hidden target as before; the stream stays read-only and Reply has its own switch. A steer to a hidden session that is not a live review is then allowed only after a blocking `guard_bypass` audit line (refused when the audit log is down), and Settings shows "Hidden-session write guards are OFF: N writes bypassed". These guards are a safety rail against UI bugs and confused deputies, not a security boundary: anyone who can reach the server can already flip this flag.

## Answer a background session's question (Reply)

A hidden (background) session that asks "Claude has a question" is a needs-human event. With Reply you answer it from the toast, the tray row, the Background row or the read-only view instead of opening a terminal.

Reply answers a single-select question from a background session with one tap; other questions, and sessions whose hook has not yet been refreshed (a service restart or session resume refreshes it, no agent restart needed), show 'Answer in the terminal'. Free text is not supported.

- **What you see**: a card directly below the read-only banner with the question and one numbered button per option (`1. Red`, `2. Green`, ...). There is no text field. A tap sends that option's number once, and the receipt reads "Sent: 2. Green at 10:42 - the question closed". A toast, tray row or Background row shows **Reply** beside **View output** only while a replyable question is pending.
- **What is answerable**: one question, one answer (not multi-select), 1 to 7 options. A free-text answer ("Type something."), "Chat about this", a multi-select question and a multi-question call show "Answer in the terminal".
- **"Reply unavailable for this session"**: the session's permission hook has no valid sender proof. A service restart or a session resume rewrites the hook (a running Claude Code reads the rewritten `settings.local.json` at its next dialog), so no agent restart is needed. Until then the toast still appears and you answer in the terminal.
- **What the server checks before it types**: the live terminal capture must show exactly this question, these option labels in order and the two trailing rows ("Type something." and "Chat about this"), with nothing after the footer. A permission dialog, a shell prompt or a different question never matches (`STALE_PROMPT`, nothing written). It then writes one digit, with no Enter and no retry, after an `fsync`ed audit line, and watches for up to two seconds for the dialog to close (`SENT`), else reports "Sent? Check the terminal output to confirm" (`SEND_INDETERMINATE`, no Retry).
- **Outcomes**: `SENT`, `SEND_INDETERMINATE`, `NOT_SENT` (nothing was typed; Retry is safe), `NO_PENDING`, `STALE_PROMPT`, `NOT_HIDDEN`, `NOT_WAITING`, `RATE_LIMITED` (1 per 5 s per session, 10 per minute), `DISABLED`. The counter `hidden_session_reply_total{outcome}` also counts `invalid`, `audit_failed` and `refused`; `hidden_session_reply_registered_unreplyable_total{cause}` counts questions that were not replyable (`no_proof`, `bad_proof`, `shape`, `path_only`).
- **Audit lines**: `kind=reply` in `<config dir>/audit/hidden-session-replies.jsonl` with a `requested` line before the write and a `result` line after (`stage` is `dialog_closed`, `dialog_still_open`, `stale_prompt`, ...): the session, the question id, the `reply_id`, the chosen `option_index`, `option_label` and `question_token`, and the Host, Origin, peer and auth mode. No audit, no write: if the line cannot be appended the answer is `Internal` and nothing is typed.
- **Who may reply**: the request must pass the same Host and Origin rebinding check as the backlog Steer, and with authentication off the caller must be local (a proxied or remote caller gets `PermissionDenied`). This is a safety rail, not a security boundary.
- **Kill switch**: the `hidden_session_reply` flag (Settings > Features, global only, default on, applies at once). Off answers `DISABLED` and hides the card and the Reply actions; it does not affect the write guards, and `hidden_session_readonly_guards` does not affect Reply. Turning it on is audited before it is persisted; turning it off is persisted first.
- **The one residual risk**: a digit written in the few milliseconds between the final capture and the byte reaching the pane could land in a permission dialog that drew in that interval and approve option 1 or 2. The kill switch stops further replies but cannot undo a keystroke.
- **Provenance**: the shared hook command reads a per-session proof file (`<config dir>/hook-proofs/<session uuid>`, mode 0600) through `STAPLER_SESSION_UUID`; the proof is never in argv and a missing file still posts (`no_proof`). It narrows who can register a replyable question; `DialogMatch` is the control that stops a wrong-dialog write.

## Roll back

Toggle the flag off. The tray, deck, handle, entry chip, Quiet mode and Pin all return to the legacy behavior. If the deck itself misbehaves on a device, the bottom-dock fallback for phones is a CSS and variant change recorded in ADR-009.
