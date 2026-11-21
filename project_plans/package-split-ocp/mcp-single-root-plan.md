# server/mcp single composition root (owner decision Q5, 2026-11-20)

Owner decision: `server/mcp` is NOT a second composition root. It receives everything from `server/dependencies.go` and may not name concrete `session`, `server/services` or `github` types or build its own deps. This file measures the gap and lists the follow-up work. No implementation is in this PR.

Note: `tierc-roadmap.md` has its own Q4/Q5 (SC6 waiver, go-list counts); they are unrelated to this Q4/Q5.

## Measurement (commands run at origin/main 196dc0e74, non-test files in `server/mcp`)

| Measure | Count | Command |
|---|---|---|
| Non-test `.go` files | 42 | `ls server/mcp/*.go \| grep -vc _test.go` |
| Distinct imports from `session*`, `server/services*`, `github` | 13 | `go list -f '{{join .Imports "\n"}}' ./server/mcp \| grep -E 'stapler-squad/(session\|server/services\|github)' \| sort -u \| wc -l` |
| Other internal imports | 6 (`config`, `gen/proto/go/session/v1`, `log`, `pkg/events`, `pkg/localtoken`, `server/wakes`) | same `go list`, minus the 13 |
| Import lines per package (non-test) | `session/domain` 18, `server/services` 16, `session` 14, `session/instance` 13, `session/domain/backlogdto` 9, `session/backlog/pipeline` 6, `github` 5, `backlogsvc` 4, `backlogflow` 4, `scrollback` 3, `reviewgate` 3, `session/git` 2, `session/flow` 1 | grep of import lines |
| Files naming a `session`/`services`/`github`/`instance`/`backlogsvc`/`backlogflow` identifier | 31 of 42 | `grep -cE '\b(session\|services\|githubpkg\|instance\|backlogsvc\|backlogflow)\.[A-Z]' *.go` |

### Who builds the deps: injected, not built

Every long-lived dependency is built outside `server/mcp` and passed in:

| Caller | Where | Passes |
|---|---|---|
| `server/server.go:1263` | `servermcp.NewHTTPHandler(deps.Storage, deps.SessionService, deps.ScrollbackManager, deps.Storage, deps.EventBus, deps.UserPRCache, ...)` | all of the `NewCore` positional deps plus `flowMCPOptions(deps)` |
| `server/backlog_flow_wiring.go:11,18` | `WithFlowService`, `WithPluginRegistry`, `WithTrackerRefParser` | flow service, plugin registry, ref parser |
| `server/wakes_module.go:235,251,258` | `servermcp.SetWakeToolDeps(...)` | wake deps, via a package-level setter |
| `main.go:181` | stdio `RunServer`, deps from `buildMCPDeps()` (`main.go:1940`) and `backlogflow.NewFlowServiceFromStorage(storage)` | built in `main.go`, not in `dependencies.go` |

Constructor calls `server/mcp` makes itself, by `grep -nE '\b(session\|services\|githubpkg\|instance\|backlogsvc\|backlogflow\|pipeline\|reviewgate\|git\|scrollback\|flow)\.New[A-Za-z]*\('` plus `wakes.NewMetrics`:

| Site | Count | What it builds | Verdict |
|---|---|---|---|
| `githubpkg.NewRepoRef` / `NewRepoRefWithHost` in `tools_backlog.go` (2), `tools_backlog_pr.go` (5) | 7 | value object parsed from request data | pure value; no dep built |
| `git.NewGitWorktreeFromStorage` in `tools_vcs.go:193`, `tools_stuck_report.go:32` | 2 | a worktree handle from stored paths | builds its own concrete gateway; belongs behind an injected factory |
| `wakes.NewMetrics()` in `tool_modules.go:200`, `tools_backlog_pr.go:170` | 2 | wake metrics for a `callerIdentityGate` | builds its own dep; second call makes a gate with metrics separate from the module's |
| `&callerIdentityGate{...}`, `&backlogHandlers{...}`, `&goalHandlers{...}`, `&diagnoseHandlers{...}` in `tool_modules.go` | `defaultToolModules`: 9 module families plus 3 assemblers (`newBacklogModules`, `newIdentityGatedModules`, `newGitHubModule`), each returning several modules | handler structs over the injected deps | module assembly, not dep construction; the open question for the follow-up (see below) |
| Package-level calls to concrete GitHub functions (`githubpkg.GetIssue`, `GetPR`, `GetPRByNumber`, `GetCommit`, `GetCurrentUserLogin`) | 6 call sites: `tools_backlog.go` (4: `GetIssue` x2, `GetPR`, `GetCommit`), `tools_backlog_pr.go` (1), `tools_github.go` (1) | live network calls | bypasses the injected `prCache`; needs a consumer-defined port |

`githubpkg.ParseGitHubRefWithHosts` (2 calls in `tools_backlog.go`) is pure parsing, so the 6 direct-network-call figure holds. The guard allow-list must cover all 13 + 6 internal imports above, not only the 13.

Concrete types named in the public surface (`NewCore`, `NewHTTPHandler`, `RunServer`, `coreDeps`): `*services.SessionService`, `*session.Storage`, `session.InstanceStore`, `*scrollback.ScrollbackManager`, `*githubpkg.UserPRCache`, `*backlogsvc.BacklogService`, `*backlogflow.BacklogFlowService`, `*session.PluginRegistry`, `*events.EventBus`. Handler structs also hold `*session.Storage` directly (`tools_peer_help.go`, `tools_goal.go`, `tools_backlog.go`, `tools_diagnose.go`).

Non-constructor package-level calls into `session`, `services`, `instance`, `pipeline`, `scrollback`, `flow`, `backlogsvc` and `backlogflow` are 73 call sites, 37 distinct functions (`grep -hoE '\b(session|services|instance|pipeline|scrollback|flow|backlogsvc|backlogflow)\.[A-Z][A-Za-z]*\('`); they are helper functions (`instance.SubmitContentWithEnter`, `session.ResolveItemLink`, `services.InjectHooksConfig`, `pipeline.SanitizeForAgentContext` at 17 sites). They are not dep construction, but they keep the import edge.

## Reading of the rule

- Already compliant: no long-lived dep (store, service, bus, cache, storage) is constructed in `server/mcp`.
- Gaps against Q5: (1) the stdio fallback builds deps in `main.go`, outside `dependencies.go`; (2) two self-built `wakes.NewMetrics()` and two `git.NewGitWorktreeFromStorage`; (3) direct `githubpkg.Get*` calls; (4) concrete types in the `NewCore` signature and in handler fields; (5) `tool_modules.go:37` calls itself "the one place server/mcp acts as a second composition root", a comment to rewrite once (2)-(4) are fixed.
- Open reading for the owner to confirm in review: whether `defaultToolModules` (module assembly over injected deps) counts as "building its own deps". This plan assumes it does not, and that `server/dependencies.go` only has to hand in the deps and the extra modules via `WithToolModules`.

## Follow-up work (cost classes, no implementation here)

| Step | Cost | Change |
|---|---|---|
| M1 | S | **DONE.** One `*wakes.Metrics` now comes from the composition root via `servermcp.WithWakeMetrics` (`server/mcp/server.go`); `defaultToolModules` builds one `callerIdentityGate` from it (`server/mcp/tool_modules.go`) and shares it with `backlogHandlers.gate`, so both `wakes.NewMetrics()` calls (`tool_modules.go:200`, `tools_backlog_pr.go:170`) are gone. HTTP: `mcpCoreOptions(deps)` (`server/wake_trigger_wiring.go`) at `server/server.go:1263`, backed by `ServerDependencies.sharedWakeMetrics()`, the same instance `registerWakeTriggerService` and `startWakes` read. stdio: `main.go` calls `runStdioMCP`, defined with `stdioMCPOptions` in `mcp_stdio_options.go` (a new file so the 1997-line `main.go` does not grow; the instance is built there until M5 moves it). `sharedWakeMetrics()` is mutex-guarded; call sites are pinned by an AST test in `server/backlog_flow_wiring_test.go` and a behavioural `runStdioMCP` test in `main_test.go`. Absent metrics is a no-op (nil `*wakes.Metrics`). Tests: `server/mcp/wake_metrics_injection_test.go`, `server/backlog_flow_wiring_test.go`, `main_test.go`. |
| M2 | S | **DONE.** The two in-package `git.NewGitWorktreeFromStorage` calls (`tools_vcs.go:193`, `tools_stuck_report.go:32`, both verified on `0b62aee73`) are gone. `server/mcp/worktree_opener.go` defines the consumer-side `Worktree` (`Diff`, `GetBranchName`, plus the stuck report's PR-checker methods; real `*git.GitWorktree` and a fake are its two implementations) and `WorktreeOpener`, injected by `WithWorktreeOpener`. `defaultToolModules` hands it to `vcsHandlers.open` and, via `stuckReportCheckerFactory`, to `backlogHandlers.prCheckerFactory`. Supplier: `server.NewMCPWorktreeOpener()` (`server/wake_trigger_wiring.go`), added to `mcpCoreOptions` (HTTP) and `stdioMCPOptions` (`mcp_stdio_options.go`). Absent opener or a nil result: `get_session_diff`/`list_session_branches` return `cannot open git worktree: ...`, the stuck report prints `unavailable: no worktree for this repo path`; neither panics. Tests: `server/mcp/worktree_opener_test.go`, `server/backlog_flow_wiring_test.go`, `main_test.go`. |
| M3 | M | **DONE.** The 6 direct `githubpkg` network calls (re-verified by grep on the M2 head: `tools_backlog.go` `GetIssue` x2, `GetPR`, `GetCommit`; `tools_backlog_pr.go` `GetCurrentUserLogin`; `tools_github.go` `GetPRByNumber`) now go through the consumer-side `GitHub` port in `server/mcp/github_port.go` (the five methods above; the real github package and a fake are its two implementations), injected by `WithGitHub` and read back by `GitHubFromOptions`. `backlogHandlers.github` is set from `coreConfig` in `newBacklogModules`; `VerifyPRMatchesBranch` now takes the port as a parameter. Absent port: `githubOrUnavailable` returns a port whose calls fail with `ErrGitHubUnavailable` (`import_github_issue` answers `ErrUnavailable`; `report_duplicate`'s ref check, `report_pr_created`'s PR cross-check and login lookup return that error through their existing transient-error paths); no panic, no typed-nil. Supplier: `server.NewMCPGitHub()` (`server/mcp_github.go`), added to `mcpCoreOptions` (HTTP) and `stdioMCPOptions` (stdio). It does not consult the injected `UserPRCache`: the cache holds open PRs without an author (the cross-check needs `Author`) and its cached login may belong to another account than the default token the identity check must use, so it stays out of these calls. `githubpkg.ParseGitHubRefWithHosts`/`NewRepoRef` (pure parsing) stay. Tests: `server/mcp/github_port_test.go` (per-site and `NewCore` behaviour, absent port), `server/backlog_flow_wiring_test.go`, `main_test.go`; the existing httptest-backed tests inject a test-only `httpGitHub`. `report_duplicate` reports an unwired port as permanent (`ErrUnavailable`, "not transient"), not as a retry. The adapter has httptest-backed tests per method plus GHE-host cases (`server/mcp_github_test.go`). Mutation check, 27 mutants all killed: 12 in the wiring and call sites (HTTP and stdio wiring, `newBacklogModules` field, option setter, nil guard, each of the 6 sites, the import unavailable branch) and 15 more (each of the 5 adapter bodies stubbed, each account/host/repo/number/sha argument blanked or shifted, and the `report_duplicate` unavailable branch). A typed-nil port defeats the nil guard; `WithGitHub` documents that callers pass an untyped nil or a value. |
| M4 | M | Replace concrete `NewCore` parameters with consumer-defined narrow interfaces (one per handler family, 2 impls or a real test seam per `interface-pollution-checklist`); `server/dependencies.go` passes the concrete types |
| M5 | S | Move the stdio assembly into a `server` function so `main.go` only calls it: `buildMCPDeps()` and the two inline builds at `main.go:181` (`backlogflow.NewFlowServiceFromStorage(storage)`, `server.NewItemSourceRegistry()`); otherwise Q5 stays violated on the stdio path. `roadmap.md:223`'s "importer set (main.go, server, inttest)" stays true only if `main.go` keeps importing `server/mcp` for `RunServer`. |
| M6 | S | Guard (below), then fix the `tool_modules.go:37` comment |

Cost classes are agent-executed PRs, one per row, each behind `go test ./server/mcp/... ./server/...` and `make ready`. Sequencing: after SRV-wiring (Q4, complete 2026-11-20), because M1-M5 add `CoreOption` wiring to `dependencies.go`.

## Proposed guard (not changed here)

`.golangci.yml` has a `depguard` block and `tools/bzl/scoping/boundaries.txt` mentions `server/mcp/` (line 740), but neither forbids `server/mcp` from importing concrete `session`/`server/services`/`github` packages (it imports 13 today). depguard is import-level only, so it can express "`server/mcp` may import only these packages" once M1-M4 shrink the set, but not "may not call `New*`". Constructor calls need a small analyzer under `tools/lint/` in the style of `norawgitopen`, scoped to `server/mcp/*.go` and run via `make lint-custom`. Add the guard only after M1-M4, with an allow-list that starts at the measured 13 imports and ratchets down.
