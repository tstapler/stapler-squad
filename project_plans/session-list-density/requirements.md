# Requirements: session-list-density

**Date**: 2026-09-23
**Type**: feature addition (UI redesign, no backend/proto changes)
**Complexity**: 2 — focused feature

## Problem Statement

The sessions sidebar (both List view rows and Board view cards) wastes horizontal
space and truncates information the user actually needs. Session names and
worktree paths are cut with a trailing ellipsis at a fixed single-line row height,
so the user can't tell sessions apart at a glance without hovering for a tooltip.
Two default-visible columns (the per-row agent glyph, the memory/RSS value) are
dead weight for most rows: nearly every session in this workspace runs Claude Code
(agent glyph is redundant), and ~80% of visible rows are paused/backlog sessions
with no live process (memory column shows "—"). On narrow/mobile viewports this is
worse — the same cramped single-line layout has even less room to work with.

## Baseline

Today: fixed 38px-tall single-line CSS grid row. Name and path are
`text-overflow: ellipsis; white-space: nowrap` — long values are silently cut off
with no way to read them except a hover tooltip. The agent-glyph and memory
columns render on every row regardless of whether they carry information for that
row. The "···" overflow button has no touch-target sizing. There is no
narrow-width/mobile-specific layout for either the List row or Board card.

## Users / Consumers

Single user (personal dev tool), viewing the sidebar in a fixed-width desktop
panel (~280px per `SessionList.css.ts`) and occasionally on mobile.

## Success Metrics

- No more ellipsis-truncated session names or paths at the sidebar's normal
  width — the longest real names/paths in the current session list (e.g.
  `pr-424-compute-nop-18c993e1e9402c…`,
  `~/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/...`) are fully
  readable in the row/card itself, without a hover tooltip.
- The per-row agent glyph and memory columns no longer occupy default grid space
  on rows where they carry no information (i.e., removed from the default List
  row and Board card layout; still reachable via the existing Columns picker /
  tooltip).
- The "···" overflow button meets a 44×44px touch target on
  `(pointer: coarse)`, matching the existing `inlineActionButton` treatment.
- Both List rows and Board cards adapt at narrow container widths without
  requiring horizontal scroll or clipped text.

## Appetite

Medium (1–2 weeks). Covers both List view (`SessionRow.tsx`) and Board view
(`SessionCard.tsx`/`BoardCard.tsx`), plus the smart path-truncation logic below.
If the smart-truncation heuristic or Board-view parity ends up needing more
design iteration than expected, cut Board view to a fast-follow rather than
extending the appetite.

## Constraints

- Personal single-user tool — no compliance/multi-tenant concerns.
- No proto/backend changes; this is presentation-layer only (React components +
  vanilla-extract CSS under `web-app/src/components/sessions/`).
- Must preserve existing ARIA labels/tooltips for data moved off the default
  grid (agent program, memory RSS) — the data isn't removed, only its default
  visual column.
- Must not regress existing Playwright e2e locators in `tests/e2e/` that target
  `SessionRow`/`SessionCard` by `data-testid`/ARIA role.

## Non-functional Requirements

- **Performance SLO**: not specified — this is a layout/CSS change, no new
  runtime computation beyond a cheap per-render truncation heuristic on path
  strings.
- **Scalability**: not applicable — session counts are what they are (dozens,
  per screenshot).
- **Security classification**: internal/personal.
- **Data residency**: not applicable.

## Scope

### In Scope
- List view row redesign (`SessionRow.tsx`, `SessionRow.css.ts`,
  `session-columns.ts`): drop `agent`/`memory` from default visible columns,
  variable row height, name/path wrap instead of ellipsis, container-query-based
  narrow layout, "···" touch-target fix.
- Board view card redesign (`SessionCard.tsx`, `BoardCard.tsx`): apply the same
  column-trimming and smart path-truncation logic; card-specific layout
  adjustments as needed (cards already have more vertical room than list rows,
  so the narrow-width problem may look different there — worth a quick check
  during planning rather than assuming an identical fix).
- Smart/dynamic path truncation: when a path is too long to fit even after
  wrapping, collapse repetitive/opaque middle segments (e.g. the workspace hash
  `6eb0b580fa0331d5`, worktree UUID suffixes like `_18d807dfb97a2b28`) rather
  than truncating from a fixed end, while preserving human-meaningful segments
  (repo/session name, branch name) at both the start and end. Exact heuristic
  (which segments count as "opaque") is a research/planning question, not
  resolved here — see Open Questions.
- Keep agent/memory data accessible via existing Columns picker and
  tooltips/ARIA labels.

### Out of Scope
- Any other sidebar UI (Backlog panel, Filters, Search box, category group
  headers) — only the individual session row/card layout.
- Backend/proto changes, new session data fields.
- Changing what data is tracked (agent program, memory RSS) — only where/how
  it's displayed.

## Rabbit Holes

- The smart middle-truncation heuristic (which path segments are "opaque
  noise" vs. meaningful) could balloon into a general-purpose path-formatting
  utility if not scoped tightly — Phase 3 planning should pin down a concrete,
  testable rule (e.g. "collapse any path segment that is a 16+ char hex/UUID
  string") rather than open-ended heuristics.
- Board view parity: cards may not have the same "wasted space" problem as
  rows (different aspect ratio, more room already) — don't force-fit the exact
  same column removal if the Board card's actual pain point turns out to be
  different. Verify with real card screenshots/data before assuming identity
  with the List-row fix.
- Container queries are new to this codebase for this component (verified: no
  existing usage in `SessionRow.css.ts`/`SessionList.css.ts`, though
  `FilesTab.css.ts` references `containerType`) — worth a quick spike to confirm
  browser/build-tool support (vanilla-extract + the project's target browsers)
  before committing the whole narrow-layout design to that primitive.

## Alternatives Considered

- Viewport `@media` breakpoint (existing repo pattern, `SessionList.css.ts:14`,
  768px) instead of a container query — rejected because the sidebar is a fixed
  ~280px column regardless of viewport width; a viewport breakpoint would
  trigger at the wrong times (or never, on a wide desktop viewport with a
  narrow sidebar).
- Truncating from a fixed end (start or end) instead of dynamic middle
  collapsing — rejected per user's explicit direction in favor of a smarter,
  content-aware heuristic that drops repetitive/opaque segments.

## Feasibility Risks

- Container-query support/tooling: needs confirmation that the project's
  vanilla-extract setup and target browsers support `container-type`/`@container`
  cleanly (spike recommended in Phase 2 research).
- `-webkit-line-clamp` + variable row height interaction with existing list
  virtualization (if `SessionList.tsx` virtualizes rows) — variable heights can
  break naive virtualization; needs a code check in Phase 2 research.
- Existing e2e locators/tests may assert on current fixed-height row structure
  or the columns being removed — needs a grep-and-audit pass before
  implementation (flagged in the prior UX review).

## Observability Requirements

Not applicable (complexity 2).

## Risk Control

Not applicable (complexity 2) — this is a low-risk, easily-revertable
presentation-layer change with no feature flag needed; standard PR review and
e2e/Axe/Lighthouse CI (already wired for `web-app/src/` PRs) is sufficient.

## Open Questions

- Exact rule for "opaque" path segments eligible for middle-collapse (hex hash,
  UUID, both, minimum length threshold?) — resolve in Phase 2/3.
- Does `SessionList.tsx` virtualize rows? If so, how does variable row height
  get reconciled with the virtualizer — resolve in Phase 2 research.
- Does Board view's `BoardCard.tsx` actually suffer the same "wasted space"
  problem, or does it need a different fix entirely — resolve in Phase 2/3 with
  a quick look at real Board view screenshots.
