# Validation (Complexity 1: one test per requirement)

| Req (requirements.md) | Test | Command / evidence |
|---|---|---|
| AC1 | `escape-analytics-global-view.spec.ts` › ShowAggregateSummary_When_TabSwitched | `bunx playwright test escape-analytics-global-view.spec.ts` |
| AC2 | same spec › ShowErrorBanner_When_RpcFails | same |
| AC3 | `jules-status-badge.spec.ts` › accessible name test | `bunx playwright test jules-status-badge.spec.ts` |
| AC4 | same › 4.5:1 contrast test | same (seed via `global-setup`, `e2e-local` instance) |
| AC5 | same › no badge renders before first Jules state (plus socket-intercepted sanity assertion) | same |
| AC6 | `multi-window.spec.ts` › renaming to empty reverts silently... | `bunx playwright test multi-window.spec.ts` |
| AC7 | all above on `chromium` and `chromium-dom`, fresh `make build` binary | full-run output pasted in the final summary |
| AC8 | no assertion weakened | `git diff` review in sdd:6-verify |
| Plan 1.5a | baseline JSON no longer lists fixed tests | `git diff tests/e2e/baseline` |
| Plan 1.5b | no other spec breaks on announcer | run any spec with a changed locator |

## Readiness gate
1 every requirement has a test: PASS. 2 no TODO/TBD in plan: PASS. 3 ADRs: N/A (none referenced). 4 adversarial blockers: PASS (0). 5 architecture review: N/A (skipped at complexity 1). 6 schema change: N/A. 7 pre-mortem P1: N/A (skipped at complexity 1). 8 architecture read: PASS (test-only edits, no packages).
**Verdict: PASS.**
