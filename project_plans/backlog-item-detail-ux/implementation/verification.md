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

## Phase 3 verification (2026-10-17)

- `pnpm jest components/backlog`: 81 suites, 871 tests passed. `tsc --noEmit` clean.
- Playwright `tests/e2e/backlog-item-detail-redesign.spec.ts --project=chromium`: 9 passed (1.9m), including "opens a review item with Description visible, criteria expanded and group labels shown, with no interaction". Run against system `/usr/bin/google-chrome-stable` through a temporary uncommitted config override, because Playwright's own `chromium_headless_shell-1194` extraction left only `v8_context_snapshot.bin` (cause undiagnosed).
- Reproduce on a machine with Playwright Chromium: `cd tests/e2e && npx playwright test backlog-item-detail-redesign.spec.ts --project=chromium`.
- Exploratory-testing findings and the alternatives comparison are in `research/phase3-exploratory-audit.md` (chosen: Alternative A).

### Re-run on the committed config, no overrides (2026-10-18)

`playwright install` stalls after `pw:install extracting archive` (download completes, 109 MB). Downloading the same zip with `curl` and extracting with `unzip` into `~/.cache/ms-playwright/chromium_headless_shell-1194` works (`headless_shell --version` → Chromium 141.0.7390.37). Why Playwright's own extractor stalls is not diagnosed.

`cd tests/e2e && npx playwright test backlog-item-detail-redesign.spec.ts --project=chromium` with the unmodified `playwright.config.ts`: **9 passed (2.0m)**, including the review-item test for criterion 8.
