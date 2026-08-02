# Implementation Plan: backlog-description-prominence

**Feature**: Flip Description's default-expanded state in `BacklogItemDetail` so it opens by default for any item/user with no stored per-item preference, matching Acceptance Criteria's always-visible prominence and the `pull-request`/`sessions` sections' existing `true`-default pattern.
**Date**: 2026-08-01
**Status**: Ready for implementation
**ADRs**: None — see "Why no ADR" note at the end of this document.

---

## Step 0.5 — Alternatives Considered

**Approach A — Flip only the driving line.**
Change `useSectionExpandState(itemId, "description", false)` → `..., true)` at
`BacklogItemDetail.tsx:323` and touch nothing else.
- *Strength*: smallest possible diff; functionally sufficient on its own — the group's
  controlled `value` is what actually drives initial open state in grouped mode
  (`Collapsible.tsx:139-157`).
- *Weakness*: leaves `DescriptionSection.tsx`'s own hardcoded `defaultExpanded={false}`
  (and its stale "collapsed by default" docstring) permanently disagreeing with the new
  reality — dead code that misleads the next reader, and the one call site in the whole
  `detail/` family that doesn't thread `defaultExpanded` as a prop like every sibling
  (`PlanArtifactsSection`, `NotesSection`, `VersionControlSection`, etc.).

**Approach B — Flip + refactor `DescriptionSection` to accept `defaultExpanded: boolean` as a prop.**
Same line-323 flip, plus give `DescriptionSection` the same `defaultExpanded` prop
signature every other section in `detail/` already has, threaded from
`BacklogItemDetail.tsx`'s existing `descriptionExpanded` state.
- *Strength*: removes the only inconsistent section in the family, keeps
  `DescriptionSection.tsx` self-documenting, and fixes the stale docstring — one extra
  file touched, still a same-day change.
- *Weakness*: marginally larger diff than Approach A (two files instead of one) for a
  benefit that's purely internal-consistency, not user-visible.

**Approach C — Make `defaultExpanded` conditional on `item.description` being non-empty.**
- *Strength*: could avoid an expanded-but-empty "No description." box for items with no
  description yet.
- *Weakness*: explicitly rejected by `requirements.md` and `research/ux.md` — adds
  complexity and flicker risk (the `item` object isn't available on first render; see the
  existing `initialExpandAppliedRef` status-dependent-default machinery this would have to
  duplicate) for no benefit, and breaks parity with Acceptance Criteria's own
  always-expanded empty-state convention ("No acceptance criteria defined.").

**Chosen: Approach B.** All four research docs (`build-vs-buy.md`, `stack.md`,
`pitfalls.md`, `ux.md`) converge on it, it costs one extra small file, and it removes a
piece of dead/misleading code rather than leaving it behind.

---

## Domain Glossary

No new domain terms — this plan reuses vocabulary already established in the codebase.

| Term | Definition | Notes |
|------|-----------|-------|
| `defaultExpanded` | Boolean prop on `CollapsibleSection`/section components controlling initial open state when no stored preference exists | Defined in `web-app/src/components/ui/Collapsible.tsx`; this plan flips its value for the `description` section, not its meaning |
| `useSectionExpandState` | Hook returning `[expanded, setExpanded]`, seeded from localStorage key `backlog-detail-section-${itemId}-${sectionKey}` or the passed `defaultExpanded` if no stored value exists | `web-app/src/lib/hooks/useSectionExpandState.ts:8-13` — untouched by this plan, only its call-site argument changes |
| `openSectionKeys` / `sectionExpandEntries` | Parent-level array threading each section's expand state into the shared `CollapsibleGroup`'s controlled `value` | `BacklogItemDetail.tsx` — already includes `"description"`; no structural change needed |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Effective default value for the `description` section | Flip `useSectionExpandState(itemId, "description", false)` → `true` at `BacklogItemDetail.tsx:323` | requirements.md, build-vs-buy.md, stack.md | Approach C: content-conditional default (`true` only when `item.description` is non-empty) | requirements.md explicitly forbids making `defaultExpanded` conditional on content presence — adds flicker risk (item loads asynchronously) and breaks parity with Acceptance Criteria's existing always-visible empty state |
| `DescriptionSection` prop API | Convert to a controlled `defaultExpanded: boolean` prop threaded from the parent's `descriptionExpanded` state, matching `PlanArtifactsSection`/`NotesSection`/`VersionControlSection`/etc. | Existing sibling-section convention in `web-app/src/components/backlog/detail/*.tsx` | Approach A: leave `defaultExpanded={false}` hardcoded inside `DescriptionSection.tsx` | Hardcoded value is dead code in grouped mode (`Collapsible.tsx:139-157`) and would permanently disagree with the parent's new `true` default — misleading to future readers, and the sole outlier among ~8 sibling sections |
| Regression test placement for new aria-expanded coverage | Add to `BacklogItemDetail.markdown.test.tsx` (existing `describe("BacklogItemDetail — description markdown rendering", ...)` block gets a sibling `describe`) | Existing file already has a minimal, description-scoped mock harness (`getBacklogItem`, `baseItem`, `localStorage.clear()` in `beforeEach`) | Add to `BacklogItemDetail.test.tsx` instead | That file's mock harness is built around session/pipeline-mode scenarios unrelated to description state; reusing the markdown test file's already-correct minimal harness avoids duplicating setup for a two-test addition |
| `expandDescription()` helper in `BacklogItemDetail.markdown.test.tsx` | Guard the click behind an `aria-expanded` check so it only clicks when currently collapsed (idempotent-open helper) | pitfalls.md #3 | Delete the helper and its 3 call sites entirely | A guarded helper keeps the three markdown-rendering tests resilient to either default value and self-documents intent, versus silent reliance on the current default being `true` |

---

## Observability Plan
- **Logs**: None — pure client-side default UI state, no server round-trip.
- **Metrics**: None — no existing analytics event fires on section expand/collapse for `description` today, and this change doesn't add one (out of scope).
- **Alerts**: None applicable.

## Risk Control
- **Feature flag**: None — appetite is XS, blast radius is a single default-prop value on one section of one detail view; a flag would be disproportionate ceremony.
- **Rollback procedure**: Revert the single commit (two boolean literals + one prop threading change); no data migration, no persisted-state cleanup needed since `useSectionExpandState`'s stored preferences are keyed independently of this default and are untouched either direction.
- **Staged rollout**: None — ships directly via normal PR/CI; `make quick-check` (build + test + lint) and the frontend Jest suite (`cd web-app && npx jest --no-coverage --testPathPatterns="DescriptionSection|BacklogItemDetail"`) are the gate.

## Unresolved Questions
None.

## Dependency Visualization

```
Task 1.1.1a (flip driving default, BacklogItemDetail.tsx:323)
        │
        ▼
Task 1.2.1a (DescriptionSection.tsx: add defaultExpanded prop + docstring fix)
        │
        ▼
Task 1.2.1b (BacklogItemDetail.tsx:1215 call site: pass defaultExpanded={descriptionExpanded})
        │
        ├────────────────────────────┬─────────────────────────────┐
        ▼                            ▼                             ▼
Task 1.3.1a                  Task 1.3.2a                   Task 1.4.1a / 1.4.2a
(DescriptionSection.test.tsx  (markdown test's             (new regression tests in
 rename + assertions)          expandDescription() guard)   BacklogItemDetail.markdown.test.tsx)
```

Tasks 1.3.1a, 1.3.2a, and 1.4.1a/1.4.2a have no dependency on each other and can run in
parallel once 1.2.1b lands; they're listed sequentially below purely for narrative order.

---

## Phase 1: Description Prominence Fix

### Epic 1.1: Flip the Effective Default
**Goal**: Make the `CollapsibleGroup`'s controlled initial-open state include
`"description"` for any item with no stored per-item preference — this is the line that
actually matters per `Collapsible.tsx:139-157` (grouped mode ignores a child section's own
`defaultExpanded`).

#### Story 1.1.1: New/never-toggled items show Description expanded on first view
**As a** solo user triaging backlog items, **I want** the Description section open the
first time I view any item, **so that** I can read the field I filled in without an extra
click.
**Acceptance Criteria**:
- Opening a backlog item's detail view with no stored `description` preference for that
  item shows the description content immediately.
  - *Given* `localStorage` has no key `backlog-detail-section-item-1-description`, *When*
    `BacklogItemDetail` renders for `itemId="item-1"`, *Then* the element with
    `data-testid="collapsible-header-description"` has `aria-expanded="true"` and the
    element with `data-testid="backlog-description-rendered"` is present in the DOM
    without any click.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.1.1a: Flip the driving default (~2 min)
- In `web-app/src/components/backlog/BacklogItemDetail.tsx:323`, change:
  `const [descriptionExpanded, setDescriptionExpanded] = useSectionExpandState(itemId, "description", false);`
  to:
  `const [descriptionExpanded, setDescriptionExpanded] = useSectionExpandState(itemId, "description", true);`
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

### Epic 1.2: Keep `DescriptionSection` Internally Consistent
**Goal**: Eliminate the one section component in `detail/` that hardcodes
`defaultExpanded` instead of accepting it as a prop, so the component's own code and
docstring don't contradict the new default (pitfalls.md #1).

#### Story 1.2.1: `DescriptionSection` accepts `defaultExpanded` like every sibling section
**As a** future maintainer reading `DescriptionSection.tsx`, **I want** its
`defaultExpanded` behavior driven by the same prop-threading pattern as
`PlanArtifactsSection`/`NotesSection`/etc., **so that** the component's own code doesn't
silently disagree with how it's actually used.
**Acceptance Criteria**:
- `DescriptionSection` rendered standalone (no `CollapsibleGroup` ancestor, as in its own
  unit test) honors a `defaultExpanded` prop rather than a hardcoded value.
  - *Given* `<DescriptionSection item={makeItem()} defaultExpanded={true} />` is rendered
    with no wrapping `CollapsibleGroup`, *When* the component mounts, *Then*
    `data-testid="collapsible-header-description"` has `aria-expanded="true"` with no
    click required.
- The stale docstring ("collapsed by default (Story 3.1.3, Task 3.1.3a)") no longer
  contradicts the code.
  - *Given* `web-app/src/components/backlog/detail/DescriptionSection.tsx`'s file-level
    doc comment, *When* read alongside the component's actual `defaultExpanded` prop
    usage, *Then* the comment accurately describes the current behavior (parent-controlled
    default, not a hardcoded collapse).
**Files**: `web-app/src/components/backlog/detail/DescriptionSection.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.2.1a: Add `defaultExpanded` prop to `DescriptionSection` (~3 min)
- In `web-app/src/components/backlog/detail/DescriptionSection.tsx`:
  - Add `defaultExpanded: boolean;` to `DescriptionSectionProps`.
  - Change the function signature to `export function DescriptionSection({ item, defaultExpanded }: DescriptionSectionProps)`.
  - Change `<CollapsibleSection sectionKey="description" title="Description" defaultExpanded={false}>`
    to `<CollapsibleSection sectionKey="description" title="Description" defaultExpanded={defaultExpanded}>`.
  - Replace the doc comment `/** The item's markdown description — secondary info, collapsed by default (Story 3.1.3, Task 3.1.3a). */`
    with `/** The item's markdown description. Expanded by default (Story: backlog-description-prominence) unless the caller passes false or the user has a stored per-item preference — see useSectionExpandState. */`.
- Files: `web-app/src/components/backlog/detail/DescriptionSection.tsx`

##### Task 1.2.1b: Thread `descriptionExpanded` into the call site (~2 min)
- In `web-app/src/components/backlog/BacklogItemDetail.tsx:1215`, change:
  `<DescriptionSection item={item} />`
  to:
  `<DescriptionSection item={item} defaultExpanded={descriptionExpanded} />`
  (Note: this prop is a no-op inside the `CollapsibleGroup` per `Collapsible.tsx:139-157`'s
  own dev-mode comment — the group's `value` prop is what actually drives display — but it
  keeps the divergence-warning check silent and the component self-consistent, per Pattern
  Decisions above.)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

### Epic 1.3: Update Existing Tests for the New Default
**Goal**: Bring the two test files research identified as coupled to the old `false`
default in line with the new `true` default, without weakening their coverage.

#### Story 1.3.1: `DescriptionSection.test.tsx` reflects the new default and prop
**As a** developer running the unit suite, **I want**
`DescriptionSection.test.tsx` to pass against the refactored prop-driven component,
**so that** the suite documents the current (expanded-by-default) behavior instead of the
old one.
**Acceptance Criteria**:
- The renamed test asserts the new default.
  - *Given* `<DescriptionSection item={makeItem()} defaultExpanded={true} />` is rendered
    standalone, *When* the component mounts with no interaction, *Then*
    `screen.getByTestId("collapsible-header-description")` has `aria-expanded="true"` and
    `screen.getByTestId("backlog-description-rendered")` is present in the DOM.
- The two other existing tests in the file (markdown reveal, empty-state message) no
  longer need their `fireEvent.click(...)` calls, since content is already visible.
**Files**: `web-app/src/components/backlog/detail/DescriptionSection.test.tsx`

##### Task 1.3.1a: Rewrite `DescriptionSection.test.tsx` for the new default (~5 min)
- In `web-app/src/components/backlog/detail/DescriptionSection.test.tsx`:
  - Rename `it("DescriptionSection_should_DefaultCollapsed_When_AnyItemRenders", ...)` to
    `it("DescriptionSection_should_DefaultExpanded_When_AnyItemRenders", ...)`.
  - Change its render call to `render(<DescriptionSection item={makeItem()} defaultExpanded={true} />);`.
  - Change its assertions to:
    `expect(header).toHaveAttribute("aria-expanded", "true");` and
    `expect(screen.getByTestId("backlog-description-rendered")).toBeInTheDocument();`
    (drop the `queryByTestId(...).not.toBeInTheDocument()` assertion — content is now
    present by default).
  - Update the second test (`"reveals the markdown description once expanded"`) to pass
    `defaultExpanded={true}` in its render call and drop the now-unnecessary
    `fireEvent.click(...)` line (content is already visible; rename the test to
    `"renders the markdown description by default"` for accuracy).
  - Update the third test (`"shows an empty-state message when there is no description"`)
    the same way: pass `defaultExpanded={true}`, drop the `fireEvent.click(...)` line.
- Files: `web-app/src/components/backlog/detail/DescriptionSection.test.tsx`

#### Story 1.3.2: Markdown test's `expandDescription()` helper stays correct under the new default
**As a** developer running `BacklogItemDetail.markdown.test.tsx`, **I want** the
`expandDescription()` helper to never collapse an already-open section, **so that** the
three markdown-rendering tests (bold/link, image, script-injection safety) keep passing
without depending on which default is currently in effect.
**Acceptance Criteria**:
- Calling `expandDescription()` against a fresh `item-1` render (no stored preference,
  default now `true`) leaves the description expanded rather than toggling it closed.
  - *Given* `BacklogItemDetail` has rendered for `itemId="item-1"` with no
    `backlog-detail-section-item-1-description` localStorage key set, *When*
    `expandDescription()` runs (finds the header, checks `aria-expanded`, and only clicks
    if `"false"`), *Then* `data-testid="collapsible-header-description"` still has
    `aria-expanded="true"` and `data-testid="backlog-description-rendered"` remains in the
    DOM.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

##### Task 1.3.2a: Guard `expandDescription()` against an already-expanded header (~3 min)
- In `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`, replace:
  ```ts
  async function expandDescription() {
    const header = await screen.findByTestId("collapsible-header-description");
    fireEvent.click(header);
  }
  ```
  with:
  ```ts
  async function expandDescription() {
    const header = await screen.findByTestId("collapsible-header-description");
    if (header.getAttribute("aria-expanded") === "false") {
      fireEvent.click(header);
    }
  }
  ```
- Update the doc comment above it (currently: "DescriptionSection (Story 3.1.3) is
  collapsed by default ... Expand it before asserting on rendered description content.")
  to: "DescriptionSection now defaults to expanded, but this helper stays idempotent —
  it only clicks the header if it finds it currently collapsed — so these tests don't
  depend on which default is in effect."
- Files: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

---

### Epic 1.4: Add Regression Coverage for Both Default Paths
**Goal**: Directly assert the two behaviors requirements.md calls out as the success
metrics — fresh items expand automatically, and a user's prior manual collapse choice for
a specific item is still honored.

#### Story 1.4.1: Fresh item (no stored preference) renders Description expanded
**As a** developer verifying the fix, **I want** an explicit test asserting
`aria-expanded="true"` for an item with no localStorage entry, **so that** the core
acceptance criterion has direct, named coverage (not just incidental coverage via other
tests).
**Acceptance Criteria**:
- A dedicated test seeds no localStorage entry and asserts the expanded state.
  - *Given* `localStorage` is cleared (no `backlog-detail-section-item-1-description`
    key) and `getBacklogItem` resolves `{ ...baseItem, description: "Some description" }`
    for `itemId="item-1"`, *When* `<BacklogItemDetail itemId="item-1" />` renders and
    settles, *Then* `screen.findByTestId("collapsible-header-description")` resolves to
    an element with `aria-expanded="true"`.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

##### Task 1.4.1a: Add fresh-item expanded-by-default test (~4 min)
- In `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`, add a new
  `describe` block after the existing one:
  ```ts
  describe("BacklogItemDetail — description prominence (default expand state)", () => {
    it("expands Description by default when no stored preference exists for the item", async () => {
      getBacklogItem.mockResolvedValue({ ...baseItem, description: "Some description" });

      render(<BacklogItemDetail itemId="item-1" />);

      const header = await screen.findByTestId("collapsible-header-description");
      expect(header).toHaveAttribute("aria-expanded", "true");
      expect(screen.getByTestId("backlog-description-rendered")).toBeInTheDocument();
    });
  });
  ```
- Files: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

#### Story 1.4.2: Item with a stored `"false"` preference stays collapsed (non-regression)
**As a** solo user who has previously collapsed Description on a specific item, **I want**
that choice to persist across visits, **so that** the default-flip doesn't silently
override my manual preference.
**Acceptance Criteria**:
- A test seeds `localStorage` with an explicit `"false"` value for `item-1`'s description
  key and asserts it remains collapsed after render.
  - *Given* `localStorage.setItem("backlog-detail-section-item-1-description", "false")`
    was called before render, and `getBacklogItem` resolves `{ ...baseItem, description: "Some description" }`
    for `itemId="item-1"`, *When* `<BacklogItemDetail itemId="item-1" />` renders and
    settles, *Then* `screen.findByTestId("collapsible-header-description")` resolves to
    an element with `aria-expanded="false"` and
    `screen.queryByTestId("backlog-description-rendered")` is not in the DOM.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

##### Task 1.4.2a: Add stored-preference-preserved regression test (~4 min)
- In the same new `describe` block added in Task 1.4.1a, add a second test:
  ```ts
  it("keeps Description collapsed when the item has a stored false preference", async () => {
    localStorage.setItem("backlog-detail-section-item-1-description", "false");
    getBacklogItem.mockResolvedValue({ ...baseItem, description: "Some description" });

    render(<BacklogItemDetail itemId="item-1" />);

    const header = await screen.findByTestId("collapsible-header-description");
    expect(header).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByTestId("backlog-description-rendered")).not.toBeInTheDocument();
  });
  ```
- Files: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

#### Story 1.4.3: Acceptance Criteria's always-visible rendering is unaffected (non-regression)
**As a** developer verifying the fix, **I want** an explicit assertion that the
Acceptance Criteria section is still rendered/visible the same way after this change, **so
that** requirements.md's Success Metric 3 ("Acceptance Criteria's current always-visible
behavior is preserved") has direct test coverage instead of being an unverified assumption.
**Acceptance Criteria**:
- The fresh-item test added in Task 1.4.1a also confirms `AcCriteriaList` still renders
  unconditionally, unaffected by the Description default flip.
  - *Given* the same render as Task 1.4.1a (`itemId="item-1"`, no stored `description`
    preference), *When* `<BacklogItemDetail itemId="item-1" />` renders and settles,
    *Then* the Acceptance Criteria section's content (e.g. `screen.getByText(/acceptance criteria/i)`
    or its existing `data-testid`, whichever `AcCriteriaList`/`BacklogItemDetail.test.tsx`
    already uses — confirm the exact selector during implementation) is present in the DOM,
    exactly as it is today, with no collapse/expand affordance introduced.
**Files**: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

##### Task 1.4.3a: Add Acceptance-Criteria-unaffected assertion (~2 min)
- Extend the test added in Task 1.4.1a (`"expands Description by default when no stored
  preference exists for the item"`) with one additional assertion confirming the
  Acceptance Criteria section is present/visible, using whichever existing selector
  `AcCriteriaList` already exposes (grep `BacklogItemDetail.test.tsx` for its current
  Acceptance Criteria assertion pattern and reuse it verbatim rather than inventing a new
  selector).
- Files: `web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

---

### Epic 1.5: Update the e2e Test That Hardcodes the Old Default
**Goal**: `tests/e2e/backlog-item-detail-redesign.spec.ts` was missed by the original
research/planning pass (it lives outside `web-app/src`, which is what earlier research
grepped) and contains a test that hardcodes the *old* collapsed-by-default precondition for
Description. Without this epic, that spec goes red the moment this change ships. Found
during the pre-mortem pass (P1 finding #1) — added here per the SDD readiness-gate
requirement to patch plan.md with the prevention before implementation starts.

#### Story 1.5.1: `backlog-item-detail-redesign.spec.ts` reflects Description's new default
**As a** developer running the e2e suite, **I want** the existing "expands a top-level
section" test to match the new expanded-by-default behavior, **so that** the e2e suite
doesn't fail on a stale assumption the moment this ships.
**Acceptance Criteria**:
- The test no longer asserts `aria-expanded="false"` as Description's initial state, and
  instead exercises the *collapse* interaction (the one still meaningfully testable after
  the flip).
  - *Given* a freshly seeded headless-triage item with no stored `description` preference,
    *When* the detail page opens, *Then* `detailPage.sectionHeader("description")` has
    `aria-expanded="true"` immediately (no click); *When* the user then clicks to collapse
    it, *Then* `aria-expanded` becomes `"false"`; *When* the user clicks again, *Then* it
    returns to `"true"`.
**Files**: `tests/e2e/backlog-item-detail-redesign.spec.ts`

##### Task 1.5.1a: Rewrite the stale e2e test (~4 min)
- In `tests/e2e/backlog-item-detail-redesign.spec.ts`, replace the test
  `"expands a top-level section from its own default-collapsed state"` (lines 96–111) and
  its stale comment (`// DescriptionSection defaults collapsed for every item (Story
  3.1.3).`) with a version asserting the new default and exercising the collapse→re-expand
  interaction instead, e.g.:
  ```ts
  test("Description defaults expanded, and can be collapsed and re-expanded", async ({ page, request }) => {
    const title = `e2e headless-triage section-collapse ${Date.now()}`;
    await seedHeadlessTriageItem(request, { title, status: "review" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    // DescriptionSection defaults expanded for every item with no stored
    // per-item preference (backlog-description-prominence).
    await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");
    await detailPage.sectionHeader("description").click();
    await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "false");
    await detailPage.sectionHeader("description").click();
    await expect(detailPage.sectionHeader("description")).toHaveAttribute("aria-expanded", "true");
  });
  ```
  Rename the test description accordingly. If `detailPage.expandSection("description")`
  (used by the old test) is a click-only helper with no collapse counterpart, either reuse
  `detailPage.sectionHeader("description").click()` directly (as above) or add a symmetric
  `collapseSection`/toggle helper to `./pages/BacklogItemDetailPage.ts` — check that file
  during implementation before duplicating logic.
- Files: `tests/e2e/backlog-item-detail-redesign.spec.ts`,
  possibly `tests/e2e/pages/BacklogItemDetailPage.ts` (only if a collapse helper doesn't
  already exist)

---

## Verification

- `cd web-app && npx jest --no-coverage --testPathPatterns="DescriptionSection|BacklogItemDetail.markdown|BacklogItemDetail.test"` — targeted run of every file this plan touches.
- `cd tests/e2e && npx tsc --noEmit -p .` then run `backlog-item-detail-redesign.spec.ts` against a local `e2e-local` instance (per that file's own header prerequisites) — confirms Epic 1.5's rewrite passes; this spec was previously unexecuted/unverified even before this plan (see its own header note), so this is the first real run, not just a regression check.
- `make quick-check` — full build + test + lint gate before shipping, per repo convention.
- No proto changes (`make proto-gen` not needed), no CSS changes (nothing to check against `.claude/rules/css-architecture.md`), no `docs/registry/features/` entry exists for `DescriptionSection` or a `description` feature marker — confirmed via `grep -rn "backlog-description\|DescriptionSection" docs/registry/` returning no matches, so no registry update is required.
- Acceptance Criteria's rendering is covered directly by Task 1.4.3a (Success Metric 3), not just assumed unaffected.
- UX AC #7 (no flash on initial mount) needs no new test: `useSectionExpandState` reads `localStorage` synchronously before first paint, so there is no async gap that could produce a "closed then snaps open" flash — inherent to the existing hook, unchanged by this plan.
- UX AC #8 (keyboard reachability/toggle) needs no new test: Radix Accordion's roving-tabindex keyboard handling (`Collapsible.tsx:6-10`, ADR-027) is unmodified by a `value`-array default change — the trigger's Tab/Enter/Space behavior is identical regardless of initial open state.

## Why no ADR

This is a default-prop-value flip plus a prop-threading refactor that brings one outlier
component in line with ~8 existing siblings using an already-established pattern
(`defaultExpanded: boolean` threaded from `useSectionExpandState`). It sets no new
precedent — it conforms to one that already exists everywhere else in
`web-app/src/components/backlog/detail/`. Per the task's own guidance to "err toward not
writing one for a change this small," no ADR is produced.
