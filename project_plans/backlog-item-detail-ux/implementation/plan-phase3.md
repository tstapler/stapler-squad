# Implementation Plan — Phase 3: AcCriteriaList + Section Grouping + Description Default
**Date**: 2026-10-17
**Status**: Ready for implementation
**Extends**: `implementation/plan-phase2.md` (Phase 2, fully implemented)
**Research basis**: `research/phase3-exploratory-audit.md`
**Chosen approach**: Alternative A — Consolidate + Collapsible Criteria

---

## Background

Exploratory audit (Phase 3 research) identified six confusion points in the post-Phase-2
detail view:

1. **C1**: AcCriteriaList is always-expanded and not in the CollapsibleGroup, blocking
   navigation to sections below it.
2. **C2**: 15 CollapsibleGroup sections with no grouping cues.
3. **C3**: Four overlapping audit/history sections with no clear entry point.
4. **C4**: DescriptionSection defaults collapsed.
5. **C5**: PlanningSection appears before AcCriteriaList (wrong reading order for review).
6. **C6**: Dead space for fresh idea items (empty GateBlockingSection / DependenciesSection).

Phase 3 addresses C1, C2, C4 directly. C3 (audit trail merge) is deferred to Phase 3.5
pending dogfooding of the group-label approach. C5 and C6 are addressed partially.

---

## Epic P3.1: AcCriteriaList into CollapsibleGroup

### Story P3.1.1: Move AcCriteriaList into CollapsibleGroup as collapsible section
**Goal**: Eliminate the always-expanded criteria barrier that prevents scrollless navigation
to CollapsibleGroup section headers.

**Acceptance Criteria**:
- AcCriteriaList renders inside the CollapsibleGroup, not between PlanningSection and it.
- The section header shows the same count as today: "Acceptance Criteria (N/M)".
- Default expanded for `review` and `pr_pending` status (completion progress is most
  relevant in these states); collapsed for all other statuses.
- A user's explicit expand/collapse choice is persisted via `useSectionExpandState`
  (key: `"ac-criteria"`) and restored on subsequent opens of the same item.
- `data-testid="ac-criteria-list"` is preserved on the AcCriteriaList itself.
  The collapsible wrapper gets `data-testid="ac-criteria-section"`.

**Files**:
- `web-app/src/components/backlog/BacklogItemDetail.tsx`
- `web-app/src/components/backlog/BacklogItemDetail.test.tsx`
- `web-app/src/components/backlog/detail/AcCriteriaSection.tsx` (new thin wrapper, ~30 lines)
- `web-app/src/components/backlog/detail/AcCriteriaSection.test.tsx`

#### Task P3.1.1a: Create AcCriteriaSection collapsible wrapper (~4 min)
- New file `detail/AcCriteriaSection.tsx`.
- Wraps `<CollapsibleSection>` (or equivalent Radix-based wrapper used by other sections)
  around `AcCriteriaList`.
- Props: `item: BacklogItem`, `defaultExpanded: boolean`.
- Header text: `Acceptance Criteria (${done}/${total})` — same count format as today.
- `data-testid="ac-criteria-section"` on the section root.
- Files: `detail/AcCriteriaSection.tsx`

#### Task P3.1.1b: Remove standalone AcCriteriaList from BacklogItemDetail scroll area (~2 min)
- Remove the `<div className={styles.section}>` block (lines ~1760–1766) that renders
  `<AcCriteriaList>` outside the CollapsibleGroup.
- Files: `BacklogItemDetail.tsx`

#### Task P3.1.1c: Add useSectionExpandState entry + wire into CollapsibleGroup (~3 min)
- Add `const [acCriteriaExpanded, setAcCriteriaExpanded] = useSectionExpandState(itemId, "ac-criteria", ...)`.
- Default: `item.status === "review" || item.status === "pr_pending"`.
- Add `["ac-criteria", acCriteriaExpanded, setAcCriteriaExpanded]` to the `expandStates` array.
- Render `<AcCriteriaSection ... defaultExpanded={acCriteriaExpanded} />` in the CollapsibleGroup
  at the Reference group position (after DescriptionSection, before PlanArtifactsSection).
- Files: `BacklogItemDetail.tsx`

#### Task P3.1.1d: Tests (~4 min)
- Test: `review` status → AcCriteriaSection is expanded by default.
- Test: `in_progress` status → AcCriteriaSection is collapsed by default.
- Test: `data-testid="ac-criteria-list"` is present inside `data-testid="ac-criteria-section"`.
- Files: `BacklogItemDetail.test.tsx`, `detail/AcCriteriaSection.test.tsx`

---

## Epic P3.2: Section Group Labels

### Story P3.2.1: Add visual group separators in CollapsibleGroup
**Goal**: Help users navigate the 15-section accordion by adding non-interactive group
labels that divide sections into 5 logical groups.

**Acceptance Criteria**:
- Five group labels are visible between section groups, styled as muted non-interactive
  text dividers (not accordion headers, not buttons).
- Group labels are: "Decision" | "Reference" | "Monitoring" | "Audit Trail" | "Admin".
- Group labels do not participate in the Radix Accordion keyboard nav (they are not
  `AccordionItem` elements — they are inert `div`s with a role of "presentation").
- Group labels are skipped by screen readers (hidden via `aria-hidden="true"`).
- On mobile (narrow viewport), group labels are readable (minimum contrast, minimum
  font size).

**Section-to-group mapping** (in render order):

| Group | Sections |
|---|---|
| Decision | ReviewingSection, LastReviewResultSection, PullRequestSection |
| Reference | SourceSection, DescriptionSection, AcCriteriaSection, PlanArtifactsSection |
| Monitoring | VersionControlSection, AutonomousHealthStrip, ItemStageCostTable |
| Audit Trail | SessionsSection, WorkflowHistorySection, ProgressHistorySection, ActivityLogSection |
| Admin | NotesSection, ManualOverrideSection |

**Files**:
- `web-app/src/components/backlog/BacklogItemDetail.tsx`
- `web-app/src/components/backlog/BacklogItemDetail.css.ts`

#### Task P3.2.1a: Add `sectionGroupLabel` CSS class (~2 min)
- In `BacklogItemDetail.css.ts`, add `sectionGroupLabel` style:
  muted text (token: `color.text.subtle` or equivalent), small font size, uppercase or
  all-caps tracking, 8px top/bottom margin, no border or background.
- Files: `BacklogItemDetail.css.ts`

#### Task P3.2.1b: Insert group label divs in BacklogItemDetail JSX (~5 min)
- Before the first section of each group, add:
  ```tsx
  <div className={styles.sectionGroupLabel} aria-hidden="true" role="presentation">
    Decision
  </div>
  ```
- Only Decision, Reference, Monitoring, Audit Trail, Admin groups need labels.
- Conditionally suppress the "Decision" label if ReviewingSection, LastReviewResultSection,
  and PullRequestSection are ALL gated out (e.g. `idea` status) — an empty group shouldn't
  show its label.
- Files: `BacklogItemDetail.tsx`

#### Task P3.2.1c: Test that group labels are present (~2 min)
- Render with `review` status → "Decision" and "Reference" group labels present.
- Render with `done` status (no reviewing/PR sections) → "Decision" label absent.
- Files: `BacklogItemDetail.test.tsx`

---

## Epic P3.3: DescriptionSection Default-Expanded

### Story P3.3.1: Flip DescriptionSection to default-expanded
**Goal**: Make item description immediately visible on first open, consistent with the
expectation that a new user's first question is "what is this item?"

**Acceptance Criteria**:
- `DescriptionSection` defaults to expanded on first open when no stored preference exists.
- A user's explicit collapse is persisted via `useSectionExpandState` and restored on
  subsequent opens.

**Note**: `useSectionExpandState`'s third argument is the `initialValue` used only when
no stored preference exists. Changing it from `false` to `true` affects only first opens
(or opens after the stored preference is cleared). No migration needed — existing stored
`"false"` preferences are respected.

**Files**:
- `web-app/src/components/backlog/BacklogItemDetail.tsx` (line ~460: `useSectionExpandState(itemId, "description", false)`)

#### Task P3.3.1a: Change Description initial expand value (~1 min)
- Change `useSectionExpandState(itemId, "description", false)` to
  `useSectionExpandState(itemId, "description", true)`.
- Files: `BacklogItemDetail.tsx`

#### Task P3.3.1b: Update the regression test that asserts Description is collapsed (~2 min)
- Find any test that asserts DescriptionSection's default collapsed state and update it
  to expect `aria-expanded="true"` (or update its setup to pre-set a `false` preference).
- Files: `BacklogItemDetail.test.tsx`, `BacklogItemDetail.regression.test.tsx`

---

## Epic P3.4: AcCriteriaList → DescriptionSection order fix

### Story P3.4.1: Reorder AcCriteriaSection to after DescriptionSection
**Goal**: "What is this item?" (Description) appears before "What does it need to achieve?"
(AcCriteriaSection) for a natural reading order.

**Acceptance Criteria**:
- In the Reference group, DescriptionSection precedes AcCriteriaSection in DOM order.
- AcCriteriaSection follows DescriptionSection immediately (no sections between them).

This is a consequence of the AcCriteriaSection placement in Task P3.1.1c: Description is
at CollapsibleGroup position 5 today (after Source); AcCriteriaSection should be placed at
position 6 (after Description, before PlanArtifacts).

**Files**: already covered by `BacklogItemDetail.tsx` change in Epic P3.1 — just confirm
render order in that task.

---

## Risk Control

- No backend changes — pure frontend display adjustments.
- AcCriteriaList is testid-based; moving it inside the CollapsibleGroup does not break
  its test surface.
- The ADR-027 single Radix Accordion root is preserved — group labels are inert `div`s,
  not AccordionItem elements, so they don't change the accordion's keyboard nav tree.
- Rollback: each epic is independently revertable.

## Not in Scope for Phase 3

- Audit Trail consolidation (Sessions + WorkflowHistory + ProgressHistory + ActivityLog →
  single section with tabs): deferred to Phase 3.5 pending dogfooding of the group-label
  approach first.
- Dead space for fresh idea items (C6): GateBlockingSection and DependenciesSection
  render null when there is no content — the dead space is CSS margin/padding, not actual
  rendered content. Address in a CSS-only pass if still noticeable after Phase 3.
- LifecycleSummary badge clutter (C7): styling concern separate from layout.
