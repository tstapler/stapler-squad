# Research: UX — Plan Approval UX

**Dimension**: UX | **Phase**: 2 — Research

## Question 1: Comparable UX patterns for reviewing/approving AI-generated plans

### GitHub PR review (Approve / Request changes / Comment tri-state)
The tri-state review verdict is the single most battle-tested pattern for "someone must
sign off on a body of proposed work before it becomes real." Three properties make it
work, all of which are directly portable to a solo-developer, single-file plan review:

- **The verdict is a first-class, persistent object**, not a transient side effect of a
  button click. GitHub shows the reviewer's verdict as a labeled entry in the PR
  timeline (avatar + "approved these changes" / "requested changes" + timestamp) that
  never disappears, plus a merge-gate badge summarizing the aggregate state. This maps
  directly to Success Criterion 1 ("not just a button that vanishes") — the fix here is
  architectural (persist a status + timestamp + actor), not just visual.
- **"Request changes" always carries a reason**, enforced by the UI (the comment box is
  part of the same modal as the verdict radio buttons — you cannot select "Request
  changes" and submit with an empty body). This is the right UX-enforcement point for
  Success Criterion 3: validate non-empty reason *at the point of submission*, not as a
  separate optional field bolted onto an existing approve button.
- **Inline/line comments are anchored to content, not a global text box**, and are
  visually distinct from the verdict itself — you can leave line comments without
  submitting a review verdict at all ("Comment" is a third, non-blocking state). For a
  solo reviewer this third state is arguably unnecessary complexity (see "scaled down"
  note below) but the anchoring mechanism (comment bound to a specific line/range,
  rendered as a threaded annotation directly under that line) is the right shape for
  Success Criterion 5.

**Scaled down for solo-developer single-reviewer context**: GitHub's tri-state exists
because *multiple* reviewers each cast independent verdicts that must be aggregated
(any one "Request changes" blocks merge until dismissed). With exactly one reviewer,
the aggregation problem disappears — a plain two-state model (approved /
changes-requested, defaulting to "pending" until either happens) is sufficient. Do not
add a distinct "Comment only" verdict state unless there's a concrete need to leave
feedback without blocking — the existing `planApproved` boolean plus a new
"changes-requested" state (per requirements.md Success Criterion 1's four states) already
covers this without a third verdict type.

### Notion / Google Docs suggestion-and-comment mode
Comments anchor to a text *selection range*, not a line number — the anchor survives
minor edits above/below it because it's stored as an offset into the *current* document
plus a fuzzy-matched text snippet, not a fixed line index. This is the most relevant
precedent for the plan-regeneration pitfall (line numbers shift when `sdd:3-plan`
reruns with feedback) flagged in requirements.md's Pitfalls dimension — worth
cross-referencing there. Google Docs additionally shows an "orphaned comment" affordance
(strikethrough + greyed anchor + "the referenced text was deleted") rather than
silently dropping comments whose anchor text vanished — a good model for what to do
when a plan is regenerated out from under an open comment.

### AI coding-agent "review the plan before I proceed" screens
This project already ships the most directly relevant comparable: **Claude Code's own
plan-mode review UX** (the flow this very agent operates under is confirmation of it).
The pattern: the plan is rendered in full as readable content (not a path or filename),
a binary choice is presented (proceed / provide feedback), and if feedback is given the
agent revises the *same* plan artifact and re-presents it — feedback does not require
re-describing the whole task from scratch. Cursor's and Aider's plan/architect-mode
review screens follow the same shape: full plan text rendered inline, an explicit
approve action, and a text-feedback path that feeds back into a single regeneration
step rather than starting a new unrelated conversation. Two properties of this pattern
worth carrying into this feature:

- **Feedback triggers regeneration of the same artifact, not a parallel one.** This
  directly answers the mental-model question in §2 below in the "expects automatic
  re-planning" direction, and matches this codebase's own `TriggerTriageRequest.feedback`
  precedent (`BacklogItemDetail.tsx` ~line 721) — feedback is already wired to *trigger*
  a rerun, not just archived as a note.
- **The plan is always shown in full before the approve action is reachable** — none of
  these tools put "Approve" behind a collapsed/hidden section. This is a direct argument
  for promoting `PlanArtifactsSection` out of `CollapsibleSection`'s default-collapsed
  state (`defaultExpanded` currently defaults closed per `PlanArtifactsSection.tsx:17`)
  when there's a plan pending review — collapsing content that gates an irreversible-ish
  action (spawning implementation work) works against the reviewer actually reading it.

## Question 2: User mental models — what does "approve" vs "reject" mean here

Two concrete mental-model questions from the research brief, answered from comparable
products plus this codebase's own existing feedback plumbing:

**Does rejection trigger automatic re-planning, or just record feedback for later?**
The dominant mental model from the closest comparables (Claude Code plan mode, Cursor/
Aider architect mode, and this repo's own `TriggerTriageRequest.feedback` /
`send_back_idea`'s "[Revision feedback]" note pattern — `BacklogItemDetail.tsx`
~line 805-813) is that **feedback is actionable, not archival** — submitting it is
expected to *do something*, not just get logged. A reviewer who writes "the plan
doesn't address the caching case" expects the next thing that happens to be a revised
plan, not a plan sitting inert with a comment attached that they now have to remember to
re-trigger action on. Recommendation: "Request changes" should default to *also*
triggering the next `sdd:2-research`/`sdd:3-plan` cycle with the feedback attached (reusing
the `TriggerTriageRequest.feedback` wiring pattern), not just flip a status flag. If
research finds this needs to be a separate explicit action (auto-trigger being too
surprising/expensive to run automatically), the UI must make the two steps ("recorded
my feedback" vs. "asked the agent to try again") visually distinct — e.g. two buttons,
not one — so the user isn't left wondering why nothing happened after they typed a
reason and clicked something.

**What does the user expect the visible state to look like?**
Three artifact types, all already precedented in this codebase — do not invent a fourth
shape:
- **Badge** (`BacklogItemBadge.tsx`) — compact, single-line, icon/color + short label,
  used in list/board views. A plan-approval indicator likely does NOT belong here per
  the codebase's own prior width-budget decision documented at
  `BacklogItemBadge.tsx:36-49` (a 4th inline element was explicitly deferred for the
  stuck-reason chip for the same reason — no free width). Any list-level surfacing of
  plan-approval state should follow the same "defer to detail view" precedent unless
  research proves otherwise.
- **Chip with icon + label** (`BlockerChip.tsx`) — "full" variant (icon + label +
  duration) for the detail view, "compact" (icon + label, no duration) for cards. The
  code's own comment is explicit: *"Never color-only — the icon and text label always
  accompany the chip's color"* (`BlockerChip.tsx:16-18`) — directly actionable for the
  approval-status indicator's accessibility requirement (color alone must never be the
  only signal; see §3).
- **Timeline/history entry** — no dedicated timeline component exists yet in
  `web-app/src/components/backlog/detail/`; the closest existing pattern is whatever
  generic status-event list renders elsewhere in `BacklogItemDetail.tsx` (worth an
  Architecture-dimension check for a `StatusHistorySection` or similar). GitHub's model
  (verdict entries persist in the timeline forever, are not editable, and show
  actor+timestamp) is the right shape for Success Criterion "Should Have: approval/
  rejection history visible in the item's status/progress timeline."

Net: expect a **persistent status chip** (analogous to `BlockerChip`, four states: no
plan / pending review / approved / changes requested) in the detail view's action/
summary area, PLUS a **timeline entry** each time the state changes — not one or the
other.

## Question 3: Accessibility requirements (WCAG / ARIA / keyboard)

### Status indicator / badge
- WCAG 1.4.1 (Use of Color) — state must be conveyed by icon + text label, not color
  alone. This codebase already enforces this convention explicitly for `BlockerChip`
  (`BlockerChip.tsx:16-18`); the new plan-approval indicator must follow the identical
  rule (e.g. ✓ "Plan approved" / ⏳ "Pending review" / ✎ "Changes requested", each with
  its own text, not a colored dot).
- Give the status element `aria-label` summarizing the full state for screen readers,
  matching `BacklogItemBadge.tsx:53-56`'s `aria-label={`Status: ${getStatusLabel(status)}`}`
  pattern — icon glyphs should be `aria-hidden="true"` (as `BlockerChip.tsx:35` already
  does) so they aren't announced as raw unicode.
- If the status can change asynchronously while the page is open (e.g. another
  triage/plan run completes plan approval elsewhere), wrap it in a `role="status"
  aria-live="polite"` region — same choice already made for `InlineNotice`
  (`InlineNotice.tsx:34-35`), documented there as deliberately *not* `alert`/`assertive`
  because it's a routine, non-blocking update. Do not reuse `InlineError`'s
  `assertive`/`alert` semantics for a normal state-change notification — reserve that
  for genuine failures (e.g. plan file read error, see §4).

### Approve / reject button pair
Existing precedent in `ActionsSection.tsx` is directly reusable — it already solves
this problem for other conditionally-enabled actions:
- Disabled-but-explained buttons use `aria-disabled` (not the native `disabled`
  attribute) *combined with* `disabled={actionLoading !== null}` for the transient
  "another action in flight" case, and a `title` attribute carrying the human-readable
  reason (e.g. `ActionsSection.tsx:136-140`: `title={!canSpawnSession ? "Approve the
  plan or enable skip_planning to spawn a session" : undefined}`). Reuse this exact
  pattern for a disabled "Request Changes" button when the reason textarea is empty —
  do not just disable it silently.
  - Caveat worth flagging to implementation: `aria-disabled="false"` (rather than
    native `disabled`) still leaves the element focusable and in the tab order, which
    is correct per WAI-ARIA APG for disabled buttons that need a `title`/tooltip to
    remain reachable by keyboard/AT — but the `onClick` handler must still no-op when
    the guard condition is false, since `aria-disabled` does not prevent JS handlers
    from firing (unlike native `disabled`). Verify the existing buttons already guard
    this in the handler, not just visually.
- `aria-busy={actionLoading === "<action>"}` during the in-flight RPC call, paired with
  `ActionButtonLabel`'s pending-state rendering (spinner/label swap) — reuse verbatim
  for "Approve Plan" / "Request Changes".
- Every button already carries a stable `data-testid` (`backlog-action-approve-plan`,
  etc.) — required per `.claude/rules/e2e-test-conventions.md`'s "locators: data-testid
  or ARIA roles only" rule. A new "Request Changes"/"Reject Plan" button must get its
  own `data-testid` (e.g. `backlog-action-reject-plan`) up front so e2e tests aren't
  retrofitted onto CSS selectors.
- Button pair grouping: wrap both in the existing `role="group" aria-label="Item
  actions"` container pattern already used at `ActionsSection.tsx:78`, or a nested
  `role="group" aria-label="Plan review"` if they should be visually/semantically
  distinguished from the rest of the actions panel — recommended, since "approve/reject
  this specific plan" is a conceptually distinct decision from the other lifecycle
  actions in the same panel (spawn session, trigger triage, etc.).

### Markdown content viewer with inline commenting affordance
- The rendered markdown container itself needs no special ARIA role (a plain `<div>`
  matching `DescriptionSection.tsx`'s existing `markdownBody` pattern is correct — it's
  static readable content, not a widget).
- Per-line/section comment affordances (the gutter-click pattern research/stack.md
  recommends reusing from `DiffRenderer.tsx`) need: each "add comment" trigger to be a
  real `<button>` (not a `<div onClick>`) with an `aria-label` naming the target line/
  section (e.g. `aria-label="Comment on line 42"`), reachable by keyboard (Tab order
  follows DOM order — ensure the gutter buttons aren't visually-positioned via absolute
  CSS in a way that breaks logical tab order per
  `.claude/rules/css-architecture.md`'s inline-style-for-layout warning), and the
  resulting comment composer should move focus into its textarea on open and return
  focus to the triggering gutter button on close/cancel (standard focus-management
  requirement for any inline-disclosure widget, WCAG 2.4.3 Focus Order).
- If comments are threaded/list-rendered per line, use a semantic list (`<ul>`/`<li>`)
  per comment thread, not divs, so screen-reader users get list semantics ("3 items")
  for free.
- Large plan documents (see §4) mean the markdown viewer may need virtualization —
  if so, ensure virtualized rows remain individually reachable by native find-in-page
  (Ctrl+F) is a known weak spot of virtualized lists; flag as a tradeoff to the
  Architecture dimension rather than solving silently, since accessibility and
  performance pull in opposite directions here (a fully-rendered DOM is more
  accessible/searchable but heavier).

## Question 4: Error states and edge cases

| Case | Recommended UX | Precedent to reuse |
|---|---|---|
| Plan artifacts path set but file missing/deleted from disk | Render an inline error *inside* the plan-content surface (not a page-level failure) — "Plan file not found at `<path>` — it may have been moved or deleted outside the app." Keep the path visible (already rendered as `<code>` today) so the user can investigate manually. | `InlineError`'s `role="alert" aria-live="assertive"` semantics ARE correct here (genuine failure, not a routine notice) — the opposite choice from the status-change case in §3. |
| Plan being regenerated while user is mid-review (new triage/plan run starts) | Do not silently swap the rendered content out from under an open review session. Show a non-blocking `InlineNotice` ("A newer plan is available — Reload to see it") with an explicit reload action, matching the exact `InlineNotice` action-button pattern (`InlineNotice.tsx:16-21`, `variant: "primary"` for the reload CTA) already used elsewhere for "this item changed elsewhere" cases (`ActionsSection.tsx`'s `terminalState` notice is the closest sibling precedent). Any comments the user was drafting against the *old* plan should be preserved in the composer (not discarded) so a regeneration mid-typing doesn't destroy unsaved feedback — this is a straightforward client-side state-preservation requirement, not a backend concern. |
| Empty/whitespace-only rejection reason | Client-side validation before the RPC fires: trim, and if empty, disable the submit button with `aria-disabled` + explanatory `title` (mirroring the manual-review-summary pattern already in `ActionsSection.tsx:302`: `disabled={!manualReviewSummary.trim() || actionLoading !== null}`) rather than allowing submission and returning a server-side error. This is a straight lift from existing code — `manual-review-summary`'s exact same guard shape applies to a new "rejection reason" textarea. |
| Very large `plan.md` files | Two independent concerns: (1) render performance — consider a "show first N lines, expand" pattern or virtualization only if profiling shows it's needed (don't pre-optimize); (2) reviewability — a very long plan defeats the "read it all before approving" goal from Q1's Cursor/Aider precedent. Consider a lightweight table-of-contents/section-jump affordance (markdown headings → anchor links) for long plans, similar in spirit to `DiffRenderer.tsx`'s existing file-jump sidebar pattern (`fileTree` + `scrollIntoView`, per stack.md Q3) — reuse that navigation shape rather than inventing a new one. |

## Question 5: Jobs-to-be-done

- **Functional job**: "When an AI pipeline proposes a plan for a backlog item, I want to
  catch a bad plan before implementation work is spawned, so that I don't waste a
  session (and review cycles) on work built on a flawed foundation." The approval gate
  is the causal lever for this job — its current narrow scope (queued-path only, per
  requirements.md finding #2) means the job is only partially served today; a plan can
  slip through un-reviewed via any non-queued path. Any UX design should make the gate's
  *actual* current scope legible to the user (don't imply universal protection the
  backend doesn't deliver) until/unless the Architecture dimension closes that gap.
- **Emotional job**: "I want confidence that the agent won't run off and build the wrong
  thing while I'm not watching." This is the job GitHub's persistent verdict badge and
  Claude Code's own plan-mode gate both serve — the emotional payoff comes specifically
  from the gate being *visible and trustworthy*, not from the approval mechanism's
  existence alone. A button that silently vanishes after approval (requirements.md
  finding #1) actively undermines this job even though the underlying data
  (`planApproved`) is correct — the user has no way to *feel* confident because the UI
  gives them nothing to check their trust against after the fact. This is the strongest
  argument in this research set for prioritizing the persistent status indicator over
  the line-level-commenting feature (Success Criterion 5) if the plan phase needs to
  size/cut scope — the emotional job is served by *visibility of state*, the
  line-commenting feature primarily serves the functional job's precision.
- **"Leaving a trail for future-self" (solo-tool social-job analog)**: even with no
  second human reviewer, a solo developer routinely revisits their own past decisions
  weeks later ("why did this plan change three times before I approved it?"). This is
  the job the timeline/history entry (§2) and preserved rejection-reason text serve —
  treat rejection reasons as a durable record (never silently overwritten by the next
  approval), not a transient form field that's discarded once acted on. This matches
  GitHub's own behavior (old review comments remain visible after a PR is later
  approved) and argues for storing rejection history as an append-only list rather than
  a single mutable "last feedback" field, if the Architecture dimension's data model
  allows it.

## Recommendation Summary

1. **Status model**: four states (no plan / pending review / approved / changes
   requested), rendered as a `BlockerChip`-style icon+label chip (never color-only) in
   the detail view, PLUS a timeline entry per state change — not a vanishing button.
2. **Reject flow**: reuse the `TriggerTriageRequest.feedback` wiring — "Request Changes"
   should be actionable (trigger the next plan cycle with the reason attached), matching
   the mental model set by Claude Code's own plan-mode review and this repo's existing
   triage-refinement pattern. Validate non-empty reason client-side before enabling
   submit, exactly like the existing `manual-review-summary` guard.
3. **Content surface**: promote `PlanArtifactsSection` out of default-collapsed when a
   plan is pending review (comparable products never gate "Approve" behind a hidden
   section); render via `react-markdown` + `markdownBody.css` (already proven at
   `DescriptionSection.tsx`, no new dependency).
4. **Line-level feedback**: hand-roll a gutter-click affordance modeled on
   `DiffRenderer.tsx`'s existing line-row pattern; every trigger must be a real
   `<button>` with a descriptive `aria-label`, not a bare clickable div.
5. **Accessibility baseline**: `aria-disabled` + `title` for gated buttons (existing
   `ActionsSection.tsx` convention), `aria-busy` during in-flight actions, `role="status"
   aria-live="polite"` for routine state-change notices vs. `role="alert"
   aria-live="assertive"` reserved for genuine errors (missing file, failed RPC) — both
   patterns already exist in this codebase (`InlineNotice` vs `InlineError`) and should
   not be conflated.
6. **Priority signal for the plan phase**: if scope must be cut, the emotional
   "confidence" JTBD is served primarily by the persistent status indicator (#1) and
   actionable reject flow (#2); line-level commenting (#4) is a precision enhancement on
   top of an already-served functional job and is the more defensible item to defer to
   a follow-up, per requirements.md's own "may land as a follow-up if scope is large"
   allowance.
