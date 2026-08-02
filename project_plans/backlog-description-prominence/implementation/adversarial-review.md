# Adversarial Review: backlog-description-prominence
**Date**: 2026-08-01
**Verdict**: CONCERNS

Plan fidelity check: every concrete claim the plan makes about the current codebase was
verified against the live source and holds up exactly —
`BacklogItemDetail.tsx:323` (`useSectionExpandState(itemId, "description", false)`),
`BacklogItemDetail.tsx:1215` (`<DescriptionSection item={item} />`),
`DescriptionSection.tsx`'s hardcoded `defaultExpanded={false}` and stale docstring,
`DescriptionSection.test.tsx`'s three tests, `BacklogItemDetail.markdown.test.tsx`'s
`expandDescription()` helper and its doc comment, `Collapsible.tsx:137-163`'s
grouped-mode dead-code/divergence-warning behavior, `sectionExpandEntries` already
including `"description"` (`BacklogItemDetail.tsx:370-381`), and the sibling
`defaultExpanded`-as-prop pattern in `PlanArtifactsSection.tsx`/`NotesSection.tsx`/
`VersionControlSection.tsx`. No line-number drift, no stale-quote risk for Task
1.1.1a/1.2.1b's literal instructions. Failure-mode analysis confirms the plan's own
claim: `useSectionExpandState` only touches `localStorage` (try/catch-wrapped, falls
back to the passed default), no network/external dependency exists anywhere in this
change — Observability Plan's "None" entries are accurate, not an oversight.

This is a genuinely small, well-researched plan. Nothing below rises to blocking; all
three concerns are cheap to address and don't require replanning.

## Blockers
(none)

## Concerns
- [ ] **Epic 1.2's `DescriptionSection` prop refactor goes beyond requirements.md's stated appetite.** requirements.md says "Appetite: Extra small... a one-line default-value flip plus test coverage, not a redesign," and Task 1.1.1a alone (flip `false`→`true` at `BacklogItemDetail.tsx:323`) is independently sufficient per the plan's own Alternatives-Considered analysis (Collapsible.tsx's grouped mode makes the child's `defaultExpanded` prop architecturally inert). Epic 1.2 adds a second production file, a prop-signature change, and touches a docstring for pure internal-consistency reasons that requirements.md never asked for. The engineering rationale (matches ~8 sibling sections, removes dead/misleading code) is sound, but it's the kind of "while I'm in here" scope expansion the repo's own global instruction ("Do what has been asked; nothing more, nothing less") flags. — Recommend either getting explicit confirmation that Approach B is in scope before implementation starts, or defaulting to Approach A (Task 1.1.1a only) to match the requirements doc's literal appetite framing, with Epic 1.2 filed as a fast-follow cleanup if wanted.
- [ ] **Task 1.2.1a's replacement docstring still doesn't disclose that `defaultExpanded` is a no-op at the component's real production call site.** `DescriptionSection` is only ever rendered inside `CollapsibleGroup` in production (`BacklogItemDetail.tsx:1215`, inside the group starting at line 1185), where `Collapsible.tsx:137-142` makes `defaultExpanded` "architecturally dead" — the group's own `value`/`openSectionKeys` is what actually drives display. The plan's own Task 1.2.1b instruction acknowledges this in an inline comment ("this prop is a no-op inside the CollapsibleGroup"), but the proposed docstring text ("Expanded by default... unless the caller passes false") doesn't carry that caveat, so a future reader of `DescriptionSection.tsx` in isolation will still be misled about what the prop does in the app's one real usage — exactly the stale/misleading-doc problem Epic 1.2 exists to eliminate. — Recommend folding a one-clause caveat into the docstring itself (e.g., "...note: inert when rendered inside a CollapsibleGroup, which is how BacklogItemDetail always uses it — the group's controlled value is authoritative there").
- [ ] **Dependency Visualization claims 1.3.2a and 1.4.1a/1.4.2a are parallelizable, but both edit the same file.** Both `Task 1.3.2a` (guard `expandDescription()`) and `Tasks 1.4.1a`/`1.4.2a` (new `describe` block) modify `BacklogItemDetail.markdown.test.tsx`. If `sdd:5-implement`/subagent-driven-development dispatches these as literally-parallel fresh subagents per the diagram's fan-out, two agents editing the same file concurrently risks a lost update or a merge conflict at write time. — Recommend sequencing 1.3.2a before 1.4.1a/1.4.2a (both are trivial, sub-5-minute edits) or explicitly merging them into one task before dispatch, rather than relying on the diagram's "no dependency on each other" claim as written.

## Minors
- Task 1.2.1b's inline rationale — "this... keeps the divergence-warning check silent" — is not quite accurate. `Collapsible.tsx:151` only warns when `defaultExpanded===true && groupSaysOpen===false`; after Task 1.1.1a's flip, the group already reports `"description"` as open, so even *without* Task 1.2.1b's prop-threading (i.e., leaving the old hardcoded `defaultExpanded={false}` in place, Approach A), `false && !true` never fires the warning. The stated justification is backwards from what the code actually does — worth fixing the plan's prose, though it doesn't change what Task 1.2.1b should actually do (threading the prop is still correct for the internal-consistency reason, just not for the warning reason given).
- Success-metrics-to-test-coverage mapping is clean: requirements.md's two testable success metrics (fresh-item auto-expand; stored-collapse-preference persists) map 1:1 to Epic 1.4's two new tests (1.4.1a, 1.4.2a) with no gap and no unrequested extra test. The third metric (Acceptance Criteria's always-visible behavior preserved) correctly gets no new test since it's genuinely untouched code (`AcCriteriaList` renders outside the `CollapsibleGroup`, unaffected by this diff).
