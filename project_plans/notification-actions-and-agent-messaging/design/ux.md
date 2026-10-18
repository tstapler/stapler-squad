# UX Design: notification-actions-and-agent-messaging

Date: 2026-10-17; triad review round 1 repair 2026-10-18 (sections 1.4, 2.4, 7.1, 11, 12 and UX-AC60 to UX-AC87 are new; UX-AC1, 7, 11, 15, 18, 23, 24, 36, 48, 49, 55 changed); triad round 2 repair 2026-10-18 (section 7 and 7.1 are now the single source for keyboard, engagement, list freeze, focus advance and digit shortcuts; Create rule leaves the primary approval row; delivery-state strings 14 to 15; a commit-delay hold is cancelled, never flushed, when the operator leaves; usability trials T8 to T10; no UX-AC was added, so the total stays 88); triad round 3 repair 2026-10-18 (digit moves no longer restart the digit grace and keep the card engaged; operator-expanded cards are engaged (7.1); the vocabulary guard is applied to every UI string, so "wakes", "Waiting on you" and the bare Cancel are gone; a 60 s still-showing dialog has its own delivery string (3.1 row 16) and a sent answer whose session ends has row 17, so delivery-state strings 15 to 17; free-text "Other" only on routes that can carry text; still no UX-AC added, total 88). Inputs: `requirements.md`, `research/ux.md` (the "R-n" row numbers below refer to the section 5 error table, numbered 1-17 in table order), `implementation/plan.md` (Stories 1.3, 2.5-2.8, 3.4, 3.7, 4.5, 4.6), ADR-001, 002, 004, 005.

Shared constants (inherited from `durable-guidance-request` and `notification-revamp` design docs): touch target 44px (`--min-touch-target`), mobile layout below 900px, 8px gap between stacked rows, 16px minimum font in text inputs (iOS zoom), polite live region present from first paint, no hover-only controls.

**Presentation rule (resolves the earlier contradiction between "never a focus-stealing modal" and the mobile bottom sheet).** Nothing in this design is a modal or a bottom sheet. Desktop: a non-modal popover (`role="region"`, `aria-label="Notifications"`, no `aria-modal`, no focus trap) anchored to the bell. Mobile: a full-height Alerts page reached from the bell, one scroll region, one browser-history entry (section 11.3 for Back). Opening either is the operator's own action and may move focus into it. No event arrival (a new notification, a poll result, an escalation) ever moves focus; "focus-stealing" means exactly that. The Remind-me menu and every sub-step are inline in the row, never a sheet.

## 0. Surface inventory

| # | Surface | Type | Flag |
|---|---|---|---|
| UX-S1 | Background feed (rows, roll-up, pinned exceptions, empty states, tab semantics) | Interactive | `background_activity_feed` |
| UX-S2 | Inline action row on notification items (approve / deny / backlog decision / open / desktop-only) | Interactive | `notification_inline_actions` |
| UX-S3 | `GuidanceRequestCard` family: single-select, multi-select, multi-question, `<select>` >8, free text, delivery-state line | Interactive | `askuserquestion_structured` |
| UX-S4 | Peer-help rows (operator-visible, informational by default, escalating) | Interactive | `agent_wake_triggers` |
| UX-S5 | "Scheduled prompts" section (pending triggers + open peer requests, Cancel prompt, "Scheduled prompts paused") | Interactive | `agent_wake_triggers` |
| UX-S6 | "Remind me" affordance on informational rows | Interactive | `agent_wake_triggers` (Task 4.6c; in scope, last wave) |
| UX-S7 | Cross-cutting: offline / reconnecting / loading / timeout states, live region, flag-off fallbacks | Condensed (section 9) | all |

Vocabulary (from research/ux.md section 3, extended): "Answer" for questions, "Approve / Deny" for permissions, "Decide" for backlog choices. Never "Resolve" as a button label.

| Internal term | Operator-facing term | Rule |
|---|---|---|
| wake / `WakeTrigger` | **scheduled prompt** ("an agent asked to be prompted again when ...") | The word "wake" never appears in UI copy. The section is "Scheduled prompts"; the paused banner is "Scheduled prompts paused" |
| `notify_operator` wake | **Reminder** (the operator's own, label "Reminder (you)") | Listed in the same section but visibly distinct from agent-owned prompts |
| "Waiting on" (old section name) | "Scheduled prompts" | "Waiting on" was ambiguous: agents waiting on the operator vs. the operator waiting on agents |
| Cancel | Reserved for ending something an agent started or the operator scheduled: **Cancel prompt** (the single label on a Scheduled prompts row), **Cancel request**, **Cancel reminder** (the operator's own, after the 10 s Undo). A bare "Cancel" button label is never used | Aborting an inline sub-step is **Back** (Deny reason, Merge confirm) and clearing a multi-select is **Clear**; declining a confirm is **Keep** |
| "Answer for them" | **Answer for {peer}** (the peer's session title) | The asker agent receives provenance `from=operator on-behalf-of={peer}`, so the agent can tell an operator answer from a peer answer |
| "Resume wakes" | **Resume scheduled prompts** | Scope stated in the confirm: "Turn scheduled prompts back on for all sessions? Overdue prompts still inside their window will be sent once." |
| `Unconfirmed` | A chip on a sent answer, never a delivery failure | Section 3.1 |

**Scope of the vocabulary guard (decided once).** UX-AC76 and `copyGuard.test.ts` apply to UI strings only, meaning every string exported by `notificationCopy.ts` and every literal rendered in the panel. Internal identifiers keep their names: `wake_trigger`, `WakeTrigger`, `agent_wake_triggers` (the flag name shown only in the flag-settings link), `CANCEL_WAKE`, `schedule_wake`, `wakes` metric labels, RPC and component file names. The section label is "Scheduled prompts"; the link chip on a Background row that has a pending decision is "Needs you (N)" (never "Waiting on you").

---

## 1. UX-S1 - Background feed

### 1.1 Wireframes

Desktop bell panel (width ~420px, non-modal popover) and mobile (full-height Alerts page, single scroll region; never a sheet):

```
 [ Notifications 3 ] [ Background 1 ]                        <- role=tablist; the badge on Background counts only unacknowledged exceptions, never sessions (section 1.4) ("Needs you" and "Recent activity" are tiers inside Notifications, not tabs; NotificationPanel.tsx:785)
 ---------------------------------------------------------
 Needs attention (1)                                        <- pinned, always first
 +-------------------------------------------------------+
 | (!) FAILED  review:pr-1042   failed 2m ago             |
 |     "Tests failed in review"          [Open session]   |
 +-------------------------------------------------------+
 11 background sessions: 10 no failures reported, 1 needs attention.   <- roll-up (data-testid=background-rollup)
 Last activity 3m ago.                  [ 1 new ]           <- pill only when poll added rows
 No failures reported (10)                            [v]   <- <button aria-expanded>
 +-------------------------------------------------------+
 | (check) FINISHED  review:pr-1040   review   3m ago     |
 |         Backlog: Fix flaky test        [Open] [Remind] |
 | (stop)  STOPPED, OUTCOME UNKNOWN  diagnose:abc  1h ago  |
 |                                         [Open]         |
 +-------------------------------------------------------+
 Scheduled prompts (2)                                       [v]   <- UX-S5
```

No-failures variant (replaces the earlier "all healthy" copy, which overclaimed because absence of a failure record is not proof of health): only the roll-up line "11 background sessions, no failures reported. Last activity 3m ago." followed by the collapsed "No failures reported (11)" disclosure; rows are `Finished` only when the server-computed `background_outcome` is `completed` (a recorded review verdict or triage result on the session's backlog item session; plan Story 1.7), `Failed` when it is `failed` (crashed pane or a failed headless call), otherwise `Stopped, outcome unknown`. Notification history is never the evidence for `Finished`: the delivery gate suppresses those records for hidden sessions. No-sessions variant: "No background work. Review and diagnose sessions will appear here while they run." No spinner, never blank.

### 1.2 Flow

| Step | Operator | System |
|---|---|---|
| Entry | Opens bell (desktop) or Alerts (mobile), selects Background tab; or arrives via a "Background" deep link | Renders from cached poll state within 200ms; focus stays on the tab |
| Happy path | Reads roll-up; scans pinned exceptions; expands "No failures reported"; clicks **Open** on a row | Navigates to the session (or linked backlog item when present); panel closes on mobile, stays open on desktop |
| New data | Reading while a poll adds a session | Existing rows do not move; "1 new" button appears; activating it merges and announces "1 new background session" via the polite region |
| New failure | Reading while a poll shows a session failed or needs a human | Never queued behind the count-only pill: a sticky strip above the list names it ("review:pr-1042 failed" with **Show** and **Open**); an existing row that becomes a failure gains its failure chip in place and moves to the pinned group at the next unengaged merge (section 1.4) |
| Exit | Esc / tab switch / close; **Open** | Esc closes panel and returns focus to the bell; no state is lost |
| Departed session | Clicks Open on a row whose session was reaped between polls | Row becomes "Session no longer exists" (read-only, text not color) and is removed on next merge; Open replaced by nothing |

### 1.3 Error and edge states

| Scenario | Operator sees | Exit path |
|---|---|---|
| Session list poll fails (network/server) | Rows keep last-known data with banner "Couldn't refresh background sessions. Showing data from {time}." and a **Retry** button | Retry; auto-retry on next 30 s poll |
| Offline | Shared offline banner (UX-S7); Open buttons remain (navigation works offline within the SPA) | Auto-recover on reconnect |
| Flag off | Legacy empty-state behavior (plan 1.4) | None needed |
| Failure row for a session whose outcome is no longer a failure | Pinned row moves to "No failures reported" on next merge, not under the cursor (follows "N new" rule) | - |
| Hidden session with unknown role | Row labeled "background", role chip text "other" | Open |
| Stopped hidden session whose outcome is unknown (no verdict recorded, or diagnose role) | Chip "Stopped, outcome unknown"; never "Finished" | Open |
| >100 rows | Capped at 100 with a "Show all (N)" button; never windowed/virtualized (a screen reader must be able to reach every row; section 11.2) | Show all |
| Row has failure and a pending decision | The Background row shows the failure chip plus a link chip "Needs you (1)" that moves focus to the item in Needs you; the controls live only in Needs you (no double display, section 1.4) | Open the Needs you item |
| First load, no cached data | "Loading background sessions..." (`role=status`, `aria-busy`), shown only if loading lasts longer than 150 ms; no skeleton flash, no spinner-only state; after 8 s becomes "Still loading. Retry" with a Retry button | Retry |
| Server unreachable while the browser is online | "Reconnecting to the server. Showing last known state." (`role=status`); after 30 s "Can't reach the server. Retry" with a Retry button; rows stay, Open stays enabled (section 9) | Retry |

---

### 1.4 Feed governance (stable order, failures, badge, acknowledge, growth, first run, no duplicates)

**Engaged-card freeze and "N new".** The same freeze applies to Background and Needs you. "Engaged" is defined once, in the section 7.1 table (programmatic focus never engages): while any row is engaged, no row is inserted, removed or reordered. Arrivals wait behind "N new" (informational) or the failure strip (failures). Resolved items leave on the 7.1 lifecycle schedule.

**Failures are never behind a count-only pill.** A new failure or needs-human session produces: (1) an entry in the sticky failure strip at the top of the tab, naming the session ("review:pr-1042 failed 1m ago", **Show**, **Open**; up to 3 named entries, then "N more failures"); (2) one polite announcement "1 background session needs attention" (count only, no focus move); (3) a bell-badge increment (rule below). **Show** scrolls to the row and moves focus to it; this is the operator's own action. The row joins the pinned "Needs attention" group at the next unengaged merge. The strip, the "N new" pill and the connectivity banner all render in the one reserved status slot of section 7.1, so showing or hiding any of them shifts no row.

**Bell-badge rule.** The bell badge is the sum of: unread actionable Needs-you items (questions, approvals, decisions), escalated peer-help requests, Unconfirmed answers (rows 3, 6 and 16 of section 3.1; a plain `sent` answer (row 2) under 60 s is not actionable and is not counted, but `SEND_INDETERMINATE` (row 6) is known-uncertain and counts immediately, at 5 s as at 60 s), and **unacknowledged Background exceptions** (a `Failed` or needs-human row not yet opened or marked reviewed). It never counts `Finished`, `Stopped, outcome unknown`, running, departed, un-escalated peer help or Reminders. The Background tab's own badge is only the exceptions part, and its accessible name is "Background, N needs attention" (omitted when 0), not the session total: a total that changes on every poll would re-announce on every change, so the total lives only in the roll-up text.

**Acknowledge or clear permanent rows.** A `Stopped, outcome unknown` row can never become anything else on its own (diagnose sessions never get an outcome), so each row has **Mark reviewed** (a text button, 44px). Reviewed rows collapse into a "Reviewed (M)" disclosure and the roll-up reads "11 background sessions, no failures reported (3 reviewed). Last activity 3m ago." A "Mark all reviewed" button appears on the group when 2 or more rows are unreviewed. A `Failed` row can be marked reviewed too (it leaves the badge count and stays pinned in a "Reviewed failures" group for 24 h). Reviewed state is stored in the browser (`localStorage`, guarded by try/catch; when storage is unavailable the marks last for the page session and the section says "Marks are not saved in this browser"). **Accepted limitation (decided, not deferred):** marks are per browser, so a phone can still badge an exception that the desktop already reviewed; a server write for a UI convenience is not worth a new RPC in v1. The cost is measured with the client-event counter `background_reviewed_mark_total{storage="local|session"}` (plan Task 1.8c); server-side marks are added only if the owner reports phantom badges during dogfood (owner-triggered, never automatic).

**Growth cap.** Rows older than 24 h with state `Finished`, `Stopped, outcome unknown` or `Reviewed` collapse into "Older (47)" (a disclosure, collapsed by default); within the open list at most 100 rows render with "Show all (N)". The Older disclosure's count is in its button text, not announced live. Render cost is bounded by test at 500 rows (Task 1.8d).

**First-run explanation.** The first time the Background tab opens in a browser it shows a dismissible note above the roll-up: "Background shows review, triage and diagnose sessions that run without a window. 'No failures reported' means no failure was recorded, not that everything succeeded." with **Got it**. The note is not repeated after dismissal; if storage is unavailable it shows once per page load.

**No double display.** One fact renders on one surface. Actionable items (questions, approvals, backlog decisions) always live in **Needs you** and carry the controls, whichever session they came from. Failure and needs-human records for **hidden** sessions live only in the Background "Needs attention" group; they are not repeated in Recent activity. A Background row whose session has an actionable item shows the link chip "Needs you (1)" and no controls. A test asserts that no notification id renders in both tabs (UX-AC72).

**Row click semantics (all lists).** A row is not a link. Clicking or tapping row text toggles its details (the disclosure button is the row title; Enter and Space on it do the same). Navigation happens only through an explicit **Open** button. A control inside a row never also toggles the row. Rows have no hover-only affordances.

## 2. UX-S2 - Inline action row on notification items

### 2.1 Wireframes

Desktop (expanded item, one row of buttons; descriptors from server):

```
 +-------------------------------------------------------+
 | (?) Needs approval     review:pr-1042      4m ago      |
 | Bash: git push origin feature/x                        |
 | [ Approve ] [ Deny ]                   [ More v ] [ Open ] |
 +-------------------------------------------------------+
 Backlog decision:
 | (?) Decide: "Fix flaky test" review verdict    9m ago  |
 | Reviewer asks for changes. 2 rework rounds used.       |
 | [ Send back ] [ Merge ] [ Mark done ]       [ Open ]   |
 Desktop-only action seen from the phone listener:
 | [ Answer from desktop ] (aria-disabled)  [ Open session ]|
```

`More` (desktop only: width >= 900px and the local listener) is a menu button holding **Create rule...**, the existing scoped-approval action. It is never in the primary row, never rendered below 900px or on `:8444`, and opens the inline confirm "Create a rule that auto-approves {tool and pattern}?" with **Create rule** and **Back** (the confirm is its guard; it has no commit delay). Reason: a rule-creating control beside Deny at an 8px gap is a wider-scope, hard-to-reverse approval one slip away from the safe action.

Mobile (390px): buttons stack vertically, full width, 44px high, 8px gap, except that Deny and Approve are separated by a 24px gap (order: Deny, gap, Approve, Open) so a thumb aimed at one cannot land on the other. Open is the last, secondary (outline) button. Deny reason field appears below on tap of Deny (16px font).

The full approval text is always shown (section 2.4).

### 2.2 Flow

1. Entry: unread actionable item in Needs you (or in a session/backlog banner reusing the same `ActionRow`).
2. Happy path: single tap on a primary action. Button disables immediately (double-tap guard) and shows inline "Approving..." (`aria-busy`). For the three consequential single taps (**Approve**, **Send back**, **Mark done**) the action is held for a 3-second commit delay with an **Undo** button ("Approving in 3s. Undo"; section 2.4); every other action dispatches at once. After the server confirms, the row shows "Approved" using the server-returned value and the polite region says "Approved". The row then stays in place until the section 7.1 lifecycle table removes it (nothing engaged for 750 ms), and only at that moment, if focus was inside it, does focus move to the next item, without engaging it.
3. Deny: tap **Deny** opens an optional one-line reason field with **Send denial** and **Back**; reason is optional, so a second tap is never mandatory beyond the reason-field confirm. Deny dispatches at once (it is the safe default and has no commit delay).
4. Merge / other `requires_confirm` actions: inline two-step ("Merge PR #901?" **Confirm merge** / **Back**) inside the row, not a modal. Focus moves to Confirm merge. Merge has the confirm instead of the commit delay.
5. Exit: **Open** always present (descriptor fallback `OPEN`); **Back** on any inline sub-step restores the row with the draft preserved; Esc collapses.

### 2.3 Error and edge states (maps ActionOutcome, ADR-002)

| Outcome / scenario (R-row) | Operator sees | Exit path |
|---|---|---|
| `APPLIED` | "Approved" / "Decided: {choice}"; for a live-pane answer the delivery line of section 3.1 (row 2 "Sent - waiting for the agent"), never "Answered" before acknowledgement | none |
| `ALREADY_RESOLVED` (R2, R8) | "Already handled: {value}". No toast, no `role="alert"` | none (moves to Recent) |
| `STALE` (R1, R9) | Read-only "Already handled in the session. No action needed." or, for approvals auto-handled, "Handled automatically by rule {name}" | Open |
| `NOT_SENT` (R6) | "Not sent. Nothing was typed into the session. Retry" (reason variants: `human_active` -> "Not sent: someone is typing in that session. Retry when they are done."). Record stays `delivery_failed` | **Retry** (safe for this outcome only: nothing was written), **Open session** |
| `SEND_INDETERMINATE` (R6) | "Sent, but we couldn't confirm it arrived. Check the session before retrying." Record stays `sent` with the Unconfirmed chip, not shown as answered | **Open session** (primary), **Retry** only after an inline "Retry anyway?" confirm because the key may already have been typed |
| Request timeout (no response in 10 s) | "Still sending... this is taking longer than usual." (text, no live announcement); at 30 s: "No response from the server. The action may or may not have been applied. Open the session to check." with **Retry** (same idempotency key, so a second write is impossible) | Retry / Open session |
| Reconnecting (browser online, server stream down) | Banner "Reconnecting to the server. Showing last known state."; action controls `aria-disabled` with reason "Reconnecting" | auto-recover; after 30 s "Can't reach the server. Retry" |
| `RATE_LIMITED` | "Too many actions. Try again in {n}s." with countdown text, Retry enabled when 0 | Retry |
| `FORBIDDEN` / Tier B on remote (R-adjacent, ADR-004) | `aria-disabled` button, description "Answer from desktop", plus **Open session** | Open session |
| `FLAG_OFF` | Row degrades to legacy actions / Open | - |
| `INVALID` | "That action isn't valid anymore. Refresh." **Refresh** | Refresh refetches descriptors |
| Transient network failure (R5), request rejected before the server accepted it | Inline `InlineError type="transient"`: "Couldn't send your action. Retry". Idempotency key reused on Retry so no duplicate | Retry / Dismiss error |
| Duplicate tap (R8) | Second result renders same final state; no second keystroke | - |
| Unknown action kind | Rendered as **Open** only | Open |
| Session gone, no durable record (R4), and nothing was sent | Read-only "Session ended before this was answered." (never shown for a row that is `sent`, Unconfirmed or `delivery_failed(still_showing)`; those show 3.1 row 17) | Open; Dismiss (non-actionable only) |
| Actionable unread item | No dismiss X; "Mark all read" never clears unread actionable (revamp Surface 11) | Resolve via action or Open |

---

### 2.4 Consequential taps, approval text, and the explicit tier list

**Commit delay with Undo (applies to Approve, Send back, Mark done only).** The tap starts a 3-second hold: the pressed button becomes `aria-disabled` "Approving in 3s" and focus moves to the adjacent **Undo** button (44px; the tap was the operator's own action, so this move is allowed). Undo returns the row to its prior state with nothing sent. The countdown is visible text, not a live region, and is not announced each second (section 11.1); the polite region says "Approval pending" once, then "Approved" or "Approval undone". **Leaving cancels, never commits.** If the operator leaves while a hold runs (panel closed, a click outside the desktop popover, route change, Back, tab switch, `visibilitychange` to hidden, `pagehide`, or the connection dropping so the browser goes offline), the hold is cancelled and nothing is sent: an irreversible action is never committed by a lifecycle event the operator may not have meant (an app switch mid-hold). On return the card is back in its prior state with the line "Approval cancelled when you left. Tap Approve to try again." and the polite region said "Approval undone" once. The restored card is an unresolved actionable item in Needs you, so the bell badge still counts it (UX-AC63, T10): a phone operator who locks the screen after tapping Approve sees the badge, and the agent's blocked state, on return rather than a silent drop. The cost, an operator who taps and immediately closes the panel must tap again, is accepted as the safer default; this replaces the earlier flush-on-exit rule. Why these three: Approve lets an agent run a command, Send back restarts a review, Mark done ends an item; none can be reversed from the pane after dispatch. Answers to questions do not use a hold (the option the operator pressed is the only input and the delivery line shows what was sent); Deny does not (denial is the safe default); Merge uses its inline confirm. The round-trip SLO (p95 under 1 s) is measured from dispatch, not from the tap.

**Approval text shown in full.** The approval row renders the complete tool name and command or argument text: wrapped (`white-space: pre-wrap; overflow-wrap: anywhere`), never ellipsized. A text longer than 8 lines or 400 characters renders in a scrollable region (`max-height: 12em`, `tabindex="0"`, `aria-label="Command, scrollable"`) with the character count ("1,204 characters") and a **Copy** button, and the Approve button is not hidden behind it. A tool input the server cannot render as text shows "Can't show this command. Open session to review it." and Approve is replaced by Open (fail closed).

**Explicit tier list (ADR-004 Tier A is data-only, Tier B writes bytes into a live pane).**

| Action (`ActionKind` or control) | Tier | Remote listener `:8444` | Commit delay / confirm |
|---|---|---|---|
| `OPEN` | A | available | none |
| `APPROVE` | A | available (parity with today) | commit delay |
| `DENY` | A | available | none (optional reason) |
| `BACKLOG_DECISION` send back | A | available | commit delay |
| `BACKLOG_DECISION` mark done | A | available | commit delay |
| `BACKLOG_DECISION` merge | A | available | inline confirm |
| `ANSWER_GUIDANCE` (agent-created request, data write) | A | available | none |
| `ANSWER_QUESTION`, route `resume` / `read_only` / durable record | A | available | none |
| `CANCEL_WAKE` (Cancel prompt), `CANCEL_PEER_HELP` (Cancel request) | A | available | inline confirm |
| Remind me create / Undo | A | available | none (Undo 10 s) |
| Resume scheduled prompts (flag write) | local only | shown disabled: "Resume from desktop" | inline confirm |
| `Create rule` (existing approval-rule control, not an `ActionKind`) | local only | not rendered | desktop only (>= 900px), inside the `More` menu, inline confirm |
| `ANSWER_QUESTION`, route `live_keystroke` | **B** | `aria-disabled`, "Answer from desktop" (until Spike S6 promotes it) | none |

**Shipped phone behaviour for `live_keystroke` questions (v1, decided).** On `:8444` a question whose route is `live_keystroke` renders its text and options with every option button `aria-disabled` ("Answer from desktop") and an enabled **Open session**; nothing else is offered (no Submit, no durable-record fallback, because a live dialog must be answered where it is). The phone user can read the question and open the session but cannot answer it from the pane until Spike S6 promotes Tier B. Such questions count as resolved in-session in the adoption share by design (requirements, Adoption targets); what a phone can answer in v1 is approvals, backlog decisions, agent-created guidance and durable-route questions.

**How an away operator learns something needs them.** This project makes the answer possible from a phone; it does not itself reach the operator who is not looking at the app. That depends on the existing, out-of-scope channels (`slack-review-notifications`, `web-push-notifications`), which carry `INPUT_REQUIRED` and `CONFIRMATION_NEEDED` notifications that these actions resolve. Whether those channels are delivering today is not verified by this design (UNVERIFIED), so the away-operator journey is a stated dependency, not a feature: if neither channel is live, the operator learns of a pending item only by seeing the bell badge when they next open the app. Success metric A1 (requirements) is the test of whether the in-pane path is used at all.

## 3. UX-S3 - GuidanceRequestCard family

One component family (`GuidanceQuestionForm` shared by `GuidanceRequestPanel` and `ActionRow`). Collapsed one-liner with `Answer ▸` (a `<button aria-expanded aria-controls>`), expanded form `role="form"` `aria-label="Answer: <question>"`.

### 3.1 Wireframes

Collapsed:
```
 (?) review:pr-1042 asked 4m ago: "Which cache should we use?"   [ Answer ▸ ]
```

Single-select (2-4 options; **option buttons, one activation sends**):
```
 Which cache should we use?                  <- <p id> naming a role="group" (aria-labelledby)
 [ Redis      - fast, in-memory ]            <- <button>, 44px, label + description
 [ Memcached                    ]
 [ Open session ]                       Esc collapse
 Delivery: (none until sent)
```
**What sends and what only moves focus (B1 decision).** Single-select options are plain `<button>` elements inside a labelled `role="group"`, not a `radiogroup`. A radiogroup would let arrow keys change the selection, and for a keyboard or screen-reader user browsing the options that would send an irreversible answer. Therefore: **sends** = click, tap, Enter or Space on a focused option button, once per card (the card then locks: all options `aria-disabled` while the delivery line shows the state). **Only moves focus** = Tab / Shift+Tab, the Up/Down arrows (ordinary scrolling, no roving behaviour on this group), and digit keys 1-9 (focus the Nth option; they never send; section 7.1). **Free-text "Other" exists only on routes that can carry text** (decided): an agent-created (`agent_mcp`) durable-record row, whose card adds `Other: [______________] [ Send ]` (16px; Send is explicit) under the options, and a `hook_reply` row if Spike S1 passes. It is absent for `live_keystroke` and hook-mirrored rows, because the write is a single digit and typed text cannot be delivered; those cards show only the options and **Open session** (a question that needs a typed reply is answered in the session). A free-text field requires **Send** (Cmd/Ctrl+Enter in a multi-line field; Enter in a single-line field). The ARIA matches: buttons announce as buttons ("Redis, button"), never as "radio button, 1 of 2", so no one is told they are making a selection that can still be changed. Multi-select keeps real checkboxes (selection only) plus an explicit Submit.

Multi-select:
```
 Which regions? (choose any)                 <- role=group, legend
 [x] us-east   [ ] eu-west   [x] ap-south    <- checkboxes, each row >= 44px
 [ Submit answers ]   [ Clear ]
 3 selected                                  <- visible count; announced once on pause, not per toggle
```

Multi-question (stacked groups, one Submit):
```
 Question 1 of 2: Cache?   (o) Redis ( ) Memcached
 Question 2 of 2: Regions? [x] A [ ] B [x] C
 [ Submit answers ]  disabled until all required groups answered; helper "Answer question 2 to continue"
```

More than 8 options: native `<select>` per question with the question as its `<label>`.

Read-only shapes (unmeasured harness, archived, S1c unmeasured multi-select): question text and options rendered as text list, no controls, note "Answer in the session" + **Open session**.

Delivery-state line (below the form, plain text with an icon, never color only; it is **not** a live region, because the single global polite region announces the short result and a second live region on the card would announce every change twice, section 11.1). It shows exactly one of these 17 settled-state strings (UX-AC23; 18 literal strings counting the `human_active` variant of row 5); transient and reason strings are a separate enumerated set of 6 listed after the table; the first column is the state it renders:

| # | State | Copy | Actions |
|---|---|---|---|
| 1 | `pending` | "Sending..." | none |
| 2 | `sent` (under 60 s) | "Sent - waiting for the agent" | none |
| 3 | `sent`, unacknowledged at 60 s with an indeterminate capture (**Unconfirmed**) | "Sent, but the agent hasn't confirmed yet. Open session." with chip "Unconfirmed". **Stays in Needs you** with a warning icon (B3) | Open session, Mark as handled (inline confirm) |
| 4 | `acknowledged` | "Answered: {value}" | none |
| 5 | `delivery_failed` after `NOT_SENT` (announced once as "Answer not delivered") | "Not sent. Nothing was typed into the session. Retry" (`human_active`: "Not sent: someone is typing in that session. Retry when they are done.") | Retry, Open session |
| 6 | `sent` after `SEND_INDETERMINATE` (carries the **Unconfirmed** chip, like row 3) | "Sent, but we couldn't confirm it arrived. Check the session before retrying." | Open session, Retry anyway (confirm) |
| 7 | client-side failure before the server accepted it | "Couldn't send your answer. Retry" | Retry, Dismiss error |
| 8 | `saved_for_later` (agent-created request, session stopped, not archived) | "Your answer is saved. The agent can read it once you resume the session." (the agent pulls it with the existing `get_guidance_request` after a resume; nothing is pushed) | Open session |
| 9 | `saved_for_later` (archived) | "This session is archived. Your answer will be saved but not delivered unless you resume it." | Open session |
| 10 | session gone, no durable row, and no keystroke was written (states `pending`, `delivery_failed` after `NOT_SENT`, or not yet answered) | "Session ended before this was answered." | Open session, Dismiss |
| 11 | `stale`, dialog gone | "Already handled in the session. No action needed." Shown only when a fresh capture shows no dialog for this question (dialog seen gone, or the session ended); never because a timer elapsed (UX-AC59) | Open session |
| 12 | `stale`, a different dialog is showing | "The session is now asking something else. Open session to answer it." Shown when `DialogMatch` fails but the pane shows another dialog; never "Already handled", because nothing was handled | Open session |
| 13 | answered in another tab or channel | "Answered elsewhere: {value}" | none |
| 14 | multi-question aborted midway | "Part of your answer was sent, then the prompt changed. Open session to finish." Submit disabled | Open session, Dismiss (confirm) |
| 15 | Tier B on the remote listener | "Answer from desktop" (description of the disabled option buttons) | Open session |
| 16 | `delivery_failed(reason=still_showing)` (announced once as "Answer sent, question still showing"): at 60 s the row's own dialog is still on screen. A digit **was** written, so this is not row 5 | "Sent, but the question is still showing. Check the session before retrying." Stays in Needs you and in the badge | Open session (primary), Retry anyway (confirm; re-verifies `DialogMatch` and writes one digit) |
| 17 | the session ended while the row was `sent`, Unconfirmed (row 3 or 6) or row 16 | "Sent, but the session ended before confirming it." Moves to Recent activity; leaves the badge | Open session |

**Transient and reason strings (6; not delivery states, so outside the 17).** They may appear in or beside the line while a request is in flight or the controls are disabled, and are never counted among the settled states: "Still sending... this is taking longer than usual." (10 s, section 2.3); "No response from the server. The action may or may not have been applied. Open the session to check." (30 s, section 2.3); "Already answered - reload to see the answer" (section 3.3 fallback when the API value is missing); and the `aria-disabled` reasons "Offline", "Reconnecting" and "Updating..." (sections 2.3 and 7).

**Honest delivery copy.** "Sent" means the keystroke was written. "Answered: {value}" appears only in state `acknowledged` (or when the route is a durable record the agent reads later). The polite announcement at the `sent` transition is "Answer sent", never "Answer delivered" or "Answered". Asynchronous transitions are announced once each (section 11.1).

A question open longer than 10 minutes changes nothing in this table: time alone never changes the card, and the dialog is re-verified when the operator answers. `saved_for_later` (rows 8 and 9) applies only to agent-created (`agent_mcp`) guidance requests; a hook-mirrored question dies with its session (the `GuidanceSessionReaper` cancels it, plan Story 3.3) and shows row 10 if nothing was sent, row 17 if an answer had been sent. Saved answers are listed in section 3.4.

**Acknowledgement signal (B3; ADR-005 amended).** `sent` becomes `acknowledged` on the first of: (a) a `PostToolUse` hook receipt for `AskUserQuestion` from that session after `sent_at` (primary; exists only if Spike S7 item (6) shows the injected matcher covers `AskUserQuestion`, otherwise Task 3.4e adds it); (b) two consecutive fresh captures 3 s apart with neither `DialogMatch` for this row's question nor `session.PaneShowsDialog` (fallback, and the only signal when (a) is unavailable). **The 60 s decision (single source).** At 60 s with no acknowledgement the server takes one more fresh capture under the write lease and the result depends on the row it finds:

| Row at 60 s | Own dialog still on screen | Dialog gone | Capture error or indeterminate layout |
|---|---|---|---|
| 2 `sent` | row 16 (`delivery_failed(still_showing)`) | row 4 (acknowledged by (b)) | row 3 (Unconfirmed) |
| 6 `sent` after `SEND_INDETERMINATE` | row 16 | row 4 | stays row 6 (already Unconfirmed; its copy is more specific) |
| 5 `delivery_failed` after `NOT_SENT` | no 60 s capture: nothing was written, so there is nothing to confirm | | |

Row 5 is therefore the only "Nothing was typed" string, and only for `NOT_SENT`; the 60 s path never produces it (a digit was written). An answer in state `sent` (row 2, under 60 s, no `SEND_INDETERMINATE`) is not actionable and is not counted in the bell badge; it counts as soon as it becomes row 3 or 16, and a `SEND_INDETERMINATE` answer (row 6) counts from the moment it is written, not at 60 s. Retry from row 16 sits behind the same "Retry anyway?" confirm as row 6. An Unconfirmed row (3 or 6) stays in Needs you and counts in the bell badge until it is acknowledged (it then turns into row 4 by itself), the operator marks it handled, or the session ends (row 17). An unacknowledged answer is never demoted to Recent activity, because that would hide a possibly stalled agent. **Mark as handled** on an Unconfirmed row is a browser-local mark (the same storage and the same limits as the Background "Mark reviewed", section 1.4): it moves the row to Recent activity and removes it from this browser's badge count without any server write, so it cannot be mistaken for an acknowledgement by the agent.

Mobile: options stacked, each option row 44px+ tall via padding, full-width Submit pinned above the on-screen keyboard (sticky action bar using `visualViewport`), `scrollIntoView({block:"nearest"})` on free-text focus, `enterkeyhint="send"` on single-line only.

### 3.2 Flow

1. Entry: new question raises a bell badge increment and polite announcement "1 item needs a decision" (count only); appears in Needs you pinned tier; or via session-view banner (never steals terminal focus).
2. Happy path (single-select): Enter on item expands (the first item is pre-expanded on panel open but neither engaged nor digit-armed until the operator Tabs onto it or clicks it; Enter on its header collapses it, section 7.1) -> tap or press Enter on an option button -> row shows "Sending..." -> "Sent - waiting for the agent" with the polite status "Answer sent" -> "Answered: Redis" once acknowledged (not announced). The row leaves, and focus advances if it was inside the row, on the section 7.1 lifecycle schedule (the next item is focused, not engaged: digit shortcuts stay inert). Interaction count after pane open: 1.
3. Happy path (multi-select/multi-question/free text): select, then Submit once.
4. Exit: Esc collapses and preserves draft; **Clear** clears selections only (not the question); **Open session** always available; drafts are kept per question id in `sessionStorage` (inside try/catch, in memory when storage is unavailable) until the answer is sent or dismissed, so closing the panel or reloading the tab loses nothing.

### 3.3 Error and edge states (research/ux.md 5, R1-R17)

| Scenario | Operator sees | Exit path |
|---|---|---|
| Stale: agent moved on (R1) | Card flips read-only "Already handled in the session. No action needed." when the dialog is gone, or "The session is now asking something else. Open session to answer it." when the pane shows a different dialog (delivery-line rows 11 and 12) | Open session; moves to Recent |
| Answered elsewhere (R2) | "Answered elsewhere: {value}" from API value; fallback "Already answered - reload to see the answer" | Reload only if value missing |
| Session stopped, durable (R3a) | saved_for_later copy (row 8); Submit stays enabled | Open session to resume |
| Session archived (R3b) | archived copy; nothing is revived (`noarchivedrevival`) | Open session |
| Session gone, no durable row (R4) | Row 10 "Session ended before this was answered." if nothing was sent; row 17 "Sent, but the session ended before confirming it." if an answer had been sent | Open; Dismiss (row 10 only) |
| Transient send failure (R5) | Inline error, draft preserved | Retry / Dismiss |
| Delivery failure to live pane, nothing written (`NOT_SENT`, R6) | Delivery-line row 5. Not shown as answered | Retry / Open session |
| Delivery uncertain (`SEND_INDETERMINATE`, R6) | Delivery-line row 6. Not shown as answered | Open session / Retry anyway (confirm) |
| Unacknowledged (R7) | "Sent - waiting for the agent" up to 60 s, then the decision table in 3.1: row 3 (**Unconfirmed**, in Needs you), row 4, or, if the dialog is still on screen, row 16 (never row 5: a digit was written) | Open session / Mark as handled / Retry anyway |
| Duplicate submit (R8) | Idempotent; same final state | - |
| Terminal-answer race (R9) | Card flips to handled via live update; polite announcement; focus untouched | - |
| Offline (R11) / Reconnecting | Offline or Reconnecting banner (section 9); options and Submit `aria-disabled` with description "Offline" or "Reconnecting"; drafts preserved | auto-retry on reconnect |
| >8 options (R16) | `<select>` | - |
| Unrenderable options (R17) | Text + "View full prompt in session" link | Open session |
| Layout-drift canary tripped (plan Task 3.3h) on a hook-mirrored question | Read-only shape: question and options as text, "Answer in the session", no answer control (unless `hook_reply` is enabled, then that route answers) | Open session |
| Multi-question aborted midway (plan 3.5: DialogMatch fails after step 1) | "Part of your answer was sent, then the prompt changed. Open session to finish." state `sent`-aborted; Submit disabled | Open session (FLAG: no retry path defined, see 8.2) |
| Multi-select unmeasured (S1c fail) | Read-only options + Open session | Open session |
| Validation: no option chosen on Submit | Inline error adjacent to group, text "Choose at least one option" (`aria-describedby`, `aria-invalid`); focus moves to first invalid group | Fix and resubmit |
| Option label very long (>80 chars) | Wrap, never truncate without a title/expand | - |

---

### 3.4 Saved answers

Answers stored for a stopped or archived session (delivery-line rows 8 and 9) are listed in Recent activity under a disclosure "Saved for later (N)" (`<button aria-expanded>`, collapsed by default, count in the text). Each row shows the session, the question, the saved answer and the session state ("stopped" or "archived"), with **Open session** (the only way to resume it). There is no discard in v1: a saved answer for a session that is later deleted is cancelled with it. The list is read-only; it is not counted in the bell badge.

## 4. UX-S4 - Peer-help rows

Peer help is informational for the operator by default (research/ux.md JTBD 7): "agent A asked agent B for help". It escalates to Needs you only on timeout or failure.

### 4.1 Wireframe

```
 Background feed / Recent activity (informational):
 +-------------------------------------------------------+
 | (people) HELP  review:pr-1042 asked diagnose:abc       |
 | "Which approach for the retry logic?"      waiting 3m  |
 | State: Waiting for diagnose:abc                        |
 |                          [ Open asker ] [ Open peer ]  |
 +-------------------------------------------------------+
 Escalated (timeout/failure) in Needs you:
 | (!) HELP STALLED  review:pr-1042 asked diagnose:abc    |
 | No reply in 10m. diagnose:abc is stopped.              |
 | [ Answer for {peer} ] [ Cancel request ] [ Open ]        |
```
Rows show asker, answerer, question, age, state: waiting / answered by {session} / failed ("peer unavailable", "rejected: archived or cross-workspace") / cancelled.

### 4.2 Flow
- Entry: appears when a session calls `request_help` with a named peer and the request is accepted. Not counted in the Needs you badge until escalated. A request rejected at call time (self-target, archived, one-shot, cross-workspace) returns a typed error to the asker and creates no row, so there is nothing to render.
- Happy path: answered by peer -> row shows "Answered by diagnose:abc" with provenance "from session B", moves to Recent. No operator action.
- Escalation: 10 minutes without an answer, or the answerer stopped/archived/deleted (emitted by the `PeerHelpEscalator`, plan Story 4.11) -> row moves to Needs you with the three actions. **Answer for {peer}** opens the UX-S3 card (operator as answerer, reuses the guidance store; the asker agent sees `from=operator on-behalf-of={peer}`); **Cancel request** (Tier A, inline confirm) cancels it and notifies the asker via its guidance_answered wake with "cancelled".
- Escalation presentation: the escalation raises the bell badge, produces one polite announcement "1 item needs a decision" (count only, the same string as any new decision), and does **not** move focus. If a card is engaged the new row waits behind the Needs-you "N new" pill (section 7.1); otherwise it is inserted in time order within the pinned tier. The row's first line reads "{asker} asked {peer}: no reply in 10m" or "Peer is stopped", so it is distinguishable from a question the operator's own sessions asked.
- Exit: Open asker / Open peer; Cancel request; row auto-clears to Recent on any terminal state.

### 4.3 Error and edge states

| Scenario | Operator sees | Exit |
|---|---|---|
| Request accepted, then undeliverable (answerer archived, deleted or cross-workspace after creation) | Row state "Not sent: {reason}"; informational | Open asker |
| Peer stopped | "Peer is stopped. Not delivered unless resumed." Escalates | Answer for {peer} / Cancel request / Open |
| Wrong answerer attempts answer (FORBIDDEN) | Not operator-visible except audit; no UI change | - |
| Operator answers after peer already answered | "Already answered by {session}: {value}" | none |
| Cancel fails (transient) | Inline error, Retry | Retry / Dismiss |
| Flag off | Rows hidden; existing guidance behavior | - |

---

## 5. UX-S5 - "Scheduled prompts" section

### 5.1 Wireframe
```
 Scheduled prompts (2)                                        [v]  <- <button aria-expanded>
 +-------------------------------------------------------+
 | review:pr-1042   prompts again when PR #901 CI passes  |
 | since 12m ago                      next check: event   |
 |                                     [ Cancel prompt ]  |
 | diagnose:abc     prompts again in 25m (13:40)          |
 |                                     [ Cancel prompt ]  |
 +-------------------------------------------------------+
 Kill switch: banner "Scheduled prompts paused. Agents will not be prompted automatically."  [ Open flag settings ]
```
Columns/fields per row: owner, condition (delay / cron / PR / CI / backlog event / session state / human answer), since, next fire. Condition text is plain language, never raw JSON. Operator reminders are listed here too, labelled "Reminder (you)" and with an **Undo** control for 10 s and then **Cancel reminder**, so agent-owned prompts and the operator's own reminders are never confused. A cron prompt states its timezone in the row ("every day 09:00 America/Chicago") so the displayed time is never ambiguous (plan Task 4.2d).

### 5.2 Flow
- Entry: Background tab, section under the feed; present only when the flag is on (otherwise hidden).
- Happy path: **Cancel prompt** -> inline confirm in row ("Cancel this scheduled prompt? The agent will not be re-prompted." **Yes, cancel** / **Keep**) -> row shows "Cancelled" then leaves list; status "Prompt cancelled".
- Exit: collapse; Keep; navigate to owner session by clicking owner name (link).

### 5.3 Error and edge states

| Scenario | Operator sees | Exit |
|---|---|---|
| None pending | "No scheduled prompts. Agents that ask to be prompted again will appear here." | - |
| Cancel race: prompt fired meanwhile | "Already fired at {time}." read-only | none |
| Cancel failed (transient) | Inline "Couldn't cancel. Retry" | Retry / Dismiss |
| `agent_wake_triggers` turned off at runtime | Banner "Scheduled prompts paused", Cancel prompt still works, rows labelled "Paused" | Resume scheduled prompts (inline confirm, local listener) / Open flag settings link |
| Prompt missed after a restart (alert condition) | Row state "Missed: {reason}" with warning icon, listed as exception | Cancel prompt / Open owner |
| Owner session archived | Row "Owner archived. This prompt will not revive it." | Cancel prompt |
| List fetch failed | "Couldn't load scheduled prompts. Retry" | Retry |
| Many rows (>20) | Cap 20 with "Show all (N)" | Show all |

---

## 6. UX-S6 - "Remind me" affordance

Scope: informational Background rows only (finished review, stopped diagnose). Never on blocking questions or approvals (research/ux.md rec 4). **In scope (decided).** The default presets below ship; the owner may change the preset values in config, which is not a gate. It is scheduled in the last wave and is optional-to-order, so a late finish delays nothing else.

### 6.1 Wireframe
```
 [ Remind me v ]  -> menu (role=menu, arrow keys):
    In 1 hour / Tomorrow 9:00 / Pick time...
 Mobile: the same list expands inline below the button (not a sheet), items 44px.
 After pick: row shows "Reminder set for 14:10"  [ Undo ]
```

### 6.2 Flow
Entry: Remind on a row. Happy path: choose a preset -> trigger `notify_operator` created -> inline confirmation "Reminder set for {time}" + **Undo** for 10 s, persisted afterwards in Scheduled prompts. At fire time an INFO-class operator notification appears linking to the row's session; it is emitted as a system-level notification with no session binding (`metadata.target_session_uuid`), because an INFO notification bound to a hidden session is suppressed by the delivery gate. Exit: Esc closes menu with focus back on the button; Undo cancels.

### 6.3 Error and edge states
| Scenario | Operator sees | Exit |
|---|---|---|
| Create fails | "Couldn't set the reminder. Retry" | Retry |
| Flag off / cap exceeded (`cap_exceeded`) | Button hidden when off; on cap "Too many pending reminders. Cancel one in Scheduled prompts." with link | Scheduled prompts |
| Picked time in past | Inline "Choose a time in the future" | re-pick |
| Session reaped before fire | Notification says "Session no longer exists" and offers no Open | none |
| Duplicate tap | Idempotent single trigger | - |

---

## 7. Keyboard and ARIA model (all surfaces)

One keyboard model, stated once. WCAG 2.1.4 (character key shortcuts): the J/K and digit shortcuts are active only while focus is inside the panel list and never on the document; nothing is bound while the panel is closed; there is no global shortcut that opens it.

| Key | Where | Effect | Never |
|---|---|---|---|
| Tab / Shift+Tab | the panel | native order: tablist, item headers, then inside an expanded card the options, Submit/Send, Clear, and on an approval row Approve, Deny, More, Open | trap focus |
| Enter / Space / Down; Up; Home / End | the `More` menu button and its open menu (`role="menu"`) | Enter, Space or Down opens it and focuses the first item; Up/Down/Home/End move between items; Enter activates **Create rule...** (which opens its inline confirm) | activate anything without the confirm; trap focus |
| Left / Right / Home / End | tablist | roving tabindex; `aria-selected` follows | send anything |
| Up / Down and J / K (identical) | an item header | move focus to the previous or next item header | send, select or expand; inside an expanded card, Up/Down is ordinary scrolling |
| Enter / Space | an item header | expand or collapse the card (operator engagement, 7.1) | |
| Enter / Space | a focused single-select option button | send once and lock the card | |
| Enter / Cmd+Enter | single-line field / multi-line field | Send (Cmd or Ctrl+Enter for multi-line) | |
| 1-9 | inside a digit-armed card (7.1) | focus the Nth option | send |
| Esc | innermost layer first | (1) an open sub-step, the `More` menu or the Remind-me list: Back to the row (focus returns to the control that opened it), draft kept; (2) an expanded card: collapse, draft kept; (3) the panel: close and return focus to the bell | submit |

Focus after the operator's own actions: activating "N new" merges the rows and moves focus to the first merged row's header; **Mark reviewed** moves focus to the next row in the group, else the group heading (`tabindex="-1"`), else the tab; **Mark all reviewed** moves it to the group heading. Focus after a row leaves the list is in the 7.1 table.

- Segment control: `role="tablist"`, roving tabindex, `aria-selected`, `aria-controls`; the Background tab's name carries only its exception count ("Background, 1 needs attention", plain "Background" at 0), never the session total.
- Panel opening is an operator action and moves focus to the first actionable item (else the tablist).
- Disclosures (`Answer ▸`, No failures reported, Older, Reviewed, Saved for later, Scheduled prompts) are `<button aria-expanded aria-controls>`.
- Live region: single polite `role="status"` present from first paint; announcements are counts or short results only ("Answer sent", "Approved", "3 items need a decision", "1 new background session"); `assertive`/`role="alert"` is never used for expected outcomes (section 11.1).
- Disabled-with-reason: use `aria-disabled="true"` plus `aria-describedby` reason text, stays focusable.
- State chips: icon + visible text + `aria-label`; color is never the only signal; contrast >= 4.5:1 for text and >= 3:1 for icons and control borders.
- Focus indicator visible and not obscured by sticky action bars (WCAG 2.4.11).
- Locators for e2e: `data-testid` or ARIA roles only.

### 7.1 Engagement, list lifecycle and shortcut guards (the single source)

Stories 2.6 and 2.10, Story 1.8 and UX-AC49, 50, 60, 61 and 62 cite this section and define nothing of their own.

The failure to design out: the operator's thumb or cursor is on a card, the list reorders, and the tap or a digit lands on a different question's option, sending an irreversible wrong answer.

**Two defined terms; no other use of the word "engaged".**

- **Engaged** (used only by the list freeze). A card is engaged while any of these holds: (a) the operator placed keyboard focus inside it (Tab, click, Enter, Space, tap, or a digit key that moved focus onto one of its options) and it is unresolved; (b) the pointer is over it (desktop), resolved or not; (c) a touch landed on it in the last 5 s, resolved or not; (d) it has an open sub-step (Deny reason, confirm), a draft or an unsent selection; (e) its commit delay is running; (f) the operator expanded it (Enter, Space or a tap on its header) and has not collapsed it (header toggle, Esc, panel close or leaving the panel) and it is unresolved. (f) exists because iOS Safari does not focus a button on tap, so on a phone a card being read would otherwise be engaged only by (c) for 5 s. **Focus placed by the program never counts** (after a resolve, on panel open, after a list change, by "Show" or "N new"), and neither does the first item that is pre-expanded on panel open; a digit's own focus move is the operator's keystroke, not a program move, so it does count under (a). A resolved card stops being engaged by (a), (d), (e) and (f). Engagement ends 750 ms after the last of these stops holding.
- **Digit-armed** (used only by the digit guard). A card is digit-armed when the operator opened it ((f)) or Tabbed onto it, it is engaged by (a), (c) or (f), and 750 ms have passed since the last programmatic focus move or list change anywhere in the panel ("list change" means a row inserted, removed or reordered; text-only updates such as a relative-time refresh are not list changes and never restart the clock). **A digit-initiated focus move is excluded from that clock**: it neither restarts the 750 ms nor disarms the card, so `1`, `2`, `3` pressed in quick succession visit options 1, 2, 3 and the card stays engaged while focus sits on an option. Digit handlers act only on a digit-armed card. The pre-expanded first item is not digit-armed until the operator Tabs onto it or clicks it; Enter on its header toggles it (collapses it), and the next Enter re-expands and arms it.

**Lifecycle of a resolved row (what removes it, when, and where focus goes).**

| Moment | The row | Focus | Announcement |
|---|---|---|---|
| Operator acts (click, tap, Enter; Approve/Send back/Mark done start the 3 s hold) | shows its pending state; engaged by (a), (b), (c), (d) or (e) | stays where the operator put it | "Approval pending" for a hold only |
| Server confirms a **final** state ("Approved", "Decided: {choice}", "Answered: {value}", saved-for-later, "Already handled") | stays in place showing that state; no longer engaged by focus, draft or hold, but pointer-over and a touch in the last 5 s still hold it. Rows 2 (`sent`), 3 and 6 (Unconfirmed), 5 (`delivery_failed` not_sent), 14 (aborted multi-question) and 16 (`still_showing`) are not final: each stays in Needs you, unchanged, and none leaves after 750 ms. Rows 3, 6, 14 and 16 count in the badge, and so does row 5 (it is unresolved and actionable via Retry); only a plain `sent` row (row 2) under 60 s does not, section 3.1 | unchanged | one short result ("Approved", "Answer sent") |
| Nothing is engaged for 750 ms (or the panel closes) | the Needs-you tier is unfrozen: the row moves to Recent activity and queued "N new" arrivals merge, in that order | if focus was inside the leaving row it moves to the next item header (focused, not engaged, not armed), else the list `<h3>`, else the empty-state heading; focus elsewhere is untouched | "N new items" once, only if rows merged. If the row was resolved asynchronously (a late acknowledgement, an answer elsewhere, a cleared mirror) **and focus was inside it**, the same single announcement also names the move: "Answer confirmed, moved to Recent activity" (without focus inside, a late acknowledgement stays silent) |
| Any programmatic focus move or list change | n/a | n/a | digits are inert for 750 ms (digit-armed rule); a digit's own focus move is not counted |
| Operator opens or Tabs to another card | that card becomes engaged and, after the grace period, digit-armed; the freeze holds again | operator-placed | none |

An event arrival (new notification, poll result, escalation) never moves focus. The only focus moves are the operator's own actions and the removal of the row that held focus.

- **Digits only move focus.** A digit moves focus to the Nth option button of the digit-armed card and never sends; sending takes a deliberate Enter, Space or tap on a focused option.
- **Settled layout and fling guard.** A newly rendered or moved interactive control ignores activation for 300 ms. A pointer or touch that begins within 120 ms of a `scroll` event (a tap that only stops a scroll fling) is ignored and its click is swallowed.
- **Reserved space.** The connectivity banner, the "N new" pill, the failure strip and the "As of {time}" line live in one always-rendered status slot (`min-height: 44px`, blank when idle) above the scroll region, never inside it, so showing or hiding any of them moves no row. The scroll region sets `overflow-anchor: auto` (any merge at an unengaged moment keeps the visible rows stationary); the status slot sets `overflow-anchor: none`.
- **Shortcuts are inert in inputs and the terminal.** Digit, J/K and Esc handlers are attached to the panel element, not the document, and return immediately when the event target is an `input`, `textarea`, `select` or `contenteditable` element, or inside the xterm terminal container; when Ctrl, Meta or Alt is held; or when `event.isComposing` (IME).

## 8. Flagged gaps (flows lacking exit path, error state, or definition)

### 8.1 Flows lacking a defined error state
1. **UX-S1 session-list poll failure**: plan 1.3 defines only success/empty. Added here (retry banner); plan needs a task.
2. **UX-S6 Remind me create/past-time/cap failures**: Task 4.6c has no error AC.
3. **UX-S5 list-fetch failure and missed-wake row**: Story 4.6 has no AC for load failure or the "missed after restart" alert condition on the card.
4. **Offline banner (R11)** appears in research but in no plan story; needs an owner task (shared component).

### 8.2 Flows lacking an exit path
1. **Multi-question aborted midway** (plan 3.5): partial state `sent`-aborted says "operator-visible message" but defines no recovery beyond Open session; operator cannot Retry safely because part was already typed. Exit here is Open session only. Recommend an explicit AC: Submit disabled, card pinned in Needs you until the operator opens the session or dismisses with confirm.
2. **Peer-help escalated row "Answer for {peer}"** and **Cancel request**: not specified in Story 4.5/4.6 (plan covers only peer->peer answers). Needs an AC and a Cancel descriptor for peer requests; today the operator has no way to cancel a stalled peer request.
3. **UX-S5 "Scheduled prompts paused"** banner: no in-pane way to resume; exit is the flag panel link. Acceptable only if the link exists; confirm.
4. **Departed session on Open**: handled here by read-only row; the plan notes "departed handling exists" but gives no copy.

### 8.4 Repair status (2026-10-17): where each gap is now owned in the plan
| Gap | Plan owner |
|---|---|
| 8.1.1 Background poll failure | Story 1.6, Task 1.6a (UX-AC5) |
| 8.1.2 Remind me create / past-time / cap failures | Story 4.10, Task 4.10b (UX-AC40, 41); last wave, Blocked on owner |
| 8.1.3 Scheduled-prompts load failure and missed-prompt row | Story 4.10, Task 4.10a (UX-AC39) |
| 8.1.4 Offline banner | Story 1.6, Task 1.6b (shared `OfflineBanner`; consumed by Task 2.6b) (UX-AC56) |
| 8.2.1 Aborted multi-question answer | Story 3.5, Task 3.5c (UX-AC29): Submit disabled, pinned in Needs you, Open session or confirmed Dismiss |
| 8.2.2 Peer-help Cancel request / Answer for {peer} | Story 4.10, Task 4.10d (`CANCEL_PEER_HELP` Tier A descriptor) (UX-AC32) |
| 8.2.3 Resume control for paused wakes | Story 4.10, Task 4.10c ("Resume scheduled prompts" inline confirm plus flag-settings link) (UX-AC38) |
| 8.2.4 Session reaped between polls / departed Open | Task 1.6c and Story 4.10 last AC (UX-AC6): "Session no longer exists" |

### 8.3 Formerly undecided items (all resolved, round 1 repair)
- **Remind me and Resume scheduled prompts ship** (in scope, last wave, optional-to-order). Defaults: presets "In 1 hour", "Tomorrow 9:00", "Pick time..."; Resume is an inline confirm, local listener only, audit-logged. Changing the preset values is a config edit, not a gate. This removes the contradiction with `requirements.md` scope item 6 (which already said kept in scope) and the plan's Unresolved Questions.
- **An unacknowledged answer stays in Needs you** as an Unconfirmed sub-state until acknowledged or the operator acts (section 3.1). The earlier "move to Recent after 60 s" rule hid a possibly stalled agent and is withdrawn.
- Peer-help rows do not count toward the bell badge before escalation (unchanged).

---

## 9. UX-S7 - Cross-cutting condensed entries

**Connectivity states (one banner component, three states).** The banner is a persistent in-panel text element, not a live region of its own (the single global polite region announces each state change once: "Offline", "Reconnecting", "Back online"). Action buttons are `aria-disabled` with the state as the reason; no state moves focus.

| State | Trigger | Copy | Exit |
|---|---|---|---|
| Offline | `navigator.onLine` is false | "Offline. Showing last known state." | auto-recover on `online` |
| Reconnecting | browser online, the server stream or a poll failed | "Reconnecting to the server. Showing last known state." | auto-recover with backoff (1 s doubling to 15 s); after 30 s the copy becomes "Can't reach the server." with a **Retry** button |
| Loading (cold, no cached data) | first open with nothing cached | "Loading..." only if it lasts longer than 150 ms (no flash); after 8 s "Still loading." with **Retry** | Retry |

- Offline appears within 2 s of connectivity loss and disappears on recovery without a reload; Reconnecting appears within 2 s of a failed poll or stream drop.
- Drafts survive all three states.
- A request in flight for more than 10 s shows "Still sending..." in the row (not announced); at 30 s it becomes the no-response copy in section 2.3 with Retry on the same idempotency key.
- Cached state renders within the 200 ms budget; "Loading" is only for the no-cache case.
- **After recovery, controls stay disabled until fresh.** When Offline or Reconnecting ends, every action control stays `aria-disabled` with the reason "Updating..." until the first successful refresh of the descriptors completes, so a stale card cannot act on an item that was resolved meanwhile. Needs you shows a plain-text line "As of {time}" (the last successful refresh), updated on each refresh and not a live region.

**Empty, loading and error copy for the two Notifications tiers.**

| Tier | Empty | Loading (no cache) | Fetch failed |
|---|---|---|---|
| Needs you | "Nothing needs you right now." | "Loading..." after 150 ms, "Still loading." with **Retry** at 8 s | "Couldn't load notifications. Showing data from {time}." with **Retry**; rows kept |
| Recent activity | "No recent activity." | same thresholds | "Couldn't load recent activity. Showing data from {time}." with **Retry**; rows kept |

**Drafts** (a typed answer, a Deny reason, a Remind-me time) are stored per question or item id in `sessionStorage` inside try/catch, fall back to memory when storage is unavailable, and are cleared on send or Dismiss. They survive panel close, mobile Back and tab reload, and never outlive the tab (no `localStorage`).

**Flag-off fallbacks**: each surface degrades to its pre-feature behavior (legacy empty state, legacy Approve/Deny, text-only question, no Scheduled prompts); flipping a flag live must not require reload and must not strand an in-flight action (it completes or returns `FLAG_OFF` with Open).

---

## 10. UX acceptance criteria

Each is human-testable. Prefix UX-AC.

### Background feed (UX-S1)
- UX-AC1. With 11 hidden sessions and no history rows, the Background tab shows 11 rows and the roll-up "11 background sessions, no failures reported. Last activity {relative time}." (never "healthy"; stopped sessions with no recorded outcome show "Stopped, outcome unknown"); the tab is never blank.
- UX-AC2. With zero hidden sessions the tab shows "No background work. Review and diagnose sessions will appear here while they run." and no spinner.
- UX-AC3. A failed or needs-human session appears in a "Needs attention" group above the roll-up; each session appears once (exception wins).
- UX-AC4. When a poll adds sessions while the tab is open, existing rows do not change position; a "N new" button appears and activating it merges rows and announces "N new background sessions".
- UX-AC5. If the session poll fails, last-known rows remain visible with "Couldn't refresh background sessions. Showing data from {time}." and a Retry button that works.
- UX-AC6. Clicking Open on a departed session shows "Session no longer exists" instead of navigating to a broken page.
- UX-AC7. The Background tab's accessible name is "Background, N needs attention" (plain "Background" at 0 exceptions) and does not change when informational rows are added; Left/Right arrows move between tabs and update `aria-selected`.

### Inline actions (UX-S2)
- UX-AC8. A pending approval, a pending question with options, and a backlog decision each resolve with exactly one click after the pane is open (Deny reason and merge confirm excepted; those take two). The 3-second commit delay of UX-AC63 is not a second interaction because Undo is optional.
- UX-AC9. After one press the pressed button is disabled and shows an in-progress label within 100ms; a second press sends nothing.
- UX-AC10. `ALREADY_RESOLVED`/`STALE` show "Already handled" read-only text and create no `role="alert"` element.
- UX-AC11. `NOT_SENT` shows "Not sent. Nothing was typed into the session. Retry" with Retry and Open session; `SEND_INDETERMINATE` shows "Sent, but we couldn't confirm it arrived. Check the session before retrying." with Open session and a Retry that needs an inline "Retry anyway?" confirm. The two strings are distinct and neither is shown as answered.
- UX-AC12. A transient failure shows an inline error with Retry; Retry reuses the idempotency key and never produces two writes.
- UX-AC13. On the remote listener, Tier B actions render `aria-disabled` with accessible description "Answer from desktop" and an enabled Open session button; Tier A actions work.
- UX-AC14. Every action row has an Open control; an unknown action kind renders only Open.
- UX-AC15. Merge and Deny-with-reason use an inline sub-step inside the row (no modal) with focus moved to "Confirm merge" or the reason field, and **Back** restoring the prior state with the draft kept. No button is labelled with a bare "Cancel".
- UX-AC16. Unread actionable items have no dismiss control; "Mark all read" leaves them unread.
- UX-AC17. `RATE_LIMITED` shows a countdown in text and re-enables the control when it reaches zero.

### Question cards (UX-S3)
- UX-AC18. A single-select question with 2-4 options renders `<button>` options inside a labelled `role="group"` (not a `radiogroup`); one click, tap, Enter or Space on an option sends the answer without a separate Submit and locks the card. Arrow keys, Tab and digit keys never send: a keyboard user can move through every option with no answer sent.
- UX-AC19. A multi-select question renders checkboxes inside a labelled group with a "Submit answers" button, a "Clear" button and a visible "N selected" count that is not announced per toggle.
- UX-AC20. A two-question payload renders both groups and one Submit that is disabled with helper text until every required group is answered.
- UX-AC21. More than 8 options render a labelled native `<select>`; a free-text "Other" input (rendered only on routes that can carry text, section 3.1; absent on `live_keystroke` and hook-mirrored cards) uses font-size >= 16px and Cmd/Ctrl+Enter sends.
- UX-AC22. Submitting with a required group empty shows an inline text error linked by `aria-describedby`, sets `aria-invalid`, and moves focus to the first invalid group.
- UX-AC23. Once a request has settled, the delivery-state line shows exactly one of the 17 settled-state strings in the section 3.1 table (the `human_active` text is the row 5 variant, the 18th literal; the 6 transient and reason strings listed after the table are a separate set that UX-AC73 and section 2.3 own) (Sending...; Sent - waiting for the agent; Sent, but the agent hasn't confirmed yet. Open session.; Answered: {value}; Not sent. Nothing was typed into the session. Retry; Sent, but we couldn't confirm it arrived. Check the session before retrying.; Couldn't send your answer. Retry; the stopped-session and archived-session saved copies; Session ended before this was answered.; Already handled in the session. No action needed.; The session is now asking something else. Open session to answer it.; Answered elsewhere: {value}; Part of your answer was sent, then the prompt changed. Open session to finish.; Answer from desktop; Sent, but the question is still showing. Check the session before retrying.; Sent, but the session ended before confirming it.), never two at once, and it is not a live region.
- UX-AC24. An answer still `sent` and unacknowledged at 60 s does one fresh capture and follows the section 3.1 decision table: dialog still showing -> row 16 "Sent, but the question is still showing. Check the session before retrying." with Open session and a Retry behind "Retry anyway?" (never the "Nothing was typed" string, because a digit was written); dialog gone -> `acknowledged`; capture error or indeterminate -> the **Unconfirmed** chip and "Sent, but the agent hasn't confirmed yet. Open session." (a row already at row 6 keeps its copy and chip). The Unconfirmed item **stays in Needs you**, counts in the bell badge (a plain `sent` answer under 60 s does not; row 6 counts from the write), turns into "Answered: {value}" by itself when an acknowledgement arrives, and leaves only on acknowledgement, **Mark as handled** (browser-local) or the session ending (then row 17). It is never moved to Recent activity by a timer.
- UX-AC25. Answering in one tab shows "Answered elsewhere: {value}" in another tab within the next sync, without moving focus.
- UX-AC26. Answering a question in the terminal while its card is open flips the card to read-only "Already handled" with a polite announcement and no focus change.
- UX-AC27. Archived sessions are never revived by answering; the card says so before the operator submits (copy visible without clicking Submit).
- UX-AC28. Unrenderable or read-only shapes show question and options as text with "View full prompt in session" and Open session.
- UX-AC29. A multi-question answer that aborts partway shows "Part of your answer was sent, then the prompt changed. Open session to finish." with Submit disabled and Open session enabled.
- UX-AC30. Esc collapses a card without submitting and re-expanding restores the draft selections or text, also after the panel is closed and reopened or the tab is reloaded (drafts live in `sessionStorage`).

### Peer help (UX-S4)
- UX-AC31. A peer request shows asker, answerer, question, age and state (waiting / answered by {session} / failed with reason / cancelled), and does not increment the bell badge until escalated.
- UX-AC32. An escalated (timed out or peer-unavailable) request appears in Needs you with Answer for {peer}, Cancel request and Open; each works or shows an inline error with Retry.
- UX-AC33. A request that was accepted and then cannot be delivered (answerer archived, deleted or moved cross-workspace) shows "Not sent: {reason}" in plain language. A request rejected at call time returns a typed error to the asker and creates no row.
- UX-AC34. Answered requests show "Answered by {session}" with the provenance text and move to Recent without operator action.

### Scheduled prompts (UX-S5) and Remind me (UX-S6)
- UX-AC35. The "Scheduled prompts" section lists each pending trigger and open peer request with owner, plain-language condition, since, next fire, and a Cancel prompt button; with none pending it shows "No scheduled prompts. Agents that ask to be prompted again will appear here."
- UX-AC36. Cancel prompt asks for inline confirmation ("Cancel this scheduled prompt? The agent will not be re-prompted.") with "Yes, cancel" and "Keep" buttons; after confirm the row disappears and "Prompt cancelled" is announced.
- UX-AC37. If a trigger fired before Cancel prompt completes, the row shows "Already fired at {time}." and no error toast.
- UX-AC38. With `agent_wake_triggers` off, the section shows "Scheduled prompts paused. Agents will not be prompted automatically." plus a "Resume scheduled prompts" button (inline confirm) and a link to the flag settings; Cancel still works.
- UX-AC39. A scheduled prompt missed after a restart is shown as an exception row "Missed: {reason}" with warning icon and text, with Cancel prompt and Open owner.
- UX-AC40. If Remind me ships, it appears only on informational Background rows, never on blocking questions or approvals; picking a preset shows "Reminder set for {time}" with Undo for 10 s; failure shows "Couldn't set the reminder. Retry".
- UX-AC41. A reminder time in the past is rejected inline with "Choose a time in the future".

### Mobile and desktop (all surfaces)
- UX-AC42. At 390px width every button, option row, checkbox row, tab and disclosure has a computed height >= 44px; at >= 900px layout uses the desktop arrangement.
- UX-AC43. Below 900px options and action buttons stack vertically; Submit is full width and stays visible above the on-screen keyboard when a text field has focus.
- UX-AC44. Text inputs use font-size >= 16px so iOS Safari does not zoom.
- UX-AC45. No control is reachable only by hover; every action is operable by touch alone.
- UX-AC46. The bell panel on mobile is one scroll region with no nested scroll trap.
- UX-AC47. Toasts carrying actions pause on hover and focus, and the item remains in the panel after the toast disappears.

### Keyboard and accessibility
- UX-AC48. The whole flow (open panel, move between items, expand, choose option, submit, close) is operable with keyboard only; no keyboard trap; Esc closes the innermost layer first (sub-step, then card, then panel) and the panel returns focus to the bell. Browsing options with the keyboard sends nothing (UX-AC18). Up/Down and J/K behave identically (section 7).
- UX-AC49. Number keys only move focus to the Nth option, and only inside a digit-armed card (section 7.1); they never send, are inert for 750 ms after any programmatic focus move or list change but not after a digit's own focus move, so `1`, `2`, `3` pressed in quick succession visit options 1, 2, 3 (UX-AC61), and never fire from the terminal, an input, or elsewhere in the page (UX-AC88).
- UX-AC50. When an item is resolved and then leaves the list (at the moment the section 7.1 lifecycle table removes it, not at resolve time) and focus was inside it, focus lands on the next item (focused but not engaged) else the list `<h3>` else the empty-state heading, never the body, and the polite status region announces a short result ("Approved", "Answer sent").
- UX-AC51. The single polite live region exists from first paint, announces counts or short results only, and no expected outcome uses `role="alert"` or `aria-live="assertive"`. No other element in the panel is a live region (UX-AC84).
- UX-AC52. Every state chip and badge has icon, visible text, and `aria-label`; text contrast >= 4.5:1 and icon/border contrast >= 3:1 in light and dark themes.
- UX-AC53. Disabled-with-reason controls use `aria-disabled` with `aria-describedby`, stay focusable, and state the reason ("Offline", "Reconnecting", "Answer from desktop", "Resume from desktop").
- UX-AC54. Focus indicators are visible and never hidden behind sticky action bars.

### Errors, offline, flags (no dead ends)
- UX-AC55. Every error or terminal state listed in sections 1.3, 2.3, 3.3, 4.3, 5.3 and 6.3 offers at least one exit control (Retry, Open, Dismiss, Back, Keep, Cancel prompt or Cancel request, or a link); none leaves the operator with only a message.
- UX-AC56. When offline, a persistent banner "Offline. Showing last known state." appears, actions are `aria-disabled` with reason, drafts are preserved, and actions work again after reconnect without reload, once the first successful refresh completes (controls stay `aria-disabled` "Updating..." until then and Needs you shows "As of {time}"). When the browser is online but the server is unreachable the banner reads "Reconnecting to the server. Showing last known state." and becomes "Can't reach the server." with Retry after 30 s (UX-AC73).
- UX-AC57. Turning any of the four flags off at runtime degrades the affected surface to its prior behavior without reload; an in-flight action completes or returns an Open-session fallback.
- UX-AC58. No UX copy claims the operator is authenticated on the local listener or promises delivery to an archived session.
- UX-AC59. A question whose dialog is still on screen stays answerable however long it has been open (tested at 45 minutes); "Already handled" appears only after a confirmed mismatch, the session ending, or an answer elsewhere, never because time passed.

### Interaction safety (B1, B2, B3 and consequential taps)
- UX-AC60. While a card is engaged (the section 7.1 definition: operator-placed focus, pointer over it, a touch in the last 5 s, an open sub-step, draft or unsent selection, or a running commit delay; programmatic focus never counts) no row in Needs you is inserted, removed or reordered; arrivals appear behind a sticky "N new" button and a resolved row keeps showing its final state until nothing is engaged or the panel closes.
- UX-AC61. Digit shortcuts do nothing unless the target is inside a digit-armed card (section 7.1); focus that arrives programmatically (after a resolve, panel open or list change) neither engages nor arms, and digits are ignored for 750 ms after it. A digit moves focus to the Nth option button and never sends; that move does not restart the 750 ms and the card stays engaged while focus is on an option, so consecutive digits work immediately.
- UX-AC62. A control that was just rendered or moved ignores activation for 300 ms, and a touch that begins within 120 ms of a scroll event (a tap that only stops a fling) activates nothing, so a tap aimed at the previous occupant of a position or at a moving list cannot trigger a control; banners, the "N new" pill and the failure strip use the reserved status slot and move no row.
- UX-AC63. Approve, Send back and Mark done start a 3-second hold with an Undo button; Undo restores the prior state with nothing sent; closing the panel, a click outside the desktop popover, switching tabs, `visibilitychange` to hidden, `pagehide`, going offline or leaving the page during the hold cancels it, sends nothing and restores the card with "Approval cancelled when you left. Tap Approve to try again."; the restored card is an unresolved actionable item that the bell badge counts; Deny and answers have no hold and Merge uses the inline confirm. The countdown is not announced each second.
- UX-AC64. The approval row shows the complete tool name and command text with no ellipsis; text over 8 lines or 400 characters renders in a labelled keyboard-scrollable region with a character count and Copy; an unrenderable input shows "Can't show this command. Open session to review it." with Open instead of Approve.
- UX-AC65. At widths below 900px Deny and Approve are separated by at least 24px and Deny comes first; every button is at least 44px high. **Create rule** is never in the primary row: it lives in the `More` menu at widths >= 900px on the local listener only, behind the inline confirm, and is not rendered below 900px or on `:8444`.

### Background feed governance (UX-S1)
- UX-AC66. A new failure or needs-human session is named in a sticky strip above the list ("review:pr-1042 failed 1m ago", Show, Open; at most 3 named, then "N more failures"), never only as a count in an "N new" pill; an existing row that becomes a failure gains its chip in place without moving; Show moves focus to the row.
- UX-AC67. The bell badge equals unread actionable Needs-you items plus escalated peer-help requests plus Unconfirmed answers (rows 3, 6 and 16) plus unacknowledged Background exceptions; it never counts a plain `sent` answer (row 2) under 60 s, Finished, Stopped-outcome-unknown, running, departed, un-escalated peer help or Reminders; the Background tab badge shows the exceptions part only.
- UX-AC68. Each Background row has Mark reviewed (and the group has Mark all reviewed when 2 or more are unreviewed); reviewed rows collapse into "Reviewed (M)" and the roll-up reads "... no failures reported (M reviewed)"; when browser storage is unavailable the marks last for the page session and the section says "Marks are not saved in this browser".
- UX-AC69. Finished, Stopped-outcome-unknown and Reviewed rows older than 24 h collapse into an "Older (N)" disclosure; at most 100 rows render with "Show all (N)"; the list is never windowed; with 500 hidden-session rows the tab still commits its first render inside the 200 ms budget (deterministic proxy: one render pass, no per-row effects).
- UX-AC70. The first time the Background tab opens in a browser a dismissible note explains what the tab shows and that "no failures reported" does not mean everything succeeded; after "Got it" it does not reappear in that browser.
- UX-AC71. Clicking or tapping row text toggles its details; navigation happens only through an explicit Open button; a control inside a row never toggles the row; no row is a link.
- UX-AC72. No notification id renders in both Background and Needs you: actionable items live only in Needs you and the Background row shows the link chip "Needs you (N)" without controls; hidden-session failure records live only in the Background Needs-attention group.

### Loading, connectivity, navigation
- UX-AC73. With no cached data the panel shows "Loading..." only after 150 ms and "Still loading." with Retry after 8 s; a request pending for 10 s shows "Still sending..." and at 30 s the no-response copy with Retry on the same idempotency key; a failed poll with the browser online shows Reconnecting, then "Can't reach the server." with Retry after 30 s; Needs you and Recent activity show their own empty, loading and fetch-failed copy (section 9).
- UX-AC74. On mobile the Alerts page adds one browser-history entry; the Back button or gesture closes it and returns to the previous page with drafts retained in `sessionStorage`; sub-steps are not history entries; Back never traps; the desktop popover does not touch history. Drafts are retained in `sessionStorage`, so they also survive a reload.
- UX-AC75. Nothing in the panel is a modal dialog or a bottom sheet (`role="dialog"`/`aria-modal` absent on every surface); no event arrival moves focus; only the operator's own action does.

### Vocabulary and tiers
- UX-AC76. No UI string (the scope is defined under the section 0 vocabulary table: strings exported by `notificationCopy.ts` and literals rendered in the panel, not internal identifiers) contains the word "wake" or "wakes", the phrase "Waiting on" (so neither a section label nor the old "Waiting on you" chip), or equals a bare "Cancel"; the only exception is the stored flag name `agent_wake_triggers` inside the flag-settings link. Scheduled prompts, Reminder (you), Cancel prompt, Cancel request, Cancel reminder, Back, Clear, Keep, Needs you (N) and Answer for {peer} are used as defined in the vocabulary table (`copyGuard.test.ts`, which also renders the panel with every state in sections 1.3, 2.3, 3.1, 3.3, 5.3 and 6.3 and scans the DOM text).
- UX-AC77. On the remote listener every action renders per the section 2.4 tier table: Tier A actions work, `live_keystroke` answers and Resume scheduled prompts are `aria-disabled` with their reasons and an enabled Open, and Create rule is not rendered.
- UX-AC78. The documentation states, in the how-to for inline actions, that alerting an away operator depends on the out-of-scope Slack and web-push channels and that their delivery was not verified by this project.
- UX-AC79. Agent-created answers saved for a stopped or archived session are listed read-only under "Saved for later (N)" in Recent activity with Open session, are not counted in the badge, and hook-mirrored questions never appear there.
- UX-AC80. A peer-help escalation raises the bell badge, produces exactly one polite announcement "1 item needs a decision", never moves focus, and respects the engaged-card freeze (it waits behind "N new" when a card is engaged).

### WCAG 2.2 and device matrix
- UX-AC81. At 320 CSS px width (WCAG 1.4.10) the panel has no horizontal scroll and no clipped control; long commands and option labels wrap.
- UX-AC82. With the WCAG 1.4.12 text-spacing overrides applied (line height 1.5, paragraph spacing 2x, letter spacing 0.12em, word spacing 0.16em) no text is clipped or overlapped.
- UX-AC83. Inline text controls (Retry, Show all (N), Mark reviewed, Undo) are at least 44px high as stand-alone buttons; any control inside a sentence has at least a 24 by 24 CSS px target or 24px of spacing (WCAG 2.5.8).
- UX-AC84. There is exactly one live region in the panel; the delivery line, banners, "Loading", countdowns (rate-limit and commit-delay) and "N selected" are not live regions; countdowns announce only their start and end; a state change that the global region announces is not also announced by a second element; each asynchronous delivery transition (`sent` to `delivery_failed`, Unconfirmed, an aborted multi-question answer) produces exactly one polite announcement (section 11.1).
- UX-AC85. Heading levels are `h2` (panel title), `h3` (tier and group headings); the list `<h3>` and the empty-state heading carry `tabindex="-1"` and are the focus fallbacks.
- UX-AC86. With `prefers-reduced-motion: reduce` there are no transitions, highlight fades or animated counters; with `forced-colors: active` chips, icons, borders and focus rings remain visible (system colors, `outline` not box-shadow only).
- UX-AC87. The mobile keyboard matrix in section 11.4 holds on iOS Safari and Android Chrome (sticky Submit above the soft keyboard, no zoom, correct Enter behavior, no focus loss on keyboard open).
- UX-AC88. Digit, J/K and Esc handlers do nothing when the event target is an input, textarea, select or contenteditable element, is inside the terminal container, when Ctrl/Meta/Alt is held, or while IME composition is active; these single-character shortcuts exist only while focus is inside the panel list (WCAG 2.1.4).

Totals: 6 interactive surfaces (UX-S1 to UX-S6) plus 1 cross-cutting condensed entry (UX-S7), 88 UX acceptance criteria (59 original, 29 added in the round 1 repair).

---

## 11. WCAG 2.2, navigation and device details (round 1 repair)

### 11.1 Live announcements, without duplicates
One polite `role="status"` region exists from first paint. It is the only live region. Banners (Offline, Reconnecting), the delivery-state line, "Loading", "N selected", the rate-limit countdown and the commit-delay countdown are plain text. The global region announces: counts ("3 items need a decision"), terminal results ("Answer sent", "Approved", "Approval undone", "Prompt cancelled") and connectivity changes ("Offline", "Back online"). Intermediate states ("Sending...", "Still sending...") are not announced. **One announcement per asynchronous delivery transition**, short and count-free, because the delivery line is not live: a `delivery_failed(not_sent)` (row 5; nothing was typed) says "Answer not delivered"; `sent` to `delivery_failed(still_showing)` (row 16; a digit was written) says "Answer sent, question still showing", never "Answer not delivered"; a card turning Unconfirmed says "Answer not confirmed"; an aborted multi-question answer says "Answer partly sent"; a late acknowledgement is silent unless keyboard focus was inside the row when it leaves, in which case the one announcement is "Answer confirmed, moved to Recent activity" (section 7.1 lifecycle table); a session ending on a sent answer (row 17) is silent. A test fires each transition twice and counts exactly one announcement per distinct transition. Countdowns announce once at their start ("Approval pending") and once at their end. A test counts live-region elements in the rendered panel (UX-AC84).

### 11.2 Reflow, spacing, targets, structure
- 1.4.10 Reflow: at 320 CSS px the panel is a single column with no horizontal scrolling; commands wrap (`overflow-wrap: anywhere`); the 2.4 scroll region scrolls vertically only.
- 1.4.12 Text spacing: heights are `min-height`, never fixed; no `overflow: hidden` on text containers.
- 2.5.8 Target size: stand-alone controls are 44px; an inline text control inside a sentence has a 24 by 24 px minimum target or 24px spacing to its neighbours.
- Headings: panel title `h2`; "Needs you", "Recent activity", "Needs attention", "Scheduled prompts" are `h3`. Disclosure buttons are not headings.
- Virtualization: none. The 100-row cap with "Show all (N)" bounds the DOM; "Show all" renders every row, using `content-visibility: auto` for paint cost only, so a screen-reader user can reach every row and `aria-setsize`/`aria-posinset` are unnecessary.
- Reduced motion: no transitions or fades under `prefers-reduced-motion: reduce`; the 300 ms settled-layout window (section 7.1) is a timing guard, not an animation, and stays.
- Forced colors: under `forced-colors: active` chips keep a 1px `CanvasText` border, icons use `currentColor`, the focus ring is an `outline` (box-shadow-only rings vanish), and no meaning is carried by background color.

### 11.3 Mobile navigation
- The bell opens the Alerts page, which pushes one history entry (`history.pushState` with a state marker). Back or the OS back gesture pops it and closes the page, returning to where the operator was; drafts survive in `sessionStorage` (UX-AC30). Sub-steps (Deny reason, confirm, Remind-me list) are not history entries; Back from inside one closes the whole page, and the next open restores the draft.
- Deep links (`/alerts?item=<id>`) open the page with that item expanded and focused by the operator's own navigation; the entry replaces rather than pushes when the page was opened by a link.
- The desktop popover never touches history; Esc closes it.

### 11.4 Mobile keyboard matrix (390px, iOS Safari 17+ and Android Chrome)

| Field | Input attributes | Enter key | Sticky bar | Zoom / viewport |
|---|---|---|---|---|
| Free-text "Other" (single line) | `type="text"`, `enterkeyhint="send"`, `autocapitalize="sentences"`, `autocomplete="off"`, `spellcheck` on | Sends (same as the Send button) | Send pinned above the keyboard via `visualViewport` | `font-size: 16px`, no zoom |
| Deny reason (single line) | `enterkeyhint="send"` | Sends the denial | Send denial pinned above the keyboard | 16px |
| Multi-line answer textarea | `enterkeyhint="enter"` | Inserts a newline; only the Send button (or Cmd/Ctrl+Enter on a hardware keyboard) sends | Submit pinned above the keyboard | 16px |
| Pick-time field (Remind me) | `type="datetime-local"` | n/a (picker) | Confirm button inline | 16px |

On focus the field is scrolled into view with `scrollIntoView({block: "nearest"})`; on keyboard open the sticky bar re-anchors to `visualViewport.height` so it never covers the focused field (WCAG 2.4.11). Opening the keyboard never moves focus off the field or collapses the card. Verified on real devices in the Task 5.3 checklist (UX-AC87).

## 12. Usability validation plan

The operator is a single person, so this is a structured dogfood protocol with instrumentation, not a recruited study. It runs in plan Task 5.3c, per flag, before each default flip.

| # | Task (scripted, run on desktop and on the phone) | Success criterion | What failure means |
|---|---|---|---|
| T1 | Open the Background tab cold and say, without clicking Open, whether anything failed | Correct in under 10 s; the "no failures reported" line is not read as "all succeeded" | Roll-up copy is rewritten; flag stays ON (read-only change) |
| T2 | Answer a 3-option question from a notification | Correct option sent; no navigation; under 5 s from pane open | Option-button layout or spacing is changed before `askuserquestion_structured` stage:answer is enabled |
| T3 | While a card is open, have a second question arrive; answer the first | Answer lands on the intended card (0 wrong-card answers in 10 trials) | Freeze/grace rules are tightened; block the flip |
| T4 | Approve a command longer than 10 lines | Operator reads the whole command before tapping (observed scroll or Copy) | Command region is redesigned; block the flip |
| T5 | Cause an Unconfirmed answer (drop the dialog fixture) and recover | Operator opens the session or marks handled within 60 s without prompting | Unconfirmed copy is rewritten |
| T6 | Handle a stopped-session answer and a saved-for-later answer | Operator states the answer was saved and the agent has not received it yet | Saved copy is rewritten |
| T7 | From the phone only, resolve one Tier A and see one Tier B marked "Answer from desktop" | Operator does not try the Tier B action twice | Tier B copy is rewritten |
| T8 | Hostile: scroll-fling a list of 30 items on the phone and tap the glass to stop it, 10 trials | 0 activations of any control | The fling guard (120 ms) is widened; block the flip |
| T9 | Hostile: arrival under the thumb. With a thumb about to tap a card below, make a new item and a failure arrive at the top, 10 trials | 0 wrong-card activations and 0 row movement while engaged | Freeze, reserved slot or settle window is tightened; block the flip |
| T10 | Hostile: app switch mid-hold. Tap Approve, switch to another app within 3 s, return, 10 trials | No approval is sent; the card is restored with the cancellation line and the bell badge still counts it | Hold-cancel logic is fixed; block the flip |

Instrumentation runs alongside: the paired quality metrics (`requirements.md`: delivery-failure rate, wrong-answer proxy, Unconfirmed rate). The flag flip is blocked while any task fails twice in a row or any quality metric misses its target over the last 7 days of the 14-day soak (the decision rule in `requirements.md`). Baselines are "to be measured in Epic 5 telemetry".
