# Research: UX — backlog-operator-feedback-loop (Gaps 1 & 2)

**Dimension**: UX | **Phase**: 2 — Research | **Agent**: 5 (UX)

## Reuse note (Gap 3 — read, not re-derived)

`project_plans/plan-approval-ux/design/ux.md` and its `research/ux.md` already fully design
Gap 3 (plan reject/request-changes), including the `PlanVerdictBox` 5-state card, the
reject-with-reason inline form, the "Regenerate Plan with This Feedback" two-button pattern
(ADR-002: `RejectPlan` only persists state; regeneration is a separate, explicit click that
calls the *existing* `triggerTriage(item.id, item.planRejectionReason)`), and 20 numbered
accessibility ACs. That work is **directly reusable almost verbatim** for this project's Gap
3 acceptance criteria (3, 4, 5), with three updates needed because it's 111 commits stale:

- **Re-verify line numbers/props cited** (`GateVerdictBox.tsx`, `ActionsSection.tsx`,
  `InlineNotice.tsx`, `InlineError.tsx`) against current HEAD before the plan phase copies
  them — this research pass did not re-audit Gap 3's file, since it's out of this agent's
  assigned scope (Gaps 1/2 only); flag to whichever Phase 2 agent owns Gap 3/architecture.
- **`InlineNotice` prop-shape bug already found and documented** (§4 of the design doc) —
  carry the corrected `actions: InlineNoticeAction[]` shape forward, don't rediscover it.
  Reference: `web-app/src/components/common/InlineNotice.tsx:23-28` — verified against
  current HEAD in this pass (see Gap 2 section below); prop shape is unchanged from what
  the reused doc's §4 flagged, so the fix is still valid, not just historically valid.
- **The two flagged gaps most likely to still matter**: §7.1 (Cancel doesn't return focus
  to the toggle button — a one-line fix, apply it in this pass rather than re-inherit the
  omission) and §7.2 (Approve/Reject controls render above the plan content that gates
  them — decide explicitly rather than silently keep the contradiction).

This document below covers **Gaps 1 and 2 only**, which have no prior art.

---

## Gap 1: Per-question answer affordance in `TriageDiffSection.tsx`

### Current state (verified against HEAD)

`web-app/src/components/backlog/TriageDiffSection.tsx:87-97` renders the "Triage Questions"
section as flat, read-only text:

```tsx
{questionSuggestions.length > 0 && (
  <div className={styles.questionsSection}>
    <h4 className={styles.questionsHeading}>Triage Questions</h4>
    {questionSuggestions.map((q, i) => (
      <div key={i} className={styles.questionItem}>
        {q.text}
      </div>
    ))}
  </div>
)}
```

No per-question identity survives past render — `q` is a `TriageSuggestion` with only
`text`/`rationale` (no stable ID field observed in the type), keyed by array index `i`. The
existing "answer" path is `TriageReviewPanel.tsx`'s unrelated "Refine triage with feedback"
box (lines 301-365): a single free-text textarea, generic to the whole triage result, that
calls `onRefine(feedback)` → `triggerTriage(item.id, feedback)`. Today an operator must
re-read the question, hold it in working memory, and retype/paraphrase it into that separate
box — exactly the problem statement in requirements.md.

### Comparable pattern: GitHub PR review comment "Reply" affordance

The canonical UI for "answer one specific structured item without retyping it" is GitHub's
inline PR review comment reply box. Its properties, in order of how directly they transfer:

1. **The reply composer is anchored directly under the specific comment**, not in a
   page-level box. Clicking "Reply" opens a small textarea *inline*, immediately below that
   one comment — never a shared textarea serving all comments on the PR. This is the direct
   fix for Gap 1: give each question its own disclosure toggle + inline textarea, not one
   shared feedback box for every question.
2. **The original text is never retyped or re-selected** — the reply is visually nested
   under the parent (indentation + a connecting rail/border), so the question-answer pairing
   is legible from layout alone, not from prose ("Re: ..."). This maps to AC1's literal
   wording: "without retyping the question text."
3. **Reply composers are collapsed by default and open on demand** (a "Reply..." affordance,
   not an always-visible textarea per comment) — this scales to N questions without turning
   the section into a wall of empty textareas. Directly relevant here since a triage run can
   emit multiple questions in one `suggestions` array.
4. **Submitting a reply does not require also resolving a top-level review verdict** — reply
   and "submit review" are decoupled actions. The stapler-squad analog: submitting one
   question's answer should not require the operator to also decide Apply/Skip on the AC
   diff in the same panel — the two should be independently actionable, matching
   `TriageReviewPanel`'s existing separation of the diff-apply flow and the refine-feedback
   flow into different action groups.

### Recommended structure for `TriageDiffSection`'s questions block

Per-question row, each independently disclosable:

```
┌─ Triage Questions ──────────────────────────────────────────────────┐
│ ❓ Should the retry limit be configurable per-workflow or global?    │
│    [ Answer ▸ ]  data-testid="triage-question-answer-toggle-{i}"    │
│                                                                       │
│    (on toggle) ┌────────────────────────────────────────────────┐  │
│                │ [textarea, focus-on-open]                       │  │
│                │ data-testid="triage-question-answer-input-{i}"  │  │
│                │                        [ Cancel ]  [ Submit ]   │  │
│                │  data-testid="triage-question-answer-submit-{i}"│  │
│                └────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────┘
```

- **Identity/keying caveat (flag for architecture/plan phase)**: `TriageSuggestion` has no
  stable ID today — `TriageDiffSection` keys questions by array index `i`
  (`questionSuggestions.map((q, i) => ...)`). An index-keyed `data-testid` is stable *within
  one triage result render* but will collide/shift if a re-triage changes the number or
  order of questions before the operator finishes answering an earlier one. This is fine for
  v1 given the stated "stateless, no persistent Q&A model" scope decision (requirements.md
  open question 2), but the plan phase should confirm the per-question toggle's open/closed
  local state (`useState<Set<number>>` or similar) is reset/discarded on `triageResult`
  identity change (e.g. new `iteration` number) rather than silently misapplied to a
  different question at the same index.
- **Submission semantics (AC1 + AC2)**: Submit should compose a feedback string that
  preserves the question↔answer link — e.g. `"Q: <question text>\nA: <answer text>"` — and
  call the **existing** `onRefine(feedback)` → `triggerTriage(item.id, feedback)` path
  (`BacklogItemDetail.tsx:787-793`'s `handleRefineTriage`), per AC2's explicit "no new
  triage mechanism." If multiple questions could be answered before submitting once, that's
  a scope decision for the plan phase (batch vs. per-question submit) — this research
  recommends **per-question submit** (matches GitHub's per-comment reply model, avoids a
  "did I lose my other answers" concern if one submit fails) but flags it as a plan-phase
  call, not a foreclosed one.
- **Visual connection to re-triage**: after a successful per-question submit, that question
  should visually mark itself "answered" (e.g. a small ✓ checkmark + the answer text
  rendered read-only below it, matching Google Docs' resolved-comment treatment cited in the
  Gap-3 prior art's research doc) so the operator can see at a glance which of N questions
  they've already acted on — this directly serves the "no retyping, no losing track" job.
  Note this is purely a **client-side, session-local** visual state (per requirements' "default
  to stateless" scope note) — it does not persist across a page reload or a new triage run;
  the plan phase should decide whether that's an acceptable v1 cut or whether the "answered"
  marker needs to survive a re-triage (open question 2).

### Reusable styling precedent

`TriageDiffSection.css.ts` already uses vanilla-extract (`style()` from `@vanilla-extract/css`,
`vars` from `@/styles/theme.css`) — the existing `questionsSection`/`questionsHeading`/
`questionItem` styles are the base to extend, not replace. Per
`.claude/rules/css-architecture.md`, new styles (toggle button, inline textarea, submit/cancel
row) belong in this same `.css.ts` file using `vars.*` tokens — no new `.module.css`, no
hardcoded colors. The inline textarea/submit-row shape can mirror `TriageReviewPanel.css.ts`'s
`refineForm`/`refineTextarea`/`applyButton`/`skipButton` styles (already proven, already
token-based) rather than inventing new visual language for what is functionally the same
"open a form, type feedback, submit or cancel" interaction at a smaller scale.

### Accessibility

- Each "Answer ▸" toggle is a real `<button>` with `aria-expanded` reflecting open/closed
  state and `aria-controls` pointing at its textarea's `id` — same pattern
  `GateVerdictBox.tsx`'s override-toggle already establishes (cited in the Gap-3 design doc,
  §1 step 2) and should be reused verbatim, not reinvented per-question.
- Focus moves into the textarea on open (`useEffect`), and **on Cancel it returns to the
  toggle button that opened it** — do the right thing from the start here rather than
  inheriting the Cancel-focus-return gap the Gap-3 design doc explicitly flagged (§7.1) as a
  pre-existing omission in `GateVerdictBox`. This component is new, so there's no excuse to
  copy the bug forward.
- Submit is `aria-disabled` + `disabled` while the trimmed answer is empty, with the guard
  enforced in the click handler (not just the attribute) — identical rule already documented
  in the Gap-3 research (`research/ux.md` in `plan-approval-ux`, §3: "`aria-disabled` does not
  block JS handlers").
- The question text itself (`q.text`) needs no new semantics — it's static readable content,
  same as today.
- If more than one question row can be open at once, each gets an independent `id`
  namespace (`triage-question-answer-{i}`) so `aria-controls`/`htmlFor` pairs don't collide —
  trivial but worth stating since the existing single-shared-textarea design never had to
  handle N simultaneous instances of the same form shape.
- Live-region choice: submitting one question's answer triggers a re-triage RPC
  (`triggerTriage`), which is a multi-minute LLM operation, not an instant state flip. Use
  `role="status" aria-live="polite"` for the post-submit "answered ✓" transition (routine,
  expected outcome of a user action, not a failure) — reserve `role="alert"
  aria-live="assertive"` only for a failed submit, mirroring the `InlineNotice` vs
  `InlineError` split already established project-wide (Gap-3 research §3, and
  `InlineNotice.tsx`/`InlineError.tsx` themselves).

### Mobile/touch

- The "Answer ▸" toggle and Submit/Cancel buttons must meet a ≥44×44px touch target (WCAG
  2.5.5, and this project's own precedent — `SessionRow.css.ts` already uses `@media`
  breakpoints from `web-app/src/styles/theme-contract.css.ts`'s exported `breakpoints`
  object, e.g. `breakpoints.sm = "640px"`). At narrow width, stack Submit/Cancel full-width
  rather than side-by-side inline buttons (the same collapse `TriageReviewPanel`'s
  `.actions` row likely already needs to make at mobile widths — verify during
  implementation, don't assume desktop-only testing covers it).
- Because multiple question rows can each independently expand, on a small viewport an
  operator answering question 2 while question 1's form is still open could lose track of
  scroll position after a submit re-renders the list — recommend the post-submit "answered"
  collapse happens in place (no scroll-jump), consistent with how `TriageReviewPanel`'s own
  apply/undo flow avoids scroll disruption today.
- No hover-only affordances: the "Answer ▸" toggle must be reachable/visible without a
  hover state (already true for a plain `<button>`, called out only because some inline
  reply/annotation UIs — including GitHub's own line-comment gutter icons — are hover-reveal
  on desktop, which does not work on touch. Do not adopt that part of the GitHub pattern).

---

## Gap 2: Steer affordance in `SessionsSection.tsx`

### Current state (verified against HEAD) — two steering paths exist, not one

This is the most important finding for Gap 2, and it directly affects AC6/AC7's requirement
to "use the existing `steer_session` path — no parallel steering implementation": **there
are currently two different, non-interchangeable steering mechanisms in the codebase**, and
the general session list's Steer dialog (`SessionActionsOverflow.tsx`, the UI requirements.md
cites as "exists and works") uses the *narrower* one, not the MCP tool named in the
requirement.

**Path A — MCP tool `steer_session`** (`server/mcp/tools_terminal.go:638-699`, exposed as
`mcp__stapler-squad__steer_session`): works on any session regardless of `autonomousMode`.
Branches on session state:
  - Stopped `OneShot` session with a stored Claude conversation UUID → resumes via
    `claude --resume` subprocess (`inst.RunWithResume`, 5-minute timeout).
  - Otherwise → sends the message via tmux PTY `SendKeys` (5-second timeout), which requires
    a live PTY. If the session's tmux pane is gone (killed, or never a PTY-backed session
    type), this fails with `"send keys failed: ... Check that the session is running and not
    paused"`.

**Path B — Web UI "Give direction" button** (`SessionActionsOverflow.tsx:722-730` →
`onSteerAutonomousSession` → `page.tsx:289-292`'s `handleSteerAutonomousSession` →
`updateSession(sessionId, { steerMessage: message })`, a **`UpdateSession` RPC field**, not
the `steer_session` MCP tool): server-side (`server/services/session_service.go:2012-2020`)
this is **hard-gated**:

```go
if req.Msg.SteerMessage != nil && *req.Msg.SteerMessage != "" {
    if !instance.AutonomousMode {
        return nil, connect.NewError(connect.CodeFailedPrecondition,
            fmt.Errorf("steer_message can only be sent to sessions with autonomous_mode enabled"))
    }
    // ... controller.SendCommandImmediate(*req.Msg.SteerMessage + "\r")
}
```

The button itself is only ever rendered `{onSteerAutonomousSession && session.autonomousMode
&& (...)}` (`SessionActionsOverflow.tsx:723`) — the UI never even offers Path B unless
`autonomousMode` is already true, so the server guard is currently unreachable from the UI in
practice, but it documents the intended scope: Path B is "give direction to a running
autonomous-mode agent loop," a narrower concept than "steer any session."

**Consequence for Gap 2**: backlog-linked sessions (triage/review/work, per
`statusToRole` in `SessionsSection.tsx:61-66`) are **not autonomous-mode sessions** — they're
headless, prompt-driven, single-purpose runs. If Gap 2's Steer affordance reuses Path B
(`SessionActionsOverflow`/`onSteerAutonomousSession` verbatim, the obvious "just lift the
existing component" move), it will render nothing (gated on `autonomousMode`, which these
sessions don't have) or fail server-side with `FailedPrecondition` if the gate were bypassed.
**AC6/AC7 as literally worded ("uses the existing `steer_session` path") point at Path A (the
MCP tool), not Path B (the UI button as currently wired)** — this is a real architecture
decision, squarely inside the requirements' own "open question 3," and this UX research
cannot resolve it alone. Flag to the architecture-dimension research and Phase 3 plan
explicitly: **does Gap 2 need a new RPC that calls the same underlying mechanism as the MCP
`steer_session` tool (PTY SendKeys / resume-subprocess), since `UpdateSession.steerMessage`
cannot serve non-autonomous sessions today?**

### Recommendation: full `SessionActionsOverflow` menu vs. Steer lifted out alone

**Recommendation: lift Steer out alone, do not embed the full overflow menu.**

Reasoning, from the JTBD lens (below) and existing UI conventions:

- `SessionsSection.tsx` already renders its own compact, purpose-built row per linked session
  (session ID, role, branch badge, cost, a Delete button) — it is not a general session
  management surface, and the requirements explicitly scope this to "backlog-linked sessions
  in the item detail view" (out-of-scope: "Steering affordances for all session types
  site-wide"). Most of `SessionActionsOverflow`'s menu (Rename, Clone, Change Program, New
  Workspace, Tags, Autonomous mode toggle, Clear Conversation, Checkpoint) is either
  irrelevant to a backlog-driven headless session or actively confusing in this context
  (e.g. "Run autonomously" toggle next to a triage session that isn't meant to run
  autonomously).
- Embedding the full `SessionActionsOverflow` component means every future addition to that
  menu (it already has 5 conditionally-visible groups) silently appears in the backlog item
  view too, unless each new menu item is individually re-scoped with backlog-awareness —
  a maintenance burden with no product upside here.
- The existing row already has exactly one secondary action rendered inline (Delete, as a
  plain `<button>`, not a menu) — a single "Steer" button following that same inline-button
  precedent (not a new `···` menu) is the smaller, more consistent addition. This also sides
  with **do the smallest thing that satisfies AC6/AC7** rather than importing an entire
  component whose scope doesn't match the surface.

Concretely: add a `Steer` button next to the existing Delete button in the non-synthetic
session row (`SessionsSection.tsx:146-165`'s `<a className={styles.sessionLink}>` branch),
visible only when the session is in a steerable state (see error states below), opening a
small inline composer (single-line input, matching the MCP tool's existing `SessionActionsOverflow`
Steer dialog's own input shape at lines 432-483 for interaction-pattern *consistency*, even
though this recommendation says don't reuse the *component* wholesale — reuse the *shape*:
label + text input + Send/Cancel, submit-on-Enter).

### Error states when the session is already terminated

The MCP tool's own error semantics (Path A, `tools_terminal.go`) already define the relevant
cases — Gap 2's UI should surface these, not invent new copy:

| Session state | What happens today (Path A) | Recommended UI treatment |
|---|---|---|
| Running, PTY-backed | `SendKeys` succeeds | Steer button enabled; submit shows pending state, closes composer on success. |
| Stopped, `OneShot`, has stored conversation UUID | Resumes via `claude --resume` subprocess (up to 5 min) | Steer button enabled even though `endedAt` is set — **do not gate the button purely on `!s.endedAt`** (`SessionsSection.tsx`'s existing `isOrphan` check uses exactly that signal for a different purpose — orphan-labeling — and must not be conflated with steerability). Submit should show a longer-running pending state (this is a subprocess call, not an instant PTY write) with the button disabled but not silently hung — `aria-busy` + a visible "Resuming…" label, since 5 minutes with no feedback reads as broken. |
| Stopped, not `OneShot`, or `OneShot` without a UUID | `findInstance` / PTY write fails — no live pane to send to | Steer button should be **disabled with an explanatory `title`** (e.g. "Session has ended — steering is unavailable"), following the exact `ActionsSection.tsx` convention (`aria-disabled` + `disabled={...}` + `title` for the non-obvious-reason case) the Gap-3 prior art already established as this codebase's standard for this exact problem shape. Do not render an enabled button that fails after a click — the terminated-vs-resumable distinction is knowable client-side from `endedAt` + `oneShot` + presence of a conversation UUID (mirror the server's own `RunWithResume` branch condition), so the UI can pre-compute disabled state rather than reactively catching an RPC error. |
| PTY write times out (`PTY_WRITE_TIMEOUT`) | Session may be blocked (e.g. mid-command) | Surface as a transient `InlineError`-style failure with a Retry, plus the tool's own remediation hint surfaced in copy: "The session may be blocked — try sending an interrupt first" (paraphrasing the MCP tool's own error `Resolution` string: `"The session may be blocked. Use send_control with key=C to interrupt"` — note this remediation path itself isn't exposed anywhere in `SessionsSection.tsx` today; flag as an out-of-scope-for-v1 nice-to-have, not a blocker). |

This table assumes Gap 2 ends up calling the same underlying mechanism as Path A (per the
architecture flag above) — if the plan phase instead decides to extend `UpdateSession` to
drop the `autonomousMode` gate for backlog-originated steer calls, re-derive this table
against that RPC's actual error surface instead (it currently has none beyond the one
`FailedPrecondition`, since it's never been exercised for a non-autonomous session).

### Jobs-to-be-done (Gap 2)

- **Functional job**: "While a triage/review/work session for this backlog item is running
  unattended, I want to redirect it without leaving the item I'm already looking at, so I
  don't lose the context I built up reading this item's history to figure out what course
  correction it needs." The causal lever is proximity — Gap 2 doesn't add a new capability
  (Path A/MCP steering already exists and works from agent tooling), it removes a navigation
  tax on a human doing the equivalent thing from the UI.
- **Emotional job**: "I want to feel like I can intervene, not just watch and hope." Same
  family as Gap 3's "confidence the agent won't run off and build the wrong thing" job (Gap-3
  research §5) — a visible, reachable Steer control *is* the reassurance, independent of how
  often it's actually clicked. A Steer button that's present but silently no-ops (the
  Path-B-gated-on-autonomousMode trap above) actively damages this job worse than not having
  the button at all, because it teaches the operator the control can't be trusted.
- **"Leaving a trail" job**: unlike Gap 3's rejection reasons (which are meant to persist),
  a one-off steer message is transient by nature (it's injected into a live PTY/conversation,
  not stored as item state) — no durable-record job applies here the way it does for Gap 1/3.
  Worth confirming this asymmetry explicitly so the plan phase doesn't over-build persistence
  for Gap 2 that the job doesn't call for.

### Accessibility

- Steer button: real `<button>`, `aria-label` naming the session (e.g. `"Steer session
  ${s.sessionId}"`, matching the existing Delete button's `aria-label="Delete session"`
  convention one line above it in `SessionsSection.tsx:169`).
- Disabled-with-reason state: `aria-disabled="true"` + native `disabled` + `title` — reuse
  `ActionsSection.tsx`'s pattern verbatim (already the established convention for "button
  present but not currently actionable, with an explained reason" in this codebase — see
  Gap-3 research §3 for the exact citation).
- Inline composer: focus-on-open into the input, Escape closes and returns focus to the
  Steer button (get this right from the start — see Gap 1's same note re: not inheriting the
  Cancel-focus-return gap the Gap-3 doc found in `GateVerdictBox`).
- Submit-in-flight: `aria-busy` on the Send button, input disabled during submit — especially
  important here since the resume-subprocess path can legitimately take up to 5 minutes
  (Path A's own timeout value), far longer than any other in-flight action pattern already
  in this codebase (`actionLoading`-style states elsewhere assume sub-second-to-a-few-second
  RPCs). A spinner alone reads as "hung" at that duration — recommend an explicit
  elapsed-time or "this can take a few minutes" caption for this specific case, distinct from
  the generic pending-button convention used everywhere else.

### Mobile/touch

- Same 44×44px minimum target as Gap 1's buttons. The existing Delete button in
  `SessionsSection.tsx` is small/text-only (`Delete`/`…`) — check its rendered hit area at
  mobile breakpoints before assuming the new Steer button can just match its current sizing;
  this is a good moment to fix both if the existing target is under-sized (this codebase's
  own `.claude/rules` don't currently call this out for `SessionsSection`, so verify rather
  than assume compliance).
- `SessionsSection`'s row layout (`sessionRowMain`) already packs session ID, role, branch
  badge, date, cost, and Delete into one row — adding Steer as another inline element risks
  overflow/wrap at narrow widths. Recommend following `SessionRow.css.ts`'s established
  pattern of `@media` breakpoint queries (`web-app/src/styles/theme-contract.css.ts`'s
  exported `breakpoints.sm`/`.md` etc.) to collapse secondary metadata (cost, date) before
  the action buttons at narrow widths, rather than letting Steer wrap awkwardly next to
  Delete. Do not solve this with `style={{ flexDirection: ... }}` inline overrides — per
  `.claude/rules/css-architecture.md`, use a `data-*` attribute + `selectors` in the
  `.css.ts` file instead.
- The inline composer (single-line input + Send/Cancel) should stack full-width below the
  row on narrow viewports rather than trying to fit inline — matches the same
  stack-on-mobile recommendation given for Gap 1's per-question form.

---

## Cross-cutting notes for the plan phase

1. **Gap 1 and Gap 2 both introduce a new "inline disclosure form" shape** (toggle → open
   form → focus-in → submit/cancel → focus-out). Recommend the plan phase define **one
   shared pattern** (hook or small wrapper component) for focus-on-open / focus-return-on-
   close / `aria-expanded`/`aria-controls` wiring, used by both, rather than three
   independently-hand-rolled copies (Gap 1's question answer, Gap 2's steer composer, and
   Gap 3's reject-with-reason form, which the Gap-3 design doc already specified
   independently). This is a DRY opportunity across gaps, not a requirement change.
2. **Two different `data-testid` naming families will exist post-implementation**:
   `backlog-action-*` (Gap 3's existing convention, per `ActionsSection.tsx`) and whatever
   this document proposes for Gap 1 (`triage-question-answer-*`, following
   `TriageReviewPanel.tsx`'s existing `triage-*` prefix convention) and Gap 2 (no existing
   prefix in `SessionsSection.tsx` today — recommend `session-steer-*`, mirroring
   `sessions-show-more`'s existing bare `session*`-prefixed testid one component over).
   Confirm these don't collide with `.claude/rules/e2e-test-conventions.md`'s locator
   requirements before the plan phase locks them in.
3. **Gap 2's architecture question (Path A vs. Path B) blocks UX sign-off on the exact
   button-disabled-state logic** in the error-states table above — this research documents
   both branches so the plan phase isn't blocked waiting on this doc, but the final
   disabled/enabled predicate depends on which RPC backs the new Steer button.
