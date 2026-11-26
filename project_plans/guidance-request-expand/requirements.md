# Guidance Request Expand — Requirements

## Problem Statement

Guidance requests on the backlog item detail page display long question text and answer
text in a fixed layout that visually truncates content (confirmed in screenshot:
`[waive AC 0] Reviewer says... re...` ends mid-sentence). There is no affordance to
reveal the full content, leaving owners unable to read the full context of a guidance
request without navigating away.

## Scope

Affects `GuidanceRequestPanel` (`web-app/src/components/guidance/`) which is shared by:
- `BacklogItemDetail` (backlog-item scope, primary report location)
- `TriageReviewPanel` (same scope, different embeds)
- `SessionDetailView` (session scope)

The fix should apply uniformly across all three embed sites by virtue of being in the
shared component.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC-0 | Long question text in a guidance request (both pending and answered, legacy and structured) is clamped to ≤5 lines by default when longer than the clamp threshold. |
| AC-1 | A "Show more" toggle button appears below clamped text. Clicking it expands the text to show all content. |
| AC-2 | An expanded item shows a "Show less" toggle that collapses it back to the clamped view. |
| AC-3 | Short question text that fits within the clamp threshold is shown without any toggle button (no unnecessary affordance). |
| AC-4 | Long answer text in an answered legacy request is also expandable with the same show-more/show-less pattern (separate expand state from the question text). |
| AC-5 | The expand/collapse state is per-item and independent (expanding one request does not affect others). |
| AC-6 | The expand state is session-local (not persisted to localStorage; resets on page reload). |
| AC-7 | The toggle button is keyboard-accessible and has appropriate ARIA attributes (`aria-expanded`, descriptive accessible label). |
| AC-8 | Existing `GuidanceRequestPanel` tests pass; new tests cover show-more/less toggling. |
| AC-9 | The implementation does not introduce jscpd/dupl violations (reuse a shared `ExpandableText` or inline hook; no copy-pasted block per request type). |

## Out of Scope

- Persistence of expand state to `localStorage` or server (AC-6 explicitly excluded)
- Modifying the `CollapsibleSection`/`CollapsibleGroup` Radix component (that is for
  section-level collapse, not inline text expansion)
- Changes to the Go backend or proto definitions
- Changes to other panels (DescriptionSection, NotesSection, etc.)

## Constraints

- CSS: use vanilla-extract (`*.css.ts`) — no inline `style={{}}` for the clamp/expand state
- No new external dependencies (the clamping pattern is achievable with
  `-webkit-line-clamp` via vanilla-extract's `style()`)
- Must not use `window.location` or any routing change — purely local state
- Duplication gate: `bun run lint:duplicates` in `web-app/` must stay under threshold
  (0.14% at time of writing; `jest.mock(...)` lines are pre-counted headroom)
