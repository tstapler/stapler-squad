# UX Research: Durable Guidance Request

Agent 5 (UX Research), SDD Phase 2. Scope per brief: comparable patterns, mental
model, accessibility, error/edge states, JTBD — grounded in this repo's
existing design language, not invented from scratch.

## 0. Existing design language (read before recommending anything)

Three components already do closely related things. The new `GuidanceRequest`
component should look like a natural sibling of these, not a new visual
system.

- **`web-app/src/components/backlog/TriageDiffSection.tsx`** already has a
  free-text "question" answer flow (`questionSuggestions`, `onAnswerQuestion`)
  that is the closest existing analog to a guidance request, but it's
  **ephemeral and short-answer-only**: no yes/no, no multiple-choice, no
  durability across sessions, and no cross-context reuse — it renders only
  inside the triage diff, keyed by array index (`i`), not a stable ID.
  - Collapsed by default: `Answer ▸` toggle button
    (`aria-expanded`/`aria-controls`), replaced in place by a `textarea` +
    Submit/Cancel row when opened.
  - Answered state renders as `role="status" aria-live="polite"` with a
    `✓ Answered: {draft}` marker — the pattern to reuse for a guidance
    request that's already been answered (locally or elsewhere).
  - Inline `InlineError` component (`type="transient"`, headline + retry +
    dismiss) is the established error-surface convention — reuse this rather
    than inventing a new error treatment.
  - Submit button uses `aria-disabled` **and** `disabled` together, plus
    `aria-busy` while submitting — the established busy/disabled convention.
- **`TriageReviewPanel.tsx`** establishes the panel-level conventions: the
  whole panel is `aria-live="polite"`; a `readOnly` prop variant renders the
  same content with interactive affordances (buttons) *absent from the DOM*
  (not disabled) for historical/read-only contexts — directly reusable
  pattern for a guidance request whose scope was deleted/archived or that's
  being viewed in a context where the operator can't answer (e.g. a
  read-only session replay).
- **Nav badge family** (`web-app/src/components/sessions/ApprovalNavBadge.tsx`,
  `ReviewQueueNavBadge`, `MemoryNavBadge`, `UnfinishedNavBadge`, `StuckNavBadge`,
  `NotificationsNavBadge`, wired into `Header.tsx`/`BottomNav.tsx`) is the
  **existing, working answer to "should there be a global N-pending
  indicator"** — yes, and the codebase already has six of them. `ApprovalNavBadge`
  is the closest precedent: it counts pending tool-use approvals via
  `useApprovalsContext()`, renders through a shared `NavBadge` primitive,
  hides itself at zero, and its `aria-label` is a full sentence
  ("N pending approvals. Click to review.") rather than just a number. A
  `GuidanceRequestNavBadge` should be built the same way: a
  `useGuidanceRequestsContext()` (or equivalent) feeding the same `NavBadge`
  primitive, added to both `Header.tsx` and `BottomNav.tsx` alongside its
  siblings.

## 1. Comparable UX patterns

| Pattern | What makes it scannable | What makes it missable | Applicability here |
|---|---|---|---|
| GitHub Actions `workflow_dispatch` / required-reviewer gate | A distinct yellow/amber run state ("Waiting") in the run list — different color from green/red, so it visually separates from pass/fail at a glance. The email + the run's own banner both name *what* needs approval and *who* can act. | Only surfaces where you're already looking (the Actions tab or an email you might filter). No standing global counter across repos. | Confirms: color/state distinctness matters more than icon choice. But GH's weakness (no cross-repo counter) is exactly what this repo's nav-badge family already fixes — lean into that strength rather than copying GH's gap. |
| CI pipeline manual-approval gate (e.g. GitLab/Jenkins "Approve or reject") | The pipeline visibly *stops* — the stage literally can't proceed, rendered as a paused/blocked icon on the stage itself, not just a side notification. | If the person isn't looking at that specific pipeline, the stall is invisible until something else (a deadline, a Slack ping) surfaces it. | Directly maps to "cost of a missed pending question is a stalled automated pipeline" (see #2). The stage-level "paused" visual (not just a toast) is worth mirroring in the session/terminal view — show the session itself as blocked, not just a hidden question buried in scrollback. |
| Slack workflow-builder approval step | Renders as an interactive message block inline in the channel where the workflow triggered — Approve/Deny buttons attached to the actual conversation, so context and action are co-located. | Old approval messages scroll out of the channel; Slack has no persistent "still waiting on you" surface beyond the message itself unless someone re-pings. | Strongest argument for "answer in place, in whichever view you're already in" (session view, backlog detail, triage panel) rather than forcing a trip to a separate inbox — matches the "ONE shared component in three contexts" requirement directly. |
| OS/app notification-center "action required" | Persistent until dismissed; usually a distinct badge count on the app icon (the mental model users already have from every phone). Grouped by app/source, so a user scanning a badge count already knows roughly where to go. | Notification fatigue — if everything (info + action-required) shares one channel, the actionable ones get lost in the noise. | Argues for **not** overloading the existing `NotificationsNavBadge` (which likely already carries general system notifications, per its own dedicated component) — a guidance request deserves its own badge family member like `ApprovalNavBadge`, not a merge into general notifications, so its scannability isn't diluted. |

**Synthesis**: the winning combination already implicit in this repo's own
conventions is GitHub's distinct-state-color + Slack's in-place inline
action + the notification-center's persistent badge count — which is
literally what `ApprovalNavBadge` + inline `TriageDiffSection` question UI
already do separately. The new component should unify those two existing
half-patterns into one.

## 2. User mental model

Single-operator context-switching across many concurrent backlog
items/sessions (confirmed by `docs/reference/*` and the backlog WIP-limit
memory: `feedback_backlog_wip_limit.md` caps concurrent work at 2, so the
operator is already juggling multiple in-flight items as the norm, not the
exception).

- **A global "N pending questions" indicator is required, not optional.**
  Per-view discovery alone ("just open the item") fails this operator's
  actual workflow: they don't proactively re-open every backlog item and
  session to check for new questions; they act on what a nav badge or the
  omnibar surfaces. This repo has already made this exact judgment call six
  times over (`ApprovalNavBadge`, `ReviewQueueNavBadge`, `MemoryNavBadge`,
  `UnfinishedNavBadge`, `StuckNavBadge`, `NotificationsNavBadge`) — a
  guidance-request feature that *didn't* get the same treatment would be the
  one inconsistent gap in an otherwise-established pattern, and would
  predictably become invisible the same way the pre-fix items in
  `project_backlog_stuck_review_investigation.md` did (silent non-durable
  state that nobody discovers until something else forces the issue).
- **Cost of a missed/stale pending question is high and asymmetric**: unlike
  a missed notification (low cost, informational), a missed guidance request
  means an entire backlog item or session sits fully stalled — not degraded,
  *blocked* — until the operator happens to look. Given the 2-item WIP cap,
  a stalled item consumes one of only two concurrent "slots," directly
  throttling total throughput. This elevates guidance requests above
  ordinary notifications: they should behave like `ApprovalNavBadge`
  (persistent, count-based, un-ignorable in the header) rather than like a
  toast that fades.
  Note: whether a guidance request should also actively re-surface via the
  existing `notify()`/`ScheduleWakeup` channels (see
  `project_backlog_wakeup_polling.md`, `feedback_document_ai_decisions_in_edge_cases.md`)
  is an architecture/backend decision for Agent 3, not a UI concern — but the
  UI's badge count must reflect whatever the backend considers "pending" so
  the two layers can't drift.
- **Discovery should be layered, not single-channel**: global badge count
  (system-wide awareness) + per-item/session inline surfacing (the actual
  answer UI, in place) + the item/session's own status affordance (e.g. a
  backlog item card shows a small "awaiting input" indicator the way a
  stalled CI stage shows "paused") so the operator sees it at every level of
  zoom they might be at — matches "Jobs to be done" functional goal #1
  (unblock automation without babysitting): the operator should never have
  to go hunting.

## 3. Accessibility

The component must switch shape at runtime (yes/no → radio group; multiple
choice → radio group or listbox; short-answer → text input) while staying
one semantic form. Concrete requirements, keyed to existing conventions:

- **Container**: wrap the whole guidance-request card in
  `role="form"` with an `aria-label` describing the question (mirrors
  `TriageDiffSection`'s `role="form" aria-label={`Answer: ${q.text}`}` for
  its answer form) — gives screen-reader users a landmark to jump to
  regardless of which of the three host surfaces it's embedded in.
- **New-question announcement**: the outer wrapper (or a dedicated live
  region sibling to it) needs `aria-live="polite"` at minimum — same as
  `TriageReviewPanel`'s `<section aria-live="polite">` — so a screen-reader
  user working elsewhere in an already-open view (e.g. the session/terminal
  view, which is inherently noisy) is told a new question appeared without
  an interruptive `assertive` announcement mid-typing. Reserve `assertive`
  only for the nav badge's count change if product wants an audible
  system-wide alert — that's a product call, not a default.
- **Yes/No and multiple-choice**: use a native `role="radiogroup"` with a
  `<legend>`/`aria-labelledby` naming the question text, individual
  `role="radio"` (or native `<input type="radio">`) options with roving
  tabindex (arrow keys move selection, Tab enters/exits the group once) —
  do **not** reinvent this with `<div>`+`onClick`; native radio inputs get
  this for free and match the "roving tabindex" precedent already fixed
  elsewhere in this codebase for FileTree
  (`e5ea4d255 fix(FileTree): give react-arborist rows a real roving tabindex`).
  If multiple-choice options can exceed ~6-8, prefer a `<select>` or
  `role="listbox"` instead of a long radio list, to keep it scannable and
  keyboard-fast (Home/End/typeahead) rather than a same shape at 15 options.
- **Short-answer**: a labeled `<textarea>` or `<input type="text">` — reuse
  `TriageDiffSection`'s textarea pattern (`id`, matching `<label>`/`aria-label`,
  Escape-to-cancel `onKeyDown`).
- **Focus management on mount in a busy panel**: do **not** auto-`.focus()`
  the first form control the instant the component mounts inside
  `TriageReviewPanel` or the session view — both hosts render many other
  interactive elements simultaneously (Apply/Skip/Refine buttons, terminal
  input), and a surprise focus steal there is disorienting and breaks
  terminal typing. Instead: announce via the `aria-live` region (above), and
  only move focus on an explicit user action — clicking/tapping the
  question card, or activating it from the nav badge's "click to review"
  affordance (mirrors `ApprovalNavBadge`'s `aria-label`: "N pending
  approvals. Click to review."). When the badge click navigates the operator
  directly to the pending question, *that* navigation is the appropriate
  moment to move focus into the form.
- **Submit affordance**: `aria-disabled` + real `disabled` together while no
  answer is chosen/entered, `aria-busy="true"` while submitting — exact match
  to `TriageDiffSection`'s existing submit-button convention, so keep it
  identical rather than inventing new busy semantics.
- **Answered/resolved state**: `role="status" aria-live="polite"` echoing the
  chosen answer (`✓ Answered: {value}`) — reuse verbatim from
  `TriageDiffSection`'s `answeredMarker`, satisfying JTBD emotional goal
  "confidence the answer did something" (see #5) via the same visible
  echo-back pattern already proven in this codebase.

## 4. Error / edge states

| Scenario | Recommended treatment | Rationale |
|---|---|---|
| **Answered elsewhere, this view is stale** | On submit, a stale-write conflict should surface as a non-destructive, specific message — not the generic `TriageErrorBanner` wording ("item may have been updated by another process. Reload and try again.") verbatim reused where it fits, but tailored: "This question was already answered — reload to see the answer." Auto-transition to the answered/resolved state (`✓ Answered: …`) if the payload comes back with the actual answer, rather than making the operator manually reload — avoids a second wasted round trip. | Matches `handleApply`'s existing optimistic-conflict message in `TriageReviewPanel.tsx` (`"Failed to apply suggestions. The item may have been updated by another process."`) — same failure *shape* (concurrent-write race), so reuse the tone/wording convention, but resolve automatically where the API allows it since the "answer" is knowable, not just "something changed." |
| **Scope (item/session) deleted/archived while pending** | Render the `readOnly`-style variant (per `TriageReviewPanelReadOnlyProps` precedent): interactive controls *absent from the DOM*, not disabled, plus explicit copy ("This session was archived before this question was answered — no longer actionable.") rather than silently vanishing the card. | Mirrors the existing `readOnly` design decision documented in `TriageReviewPanel.tsx`'s own comment: "A historical record shouldn't be dismissible in the first place." A guidance request whose scope disappeared is functionally the same category — a dead-but-worth-recording historical artifact, not an active prompt. |
| **Per-scope cap rejected a new question** | Yes — proactively show "N/cap pending" once close to or at the cap, *before* the agent hits it, using the same visual language as the nav badge (a small count chip) rather than a plain-text sentence, so it's scannable rather than something the operator has to read carefully. Surface this at the point where a new question would be created (backlog item detail / triage panel), not just after a rejection — a rejection-after-the-fact forces the agent into a failure/retry loop for something the UI could have signaled ahead of time. | Directly analogous to the WIP-limit-awareness UX already valued in this codebase (`feedback_backlog_wip_limit.md`: "check in_progress count before spawning") — the operator-facing equivalent of the same idea: don't let the system silently hit an invisible ceiling. Exact placement/enforcement of the cap itself is a backend/architecture question (Agents 2/3), but the UI contract should be: the API response for a scope should include current-pending-count and cap so the frontend can render "N/cap" without a separate round trip. |

## 5. Jobs-to-be-done

- **Functional** — *"Let me unblock stuck automation without babysitting
  it."* Satisfied by: the global badge (discovery without hunting) + answer
  in place in whatever surface the operator is already in (Slack-approval-step
  precedent) + the answer actually resuming the specific session/pipeline
  that asked (durability requirement in the feature brief — the answering
  view and the asking view can be different sessions entirely).
- **Emotional** — *"When I click Submit, I need to believe it worked."* This
  is the single biggest risk given the durable/cross-session nature of this
  feature: the operator may answer a question and then have **no visible
  confirmation the original (possibly now-different) session actually
  resumed**, since that session isn't the one they're looking at. Mitigate
  with: (a) the immediate in-component `✓ Answered: …` echo (already-proven
  pattern, addresses "did my click register"), and (b) something *beyond*
  that immediate echo that confirms downstream resumption — e.g. the nav
  badge count decrementing live (proves the system-wide state changed, not
  just the local DOM), and ideally a follow-up notification
  (`NotificationsNavBadge`/existing notification system) along the lines of
  "Session X resumed after your answer" once the asking session actually
  consumes it. Without step (b), this feature reproduces exactly the
  "did that even do anything?" feeling the brief calls out — a local
  optimistic UI update is not evidence the backend queue was drained.
- **Social** — none; single-operator tool, skip (per brief).

## Summary of concrete recommendations for Agents 3/4 (backend/architecture) to support

1. API responses for a scope (item/session) should include current pending
   count + cap so the UI can render "N/cap" proactively (edge-case #4).
2. Backend should support resolving a stale/already-answered submit with the
   actual answer payload inline, not just a generic conflict error, so the
   frontend can auto-transition to the answered state instead of forcing a
   manual reload.
3. A `GuidanceRequestNavBadge` (context provider `useGuidanceRequestsContext`
   or reuse of an existing polling/subscription mechanism) should follow the
   exact `ApprovalNavBadge` shape — this needs a backend "list pending
   guidance requests across all scopes" query, analogous to whatever backs
   `useApprovalsContext()`.
4. When an answered guidance request's asking session actually resumes, that
   event should be observable by the frontend (existing notification
   pipeline) so the "did that do anything" emotional gap can be closed with
   a genuine resumption confirmation, not just an optimistic local update.
