# Research: pitfalls of flipping Description's default-expanded flag

Scope: verify the specific landmines called out in the research question, for the proposed
one-line change `useSectionExpandState(itemId, "description", false)` → `true`
(`BacklogItemDetail.tsx:323`).

## 1. `defaultExpandedDiverges` dev-warning in `Collapsible.tsx`

**Confirmed real, but with a twist: the warning will NOT fire, which is itself the landmine.**

- `DescriptionSection.tsx:20` hardcodes `defaultExpanded={false}` directly on
  `CollapsibleSection`, unlike every other sibling section (`ReviewingSection`,
  `PullRequestSection` — hardcodes `true` — `PlanArtifactsSection`, `VersionControlSection`,
  `SessionsSection`, `WorkflowHistorySection`, `ProgressHistorySection`, `NotesSection`,
  `LastReviewResultSection`), which all declare a `defaultExpanded: boolean` prop and thread
  the parent's `useSectionExpandState`-derived value straight through. `DescriptionSection`
  breaks this pattern — it takes no `defaultExpanded` prop at all.
- `Collapsible.tsx:150-156`'s guard is **asymmetric**:
  ```ts
  const groupSaysOpen = openKeys?.includes(sectionKey) ?? false;
  const defaultExpandedDiverges = defaultExpanded && !groupSaysOpen;
  ```
  It only warns when the child claims `true` while the group says `false`. It does **not**
  warn the other direction (child claims `false`, group says `true`).
- After the flip, for an item with no stored preference: parent `descriptionExpanded`
  initializes `true` → `openSectionKeys` includes `"description"` → `groupSaysOpen = true`.
  `DescriptionSection`'s hardcoded `defaultExpanded={false}` → `defaultExpandedDiverges =
  false && !true = false`. **No console.warn fires**, even though the child's own prop now
  permanently disagrees with reality.
- Practical effect: rendering is *not* broken — `CollapsibleItem` never reads `defaultExpanded`
  in grouped mode; the group's controlled `value`/`onValueChange` (backed by `openSectionKeys`)
  is the sole source of truth for actual open/closed state (confirmed by reading
  `CollapsibleGroup`/`CollapsibleSection` in full — `defaultExpanded` truly is dead code on the
  grouped-render path, only used for the warning check and for `DescriptionSection`'s own
  *standalone* unit tests, see §3). So the literal one-line fix in `BacklogItemDetail.tsx`
  works correctly on its own without touching `DescriptionSection.tsx`.
- What's left behind is stale/misleading code, silently: `DescriptionSection.tsx`'s hardcoded
  `defaultExpanded={false}` and its docstring ("collapsed by default (Story 3.1.3, Task
  3.1.3a)") will no longer match production behavior, and unlike every sibling section it
  won't be threaded from parent state — a future reader has no compile-time or lint-time signal
  that this file is out of sync, and the dev-mode warning that exists for exactly this class of
  bug has a blind spot for this specific direction of divergence.
- **Recommendation**: as part of this fix, convert `DescriptionSection` to accept a
  `defaultExpanded: boolean` prop threaded from `BacklogItemDetail.tsx`'s `descriptionExpanded`
  state (matching every sibling section), rather than leaving the hardcoded `false`. This is a
  few extra lines but closes the inconsistency and keeps the file's behavior legible; it is not
  strictly required for the feature to function due to the dead-code/blind-spot combination
  above, but leaving it as-is is a foot-gun for the next person who edits this file expecting
  `defaultExpanded` to do something.

## 2. Mobile viewport implications

- One more section (~a markdown-rendered description, usually a few sentences to a paragraph)
  starts expanded, pushing Acceptance Criteria and the rest of the `CollapsibleGroup` sections
  further down. On mobile this means more scrolling before reaching Reviewing/PR/Sessions
  sections, but per the requirements doc `AcCriteriaList` already renders above the
  `CollapsibleGroup` unconditionally, and Description is explicitly the "field that's already
  there" vs. Acceptance Criteria's frequently-empty content — the requirements doc treats this
  added scroll length as the intended tradeoff, not a defect. No CSS/layout change needed;
  `CollapsibleSection`'s content is simply un-hidden, using existing styles.
- No distinct mobile-only regression identified beyond ordinary "one more open accordion panel
  adds scroll length," which the requirements doc already accepts.

## 3. Test flakiness / snapshot brittleness

- `DescriptionSection.test.tsx` renders `<DescriptionSection item={...} />` **standalone**
  (no `CollapsibleGroup` wrapper), so `insideGroup` is `false` and the component's own
  hardcoded `defaultExpanded={false}` drives the *real* `Accordion.Root` in that test (the only
  place this prop is not dead code). This test suite is unaffected by the
  `BacklogItemDetail.tsx` flip since it never goes through `useSectionExpandState`/the group —
  it will keep asserting "collapsed by default" for the standalone-render path, which will
  become inconsistent with the grouped/production default once the flip ships (see §1
  recommendation — if `DescriptionSection` is refactored to accept a `defaultExpanded` prop,
  this test file also needs a prop update, e.g. default it to `true` in the test or pass it
  explicitly per case).
- `BacklogItemDetail.test.tsx` has no assertion pinning
  `collapsible-header-description`'s `aria-expanded` value (checked directly — only
  `version-control` and `reviewing` headers are asserted this way, at lines 1053/1076/1078/1099
  and 1135). So the flip itself will not break any existing integration-test assertion. Line
  1014's `descriptionHeader` usage is only for keyboard-focus/roving-tabindex testing
  (ArrowDown nav), not open/closed state — unaffected.
- No visual/snapshot regression tests (e.g. Playwright screenshot diffs) were found for this
  page; `tests/e2e/` was not grepped exhaustively but no snapshot-testing convention is
  documented in this repo's e2e conventions (`.claude/rules/e2e-test-conventions.md` forbids
  `waitForTimeout` and CSS selectors but doesn't mention screenshot snapshots).
- **New test needed**: an assertion (in `BacklogItemDetail.test.tsx` or a new item-detail test)
  that a fresh item (no `backlog-detail-section-<id>-description` localStorage key) renders
  `collapsible-header-description` with `aria-expanded="true"`, and a companion test that an
  item with a *stored* `"false"` preference still renders collapsed (per the requirements doc's
  explicit non-regression bullet).

## 4. Accessibility (Radix Accordion default-open items)

- Radix `Accordion` (`type="multiple"`) with a section pre-included in `value`/`defaultValue`
  renders that panel's content in the DOM on first paint with `aria-expanded="true"` on the
  trigger button and the content region present (not `display:none`-only — `CollapsibleSection`
  already documents this: "body is removed from the DOM (not just visually hidden) while
  collapsed", so expanded content is genuinely present, not merely revealed).
- Focus order: no autofocus is forced onto the newly-expanded panel — Radix Accordion does not
  move focus into content on mount, so keyboard/tab order is unaffected; the page's initial
  focus behavior is unchanged.
- Screen reader announcement: an SR user tabbing to the Description header will hear
  `aria-expanded="true"` and encounter the rendered markdown immediately after activating/
  reading past the trigger, one section earlier in document order than before — a minor,
  expected-by-design change (surfacing content SR users previously had to expand manually is
  arguably an accessibility improvement, not a regression), and consistent with how
  `pull-request`/`sessions` sections already default open today.
- Because `Accordion.Root` is `type="multiple"`, expanding one more section doesn't collapse or
  otherwise disturb any other section's state — no group-wide side effect.
- No accessibility regression identified; if anything this aligns Description with the existing
  precedent of `pull-request` (`defaultExpanded={true}` hardcoded) and `sessions`
  (`useSectionExpandState(itemId, "sessions", true)`), both already shipped default-open.

## Summary of concrete follow-ups for the implementation step

1. Flip `useSectionExpandState(itemId, "description", false)` → `true` in
   `BacklogItemDetail.tsx:323` (the core fix — functionally sufficient on its own).
2. Also refactor `DescriptionSection.tsx` to accept `defaultExpanded: boolean` as a prop
   (matching `PlanArtifactsSection`/`VersionControlSection`/etc.) instead of hardcoding
   `defaultExpanded={false}`, to remove stale/misleading code the dev-warning can't catch.
   Update its stale docstring ("collapsed by default (Story 3.1.3, Task 3.1.3a)").
3. Update `DescriptionSection.test.tsx` for the new prop if #2 is done.
4. Add assertions in `BacklogItemDetail.test.tsx` (or new tests) for: (a) fresh item →
   `collapsible-header-description` `aria-expanded="true"`; (b) item with stored `"false"`
   preference → stays `aria-expanded="false"` (non-regression per requirements doc).
5. No CSS, mobile-layout, or accessibility fix required — behavior is consistent with existing
   default-open sections (`pull-request`, `sessions`).
