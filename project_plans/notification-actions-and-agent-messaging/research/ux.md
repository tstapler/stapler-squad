# UX Research: notification-actions-and-agent-messaging

Date: 2026-10-17. Scope: competitive survey, interaction flows, mental models, a11y/mobile, edge states, JTBD.

**Superseded in part (triad round 3):** the `radiogroup` role (section 4.1, "Name, role, value" row), the keyboard bullet "inside an expanded card Tab walks options -> Submit -> Cancel; number keys 1-9 pick the Nth option" (section 4.2) and the "bottom sheet / full page" panel (section 4.3) are replaced by `design/ux.md` (option buttons in a `role="group"`, digits only move focus, no bare Cancel button, no sheet: the mobile panel is the Alerts page). Read those three lines as history.

Confidence labels: **VERIFIED** = I opened the primary source (page content read, not only a search snippet). **SNIPPET** = a search-result summary only, primary page not opened. **INFERRED** = my synthesis. Tools with no evidence found are listed at the end and skipped.

Consistency anchors (read in full before writing):
- `project_plans/durable-guidance-request/design/ux.md`: `GuidanceRequestCard` (yes/no, multiple-choice, short-answer; collapsed one-line summary with `Answer ▸`; `role="form"`, `aria-live="polite"`), `GuidanceRequestNavBadge`, `44px` / `--min-touch-target`, `900px` breakpoint, 16px textarea font floor (iOS zoom), `scrollIntoView` on textarea focus, §8 edge-state table with exact copy, session-view banner that never steals terminal focus.
- `project_plans/notification-revamp/design/ux.md`: "Needs a decision" pinned tier, AC26-AC34 cross-cutting a11y (Tab order, `aria-label` on every badge, non-color encoding, polite live region present from first paint with count-only announcements, `<button aria-expanded>` collapsibles, no dead-end errors, focus moves to next item on removal), Surface 11 (bell dropdown stays flat; actionable unread items get no dismiss X; "Clear history" never deletes unread actionable).

## 1. Competitive survey

### 1.1 Findings by tool

| Tool | What the evidence shows | Confidence | Source |
|---|---|---|---|
| **Warp** | Three notification types: Complete, **Request** (agent blocked: command approval, permission, idle prompt), Error. Toasts (max 2 visible, oldest replaced, hover pauses timer, click jumps to session). Bell opens a **mailbox** sidebar with All / Unread / Errors tabs (filters hidden when empty), Mark all as read, click navigates. Keyboard: Up/Down select, Enter opens, Shift+Tab cycles filters, Esc closes. Tabs show a state icon (working/blocked/completed/errored) and an unread attention badge; notifications auto-mark-read when you open the tab. Multi-agent: child agents are excluded from mailbox/toasts to avoid churn; per-child state lives in a pill bar; a blocked child is resolved by opening it. Works with Claude Code, Codex, OpenCode via plugin/config. Notifications navigate to the agent; they do not answer in place. | VERIFIED | https://docs.warp.dev/agents/capabilities/agent-notifications.md |
| **Claude Code Remote Control / mobile** | Push to phone when a long task finishes or a decision is needed; suppressed while you are typing in the connected terminal (presence). Permission prompts and `AskUserQuestion` questions **stay open until answered** (no expiry); other forwarded dialogs expire after 5 minutes by default. Sessions reconnect automatically after sleep/network loss; server mode gives up after ~10 min. Local process must keep running or the session goes offline. Answering happens inside the session view on phone/web, not from the notification itself. | VERIFIED (lines read from saved page) | https://code.claude.com/docs/en/remote-control |
| **Cursor web/mobile agents** | Start agents from phone/browser; add follow-up instructions to a running agent; Slack notification on completion. Queued follow-ups have Send now / Edit / Remove controls that were **hover-only and broken on touch** (staff confirmed). No official native app; push for "needs input/approval" was a forum feature request, not confirmed shipped. | SNIPPET (forum + docs summaries) | https://forum.cursor.com/t/no-send-now-button-in-mobile-agents-mode/155685 ; https://forum.cursor.com/t/push-notifications-mobile-desktop-when-agent-needs-user-input-or-approval/148230 ; https://docs.cursor.com/get-started/web-and-mobile-agent |
| **OpenAI Codex cloud** | Follow-ups on cloud tasks; community request for push on approval, a visible "Waiting for approval" state in the task list, and in-app badges (staff: passed along, no timeline). Not shipped as far as evidence shows. | SNIPPET | https://community.openai.com/t/codex-approval-requests-should-trigger-mobile-push-notifications/1381134 |
| **Zed agent panel** | Permission request renders as a tool card in the thread with **Allow once / Deny once** plus **Always for <tool> / Always for <pattern>**. Global default `confirm`/`allow`/`deny`; always_deny patterns still win. Approve-inline-with-scope is the pattern. No evidence found on notifications. | SNIPPET (docs summary) | https://zed.dev/docs/ai/tool-permissions |
| **Devin** | API session statuses: new, claimed, running, exit, error, suspended, resuming (no distinct "waiting for user" status in v3). A message can be sent to an active session; a suspended one resumes automatically. Slack: 👀 reaction while working, replies in thread. | SNIPPET | https://docs.devin.ai/api-reference/v3/sessions/post-organizations-sessions-messages.md ; https://devin.ai/blog/devins-slack-etiquette |
| **GitHub Copilot coding agent** | Agents panel lists delegated tasks with status; click goes to the PR. No evidence of a waiting-for-input state. | SNIPPET | https://github.blog/changelog/2025-08-19-agents-panel-launch-copilot-coding-agent-tasks-anywhere-on-github-com |
| **Factory Droid** | `Notification` hook fires when Droid waits for input (desktop/Slack alerts via user scripts); SDK has an interaction handler for permission requests. | SNIPPET | https://docs.factory.ai/guides/hooks/notifications |
| **Conductor** | Dashboard of agents per worktree: "who's working, what needs attention, review code". No documentation of its own notification or answer UX found. | SNIPPET (third-party listings) | https://alternativeto.net/software/conductor/about |
| **Vibe Kanban** | Board columns map to git/review state; inline diff comments are sent to the agent; notification preferences at setup. No agent-question inbox evidence. Project reported (by a competitor page) as moving to community maintenance. | SNIPPET | https://vibekanban.com/docs/getting-started |
| **Crystal, Sculptor** | Crystal: Electron app, worktree-per-session, session persistence. Sculptor: container-per-agent, Pairing Mode, merge/discard. No evidence on notification or question UX for either. | SNIPPET | https://github.com/nsttt/crystal ; https://imbue.com/sculptor |

Not found (skipped): Claude Squad, Amp, Claude Code on the web's in-page question rendering (only the Remote Control behaviour above is verified). Do not cite these.

### 1.2 What the survey tells us

1. **Nobody verified answers in place from a notification.** Warp, the best documented, treats the mailbox as a navigator ("click to jump to the agent"). Zed and Claude Code resolve prompts in the thread, not in a separate inbox. Inline-answer-from-the-inbox is therefore a differentiator but also unproven ground: expect to invent the edge-state handling. (INFERRED from the table.)
2. **Blocked is a first-class state, distinct from errored and complete** (Warp's Request type; Codex users asking for "Waiting for approval"; Devin's API lacking one is a gap people notice). Maps directly to the revamp's "Needs a decision" tier.
3. **Per-child/background noise stays out of the interrupting channel** (Warp excludes child agents from mailbox and toasts and gives them a separate status surface). Direct precedent for splitting our Background tab from the attention feed, with exceptions pinned.
4. **Presence-aware suppression** (Claude Code skips push while you are at the terminal) matches our delivery gate for hidden/visible sessions.
5. **Hover-only affordances break on touch** (Cursor mobile bug). Confirms the existing "no hover-only" AC33.
6. **Approve-with-scope** (Zed: once vs always for tool/pattern) matches the repo's existing "Create rule" action on approvals.
7. **Questions should not expire silently** (Claude Code keeps AskUserQuestion open indefinitely; only secondary dialogs time out). Our staleness model should be driven by the session/question lifecycle, not a timer.

## 2. Interaction flows that work

Ordered by fit for stapler-squad.

1. **Inbox triage with a pinned "needs a decision" tier** (Warp mailbox filters; revamp Surfaces 1/6). Keep: Unread/Needs-decision first, collapsible activity below, keyboard Up/Down/Enter.
2. **Option buttons for closed choice prompts.** AskUserQuestion with 2-4 options and `multiSelect` maps to the existing `GuidanceRequestCard` variants: yes/no (2 options), multiple-choice radios (single select), checkboxes (multiSelect, new), and a short-answer fallback for "Other". One tap sends; for multiSelect and free text an explicit Submit is required. Always include a path to "Open session" for context the card cannot show.
3. **Approve / Deny with optional scope** (Zed). Primary Approve and Deny stay one tap each; "Create rule" (existing) is the secondary scoped action. Deny should optionally take a reason because the agent acts on it (INFERRED).
4. **Quick-reply for free text** (Cursor follow-up, Devin message). Collapsed one-liner with `Answer ▸` (existing pattern), `Cmd/Ctrl+Enter` to send, textarea at 16px, keyboard-safe on mobile.
5. **Snooze.** No competitor evidence of snooze on agent prompts. It is an email-inbox idiom (INFERRED). Recommendation: do not offer snooze on blocking questions (the agent is stalled; snoozing hides a stall, the exact bug class the revamp fixed with "never dismiss unread actionable"). Offer "Remind me" only on informational/Background rows if at all; defer to a later phase.
6. **Optimistic resolve with rollback.** Card flips to "Answering..." then "Answered: X" using the server-returned value (matches durable-guidance §8 "answered elsewhere" row). Do not remove the card until the delivery result is known.
7. **Toast + badge, not modal.** Warp: max 2 toasts, pause on hover, click opens. Ours: bell badge + panel; never a focus-stealing modal.

## 3. Mental models

- **Inbox / mailbox** (Warp, email): items are things to clear. Risk: operators expect "mark read" to mean "handled"; the repo already forbids marking unread actionables read in bulk. Keep that.
- **Agent as a blocked colleague**: the question is a message from a person who is waiting. The card should state who is waiting and for how long ("review:foo asked 4m ago"), not just the question.
- **Process monitor / activity feed** for Background: "what is running, is it healthy, did it finish". Distinct from the inbox: nothing here is expected to be cleared. Treat Background as a feed with pinned exceptions (matches requirements: "activity feed + pinned exceptions"), not as a second inbox.
- **Durable ticket** (existing GuidanceRequest): an answer is recorded even if the session is dead. The UI must make the delivery state explicit: "Sent to live session" vs "Saved; will be delivered when the session resumes" vs "Session ended; recorded only".
- Naming: use "Answer" for questions, "Approve/Deny" for permissions, "Decide" for backlog choices; avoid "Resolve" as a generic verb (revamp already analysed the Skip/Dismiss/Mark read/Resolve verbs).

## 4. Accessibility, keyboard, mobile

### 4.1 Standards that apply (WCAG 2.2 AA)

| Need | Requirement | Source |
|---|---|---|
| Live updates announced without moving focus | SC 4.1.3 Status Messages; `role="status"` / `aria-live="polite"`, region present from first paint, announce counts not content (already AC30) | https://www.w3.org/TR/WCAG22/#status-messages |
| Never use `assertive` for new questions | Existing repo rule; assertive reserved for nothing in this feature | durable-guidance ux.md §2 |
| Target size | Repo standard 44x44px (exceeds SC 2.5.8 minimum 24x24); 8px gap between stacked rows (`vars.space["2"]`) | https://www.w3.org/TR/WCAG22/#target-size-minimum |
| Name, role, value | Card `role="form"` `aria-label="Answer: <question>"`; option group = `role="radiogroup"`/`group` with the question as legend; checkboxes for multiSelect; every icon/badge has text and `aria-label` | https://www.w3.org/TR/WCAG22/#name-role-value |
| Error identification and suggestion | Inline error adjacent to the control, text not color, retry action (SC 3.3.1, 3.3.3) | https://www.w3.org/TR/WCAG22/#error-identification |
| No keyboard trap, visible focus, focus not obscured | SC 2.1.2, 2.4.7, 2.4.11; sticky action bars on mobile must not cover the focused field | https://www.w3.org/TR/WCAG22/#focus-not-obscured-minimum |
| Timing | Toasts that carry actions must not vanish before use: pause on hover/focus, and the item stays in the panel (SC 2.2.1). Prefer persistent panel entry over any auto-dismiss | https://www.w3.org/TR/WCAG22/#timing-adjustable |
| Non-color status | Icon + text + aria-label for every state chip (waiting / working / failed / done) (AC29) | revamp ux.md |
| Tab pattern | Panel segments (All / Needs you / Background) use `role="tablist"` with roving tabindex, Left/Right arrows, `aria-selected`, `aria-controls`; count in the tab's accessible name ("Background, 11 sessions") | https://www.w3.org/WAI/ARIA/apg/patterns/tabs/ |
| Disclosure | `Answer ▸` is a real `<button aria-expanded aria-controls>` (existing pattern) | https://www.w3.org/WAI/ARIA/apg/patterns/disclosure/ |

### 4.2 Keyboard model (desktop)

- Panel open: focus moves to first actionable item (or the tablist if the Needs-you tab is empty). Esc closes and returns focus to the bell.
- Within a list: Up/Down (or J/K) move between items; Enter expands the card; inside an expanded card Tab walks options -> Submit -> Cancel; number keys 1-9 pick the Nth option only when the card has focus (do not bind globally; would collide with the terminal, see durable-guidance §7.3).
- Cmd/Ctrl+Enter submits a free-text answer; Esc collapses the card without submitting and preserves the draft.
- After an action resolves and the item leaves the list, focus goes to the next item, else to the list heading (AC34). Announce "Answer sent" via the polite status region.
- A new question arriving while the user types in the terminal: polite announcement and a banner; never focus change (existing durable-guidance §7.3 decision).

### 4.3 Mobile requirements

- 44px targets on every option row and primary buttons; row padding does the sizing, not the glyph. Stack options vertically below 900px; full-width Submit.
- No hover-only controls (Cursor's queued-message bug is the cautionary case).
- Free-text: `font-size: 16px` minimum (iOS zoom); `scrollIntoView({block:"nearest"})` on focus; Submit remains reachable above the on-screen keyboard (use `visualViewport` or sticky action bar). `enterkeyhint="send"` is appropriate on a single-line quick reply (INFERRED); multi-line stays Enter = newline.
- Use `inputMode="text"`; no special mode needed.
- Option buttons must be tap-safe: confirm step only for destructive actions (Deny with a reason, merge), not for ordinary option choice; prevent double-tap by disabling after first press.
- Bell panel on mobile is a bottom sheet / full page (BottomNav "Alerts"), not a hover dropdown. The panel must scroll as one region; avoid nested scroll traps.
- Network flakiness is normal on mobile: every submit needs an idempotency key and visible pending state (see section 5).

## 5. Error and edge states (copy proposals, extending durable-guidance §8)

| Scenario | What the operator sees | Exit path |
|---|---|---|
| Question stale: agent already moved on (answered in terminal, or session advanced) | Card becomes read-only: "Already handled in the session. No action needed." | None needed; card moves to Recent activity |
| Answered elsewhere (another tab/device) | Auto-transition to "Answered: {value}" using the value returned by the API; fallback "Already answered - reload to see the answer" | Reload only if the value is missing |
| Session gone (stopped/killed/archived) with a durable record | "Session ended. Your answer is saved and will be delivered if the session resumes." Submit stays enabled and records the answer. | "Open session" to resume; archived sessions are NOT auto-revived (`noarchivedrevival`) so the copy must not promise delivery for archived: "This session is archived. Your answer will be saved but not delivered unless you resume it." |
| Session gone with no durable record (ephemeral prompt, e.g. raw INPUT_REQUIRED) | Card read-only: "Session ended before this was answered." | Open session; dismiss |
| Answer send failed (transient) | Inline error under the control: "Couldn't send your answer. Retry" with draft preserved; existing `InlineError type="transient"` | Retry / Dismiss |
| Answer send failed (delivery to live session: keystroke not accepted, prompt no longer on screen) | "Sent, but the session didn't pick it up (the prompt may have changed). Open session." The record stays "delivery failed", not "answered" | Open session; Retry |
| Answer delivered, agent has not acknowledged | State "Sent - waiting for the agent" (distinct from "Answered"); after a threshold show "No response yet. Open session." | Open session |
| Duplicate submit (double tap, retry after timeout) | Idempotent: second result shows the same "Answered" state; never sends a second keystroke | N/A |
| Race: user answers in the terminal while card is open | Card flips to stale/handled state via live update; focus not stolen; polite announcement | N/A |
| Approval for an action that already timed out or was auto-handled | "Handled automatically by rule {name}" (reuse Auto-handled section copy) | Create rule |
| Offline / lost connection while panel open | Persistent banner: "Offline. Showing last known state." Action buttons disabled with `aria-disabled` and a reason | Auto-retry on reconnect |
| Empty **Needs you** tab | "Nothing needs a decision." plus last-checked time (revamp Surface 4 precedent) | None |
| Empty **Background** tab, healthy (the live case: 11 hidden sessions, 0 records) | Must never be blank. Show a summary row per hidden session or a roll-up: "11 background sessions, all healthy. Last activity 3m ago." with each row: name, role (review/diagnose), status chip (running / finished / stopped), last activity time, "Open". | Expand to list |
| Empty **Background** tab, truly no hidden sessions | "No background work. Review and diagnose sessions will appear here while they run." | None |
| Background has failures | Pinned exception rows first with failure chip; healthy rows collapsed below ("11 healthy") | Open session / retry per existing actions |
| Tab with live updates while the user reads it | Do not reorder rows under the cursor; new items insert with a "N new" pill at the top rather than shifting content (INFERRED from general list-update practice). | Click pill to merge |
| Multi-choice question with many options (>8) | Native `<select>` (existing durable-guidance decision) | N/A |
| Question references options the UI cannot represent (preview panes, rich markdown) | Show question text and options as text; "View full prompt in session" link | Open session |

## 6. Jobs to be done

1. When an agent is blocked on a choice and I am not in that session, let me see the question and its options and answer in one interaction, so the agent resumes without me hunting for the prompt. (Primary success metric: 1 interaction after the pane is open.)
2. When I step away (phone, other window), let me know something needs me, and let me answer from a phone-sized screen with large targets.
3. When many sessions run, show me only the things that need a human, and keep healthy background activity visible but quiet, so "no news" is distinguishable from "broken".
4. When I answered, confirm the agent actually received it, and tell me plainly if it did not.
5. When a session ends before I answer, let me leave the answer for later without losing it, and be honest about whether it will be delivered.
6. When a backlog item needs a decision (review verdict, stuck, rework vs merge), let me decide from the same pane with enough context (title, one-line reason, link) to be safe.
7. When an agent needs help from another agent, let me see that a request exists and intervene only if it stalls (INFERRED; peer messaging is not an operator-facing workflow by default). Peer requests should appear in the feed as informational and escalate to Needs you only on timeout or failure.
8. When I am at the terminal already, do not interrupt me twice (presence-aware suppression, Claude Code precedent).

## 7. Recommendations for design/ADR phase

1. One action-card component family (reuse `GuidanceRequestCard`; add multiSelect checkbox variant and a delivery-status line) hosted in the bell panel, Notifications page, and session banner. Do not create a second question UI.
2. Delivery state is part of the model: `pending -> sent -> acknowledged | delivery_failed | saved_for_later | stale`. UI copy above keys off these.
3. Background tab = feed of per-session activity rows plus pinned exceptions plus a never-blank roll-up; Needs-you tab = actionable cards. Do not mix.
4. No snooze on blocking questions in v1.
5. Add AC for: tablist semantics, polite status announcements ("Answer sent", "3 items need a decision"), focus move after resolve, no hover-only controls, 44px targets, 16px textarea, idempotent submit, offline banner. Each should get an e2e spec (data-testid/ARIA locators per repo convention).
6. Open UX question for the owner: should an answered-but-unacknowledged state ever block the card from leaving Needs you? Recommendation: keep it visible in Needs you with "Sent - waiting for the agent" until acknowledged or 60s elapses, then move it to Recent activity with a warning.
