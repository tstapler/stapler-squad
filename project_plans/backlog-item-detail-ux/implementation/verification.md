# Verification record — backlog-item-detail-ux

**Date**: 2026-10-12

## Real-browser run (AC1, AC2, AC5-10)

`tests/e2e/backlog-item-detail-redesign.spec.ts`, Playwright `chromium` project, isolated
test server, binary built at `01e7e0445` (`make build`): **8/8 pass**.

| AC | Test |
|---|---|
| 1 | shows the Lifecycle Summary with zero prior clicks |
| 2 | reveals the synthetic session row's TriageReviewPanel readOnly summary when expanded |
| 5 | shows a PR link in the sticky header without scrolling or expanding anything |
| 6 | hides the header PR badge for pr_pending |
| 7 | renders no PR badge when the item has no prUrl |
| 8 | puts the actions in view at the top of the scroll area, above Version Control |
| 9, 10 | auto-expands Version Control for an archived item with a PR; a manual collapse survives a real reload |

## Dedup audit (AC4) — `BacklogItemDetail.tsx`

| Information | Where it renders | Duplicates |
|---|---|---|
| Title | header `h2` | none (the other occurrences are aria-labels / session titles) |
| Lifecycle status | `StageTracker` in `LifecycleSummary` | none; standalone status badge removed (D1) |
| Stuck reason | `BlockerChip` in `LifecycleSummary` | none; `GateBlockingSection` shows the gate checklist, not the stuck reason |
| PR link, `pr_pending` | `PullRequestSection` only | header badge and VCS link suppressed |
| PR link, other statuses | header badge (glance) + Version Control | required by AC5/AC9; VCS falls back to `item.prUrl` only when the widget has no `github` block |

## AC3

`web-app/src/components/backlog/detail/crossSurfaceConsistency.test.tsx`: card and detail
use the same `getStatusLabel` and one `BlockerChip` for the same `stuckItem`.
