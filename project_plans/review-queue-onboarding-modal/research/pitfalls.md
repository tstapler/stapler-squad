# Pitfalls / Risk Research: review-queue-onboarding-modal

Scope: risk assessment only, per requirements.md's "Scope for this pass." Root
cause is not re-derived here (see requirements.md).

## 1. Risk of the ReviewQueuePanel.tsx sentinel move

**Diff** (`git show HEAD -- web-app/src/components/sessions/ReviewQueuePanel.tsx`,
commit `2811df54e`):

```diff
   return (
     <div className={panel} data-testid="review-queue">
+      {/* Signals the initial fetch has resolved, independent of whether the
+          queue ended up empty — tests and other consumers use this to know
+          when it's safe to make assertions about queue contents. */}
+      {!loading && <div data-testid="review-queue-loaded" aria-hidden="true" />}
       {/* Screen reader live region for queue count changes */}
       ...
         ) : (
           <>
-            {!loading && <div data-testid="review-queue-loaded" aria-hidden="true" />}
             {groupedItems ? (
```

`web-app/src/components/sessions/ReviewQueuePanel.tsx:904-909` (new location,
top of the panel, gated only on `!loading`) vs. the old location at the top of
the non-empty-queue branch (`~line 1230`, gated on `!loading` **and**
`items.length > 0`).

**Repo-wide consumers** (`grep -rn "review-queue-loaded"`):
- `tests/e2e/escalation-reasoning.spec.ts:177` — `waitForSelector(..., state: 'attached')`
- `tests/e2e/review-queue.spec.ts:113,121,128,153` — three tests, all `toBeAttached()`/`waitForSelector(..., state: 'attached')`
- `web-app/src/lib/features/features/review-queue.ts:24` — feature-registry entry, references the test **name**, not DOM position
- `web-app/src/components/sessions/ReviewQueuePanel.tsx:909` — the sentinel itself

No consumer queries by sibling position, `:nth-child`, or DOM-tree structure — every
consumer is a flat `data-testid` attribute selector. Moving the node earlier in
the tree cannot break any of them.

**Behavioral risk assessment:**
- **Timing**: strictly earlier or equal. Old code rendered the sentinel only when
  `!loading && items.length > 0`; new code renders it whenever `!loading`, a
  strict superset of when it used to appear. No consumer waits for the sentinel
  to be *absent*, so making it appear in more states (loading-done-but-empty)
  cannot regress an existing pass.
- **Accessibility**: the node was `aria-hidden="true"` in both the old and new
  location (`ReviewQueuePanel.tsx:909`) — a genuinely empty, presentation-only
  marker div. It carries no text content, role, or focusable interactive element,
  so screen readers were never meant to encounter it either before or after the
  move; there is no AT-visible behavior change. The real screen-reader live
  region for queue count changes (`aria-live="polite"`, `ReviewQueuePanel.tsx:911-913`)
  is unrelated and untouched by this diff.
- **Other code paths**: `grep -rln SessionWizard web-app/src` (re-run this pass)
  still returns zero importers of `SessionWizard.tsx` besides itself, so the
  component tree containing the sentinel has no code path depending on the old
  position for anything besides the tests already enumerated above.

**Conclusion**: no behavioral risk identified. The change is a pure widening of
when a purely diagnostic, `aria-hidden` marker appears; every known consumer is
tolerant of (or specifically requires) that widening.

## 2. Risk of the two rewritten "Session Creation Flow (UI Only)" tests

Full diff (`git show HEAD -- tests/e2e/review-queue.spec.ts`, same commit),
`test.describe('Session Creation Flow (UI Only)', ...)` block, now at
`tests/e2e/review-queue.spec.ts:61-91`.

**Old tests** (targeted `SessionWizard`, confirmed zero importers / `@deprecated`
docblock, i.e. dead DOM):
- `session creation wizard has all steps` — asserted 4 `[data-testid="wizard-step-label"]`
  nodes with specific text ("Basic Info", "Repository", "Configuration", "Review")
  were visible.
- `session creation form has required test IDs` — filled `session-title` →
  clicked "Next" → filled `session-path` → clicked "Next" → asserted
  `auto-yes-checkbox` visible → clicked "Next" → asserted `create-session-button`
  visible. A 4-step wizard-navigation walk, never actually submitting.

**New tests** (target the real live component tree, `OmnibarCreationPanel.tsx`
reached via `/sessions/new`'s client-side redirect):
- `session creation opens the omnibar with a session source input`
  (`tests/e2e/review-queue.spec.ts:70-75`) — asserts
  `input[aria-label="Session source input"]` is visible.
- `session creation form has required fields` (`:77-91`) — types `/tmp` into
  the source input (triggers `LocalPathDetector`, switching Omnibar from
  discovery to creation mode), then asserts a `radiogroup` named "Session Type",
  a "Session Name" textbox, clicks the "Existing folder" radio, asserts a
  "Working Directory" field appears, and asserts a "Create Session" button is
  visible (`.first()`, with an inline comment flagging a pre-existing UI
  duplication — two simultaneously-rendered "Create Session" buttons — as a
  separate, un-fixed issue worth its own follow-up).

**Coverage comparison:**
- **Locator style**: new tests use ARIA role/label locators exclusively
  (`getByRole`, `getByLabel`, `aria-label` attribute selector) — compliant with
  `.claude/rules/e2e-test-conventions.md`'s "Locators: data-testid or ARIA roles
  only" rule. The old tests used `data-testid` selectors that, while compliant
  with the *rule*, pointed at nodes that do not exist in any reachable render
  path — so they were not exercising real coverage.
- **User-facing capability tested**: both old and new versions test "can a user
  reach the session-creation form and see the fields needed to create a
  session from `/sessions/new`" — neither ever exercises actual submission
  (`create-session-button`/`Create Session` visibility is asserted, click is
  not). This limitation is identical before and after, not introduced by the
  rewrite.
- **What the rewrite adds that the old test didn't test**: exercising the
  `LocalPathDetector` mode-switch (discovery → creation) is new coverage — the
  old wizard test never had a discovery/detection step to cross since
  `SessionWizard` was a fixed multi-step form with no input-driven mode
  transition.
- **What the rewrite can't test that the old test conceptually could**: a
  literal multi-step "Next" navigation flow across 4 named steps. This isn't a
  coverage gap so much as a reflection of current UI reality —
  `OmnibarCreationPanel.tsx` is a single-view form, not a wizard, so there is
  no multi-step flow left to assert on. A test asserting 4 wizard steps would
  necessarily fail against the current UI regardless of authorship, since those
  steps don't exist.

**Conclusion**: the rewrite is not a like-for-like swap of assertion targets on
an unchanged capability — it is a correction from asserting on dead code (a
component with zero importers, confirmed by `grep -rln SessionWizard
web-app/src` returning only `SessionWizard.tsx` itself and an unrelated
generated proto symbol) to asserting on the actual code path a user hits at
`/sessions/new`. Rigor is comparable (fields visible after correct navigation,
no actual submission tested in either version); the new tests additionally
exercise the detector-driven mode switch the old tests had no equivalent for.

## 3. Procedural handling of a fail-marked AC among otherwise-passing ACs

**Tooling contract** (from `report_progress` / `request_review` / `submit_review_verdict`
tool descriptions, loaded this session):
- `report_progress(status="fail")` on one criterion "marks it blocked" — it does
  not block calling other tools or gate `request_review`.
- `request_review`'s own description says "Call after all acceptance criteria
  are marked pass" — this item will violate that literally (AC4 marked `fail`).
  There is no alternative tool for "submit for review with a known-failing
  criterion and an explanation"; `request_review`'s `message` and
  `verification_notes` fields are the only channel to carry that explanation
  to the reviewer.
- `submit_review_verdict` (reviewer-side, not available to this session) takes
  independent per-criterion outcomes (`PASS|FAIL|PARTIAL|UNVERIFIABLE`) — a
  `FAIL` on any criterion sends the whole item back for rework, `PASS` on all
  transitions it to done. There is no "accept with a known exception" verdict
  in the tool surface; a human reviewer applying judgment to the evidence is
  the only path to accepting a fail-marked-but-justified AC as ultimately
  correct.

**Procedural guidance found** (`.claude/commands/backlog/review.md`, loaded via
the `backlog:review` skill listing):
> FAIL or PARTIAL means fixing the noted gaps in this same session and running
> `/backlog/review` again. Keep count of how many times you have run
> `/backlog/review` in THIS session — nothing tracks it for you. After 3 review
> cycles without a PASS, stop looping: run `/backlog/ship anyway` to open a PR
> so a human can pick up the review directly, rather than retrying
> `/backlog/review` again.

This is the item's **second** review cycle (first cycle's verdict, per
requirements.md, was FAIL on AC4 for the same reason this pass re-confirms).
The doc gives no separate "AC conflicts with ground truth" escape hatch inside
the review loop itself — the only sanctioned outs are (a) satisfy the AC as
written, or (b) exhaust 3 review cycles and hand off to a human via
`/backlog/ship`. It does not say an agent may unilaterally decide an AC is
wrong and mark it `pass` anyway.

**No standing rule found permitting override.** Searched `.claude/rules/*.md`
and `project_plans/**/*.md` for "acceptance criteri*" combined with
conflict/override/cannot-be-met language:
- No `.claude/rules/*.md` file mentions acceptance-criteria conflicts at all.
- The one directly on-point precedent, `project_plans/terminal-resize-fit-loop/decisions/ADR-001-add-xterm-addon-canvas-dependency.md:56-72`,
  is the **opposite** direction of this case: the AC (a literal "canvas
  renderer" requirement) was honored even though it created friction with a
  separate default expectation ("no new dependencies"), and the agent wrote an
  ADR explaining why literal compliance was chosen over a looser
  reinterpretation. It establishes the norm "when an AC's literal wording is in
  tension with something else, write it down and justify the choice made" —
  but in that case the AC itself was still achievable; here it is not.
- `project_plans/bulk-select-ux/implementation/architecture-review.md:21` is a
  reviewer (not the implementing agent) flagging that an AC "cannot be met as
  written" and recommending the *plan* be changed before implementation — i.e.
  the sanctioned move when an AC turns out to be unsatisfiable is to surface it
  explicitly (to a plan reviewer, or here, to the human backlog reviewer) and
  let a human decide, not to silently substitute a different requirement.
- `project_plans/backlog-event-driven-updates/implementation/adversarial-review.md:22`
  is the same pattern: an AC modeled a wrong data shape, a reviewer caught it,
  and the recommendation was to **correct the AC's own text**, not to
  reinterpret it silently during implementation.

**No formal "PARTIAL" self-declaration mechanism exists.** `report_progress`'s
`status` enum is only `pass|fail|in_progress` — there is no `partial` status an
agent can set; `PARTIAL` only appears as a reviewer-side `submit_review_verdict`
outcome. So "AC4 fail, AC1/2/3/5/6/7 pass" is the closest an agent can get to
signaling a partial result through the progress-tracking tool itself; the
narrative nuance ("literally unsatisfiable, not skipped/incomplete work") has
to live in `report_progress`'s `note` field and in `request_review`'s
`message`/`verification_notes`.

## Recommendation

Call `report_progress(criteria_index=3, status="fail")` with a `note` that
points to this document and to requirements.md's "AC4 question" section
(one line, not a re-argument) — then call `report_progress(status="pass")` for
AC1/2/3/5/6/7 with their respective verifying evidence (test names / counts).
In `request_review`'s `message`, lead with the one sentence that matters to a
human skimming the queue: *"AC4 cannot be satisfied together with AC2 given
the real, verified root causes (production-code sentinel bug + dead-component
selectors) — recommend accepting the fix as-is or explicitly amending AC4."*
Put the supporting evidence (both root causes, the "no test-only alternative
exists" argument for each, and this document's file:line citations) in
`verification_notes`, not the summary `message`, so the reviewer sees the
one-line ask first and the full case only if they need it. Do not mark AC4
`pass` — that would repeat exactly the "silently overriding it a second time"
the requirements doc says this pass is deliberately avoiding, and no rule or
precedent found in this research authorizes an implementing agent to overrule
a literal AC unilaterally; a human decision either way (amend AC4, or affirm
the fail and accept the fix regardless) is what's missing, not more evidence.
