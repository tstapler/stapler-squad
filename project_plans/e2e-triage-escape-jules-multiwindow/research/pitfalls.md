# Pitfalls

Verified by reproduction (run against a freshly built binary, merged with main at e548f1837's parent):

1. **Stale binary hides routes (Jules seed).** The reported `Unexpected token '<', "<!doctype"` from `seedJulesWorkSessionDirect` did NOT reproduce after `make build`: the route `/api/debug/backlog/seed-jules-work-session` is registered (`server/services/backlog_debug_seed_handler.go:121`). The a11y and contrast tests passed on the fresh binary. An older binary lacking that route falls through to the SPA's HTML. Not a code defect.
2. **Always-present assertive live region.** `web-app/src/components/ui/Announcer.tsx:46` renders `<div role="alert" data-testid="announcer-assertive">` permanently. Locators `[role="alert"]:not(#__next-route-announcer__)` in `tests/e2e/pages/EscapeAnalyticsGlobalViewPage.ts` and `tests/e2e/multi-window.spec.ts` now match two elements (strict-mode violation / count 1 instead of 0).
3. **Fixture drift (analytics).** `EscapeAnalyticsPage.tsx:296` renders the fleet card from `correlationOutcomes`, not `totalSequences` (field added by #763). The spec's mocked response omits `correlationOutcomes`/`captureHealthy`, so the card shows `0` outcomes and a capture-health banner (also `role="alert"`) appears.
4. **WebSocket bypasses HTTP route gating (Jules badge).** Playwright `page.route` held `ListBacklogItems`/`GetBacklogItem`/`WatchBacklogItems` HTTP calls, yet the badge rendered immediately. Probe showed `WS ws://…/WatchBacklogItems` opened and delivered the snapshot. The gate must also cover the WebSocket via `page.routeWebSocket`.
5. Weakening risk: do not delete assertions; only exclude the announcer and fix the mocks.
