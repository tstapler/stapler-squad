# ADR-001: Session List Compact Row Implementation Strategy

**Status**: Accepted
**Date**: 2026-05-14
**Project**: sexy-ui

## Context

The sexy-ui redesign requires session list items to render as 36–40px compact single-line rows (REQ-2). The current implementation is `SessionCard.tsx`, a 700+ line component with:

- `padding: 16px` and a multi-row layout (Program, Branch, Path, Working Dir, Repository, PR link sub-rows)
- A 120px terminal snapshot pane
- A footer with timestamps
- Embedded features: selection mode, fork dialog, rate-limit badges, approval indicators, inline actions

These embedded features are load-bearing — approval indicators and rate-limit badges in particular surface async backend state that is not trivially re-implemented. The compact row design cannot carry all of this visual weight in a 36–40px tall cell.

The project also has an established "power user configurability" theme (density preferences, multiple grouping strategies, workspace isolation modes), making a user-controlled density toggle a natural fit.

## Decision

We decided to create a new `SessionRow.tsx` component alongside the existing `SessionCard.tsx`. A density setting in the app's Settings panel controls which component renders in the session list. `SessionCard.tsx` is preserved without modification.

## Alternatives Considered

- **Option B — Replace `SessionCard.tsx` entirely**: Migrate all features (selection, fork dialog, rate-limit badges, approval indicators, inline actions) into the new compact row design. Rejected because the migration surface is large and the risk of silently breaking approval/rate-limit state display is high. There is no incremental fallback path if a feature regresses.

- **Option C — Progressively refactor `SessionCard.tsx` in-place**: Reduce padding, collapse multi-row layout to single line, inline sub-rows. Rejected because `SessionCard.tsx` is already highly coupled; in-place surgery on a 700+ line component that embeds selection mode and fork dialog state is likely to introduce subtle regressions without a clean isolation boundary.

## Rationale

Option A provides a hard isolation boundary: `SessionRow.tsx` is built greenfield against REQ-2 without touching the approval/rate-limit code paths in `SessionCard.tsx`. The density toggle in Settings follows the existing configurability pattern (grouping strategies, workspace isolation) and gives users who rely on the rich card view an explicit opt-out. The two components can coexist indefinitely; `SessionCard.tsx` can be deprecated in a later ADR once `SessionRow.tsx` achieves full feature parity.

## Consequences

**Positive:**
- `SessionCard.tsx` is untouched; approval indicators and rate-limit badges cannot regress.
- `SessionRow.tsx` is independently testable from day one with no legacy state to carry.
- A density toggle is a natural UX primitive that aligns with the app's existing power-user configurability pattern.
- Future work can incrementally migrate individual features from `SessionCard.tsx` to `SessionRow.tsx` one at a time.

**Negative / Risks:**
- Two components rendering the same conceptual entity create a surface area for feature drift: new features added to `SessionCard.tsx` must also be evaluated for `SessionRow.tsx`.
- The density toggle adds a Settings UI touchpoint and a conditional render in the session list parent that must be maintained.
- `SessionRow.tsx` starts with a reduced feature set; users who switch to compact mode will not see fork dialog, terminal snapshot, or full timestamp footer until those are ported.

**Follow-up work:**
- Define the minimum viable feature set for `SessionRow.tsx` v1 (which of the embedded features are in-scope for the initial compact row).
- Add a `sessionListDensity: "compact" | "card"` field to the app's settings schema and persist it.
- Wire the density toggle in the Settings panel.
- Implement the conditional render in the session list parent (`SessionList.tsx` or equivalent).
- Establish a feature parity tracking checklist between `SessionCard.tsx` and `SessionRow.tsx` to manage drift.
- Schedule a deprecation review for `SessionCard.tsx` once `SessionRow.tsx` reaches parity.

## Implementation Notes

- `SessionRow.tsx` should live colocated with `SessionCard.tsx` in `web-app/src/components/sessions/`.
- Use vanilla-extract (`.css.ts`) for all new styles per ADR-009 / `.claude/rules/css-architecture.md`. The row height target is `36px` min / `40px` default; use `vars.space` tokens, not hardcoded pixel values.
- The density preference should be read from a React context or the existing settings store — do not thread it as a prop through the session list hierarchy.
- `SessionRow.tsx` must carry `data-testid` attributes on all interactive elements from the start; Playwright e2e tests rely exclusively on `data-testid` and ARIA roles (no CSS class selectors).
- Rate-limit badges and approval indicators are explicitly out of scope for `SessionRow.tsx` v1. Document this as a known limitation in the feature registry entry.

## Related

- Requirements: `project_plans/sexy-ui/requirements.md`
- CSS architecture: `.claude/rules/css-architecture.md` (ADR-009)
- Supersedes: (none)
- Related ADRs: (none)
