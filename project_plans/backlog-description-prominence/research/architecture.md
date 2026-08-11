# Architecture Research — Backlog Description Prominence

No prior `project_plans/*/research/architecture.md` documents this exact change (checked all
existing architecture.md files under `project_plans/`; `backlog-item-detail-ux` documents the
broader CollapsibleSection/CollapsibleGroup extraction this item builds on, cited below, but
does not cover Description's default-expanded value specifically).

## 1. Sibling prop-threading pattern to mirror

Canonical example — `web-app/src/components/backlog/detail/NotesSection.tsx`:
- Line 8-18: `NotesSectionProps` includes `defaultExpanded: boolean` (required, not optional).
- Line 38: `<CollapsibleSection sectionKey="notes" title="Notes" defaultExpanded={defaultExpanded}>` — the prop is passed straight through, never hardcoded.

Second example — `web-app/src/components/backlog/detail/ReviewingSection.tsx` lines 13, 39, 59: identical shape (`defaultExpanded: boolean` prop → passed verbatim to `CollapsibleSection`).

`BacklogItemDetail.tsx` side of the pattern (`web-app/src/components/backlog/BacklogItemDetail.tsx`):
- Line 322: `const [notesExpanded, setNotesExpanded] = useSectionExpandState(itemId, "notes", false);`
- Line 380: `["notes", notesExpanded, setNotesExpanded]` entry in `sectionExpandEntries`.
- Lines 1244-1250: `<NotesSection item={item} ... defaultExpanded={notesExpanded} ... />`.

**Exact change to mirror for Description:**
- `BacklogItemDetail.tsx:323` — change `useSectionExpandState(itemId, "description", false)` → `useSectionExpandState(itemId, "description", true)` (only the third arg changes; this is the one-line default-value flip in the requirements doc).
- `BacklogItemDetail.tsx:1215` — change `<DescriptionSection item={item} />` → `<DescriptionSection item={item} defaultExpanded={descriptionExpanded} />`.
- `DescriptionSection.tsx:10-12` — add `defaultExpanded: boolean` to `DescriptionSectionProps` (required, matching `NotesSectionProps`/`ReviewingSectionProps`, not optional-with-default).
- `DescriptionSection.tsx:18-20` — accept `defaultExpanded` in the destructured props and pass it through: `<CollapsibleSection sectionKey="description" title="Description" defaultExpanded={defaultExpanded}>` (replacing the hardcoded `defaultExpanded={false}`).
- `DescriptionSection.tsx:14-17` — stale docstring says "secondary info, collapsed by default (Story 3.1.3, Task 3.1.3a)"; must be updated since it will no longer be collapsed by default.

Note: `PullRequestSection.tsx:31` still hardcodes `defaultExpanded={true}` internally (does **not** accept a `defaultExpanded` prop, unlike its neighbor `ReviewingSection`) — it is not actually part of the ~8 prop-threaded siblings despite being grouped with `ReviewingSection` in the requirements doc's parenthetical; `NotesSection`/`ReviewingSection`/`SessionsSection`/`VersionControlSection`/`PlanArtifactsSection`/`WorkflowHistorySection`/`ProgressHistorySection`/`LastReviewResultSection` are the real 8 reference implementations. `DescriptionSection` and `PullRequestSection` are the only two sections in the file that still hardcode their default internally rather than accepting it as a prop — this change brings Description in line with the majority pattern (PullRequestSection is out of scope per the constraints).

## 2. Full data flow — confirmed single integration point, but with an important nuance

Flow: `BacklogItemDetail.tsx:323` (`useSectionExpandState` third-arg default) → `descriptionExpanded` state var → `sectionExpandEntries` (line 374: `["description", descriptionExpanded, setDescriptionExpanded]`) → `openSectionKeys` (line 382, `.filter(([, expanded]) => expanded).map(([key]) => key)`) → passed as the **controlled** `value` prop to `<CollapsibleGroup value={openSectionKeys} onValueChange={handleGroupValueChange}>` (line 1185) → `DescriptionSection` rendered at line 1215 *inside* that group.

**Critical finding from `web-app/src/components/ui/Collapsible.tsx` (lines 127-177):** `DescriptionSection` is always rendered inside `CollapsibleGroup` (not standalone), so `CollapsibleSection`'s own `insideGroup` branch (line 137) applies. In that branch:
- The prop `defaultExpanded` passed to `CollapsibleSection` is **architecturally dead for driving actual open/closed state** — only the group's own `value`/`defaultValue` (i.e. `openSectionKeys`, ultimately sourced from `useSectionExpandState`'s return value) drives the Radix Accordion's real open state (comment at Collapsible.tsx:49-53: "this is what determines each section's real open/closed state — never a child's own defaultExpanded").
- However, in dev (`NODE_ENV !== "production"`), lines 138-157 compare the passed `defaultExpanded` against `openKeys.includes(sectionKey)` and **`console.warn` if they diverge** (`defaultExpanded && !groupSaysOpen`). This means the current code (hardcoded `defaultExpanded={false}` in `DescriptionSection.tsx:20`) is *latently buggy*: if a user previously expanded Description (stored `"true"` in localStorage) revisits the item, `descriptionExpanded` is `true`, `openSectionKeys` includes `"description"`, the accordion correctly renders it open — but the hardcoded `false` passed into `CollapsibleSection` diverges from that, firing a dev console warning every render. Threading the real `defaultExpanded={descriptionExpanded}` prop through (per the fix above) eliminates this latent warning as a side effect, in addition to matching the sibling pattern required by requirement #4.

**So the answer to "does changing the default value of `useSectionExpandState`'s third arg have any effect on `CollapsibleGroup`'s controlled `value` prop behavior, or are they independent?"**: they are **not independent** — the third arg *is* the sole source of the initial (no-stored-preference) value that flows into `openSectionKeys` → the group's controlled `value`. Changing `false` → `true` on line 323 is what actually flips the rendered default-expanded state for Description. The `defaultExpanded` prop threaded down to `DescriptionSection`/`CollapsibleSection` itself has no effect on rendering while inside the group (per the code comment) — its only live purpose is to avoid the dev-mode divergence warning and to satisfy the structural-consistency requirement (#4).

No other place sets a default for Description: `CollapsibleGroup` itself has no `defaultValue` supplied at this call site (only `value`/`onValueChange`, i.e. fully controlled — see `BacklogItemDetail.tsx:1185`), and `CollapsibleItem`/`Accordion.Item` (Collapsible.tsx:91-115) has no independent default-state logic. This confirms **BacklogItemDetail.tsx:323 is the only integration point** that needs to change to alter the actual rendered behavior; the `DescriptionSection.tsx` prop-threading change is required for API consistency (req #4) and to fix the latent dev-warning divergence, not to change the rendered default itself.

## 3. Acceptance Criteria isolation — confirmed, no coupling risk

`AcCriteriaList` renders at `BacklogItemDetail.tsx:1147-1152`, **before** `ActionsSection` (line 1155) and well before `<CollapsibleGroup>` opens at line 1185. It is a sibling JSX block in the always-visible primary content area, entirely outside the `CollapsibleGroup`/`CollapsibleGroupContext` tree — it never calls `useContext(CollapsibleGroupContext)`, is not part of `sectionExpandEntries`, and has no `useSectionExpandState` call keyed to it. There is no shared state, no shared React context, and no rendering dependency between `AcCriteriaList` and `DescriptionSection`/`descriptionExpanded`. The two are structurally isolated branches of the same parent's JSX return; the requirements doc's "always-visible rendering/layout must be provably unaffected" constraint is satisfiable purely by an assertion test that AC's DOM/test-ids are unchanged — there is no code path by which this change could touch it.

## Summary of the minimal diff

1. `BacklogItemDetail.tsx:323` — `useSectionExpandState(itemId, "description", false)` → `..., true)`.
2. `BacklogItemDetail.tsx:1215` — thread `defaultExpanded={descriptionExpanded}` into `<DescriptionSection>`.
3. `DescriptionSection.tsx` — add `defaultExpanded: boolean` to props, destructure it, pass to `CollapsibleSection`, update stale docstring.

No changes needed to `Collapsible.tsx`, `CollapsibleGroup`'s call site props, `sectionExpandEntries`, `openSectionKeys`, or `AcCriteriaList`.
