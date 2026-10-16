# Live a11y sweep findings (complements `../audit-ux-component-library/audit.md`)

Method: `tests/e2e/a11y-route-sweep.spec.ts` runs axe (WCAG 2.1 A/AA) on 29 statically-routable pages in the e2e harness (Playwright MCP was unavailable). Parameterized routes (`/resolve`, `/sessions/summary`, `/insights/session-detail`, `/test/*`, `/debug/*`, `/login`, `/analytics/*`) are not swept.

| # | Severity | Finding | Routes | Status | Evidence |
|---|---|---|---|---|---|
| 1 | Critical | Four history filter `<select>`s had no accessible name (axe `select-name`) | `/history` | **Fixed** (aria-labels) | before: `HistoryFilterBar.test.tsx` fails without the change; after: passes. Sweep: `/history` no longer reports select-name |
| 2 | Test infra | `every control in the window tab strip has a distinct accessible name` used removed `page.accessibility` (Playwright 1.56) | e2e | **Fixed** (`ariaSnapshot()`) | before: 2 failures, `TypeError ... 'snapshot'`; after: `accessibility.spec.ts` passes in chromium |
| 3 | Serious | `color-contrast` | 17 routes (see `KNOWN` in the spec) | **Ticketed** (baseline; theme-token work) | sweep |
| 4 | Serious | `document-title` empty | `/backlog`, `/backlog/board` | **Ticketed** | sweep |
| 5 | Critical/Serious | `aria-required-parent/children`, `scrollable-region-focusable` | `/logs`, `/rules` | **Ticketed** | sweep |
| 6 | Serious | `nested-interactive` | `/sessions/new`, `/unfinished` | **Ticketed** | sweep |

The spec fails on any serious/critical violation not in its `KNOWN` baseline, so new regressions are caught while the ticketed debt is explicit (entries may only be removed).
