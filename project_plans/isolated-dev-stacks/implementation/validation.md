# Validation Plan: isolated-dev-stacks

Date: 2026-07-04

## Happy Path Scenario

Given the Baseline (backend defaults to `:8543`, `next dev` hardcoded to `3001`, no free-port bookkeeping, no cross-process CORS trust), when a developer runs `make dev-stack NAME=my-feature-test` from a worktree, then a `StackLauncher` allocates two free ports, starts a `BackendChild` bound to a real OS-assigned port and a `FrontendChild` (`next dev`) on a second OS-assigned port wired to the backend via `NEXT_PUBLIC_API_URL`, both report ready, a banner prints both URLs plus the literal phrase "NOT the systemd instance," and the developer can open the printed frontend URL in a browser and successfully exercise a ConnectRPC-backed feature (e.g. backlog) — without ever setting `PORT`, `STAPLER_SQUAD_INSTANCE`, or `NEXT_PUBLIC_API_URL` by hand, and without the systemd-managed instance on `:8543` being touched.

Everything below — CORS validation, MCP/hook URL laziness, orphan reconciliation, bounded timeouts — is a variation or a failure-mode guard on this one flow, not an equal-priority feature in its own right.

## Requirement → Test Mapping

Requirement IDs follow requirements.md's Scope → In Scope list (REQ-1 through REQ-6). Each maps to the plan.md Story/Epic that implements it. (Phase 4 "list running stacks" was cut from plan.md as out-of-scope scope creep — see plan.md's "Out of Scope (deferred during planning)" note and requirements.md's Out of Scope section — so there is no REQ-7 in this revision; the requirement-to-test mapping below covers exactly the 6 numbered Scope bullets, plus the four "hardest-won fix" call-outs that plan.md's repair history singled out as needing dedicated regression coverage.)

**Summary counts:** 6 of 6 requirements (REQ-1 through REQ-6) have direct test coverage below — 100% of the numbered Scope bullets. Total test cases across all tables in this document: 36 (REQ-1: 6, REQ-2: 4, REQ-3: 6, REQ-4: 5, REQ-5: 5, hardest-won fix #3: 3, hardest-won fix #4: 4, REQ-6: 3).

### REQ-1: Dynamic port/config resolution (backend `net.Listen`/`PORT=0`; frontend `$PORT`) — Epic 1.1 (Story 1.1.1), Epic 2.2 (Story 2.2.1)

| # | Test | File | Test name | Type | Case |
|---|---|---|---|---|---|
| 1 | Backend binds OS-assigned port | `server/server_test.go` | `Start_should_BindOSAssignedPort_And_UpdateAddr_When_ListenAddressEndsInZero` | Unit | Happy path |
| 2 | Backend keeps explicit port (no regression) | `server/server_test.go` | `Start_should_BindExplicitPort_When_PortIsAlreadyKnown` | Unit | Happy path (regression guard for `test-server.ts`'s explicit-port path) |
| 3 | Backend surfaces bind failure | `server/server_test.go` | `Start_should_ReturnListenError_When_RequestedPortIsAlreadyBound` | Unit | Error path (pre-bind a listener on the target port, assert `Start()` returns the `net.Listen` error, not a panic or silent retry) |
| 4 | Real bind + health check end-to-end | `server/server_integration_test.go` | `Server_should_ServeHealthCheck_When_StartedWithPortZero` | Integration | Real `net.Listen("tcp","localhost:0")`, real `Serve(ln)`, real `http.Get(addr+"/health")` → 200, asserting the resolved port is non-zero |
| 5 | `next dev` honors `$PORT` | `web-app/package.json` scripts, exercised via the launcher | — | Integration | Covered by the Epic 3.2 launcher integration test below (Story 3.2.1's AC already requires `FrontendChild` to bind the allocated port); a standalone Jest unit test is not meaningful for a shell script string, so the package.json script content is asserted with a plain string-equality unit check instead |
| 6 | `dev` script content unit check | `web-app/package.json` | n/a (no test framework covers `package.json` scripts) | Unit | Happy path — a `grep`-style assertion in CI (`Makefile` or a tiny `scripts/dev-stack/*.test.ts` check) that `"dev"` contains `${PORT:-3001}` — cheap regression guard against someone re-hardcoding `3001` |

### REQ-2: Automatic free port allocation (`PortAllocation`, `AllocatedPortSet`, `BindRetry`) — Epic 3.1 (Story 3.1.1)

| # | Test | File | Test name | Type | Case |
|---|---|---|---|---|---|
| 1 | Two allocations in one run never collide | `scripts/dev-stack/ports.test.ts` | `allocatePort() should return distinct ports when called twice back-to-back in the same process` | Integration (real OS sockets, no mocks) | Happy path |
| 2 | `release()` actually frees the OS socket | `scripts/dev-stack/ports.test.ts` | `release() should free the underlying socket so a real server can bind the same port afterward` | Integration | Happy path |
| 3 | Retry on bind failure | `scripts/dev-stack/ports.test.ts` | `allocatePort() should retry with a fresh listener when the bind throws EADDRINUSE, and succeed within 3 attempts` | Unit (mocked `net.createServer`) | Error path recovered |
| 4 | Retry exhaustion throws | `scripts/dev-stack/ports.test.ts` | `allocatePort() should throw a clear error when every retry attempt fails` | Unit (mocked `net.createServer`, always throws) | Error path (unrecoverable) |

### REQ-3 (hardest-won fix #1): MCP/hook URLs resolve lazily against the real bound address — Epic 1.1 (Task 1.1.1c), Epic 1.3 (Story 1.3.1)

This is the lazy `baseURLFn`/`GetAddr()` fix called out explicitly in the assignment. The regression this guards against: baking `"http://"+srv.addr+"/mcp"` (or a hook URL) into a struct field at *construction* time — before `Start()` resolves a `PORT=0` listener — permanently freezing it at `http://localhost:0/...`.

| # | Test | File | Test name | Type | Case |
|---|---|---|---|---|---|
| 1 | `mcpServerURLFn` re-reads the real address, not a construction-time snapshot | `server/services/session_service_test.go` | `SessionService_should_ReturnUpdatedPort_When_MCPServerURLFnInvokedAfterAddrChanges` | Unit | Happy path — set `mcpServerURLFn` to a closure over a mutable `*string`, mutate the string after construction (simulating `Start()`'s post-bind reassignment of `s.addr`), call the 3 read sites, assert all 3 observe the new value, not the value at construction time |
| 2 | Nil-checked read doesn't panic before the fn is wired | `server/services/session_service_test.go` | `SessionService_should_ReturnEmptyString_When_MCPServerURLFnNotYetConfigured` | Unit | Error path — construct without calling `SetMCPServerURL`, assert the 3 read sites return `""`/a safe default instead of a nil-pointer panic |
| 3 | Hook URLs re-read lazily, not baked at hook-injector construction | `server/services/hook_injector_test.go` | `hookEndpoints_should_ReflectCurrentBaseURLFn_When_CalledTwiceWithDifferentAddresses` | Unit | Happy path — call `hookEndpoints(fn)` once with `fn` returning `http://localhost:0`, then again with `fn` returning `http://localhost:54211`; assert the second call's map contains the new address (proving it's rebuilt per-call, not cached) |
| 4 | `hookApprovalURL` reads `baseURLFn` at point of use | `server/services/approval_handler_test.go` | `ApprovalHandler_should_UseBaseURLFnValueAtCallTime_When_ThreeUsageSitesInvoked` | Unit | Happy path — stub `baseURLFn` to return different values on successive calls, assert all 3 known usage sites (lines ~671/710/734 per plan.md) reflect the value current at *their* call time |
| 5 | End-to-end regression AC: `PORT=0` never leaks into a session's hooks or MCP URL | `server/server_integration_test.go` | `Server_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated` | Integration | Happy path — this is the literal AC from Story 1.3.1: start the real server with `PORT=0`, create a session, read the generated `.claude/settings.local.json`, assert the `PermissionRequest` hook URL and `mcpServerURLFn()`'s output both contain the real OS-assigned port and **never** contain `:0` |
| 6 | Regression: systemd-style explicit port unchanged | `server/server_integration_test.go` | `Server_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` | Integration | Error/regression path — explicit, non-zero port obtained via `findFreePort` (no `PORT=0`, and never the real production port), assert hook URL is still `http://localhost:<port>/...`, proving Epic 1.3 didn't regress the default instance |

### REQ-4 (hardest-won fix #2 target — CORS): `STAPLER_SQUAD_EXTRA_ORIGINS` validation — Epic 1.2 (Story 1.2.1)

| # | Test | File | Test name | Type | Case |
|---|---|---|---|---|---|
| 1 | Valid localhost origins accepted | `main_test.go` (or extracted `config`/`cors`-validation helper's own `_test.go`) | `parseExtraOrigins_should_AcceptEntry_When_GivenWellFormedHttpLocalhostOrigin` | Unit | Happy path — table test over `http://localhost:54212`, `https://127.0.0.1:9999`, asserting each is appended |
| 2 | Malformed/unsafe entries rejected with a warning, not silently dropped or included | `main_test.go` | `parseExtraOrigins_should_RejectAndLogWarning_When_GivenMalformedEntry` | Unit | Error path — table test over `not-a-valid-origin`, `http://*`, `http://localhost:1234/path`, `http://example.com:1234` (non-localhost host), `http://localhost` (missing port); assert each is excluded from the returned slice AND a warning log line is emitted naming the offending entry |
| 3 | Mixed valid+invalid input in one env var (Story 1.2.1's literal AC) | `main_test.go` | `parseExtraOrigins_should_AcceptValidEntry_And_RejectInvalidEntry_When_BothPresentInOneCommaSeparatedList` | Unit | Happy + error combined — input `http://localhost:54212,not-a-valid-origin`, assert exactly `["http://localhost:54212"]` returned and one warning logged for the second entry |
| 4 | Real cross-origin request succeeds once extra origin is trusted | `server/server_integration_test.go` | `Server_should_AllowCrossOriginRequest_When_ExtraOriginConfiguredViaEnvVar` | Integration | Happy path — start real server with `STAPLER_SQUAD_EXTRA_ORIGINS=http://localhost:54212`, issue a real HTTP request with `Origin: http://localhost:54212`, assert `Access-Control-Allow-Origin` reflects it and the request is not rejected |
| 5 | Unset env var is a no-op (regression) | `server/server_integration_test.go` | `Server_should_KeepSingleOriginAllowlist_When_ExtraOriginsEnvVarUnset` | Integration | Regression path — no env var set, assert `srv.GetOrigins()` is unchanged from today's single-origin behavior |

### REQ-5: CLI entry point to spin up a named isolated stack (`StackLauncher`) — Epic 3.2 (Story 3.2.1)

| # | Test | File | Test name | Type | Case |
|---|---|---|---|---|---|
| 1 | Config assembly picks distinct ports and doesn't swap them | `scripts/dev-stack/launch.test.ts` | `startDevStack() should assemble a DevStackConfig with distinct backendPort and frontendPort when given an instance name` | Unit (mocked `allocatePort`/`spawn`) | Happy path |
| 2 | Missing instance name rejected at the CLI boundary | `scripts/dev-stack/launch.test.ts` | `main() should exit with a usage error when no instance name is passed as argv[2]` | Unit | Error path |
| 3 | Import has zero ambient side effects (Architecture Blocker 2 regression guard) | `scripts/dev-stack/launch.test.ts` | `importing launch.ts should not register any SIGINT/SIGTERM listeners on the current process` | Unit | Error path / regression — assert `process.listenerCount('SIGINT')` and `('SIGTERM')` are unchanged immediately after `import { startDevStack } from './launch'`, before `main()` ever runs. This is what makes it safe for Epic 5.1's `global-setup.dev-mode.ts` to import the module inside the Playwright runner process. |
| 4 | Real spawn + process-group teardown | `scripts/dev-stack/launch.test.ts` | `startDevStack() should terminate both children's process groups within 5s when teardown() is invoked` | Integration | Happy path — per Task 3.2.1h option (b): spawn two real dummy long-lived children (`spawn('sleep', ['300'], { detached: true })`) standing in for `BackendChild`/`FrontendChild`, call `teardown()`, assert (via `process.kill(pid, 0)` throwing `ESRCH`) both are gone, and that no `SIGKILL` was needed (graceful `SIGTERM` exit within the grace window) |
| 5 | Manifest round-trips the two child PIDs, not the launcher's own | `scripts/dev-stack/launch.test.ts` | `startDevStack() should write backendPid and frontendPid (not its own pid) into dev-stack.json once both children are ready` | Integration | Happy path — real filesystem write to a temp `~/.stapler-squad/instances/<name>/dev-stack.json`-shaped path, read back, assert `backendPid`/`frontendPid` match the two spawned children's actual PIDs and `schemaVersion: 2` is present |

### Hardest-won fix #3: bounded frontend-readiness poll kills the already-started backend on timeout — Epic 3.2 (Tasks 3.2.1c/d)

| # | Test | File | Test name | Type | Case |
|---|---|---|---|---|---|
| 1 | Frontend never becoming ready kills the backend and rejects, doesn't hang | `scripts/dev-stack/launch.test.ts` | `startDevStack() should kill the already-started BackendChild and reject when the FrontendChild readiness poll times out` | Integration | Error path — spawn a real dummy `BackendChild` stand-in that opens its port immediately (so the backend leg "succeeds"), but inject a `FrontendChild` stand-in that never opens its port (or mock the frontend poll function to always report not-ready); with the poll's max-attempts/interval injected as short test-only values (not the real ~90s ceiling — see note below), assert: (a) the returned promise rejects, (b) the backend stand-in's PID is confirmed dead via `process.kill(pid, 0)` throwing `ESRCH` after teardown, (c) no orphaned process remains |
| 2 | Happy path counterpart: both ready before timeout resolves cleanly | `scripts/dev-stack/launch.test.ts` | `startDevStack() should resolve backendUrl and frontendUrl when both children become ready before the timeout` | Integration | Happy path — both dummy stand-ins open their ports promptly; assert the promise resolves with both URLs and no teardown is triggered |
| 3 | Poll bound is real and finite (not accidentally infinite) | `scripts/dev-stack/ports.test.ts` or `launch.test.ts` | `waitForReady() should throw after maxAttempts is exhausted rather than polling forever` | Unit | Error path — mock the readiness check to always return false, assert the function throws once `maxAttempts` (parameterized, default 90 per `TestServer.waitForServer()`'s precedent) is reached, and that it does so within a bounded wall-clock time in the test (use fake timers or an injected small `maxAttempts` for test speed) |

**Testability note:** Task 3.2.1c's real ceiling (~90s, mirroring `TestServer.waitForServer()`) is too slow to exercise directly in a unit/integration test on every CI run. The poll interval and max-attempts should be constructor/parameter-injectable (not hardcoded module constants) specifically so tests #1–#3 above can run in milliseconds with a small injected bound, while the real CLI/Playwright-harness callers use the production ~90s default. This is a testability requirement worth calling out to the implementer now, before Task 3.2.1c is written, rather than discovering it's untestable after the fact.

### Hardest-won fix #4: orphan-reconciliation sweep using `backendPid`/`frontendPid` — Epic 3.2 (Task 3.2.1g), mixed alive/dead PID scenario

This is Adversarial Blocker 1 from the plan's repair loop: a test that only encodes "kill the launcher's own PID" would pass while leaving both real children running. The tests below assert against `backendPid`/`frontendPid` specifically, never the manifest's `pid` field.

| # | Test | File | Test name | Type | Case |
|---|---|---|---|---|---|
| 1 | Mixed alive/dead PIDs: reap only the live one, leave the dead one alone, then proceed | `scripts/dev-stack/launch.test.ts` | `startDevStack() should reap only the alive PID and skip the already-dead PID when a stale manifest records one of each` | Integration | Happy path (the exact scenario named in the assignment) — write a fixture `dev-stack.json` with `backendPid` = a real, currently-alive detached child's PID (e.g. `spawn('sleep',['300'],{detached:true})`) and `frontendPid` = a PID confirmed dead (spawn-then-wait-for-exit, or a PID reserved and released beforehand); invoke `startDevStack(name)` again for the same instance; assert: `process.kill(-backendPid, 'SIGTERM')` was sent (real signal, real process group), the backend PID is confirmed `ESRCH` within 5s, the frontend PID's dead status required no action beyond the initial liveness probe, the stale manifest is deleted only *after* the alive PID is confirmed reaped, and fresh port allocation/spawn proceeds without an `EADDRINUSE`-style failure |
| 2 | Escalation to SIGKILL when SIGTERM is ignored | `scripts/dev-stack/launch.test.ts` | `startDevStack() reconciliation sweep should escalate to SIGKILL when the orphaned process ignores SIGTERM for 5s` | Integration | Error/edge path — spawn a real child that traps `SIGTERM` (`spawn('sh',['-c','trap "" TERM; sleep 300'])`) as the fixture's `backendPid`; assert `SIGTERM` is sent first, the process is still alive after the grace window, `SIGKILL` follows, and the process is confirmed gone afterward |
| 3 | Anti-regression: sweep must target `backendPid`/`frontendPid`, never the manifest's own `pid` | `scripts/dev-stack/launch.test.ts` | `startDevStack() reconciliation sweep should signal backendPid and frontendPid independently and must never signal the manifest's own pid field` | Unit (mocked `process.kill`) | Error path (regression guard) — fixture manifest with three *distinct* values for `pid`, `backendPid`, `frontendPid` (all fake/mocked, no real spawn needed here); assert `process.kill` is called with `-backendPid` and `-frontendPid` and assert it is **never** called with `-pid` (the launcher's own recorded PID) at any point during the sweep — this is the test explicitly demanded by Task 3.2.1h to guard against Adversarial Blocker 1 reappearing |
| 4 | Both-dead case: no signals sent, stale manifest still cleaned up | `scripts/dev-stack/launch.test.ts` | `startDevStack() reconciliation sweep should skip signaling entirely and just delete the manifest when both PIDs are already dead` | Unit/Integration | Happy path (no-op case) — both `backendPid`/`frontendPid` confirmed dead via `process.kill(pid, 0)` → `ESRCH`; assert no `SIGTERM`/`SIGKILL` calls occur, and the stale manifest is still removed before fresh allocation proceeds |

### REQ-6: Playwright dev-mode harness + at least one e2e spec proving it end-to-end — Epic 5.1 (Story 5.1.1), Epic 5.2 (Story 5.2.1)

| # | Test | File | Test name | Type | Case |
|---|---|---|---|---|---|
| 1 | `global-setup.dev-mode.ts` sets `TEST_SERVER_URL` to the frontend, not the backend | `tests/e2e/global-setup.dev-mode.test.ts` | `global-setup.dev-mode should set process.env.TEST_SERVER_URL to the frontend URL returned by startDevStack, not the backend URL` | Unit (mocked `startDevStack`) | Happy path |
| 2 | Setup failure propagates loudly | `tests/e2e/global-setup.dev-mode.test.ts` | `global-setup.dev-mode should rethrow when startDevStack rejects, rather than swallowing the error and leaving TEST_SERVER_URL unset` | Unit (mocked `startDevStack` rejecting) | Error path |
| 3 | End-to-end proof: real dual-process stack serves backlog across the origin boundary | `tests/e2e/backlog.dev-mode.spec.ts` | `describe('backlog-dev-mode') > 'should display a seeded backlog item when navigating to the backlog page'` | E2E (Playwright, real `next dev` + real dynamically-ported backend, per `playwright.dev-mode.config.ts`) | Happy path — this is the literal success-metric proof from requirements.md: a real browser, a real `next dev` origin, and a real ConnectRPC call across the dynamic-port boundary, reusing `backlog.spec.ts`'s existing locator/seeding so it's a parity check, not new coverage |

## Test Stack

- **Go unit/integration**: Go's stdlib `testing` + `github.com/stretchr/testify` (already a `go.mod` dependency, already the convention in `server/services/*_test.go`, e.g. `approval_handler_integration_test.go` — new integration tests follow that existing `_integration_test.go` naming split between unit and integration within `server/`).
- **TypeScript/Node unit + integration** (`scripts/dev-stack/`): Jest + `ts-jest` (already a `web-app/package.json` devDependency: `jest@^30.2.0`, `ts-jest@^29.4.11`). `scripts/dev-stack/` lives outside `web-app/`'s existing Jest scope, and there is **no root `package.json`** in this repo (confirmed — only `web-app/package.json` hosts `jest`/`test:e2e` scripts today). This is resolved, not deferred: per plan.md's Task 3.2.1h, `scripts/dev-stack/**/*.test.ts` (both `ports.test.ts` and `launch.test.ts`) is added to `web-app/package.json`'s EXISTING Jest configuration (its `roots`/`testMatch`, or a `jest.config.js` `projects` entry pointed at `../scripts/dev-stack`), so `cd web-app && npx jest` — the exact command root `CLAUDE.md` already documents (`cd web-app && npx jest --no-coverage`) — picks up both files with no new root `package.json` and no new CI step. The new tests run under whatever CI step already invokes `cd web-app && npx jest`.
- **E2E**: Playwright (already used by `tests/e2e/`), extended with the new opt-in `playwright.dev-mode.config.ts` / `test:e2e:dev-mode` script — separate from the default `test:e2e` run per requirements' explicit non-goal of migrating the whole suite.

## UX Acceptance Tests

N/A — pure infrastructure/dev tooling with no end-user UI surface (per `project_plans/isolated-dev-stacks/design/ux.md` not existing, and per this project's own framing in requirements.md and plan.md Step 0.5: "an infrastructure/dev-tooling problem ... not a multi-actor business domain").

## E2E / Dev-Mode Harness (in place of UX Acceptance Tests)

The new opt-in `test:e2e:dev-mode` Playwright run (Epic 5.1/5.2) stands in for a UX acceptance-test section here. What it proves that unit/integration tests below it cannot: a **real** browser loading a **real** `next dev` process, making a **real** cross-origin ConnectRPC call to a **real**, dynamically-ported Go backend — i.e., that Phase 1's CORS/hook/MCP fixes, Phase 2's API-base-URL override, and Phase 3's launcher actually compose correctly end-to-end, not just in isolation. `backlog.dev-mode.spec.ts` (REQ-6, test #3 above) is the concrete instance of this proof.

## Migration Plan Test Coverage

N/A — plan.md has no Migration Plan section (no schema/data-format changes: `dev-stack.json` is new-and-additive, not a migration of existing data; the systemd-managed instance never reads it). Per Step 5 of this validation task, this is stated explicitly rather than silently omitted.

## Coverage Targets and How to Measure

| Suite | Command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with 100% of the new/changed files in scope (`server/server.go`'s listener change, `session_service.go`'s `mcpServerURLFn`, `approval_handler.go`/`hook_injector.go`'s `baseURLFn`, `main.go`'s origin-validation block) |
| TypeScript/Jest (`scripts/dev-stack/`) | `cd web-app && npx jest ../scripts/dev-stack --coverage --coverageThreshold='{"global":{"lines":80}}'` (runs via `web-app/package.json`'s existing Jest config per the Test Stack section above) | ≥80% line |
| Playwright (default suite) | `cd tests/e2e && npm test` | No regression — untouched by this feature except for the new opt-in config living alongside it |
| Playwright (dev-mode, opt-in) | `npm run test:e2e:dev-mode` | 100% pass for `backlog.dev-mode.spec.ts`; not counted toward the default suite's coverage numbers |

- All public service methods touched by this feature (`Start()`, `SetMCPServerURL`, `allocatePort`, `startDevStack`): happy path + error path covered per the tables above.
- All external integrations (subprocess spawn — `BackendChild`/`FrontendChild`; port bind — `net.Listen`/`net.createServer`; HTTP health check — `/health` poll; file I/O — `dev-stack.json`): unit-mocked coverage **and** at least one real (unmocked) integration test each, per the tables above.
- Migration: N/A — no schema/data changes in this feature (see above).
