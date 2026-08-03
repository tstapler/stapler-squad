# Adversarial Review: backlog-description-prominence

**Date**: 2026-08-02
**Verdict**: CONCERNS

## Method

Read `requirements.md` and `plan.md` in full, plus `research/pitfalls.md`. Independently
verified the plan's factual claims against the actual source rather than trusting the
plan's own citations: read `DescriptionSection.tsx`, `Collapsible.tsx` (both the
`insideGroup` branch and the standalone branch), the relevant line ranges of
`BacklogItemDetail.tsx` (seed line, call site, `AcCriteriaList` block), all three
targeted test files (`DescriptionSection.test.tsx`, `BacklogItemDetail.markdown.test.tsx`,
the roving-tabindex test in `BacklogItemDetail.test.tsx`), `tests/e2e/backlog-item-detail-redesign.spec.ts`
(both the file header and the specific test block being rewritten), `BacklogItemDetailPage.ts`,
`useSectionExpandState.ts`'s existing hook-level test file, and the `Makefile` targets
`quick-check`/`ci`/`lint`/`build` to check what they actually execute.

## Blockers

None. This is a genuinely small, well-scoped, well-researched change: one boolean seed
flip plus mechanical prop-threading through a single existing call site, confined to
exactly the 5 files requirements.md names. Every line number, code snippet, and behavioral
claim in the plan that I independently checked against the actual source (Collapsible.tsx's
grouped-vs-standalone branches, the `defaultExpanded` dead-code analysis, `NotesSection`'s
required-prop pattern, the `expandDescription()` toggle-inversion bug, `useSectionExpandState`'s
existing stored-preference test coverage) turned out to be accurate. Nothing here rises to
"must fix before implementation starts."

## Concerns

- [ ] **The e2e spec the plan modifies is documented as never having been executed, and the verification gate (Story 1.1.3) never runs it either.** `tests/e2e/backlog-item-detail-redesign.spec.ts`'s file header states outright: "this spec was written and type-checked ... but was NOT run against a live ./stapler-squad instance in this environment ... treat it as unexecuted until a real run confirms it." Task 1.1.2c rewrites this file's assertions (new `aria-expanded="true"` default, a raw `.click()` to force collapse, then `expandSection()` to re-expand) purely from static reasoning about Radix/Playwright behavior, and Task 1.1.3a's verification gate only runs `npx jest --testPathPatterns=...` + `make quick-check` — neither touches Playwright at all. So the one test file whose entire job is proving AC5 ships without ever being run, layered on top of a spec that was already unverified before this change. — **Recommendation**: add a task to Story 1.1.3 that starts an isolated `e2e-local` instance (per `CLAUDE.md`'s E2E Tests section — `global-setup.ts` auto-manages this, no manual server needed) and runs `cd tests/e2e && npx playwright test backlog-item-detail-redesign.spec.ts`, or explicitly note in the plan that this remains an accepted, unexecuted risk if a live run genuinely isn't feasible in this environment.

- [ ] **`make quick-check` does not validate the frontend at all beyond the TypeScript compile step inside `next build`.** Reading the `Makefile`: `quick-check` = `build test-coverage test-race lint lint-css-tokens registry-diff`, and `lint` runs only `golangci-lint` (Go) + a custom Go linter — there is no ESLint or Jest invocation anywhere in `quick-check` or `ci`. It does catch the required-prop wiring error (Next.js's `next build` type-checks by default, confirmed no `ignoreBuildErrors` in `next.config.ts`), but requirement 6's phrasing ("`make quick-check` ... must all pass") reads as if it's a meaningful frontend gate on its own. Task 1.1.3a compensates correctly by also running the explicit `npx jest --testPathPatterns=...` command, so this doesn't threaten correctness — it's a documentation-precision gap, not a functional one. — **Recommendation**: add one clause to AC6's Given-When-Then clarifying that `make quick-check`'s only frontend-relevant effect here is the `next build` TS check, and the actual frontend test coverage comes from the separately-run `npx jest` command.

- [ ] **The claim that `BacklogItemDetail.test.tsx`'s roving-tabindex test is unaffected is reasoned, not executed, despite being trivially checkable.** `pitfalls.md` §5 and the plan both argue (correctly, per Radix's Accordion.Trigger semantics) that focus/keyboard-nav behavior at lines 995-1023 doesn't depend on open/closed state — but this was never actually run, and Task 1.1.3a still frames it as an open question ("if it fails, investigate ... this is the one file not fully audited"). I independently grepped the full test file for `description`/`aria-expanded` and confirmed the roving-tabindex test asserts only `.focus()`/`.toHaveFocus()`, nothing about expand state, so the claim does hold up — but the plan carried this as a live risk into the implementation gate rather than closing it during planning, when confirming it took under five minutes. — **Recommendation**: none needed functionally (the claim is correct), but future plans in this repo should spend the few minutes to directly verify a flagged "not fully audited" file during planning rather than deferring to "investigate if it fails" at the verification gate.

## Minors

- The plan's Domain Glossary and Pattern Decisions sections are notably heavyweight (multiple paragraphs of dead-code analysis, a full domain glossary table) for a change the plan itself correctly judges not to warrant a formal ADR. Not harmful — the analysis is accurate and the extra rigor is what caught the "flipping only the internal `defaultExpanded={false}` would be a silent no-op" pitfall in the first place — just noting the ceremony-to-diff ratio is unusually high for a 3-line functional change.
- Task 1.1.2c deliberately avoids adding a `collapseSection()` method to `BacklogItemDetailPage.ts` (correctly, to respect the constrained file list) and instead reuses a raw `.click()` on `detailPage.sectionHeader("description")`. This creates a slightly asymmetric page-object API (`expandSection()` exists as a named, guarded helper; "collapse" does not and must be done inline) — acceptable given the stated constraint, but worth a one-line comment in the spec itself (which the plan's snippet already includes) so a future reader doesn't go looking for a nonexistent `collapseSection()` helper.
- Acceptance Criterion 2 (stored preference wins) has no new dedicated test at the `DescriptionSection`/`BacklogItemDetail` level — this is correct and sufficient, not a gap: I confirmed `useSectionExpandState.test.ts` already generically covers "stored value wins over default" and "falls back to default when nothing stored" for arbitrary section keys, and that hook is explicitly out of scope/unmodified. Calling this out only so it's clear the omission was verified as intentional, not overlooked.

## Verification of the four specific checks requested

1. **Failure modes** — N/A, confirmed honestly rather than invented. This is a synchronous client-side default-value change (one `useState` seed argument) with no network call, no async boundary, and no external dependency anywhere in the diff (`DescriptionSection.tsx` render is pure; the changed `useSectionExpandState` call already existed and reads `localStorage` synchronously with its own established try/catch fallback, unmodified by this plan). There is no retry/timeout/error-path category that applies here.
2. **Architecture risks** — None found. `DescriptionSection` has exactly one call site in production code (`BacklogItemDetail.tsx`); `CollapsibleSection`/`CollapsibleGroup` are unmodified, well-isolated, and already unit-tested independently of this component. No new coupling introduced.
3. **Scope drift** — None. The plan's five touched files (`DescriptionSection.tsx`, `BacklogItemDetail.tsx`, `DescriptionSection.test.tsx`, `BacklogItemDetail.markdown.test.tsx`, `tests/e2e/backlog-item-detail-redesign.spec.ts`) match requirements.md's constrained list exactly — verified by direct comparison, not by trusting the plan's own "Files" listings.
4. **Missing coverage** — All 6 numbered ACs have a corresponding task and a Given-When-Then (AC1/2/4 → Story 1.1.1; AC3/5 → Story 1.1.2; AC6 → Story 1.1.3). The roving-tabindex "unaffected" claim is asserted via sound reasoning and independently confirmed correct by me, but was not actually executed before/during planning — see Concern #3 above.
