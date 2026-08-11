# Research: Stack Surface for the Hook-URL/MCP-URL `-race` CI Flake

Scope: technology-level facts needed to choose between "raise timeout," "narrow
`-race` scope," "isolate as a separate job," and "make the pipeline itself
faster" for `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`
and `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
in `server/server_integration_test.go`.

---

## 1. Go race detector: documented overhead and interaction with real-subprocess timing

Source: [Data Race Detector — go.dev](https://go.dev/doc/articles/race_detector.html), [Introducing the Go Race Detector — go.dev/blog](https://blog.golang.org/race-detector).

- **Documented overhead**: memory usage may increase **5-10x**, execution time
  **2-20x**, "varies depending on the program." This is instrumentation
  overhead on every memory access/goroutine event, not a fixed tax — it scales
  with how much concurrent/shared-memory activity a test actually exercises.
- **A specific, easy-to-miss cost**: the race detector allocates an extra 8
  bytes per `defer`/`recover`, and *these allocations are not recovered until
  the goroutine exits*. Long-lived goroutines that defer/recover in a loop can
  see unbounded-looking memory growth that never shows up in
  `runtime.ReadMemStats`/`pprof` — not directly relevant to this bug (the two
  flaky tests are short-lived), but relevant if a *different* long-running
  goroutine in the same `-race` process (e.g. a poller) is holding memory that
  pressures the runner.
- **Official guidance for handling it**: the same doc gives the canonical
  pattern for excluding specific tests that "fail under the race detector due
  to timeouts" or "take too long under the race detector" — a `//go:build
  !race` tag on the test file, not a bare timeout bump. This is directly
  relevant to acceptance criterion #3 (any `-race` scope narrowing must be
  explicit and justified) — it's the sanctioned mechanism if this repo decides
  to go that route for these two tests.
- **Interaction with real-subprocess timing (this repo's specific case)**: the
  race detector does not slow down the tmux subprocess itself (that's a
  separate OS process, unaffected by Go's instrumentation) — but it **does**
  slow down every goroutine in the *test binary* competing for CPU while that
  subprocess is warming up (event polling, JSON parsing/marshaling in
  `InjectHookConfig`, the actor-model `sendSyncErr` dispatch in
  `Instance.Start`). Under `-race`, those all take longer walltime per unit of
  CPU work, and on a CPU-constrained runner (see §2) that pushes the
  filesystem-write side of `waitForPermissionRequestHookCommand`'s poll loop
  later, independent of tmux itself.
- Community-documented gotcha corroborating this shape of bug (see search
  results): "test passes consistently without `-race`, fails occasionally with
  `-race`... failure appears to be timing-related due to race detector
  instrumentation overhead" — i.e. exactly this failure mode, not a real race,
  is a known category, not unique to this repo.

## 2. CI job topology (`.github/workflows/build.yml`) and runner spec

**The `test` job** (`runs-on: ubuntu-latest`, `needs: prepare`) runs, in order:
checkout → setup-go → download prepare's artifacts → apt-get tmux build deps →
restore/build pinned tmux 3.4 → **"Run tests with coverage (pinned tmux 3.4)"**
(`go test -race -coverprofile=... -covermode=atomic ./server/... ./session/...
./config/...`, with a full `-race -v ./...` fallback rerun on failure) → scanner
unit tests → coverage gate → feature-coverage report → registry check →
**"Integration coverage (advisory)"** (`go test -race -tags integration ./...`,
`continue-on-error: true`, runs sequentially *after* the main test step, not
concurrently with it).

- **No `-short`, no `-p`, no `-parallel` override** on the CI race invocation —
  it inherits Go's defaults (both default to `GOMAXPROCS`, i.e. `runtime.NumCPU()`
  on the runner).
- **Other jobs in the workflow** (`web-build-smoke`, `build` matrix ×5,
  `install-check`) all `needs: prepare` only, so they run **concurrently with**
  `test`, each on its *own* separate GitHub-hosted runner VM — they do not
  share CPU with `test`'s VM. `benchmark-gate` explicitly `needs: [prepare,
  test]` and only runs on `main` push, so it's serialized after `test`, not
  concurrent with it. **Conclusion: cross-job contention is not the mechanism**
  — the contention is entirely *intra*-job, from `go test`'s own default
  parallelism across the packages under `./server/...`, `./session/...`,
  `./config/...`.
- **Runner spec** (GitHub Docs,
  [github-hosted-runners reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)):
  for a **public repository** (confirmed via `gh repo view
  tstapler/stapler-squad --json isPrivate,visibility` → `PUBLIC`), the
  standard `ubuntu-latest` label now maps to a **4 vCPU / 16 GB RAM** VM (this
  is a relatively recent bump — private repos on the same label still get 2
  vCPU / 7 GB; older docs/blog posts citing "2 vCPU / 7 GB for ubuntu-latest"
  predate this and are stale for this repo). So `GOMAXPROCS` on this runner is
  effectively 4.

## 3. `go test` levers for isolating `-race` contention

- **`-p n`**: number of test *binaries* (packages) built/run in parallel.
  Defaults to `GOMAXPROCS` (4 here). `./server/...`, `./session/...`,
  `./config/...` expand to many packages (`server`, `server/services`,
  `server/mcp`, `session`, `session/tmux`, `session/git`, `session/unfinished`,
  etc.), and per the grep below, at least 9 test files across `session/` and
  `server/` spawn **real tmux sessions**. With the default `-p 4` on a 4-vCPU
  box, up to 4 of those packages' test binaries run at once, several of which
  independently fork real tmux subprocesses under `-race`'s 2-20x/5-10x
  overhead — this is the concrete oversubscription mechanism.
- **`-parallel n`**: number of `t.Parallel()` subtests within *one* package
  running concurrently. Also defaults to `GOMAXPROCS`. The two flaky tests
  don't call `t.Parallel()` themselves, but other tests in `server` package
  might, compounding contention within that one package's binary.
- **Per-package `-race` narrowing**: `go test -race ./config/...` (small,
  no-subprocess package) plus `go test ./server/... ./session/...` (no
  `-race`, or `-race -p 1`) as two separate invocations/steps is a
  straightforward way to keep `-race` coverage somewhere while removing it (or
  de-parallelizing it) exactly where the real-subprocess tests live — this is
  the kind of trade-off acceptance criterion #3 requires being explicit about,
  since it would mean `server`/`session` packages lose blanket `-race`
  coverage in CI (they'd still get it locally via `make test-race`, and via
  the advisory `test-integration` step, which already runs `-race -tags
  integration` today).
- **Splitting into a separate job/matrix entry**: moving
  `./server/...`/`./session/...` to their own CI job means it gets its **own**
  runner VM (own 4 vCPUs), so it stops competing with `./config/...` (and, if
  the packages within `server`/`session` are further split by matrix entry,
  with each other) — trading extra job-startup overhead (checkout, artifact
  download, tmux build/cache-restore, `setup-go`) for CPU isolation. This is
  the cleanest way to satisfy "narrow `-race` scope... explicit and justified"
  without losing `-race` coverage anywhere.
- **`-timeout`**: `go test`'s overall per-package kill switch (not per-test).
  Not currently set on the CI race step, so it falls back to Go's default (10
  minutes) — not implicated in this specific flake (individual test-level
  `waitForPermissionRequestHookCommand(t, ..., 30*time.Second)` budgets are
  what's timing out, well under any package-level `-timeout`), but worth
  confirming stays comfortably above any raised per-test budget if one is
  chosen.

## 4. Where hook-injection pipeline latency actually goes

Call chain for both flaky tests: `SessionService.CreateSession` → spawns an
unawaited goroutine (`server/services/session_service.go` ~line 1524) that
calls **`instance.Start(true)`** (real tmux spin-up) synchronously, then, only
after `Start` returns, calls **`InjectHookConfig`**
(`server/services/approval_handler.go:672`, real file write to
`.claude/settings.local.json`). The test polls for that file write via
`waitForPermissionRequestHookCommand` (20ms `time.Sleep` between reads,
budgeted 30s per the test call sites — see §5 note on a doc/code mismatch) and
separately polls `FindLiveInstance` via `waitForLiveInstance` (20ms interval,
30s budget).

`Instance.Start` → `startLocked` → `i.pm().Start(startPath)` →
`TmuxSession.Start`/`RestoreWithWorkDir` (`session/tmux/tmux.go`). Concrete
levers, from reading the actual polling/retry code (not guessed):

- **Real subprocess fork/exec is the dominant cost, not polling overhead.**
  Every `tmux new-session`/`list-sessions`/`set-option` call is a real
  `exec.Command` fork of the pinned tmux 3.4 binary (`t.cmdExec.Run(...)`),
  gated through `runGatedErr` (a circuit breaker / rate gate around tmux
  exec). Fork/exec cost itself isn't inflated by `-race` (separate process),
  but the Go-side orchestration around each call (building the command,
  circuit-breaker bookkeeping, JSON/registry updates) is, and each of those
  synchronous calls blocks the goroutine actually doing the work.
- **Fast path already exists and is well-tuned**: after `new-session`
  succeeds, `TmuxSession.Start` invalidates its exists-cache and calls
  `DoesSessionExistNoCache()` directly (`session/tmux/tmux.go:998`) instead of
  trusting the push-based registry, specifically because "the push-based
  registry can lag behind tmux reality" — this fast path avoids the slow poll
  loop in the common case. If it fails, it falls into a poll loop with
  `sessionCreateTimeout = 10 * time.Second` and `sessionPollInitialDelay = 5 *
  time.Millisecond` (exponential backoff, capped at 50ms) — i.e. worst case
  the tmux-session-exists wait alone can consume up to 10s before the test's
  30s hook-write budget even starts counting meaningfully.
- **`RestoreWithWorkDir`'s retry ladder** (used on the "existing session"
  path) has `maxRetries = 5` with **100ms/200ms/400ms/800ms/…** exponential
  backoff (`session/tmux/tmux.go:1092-1095`), plus a separate `ptyMaxRetries =
  3` loop with the same 100ms-doubling backoff for PTY attach
  (`session/tmux/tmux.go:1175-1180`). These are defensive retry ladders for
  "slow/loaded box," not something a fast CI box should normally hit — but
  under `-race` CPU contention (§1, §3) the box effectively *becomes* a loaded
  box, and each rung is a real `time.Sleep`, additive to the test's wall-clock
  budget.
- **Concrete levers to make the pipeline itself faster** (rather than just
  raising the timeout): (a) reduce `sessionCreateTimeout`'s poll-loop entry
  frequency further only if profiling shows the poll loop (not the tmux
  fork/exec or the registry wait) is actually where time goes — no evidence of
  that from static reading, so acceptance criterion #4 (measure before
  optimizing) applies directly here; (b) short-circuit
  `InjectHookConfig`'s file write to happen from a goroutine that isn't
  serialized *after* the full `Start()` return (currently strictly sequential
  in `session_service.go`) if hook-injection doesn't actually need
  post-`Start()` state — worth confirming against `Start()`'s contract before
  touching, since requirement #5 says no behavior change unless research shows
  the pipeline itself is the bottleneck; (c) the registry health-wait loop at
  `tmux.go:1009-1019` (2s deadline, 5ms sleep) is already tightly bounded and
  unlikely to be the culprit.
- **No evidence found (from static reading) that the pipeline itself is
  pathologically slow** under normal (non-`-race`, non-contended) conditions —
  CLAUDE.md states `make test` (`-short`, no `-race`) "passes reliably" — which
  points toward "CI-load/`-race`-contention," not "slow pipeline," as the
  primary driver, consistent with the already-diagnosed root cause in the
  requirements doc. Actual measurement (e.g. instrumenting
  `waitForPermissionRequestHookCommand` to log elapsed time on both pass and
  fail, or a targeted `-race`-vs-not timing comparison of `Instance.Start` +
  `InjectHookConfig` in isolation) is still needed before concluding this —
  static reading can only rule out obvious pipeline bugs, not prove absence of
  a real latency problem.

## 5. Existing `-short`/integration-tier conventions in this repo

- **`testing.Short()` is the repo's established unit/integration split**,
  used pervasively: `server/services/session_service_test.go:1272`,
  `session/instance_cold_restore_test.go`, `session/native_process_manager_test.go`,
  `session/session_creation_test.go`, `session/comprehensive_session_creation_test.go`,
  `session/instance_workspace_test.go`, `session/session_restart_test.go`,
  `session/tmux/{session_recovery,exec_gate,tmux}_test.go`,
  `session/unfinished/gogitstore/soak_test.go` — all skip themselves under
  `-short` with `t.Skip(...)`.
- **`//go:build integration` build-tag tier also exists** for a separate,
  heavier category: `server/mcp/server_integration_test.go`,
  `session/mcp_integration_test.go`, `session/headless/integration_test.go`,
  `session/tmux/server_registry_integration_test.go` — these are excluded from
  ordinary `go test ./...` entirely and only run via `make test-integration`
  (`go test -race -tags integration ./...`) or CI's separate "Integration
  coverage (advisory)" step.
- **The two flaky tests use *neither* mechanism** —
  `server/server_integration_test.go` has no `//go:build integration` tag (it
  compiles into the ordinary `server` package's test binary) and neither test
  calls `testing.Short()`. They run unconditionally in every `go test
  ./server/...` invocation, including plain `make test` (`-short`, no
  `-race`) — which is why CLAUDE.md can say `make test` "passes reliably" while
  CI (which runs the *same* tests, just under `-race` and without `-short`
  restricting anything *else* running concurrently in the same package/`-p`
  batch) sees intermittent timeouts. This is the single most actionable
  observation: the file's own doc comments (`installFakeClaudeBinary`,
  `waitForPermissionRequestHookCommand`) already show awareness of CI-load
  sensitivity, but the test itself opted out of both existing tiering
  mechanisms, so it always runs, always under full `-race` contention, with no
  way to shrink its footprint short of editing the test file directly.
- **`Makefile`'s own `test-race` target
  (`TMUX_BIN=$(CURDIR)/$(BIN_TMUX) go test -race -short ./...`, line 494-495)
  passes `-short`, but CI's race step does not.** Since these two tests don't
  check `testing.Short()`, `-short` wouldn't skip *them* — but it does skip
  every *other* short-skipped test in `session/`/`server/` that would
  otherwise also be running (and forking tmux) concurrently in the same `-p`
  batch. **This means `make test-race` locally is not actually representative
  of CI's race invocation** — CI's `go test -race ./server/... ./session/...
  ./config/...` runs strictly more concurrent load (every slow/integration
  test that `-short` would otherwise skip) than the closest local equivalent.
  This is a second concrete, previously-undocumented contributor to "why CI
  flakes but local doesn't," independent of runner CPU count, and a candidate
  fix in its own right: adding `-short` to CI's race step (or moving the
  slow/short-skipped tests to a dedicated non-`-short` job) would shrink the
  contention pool without touching timeouts or `-race` scope at all — but
  needs to be checked against whatever the coverage gate (`local-threshold:
  0`, `global-threshold: 60`) expects, since `-short` would drop coverage
  contribution from every currently-unskipped slow test too.

## 6. A found doc/code mismatch worth flagging to the planner

`waitForPermissionRequestHookCommand`'s docstring
(`server/server_integration_test.go:454-462`) says: *"Callers pass 60s, not the
~instant time InjectHookConfig itself takes... Observed CI flakiness at 30s...
motivated the wider budget."* But **both actual call sites pass `30 *
time.Second`** (lines 338 and 409), not 60s. Either the comment is stale (a
60s bump was written up but the actual value was reverted/never landed), or a
previous fix regressed. This directly bears on acceptance criterion #2
("timeout values... documented rationale, not a bare number bump") — whatever
the planning phase decides for the timeout value, it should reconcile the
comment and the code so they say the same number.

---

### Sources
- [Data Race Detector — go.dev](https://go.dev/doc/articles/race_detector.html)
- [Introducing the Go Race Detector — blog.golang.org](https://blog.golang.org/race-detector)
- [GitHub-hosted runners reference — docs.github.com](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
- Repo evidence: `.github/workflows/build.yml`, `Makefile`,
  `server/server_integration_test.go`, `server/services/approval_handler.go`,
  `server/services/session_service.go`, `session/instance.go`,
  `session/instance_tmux.go`, `session/tmux/tmux.go`, and the `testing.Short()`
  / `//go:build integration` call sites listed in §5.
