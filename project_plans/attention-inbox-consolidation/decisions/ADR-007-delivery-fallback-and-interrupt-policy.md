# ADR-007: Delivery fallback and interrupt policy for finished and waiting sessions

**Status**: Proposed (defaults are OWNER TO CONFIRM; listed in plan.md Owner to confirm). Amended in triad repair iteration 2 (decisions 1, 2, 4, 5, 6 revised; decision 7 added).
**Date**: 2026-10-23
**Project**: attention-inbox-consolidation

## Context

ADR-002 made `AwaitingDirection` a delivered notification (tray plus web push, one per turn-end episode, at most one push per session per 5 minutes) and `ProseQuestion` an `INPUT_REQUIRED` notification. The triad review found these holes in that policy:

1. Nothing tells the operator when push cannot reach them (no subscription, permission denied, installed-PWA gaps, backgrounded tab). The row exists, but the operator was not told, which is the original complaint.
2. The 5-minute cap is per session. Eight sessions finishing in ten minutes produce eight pushes.
3. There is no way to quiet one noisy session or mute finished-session alerts for a while.
4. A needs-you item detected from screen text looks identical to a hook-confirmed one.
5. (iteration 2) A chatty agent whose replies are misread as questions re-raises toast, push and Slack every turn, and "Not a question" only clears one episode.

## Decisions

1. **One unseen-finished rule, independent of push.** The nav shows the text cue "N new finished" ("new" is the visible word for unseen, and the label is distinct from the list chip "Finished N", which counts every open informational row, Seen or not), where N counts every visible-session `awaiting_direction` item that is not Seen (server-derived, decision 2). The count never depends on whether push can reach this device: the server cannot detect that, and a cue that differs by device is the defect this decision removes. Push is a best-effort extra on top. The cue is text, separate from the needs-you badge number and from the document title, with no animation, no `aria-live`, no sound; it is hidden at 0. A device-local line "Push is off on this device" may explain why push is silent, but it is informational and never gates the cue.
2. **Seen is server-derived.** `Seen = LastViewed >= TurnEnded.At`, using the existing `Instance.LastViewed` so phone and desktop agree. The earlier per-viewer `localStorage` Seen is removed. Verified in code (iteration 2): `LastViewed` is already in `InstanceSnapshot` through the embedded `ReviewState` (`session/review_state.go:64`, copied by value at `session/instance_snapshot.go:161`, read as `snap.LastViewed` at `session/instance_serialization.go:133`), so no snapshot field is added. It persists only on full saves: `EntRepository.UpdateLastViewed` (`session/ent_repository.go:1239`) has one non-test caller, `Storage.UpdateInstanceTimestampsOnly` (`session/storage.go:709`), which itself has no production caller. Crash window: after a non-graceful restart a Seen mark made since the last full save can revert, and the cue re-counts that session until it is opened again (a safe direction: over-counting, never hiding). The four production `MarkViewed()` callers are terminal stream handlers (`streamViaControlMode`, `streamViaHub`, `streamViaTmuxCapturePane` in `server/services/connectrpc_websocket.go:1320,2112,3609`, and `streamShellViaControlMode` in `connectrpc_websocket_shell.go:420`), not an "open session detail" call. Cost: one proto field (`last_viewed`, Task 5.2.1j) and a check that opening a session on the web attaches one of those streams; if some open path does not, Task 5.2.1j adds a unary `MarkSessionViewed` write RPC.
3. **Global cap plus digest.** In addition to the per-session 5-minute cap, at most 6 `awaiting_direction` web pushes per rolling 15 minutes across all sessions; overflow becomes one collapsed digest push with a stable notification tag. Tray records are never capped. `ProseQuestion` is not held by this cap; it has its own burst guard (decision 7). Numbers are constants, owner adjustable.
4. **Mute: one word, one duration set, explicit scope.** "Mute" (the words pause and snooze are not used in the UI) is offered per session and globally, both for 1 h or 8 h, persisted server-side so every device honors it, auto-expiring (max 24 h), shown as static text ("Muted until 16:20"), undoable. Scope:

   | Mute | Suppresses push and Slack for | Never suppresses |
   |---|---|---|
   | Session | that session's `awaiting_direction` AND `prose_question` | approvals, `input_required`, errors, tests failing, waiting-for-input |
   | Global | `awaiting_direction` of all sessions | everything else, including every `prose_question` |

   Mute never hides a list row, the badge count, the unseen cue or a toast, so a muted session cannot make a question disappear. The global-mute marker rule (one rule, desktop and mobile, every page): a global mute shows the short visible word "muted" on the nav cue line (inline on desktop, on the second text line on mobile) and the accessible-name suffix ", alerts muted". A mute management list (active mutes plus opted-out sessions, each reversible; popover on desktop, bottom sheet from a 44x44 "More" button beside [?] on mobile) is reachable after any row has cleared.
5. **Unconfirmed detection signal.** A needs-you item whose `TurnEndSource` is not `Hook` carries a text tag, a "N unconfirmed" count in the list header, and a suffix in the nav badge's accessible name. It still counts in the badge (no silent drop of a possible question).
6. **Completion context.** The row shows the scrubbed, tail-biased excerpt (<= 160 chars) plus the timeline. Still deferred: any "PR opened / tests passed" outcome line (the Stop payload content is unverified, Spike 0.1.1, and outcomes are not available per program). Changed in iteration 2: the timeline drawer fetches the working-tree diff stat ONCE when it opens, through the existing read RPC `GetSessionDiff` (`proto/session/v1/session.proto:53`, returns `DiffStats{added, removed}`), and shows "Working tree now: +A -D" (labelled "now", never "at completion"; hidden on error). That removes the per-row cost objection (nothing runs on the list path) and adds no server surface. Revisit an outcome line if Spike 0.1.1 finds a last-message field carrying one.
7. **Prose-question recourse and burst guard.** (a) Row action "Stop flagging questions for this session" (persisted per session with the mute store, no expiry, reversible via "Flag questions for this session", an Undo toast, and, after the row has cleared, the mute management list and a "Questions not flagged" tag on the session card and rows; no auto-expiry, because an expiry would silently restart the flood, but a dismissible 14-day reminder line "Questions aren't flagged for N sessions. [Review]" (OWNER TO CONFIRM expiry vs reminder and the 14 days)): while set, a `Question` turn end for that session is recorded as `Finished` (so it becomes `awaiting_direction`). (b) Burst guard: per session, the first 3 `prose_question` episodes in any 30 minutes get full delivery; later ones in that window are list row, badge and tray record only (no toast, push or Slack), and the row shows the hint "Frequent questions from this session" next to the (a) action. Counted as `notifications_suppressed_total{gate_reason="prose_question_burst"}`. The guard applies to `prose_question` only and never to approval, `input_required` or error notifications. A tray record is still a channel, so the alert guarantee holds.

## Alternatives considered

- Make the unseen count part of the badge number: rejected, it would reintroduce the flood ADR-002 removed from the badge.
- Show the cue only when push cannot reach this device (iteration 1 wording): rejected, undetectable server-side, so devices would disagree.
- Per-viewer Seen (kept `localStorage`): rejected, devices disagree.
- Mute that also hides rows: rejected, it is silent burial.
- A per-row "PR opened / files changed" line computed on the list path: rejected (git operation per row); the lazy drawer stat replaces it.
- A passive "N quiet" cue in the nav: rejected for this release; Quiet is derived state, not an event, and a second count in the nav drifts toward the flood decision 1 avoids. Quiet stays pull-only (the Quiet chip count and Recent's "Quiet now").
- Remap UI for list shortcuts: not possible in this release, the shortcut registry has no per-key override (plan.md Verification Notes); on/off plus focus scope satisfies WCAG 2.1.4.

## Consequences

- New server surface: `SetNotificationMute`/`GetNotificationMutes`, the per-session question opt-out (same store), `ReviewItem.turn_end_source`/`unconfirmed`/`muted_until`, `last_viewed` (Stories 3.2.6, Task 5.2.1j). The mute and opt-out writes are added to `guardedProcedures` (plan Security NFR).
- A testable guarantee (Task 7.1.2a): every needs-you item and every `awaiting_direction` episode on a visible session produces at least one of tray, toast, push, digest, Slack, or an unseen-cue increment; a negative control proves the test can fail. Limitation stated in requirements: it proves a channel was produced, not that the operator perceived it.
- New metrics: `notifications_suppressed_total{gate_reason="awaiting_direction_global_cap"|"muted"|"prose_question_burst"}`, `awaiting_direction_digest_total`.
- If the owner rejects the delivered `awaiting_direction` (ADR-002 owner-to-confirm), decisions 1, 2 and 4 still stand and decision 3 becomes moot.
