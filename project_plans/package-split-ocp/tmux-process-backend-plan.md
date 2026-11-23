# tmux / process-backend extraction out of `session` (plan, 2026-11-23)

Owner decision (2026-11-23): reopen the held tmux/process-backend cluster as the route to `session` under 10,000 non-test LOC (13,981 now). SC3 (at most 5 non-test importers) may be waived for this cluster if needed; SC1 (boundary budget, no new cycle, at most 10 relocated declarations) and isolation still apply, and the design must be justified. This file supersedes the "P4-procbackend (package move)" rejection in [roadmap.md](roadmap.md) ("Deferred or rejected"), whose blockers are now gone (see Findings 1 and 2). Nothing here is implemented; every number below was measured on `origin/main` `02ee5a007` (PR #1769 merged) with the commands shown. Labels: VERIFIED = command run, output seen; INFERRED = reasoned from verified facts, not run.

## Summary

1. **The cluster has zero back-edges into `session`.** VERIFIED. The ranking's "7 back-edges" were forward edges: the held review-queue poller using the cluster. A go/types scan of the 12 linux cluster files finds no use of any symbol declared in another non-test `session` file, and a throwaway copy of the 15 files (windows variants included) compiled as its own package under linux, `GOOS=windows` and `GOOS=darwin`.
2. **Only one staying non-test file uses the cluster** (`review_queue_poller.go`: 4 symbols). VERIFIED. The 6 non-test importers are real, but 5 of them can be removed by re-homing 4 small, unrelated pieces, so SC3 passes with **no waiver** (1 or 2 importers).
3. **The cluster is three independent clusters**, not one: the process backends (2,357 LOC), `PTYDiscovery` (895 LOC, used only by the `test-pty` debug subcommand) and three small helpers (96 LOC). No file edge crosses between them. VERIFIED.
4. **This cluster alone does not reach 10k.** `session` lands at about 10,650 (13,981 minus 3,348 plus about 18 LOC that stay or are added). About 650 more LOC must leave; the cheapest ways are listed in "Reaching 10,000" and need an owner choice.
5. **The hard part is tests, not production code.** About 3,000 LOC of tests share fakes (`mockTmuxManager` and others) with held review-queue tests; they need a test-double package first (PR S3).
6. **Several guards would keep passing while protecting nothing** (10 rows in "Silent-guard inventory"); the worst is `backend_assertion_guard_test.go`.

## 1. Exact file list (VERIFIED)

Command: `ls session/*.go`, `wc -l`, `grep -n go:build`. Non-test LOC; the three `*_windows.go` files are in the all-files total (13,981, 74 files) but not in the `scoping t9`/budget basis (13,871, 71 files; `tools/bzl/scoping/budget.txt:4`, the difference is 110 LOC).

| File | LOC | Build tag | Goes to |
|---|---|---|---|
| `tmux_process_manager.go` | 745 | none | `procbackend` |
| `native_process_manager.go` | 501 | `!windows` | `procbackend` |
| `native_process_manager_windows.go` | 83 | `windows` | `procbackend` |
| `tmux_backend.go` | 254 | none | `procbackend` |
| `backend_tymux.go` | 301 | none | `procbackend` |
| `backend_factory.go` (has `init()` at line 113) | 113 | none | `procbackend` (the `init()` stays in `session`, see 3) |
| `backend_registry.go` | 34 | none | `procbackend` |
| `backend_observability.go` | 187 | none | `procbackend` |
| `pty_subscriber.go` | 115 | `!windows` | `procbackend` |
| `process_terminate.go` / `_windows.go` | 13 / 11 | `!windows` / `windows` | `procbackend` |
| **procbackend subtotal** | **2,357** | | |
| `pty_discovery.go` | 895 | none | `ptydiscovery` (minus `batchPaneActivity`, about 42 lines, which goes to `session/tmux`) |
| `tmux_socket_querier.go` | 57 | none | `session/tmux` (all but `groupInstancesBySocket`, 10 lines, which stays) |
| `process_suspend.go` / `_windows.go` | 23 / 16 | `!windows` / `windows` | `session/importer` |
| **total** | **3,348** | 4 non-test files `!windows` (`native_process_manager.go`, `pty_subscriber.go`, `process_suspend.go`, `process_terminate.go`), 3 `windows` | |

Recount: 2,357 + 895 + 57 + 39 = 3,348 (`wc -l` over the 15 files = 3,348). The ranking's 3,354 differs by 6 LOC (it predates #1769; INFERRED, not chased).

Tests. The cluster's own test files are 8 (`backend_*_test.go` x7 plus `native_process_manager_test.go`, `pty_*_test.go` x2, `tmux_backend_test.go`, `tmux_process_manager_test.go`, `tmux_socket_querier_test.go`); 21 other staying test files also name cluster symbols. Recount: 6,111 LOC across 31 candidate test files (`wc -l`, including `backend_assertion_guard_test.go`, which stays), of which about 1,100 move with no change today (list in S5).

## 2. Dependency graph

### 2.1 Cluster to `session` (back-edges): none (VERIFIED)

Method: a go/types scan (`golang.org/x/tools/go/packages`, `Tests: false`, linux) listing, for every use inside the 12 linux cluster files, objects declared in a `session` file outside the set. Output: empty. Independent proof: copying the 15 files into `session/zz_probe` (package renamed, no other change) and running `go build ./session/zz_probe` succeeds, and so does `GOOS=windows go build` and `GOOS=darwin go build` of the same copy. The probe was deleted afterwards.

Why: the seams are already in `session/instance`, not `session`. `ProcessManager`, `ProcessManagerOptions`, `BackendKeyed`, `ErrUnrecognizedBackend` and the factory hook live in `session/instance/process_manager.go` (hook at line 164: `var ProcessManagerFactoryHook ProcessManagerFactory`; `newProcessManager` at 166 panics if unset); the selection precedence input `instance.GetSelectedBackend()` is `session/instance/backend_resolution.go:67`; `domain.ProcessManagerBackend` and the three constants are in `session/domain`.

Direct module imports of the cluster (12, `go list -f '{{join .Imports "\n"}}'` on the probe): `executor/safeexec`, `internal/qos`, `internal/syncutil`, `log`, `session/claudectl`, `session/domain`, `session/instance`, `session/procinfo`, `session/slowops`, `session/tmux`, `session/tymux`, `telemetry`. `go list -deps` of the probe shows `github` transitively (through `instance`, so no `!github` boundary row; `!server` holds) and no `session`, no `server`.

### 2.2 `session` (staying non-test) to cluster: one file, four symbols (VERIFIED)

Same scan, other direction. Only `review_queue_poller.go` (held REVIEWQ cluster) uses the cluster: `TmuxSocketQuerier`, `TmuxSocketQuerier.ListSessions`, `realTmuxSocketQuerier` (line 204), `groupInstancesBySocket` (line 488), `batchPaneActivity` (lines 668, 1120). The older ranking also listed `slow_call`/`slow_op_recorder`; those moved to `session/slowops` in #1769 and `backend_observability.go` imports the leaf (VERIFIED by the import list above).

Also `tmux_process_manager.go` -> `process_terminate.go` (`terminateProcess`), `native_process_manager.go` -> `pty_subscriber.go`, and the backend files among themselves; `pty_discovery.go`, `process_suspend.go` and `tmux_socket_querier.go` have no edge to any other cluster file (VERIFIED, file-pair scan), which is what makes the three-way split free.

### 2.3 Staying tests to cluster (VERIFIED)

21 staying test files use cluster symbols; 168 use sites in `scoping move --dry-run`'s warning list (`go run ./tools/bzl/scoping move --from session --to session/procbackend --files <12 files> --dry-run` prints only "staying uses moved symbol" warnings and no errors, which agrees with 2.1). A fixpoint over file-level symbol dependencies (my own scan; the same idea as `scoping move`'s warning pass) shows what can move today and what is blocked by shared fixtures:

- Movable with the cluster as is (8 test files, 1,111 LOC): `backend_registry_test.go` 24, `backend_slow_call_test.go` 39, `capture_test.go` 107, `instance_serialization_wiring_test.go` 78, `instance_timestamp_test.go` 258, `native_process_manager_test.go` 280, `pty_subscriber_test.go` 168, `tmux_process_manager_test.go` 157. Plus `pty_discovery_test.go` (725, no staying deps) for `ptydiscovery`.
- Blocked by shared test doubles (the real cost): `mockTmuxManager` (defined in `tmux_backend_test.go`, used 83 times there, and by held tests `review_queue_poller_test.go` x4 and `tmux_socket_querier_test.go`); the `fakeTymuxManagerFor*` family, `stubEnsureDaemonRunning` and `gotCommand` (in `backend_tymux_test.go`, used by 6 files); `fakeSendKeysProcessManager` (used by `pm_lock_wait_test.go`, `redraw_capture_test.go`); `makeTestInstance`/`NewTestInstance` (defined in `instance_claude_test.go`/`comprehensive_session_creation_test.go`, used by unrelated staying tests). INFERRED: after S3 extracts the doubles, about 3,000 more test LOC move and about 1,600 stay in `session` (`instance_identity_seam_test.go` 314, `instance_tymux_identity_test.go` 123, `instance_backend_migration_test.go` 89, `instance_serialization_backend_test.go` 142, `comprehensive_session_creation_test.go` 950 because they need `createTestEntRepository`/`createTestSession`/`testTmuxSocket`). A staying `package session` test may import the leaf (the leaf never imports `session`), so "36 in-package tests cannot import a leaf that imports session" (roadmap, P4) no longer applies.

### 2.4 Importers outside `session` (VERIFIED)

Method: go/types over `./...` with tests; uses of `session` objects declared in the cluster files, grouped by user package.

| Importer (non-test) | Symbols used | Disposition |
|---|---|---|
| `main.go` | `NewPTYDiscovery` and its methods (only inside the `test-pty` cobra command, `main.go:668-736`) | imports `session/ptydiscovery` (composition root, allowed). Or delete the command, Q1 |
| `session/external/external_discovery.go:183` | `NewProcessManager` (its only `session` use; `rg -o 'session\.[A-Z]\w+' session/external` gives 1) | imports `procbackend.NewProcessManager` directly; it drops its `session` import |
| `session/importer` (5 files) | `ResumeOriginalProcess` x3, `SuspendOriginalProcess`, `TmuxSocketQuerier` | suspend/resume move into `importer`; querier goes to `session/tmux` |
| `server/services/importsvc/import_service.go` | `NewRealTmuxSocketQuerier`, `TmuxSocketQuerier` (test also `SuspendOriginalProcess`) | `session/tmux`; test uses `importer.SuspendOriginalProcess` (it already imports `importer`) |
| `session/autonomous/health.go` | `TmuxSocketQuerier`, `NewRealTmuxSocketQuerier`, `IsServerDown` | `session/tmux` |
| `session/driver/session_driver.go:304` | `CapturePaneCacheTTL` | const lowered to `session/tmux` (`driver` already imports `paneactivity` and `tmux`) |

Test-only importers after the move (no SC3 weight): `server/services` (`liveness_consolidation_test.go`: `SetTymuxManagerFactoryForTest`), `server/services/wsconn` (4 tests: `NewTymuxBackend`), `session/sweeper`, `session/autonomous`, `session/driver`, `session/external` tests (`NewTmuxBackend`, `TmuxManager`, `NativeProcessManager`).

## 3. Backend selection wiring and the target shape

### Today (VERIFIED)

1. `Instance.pm()` (`session/instance/instance_tmux.go:336`) lazily builds the manager through `newProcessManager` (`process_manager.go:166`), which calls `ProcessManagerFactoryHook` (a package variable).
2. `session/backend_factory.go:113` assigns `instance.ProcessManagerFactoryHook = NewProcessManager` in `init()`, so any binary that links `session` has the hook.
3. `NewProcessManager` resolves `opts.Backend`, then `instance.GetSelectedBackend()` (live config, tymux flag), then the caller's default, then tmux (`backend_factory.go:28-43`), and `buildBackend` (`backend_registry.go:28-34`) looks the key up in `backendRegistry` (`backend_registry.go:17-21`, a map literal of the three backends) or returns `ErrUnrecognizedBackend`.
4. `server/dependencies*.go` never names a backend: `rg` finds no non-test use of the cluster outside the six importers above. Session-driver code never names one either (`session/driver` uses only the TTL const).

So the OCP registry already exists in place (BACKEND-registry in the roadmap; the kind-enum switches are gone). A new backend is already "a new file plus one registry entry"; the move does not change that, it makes the extension surface a package.

### Recommended layout

One leaf for the backends, one for PTY discovery, three lowered helpers:

```
session/procbackend         tmux_backend, tmux_process_manager, backend_tymux, native_process_manager (+windows),
                            pty_subscriber, process_terminate (+windows), backend_factory, backend_registry,
                            backend_observability              2,357 LOC, 11 files
session/ptydiscovery        pty_discovery (~850 LOC)           importer: main
session/tmux                + SocketQuerier, NewRealSocketQuerier, BatchWindowActivity, CapturePaneCacheTTL (~100 LOC)
session/importer            + SuspendOriginalProcess, ResumeOriginalProcess (+windows stub) (39 LOC)
session/internal/procbackendtest   MockTmuxManager, fake tymux managers, fake send-keys PM (test-only)
```

Why one backend package and not `tmuxbackend`/`nativebackend` sub-leaves: the three backends share `backend_observability.go` (`withBackendOperationSpan`, `timedBackendOp`, 23 edges from `tmux_backend.go` and `backend_tymux.go`), the registry names all of them, and the tests share fakes. Sub-leaves would add packages and cross imports with no new extension point (a new transport is still one file and one registry line). Rejected: speculative split (`interface-pollution-checklist`). Revisit only if a fourth backend needs a dependency the others must not link.

Rejected: leaving `backend_registry.go` in `session` and moving only the implementations. The registry names the concrete constructors, so `session` would keep importing all three and the leaf would be imported by the hub for nothing.

### Ports for the back-edges

There is none to design: zero back-edges (2.1). The forward edge from the held poller is removed by lowering, not by a port: `TmuxSocketQuerier` and `batchPaneActivity` go to `session/tmux`, which `review_queue_poller.go` already imports; `groupInstancesBySocket` (10 lines, depends on `*instance.Instance`) stays in `session`. This keeps SC1 for `procbackend` at 1 declaration (see 5).

Deviation to flag: `TmuxSocketQuerier` is consumed by four packages and implemented by one real type plus test fakes, so it is a legitimate seam, but putting it in the producer package (`tmux`) rather than each consumer breaks "define interfaces where consumed". Alternative with the same SC result: each consumer defines its own 1-2 method interface (importer needs only `ListSessions`). Recommended: producer-side in `tmux` (4 call sites, one shared fake in tests); the consumer-side form is a 10-line change per consumer if reviewers prefer it.

### Where `init()` registration moves

Keep process-wide registration in `session`, as `history_wiring.go` does for `historyadapters` (VERIFIED: `session/history_wiring.go` blank-imports the leaf; `session/history_wiring_test.go` pins it):

- `session/backend_wiring.go` (new, about 8 LOC): `func init() { instance.ProcessManagerFactoryHook = procbackend.NewProcessManager }`. The leaf has no `init()`, so a test binary can install a different factory.
- `TestProcessManagerHook_RegisteredWhenSessionLinked` (new, in `session`): fails if the hook is nil or not `procbackend.NewProcessManager`. Mutation: delete the file, the test fails and so does any test that builds an Instance.
- Every package that stops linking `session` loses the hook. Only `session/external` does (INFERRED from the per-package `session.` use counts) (it drops its `session` import) and it calls `procbackend.NewProcessManager` directly, so it needs no hook; its tests that build Instances need `TestMain` setting the real hook (not `InstallHermeticHooksForTest`, which installs a no-op manager and is wrong for tests that exercise tmux fakes). This is the `session/tagpoller/main_test.go` lesson, reversed: tagpoller needed the hermetic no-op; leaf tests of real backends need the real hook.
- `procbackend`'s own tests: `TestMain` = `instance.ProcessManagerFactoryHook = NewProcessManager` + `os.Exit(sessiontest.Main(m))`. `sessiontest.Main` (`session/internal/sessiontest/main.go`) supplies the keyring mock, non-loopback network denial, `claude` CLI isolation, ambient-env clearing and `tmuxreap`; a new test binary without it silently loses all five (the existing leaf `main_test.go` files in `importer`, `autonomous`, `historyadapters`, `workflowengine` call it).

Explicit composition-root registration (`server/dependencies.go` and `main.go` call `procbackend.Install()`) is the purist form and is **not** recommended now: every test binary that builds an Instance through a package other than `session` would panic with "ProcessManagerFactory not registered" (`process_manager.go:168`). INFERRED count: `server/services` (and `wsconn`, `inttest`, `svctest`) would each need a `TestMain`. Same trade-off the historyadapters move accepted; revisit when `session` itself is a thin hub.

### GOOS-tagged files

- `scoping move` is blind to build tags and GOOS files (leaf-packages.md rule 2). It failed on my first dry run with `file .../native_process_manager_windows.go not found in any loaded package`, so the windows files must be moved by hand with `git mv` plus a package-clause edit, in the same commit, and `rg` for the moved symbols in `*_windows.go`, `*_darwin.go` and `//go:build` files before and after.
- The pairs that must stay paired in `procbackend`: `native_process_manager.go` (`!windows`) with `native_process_manager_windows.go`; `pty_subscriber.go` (`!windows`, no windows twin: the windows `NativeProcessManager` stub does not use it); `process_terminate{,_windows}.go`. In `session/importer`: `process_suspend{,_windows}.go`.
- Verified today: the copied cluster builds for linux, windows and darwin. After each PR run `GOOS=windows go build ./session/... ` and `GOOS=darwin go build ./session/...` (both pass on `origin/main` for `./session`, `./session/tmux`, `./session/importer`; VERIFIED). `go vet` under `GOOS=windows` already fails on main for an unrelated reason (`session/internal/sessiontest` imports `testutil/tmuxreap`, which excludes windows), so use `go build`, not `go vet`, as the windows check.
- Bazel: `session/BUILD.bazel` lists the `*_windows.go` files in plain `srcs` (lines 39, 47, 49) and relies on rules_go build-constraint filtering, with `select` only for deps (line 146); Gazelle will do the same for the new packages. Windows is cross-compile only (`tools/bzl/windows/BUILD.bazel`, `manual`); the new packages enter that closure automatically through `//:stapler-squad_windows_amd64`. `session/tmux/embed/BUILD.bazel` (hand-written, `tools/bzl/hand-build-files.txt`) belongs to `session/tmux`'s bundled-tmux embed and is not touched, because no cluster file embeds (VERIFIED: the only `go:build` lines in the 15 files are `!windows`/`windows`).

## 4. Silent-guard inventory

Each row says what would stay green while no longer protecting, and what to do. VERIFIED unless marked.

| # | Guard | Path/name dependence | Failure mode | Action |
|---|---|---|---|---|
| 1 | `session/backend_assertion_guard_test.go:17-35` | globs `session/*.go`, regex `\.\(\*(TmuxBackend\|TymuxBackend)\)` (unqualified) | after the move, `x.(*procbackend.TmuxBackend)` in a `session` file does not match, and the exempt `own` list names files no longer in the glob: the test stays green forever | rewrite to walk `session/**` non-test files, regex with optional qualifier `(\w+\.)?`, exempt only `session/procbackend`; add a non-vacuous check (the scan saw more than N files) and a mutation (plant a qualified assertion) |
| 2 | `server/services/terminal_write_callers_test.go:212-213` | `exemptFiles` by basename (`tmux_process_manager.go`, `native_process_manager*.go`, `tmux_backend.go`, `backend_tymux.go`); `exemptPkgs` by path | keeps working only if basenames are kept; a rename makes the moved PTY writers show up as unpinned callers (loud, not silent) | `git mv` without renaming; add `procbackend` to `exemptPkgs` as well; run `go test ./server/services -run TerminalWrite` |
| 3 | `session/driver/pane_gate_wiring_test.go:17,24` | reads `session_driver.go` source for the literal `paneactivity.NewDriverPreview(inst, session.CapturePaneCacheTTL)`; `t.Skip`s under the Bazel sandbox | after lowering the const the literal changes; if the edit is partial the test fails natively but is skipped under Bazel | update both lines to `tmux.CapturePaneCacheTTL`; the guard stays native-only (existing limit) |
| 4 | `tools/lint/*` | analyzers are type- and import-path based (`norawexec` exempts `/executor`, `/executor/safeexec`, `/tools/bzl/`; `norawgitcli` and `tmuxsocketscope` match `session/tmux` by suffix; `nosessiondrivertest` matches `session/driver`; `noarchivedrevival` lists six `session/*.go` files, none in the cluster) | none depend on `session/<cluster file>`; `tmuxsocketscope` testdata only mentions `pty_discovery.go` in a comment (`testdata/src/a/a.go:38`) | none; run `make lint-custom` |
| 5 | `.golangci.yml` | `no_server_in_core` (`**/session/**`) and `^session/` exclusions (forbidigo `log\.`, `ent\.IsNotFound`, gochecknoglobals) cover new `session/*` dirs by prefix; `exhaustive` exclusion rows `^session/[^dl]` etc. cover `procbackend`/`ptydiscovery` (first letter `p`); `session_core_off_itemsources` is `**/session/*.go` only | new leaves inherit the loose rules; **no `isolate_` row exists unless added**, so a later `procbackend` import of `session` would not fail depguard | add `isolate_procbackend` and `isolate_ptydiscovery` strict allow-lists (`list-mode: strict`, every real non-test import, nothing unused) |
| 6 | Scoping budget and boundaries | `tools/bzl/scoping/budget.txt` rows are exact caps; `boundaries.txt` rows `!server` | a missing row means the ratchet does not cover the new package; the `session` row must drop (lowering needs no raise line) | add `session/procbackend files=11 loc=2357` (imports from `scoping budget --init session/procbackend`), `session/ptydiscovery`, `boundaries.txt`: `session/procbackend !server`, `session/ptydiscovery !server` (`!github` does not hold: `instance` imports `github`); lower `session`; `go run ./tools/bzl/scoping budget --base origin/main` prints nothing |
| 7 | Bazel | `session/BUILD.bazel` lists the files and test files explicitly; `//session:session_test` and `//server/services:services_test` `data` list leaf `go_sources` one by one (leaf-packages.md rule 6) | scanner tests that read source files see an empty or stale tree under Bazel | `go run ./tools/bzl/scandata -fix .`, `make gazelle` (no diff), `bazel build --nobuild //...` |
| 8 | `tmux_exec_total{caller}` and `tmux_exec_site_total{site}` | label values are Go function names (`executor/safeexec/tmuxspawn.go:20-24`), bounded to 64 | after the move labels read `procbackend.(*TmuxProcessManager)...`, `ptydiscovery.(*PTYDiscovery)...`; `docs/observability/grafana/dashboards/tmux-spawns.json` has no `session.` literal (`rg` returns nothing), so no panel breaks | note it in the PR body; the 64-label cap is not at risk (names are replaced, not added) |
| 9 | goroutine-stack and dump filters | `session/internal/svctest`, `executor/safeexec/tmuxspawn_test.go:44-47` only use `session.` names as string fixtures of `shortFunc`; no cluster function name is matched (`rg` over the repo) | the #1769 slowops move needed its own dump-filter rewrite; this move does not | re-run `rg 'session\.\(\*(Tmux\|Native\|Tymux\|PTY)'` after each PR |
| 10 | doc and comment path references | `docs/explanation/package-layout.md:70,90` ("registry in `session/backend_registry.go`"), `docs/reference/bundling-tymuxd.md:5,140,329`, `docs/explanation/polling-audit.md:127,131`, `docs/how-to/enable-opentelemetry.md:45`, `session/tymux/*.go` comments | stale paths, not failures | `rg` and update in S5; `make docs-coverage` |

Live-service impact: none functional. The move changes no persisted field (`Instance.Backend` and `TymuxSessionID` keep their serialization), no metric name (instruments `session_backend_operation_duration_ms`, `terminal_slow_op_total`, `terminal_slow_call_total` are string literals, unchanged), and no tmux argument. `make install-service` restarts the unit with `--tmux-keep-server` (CLAUDE.md note, #180), so live tmux sessions survive; the plan never runs it for trial builds. Tests that touch real tmux must keep a private socket: the cluster's tests use fakes and a fake `TMUX_BIN` script (`pty_discovery_test.go:640-653`), `testTmuxSocket(t)` (`session_creation_test.go`) for the real-socket ones, and `STAPLER_SQUAD_TMUX_SOCKET` for manual runs. Embedded/bundled tmux: `embed_tmux`/`embed_tymux` tags live in `session/tmux` and `session/tymux`, which do not move.

## 5. SC1 / SC3 / isolation (what `scoping criteria` would say)

| Leaf | SC1 relocated (declarations staying code still uses) | SC1 cycle | SC3 non-test importers outside `session` | Isolation |
|---|---|---|---|---|
| `session/tmux` additions (S1) | 3 lowered decls (`SocketQuerier`, real impl, `CapturePaneCacheTTL`) + 1 function, all into a lower package | none (`tmux` imports nothing of `session`) | n/a (lower package) | `tmux` row unchanged |
| `session/importer` addition (S2) | 2 | none | n/a | none |
| `session/ptydiscovery` (S4) | 0 (nothing staying uses it after S1) | none | 1 (`main`) | `isolate_ptydiscovery`: `$gostd`, `executor/safeexec`, `internal/syncutil`, `log`, `session/claudectl`, `session/domain`, `session/instance`, `session/tmux` |
| `session/procbackend` (S5) | 1 (`NewProcessManager`, used by the new `session/backend_wiring.go`) | none (2.1) | 1 (`session/external`) plus the hub; tests excluded | `isolate_procbackend`: take the allow-list from `go list` at PR time (INFERRED: the 12 imports in 2.1 minus `claudectl`, and probably `syncutil`, which only `pty_discovery.go` uses) |

`go run ./tools/bzl/scoping criteria` is the authority; run it per PR with the real numbers above (`--relocated N --uses N --importers a,b`). Before the lowering PRs the cluster would have failed SC3 (6 > 5); after S1-S2 it passes with no waiver, so the owner-approved SC3 waiver is **not needed** for this cluster. If a reviewer rejects the lowering (for example wants `TmuxSocketQuerier` to stay in `session`), the fallback is the waiver, stated precisely: "SC3 6 > 5 for `session/procbackend` (importers: `main`, `importsvc`, `autonomous`, `driver`, `external`, `importer`), waived by owner decision 2026-11-23 to bring `session` under 10k LOC."

## 6. Staged PR sequence

Each PR is one cluster, independently green, with the leaf-packages "Verify before you push" checks (whole-tree `golangci-lint --enable-only=depguard,unused,ineffassign,staticcheck`, whole-package `gocognit,gocyclo`, `scoping budget`, `make gazelle` clean, `-v` RUN/PASS counts for any `-run`). Open PRs at most 3 at once (ADR-010 Decision 4).

| PR | Title (Conventional) | Files | Session delta | Depends on | Risk |
|---|---|---|---|---|---|
| S1 | `refactor: lower tmux socket querier, window-activity batch and capture TTL into session/tmux` | `tmux_socket_querier.go` (47 of 57 lines) and `batchPaneActivity` (~42 lines) into `session/tmux`; `CapturePaneCacheTTL` const; `groupInstancesBySocket` stays in `review_queue_poller.go`; rewrites in `review_queue_poller.go`, `importer`, `importsvc`, `autonomous`, `driver`; tests `tmux_socket_querier_test.go`, `pane_activity_test.go` | about -90 | none | low |
| S2 | `refactor: move original-process suspend/resume into session/importer` | `process_suspend{,_windows}.go` -> `importer`; 6 call sites | -39 | none (parallel with S1) | low; windows stub must move |
| S3 | `refactor: extract tmux and tymux process-manager test doubles into session/internal/procbackendtest` | `mockTmuxManager`, `fakeTymuxManagerFor*`, `stubEnsureDaemonRunning` (as `SetEnsureDaemonRunningForTest(tb, fn)`), `fakeSendKeysProcessManager`, `fakeLivenessProcessManager` (`fake_liveness_process_manager_test.go:12`); exported fields; codemod over about 12 test files | 0 non-test | S1 (so the poller tests are already on `tmux`) | medium, large mechanical test diff (about 600-900 changed lines, INFERRED) |
| S4 | `refactor: move PTY discovery into session/ptydiscovery` (or delete, Q1) | `pty_discovery.go` (~853) + `pty_discovery_test.go` (725); `main.go` import; budget/boundaries/depguard rows | about -853 | S1 | low; `goleak` and `TMUX_BIN` tests are non-parallel, keep that |
| S5 | `refactor: move process backends into session/procbackend` | 11 files (2,357) + moved tests (8 now-movable files about 1,111 LOC, plus about 3,000 after S3); `session/backend_wiring.go`; guard rewrites 1-3; docs (`package-layout.md`, `CLAUDE.md`, `bundling-tymuxd.md`); `external` import | about -2,357 + 8 | S1, S3 (so `batchPaneActivity` is gone and fakes are exported); S4 is independent | high: `init()`/hook, GOOS files, guards, test moves |

Expected `session` LOC: 13,981 now (13,871 on the budget basis) -> S1 about 13,891 -> S2 about 13,852 -> S4 about 12,999 -> S5 about 10,650 (budget basis about 10,640; files 71 -> 60). Arithmetic: 13,981 - 3,348 + 10 (`groupInstancesBySocket`) + 8 (`backend_wiring.go`) = 10,651.

Per-PR test plan:
- S1: `go test ./session ./session/tmux ./session/importer ./session/autonomous ./session/driver ./server/services/importsvc -race`; mutation: change the `tmux.BatchWindowActivity` format to `#{window_active}` and see `pane_activity_test` fail; remove the driver TTL literal and see `pane_gate_wiring_test` fail.
- S2: `go test ./session/importer ./server/services/importsvc`; `GOOS=windows go build ./session/importer`; mutation: make `SuspendOriginalProcess` return nil and see the importer reconcile tests fail (INFERRED that they cover it; confirm in the PR).
- S3: pure refactor, whole `session` test package unchanged in count (`go test ./session -v | grep -c '^--- PASS'` before and after must match).
- S4: `go test ./session/ptydiscovery -race`; `go run . test-pty` against the manual instance (below).
- S5: `go test ./session/... ./server/... -race`; the guard mutations (a) delete the `BackendTymux` registry entry, `TestBuildBackend...` fails; (b) delete `session/backend_wiring.go`, the wiring test fails; (c) plant `x.(*procbackend.TmuxBackend)` in `session/instance_*.go`, the rewritten assertion guard fails; (d) swap `TmuxBackend`/`TymuxBackend` keys in the registry, `instance_tymux_identity_test`/`backend_registry_test` fail. Windows/darwin: `GOOS=windows go build ./...`, `GOOS=darwin go build ./...`. Bazel: `tools/bazel build //session/... //server/...` per `docs/how-to/build-with-bazel.md`, and `bash tools/bzl/check-go-toolchain.sh` is untouched (no go.mod change).
- Live-socket-isolated manual run (S4 and S5; copy of the CLAUDE.md recipe, instance #1 of the manual port block):

```bash
mkdir -p ~/.stapler-squad/manual-builds/manual-1
go build -o ~/.stapler-squad/manual-builds/manual-1/stapler-squad .
ss -ltnp | grep -E '62871|62872'   # must print nothing
PORT=62871 STAPLER_SQUAD_INSTANCE=claude-manual-test STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-1 \
  ~/.stapler-squad/manual-builds/manual-1/stapler-squad --tmux-keep-server &
# browser http://localhost:62871: create a session on each backend (tmux default; tymux via the `tymux` flag; native via
# config.ProcessManagerBackend=native in the instance's config.json), type, resize, pause/resume, restart the process and
# confirm sessions resume (instance_serialization path); grep the log for "Using private tmux socket".
# S4: ~/.stapler-squad/manual-builds/manual-1/stapler-squad test-pty  (uses the instance's own state only)
kill %1; tmux -L ssq-manual-1 kill-server
```

Never `make install-service` for this, and never run the manual binary without `STAPLER_SQUAD_TMUX_SOCKET` (without it the manual instance shares the live tmux server and its leaked-control-mode sweeper can kill live control clients).

Rollback: each PR is a `git revert` (no data migration, no flag, no persisted format). S5 revert restores `backend_factory.go`'s `init()`. If a regression appears only in the live service, `make install-service` from the reverted main (owner-run); tmux sessions survive the restart.

## Reaching 10,000

This cluster alone ends at about 10,651, 651 over. Candidates still in `session` (sizes from `wc -l` today):

| Candidate | LOC | Result | Cost / blocker |
|---|---|---|---|
| HELD PRGATE (`pr_cleanup_gate*`, `pr_legacy_safe`, `pr_live_work_*`) | 684 | 9,967 (33 under) | SC3 6 > 5; one back-edge (`workSessionNewer` in `backlog_item_prs.go`) |
| HELD BLPLUG (`backlog_plugin`, `backlog_sync`, `feature_controller`) | 815 | 9,836 | SC3 6 > 5; 7 back-edges (`Storage` x5) |
| HELD REVIEWQ (poller, determiner, suppression, options, queue, diff budget, spawn guard, startup scanner) | 2,144 | 8,507 | SC3 8 > 5; its back-edges to this cluster vanish after S1 (4 symbols); remaining 1 (`Storage.UpdateInstanceLastAddedToQueue`) plus its test fixtures |
| READY leftovers (backlog helpers 311, `session_pr_mirror_reconciler` 141, tiny group 175, `ent_repository_integration_credential` 105) | 732 | 9,919 (81 under) | no SC3 problem; grab-bag cohesion; test traps noted in `leaf-candidates.md` |

(All four are INFERRED from the ranking plus a fresh `wc -l`; the ranking's own back-edge columns for the tmux cluster were wrong, so these should be re-measured with the same type scan before each PR.) Deleting PTYDiscovery instead of moving it (Q1) changes nothing here: either way the 895 LOC leave `session`.

## Open questions for the owner

1. **Delete the `test-pty` subcommand and `PTYDiscovery` (about 850 production + 725 test LOC), or keep it as `session/ptydiscovery`?** VERIFIED: its only caller is the `test-pty` command in `main.go:668-736`, documented in `README.md:260` as "Test PTY initialization and discovery". Moving costs a package and a depguard row for a debug command; deleting costs the command. The plan defaults to moving.
2. **Which route closes the last 651 LOC?** Options are in the table above: PRGATE alone clears 10k by 33 LOC (needs an SC3 waiver; margin is thin), the READY leftovers by 81 LOC (no waiver, grab-bag leaves), or REVIEWQ (waiver, large margin, biggest PR). This needs a decision because every route either takes a waiver or accepts cohesion cost; the code does not decide it.

Decisions this plan took that need no answer but are reversible: `init()` stays in `session` (hub wiring, historyadapters precedent) instead of an explicit composition-root call; `TmuxSocketQuerier` goes producer-side into `session/tmux`; one `procbackend` package instead of per-backend sub-leaves.

## Method notes

Reproduce: `make sync` (generates `gen/` and `session/ent`, plus a stub `server/web/dist/index.html`); `go run ./tools/bzl/scoping move --from session --to session/procbackend --files <the 12 linux files> --dry-run`; a `go/packages` scan (Tests true and false) of `github.com/tstapler/stapler-squad/session` and `./...`, comparing the declaring file of each used object with the cluster file set; the probe copy under `session/zz_probe` built for linux, `GOOS=windows`, `GOOS=darwin`. The scan and probe were throwaway and are not committed. Disk gate before starting: `/home` 143 GB free, btrfs metadata 83.98% used.
