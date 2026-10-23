# Requirements: e2e triage — escape-analytics-global-view, jules-status-badge, multi-window

Backlog item fb352a5d-f19a-4f67-a7df-95a9232769ef (GitHub issue #1135). The item has no
acceptance criteria; the criteria below are derived from its description.

## Problem
Five-plus Playwright tests fail identically on `WEB_BUILD=next` and `WEB_BUILD=bun`, so they are
test/app drift, not a Bun-migration regression. Each needs a root cause, not a symptom patch.

## Derived acceptance criteria
1. `escape-analytics-global-view.spec.ts` › `ShowAggregateSummary_When_TabSwitched` passes, with root cause stated.
2. `escape-analytics-global-view.spec.ts` › `ShowErrorBanner_When_RpcFails` passes, with root cause stated.
3. `jules-status-badge.spec.ts` › accessible-name test passes.
4. `jules-status-badge.spec.ts` › 4.5:1 contrast test passes (the `<!doctype` JSON error from `seedJulesWorkSessionDirect` is explained).
5. `jules-status-badge.spec.ts` › `no badge renders before the first Jules state arrives` passes.
6. `multi-window.spec.ts` › `renaming to empty reverts silently...` passes.
7. All of the above pass on both the `chromium` and `chromium-dom` projects, run against a freshly built binary.
8. No test is weakened: an assertion is only changed when the app behaviour it encodes legitimately changed.

## Out of scope
Bun-side e2e assertions (PR 7b); unrelated flaky tests.
