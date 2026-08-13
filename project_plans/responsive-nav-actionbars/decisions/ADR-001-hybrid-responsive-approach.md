# ADR-001: Hybrid Responsive Approach (ActionBar Extension + Targeted CSS Fixes)

Status: Accepted
Date: 2026-04-13
Deciders: Tyler Stapler

## Context

The Stapler Squad web UI has responsive layout defects across all toolbar/action bar areas. Three approaches were evaluated during research:

- **Approach A**: Extend ActionBar with a `compact` prop, then apply it consistently to SessionList filter bar, HistoryFilterBar, and logs toolbar.
- **Approach B**: Use CSS container queries for component-intrinsic breakpoints.
- **Approach C**: Targeted CSS-only media query fixes per component (Header, WorkspaceSwitcher).

## Decision

Use a hybrid of **Approach A + Approach C**.

**Approach A** (ActionBar extension) covers 80% of toolbar responsiveness:
- Add a `compact` prop to `ActionBar` that reduces gap and enables scroll behavior below 1024px (instead of the current 640px).
- Wrap SessionList filter controls, HistoryFilterBar `.filters` row, and logs page `.headerActions` + `.filters` in ActionBar with `scroll` + `compact`.
- This gives all toolbars a consistent responsive pattern without duplicating media queries.

**Approach C** (targeted CSS fixes) handles the remaining 20% that ActionBar cannot cover:
- Header.module.css needs a 1024px intermediate breakpoint (compress `.actions` gap, hide "New Session" label, shrink nav link padding).
- WorkspaceSwitcher.module.css dropdown needs `max-width: min(320px, calc(100vw - 1rem))` to prevent viewport overflow on phones.
- globals.css `--header-height` transition from 4rem to 3.5rem needs to be smoothed to avoid content shift.

**Approach B** (container queries) was rejected because iOS Safari 15.4 does not support `@container` (requires Safari 16+). This constraint comes from the project's stated browser targets.

## Consequences

**Positive:**
- Single responsive pattern for toolbars (ActionBar) reduces CSS duplication and maintenance burden.
- Targeted Header/WorkspaceSwitcher fixes are surgical and low-risk.
- No new dependencies required.
- Breakpoints will be aligned: 768px (mobile), 1024px (tablet/compact), with ActionBar scroll at 640px as a final fallback.

**Negative:**
- ActionBar gains a new `compact` prop, adding one axis of complexity to the component API.
- Header.module.css gets a second media query (1024px), increasing its specificity surface.
- Existing components that currently use `display: contents` for their filter wrappers (SessionList `.filterTopRow`, `.filterControls`) will need structural changes to wrap in ActionBar.

**Risks:**
- The `display: contents` removal in SessionList filter bar changes the DOM flow; mobile filter toggle behavior must be preserved.
- Adding `overflow: hidden` to Header to prevent silent clipping may require careful z-index management for the mobile nav dropdown.

## Alternatives Considered

| Approach | Pros | Cons | Verdict |
|----------|------|------|---------|
| A: Extend ActionBar | Single pattern, reusable, already exists | Needs `compact` prop, structural changes to consumers | **Accepted** |
| B: Container queries | Most elegant, component-intrinsic | iOS Safari 15.4 not supported | **Rejected** |
| C: CSS-only per component | No code changes, just CSS | Duplicates responsive logic across 6+ files | **Partially accepted** (Header/WorkspaceSwitcher only) |
