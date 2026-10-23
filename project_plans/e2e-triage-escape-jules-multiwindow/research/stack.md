# Stack research

- Playwright (`@playwright/test` pinned in `tests/e2e/package.json`, run with `bunx playwright test`, projects `chromium` and `chromium-dom`). `page.routeWebSocket(url, handler)` intercepts WebSocket traffic; `page.route` does not.
- App stack: React SPA; watch streams (`WatchBacklogItems`, `WatchSessions`, ...) go over `ws://…/api/session.v1.<Svc>/<Rpc>`; unary RPCs go over HTTP Connect.
- `tests/e2e` needs `bun install` once in that dir (it had no `node_modules` in this worktree).
- Test server: `tests/e2e/helpers/test-server.ts` spawns the binary with `STAPLER_SQUAD_INSTANCE=e2e-local`, which gates `/api/debug/backlog/*` seed routes (`server/server.go:1186`). `ensureBinary` runs `make stapler-squad`.
