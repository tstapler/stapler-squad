# Adversarial Review: repo-path-picker-parity

**Date**: 2026-08-01
**Verdict**: CONCERNS

## Blockers

(none — both prior blockers are resolved)

- **AC5 vertical-clipping remediation** — RESOLVED. Story 3.1.1's AC5 GWT (plan.md:521-533)
  and Task 3.1.1f (plan.md:592-623) now commit to a single verification method:
  `page.evaluate()` + `getBoundingClientRect()` on the dropdown and modal elements,
  explicitly rejecting `boundingBox()` by name with the correct reasoning (it reports the
  element's own box regardless of clipping by an `overflow: hidden` ancestor). No hedging
  language remains — grepped the whole doc for `boundingBox`/`screenshot-diff` and the only
  two hits are both explicit rejections of `boundingBox()`, not an open choice between two
  methods. A real contingency task, `Task 3.1.1f-contingency` (plan.md:625-642), now exists:
  open-upward fallback via a `data-placement="above"` attribute + CSS flip in
  `RepoPathInput.css.ts` (matching the repo's `data-*`-attribute convention from
  `.claude/rules/css-architecture.md`, not an inline style), gated on Task 3.1.1f's
  assertions actually failing ("Trigger condition: only implement this task if..."), with
  files listed (`RepoPathInput.tsx`, `RepoPathInput.css.ts`, the spec) and a re-run step to
  confirm the fallback closes the gap. The Dependency Visualization diagram
  (plan.md:90-91) lists `3.1.1f` and `3.1.1f-contingency` in the same order and naming as
  the body — numbering is consistent, not just present in one place.

- **Missing `session-create-existing-worktree.spec.ts` regression task** — RESOLVED. Task
  3.1.1h (plan.md:652-664) mirrors Task 3.1.1g's shape exactly (same "no file changes
  expected, if it fails the regression must be fixed in the replacement not the spec"
  language) and is explicitly scoped as "Required, blocking verification step after Task
  2.1.2a lands," directly matching the requested remediation. It names the specific
  assertions this ticket cares about — `page.getByLabel('Existing Worktree Path').fill(...)`
  and the empty-path `canSubmit` gating test — which is the concrete coverage AC6's "no
  regression to canSubmit gating" claim needed. The Dependency Visualization diagram
  (plan.md:93) lists `3.1.1h` right after `3.1.1g`, matching the body's task numbering, and
  Story 3.1.1's Files list (plan.md:544) and its own new GWT bullet (plan.md:538-543) both
  reference the file — this isn't just a stray task with no story-level acceptance
  criterion backing it.

Both previously-blocked items are concretely and consistently resolved — not just
re-worded to sound resolved. The fixes hold together: task numbering in the diagram matches
the body, the contingency task has a real approach and real files, and the new verification
task has the same rigor (blocking, no-workaround-allowed) as its precedent.

## Concerns

All 4 prior CONCERNS were addressed as cheap doc additions, confirmed present:

- **`usePathCompletions` error-swallowing** — now documented (plan.md:136-138) as a
  one-line acknowledgment under Story 1.1.1, worded almost exactly per the prior
  recommendation ("pre-existing behavior... not fixed by this plan"). No test was added for
  it (the recommendation said "ideally add a unit test," not required) — acceptable, this
  was a "document the decision" ask primarily.
- **`ux.md` §4 existence-validation override** — now acknowledged in the Pattern Decisions
  table (plan.md:42): "research/ux.md §4's recommendation... was reviewed and is knowingly
  deferred as a follow-up — not missed or overlooked." Matches the requested remediation
  verbatim in spirit.
- **Mock restructuring not budgeted** — now called out explicitly as a "Prerequisite
  sub-step" in Task 1.1.1b (plan.md:163-170), with the exact restructuring pattern shown
  (`jest.fn()` + `mockReturnValue`) and a note that later tasks (1.1.1c, 1.1.2b) assume it's
  already done. Fully addressed.
- **Ambiguous Phase 1/Phase 2 merge boundary** — resolved by rewording both Risk Control
  (plan.md:53, "Phases 1-3 ship as ONE PR, not staged across multiple PRs... there is no
  reason a change this small needs an interim merge point") and the Dependency
  Visualization's Phase 2 header (plan.md:74-75, "depends on Phase 1's code existing in the
  same PR — built sequentially, shipped together as a single atomic change; see Risk
  Control"). The two sections now agree with each other instead of contradicting.

No new concerns surfaced during this re-review of the diff. Nothing flagged here needs to
block merge.

## Minors

- **Touch-target sizing note (prior Minor 3) — not carried into the doc.** The prior review
  recommended "worth a one-line comment in the e2e test itself noting touch-target sizing
  is intentionally unverified." Grepped plan.md for `touch-target`/`tap-target`/`tappable`
  — no hits. Task 3.1.1f (plan.md:592-623) still only asserts overflow/clipping, with no
  note (in the plan, or a forward-pointer to add one in the test) that ~30px dropdown rows
  vs. the 44px comfort target remains intentionally out of scope. Low stakes — this was
  flagged as a "future reader" clarity nit, not a functional gap, and the underlying
  omission (touch-target sizing) was already correctly identified as pre-existing/
  out-of-scope in `pitfalls.md`/`ux.md` before this plan existed. Worth a one-liner in
  Task 3.1.1f if it's cheap at implementation time, but not worth blocking or re-reviewing
  over.
- **Shared `id="omnibar-existing-worktree"` safety reasoning (prior Minor 1)** — carried
  forward correctly. Story 2.1.2's GWT (plan.md:439-444) now spells out the mutual-
  exclusivity reasoning inline ("the three render branches... are mutually exclusive arms
  of a single ternary, so exactly one element carrying that id is ever present"), matching
  the prior recommendation.
- **Combobox a11y triad has no lettered AC (prior Minor 2)** — the traceability caveat is
  still present verbatim in Story 1.1.2's header (plan.md:223, "supports AC5's broader
  UX-quality bar; not a lettered AC of its own but explicitly recommended by ux.md"). This
  was already documented before this revision and doesn't need further changes — it's a
  standing, acknowledged traceability gap, not a defect.
