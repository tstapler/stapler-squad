# ADR-012: react-virtuoso for LogViewer Virtual Scroll

## Status

Accepted

## Context

The new `LogViewer` component (Epic 1, `stapler-squad-logs-mobile` branch) must render up to 10 000 log rows with three characteristics that constrain library choice:

1. **Variable row heights.** FR-5 requires expandable rows. When a row expands, its height changes at runtime after the initial render.
2. **Live tail scroll anchoring.** FR-1 requires the view to follow the bottom of the log stream in live-tail mode and to automatically pause when the user scrolls up.
3. **iOS Safari momentum scroll.** iOS Safari has known interactions between programmatic `scrollTop` assignment and native momentum scroll (pitfalls research, P-05).

No virtual scroll library was installed prior to this feature. A new dependency is required regardless of the choice made.

The project planning research produced two conflicting recommendations:

- `research/stack.md` recommended `@tanstack/virtual` (v3) — headless model, ~4 KB gzipped, clean CSS Grid integration for the sticky horizontal gutter.
- `research/pitfalls.md` recommended `react-virtuoso` — built-in `followOutput` prop, automatic ResizeObserver-based height remeasurement, proven production use in log viewer UIs (Grafana Loki).

The synthesis decision (recorded in full in `project_plans/logs-datadog-ux/decisions/ADR-001-virtualization-library.md`) resolved the conflict in favour of `react-virtuoso`.

## Decision

Use **`react-virtuoso`** (v4.x) as the virtualization library for `VirtualLogList`.

Installed as `react-virtuoso@^4.18.7` in `web-app/package.json`.

## Consequences

### Positive

- **`followOutput` eliminates custom live-tail scroll management.** The `followOutput` prop handles the "following" ↔ "paused" transition without custom event listeners or `requestAnimationFrame` debouncing. This removes the most complex custom scroll logic and directly addresses the highest-severity pitfall (P-03, live tail scroll anchoring).

- **Automatic height remeasurement for expandable rows.** `react-virtuoso` wraps a `ResizeObserver` around every rendered item. When a row expands (FR-5), positions are recalculated reactively without calling `resetAfterIndex`. `@tanstack/virtual` requires manual `measureElement` ref callbacks per row.

- **iOS rubber-band scroll handled by `overscan`.** The `overscan` prop pre-renders rows above and below the viewport, absorbing iOS momentum scroll without requiring manual `offsetTop` correction or `visualViewport` math (P-05).

- **Proven in production log viewer UIs.** `react-virtuoso` is the virtualizer used in the Grafana Loki log viewer — the closest open-source analogue to this feature.

### Negative / Constraints

- **Bundle size.** `react-virtuoso` is approximately 18 KB gzipped; `@tanstack/virtual` is approximately 4 KB gzipped. The 14 KB delta is acceptable within the 5 MB total JS budget in `package.json` `size-limit` config (0.28% of budget).

- **Less layout control.** `react-virtuoso` renders its own wrapper `div`. The headless model of `@tanstack/virtual` gives full DOM control. For the sticky horizontal gutter (FR-2, P-04), the split-column layout approach is still required — `react-virtuoso` does not natively support a sticky left column during horizontal scroll. `LogRow` must implement the fixed-left / horizontally-scrollable-right layout independently of the virtualizer.

- **Horizontal scroll not the virtualizer's concern.** Each `LogRow` manages its own horizontal scroll container. The virtualizer only handles vertical virtualization.

## Alternatives Considered

### `@tanstack/virtual` v3

**Rejected.** More bundle-efficient (4 KB vs 18 KB) but its headless model requires the `LogViewer` to implement live-tail scroll anchoring manually — approximately 50–80 lines of custom scroll management code that `react-virtuoso` provides via `followOutput`. The sticky gutter concern (the primary argument for `@tanstack/virtual` in `research/stack.md`) must use the split-column pattern regardless of virtualizer choice (P-04, iOS Safari `position: sticky` inside `overflow-x: scroll` is broken on iOS Safari ≤ 16).

### `react-window`

**Rejected.** Does not support variable row heights. `VariableSizeList` requires heights declared before render and updated via `resetAfterIndex(i)` — O(n) over rows below `i` and brittle under live-tail append. Low-maintenance status (last substantive release 2023). No equivalent of `followOutput`.

### Custom lightweight virtual scroller (zero new dependency)

**Considered and rejected.** Viable for fixed-height rows only. Expandable rows (FR-5) require per-row height measurement via `ResizeObserver`, which is the core of what `react-virtuoso` provides. Correctly implementing this is 150–200 lines of custom code with edge cases around measurement timing and position recalculation. The maintenance cost outweighs the zero-dependency benefit.

## Revisit Conditions

Revisit this decision if:
- Bundle size constraints tighten below 1 MB total (currently 5 MB).
- `react-virtuoso` releases a breaking change incompatible with the live-tail pattern.
- Expandable rows are constrained to a fixed declared height, making the custom approach viable.

## References

- `project_plans/logs-datadog-ux/decisions/ADR-001-virtualization-library.md` — full research synthesis
- `project_plans/logs-datadog-ux/research/pitfalls.md` — P-01, P-03, P-04, P-05, P-08
- `project_plans/logs-datadog-ux/research/stack.md` — library comparison matrix
- react-virtuoso documentation: https://virtuoso.dev/
- `followOutput` API: https://virtuoso.dev/live-scrolling-appending-items/
