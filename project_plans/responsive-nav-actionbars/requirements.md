# Requirements: Responsive Nav & Action Bars

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-13

## Problem Statement

All toolbar and menu areas in the Stapler Squad web UI overflow, clip, or behave poorly at non-desktop widths. The user resizes their browser window frequently during use and uses the app on mobile (phone). Every horizontal bar — the top navigation, session action bars, filter bars, and modal toolbars — must adapt seamlessly to any width without clipping, overflow, or unreachable controls.

Specific known breakages:
1. **Top nav header** (`Header.tsx`) — at medium widths (~800–1100px) the `.actions` area (WorkspaceSwitcher + New Session + ApprovalNavBadge + 3 icon buttons) competes with the centered nav links; no intermediate breakpoint exists between desktop (full) and mobile (hamburger).
2. **Session filter/action bars** — various filter dropdowns, search inputs, and group-by selectors in `SessionList` are ad-hoc flex rows without scroll or wrap behavior.
3. **ActionBar component underused** — `ActionBar.tsx` already exists with `scroll` and `wrap` support, but most bars in the app don't use it.
4. **Mobile (phone widths)** — on the `.actions` row at ≤768px the WorkspaceSwitcher still renders at full width, crowding the icon buttons.

## Success Criteria

- Top nav header renders without overflow or clipping from 320px to 2560px
- Every filter bar, action bar, and toolbar in the app is fully accessible at any viewport width (no unreachable controls)
- Dynamic window resizing produces no layout jumps or clip artifacts at any intermediate width
- Mobile (≤768px) — all interactive elements meet 44px minimum touch target
- No new component APIs introduced unless the existing `ActionBar` is genuinely insufficient

## Scope

### Must Have (MoSCoW)
- Fix `Header.module.css` at intermediate widths (~800–1100px): either add a mid breakpoint or compress the actions area
- Audit every toolbar/filter bar in the app and ensure scroll/wrap behavior is applied
- Apply `ActionBar` component (or equivalent pattern) consistently everywhere
- WorkspaceSwitcher in the header must not overflow at any width

### Should Have
- Introduce a `768px–1024px` intermediate header breakpoint that hides text labels while keeping all action buttons
- Consolidate responsive toolbar patterns into a single reusable approach (extend `ActionBar` if needed)
- Ensure `HistoryFilterBar`, `SessionList` filters, `TerminalOutput` toolbar all use the same pattern

### Out of Scope
- Terminal toolbar overflow on mobile — covered in `project_plans/mobile-ux-improvements/`
- iOS virtual keyboard handling — covered in `project_plans/mobile-ux-improvements/`
- Safe-area insets — covered in `project_plans/mobile-ux-improvements/`
- Full mobile redesign or new layout architecture
- Performance optimization
- Adding new navigation items or features

## Constraints

Tech stack: React (Next.js App Router), CSS Modules + vanilla-extract (new styles in `.css.ts`), existing globals.css tokens
CSS rule: All new styles must use `.css.ts` per ADR-009. Existing `.module.css` edits are fine for surgical fixes.
Theme: Must not break dark or light mode (use defined CSS variables only)
Dependencies: No new npm packages unless unavoidable
Browser targets: iOS Safari 15.4+, Chrome 108+, Firefox 101+

## Context

### Existing Work
- `Header.module.css` already has `@media (max-width: 768px)` for hamburger menu — good foundation, needs mid-breakpoint
- `ActionBar.tsx` + `ActionBar.module.css` already exist with `scroll` prop (overflow-x: auto at ≤640px) and `flex-wrap: wrap` default
- `ViewportProvider.tsx` already exists and writes `--keyboard-height` / `--viewport-height` CSS vars
- `globals.css` defines `--min-touch-target: 44px` (unused in most toolbars)
- `TerminalOutput` toolbar overflow fix is tracked separately in `mobile-ux-improvements`
- mobile-ux-improvements research is fully complete — stack/features/architecture/pitfalls all have findings

### Affected Components (initial audit)
- `web-app/src/components/layout/Header.tsx` + `Header.module.css` — top nav (primary)
- `web-app/src/components/layout/WorkspaceSwitcher.tsx` — overflows in header at narrow widths
- `web-app/src/components/sessions/SessionList.tsx` — filter bar (Group by, Status, Tag dropdowns + search)
- `web-app/src/components/history/HistoryFilterBar.tsx` — history filter toolbar
- `web-app/src/components/logs/` — DensityToggle, ExportButton, LiveTailToggle, TimeRangePicker, MultiSelect in a toolbar row
- `web-app/src/components/ui/ActionBar.tsx` — the shared component to standardize on

### Stakeholders
Solo developer/owner using the app daily — active window resizing, mobile use.

## Relation to Existing Plans

This plan is **additive** to `mobile-ux-improvements`. That plan focuses on iOS-specific issues (virtual keyboard, safe-area insets, terminal toolbar). This plan focuses on responsive layout correctness at all widths. Implementation may be done in a single PR or sequentially.

## Research Dimensions Needed

- [x] Stack — viewport CSS, breakpoints, overflow patterns (covered in mobile-ux-improvements/findings-stack.md)
- [ ] Features — audit all existing toolbars/action bars and map which use ActionBar vs ad-hoc flex
- [ ] Architecture — decide: extend ActionBar, use container queries, or CSS-only breakpoints per component
- [ ] Pitfalls — overflow: auto vs hidden conflicts, WorkspaceSwitcher min-width issues, sticky header height changes across breakpoints
