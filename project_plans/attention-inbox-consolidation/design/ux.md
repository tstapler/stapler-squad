# UX Design: attention-inbox-consolidation

**Date**: 2026-10-23
**Inputs**: `requirements.md`, `research/ux.md`, `implementation/plan.md` (Stories 3.2.x, 4.2.x, 5.x, 6.x), ADR-001 (staged hybrid), ADR-002 (new visible reasons, `IDLE` stays hidden), ADR-005 (`LastMeaningfulChangeAt`), ADR-007 (delivery fallback, mute, server-derived Seen). Triad repair iteration 1 revised S1-S4, S6, S9, S10, S13 and added S14-S16 and UX-40..UX-48; iteration 2 replaced the live-region topology (S10), the unseen-cue rule (S14), the vocabulary table, and added UX-49; plan.md's Triad Repair Iteration 1 and 2 Disposition tables list every change.
**Grounding**: source and plan only. The live app at :8543 was not inspected. Component names and line refs come from `research/ux.md` section 3 and the plan; treat them as INFERRED until the implementer opens the files.
**Form factors**: every surface is specified for desktop (>= 1024 px, two-pane where noted) and mobile (< 768 px, 375 px reference). < 768 px uses the mobile layout and the bottom sheet; >= 768 px uses the desktop drawer (list layout switches at 1024 px).

## Design decisions in force (not re-litigated here)

1. Route stays `/review-queue`; UI copy calls it "Attention" (ADR-001). No new route, no new RouteMeta row.
2. State list (per session, ranked, ack-able) is the source of truth. The tray is a transient alert/history layer that links into the list.
3. `ProseQuestion` (Medium, needs-me, delivered as `INPUT_REQUIRED`: toast, web push, Slack as today) and `AwaitingDirection` (Low, informational, visible, DELIVERED once per turn-end episode as a low-urgency tray record plus web push, no toast, no Slack by default; owner to confirm) are the only new reasons. Bare `IDLE` remains a chip, not a row (ADR-002).
4. Opening a session marks it "seen" implicitly and never removes an item; Seen is server-derived (`LastViewed >= TurnEnded.At`), so every device shows the same marker (ADR-007; the earlier per-viewer `localStorage` Seen is gone). "Done" is explicit and exists only on informational rows (plan.md Attention Reason Classification: `awaiting_direction`, `task_complete`, `uncommitted_changes`, `stale`). The nav badge, Sessions-tab badge and document title count needs-me reasons only (`approval_pending`, `input_required`, `error_state`, `tests_failing`, `waiting_for_user`, `prose_question`). Needs-me rows are cleared by the operator replying, or by "Not a question" (prose only), or by state change.
5. One timeline component: right drawer on desktop, bottom sheet on mobile.
6. Time display: "Last activity" (from `LastMeaningfulChangeAt`) plus static "Created"; relative under 7 days, absolute after; absolute always reachable without hover.

## Surface inventory

| # | Surface | Type | Treatment |
|---|---|---|---|
| S1 | Attention list (grouped, filtered, populated) | interactive | full |
| S2 | Attention row anatomy, incl. new reasons | interactive | full |
| S3 | "Not a question", "Stop flagging questions" and the burst guard | interactive | full |
| S4 | Done / Seen semantics and bulk "mark all finished done" | interactive | full |
| S5 | Tray row to list deep link (`/review-queue?session=`) | interactive | full |
| S6 | Session event timeline (desktop drawer, mobile bottom sheet) | interactive | full |
| S7 | SessionCard "Last activity" and "Created" | interactive | full |
| S8 | Up Next "Sessions" tab (flag `attention_inbox_consolidation`) | interactive | full |
| S9 | Empty, filtered-empty, error, offline, flag-off states | interactive | full (state table) |
| S10 | Announcer (single live region), nav badge, document title | partly non-interactive | full |
| S11 | `/review-queue` redirect | non-interactive | condensed |
| S12 | Time formatting contract (`useNowTicker`, `formatRelativeTime`) | non-interactive | condensed |
| S13 | Recent activity view (cross-session: finished, quiet now, changed, done) | interactive | full |
| S14 | Unseen-finished cue (push-independent), mute, unconfirmed signal | interactive | full |
| S15 | Keyboard shortcuts: focus scope, setting, undo, focus placement | interactive | full |
| S16 | First-run help for the new labels | interactive | full |

16 surfaces (14 full, 2 condensed).

## Vocabulary (one word per concept)

Every user-facing string uses the word in the first column and no synonym. `needs-me`, `needs-human` and `ackSnooze` are code and doc shorthand for classes and rules; they never appear in a string.

| Word | Means | Where it appears |
|---|---|---|
| Needs you | items that need the operator (approval, waiting for input, error, tests failing, asked a question) | chip "Needs you", group title "NEEDS YOU", nav accessible name "N need you" (the plural of the same word), empty state "Nothing needs you" |
| Finished | ALL open informational items: `awaiting_direction`, `task_complete`, `stale`, `uncommitted_changes`, Seen or not; N is what is true now and does not drop when a session is opened | chip "Finished N", group title "Finished"; row reason labels "Finished, waiting for direction" (turn ended, no question) and "Task complete" (the program reported success and no turn-end fact exists); the other informational labels are "Uncommitted changes" and "No change for N min" |
| New finished | ONLY `awaiting_direction` items not yet Seen on any device (a subset of Finished, so N <= the chip number; opening a session lowers N and leaves the chip number unchanged). "new" is the visible word for unseen | nav cue "N new finished" ("9+ new finished" above 9), accessible name "3 new finished"; never the chip or the group title |
| Waiting for input | one label for both `input_required` and `waiting_for_user` (both mean the agent is blocked on the operator; the two reasons stay distinct in metrics) | row reason label |
| Quiet | NOW: no meaningful change for N minutes and no open item, with Running/Idle status text; pull-only by design (no nav cue) | chip; Recent section "Quiet now (no change in N min)" |
| Done | the explicit action that removes an informational item; past tense "Marked done" | button, toast, timeline entry, bulk "Mark all finished done", Recent section "Done (24 h)" |
| Not a question | the prose-only action; past tense "Marked not a question" | button, toast, timeline entry |
| Dismiss | tray records ONLY (a tray row is dismissed; an Attention row is never "dismissed") | tray button |
| Mute | the one word for suppressing push and Slack; shown as static text "Muted until 16:20"; a global mute shows the short visible word "muted" on the nav cue line and ", alerts muted" in the accessible name | row menu, header control, nav marker. "Pause" and "snooze" are not used |
| Questions not flagged | the visible state of "Stop flagging questions for this session": a text tag on that session's rows and card and an entry in the mute management list; reversed by "Flag questions for this session" | row, session card, mute management list |
| Paused | only sessions whose Status is Paused, Stopped or Hibernated | collapsed "Paused (N)" group |
| Seen | marker only, never an action; server-derived | text marker on a row |
| Unconfirmed | needs-you item detected from screen/transcript text, not a hook | row tag, list header count, nav accessible name |
| Attention | the nav label and the list heading; after consolidation also the Up Next tab label ("Attention (3)"); the Up Next page heading stays "Up Next" and the URL value stays `tab=sessions` | nav, heading, tab |

"Acknowledged" is an internal event kind and never appears in a string (the timeline reads "Marked done" or "Marked not a question"). List chips describe what is true now; the Recent view describes what happened in a window, so its section titles carry the window ("Finished (24 h)", "Changed (24 h)", "Done (24 h)"). "Just finished", "Gone quiet", "Done recently", "Mark all seen", "Needs you", "Dismissed" (for Attention rows) and the separate "calm" empty state are retired.

---

## S1. Attention list

### Desktop (>= 1024 px): list plus optional detail drawer

```
+--------------------------------------------------------------------------------------------+
| Attention                       [Needs you 3] [Finished 4] [Quiet 2] [Recent]            [?]   |
| Live: connected - last update 14:02:11   (plain text, not a live region)                    |
+--------------------------------------------------------------------------------------------+
| Oldest waiting: api-refactor, waiting 47 min        (sticky, plain text, NOT a live region)  |
+--------------------------------------------------------------------------------------------+
| NEEDS YOU (3)                                                                                |
| [!] Approval needed      api-refactor     47 min waiting   "Allow Bash: rm -rf build"   ... |
| [?] Asked a question     docs-site        12 min waiting   "Want A or B for the router?" ...|
| [x] Error                infra-lint        3 min waiting   "tests failing: 4"           ... |
|                                                                                             |
| FINISHED (4)                                                       [Mark all finished done] |
| [ok] Finished            blog-pipeline    2 h ago          "Draft is written"           ... |
| [ok] Finished            ssq-fix-413      5 h ago          ...                          ... |
|   show 2 more                                                                               |
|                                                                                             |
| PAUSED (1)  >  (collapsed; no "needs you" styling)                                          |
+--------------------------------------------------------------------------------------------+
```

The "Recent" chip is a navigation control, not a filter toggle: it opens the cross-session Recent activity view (S13), carries no `aria-pressed` (it has `aria-current="page"` while that view is open), whereas Needs you, Finished and Quiet are toggle buttons with `aria-pressed`. There is no "Show hidden" toggle here: hidden sessions never produce attention items, so it would show nothing; their activity is reachable only through S13's "Include hidden sessions" toggle.

Quiet filter replaces the grouped list with sessions that have no open item and `lastMeaningfulChangeAt` older than the Quiet threshold (default 30 min; Settings option 15 / 30 / 60 / 120, per viewer), sorted oldest first, labelled "Quiet (no change in N min)". Each Quiet row shows the session status as text ("Running", "Idle", "Paused"), because a 30-minute build is Running and Quiet while a finished agent at a prompt is Idle and Quiet; the two need different reactions. It is a filter chip, not a group, because quiet is derived state and not an event. The sticky "Oldest waiting" callout is plain text outside any live region (a ticking age inside `role=status` would be re-announced every 30 s, violating UX-17); when the oldest item changes, only the announcer speaks, and only the session name. Quiet is pull-only by design (the chip count and Recent's "Quiet now"); it has no nav cue, because a second nav count would drift toward the flood ADR-007 #1 avoids (decision recorded in ADR-007 alternatives).

### Mobile (375 px): single column, needs-me above the fold

```
+--------------------------------------+
| Attention                      [?]   |
| [Needs you 3][Finished 4][Quiet 2][Recent] -> |  chips scroll horizontally, 44 px tall
| Live - last update 14:02:11          |
+--------------------------------------+
| Oldest waiting: api-refactor 47 min  |
+--------------------------------------+
| NEEDS YOU (3)                         |
| +----------------------------------+ |
| | [!] Approval needed       47 min | |  whole row = primary link (>= 44 px)
| | api-refactor                     | |
| | Allow Bash: rm -rf build         | |
| |                         [ ... ]  | |  overflow button 44x44: Timeline, Done, Mute
| +----------------------------------+ |
| ...                                  |
| FINISHED (4)                         |
|  show 4 more                         |
+--------------------------------------+
```

Mobile shows only the first 3 rows of each group by default, then "Show N more" (44 px button). Swipe is never the only path: every swipe action has the overflow-menu button equivalent (WCAG 2.5.1, 2.5.7).

### Tablet (768-1023 px)

Single column with the desktop row anatomy (S2: 56 px rows; secondary buttons inline when the row container is >= 720 px wide, else the S2 overflow menu), the desktop group cap of 10, and chips that wrap onto a second line instead of scrolling. The timeline overlays the list as a modal drawer (S6). Target size follows the input, not the width: `@media (pointer: coarse)` (any width, including a touch laptop or a tablet at 1024 px) gets 44 px targets; `(pointer: fine)` gets 24 px minimum (UX-31).

### Empty and "nothing needs you" states

- Needs-you is 0 but Finished has rows: the Needs you group is replaced by the affirmation "Nothing needs you. 4 finished sessions are waiting for direction." (the Finished group follows). The empty card appears only when there are no rows at all.
- Detection coverage caveat: the empty text "All caught up. Nothing needs you." is followed, when any running visible session is on a program with no turn-end signal (`ProgramSignalTable` row `none`, or Finished-only), by the line "Question detection isn't available for 2 sessions (aider, pi), so they only show approvals and errors." (S9).

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Opens `/review-queue` (or taps nav badge) | Skeleton rows for <= 300 ms, then snapshot from `GetReviewQueue`; subscribes to `WatchReviewQueue`. Header "Live" status appears |
| 2 | Scans needs-me group | Rows sorted by time-waiting descending within a group; groups ordered: needs-me (approval/question prose or tool/error/tests failing), finished-awaiting-direction, uncommitted/idle-visible reasons |
| 3 | Activates a row (click, Enter, tap) | Navigates to the session; the item is marked seen but stays in the list (S4) |
| 4 | Changes a filter chip | List filters in place; chip counts stay accurate; selection persists in `localStorage` (try/catch, per-viewer convenience only) |
| 5 | List changes while the user is interacting (pointer over list, or focus inside) | New arrivals are not inserted under the cursor; a "2 new - show" button appears at the top of the group; activating it inserts them and announces once. A new `approval_pending` or `error_state` arrival is never held behind this button: when the pointer or focus is inside the Needs you group the button is promoted to a sticky line above the list that names it ("1 new approval - show") and the announcer speaks it at priority 1 (S10); when neither is inside the group the row is inserted at the top |
| 6 | Presses `j`/`k`/arrows while focus is inside the list (S15) | Roving tabindex moves between rows; the focused row's own buttons (Timeline, Not a question, Done, Mute) are reachable with Tab (S15); `Enter` opens; `t` opens timeline; `d` Done (informational rows only); `?` is the existing global shortcut overlay. Outside list focus these keys do nothing from this feature |
| Exit | Back, Esc (from drawer), or navigating away | No state is lost; list state is server-driven. No modal blocks leaving |

### Edge cases

- After a row is removed (Done, Not a question, bulk), focus moves to the next row in the same group, else the previous row, else the group header, else the list heading; it never falls to `body` (UX-46).
- 50+ items: group counts, "show N more" (per-group cap 10 desktop / 3 mobile), sticky oldest-waiting callout, no virtualized-list focus loss (focused row remains rendered).
- Hidden/system sessions: excluded from this list entirely (no toggle). They appear, tagged "hidden" and never notifying, only in S13 when "Include hidden sessions" is on.
- Paused/Stopped/Hibernated: collapsed "Paused (N)" group built client-side by joining the sessions slice (the queue never holds these sessions), neutral styling, no "needs you" colour, no resume action and no resume on view (honors `noarchivedrevival`). Row action is "Open" only.
- Row's session deleted while listed: row animates out (no motion under `prefers-reduced-motion`) and the announcer does not announce a loss; if focus was on it, focus moves to the next row, else to the list heading.

---

## S2. Row anatomy and reasons

Row is a `listitem` of a `ul`; group titles are headings. Primary target is a link (session name + reason + excerpt). Secondary actions are sibling buttons, never nested inside the link (no `role="button"` container; axe `nested-interactive`). Excerpts are rendered as plain text nodes only (no HTML, no markdown).

```
Desktop row (min height 56 px; buttons >= 24 px on `pointer: fine`, >= 44x44 hit area on `pointer: coarse`)

[icon] Reason label   session-name          47 min waiting    [Timeline] [Not a question] [Stop flagging questions]|[Done] [Mute]
       "excerpt, max 2 lines, redacted"     abs time in title  secondary buttons right-aligned

Mobile row (min height 72 px)

[icon] Reason label                               47 min
session-name
"excerpt max 2 lines"
                                                  [ ... ]
```

Row action density (container-width rule; the threshold is measured on the list container with a container query, not the viewport, so a 768-1023 px window with a side drawer and a 1280 px window with a narrow pane behave the same):

| Row container width | Visible on the row | In the overflow menu "More" (44x44 on coarse pointers) |
|---|---|---|
| >= 720 px | all secondary buttons inline (Timeline, Not a question, Stop flagging questions, Done, Mute) | none |
| < 720 px, including 768-1023 px with the drawer open, 320 px reflow and 400% zoom | "Not a question" (prose rows) and "Done" (informational rows), each wrapping onto its own line below the excerpt when needed | Timeline, Stop flagging questions, Mute 1 h, Mute 8 h |

"Not a question" is never moved into the overflow menu at any width or zoom (it is the common, time-critical action). Every overflow item has a keyboard path (Tab to "More", Enter, arrows); no content clips or scrolls horizontally (UX-49).

| Reason | Label (words) | Icon | Visual | Actions | Interrupt |
|---|---|---|---|---|---|
| `approval_pending` | Approval needed | shield | high-priority accent | Open, Timeline | toast + push |
| `input_required` | Waiting for input | keyboard | medium accent | Open, Timeline | toast |
| `prose_question` (new) | Asked a question | speech bubble with "?" | medium accent | Open, Timeline, Not a question, Stop flagging questions for this session, Mute | `INPUT_REQUIRED` delivery: toast (tier 2), web push, Slack, below approvals; counts in badge; subject to the per-session burst guard (S3) and session Mute (push and Slack only) |
| `error_state` / `tests_failing` / `waiting_for_user` | Error / Tests failing / Waiting for input | alert triangle | high accent | Open, Timeline (no Done) | toast; count in badge |
| `awaiting_direction` (new) | Finished, waiting for direction | check circle | neutral/low | Open, Timeline, Done, Mute | one delivered notification per episode: tray record + web push, low urgency, no toast, no Slack by default; capped globally with a digest; mutable; always counted in the non-alerting "N new finished" nav cue until Seen, whether or not push worked (S14); NOT counted in the badge number or title; auto-clears on a new turn or after 24 h untouched (S4) |
| `task_complete` | Task complete | check | neutral/low | Open, Timeline, Done | unchanged (routine, tray only); not counted in badge |
| `stale` / visible `uncommitted_changes` | No change for N min / Uncommitted changes | clock / diff | neutral/low | Open, Timeline, Done | unchanged; not counted in badge |

Rules:
- Reason is always icon plus text label (WCAG 1.4.1); color is supplemental. Text contrast >= 4.5:1; icon and focus outline >= 3:1 in light and dark.
- Excerpt: at most 160 chars; for prose questions the matched question sentence (tail-biased extraction, plan Task 3.1.2a), otherwise the last 160 chars at a sentence boundary, truncated with an ellipsis, secrets redacted server-side; never a raw terminal tail. Prose question rows show the matched sentence so the operator can judge a false positive without opening the session.
- Waiting time: relative text from `useNowTicker` (S12) in a `<time datetime>` with absolute time in `title` and in the drawer header (touch has no tooltip).
- Source tag for uncertainty: a small "detected from screen, unconfirmed" text tag when `TurnEndSource` is not `Hook` on a needs-human row (S14). Absent for hook-sourced items.
- Completion context (decision, ADR-007 #6): a row shows the scrubbed excerpt only; there is no "PR opened / tests passed / files changed" outcome line in this release (payload unverified, per-row diff cost). The timeline holds the full sequence; revisit if Spike 0.1.1 finds an outcome in the Stop payload.
- Seen is server-derived and identical on every device; the Seen/unread difference is a text marker "Seen", never colour alone.
- Programs with `Source=None` never produce these rows; they show the "cannot detect" note in the session detail (see S9).

---

## S3. "Not a question" and question recourse

```
Desktop:  [icon] Asked a question  docs-site   12 min   "Want me to also add tests?"
                                   [Timeline] [Not a question]
After click -> row removed, toast bar at bottom-left (visual only, not a live region; the announcer speaks it, S10):
          "Marked not a question. Undo (Ctrl+Z)"          (10 s, see toast rules below)
```

Toast rules (all undoable actions: Done, Not a question, Mark all finished done, Mute, Recent's Undo; WCAG 2.2.1): the 10 s countdown pauses while the pointer is over the toast or focus is inside it and restarts on leave/blur; the Undo button is a normal tab stop (the toast region follows the list in tab order) and Ctrl/Cmd+Z undoes the most recent action while the list has focus; up to 3 toasts stack, each with its own Undo and timer, extra ones queue; the toast is visual only (no `role=status`, `role=alert` or `aria-live` on it): its text is spoken once by the single announcer (S10), contains no timestamp, and ends with the hotkey hint "Undo (Ctrl+Z)" so keyboard users can find the Undo. The toast is a convenience, not the only recovery: Recent's "Done (24 h)" section keeps Undo for the whole window while the episode is not superseded.

Mobile: "Not a question" is in the row overflow menu AND as a visible text button below the excerpt for prose rows (the common action should not be hidden). 44 px tall.

| Step | User does | System responds |
|---|---|---|
| 1 | Reads excerpt, decides it is not a request for input | n/a |
| 2 | Activates "Not a question" | Optimistic removal; RPC `AcknowledgeSession{dismiss_kind=not_a_question}`; undo toast for 10 s; `attention_prose_question_dismissed_total` increments server side |
| 3a | Clicks Undo within 10 s | `UnacknowledgeSession` for that episode; item restored in place on the next snapshot/refetch; toast "Restored". If a newer reply already replaced it: toast "A newer reply replaced this" and nothing returns |
| 3b | Does nothing | The mark sticks for that turn-end episode only; the next question from the same session raises a new item (ADR-006) |
| 4 | Wants no more question flags from a chatty session | Row action "Stop flagging questions for this session" (also shown as a hint "Frequent questions from this session" once the burst guard engages, ADR-007 #7): persisted per session, no expiry, Undo toast, reversible after the row has cleared from the mute management list and the session card (below), and from the session's Finished rows via "Flag questions for this session". Afterwards its question-shaped turn ends appear as "Finished, waiting for direction". After the 3rd prose-question episode from one session in 30 minutes, later ones are list row, badge and tray record only (no toast, push or Slack). Neither the opt-out nor the guard touches approvals, waiting-for-input or error items |
| 4a | Wants to review or reverse an opt-out later | While an opt-out is set the session's rows and its session card show the text tag "Questions not flagged" (a tag, not colour only), and the mute management list (S14) lists it as "Questions not flagged since Oct 9" with [Flag questions again]. Decision: no auto-expiry (an expiry would silently restart the flood the operator judged noise) but a periodic reminder: once an opt-out is 14 days old the list header shows one dismissible informational line "Questions aren't flagged for 2 sessions. [Review]" that opens the management list; dismissal is per viewer and the line returns after 14 more days. OWNER TO CONFIRM (expiry vs reminder, 14 days) |
| Exit | Esc dismisses the undo toast without undoing | n/a |

Errors:

| Failure | User sees | Exit |
|---|---|---|
| RPC fails (network/5xx) | Row returns, inline message under it: "Couldn't mark it. Check your connection." with [Retry]; the announcer speaks it once, politely ("Couldn't mark docs-site not a question") | Retry, or leave the row (still actionable via Open) |
| Item already cleared server-side (state changed) | Row disappears; toast "Already cleared" | none needed |
| Undo fails | Toast: "Couldn't restore. The next question will show up again." | Dismiss toast |

Do not offer "Not a question" on tool-based questions or approvals (they are real prompts).

---

## S4. Done / Seen semantics

| Action | Allowed on | Effect | Reversible |
|---|---|---|---|
| Open session (implicit Seen) | all rows | `LastViewed` is set server-side; on every device the row gets a small "Seen" text marker (not colour only) and drops its unread emphasis; row stays; a newer turn end makes it unseen again | n/a |
| Done | informational rows only (`awaiting_direction`, `task_complete`, `uncommitted_changes`, `stale`) | Row removed; event `Acknowledged` logged (timeline text "Marked done") | Undo toast 10 s (`UnacknowledgeSession`); the "Done (24 h)" section of S13 lists recently done for recovery, with Undo |
| Done on needs-me row | not offered | The button is absent (not disabled); keyboard `d` on a needs-me row announces "This needs a reply; Done isn't available" via the announcer | n/a |
| Auto-clear (no action) | `awaiting_direction` only | The row leaves the list by itself when the session starts a new turn (new meaningful output or operator input after `TurnEnded.At`) or when the item is 24 h old and untouched; a `task_complete`/`stale`/`uncommitted_changes` row follows its own state change. Nothing is announced for an informational row leaving | Recovery path: Recent (S13) keeps the turn end under "Finished (24 h)" and the 7 d window, each with a "Now:" state marker and Open/Timeline, so an auto-cleared finish is still findable; the timeline keeps the `TurnEnded` and `AttentionCleared` entries |
| "Mark all finished done" | group header of informational rows | Affects only informational rows, never needs-me (tray ADR-008); the server (`AcknowledgeSessions`) also rejects a needs-me row. Confirmation is not modal; undo toast with count: "4 marked done. Undo" | Undo within 10 s |

Errors: partial bulk failure shows "3 of 4 marked done. [Retry remaining]"; rows that failed stay in the list.

---

## S5. Tray to list linking

```
Tray row (NotificationItem):
[?] docs-site asked a question - 14:02            [View in Attention]  [Dismiss]
     "Want A or B for the router?"
```

| Step | User does | System responds |
|---|---|---|
| 1 | Activates a tray row or "View in Attention" | Navigates via `@/lib/nav` to `/review-queue?session=<id>` |
| 2 | Page loads | List scrolls the matching row into view and moves focus to it once (programmatic focus, visible ring); row briefly highlighted (no animation under reduced motion; a persistent "Linked" text tag until the user interacts) |
| 3 | Clicking Dismiss in tray | Tray row clears only; the Attention item is untouched. Copy in the tray tooltip/help: "Dismissing here doesn't remove it from Attention" |

Edge and error cases:

| Case | Behavior | Exit |
|---|---|---|
| Item already cleared (operator answered, or Done elsewhere) | List loads; inline banner above list: "docs-site is no longer waiting. [Open session] [Open timeline]". There is no row to focus, so focus moves once to the banner's heading (`tabindex=-1`) and the announcer speaks "docs-site is no longer waiting" once, politely | Open session, open timeline (shows what happened), or close banner |
| Session in a filtered-out group (e.g. Quiet or Paused) | Filters reset to show it; banner "Showing docs-site (filters cleared)" (announced once like the row above; focus goes to the row) | Re-apply filters |
| Unknown or deleted session id | Banner: "That session no longer exists. [Back to Attention]" and the normal list renders | Dismiss banner |
| Flag `attention_inbox_consolidation` on | Redirect (S11) preserves `session=` | n/a |
| Toast itself (transient) | Never the only record; the toast "View" action uses the same link | n/a |

---

## S6. Session event timeline (drawer / bottom sheet)

Opened from: a timeline button on each list row, the `SessionCard` ("Timeline" button next to the Last activity label), and the S5 "no longer waiting" banner. Reaching it takes <= 2 clicks from either place.

### Desktop: right-side modal drawer (width 400 px, resizable not required)

Decision (triad iter 1): the drawer and the sheet are MODAL on every width: `role="dialog"`, `aria-modal="true"`, `aria-labelledby` the heading, rest of the page `inert`, focus trapped, backdrop click and Esc close. Layout: >= 1024 px the list is two-pane and the 400 px drawer overlays the right pane; 768-1023 px the list is single-column and the 400 px drawer overlays it (no side-by-side); < 768 px the bottom sheet. A non-modal side panel was rejected because the list rows behind it keep their shortcuts and focus order, which makes the focus model ambiguous.

```
+-----------------------------------------+
| docs-site                      [Close X]|  <- heading level 2, aria-labelledby
| Last activity: Oct 23, 14:02:11 (3 h ago)|  <- absolute always visible here
| Working tree now: +120 -14 (existing read)|  <- one GetSessionDiff call when opened; "now", not "at completion"; omitted on error
| Created: Oct 22, 09:10                   |
+-----------------------------------------+
| Filter: [All v]                          |  all / attention only / lifecycle
+-----------------------------------------+
| Today                                    |  <- date heading (h3) when entries span days
| <ol>                                     |
| 14:02:11  Finished, waiting for direction|
|           "Draft is written. Anything..."|
| 13:58:40  Status changed: Running -> Idle |
| 13:41:02  Acknowledged (Not a question)  |
| 13:40:55  Asked a question               |
| Yesterday                                |
| 22:10:03  Started                        |
| ... (50 per page)                        |
| [Load older]                             |
| -- Older events trimmed --               |  <- only when trimmed=true
+-----------------------------------------+
| [Open session]                           |
+-----------------------------------------+
```

### Mobile: bottom sheet (max height 85 vh, content scrolls inside)

```
+--------------------------------------+
|              ____  (drag handle)     |
| docs-site                    [Close] |  Close button 44x44, always present
| Last activity Oct 23, 14:02:11       |
| Created Oct 22, 09:10                |
+--------------------------------------+
| 14:02  Finished, waiting for direction|
|        "Draft is written..."          |
| ...                                   |
| [Load older]                          |
| [Open session]                        |   padding-bottom: env(safe-area-inset-bottom)
+--------------------------------------+
```

### Interaction flow

| Step | User does | System responds |
|---|---|---|
| 1 | Activates "Timeline" on a row/card | Opens the modal drawer/sheet; focus moves to the heading (or Close button); background gets `inert`; skeleton rows while `ListSessionEvents{session_id, limit:50}` loads |
| 2 | Reads events | `<ol>` newest first, grouped under date headings ("Today", "Yesterday", then "Oct 21") whenever the loaded page spans more than one calendar day; each entry is `<time datetime>` + reason in words + optional excerpt (redacted, <= 160 chars, plain text). Entries are text, not icon-only. The first open also calls `GetSessionDiff` once for the "Working tree now" line |
| 3 | Activates "Load older" | Fetches next page; focus stays on the button; new entries appended; "50 older events loaded" is sent to the single announcer (S10), which sits outside the inert page so it still speaks while the dialog is open |
| 4 | Activates "Open session" | Navigates to the session; drawer closes |
| Exit | Esc, Close, backdrop click, or (mobile) swipe down on the handle | Drawer closes; focus returns to the opener element (or, if it no longer exists, to the list heading). A swipe is never the only path |

Keyboard and a11y: focus trap while open (Tab cycles inside; Shift+Tab from the heading goes to the last control); Esc closes; no trap that blocks Esc; bottom sheet must not cover the focused element when the virtual keyboard is open (sheet uses `visualViewport` height); transitions disabled under `prefers-reduced-motion`; target size 44 px on touch.

Event wording map (exhaustive over `SessionEventKind`; unknown kinds render "Event: <kind>" rather than crashing):

| Kind | Text |
|---|---|
| StatusChanged | Status changed: A to B |
| TurnEnded | Finished a turn |
| AttentionRaised (reason) | Needs you: <reason label> |
| AttentionCleared | No longer waiting (cleared by <you / state change>) |
| Acknowledged | Marked done / Marked not a question (undone: "Restored") |
| NotificationSuppressed | Notification held back (<gate reason in words>) |
| LifecycleStarted/Exited/Stopped | Started / Exited / Stopped |
| ApprovalResolved | Approval resolved |

### States

| State | Display | Exit |
|---|---|---|
| Loading | Three skeleton rows, `aria-busy="true"` | Close |
| Empty (flag on, no events yet) | "No events recorded for this session yet. New activity will appear here." | Close / Open session |
| Flag `session_event_log` off | "Event history isn't turned on. Turn it on in feature flags." with link to the flag panel; button hidden on cards when the server does not advertise the flag | Close |
| RPC error | Inline in the sheet: "Couldn't load history. [Retry]" Retry re-requests; list header still shows "Last activity" from the session | Close |
| Offline (browser or stream down) | Cached entries (if any) stay at full contrast with an "Offline" text tag and the plain-text line "Showing what was loaded at 14:02:11" (not a live region); Retry disabled until online | Close |
| Partial page failure on "Load older" | Existing entries remain; error line under the button with Retry | Close |
| Retention bound reached | Last row: "Older events trimmed" (non-interactive) | n/a |
| Session deleted while open | "This session no longer exists." with [Close] | Close |

Privacy: excerpts are redacted and capped; no full terminal tails.

---

## S7. SessionCard "Last activity" and "Created"

Today: `Updated <relative>` from `updatedAt`, tooltip with created/updated (research/ux.md section 4). New: two labelled values; "Updated" is removed from the card but `updatedAt` still drives sorting (ADR-005).

```
Desktop card footer
 Last activity 3 h ago  |  Created Oct 22        [Timeline]
 (title attr on label: "Oct 23, 2026, 14:02:11")

Mobile card (two lines, labels not truncated)
 Last activity 3 h ago          [Timeline]
 Created Oct 22
```

| Aspect | Spec |
|---|---|
| Source | "Last activity" = `lastMeaningfulChangeAt`; "Created" = `createdAt`, static, absolute date (relative if < 24 h: "Created 3 h ago") |
| Format | `formatRelativeTime(then, now)`: "just now", "5 min ago", "3 h ago", "2 d ago"; absolute date after 7 d |
| Ticking | Existing `useNowTicker` (30 s default); never a per-card interval; not inside any `aria-live` region |
| Absolute access | `title` attribute (pointer) AND tapping/activating the label opens the drawer, whose header shows the absolute time (touch/keyboard path; no hover dependence) |
| Reload | Value identical before/after reload at the same wall time (derived from the server timestamp only) |
| Fallback: field absent (older server) | Render the old `updatedAt` text with the old label "Updated"; no mixed labels |
| Fallback: value missing | "No activity recorded" (not "Never", not "N/A") |
| Markup | `<time dateTime="2026-10-23T14:02:11Z">3 h ago</time>` |
| Target size | Timeline button and tappable label >= 44x44 on touch, >= 24 px on pointer |

Flow: user glances at card (read-only) -> to know exactly when, activates the label/Timeline -> drawer (S6) with absolute time. Error states: if the drawer fails, S6's error table applies; the card text is unaffected. Exit: Close/Esc.

Edge: a session with a recent rename or pin but old activity continues to show the old "Last activity" (that is the point of ADR-005). Add `title` tip on the label: "Changes when the session produces output or changes state. Renaming doesn't count."

---

## S8. Up Next "Sessions" tab (flag `attention_inbox_consolidation`, default off)

### Desktop

```
+-----------------------------------------------------------------------+
| Up Next                                                               |
| [Attention (3)] [PRs (5)] [Backlog (2)]                                |
+-----------------------------------------------------------------------+
|  (ReviewQueuePanel body: same filters, groups, rows, drawer)           |
+-----------------------------------------------------------------------+
```

### Mobile 375 px

```
+--------------------------------------+
| Up Next                              |
| [Attention 3][PRs 5][Backlog 2] ->   |  horizontally scrollable, each tab >= 44 px tall
|                                      |  fade/edge cue shows more tabs exist
+--------------------------------------+
| (panel body)                         |
```

Rules:
- Tab order and default per ADR-001/plan Story 6.1.1: `?tab=` wins, then (if flag on AND needs-me count > 0) `sessions`, then stored tab, then `prs`. With the flag off the tab set is exactly today's.
- The tab is labelled "Attention" (vocabulary table; the URL value and code id stay `sessions`). Its badge equals the nav badge (same array); it counts needs-you only, not informational rows. Badge has a text alternative: tab accessible name "Attention, 3 need you".
- Selected tab announced via `aria-selected`; arrow keys move between tabs; the tab strip scrolls the focused tab into view.
- The tab body is `ReviewQueuePanel` composed unchanged; the 1 Hz "last scanned" ticker on the PR tab stays inside the PR tab only.
- The attention stream is lifted to `UnfinishedTab`, so switching tabs does not drop the subscription; badge updates while on another tab.

| Step | User does | System responds |
|---|---|---|
| 1 | Opens `/unfinished` | Resolves default tab per rule above; tab strip rendered with badges |
| 2 | Switches tab | URL gets `?tab=`; focus stays on the tab; panel body swaps; tab choice stored in `localStorage` (try/catch) |
| 3 | Arrives from a `/review-queue?session=x` redirect | `tab=sessions&session=x` preserved; S5 scroll/focus applies |
| Exit | Any tab, or nav away | none |

Errors and states: attention stream disconnect shows the S9 banner inside the Sessions tab and a small "!" text marker on the tab badge ("Attention, updates paused"); PR/backlog tabs unaffected. If the attention RPC fails on load, Sessions tab body shows the S9 error card with Retry while other tabs work. Zero count: tab shows no badge (not "0").

---

## S9. Empty, error, offline and flag states (applies to S1, S6, S8)

| State | Where | Copy / behavior | Exit path |
|---|---|---|---|
| Initial loading | list | Skeleton rows, `aria-busy`; no spinner-only | Wait, Back |
| Empty (no items) | list | One stable wording: "All caught up. Nothing needs you." + "N sessions working" + "Last checked 14:02:11" (plain text). It does not depend on in-memory history, so a server restart cannot change it; empty is distinguished from broken by the "Last checked" time and by the error card below. Coverage caveat: when any running visible session is on a program without a turn-end signal, a second line states it ("Question detection isn't available for 2 sessions (aider, pi), so they only show approvals and errors."). When Needs you is 0 but Finished has rows, the affirmation "Nothing needs you. N finished sessions are waiting for direction." replaces this card (S1) | Link "View sessions" |
| Empty, filtered | list | "No sessions match 'Quiet'. [Clear filters]" | Clear filters |
| RPC error on load | list | Card: "Couldn't load the attention list. [Retry] [Open sessions list]" and the technical code in a collapsible "Details" | Retry, or Sessions list fallback |
| Stream disconnected (> 10 s) | banner above list, nav badge | Visible banner: "Live updates paused, last update 14:02:11 (absolute). Reconnecting..." The banner is NOT a live region (it holds a timestamp, UX-17); the announcer says only "Live updates paused". Stale items keep full text contrast (no opacity change, UX-28): the group header gets a "stale" text tag and a clock icon, and rows get a dashed left border. The nav badge gets a text suffix and its accessible name reads "Attention, 3 need you, may be out of date" until reconnect (the operator on another page must not trust a frozen count). [Retry now] button | Retry now; auto-reconnect with backoff |
| Reconnected | banner, nav badge | Banner, stale tags and nav suffix clear; snapshot refetched; no "new item" announcements for items that already existed; one polite "Updates resumed" message (no time) | n/a |
| Offline (navigator offline) | banner | "You're offline. Showing what was last loaded at 14:02:11." (visible text, not in a live region; the announcer says "You're offline") Actions that need the server (Done, Not a question) are disabled with visible reason text, not just greyed | Reconnect (auto) |
| Action RPC failed | row | Inline message + Retry (S3); the announcer speaks it once, politely (never assertive) | Retry or leave |
| Program cannot be detected (`Source=None`, e.g. unsupported agent) | session detail / card tooltip | "Waiting-for-you detection isn't available for <program>. Sessions of this type only appear for approvals and errors." | Dismiss tooltip |
| Detection flag off | list | Behaves as today; no new reasons; no mention in UI | n/a |
| Event log flag off | S6/S7 | See S6 table; card falls back to "Updated" label | n/a |
| Large backlog | list | Group caps + "Show N more" (S1); bulk "mark all finished done" only for informational rows | n/a |
| Permission/auth failure (401/403) | list | "You're signed out of this server. [Reload]" | Reload |

No dead ends: every error state carries at least one action (Retry, Clear filters, Open sessions list, Reload, Close).

---

## S10. Announcer, nav badge, document title

One announcer speaks everything; nothing else on the page is a live region.

| Element | Spec |
|---|---|
| Announcer | `AttentionAnnouncer`: ONE visually hidden `aria-live="polite" aria-atomic="true"` region mounted at the app shell, a sibling of the page root and OUTSIDE any subtree that becomes `inert` or `aria-hidden` when the timeline dialog opens, so a needs-you arrival is still spoken with the dialog open. The existing region in `ReviewQueuePanel`, the timeline sheet's own region and Recent's own region are removed; all of them call the announcer. Never `assertive`; no `role=alert` anywhere in this feature |
| Priority queue | Messages carry a priority: 1 needs-you arrival ("3 sessions need you, 1 new", "1 new approval"), 2 action result ("Marked done", "Restored", "Couldn't mark docs-site not a question", "50 older events loaded", "docs-site is no longer waiting"), 3 connection state ("Live updates paused", "Updates resumed", "You're offline"). One message is spoken at a time; the queue holds at most 3 entries; a higher priority goes first; same-kind messages inside a 2-3 s window coalesce into one; an entry older than 10 s is dropped. An inline error is queued once (priority 2) when it first appears and never re-announced on re-render |
| Content rules | Counts only plus the newest session name when exactly one arrives ("docs-site asked a question"). No excerpts (privacy, length). No timestamps and no relative ages in ANY announcement. Info-only changes (new `awaiting_direction` rows) are not announced |
| Toasts | Visual only: no `role=status`, `role=alert` or `aria-live` on a toast or its container (the container is a labelled `group` so the Undo buttons are reachable); the toast text is sent to the announcer exactly once. A toast never contains a timestamp: the mute toast reads "Alerts muted for 1 hour. Undo (Ctrl+Z)" and the absolute "Muted until 16:20" is static text on the row/header, not in the toast |
| Plain-text status lines | "Live: connected - last update 14:02:11", "Oldest waiting: api-refactor, waiting 47 min", the disconnect and offline banners, "Last checked 14:02:11" and the drawer's offline line are ordinary text with no live role on them or any ancestor (their ages tick or hold an absolute time) |
| Focus | Never moved on arrival of new items. The only programmatic focus: the S5 deep link, once (to the row, or to the "no longer waiting" banner heading when there is no row) |
| Reorder | No reordering of rows under focus or pointer; "N new - show" button instead (S1), except that a new approval or error is promoted to a sticky line (S1 step 5) and spoken at priority 1 |
| Nav cue | Separate from the badge number: text "N new finished" for items not Seen on any device (S14). No animation, no `aria-live`; visible text "muted" while a global mute is active; accessible name "Attention, 2 need you, 3 new finished" (+ ", 1 unconfirmed", + ", alerts muted", + ", may be out of date" when applicable) |
| Nav badge | Count = needs-you items (the needs-human reasons in plan.md Attention Reason Classification; informational rows such as `awaiting_direction` never count). The Sessions-tab badge and the document title use the same selector. Accessible name "Attention, 3 need you". Hidden when 0. Badge text is also present as text, not a coloured dot only |
| Document title | "(3) Stapler Squad" when needs-you count > 0; plain title otherwise. Updated on count change only |
| Reduced motion | No pulse/flash on new items under `prefers-reduced-motion`; use a static "New" text tag until seen |
| Connection loss | The announcer says "Live updates paused" with no time (the visible banner carries the time as non-live text), then "Updates resumed" |
| Mobile | Nothing relies on audio/vibration; push and toast are separate channels and unchanged |

## S11. `/review-queue` redirect (condensed, non-interactive)

Sample: `/review-queue?session=abc&filter=needs` with `attention_inbox_consolidation` on becomes `/unfinished?tab=sessions&session=abc&filter=needs` (replace, not push, so Back does not loop).

Acceptance bullets:
- Flag off: page renders as today; flag on: redirect via `Redirect` from `@/lib/nav`, all query params preserved.
- No flash of the old page content before redirect (render nothing or a skeleton while the flag resolves); if the flag fetch fails, the old page renders (fail to the legacy surface).
- Browser Back from the new location returns to the page before the redirect, not to `/review-queue`.
- Document title and screen reader context update to "Up Next" and the focus lands on the page heading (or on the linked row if `session=` is present).
- Keyboard shortcut, omnibar command, Slack deep links, onboarding text keep working (either via redirect or re-pointed to canonical URL).

## S12. Time formatting contract (condensed, non-interactive)

Sample: `formatRelativeTime(11:55:00Z, 12:00:00Z)` returns `"5 min ago"`; 8 days earlier returns `"Oct 15"` (absolute); `undefined` returns `"No activity recorded"`.

Acceptance bullets:
- One pure formatter and the existing `useNowTicker` hook (default 30 s; no new hook); exactly one interval for 200 rows; paused when `document.hidden`; refreshes on `visibilitychange`.
- The same age reads identically on card, list, tray, and drawer.
- Absolute timestamp is always available via `title` AND a non-hover path (drawer header, or visible text after 7 days).
- `<time dateTime>` is used wherever a timestamp is rendered.
- Locale-aware absolute format via `Intl.DateTimeFormat`; no raw ISO strings shown.

## S13. Recent activity view (cross-session)

Answers "what just finished, what has gone quiet, what changed" for every session, including ones marked Done, ones that never raised an item and (optionally) hidden ones. Opened by the "Recent" chip in S1 (`/review-queue?view=recent`; no new route). Rows are grouped under date headings ("Today", "Yesterday", "Oct 21") whenever a loaded window spans more than one calendar day, and every Finished row carries a current-state marker "Now: Running / Idle / Needs you / Done / Paused" read from the sessions slice, so a stale "finished" entry is not mistaken for the session's present state. Backed by cross-session `ListSessionEvents` (kinds/time filters, `before_occurred_at` cursor) plus the sessions slice for "Quiet now". Chips on the list show what is true now; this view shows what happened in a window, so each section title carries the window. "Changed" is exactly {AttentionRaised, AttentionCleared, StatusChanged, ApprovalResolved}; consecutive StatusChanged events of one session within 2 minutes collapse into one row ("Status changed 6 times") and each session shows at most 5 Changed rows with "Show N more" (flapping cap).

```
Desktop
+------------------------------------------------------------------------------------+
| Attention > Recent      [1 h][24 h][7 d]  [Finished][Changed] [x] Show done          |
|                                              [ ] Include hidden sessions      [?]    |
+------------------------------------------------------------------------------------+
| FINISHED (24 h) (4)                                                                  |
|  blog-pipeline      14:02  Now: Idle           "Draft is written"    [Timeline] [Open]|
|  ssq-fix-413        11:40  Now: Needs you      "Want A or B?"        [Timeline] [Open]|
| QUIET NOW (2)  (no change in 30+ min, no open item; oldest first; Running or Idle)    |
|  infra-lint         45 min                                          [Timeline] [Open]|
| CHANGED (24 h) (6)                                                                    |
|  api-refactor       Needs you: Approval needed   13:55              [Timeline] [Open]|
| DONE (24 h) (2)   (when Show done is on)                                              |
|  docs-site          Marked not a question        13:20  [Undo]      [Timeline] [Open]|
|                                   [ Load older ]  /  Older events trimmed             |
+------------------------------------------------------------------------------------+

Mobile 375 px: single column, chips scroll horizontally (44 px), rows >= 72 px, sections collapsible,
"Load older" 44 px, Timeline/Open in the row overflow menu (no swipe-only path).
```

| Step | User does | System responds |
|---|---|---|
| 1 | Activates "Recent" (from the nav badge that is 2 clicks) | View opens; skeleton rows <= 300 ms; `ListSessionEvents{session_id:"", kinds, since, limit:50}`; "Quiet now" is computed immediately from the sessions slice (works with the event log off) |
| 2 | Changes the time window (1 h / 24 h / 7 d, default 24 h) or kind chips | Re-requests with the new `since`/`kinds`; selection kept in `localStorage` (try/catch) |
| 3 | Toggles "Show done" | "Done (24 h)" section lists `Acknowledged` events; Undo available while the episode is not superseded |
| 4 | Toggles "Include hidden sessions" | Hidden sessions' events appear with a "hidden" tag; viewing never notifies |
| 5 | Activates "Load older" | Next page via `before_occurred_at`, appended; focus stays on the button; the announcer says "50 older events loaded" |
| 6 | Activates Timeline / Open on a row | S6 drawer/sheet, or navigate to the session (never resumes a paused session) |

States: flag `session_event_log` off shows "Event history isn't turned on" with a flag-panel link while "Quiet now" still works; RPC error shows an inline message with Retry; failed "Load older" keeps loaded entries with Retry under the button; offline keeps cached entries at full contrast with an "Offline" text tag and the plain-text line "Showing what was loaded at HH:MM:SS"; deleted sessions render their stored title with a "deleted" tag and Open disabled.

---

## S14. Unseen-finished cue, mute and unconfirmed signal

Why: a row existing is not the same as the operator having been told. Push is best-effort and its reach cannot be detected server-side, so the nav carries one quiet, honest cue that does not depend on push at all (ADR-007 #1).

```
Nav entry (desktop sidebar)
  Attention  [2]  3 new finished  muted     <- badge number = needs-you only; the rest is plain text, no animation
  accessible name: "Attention, 2 need you, 3 new finished, 1 unconfirmed, alerts muted"

Mobile bottom nav (width budget: one item is ~25% of 375 px)
  icon + badge [2] on the icon; label "Attention"; second text line "3 new finished  muted" (shows "9+ new finished"
  above 9; "muted" is the global-mute marker and stays visible, alone when no finished session is unseen;
  "1 unconfirmed" is in the accessible name only). Badge and "N new finished" are hidden at 0 (no "0").

List header (informational, device-local, dismissible; never gates the cue)
  Push is off on this device, so finished sessions will not buzz you. They still count as "N new finished" in the nav.   [Got it]

Row menu:  [Timeline] [Not a question] [Stop flagging questions for this session] [Done] [Mute 1 h] [Mute 8 h]
Header:    [Mute finished-session alerts v]  1 h / 8 h        -> "Muted until 16:20"  [Unmute]
           the control opens the mute management list (below)
Mobile header: the same control is one 44x44 "More" button beside [?] in the Attention header; it opens a bottom sheet
           with Mute 1 h / Mute 8 h and the management list, so it takes no vertical space above the Needs you rows

Mute management list (popover on desktop, bottom sheet on mobile; reachable after any row has cleared)
  Alerts muted for all finished sessions, until 16:20                     [Unmute]
  docs-site   Muted until 15:10                                           [Unmute]
  api-docs    Questions not flagged since Oct 9                           [Flag questions again]
  (empty: "Nothing is muted and every session's questions are flagged.")
Unconfirmed row:  [?] Asked a question  docs-site  detected from screen, unconfirmed   "Want A or B?"
List header: "1 unconfirmed"
```

The cue rule has no branches: N = visible-session `awaiting_direction` items with `LastViewed < TurnEnded.At`, identical on phone and desktop. Mute durations are the same for both scopes (1 h, 8 h; max 24 h). ONE global-mute marker rule on every page and form factor: a global mute shows the short visible word "muted" on the nav cue line (inline after the cue on desktop, on the second text line on mobile) and ", alerts muted" in the accessible name, so the operator on another page can see alerts are off without opening anything. The cue says "N new finished" (unseen `awaiting_direction` only, visible in text as "new"); the list chip "Finished N" counts every open informational row (vocabulary table).

| Step | User does | System responds |
|---|---|---|
| 1 | A visible session finishes a turn | Tray record created; push is attempted (best effort); the nav text cue increments silently regardless of the push outcome; no toast, sound, animation or live-region speech |
| 2 | Opens the session on either device | `MarkViewed` sets `LastViewed` (ADR-007 #2); the cue drops on every device at the next snapshot |
| 3 | Chooses "Mute 1 h" on a noisy session | `SetNotificationMute`; the row shows static text "Muted until HH:MM"; the toast reads "Alerts muted for 1 hour. Undo (Ctrl+Z)" with no time; the row stays in the list and still counts in the cue; the session's `prose_question` push and Slack are also held, its toast, badge count and row are not (ADR-007 #4) |
| 4 | Chooses "Mute finished-session alerts 8 h" | Global mute, same static display in the header and the visible "muted" marker with the nav cue (accessible name ", alerts muted"); `prose_question`, approvals and errors are not muted |
| 5 | Sees "detected from screen, unconfirmed" | Opens the session to verify; the item still counted in the badge |
| 6 | Sees "Frequent questions from this session" on a prose row | Uses "Stop flagging questions for this session" (S3 step 4) |

Errors: mute RPC fails -> inline "Couldn't mute. [Retry]" (announced once, polite); the row is unchanged. A mute older than its expiry disappears without a reload. Push capability detection failing (no `Notification` API) only shows the informational header line; the cue is shown either way.

## S15. Keyboard shortcuts (focus-scoped), setting, undo and focus placement

| Aspect | Spec |
|---|---|
| Scope | `j`, `k`, arrows, `Enter`, `t`, `d`, Esc act only while focus is inside the list container (WCAG 2.1.4 mechanism: active only on component focus). Never in inputs, textareas, selects, contenteditable (the omnibar), dialogs, the terminal, or the session detail view (focus scope excludes it; there is no bare-`t` binding to collide with: `SessionDetailView.tsx:628-634` binds Ctrl+T, id `terminal.spawn-shell`, context `terminal`) |
| Conflicts | `?` is NOT re-registered: the global `shortcuts:open` (`CockpitShell.tsx:51`) opens the overlay, which gains an "Attention list" section. `j`/`k`/`?` are reserved list-navigation keys in the registry (`RESERVED_SEQUENCE_KEYS`), consistent with this use |
| Setting | Settings > Keyboard shortcuts > "Attention list single-key shortcuts", default on, stored per viewer (`localStorage`, try/catch). Off makes the handlers inert; arrows, Enter and Tab keep working. No per-key remap: the registry has none (plan Verification Notes) |
| `d` on a needs-me row | Announcer (priority 2): "This needs a reply; Done isn't available" |
| Row buttons | Roving tabindex covers rows only. Only the current row's own buttons (Timeline, Not a question, Stop flagging questions, Done, Mute) are tab stops (`tabindex=0`); every other row's buttons are `tabindex=-1`. So Tab from a focused row's link walks that row's buttons, then leaves the list; `j`/`k`/arrows move to another row and re-point the roving stop. A row's buttons are never unreachable by keyboard |
| Undo | Ctrl/Cmd+Z in the list undoes the most recent action; the toast Undo is also a tab stop and the toast text ends with "Undo (Ctrl+Z)" so the hotkey is discoverable |
| Focus after removal | next row in group, else previous, else group header, else list heading; never `body` |

## S16. First-run help for the new labels

On the first visit after `prose_question` or `awaiting_direction` becomes visible, a dismissible, non-modal disclosure (never moves focus) is rendered COLLAPSED as one line, "New labels: what they mean [Show]", placed below the Needs you group (above the Finished group; first only when Needs you is empty), so at 375 px it never pushes a needs-you row below the fold. Expanded, it lists in one line each: "Asked a question" (the agent asked in plain text), "Finished, waiting for direction" (turn ended, no question), "Task complete", "Quiet" (no change for N minutes; check Running vs Idle), "detected from screen, unconfirmed", "N new finished" (finished sessions you have not opened). Dismissal is stored per viewer (try/catch) and the panel is re-openable from the `?` overlay's Attention section. Copy avoids the word "paused" except for `Status=Paused` sessions.

## UX acceptance criteria

Numbering is stable for test mapping. Each is testable by a human or by the named e2e/jest check.

### Findability and flow

1. UX-01 From any page, the operator reaches the list of sessions that need them in <= 1 click (nav badge) and sees needs-me rows without scrolling at 375x812 and 1280x800.
2. UX-02 A prose-question item shows reason text "Asked a question", an icon, and the matched excerpt; the operator can decide it is a false positive without opening the session (0 extra clicks).
3. UX-03 "Not a question" clears a prose item in 1 click and offers Undo for 10 s; the control is absent on approval/tool-input/error rows. "Stop flagging questions for this session" is on every prose row, persists, has Undo, and afterwards that session's question-shaped turn ends appear as Finished; after the 3rd prose-question episode in 30 minutes from one session the row shows "Frequent questions from this session" and later ones raise no toast, push or Slack (approval/waiting-for-input/error alerts are never affected).
4. UX-04 An `AwaitingDirection` item appears in the list and produces exactly one low-urgency tray record plus web push per turn-end episode (no toast, no Slack by default, at most one push per session per 5 minutes and 6 per 15 minutes across sessions, the rest collapsed into one digest push); it is not counted in the nav badge number or title; opening the session does not remove it; "Done" removes it; "Done" is absent on needs-me rows.
5. UX-05 Clicking a tray row lands on `/review-queue?session=<id>` with the row scrolled into view and focused once, in 1 click; if the item is already cleared, a banner explains it, is spoken once by the announcer, receives focus once (its heading), and offers Open session and Open timeline.
6. UX-06 The timeline is reachable in <= 2 clicks from a list row and from a SessionCard, on desktop and mobile.
7. UX-07 "Mark all finished done" never changes a needs-me row and is undoable (toast rules in S3).
8. UX-08 Intentionally Paused/Stopped sessions appear only in a collapsed "Paused (N)" group with no needs-you styling, and viewing them never resumes them.
9. UX-09 With 55 items, groups show counts, "Show N more", and a sticky oldest-waiting callout; the operator reaches any item in <= 3 interactions.
10. UX-10 The Sessions tab on `/unfinished` shows a badge equal to the nav badge, is first/default only when the flag is on and needs-me count > 0, and with the flag off the tab set is unchanged.

### Timeline and time

11. UX-11 The drawer (>= 768 px) and bottom sheet (< 768 px) list events newest-first as an `<ol>` of `<time>` entries with reason in words under date headings when the page spans days; the header shows the absolute time of last activity and, once per open, "Working tree now: +A -D" from the existing `GetSessionDiff` (omitted on error, never labelled as the state at completion). Recent's rows carry the same date headings and a "Now: <state>" marker on Finished rows.
12. UX-12 Esc, Close, backdrop click close the drawer; on mobile a Close button exists in addition to the drag handle; focus returns to the opener element.
13. UX-13 When retention dropped rows, the last entry reads "Older events trimmed".
14. UX-14 SessionCard shows "Last activity <relative>" and "Created <date>"; the text is identical before and after a browser reload at the same wall time; a rename or pin does not change "Last activity".
15. UX-15 Missing value renders "No activity recorded"; a server without the field renders the legacy "Updated" text with no mixed labels.
16. UX-16 Relative time is used for < 7 days and an absolute date after; absolute time is reachable by touch without hover.
17. UX-17 There is exactly one ticking interval for all timestamps on a page, and exactly ONE live region (the announcer, S10) exists in the feature: no `aria-live`, `role=status` or `role=alert` node anywhere else (callout, banners, toasts, drawer, Recent), and the announcer is mounted outside every subtree that becomes `inert` when the dialog opens. No announcement or toast contains a timestamp or relative age (mute copy is "Alerts muted for 1 hour"; the absolute "Muted until 16:20" is static text). The test walks the DOM for live nodes and asserts that count, and that a needs-you arrival is spoken while the timeline dialog is open.

### Error, empty and offline

18. UX-18 Every error state in S3, S6, S9 shows a specific message and at least one exit action (Retry, Clear filters, Open sessions list, Reload, Close).
19. UX-19 Stream disconnect for > 10 s shows "Live updates paused, last update HH:MM:SS" (plain text) with a "stale" text tag, clock icon and dashed row border and NO opacity change; reconnect refetches the snapshot and does not announce already-known items as new.
20. UX-20 Empty states distinguish "all caught up" (one stable wording that does not depend on in-memory history) from "filtered" and always show "last checked <time>" so empty is distinguishable from broken (the error card covers broken). When a running visible session has no turn-end signal the empty state names how many sessions and which programs are not covered; when Needs you is 0 but Finished has rows the affirmation "Nothing needs you. N finished sessions are waiting for direction." is shown instead of the empty card.
21. UX-21 Failed Done/Not a question restores the row and shows "Couldn't ... [Retry]"; the row remains usable via Open.
22. UX-22 Offline: actions that need the server are disabled with visible reason text.
23. UX-23 Flag-off behavior: with `attention_prose_question_detection` off no new reasons or controls render; with `session_event_log` off the timeline button is hidden or explains how to enable it, and the card falls back to the legacy label.

### Accessibility

24. UX-24 The list, drawer/sheet and Sessions tab pass Axe (WCAG 2.2 AA) in empty, populated, drawer-open and disconnected states, desktop and 375 px.
25. UX-25 Rows have no nested interactive controls (no `nested-interactive` violation): primary link plus sibling buttons.
26. UX-26 Keyboard-only operation: roving tabindex, `j/k`/arrows, `Enter` open, `t` timeline, `d` Done (informational only), the existing global `?` overlay, Esc closes overlays; visible focus ring (2.4.7) not obscured by the sticky callout or bottom sheet (2.4.11).
27. UX-27 New arrivals trigger one debounced polite announcement ("3 sessions need you, 1 new") per burst; focus never moves; rows do not reorder under focus/pointer; a "N new - show" button appears instead, EXCEPT a new approval or error, which is promoted to a sticky "1 new approval - show" line and announced at priority 1 so it is never buried behind a hold.
28. UX-28 Status is never colour-only: each reason has icon plus text; text contrast >= 4.5:1, icon/border/focus >= 3:1 in light and dark, in every state including stale, offline and muted: no state lowers text opacity (the axe contrast run covers the disconnected state).
29. UX-29 No pulse or animation under `prefers-reduced-motion`; a static "New" text tag is used instead.
30. UX-30 Document title shows "(N) Stapler Squad" when N > 0 needs-me items; the nav badge has accessible name "Attention, N need you".

### Touch / mobile

31. UX-31 All interactive targets are >= 44x44 CSS px under `(pointer: coarse)` at any viewport width (rows, chips, tabs, Close, overflow, Load older) and >= 24 px under `(pointer: fine)` (WCAG 2.5.8); adjacent targets do not overlap. Size keys on the input type, not on a width breakpoint (a touch tablet at 1024 px and a 700 px desktop window both pass).
32. UX-32 Every swipe gesture has a visible button equivalent (WCAG 2.5.1/2.5.7).
33. UX-33 The bottom sheet respects `env(safe-area-inset-bottom)`, does not cover the focused control when the virtual keyboard is open, and scrolls its content internally.
34. UX-34 The Up Next tab strip scrolls horizontally at 375 px with every tab >= 44 px and an edge cue indicating more tabs.

### Redirect and deep links

35. UX-35 With `attention_inbox_consolidation` on, `/review-queue?session=abc&filter=needs` lands on `/unfinished?tab=sessions&session=abc&filter=needs` with Back returning to the previous page (replace navigation), no flash of legacy content, and focus on the linked row.
36. UX-36 With the flag off or the flag fetch failing, `/review-queue` renders the legacy page (fail to legacy).

### Privacy

37. UX-37 No row, tray item or timeline entry shows more than 160 characters of agent text, and no raw terminal tails or unredacted secrets appear; agent text is rendered only as text nodes (no `dangerouslySetInnerHTML`, no markdown or HTML parsing in the new files).

### Recent activity and delivery

38. UX-38 The Recent activity view is reachable in <= 2 clicks from the nav badge on desktop and mobile, lists just-finished sessions (including ones marked Done or never raised), gone-quiet sessions, changed sessions and (via "Show done") recently done ones with Undo, supports 1 h / 24 h / 7 d windows and "Load older", and hides Hidden sessions unless "Include hidden sessions" is on.
39. UX-39 The nav badge, Sessions-tab badge and document title count the same set (needs-me reasons only); an `awaiting_direction` item raises one low-urgency tray + web-push notification per episode and never a toast or Slack message by default.
40. UX-40 Single-key shortcuts (WCAG 2.1.4): `j`, `k`, `t`, `d` act only while focus is inside the Attention list, never in inputs, the omnibar, dialogs, the terminal or the session detail view; `?` is not re-bound; Settings has an on/off switch (default on, per viewer) that makes them inert; no conflict with the global shortcuts or with the session detail view's Ctrl+T (`terminal.spawn-shell`). Roving tabindex keeps the current row's own buttons reachable by Tab.
41. UX-41 Finished sessions the operator has not opened appear in the nav as text "N new finished" separate from the needs-you number, whether push worked, was denied or never arrived (the cue never depends on push); no sound, toast, animation or live-region speech; the count is the same on phone and desktop (server-derived Seen) and drops when the session is opened on either; hidden at 0; on a 375 px bottom nav it is a second text line under the label ("9+ new finished" above 9), and "new finished" is distinct from the list chip "Finished N" (which counts every open informational row); the list header explains once, informationally, that push is off on this device.
42. UX-42 Volume bounds: the per-session 5-minute cap and the global 6-per-15-minutes cap with one collapsed digest push apply to `awaiting_direction` only; Mute exists per session and globally with the same durations (1 h, 8 h), shows "Muted until HH:MM" as static text (never inside a toast), is undoable, and never hides a row, the badge count, the unseen cue or a toast. Session Mute also holds that session's `prose_question` push and Slack; global Mute holds `awaiting_direction` only; no Mute ever holds approvals, waiting-for-input or error alerts. A global Mute shows the visible word "muted" with the nav cue (second line on mobile) on every page, plus ", alerts muted" in the accessible name; one rule for desktop and mobile. The global mute control is the 44x44 "More" button beside [?] on mobile, and a mute management list (active mutes and opted-out sessions, each reversible) is reachable from it.
43. UX-43 A needs-human row detected without a hook shows "detected from screen, unconfirmed"; the list header shows "N unconfirmed" and the nav accessible name appends it; the item still counts in the badge.
44. UX-44 Toast accessibility (WCAG 2.2.1): the countdown pauses on hover and focus, Undo is keyboard reachable (tab stop after the list, Ctrl/Cmd+Z in the list, hotkey hint in the toast text), up to 3 toasts stack with independent timers, every state-changing action (Done, Not a question, Stop flagging questions, bulk, Mute) has an Undo, toasts are not live regions (the announcer speaks them once), and Recent's "Done (24 h)" Undo remains available beyond the toast.
45. UX-45 The timeline drawer/sheet is a modal dialog (`role="dialog"`, `aria-modal="true"`, labelled, background inert) at 1280, 900 and 375 px; 768-1023 px is single-column with the drawer overlaying the list.
46. UX-46 After Done, "Not a question", bulk or Undo, focus lands on the next row, else the previous row, else the group header, else the list heading, and is never lost to `body`.
47. UX-47 Quiet rows show Running/Idle/Paused text; the Quiet threshold is a setting (15/30/60/120 min, default 30) shared by the chip and Recent's "Quiet now"; Recent's "Changed" is exactly {AttentionRaised, AttentionCleared, StatusChanged, ApprovalResolved} with the 2-minute StatusChanged collapse and the 5-rows-per-session cap.
48. UX-48 Opt-out reviewability: "Stop flagging questions" shows the tag "Questions not flagged" on the session's rows and card, is listed with its set date in the mute management list with a one-step reverse, has no auto-expiry, and a 14-day reminder line (dismissible per viewer) offers Review. Coupling and staleness: acting on an item in Attention resolves its tray record, dismissing a tray record never removes the item (tooltip says so), and a dropped stream marks the nav badge "may be out of date"; first-run help is collapsed to one line below the Needs you group (never pushes a needs-you row below the fold at 375x812), explains every new label once and is re-openable.
49. UX-49 Reflow, zoom and semantics: at 320 CSS px width and at 400% zoom there is no horizontal page scroll and no clipped or overlapping content, rows below the 720 px container threshold move Timeline, Stop flagging questions and Mute into the overflow menu while "Not a question" stays visible (S2; asserted at 768, 1023, 320 px and 400% zoom) (chip rows and the tab strip scroll inside their own container); WCAG 1.4.12 text-spacing overrides (line height 1.5, letter 0.12em, word 0.16em, paragraph 2em) clip nothing in a row, the drawer or a toast; semantics are: rows are `li` of a `ul`, group titles are headings, Needs you, Finished and Quiet chips are a `role=group` of toggle buttons with `aria-pressed` while the Recent chip is a navigation control without `aria-pressed` (`aria-current="page"` when open), the Up Next strip is a `tablist` with `aria-selected`, the drawer is a labelled modal dialog, and the cue and badge are text.

49 acceptance criteria.

## Open UX questions for the owner (do not block implementation)

- Whether "Show done" should be a chip or a menu item (the 10 s toast length is no longer load-bearing: it pauses on focus and Recent keeps Undo for 24 h).
- Whether Seen should also be recorded in the event log (currently not; it changes no state).
- Delivery defaults in ADR-007 (caps, mute durations) are owner-to-confirm in plan.md.

## Verification note

Nothing here was run against the live app. The e2e spec files named in plan Story 7.2.1 should assert UX-01, 05, 11, 12, 14, 19, 24, 31, 35 at minimum; the rest are jest/component or manual checks.
