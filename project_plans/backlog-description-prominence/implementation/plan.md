# Implementation Plan: backlog-description-prominence

**Feature**: `DescriptionSection` in `BacklogItemDetail` shows expanded by default (zero clicks) instead of collapsed, matching how load-bearing Description actually is versus the frequently-empty, already-always-visible Acceptance Criteria block.
**Date**: 2026-08-02
**Status**: Ready for implementation
**ADRs**: None. Root cause and fix are fully diagnosed by research (single seed-value flip + one required prop threaded through one existing component); the change is trivially reversible via `git revert` and touches no schema, no RPC, no cross-cutting pattern. Not worth the ceremony of a formal ADR.

---

## System Type

Pure frontend UI-default change inside the existing `@radix-ui/react-accordion`-backed `CollapsibleSection`/`CollapsibleGroup` pattern (`web-app/src/components/ui/Collapsible.tsx`). No proto, no backend, no new dependency, no CSS change. Two production files change (one seed-value flip, one prop threaded through one component + its single call site); three test files are updated to match the new default.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| **`defaultExpanded`** | Boolean prop on `CollapsibleSectionProps` and on each extracted detail-section component (`NotesSection`, `DescriptionSection`, etc.). Drives real initial open state **only** when the `CollapsibleSection` is *not* inside a `CollapsibleGroup` (`Collapsible.tsx:165-176`, `Accordion.Root defaultValue`). Inside a group (`insideGroup` branch, `Collapsible.tsx:137-163`) it is **architecturally dead** — the group's controlled `value` is what actually drives open state; a passed `defaultExpanded={true}` that disagrees with the group's state only triggers a dev-mode `console.warn` (`Collapsible.tsx:150-156`), it never overrides the group. | `DescriptionSection` is always rendered inside `BacklogItemDetail`'s page-level `CollapsibleGroup` in production, so this prop's functional effect there is zero — it must still exist per requirement 4 (API consistency + dev-mode warning avoidance) and is the thing that makes `DescriptionSection.test.tsx`'s **standalone** (non-grouped) renders meaningfully testable. |
| **`descriptionExpanded`** | The `useState` value returned by `useSectionExpandState(itemId, "description", <seed>)` at `BacklogItemDetail.tsx:323`. The `<seed>` third argument is the ONLY thing that actually determines Description's real initial open state in the running app. | Changing `false` → `true` here is the one functional line of this change. |
| **`sectionExpandEntries` / `openSectionKeys`** | `sectionExpandEntries` (`BacklogItemDetail.tsx:370-381`) is the array of `[sectionKey, expanded, setExpanded]` tuples for every grouped section, including `["description", descriptionExpanded, setDescriptionExpanded]` (line 374). `openSectionKeys` (line 382) filters it down to just the expanded keys and is passed as `CollapsibleGroup`'s controlled `value` (line 1185). | This is the wiring that makes `descriptionExpanded` reach the actual rendered Radix Accordion state. |
| **`useSectionExpandState` precedence** | Hook (not touched by this change) whose `stored === null` (i.e. no localStorage key yet written for this item+section) is the only path that falls through to the passed default/seed. `setExpanded` → `localStorage.setItem` only ever fires from a real user toggle click, never on mount. | Confirms requirement 2 (stored preference beats the new default) is already satisfied by existing logic — no code change needed in the hook itself. |
| **grouped vs. standalone `CollapsibleSection`** | "Grouped" = rendered as a descendant of `CollapsibleGroup` (production `BacklogItemDetail`); "standalone" = rendered alone, in which case `CollapsibleSection` mounts its own implicit single-item `Accordion.Root` (`Collapsible.tsx:165-176`) and `defaultExpanded` genuinely drives initial state. | `DescriptionSection.test.tsx` renders `<DescriptionSection>` standalone (no wrapping group), so `defaultExpanded` is live there even though it's dead in the real app — this is what makes the unit tests in Story 1.1.2 meaningful. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Description's initial-open state | Existing convention: controlled boolean state lifted to the parent (`BacklogItemDetail`) via `useSectionExpandState`, passed down as a `defaultExpanded` prop that mirrors the group's own controlled value | In-repo convention already used by 8 sibling sections (`NotesSection`, `ReviewingSection`, `SessionsSection`, `VersionControlSection`, `PlanArtifactsSection`, `WorkflowHistorySection`, `ProgressHistorySection`, `LastReviewResultSection`) | (a) Flip only `DescriptionSection`'s internal hardcoded `defaultExpanded={false}` → `true`; (b) flip only `BacklogItemDetail.tsx:323`'s seed value without touching `DescriptionSection`'s prop signature | (a) is dead code — `DescriptionSection` always renders inside the page-level `CollapsibleGroup`, so its internally hardcoded prop has zero effect on real rendered state (confirmed by reading `Collapsible.tsx:137-163`); flipping it alone changes nothing observable. (b) satisfies AC1/AC2 functionally but leaves requirement 4 (explicit prop-threading, matching the 8-sibling pattern) and the stale docstring unmet. Only doing both together satisfies both the functional requirement and the structural one — this is **not** a new pattern, it is applying the existing sibling-section convention to the one holdout. |

No GoF/PoEAA pattern applies here beyond "follow the established convention already used by 8 other components in this file" — flagged N/A for a formal pattern citation.

*(No Migration Plan section — no schema, data, or persisted-format change. `useSectionExpandState`'s localStorage contract is unchanged; existing stored keys keep meaning exactly what they already mean.)*

---

## Observability Plan

Pure client-side UI default with no user-facing state transition worth logging. No new logs, metrics, or alerts. Nothing in this change touches a code path with existing structured logging (`log.InfoLog` etc. are backend-only and untouched here). The only "observability" surface is the existing dev-mode `console.warn` in `Collapsible.tsx:150-156`, which this change must not trigger — verified by Story 1.1.2's task of keeping the threaded `defaultExpanded={descriptionExpanded}` value always in sync with the group's own state (see Domain Glossary).

## Risk Control

- **Feature flag**: none needed. The change is a single boolean default flip plus mechanical prop threading; it is fully and trivially reversible.
- **Rollback procedure**: `git revert` the commit. No table, no persisted format, no proto changed — a revert instantly restores prior behavior with zero cleanup.
- **Staged rollout**: N/A — single-user internal tool, no staged deploy; ship in one PR.

## Unresolved Questions

None. `defaultExpanded`'s unconditional-true seed (vs. `NotesSection`'s conditional-on-non-empty-content pattern) was resolved during research: AC1's literal wording ("shows Description expanded ... zero clicks", no non-empty qualifier) requires unconditional-true, and `DescriptionSection` already renders a graceful "No description." empty state, so an unconditionally-expanded empty state is harmless. This is the smaller diff (one seed-value flip, no new `useEffect` branch) and is explicitly out of scope to second-guess per requirements' Out of Scope section.

## Dependency Visualization

```
Story 1.1.1: Flip the functional default + thread the prop  (AC1, AC2, AC4)
  Task 1.1.1a  BacklogItemDetail.tsx:323  seed false→true          ─┐
  Task 1.1.1b  DescriptionSection.tsx  add required prop            │
               + pass-through + docstring rewrite                   ├─► must land together
  Task 1.1.1c  BacklogItemDetail.tsx:1215  call site passes          │  (1b alone leaves the app
               defaultExpanded={descriptionExpanded}                │   TS-broken; 1a alone leaves
                                                                    ─┘   the prop unthreaded)
        │
        ▼
Story 1.1.2: Update tests to match the new default + prove AC isolation  (AC2, AC3, AC5)
  Task 1.1.2a  DescriptionSection.test.tsx rewrite
  Task 1.1.2b  BacklogItemDetail.markdown.test.tsx — drop expandDescription()
  Task 1.1.2c  tests/e2e/backlog-item-detail-redesign.spec.ts:107-110 rewrite
               + Acceptance-Criteria-isolation assertions
  Task 1.1.2d  BacklogItemDetail.markdown.test.tsx — stored-preference-wins
               regression test (added Phase 4, closes the AC2 coverage gap)
        │
        ▼
Story 1.1.3: Verification gate  (AC2, AC3, AC5, AC6)
  Task 1.1.3a  targeted jest suites + make quick-check
  Task 1.1.3b  make registry-generate — confirm empty diff
  Task 1.1.3c  npx playwright test — execute the rewritten e2e spec
               (added Phase 4, closes the AC3/AC5 unexecuted-verification gap)
```

Story 1.1.1's three tasks are one atomic unit (the app does not compile/behave correctly with only one or two of them applied — see Domain Glossary's `defaultExpanded` entry). Story 1.1.2 depends on 1.1.1 being complete (the tests assert the new post-flip behavior). Story 1.1.3 is the terminal gate and depends on both, and now also executes (not just statically reviews) the e2e spec and the stored-preference regression test.

---

## Phase 1: Description Prominence Default

### Epic 1.1: Description expanded by default, prop-threaded like its siblings, tests updated to match

**Goal**: Make `DescriptionSection` open by default on first view (no stored preference), matching the sibling detail-section pattern structurally, without touching Acceptance Criteria's always-visible unconditional rendering or any other section's default.

#### Story 1.1.1: Flip the functional default + thread `defaultExpanded` through `DescriptionSection`

**As a** user filling out or reviewing a backlog item, **I want** the Description I wrote to be visible the instant I open the item, **so that** I don't have to click to see the one field I actually authored.

**Acceptance Criteria** (maps to requirements.md ACs 1, 2, 4):

- **AC1 — Description expanded by default, zero clicks, no stored preference.**
  *Given* item `5b0e1d57-5244-4847-9b61-029b147f6aab` has never been opened before on this browser (no `backlog-detail-section-5b0e1d57-5244-4847-9b61-029b147f6aab-description` localStorage key exists), *When* `BacklogItemDetail` mounts for that item, *Then* `useSectionExpandState(itemId, "description", true)` initializes `descriptionExpanded` to `true`, `openSectionKeys` includes `"description"`, `CollapsibleGroup`'s controlled `value` includes it, and the rendered `collapsible-header-description` button has `aria-expanded="true"` with `backlog-description-rendered` (or the "No description." empty state) already present in the DOM — no click required.

- **AC2 — Stored per-item localStorage preference still wins over the new default.**
  *Given* localStorage already has `backlog-detail-section-5b0e1d57-5244-4847-9b61-029b147f6aab-description` = `"false"` (the user previously collapsed it), *When* `BacklogItemDetail` mounts for item `5b0e1d57-5244-4847-9b61-029b147f6aab` again, *Then* `useSectionExpandState`'s existing `stored !== null` branch reads `"false"` and `descriptionExpanded` initializes to `false` — the seed default (`true`) is never consulted, and `collapsible-header-description` renders `aria-expanded="false"`.

- **AC4 — `DescriptionSection` accepts `defaultExpanded` as a required prop, matching the 8-sibling pattern; stale docstring removed.**
  *Given* `DescriptionSectionProps` declares `defaultExpanded: boolean` (no `?`, matching `NotesSectionProps`'s shape), *When* `<DescriptionSection item={item} defaultExpanded={descriptionExpanded} />` is rendered at its call site, *Then* the project compiles with no TypeScript error, `CollapsibleSection` receives `defaultExpanded={defaultExpanded}` instead of a hardcoded literal, and the component's docstring no longer references "secondary info, collapsed by default (Story 3.1.3, Task 3.1.3a)".

**Files**: `web-app/src/components/backlog/detail/DescriptionSection.tsx`, `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.1.1a — Flip the seed default at `BacklogItemDetail.tsx:323` (~2 min)

- **File**: `web-app/src/components/backlog/BacklogItemDetail.tsx`
- **Change**: line 323 —
  ```ts
  const [descriptionExpanded, setDescriptionExpanded] = useSectionExpandState(itemId, "description", false);
  ```
  → 
  ```ts
  const [descriptionExpanded, setDescriptionExpanded] = useSectionExpandState(itemId, "description", true);
  ```
- **Acceptance criteria covered**: AC1, AC2 (functional fix — this is the one line that actually changes rendered behavior).
- **Note**: no other line in this file needs to change for the seed flip itself — `sectionExpandEntries` (line 374) and `openSectionKeys` (line 382) already read `descriptionExpanded` by reference and need no edit.

##### Task 1.1.1b — Thread `defaultExpanded` through `DescriptionSectionProps` + rewrite docstring (~4 min)

- **File**: `web-app/src/components/backlog/detail/DescriptionSection.tsx`
- **Change**:
  ```tsx
  export interface DescriptionSectionProps {
    item: BacklogItem;
    defaultExpanded: boolean;
  }

  /**
   * The item's markdown description — the primary field a user actually
   * fills in when creating an item, expanded by default. A stored per-item
   * localStorage preference (see useSectionExpandState) can still collapse
   * it; defaultExpanded here mirrors the parent's controlled expand state,
   * matching the sibling detail-section pattern (NotesSection, etc.).
   */
  export function DescriptionSection({ item, defaultExpanded }: DescriptionSectionProps) {
    return (
      <CollapsibleSection sectionKey="description" title="Description" defaultExpanded={defaultExpanded}>
        ...
  ```
- Delete the old docstring's "secondary info, collapsed by default (Story 3.1.3, Task 3.1.3a)" line entirely — it is now false in both intent and mechanism.
- **Docstring wording (pre-mortem #3)**: say `defaultExpanded` seeds the *initial* `useSectionExpandState`-backed value only — do not say it "mirrors" the parent's state, since per the Domain Glossary it is architecturally dead post-mount inside a `CollapsibleGroup` and "mirrors" implies an ongoing sync that doesn't exist.
- **Acceptance criteria covered**: AC4.

##### Task 1.1.1c — Update the call site at `BacklogItemDetail.tsx:1215` (~2 min)

- **File**: `web-app/src/components/backlog/BacklogItemDetail.tsx`
- **Change**: line 1215 —
  ```tsx
  <DescriptionSection item={item} />
  ```
  →
  ```tsx
  <DescriptionSection item={item} defaultExpanded={descriptionExpanded} />
  ```
  — mirrors the existing call-site pattern immediately below it (`PlanArtifactsSection` at line ~1218: `defaultExpanded={planArtifactsExpanded}`, `SessionsSection` at ~1236, `NotesSection` at ~1249).
- **Acceptance criteria covered**: AC4 (this is what makes the required prop compile and keeps it consistent with the group's real state, so the dev-mode divergence warning in `Collapsible.tsx:150-156` never fires).

---

#### Story 1.1.2: Update tests to match the new default and prove Acceptance Criteria is unaffected

**As a** maintainer, **I want** the test suite to assert the new expanded-by-default behavior (not the old collapsed-by-default one) and to directly prove Acceptance Criteria's rendering is untouched, **so that** this change ships with regression coverage instead of just passing by accident.

**Acceptance Criteria** (maps to requirements.md ACs 2, 3, 5 — AC2's test task was added during Phase 4's cross-artifact consistency review, which found the functional fix in Story 1.1.1 had no dedicated regression test):

- **AC3 — Acceptance Criteria's always-visible rendering/layout is provably unaffected.**
  *Given* `AcCriteriaList` renders unconditionally at `BacklogItemDetail.tsx:1147-1152` as a plain `<div>` entirely outside `CollapsibleGroup`/`CollapsibleGroupContext`, sharing no state or hook with `descriptionExpanded`, *When* an e2e test seeds a fresh item and observes Description transition through its default-expanded, collapsed, and re-expanded states, *Then* the "Acceptance Criteria (…/…)" heading and `AcCriteriaList` remain visible throughout every one of those Description-state changes, with no `collapsible-header-*` testid gating them.

- **AC5 — `tests/e2e/backlog-item-detail-redesign.spec.ts` stops asserting the old collapsed-by-default precondition; exercises collapse/re-expand instead.**
  *Given* the test at lines 96-111 currently asserts `collapsible-header-description` starts at `aria-expanded="false"`, *When* the spec is rewritten, *Then* it instead asserts `aria-expanded="true"` immediately after opening a freshly-seeded item (zero prior localStorage for that item), then collapses Description via a direct click on its header (not `expandSection`, which is a toggle-open-only guard per `BacklogItemDetailPage.ts:44-49` and cannot force a collapse), asserts `aria-expanded="false"`, then calls `detailPage.expandSection("description")` to re-expand it and asserts `aria-expanded="true"` again — exercising the full collapse/re-expand cycle the old test never did.

- **AC2 — Stored per-item localStorage preference still wins over the new default.**
  *Given* `localStorage` has `backlog-detail-section-5b0e1d57-5244-4847-9b61-029b147f6aab-description` = `"false"`, *When* `BacklogItemDetail` mounts for that item, *Then* `useSectionExpandState`'s existing `stored !== null` branch reads `"false"` (the new `true` seed is never consulted) and `collapsible-header-description` renders `aria-expanded="false"` — proven by an executed Jest test, not just by citing the hook's unmodified logic.

**Files**: `web-app/src/components/backlog/detail/DescriptionSection.test.tsx`, `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`, `tests/e2e/backlog-item-detail-redesign.spec.ts`

##### Task 1.1.2a — Rewrite `DescriptionSection.test.tsx` for the required `defaultExpanded` prop (~5 min)

- **File**: `web-app/src/components/backlog/detail/DescriptionSection.test.tsx`
- **Change**:
  - Rename the first test from `DescriptionSection_should_DefaultCollapsed_When_AnyItemRenders` to `DescriptionSection_should_RenderCollapsed_When_defaultExpandedFalse`; pass `defaultExpanded={false}` explicitly; body (asserting `aria-expanded="false"` and no rendered markdown testid) stays unchanged otherwise.
  - Add a new sibling test `DescriptionSection_should_RenderExpanded_When_defaultExpandedTrue`: render `<DescriptionSection item={makeItem({ description: "**bold**" })} defaultExpanded={true} />`, assert `collapsible-header-description` has `aria-expanded="true"` and `backlog-description-rendered` is present in the DOM with **zero** clicks — this is the unit-level proof of requirement 1's behavior.
  - The two remaining tests ("reveals the markdown description once expanded", "shows an empty-state message when there is no description") keep their existing click-then-assert bodies unchanged; just add `defaultExpanded={false}` to their `<DescriptionSection>` renders so they keep starting collapsed (minimal diff, per the research note's stated preference).
- **Acceptance criteria covered**: AC1 (unit-level proof), AC4 (compiles against the new required prop).

##### Task 1.1.2b — Drop the now-inverted `expandDescription()` helper and its call sites (~3 min)

- **File**: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`
- **Change**: Delete the `expandDescription()` helper function (lines 12-21, including its now-stale docstring) and all three `await expandDescription();` call sites (~line 128, ~144, ~158). The section is expanded by default now, so `backlog-description-rendered` is already in the DOM by the time `screen.findByTestId("backlog-description-rendered")` runs — no click needed. `beforeEach`'s `localStorage.clear()` (line 118) already guarantees no stored-preference interference, so no other change is needed in this file.
- **Acceptance criteria covered**: AC6 (this file must keep passing; requirement 6 names it explicitly).

##### Task 1.1.2d — Add a regression test proving a stored localStorage preference still wins over the new default (~4 min)

- **File**: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`
- **Why**: cross-artifact consistency review (Phase 4) found this was the one requirement (AC2 — "must not regress") with zero planned test coverage: `DescriptionSection.test.tsx`'s tests pass `defaultExpanded` explicitly and never exercise `useSectionExpandState`'s stored-value precedence path at all, and this file is the only one in the constrained file list that renders the *real* `BacklogItemDetail` (so it's the only place that can actually seed `localStorage` and observe `useSectionExpandState(itemId, "description", true)` resolve against a stored value).
- **Change**: add a new test, e.g. `it("keeps Description collapsed when a stored per-item preference says so, even though the new default is expanded", ...)`:
  ```ts
  it("keeps Description collapsed when a stored per-item preference says so, even though the new default is expanded", async () => {
    const item = makeItem({ description: "**bold**" });
    localStorage.setItem(`backlog-detail-section-${item.id}-description`, "false");
    render(<BacklogItemDetail itemId={item.id} />);
    // ...await initial load per this file's existing setup pattern...
    const header = await screen.findByTestId("collapsible-header-description");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByTestId("backlog-description-rendered")).not.toBeInTheDocument();
  });
  ```
  Match this file's existing mock/setup conventions (item id, RPC mocks) rather than the pseudocode above verbatim.
- **Load-bearing note (pre-mortem #2)**: add a one-line comment near this test (and near the file's `beforeEach`) stating this suite is the only place that verifies the seed value (`BacklogItemDetail.tsx:323`) and the threaded prop (`DescriptionSection.tsx`) stay in sync end-to-end — so a future editor doesn't assume `DescriptionSection.test.tsx` already covers this and weaken/remove it.
- **Acceptance criteria covered**: AC2.

##### Task 1.1.2c — Rewrite the e2e default-state assertions and add the Acceptance Criteria isolation proof (~5 min)

- **File**: `tests/e2e/backlog-item-detail-redesign.spec.ts`
- **Change** (replacing lines 107-110 inside the `"expands a top-level section from its own default-collapsed state"` test — rename it to `"Description defaults expanded; collapsing and re-expanding it never affects Acceptance Criteria"`):
  ```ts
  // Description now defaults expanded (backlog-description-prominence) —
  // Acceptance Criteria stays visible throughout every state change below,
  // proving it shares no state with Description's collapse toggle.
  const acHeading = page.getByRole("heading", { name: /Acceptance Criteria/ });
  await expect(acHeading).toBeVisible();
  await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");

  // expandSection() only opens (BacklogItemDetailPage.ts:44-49 guards on
  // aria-expanded !== "true"), so force the collapse with a direct click.
  await detailPage.sectionHeader("description").click();
  await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "false");
  await expect(acHeading).toBeVisible();

  await detailPage.expandSection("description");
  await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");
  await expect(acHeading).toBeVisible();
  ```
- **Revised during Phase 6 verify**: a raw `.click()` on `detailPage.sectionHeader("description")` was tried first to keep `BacklogItemDetailPage.ts` untouched, but this violates `.claude/rules/e2e-test-conventions.md`'s hard convention #4 ("New page helpers go in `tests/e2e/pages/` — don't inline repeated navigation logic in spec files") and required a comment anchored to `BacklogItemDetailPage.ts:44-49`'s line numbers, which goes stale the moment that file changes for an unrelated reason. Added a symmetric `collapseSection(sectionKey)` method next to `expandSection` in `BacklogItemDetailPage.ts` instead (same guard, inverted) and call `detailPage.collapseSection("description")` from the spec.
- **Acceptance criteria covered**: AC3, AC5.

---

#### Story 1.1.3: Verification gate

**As a** maintainer, **I want** the full required check suite green before this ships, **so that** the default flip and prop-threading are proven correct end to end.

**Acceptance Criteria** (maps to requirements.md AC6; also the executed-proof gate for AC2/AC3/AC5 via Tasks 1.1.2d and 1.1.3c):

- **AC6 — No new deps, no CSS changes, no registry entries; `make quick-check` and the three named Jest suites pass.**
  *Given* the four production/test tasks in Story 1.1.1-1.1.2 (1.1.1a-c, 1.1.2a-d) are complete, *When* `make quick-check` runs (build + test + lint) alongside `cd web-app && npx jest --no-coverage --testPathPatterns="DescriptionSection.test|BacklogItemDetail.markdown.test|BacklogItemDetail.test"` and `cd tests/e2e && npx playwright test backlog-item-detail-redesign.spec.ts`, *Then* all pass with zero failures, and `make registry-generate` produces an empty `git diff` on `docs/registry/` (no new RPC/UI feature was added — this is an existing-component behavior change).

**Files**: none (verification-only story; no source files change).

##### Task 1.1.3a — Run targeted Jest suites + `make quick-check` (~4 min)

- **Commands**:
  ```bash
  cd web-app && npx jest --no-coverage --testPathPatterns="DescriptionSection.test|BacklogItemDetail.markdown.test|BacklogItemDetail\.test"
  make quick-check
  ```
- **Acceptance criteria covered**: AC6.
- **Note on `make quick-check`'s frontend coverage (adversarial review concern #2)**: `quick-check` (`build test-coverage test-race lint lint-css-tokens registry-diff`) is Go-only — it validates the frontend only incidentally, via `next build`'s TypeScript compile step (which does catch the required-`defaultExpanded`-prop wiring error). The actual frontend test coverage for this change comes from the explicit `npx jest` command above, run separately.
- **Roving-tabindex test (pre-mortem #5, resolving adversarial review concern #3)**: `BacklogItemDetail.test.tsx`'s keyboard roving-tabindex test (~lines 995-1023) has already been independently verified twice — once in research (`pitfalls.md` §5) and once in the Phase 3 adversarial review — to assert only `.focus()`/`.toHaveFocus()`, with zero coupling to expand/collapse state. Treat this run as confirmation, not discovery; a failure here would be surprising and should be escalated/root-caused rather than assumed unrelated, but it is not an open question going into this task.
- **Scope note (consistency review concern #1)**: `BacklogItemDetail.test.tsx` is outside requirements.md's file-confinement list but is explicitly named in AC6 as a suite that must keep passing. If (contrary to the above) a genuine fix to that file is needed, that is a deliberate, justified exception to the file list — not scope creep — since AC6 itself requires this suite to pass.

##### Task 1.1.3b — `make registry-generate`, confirm empty diff (~2 min)

- **Command**: `make registry-generate && git status --porcelain docs/registry/`
- **Acceptance criteria covered**: AC6 (no registry entries expected — this is a behavior change to an existing component, not a new RPC or UI feature marker).

##### Task 1.1.3c — Run the rewritten e2e spec against a live isolated instance (~10 min)

- **Why (pre-mortem #1, top failure mode; adversarial review concern #1)**: `tests/e2e/backlog-item-detail-redesign.spec.ts` is the only test proving AC3 (Acceptance Criteria isolation) and AC5 (new collapse/re-expand behavior) in a real browser, but it is executed by neither Task 1.1.3a's gate nor any CI workflow (`e2e-video.yml`'s `FEATURE_SPECS` excludes this file; confirmed by grepping all `.github/workflows/*.yml` for `playwright test`). Without this task, AC3 and AC5 ship with zero executed verification.
- **Commands** (per `CLAUDE.md`'s E2E Tests section — `global-setup.ts` auto-manages an isolated instance, no manual server needed):
  ```bash
  cd tests/e2e && npx playwright test backlog-item-detail-redesign.spec.ts
  ```
- If this environment genuinely cannot run a live browser/isolated instance, explicitly note that as an accepted, unexecuted risk in the request-review summary rather than silently skipping it.
- **Acceptance criteria covered**: AC3, AC5 (executed proof, not just static review of the rewritten assertions).
