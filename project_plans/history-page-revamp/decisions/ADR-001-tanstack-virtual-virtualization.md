# ADR-001: TanStack Virtual v3 for History List Virtualization

**Status**: Accepted
**Date**: 2026-04-12
**Project**: history-page-revamp

## Context

The history page currently loads up to 500 session entries eagerly and renders all of them in the DOM. This produces:
- Slow initial render (200ms+ even with moderate session counts)
- DOM thrashing when scrolling through large lists
- Memory pressure from 500+ mounted React components

We need a virtualization solution that renders only the visible rows, handles dynamic row heights (session cards expand/collapse to show message previews), and is compatible with Next.js App Router (React 18, RSC-safe).

## Decision

Use `@tanstack/react-virtual` v3 (`useVirtualizer` hook) for list virtualization.

## Rationale

| Criterion | react-window | react-virtuoso | TanStack Virtual v3 |
|---|---|---|---|
| Bundle size (minzipped) | ~6 KB | ~25 KB | ~10–15 KB |
| Next.js App Router compat | Yes | Yes | Yes (headless, no DOM assumptions) |
| Dynamic/variable item heights | Manual (VariableSizeList) | Built-in, automatic | Built-in (`measureElement`) |
| Scroll position on expand/collapse | Manual correction required | Built-in `followOutput` | Stable with `scrollToIndex` + dynamic measurement |
| Maintenance status (2025) | Low — last major release 2020 | Active | Active (v3.13.23, April 2026) |
| API surface | Component-based | Component-based | Hook-based, headless |

**react-window** is effectively unmaintained for new features and does not handle items that change height at runtime (expand/collapse cards) without additional helpers.

**react-virtuoso** is more batteries-included but adds ~15 KB and its component-based API limits customization of the card markup needed for accessibility (`role="option"`, `aria-selected`, custom expand controls).

**TanStack Virtual** wins on flexibility, maintenance, and RSC safety. Its headless hook API means zero DOM opinions — we control the full component tree. The only additional work is implementing keyboard navigation manually (~20 lines), which is standard practice for accessible list implementations.

## Consequences

**Positive:**
- Initial render drops from 500-item DOM to ~15 visible rows + overscan buffer
- Expand/collapse height changes handled via `measureElement` + `ResizeObserver`
- Composable with `useInfiniteQuery` for paginated loads
- No bundler config or Babel changes required

**Negative:**
- Keyboard navigation (`aria-activedescendant`, `onKeyDown`) must be implemented manually
- `scrollToIndex` is unreliable with dynamic heights when scrolling to distant unmeasured items (acceptable — we don't need programmatic long-distance scrolling)
- Upward scroll stutter with dynamic heights is a known upstream bug; mitigation: set `shouldAdjustScrollPositionOnItemSizeChange: () => false` during expand animation, reset after `ResizeObserver` stabilizes

## Patterns Applied

- **Virtual DOM windowing** (React virtualization pattern)
- **Headless component library** pattern — logic separated from presentation
- **ResizeObserver integration** for dynamic height measurement

## Related

- `web-app/src/components/history/HistoryGroupView.tsx` — primary refactor target
- research/stack.md — full library comparison
- pitfalls.md §1 — known TanStack Virtual edge cases and mitigations
