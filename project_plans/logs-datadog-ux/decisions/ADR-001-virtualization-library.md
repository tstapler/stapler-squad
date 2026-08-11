# ADR-001: Virtualization Library for LogViewer

## Status

Accepted

## Context

The new `LogViewer` component must render up to 10 000 log rows with three characteristics that constrain library choice:

1. **Variable row heights.** FR-5 requires expandable rows. When a row expands, its height changes at runtime after the initial render. Any library that uses a fixed height or requires pre-declaring heights before render is incompatible.

2. **Live tail scroll anchoring.** FR-1 requires the view to follow the bottom of the log stream while in live-tail mode and to automatically pause scrolling when the user scrolls up. This is the most complex scroll behavior in the component: it requires a clean "following" ↔ "paused" state transition with no visible jump when new rows are appended.

3. **iOS Safari momentum scroll.** The mobile target (iOS Safari) has known interactions between programmatic `scrollTop` assignment and native momentum scroll. Setting `scrollTop` during a momentum animation causes the view to appear to "bounce back" away from the bottom (pitfalls research, P-05).

No virtual scroll library is currently installed. The project's `package.json` has no `react-window`, `@tanstack/virtual`, or `react-virtuoso`. A new dependency is required regardless of choice.

The two research artifacts produced conflicting recommendations:

- `research/stack.md` recommended `@tanstack/virtual` (v3) on the basis of its headless model, small bundle (~4 KB gzipped), and a requirement for a sticky horizontal gutter via CSS Grid.
- `research/pitfalls.md` recommended `react-virtuoso` for its built-in `followOutput` prop, automatic ResizeObserver-based height remeasurement, and proven production use in log viewer UIs (Grafana Loki).

The architecture research (`research/architecture.md`) noted that hand-rolling live tail scroll management is approximately 50 lines of custom code citing `@tanstack/virtual`'s `scrollToIndex` + scroll event approach, and that the custom path is feasible but adds maintenance surface.

## Decision

Use **`react-virtuoso`** (v4.x) as the virtualization library for `VirtualLogList`.

## Consequences

### Positive

- **`followOutput` eliminates custom live-tail scroll management.** `react-virtuoso`'s `followOutput` prop handles the "following" ↔ "paused" transition automatically, including the correct detection of user-initiated scroll-up. This removes the most complex custom scroll logic from the project and directly addresses the highest-severity pitfall (P-03).

- **Automatic height remeasurement for expandable rows.** `react-virtuoso` wraps a `ResizeObserver` around every rendered item and updates its internal position map reactively. When a row expands (FR-5), the scroller recalculates all positions below that row without any explicit callback. `@tanstack/virtual` requires manual `measureElement` ref callbacks wired per row.

- **iOS rubber-band scroll is handled by `overscan`.** The `overscan` prop pre-renders rows above and below the visible window, absorbing iOS momentum scroll without requiring manual `offsetTop` correction or `visualViewport` math. This addresses P-05 at the library level.

- **Proven in production log viewer UIs.** `react-virtuoso` is used in Grafana Loki's log viewer — the closest open-source analogue to the feature being built.

### Negative / Constraints

- **Bundle size increase.** `react-virtuoso` is approximately 18 KB gzipped. `@tanstack/virtual` is approximately 4 KB gzipped. The 14 KB delta is acceptable within the 5 MB total JS budget specified in `package.json` `size-limit` config, but it is non-zero.

- **Less layout control.** `react-virtuoso` renders its own wrapper `div` and scroll container. The headless model of `@tanstack/virtual` (renders nothing, positions via `transform`) gives full DOM control. For the sticky horizontal gutter (FR-2), this means the split-column layout approach (pitfalls research, P-04) is still required — `react-virtuoso` does not natively support a sticky left column during horizontal scroll. The `LogRow` component must implement the fixed-left / horizontally-scrollable-right split independently of the virtualizer.

- **Horizontal scroll is not the virtualizer's concern.** Each `LogRow` manages its own horizontal scroll container. The virtualizer only handles vertical virtualization.

## Alternatives Considered

### `@tanstack/virtual` v3

**Rejected.** While `@tanstack/virtual` is more powerful and more bundle-efficient (4 KB vs 18 KB), its headless model requires the `LogViewer` to implement live-tail scroll anchoring manually. The "following" ↔ "paused" state transition is the most complex behavior in the component. `@tanstack/virtual` has no equivalent of `followOutput` — the developer must wire `scrollToIndex(-1)` on each append, detect user-initiated scroll-up via an `isScrolling` observer, and debounce scroll events with `requestAnimationFrame` to avoid fighting iOS momentum scroll. This is approximately 50–80 lines of custom scroll management code that `react-virtuoso` provides out of the box. The additional code is a maintenance liability, not a performance gain — the CPU cost of the `followOutput` implementation is negligible compared to DOM reflow.

The stack research recommendation for `@tanstack/virtual` prioritized the sticky horizontal gutter concern (FR-2), arguing that the headless model makes CSS Grid layout simpler. However, the pitfalls research (P-04) established that `position: sticky` inside `overflow-x: scroll` is broken on iOS Safari ≤ 16 regardless of which virtualizer is used. The sticky gutter must be implemented as a split-column layout in both cases — so the headless advantage does not materialize.

### `react-window`

**Rejected.** `react-window` does not support variable row heights in the way this project requires. `FixedSizeList` requires a uniform height for all rows. `VariableSizeList` requires heights to be declared before render and updated via `resetAfterIndex(i)` when row `i` changes height — this is O(n) over rows below `i` in the worst case and brittle under live-tail append. Expandable rows (FR-5) changing height after render are incompatible with this model.

Additionally, `react-window` is in low-maintenance status (last substantive release 2023) and has no equivalent of `followOutput`.

### Custom lightweight virtual scroller (no new library)

**Considered but not chosen.** The architecture research proposed a custom implementation: a spacer `div` for total height, index arithmetic for the visible window, and `transform: translateY` for positioning. This works for fixed-height rows. However:

- Variable heights (FR-5, expandable rows) require per-row height measurement via `ResizeObserver`, which is the core of what `react-virtuoso` provides. Implementing this correctly is non-trivial — at least 150–200 lines of custom code with edge cases around measurement timing and position recalculation.
- The custom approach was proposed under the assumption of fixed row heights (density toggle controls a constant `rowHeight`). Expandable rows invalidate this assumption.

The custom approach remains viable only if expandable rows are always fixed-height (e.g., pre-declared panel height). If that constraint is accepted, it should be revisited. For now, expandable row height is dynamic (user can expand JSON fields, long stack traces, etc.).

## References

- `project_plans/logs-datadog-ux/research/stack.md` — comparison matrix and `@tanstack/virtual` recommendation
- `project_plans/logs-datadog-ux/research/pitfalls.md` — P-01 (dynamic heights), P-03 (live tail anchoring), P-04 (iOS sticky), P-05 (iOS momentum scroll), P-08 (row expansion cache invalidation)
- `project_plans/logs-datadog-ux/research/architecture.md` — component tree, `VirtualLogList` placement, state model
- react-virtuoso docs: https://virtuoso.dev/
- `followOutput` API: https://virtuoso.dev/live-scrolling-appending-items/
