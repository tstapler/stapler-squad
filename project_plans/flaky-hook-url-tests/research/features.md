# Research: Feature Landscape — flaky-hook-url-tests

## 1. Well-documented Go-community patterns for `-race`-induced flakiness

- **`-race` overhead is large and well-documented, not a bug to "fix away."** The Go race
  detector doc (`golang.org/doc/articles/race_detector`) states typical overhead is
  ~2-20x execution time and ~5-10x memory, and that it uses extra goroutine scheduling
  points that change timing non-deterministically run-to-run. This is exactly the shape
  of failure this item describes: a 30s budget that's fine uncontended but marginal under
  `-race` + a shared/contended CI runner.
- **GitHub-hosted `ubuntu-latest` runners for public repos are 4 vCPU / 16GB** (standard
  Linux runner spec as of the 2024 default-runner upgrade). Combined with `-race`'s
  overhead and this repo's single `go test -race ./server/... ./session/... ./config/...`
  invocation running the *entire* three-package test suite in one process group, real
  tmux spin-up + hook-injection goroutines in these two tests compete for the same small
  core count as every other concurrently-scheduled test in the binary — this is the
  "load-driven marginal timeout" the requirements doc describes, distinct from the
  already-worked-around fake-binary-exits-too-early race.
- **Common community fixes for this exact shape of flake**, roughly in order of how
  surgical they are:
  1. **Widen the timeout budget** (what `waitForPermissionRequestHookCommand`'s comment
     already did once: 30s → the callers still pass 30s in the two tests, but the helper's
     own doc comment argues for 60s — inconsistent, see Edge Cases below).
  2. **Isolate contentious tests into their own `go test` invocation/job** so they don't
     share CPU with hundreds of other parallelized `-race`-instrumented tests. This is the
     "give it its own lane" pattern — GitHub Actions matrix jobs or a separate `-run`
     invocation are the standard mechanism.
  3. **`t.Parallel()` discipline** — tests that spin up real tmux sessions and are *not*
     marked `t.Parallel()` still run concurrently with the package's `TestMain`-driven
     goroutines and any parallel subtests elsewhere in the binary, because `go test`
     schedules all top-level tests of a package as goroutines gated only by `-parallel`
     (default `GOMAXPROCS`). Neither of the two flaky tests here calls `t.Parallel()`
     (confirmed by grep — 0 hits in `server_integration_test.go`), so they aren't
     *adding* to the contention themselves, but they're still victims of contention from
     everything else in `./server/...` scheduled alongside them.
  4. **Retry the specific flaky test at the CI level** (rerun-failed-tests action) — a
     blunt but common last resort; not applicable here per the "no new dependencies"
     constraint if it requires a new Action, though `go test -run` re-invocation in a
     shell retry loop needs no new dependency.

## 2. Existing patterns in THIS codebase for avoiding the same problem

- **`testing.Short()` guards exist widely** (23 hits across `session/*_test.go`,
  `server/services/session_service_test.go`) but **do not help here**: `make test` uses
  `go test -short` (Makefile line 437), but the CI `test` job's actual invocation
  (`.github/workflows/build.yml` line 155) has **no `-short` flag**. This matches the
  requirements doc's Baseline section verbatim ("Locally, `make test` ... passes
  reliably. CI's `Test` job runs under `-race` and intermittently times out") — CI
  deliberately runs the full, un-shortened suite, so gating these two tests behind
  `testing.Short()` would silently remove them from the very job that's flaking, which
  contradicts the "no new dependencies / no weakening" spirit of the fix (it would fix
  the symptom by no longer running the regression test at all under `-race`).
- **`//go:build integration` build tag is the established isolation mechanism** for
  slow/expensive tests in this repo: `server/mcp/server_integration_test.go`,
  `session/mcp_integration_test.go`, `session/headless/integration_test.go`, and
  `session/tmux/server_registry_integration_test.go` all use this tag. They run in a
  **separate, `continue-on-error: true`, advisory-only step** — "Integration coverage
  (advisory)" (build.yml lines ~245-255) — via `go test -race -tags integration ./... || true`,
  which explicitly swallows failures and uploads coverage as a build artifact rather than
  gating anything. `server_integration_test.go` (the file under investigation) does
  **not** carry this tag today — it's a plain file in the `server` package, so it's part
  of the main gating `-race` + `-coverprofile` invocation.
- **No `Eventually`/retry helper library is used anywhere** (no `testify/require.Eventually`
  import in the repo — confirmed via grep across `server/`, `session/`, `config/`). The
  established idiom instead is a hand-rolled `for time.Now().Before(deadline) { ...;
  time.Sleep(N) }` polling loop with a `t.Fatalf` on timeout — exactly the pattern already
  used by `waitForResolvedAddr`, `waitForLiveInstance`, `waitForPermissionRequestHookCommand`,
  and `waitForTmuxTeardown` in `server_integration_test.go` itself. Any fix should keep this
  idiom rather than introducing a new dependency (e.g. `testify/require.Eventually`), which
  would also violate the "no new dependencies" constraint.
- **Shared-state contention is already a named, documented problem in this exact file**:
  `waitForTmuxTeardown`'s comment (lines 503-526) documents that *every* integration test
  in the `server` package shares one process-scoped, PID-keyed tmux server socket
  (`session/tmux.testSocketOnce`, gated by `config.IsTestMode()`), and that teardown
  latency **monotonically increases** the more CreateSession/DeleteSession cycles run in
  the same process (reproduced locally with `go test -race -count=10`). This is direct,
  first-party evidence that the "marginal timeout" flakiness this item targets isn't
  purely a CI-runner-CPU-count problem — it's compounded by same-process test ordering
  and cross-test resource accumulation within this one package's test binary, which
  isolating the file into its own `go test` invocation would only partially address (see
  Edge Cases below).
- **`-race` is layered consistently everywhere it matters**: `make race-test` (Makefile
  line 495: `go test -race -short ./...`), `make ci` (line 541: `go test -race ./...`),
  and the CI `test` job all use `-race`, so any fix must preserve `-race` on whatever
  invocation ends up running these two tests — there is no precedent in this repo for a
  `-race`-exempt test lane for otherwise-gating tests.

## 3. Edge cases: does isolation alone explain the flake, or could `approval_handler.go` itself be slow?

- **Isolation only removes *cross-test* CPU contention, not *intra-test* real work.**
  Both flaky tests do real, non-trivial work sequentially: real tmux session spin-up
  (fork/exec + tmux control-mode handshake), a real `CreateSession` RPC, and
  `InjectHookConfig`'s real file write to `.claude/settings.local.json` — all inside one
  `-race`-instrumented process. If isolated into their own `go test` invocation (own
  process, own tmux socket per the PID-keyed scheme), they'd no longer compete with
  *other* tests' tmux sessions/CPU, but they would still incur `-race`'s own per-test
  slowdown (2-20x) on the tmux fork/exec and JSON-marshal-heavy `InjectHookConfig` path
  itself. On a maximally-loaded/throttled runner (e.g. GitHub Actions experiencing noisy
  neighbor contention at the *hypervisor* level, which isolation within one job's runner
  cannot fix), a sufficiently slow real tmux spin-up could still blow even a generous
  budget (60s+) — the requirements doc's own comment history (30s and 60s both observed
  failing for the *other*, already-fixed race) is evidence this exact codepath has
  blown large budgets before, just for a different root cause. **Isolation reduces the
  probability of a marginal timeout; it does not provide a correctness proof that
  `approval_handler.go`'s real path is bounded-time.** A robust fix should pair isolation
  (or budget widening) with either (a) a mechanism to positively confirm the async
  hook-injection goroutine has *started* (not just polling for its *result* file), so a
  slow-but-progressing case can be distinguished from a genuinely hung one in failure
  logs, or (b) profiling/timing instrumentation added temporarily to a stress run to
  measure actual `InjectHookConfig` latency distribution under `-race` + contention,
  rather than assuming isolation alone is sufficient.
- **The two callers already pass an inconsistent budget vs. the helper's own documented
  intent**: `waitForPermissionRequestHookCommand`'s doc comment (lines 458-462) explicitly
  argues for widening because "30s ... intermittently timed out waiting on scheduling,
  not on a real hang," yet both call sites (lines 338, 409) still pass `30*time.Second`,
  not `60*time.Second`. This is a plausible root cause on its own — the fix the comment
  describes was apparently written but never applied to the call sites — worth checking
  directly against `git log -p` / `git blame` on those two lines before reaching for a
  heavier isolation-based fix.

## 4. Unstated needs: `-race` coverage semantics and `coverage.out` merging

- **Constraint**: "Must not weaken `-race` coverage" (requirements.md). The current single
  invocation is:
  ```
  TMUX_BIN="$(pwd)/bin/tmux" go test -race -coverprofile=coverage.out \
    -covermode=atomic ./server/... ./session/... ./config/...
  ```
  This produces one merged `coverage.out` across all three module trees, which then feeds
  `vladopajic/go-test-coverage@v2` with `global-threshold: 60` (repo-wide) and
  `local-threshold: 0` (no per-file minimum enforced).
- **Risk in an "isolate into its own job" fix**: if `server_integration_test.go` (or just
  the two flaky tests) is moved behind a `//go:build integration` tag — mirroring the
  existing precedent — it would move from the primary gating invocation into the
  `continue-on-error: true` "Integration coverage (advisory)" step. That step:
  1. Does **not** feed into `coverage.out` / the `go-test-coverage` gate at all — it
     writes to a separate `integration.out` via `go tool covdata textfmt`, uploaded only
     as a build artifact, never checked against the 60% global threshold.
  2. Runs with `|| true` (whole-command level) and the step itself is
     `continue-on-error: true` — a real regression in these two tests would no longer
     fail CI at all, only silently show up in an uploaded artifact nobody is required to
     look at.
  - This is a **direct, material regression against "must not weaken -race coverage"**:
    the tests would still technically run under `-race`, but their *pass/fail signal*
    would stop gating merges, and their lines would silently drop out of the
    threshold-checked `coverage.out`. Given `local-threshold: 0`, no automated check
    would flag the coverage drop either — it would only show up as a smaller `coverage.out`
    diff a reviewer might not think to inspect.
  - **Implication for the eventual fix (Phase 3 planning)**: if isolation is pursued, it
    must be done as a *separate, still-gating* `go test` invocation for just this file
    (e.g. its own `-run` pattern within the same coverage-producing command, or a second
    `-coverprofile` output later merged with `gocovmerge`/`go tool covdata merge` into the
    same `coverage.out` before the coverage-gate step) — not by reusing the existing
    `-tags integration` advisory lane, which was designed for genuinely optional/expensive
    tests, not for tests whose pass/fail is a real regression signal (this file explicitly
    documents itself as "the regression test for the early-binding bug found in
    architecture-review.md" and "the literal Story 1.3.1 acceptance criterion" — i.e. its
    own comments assert it must keep gating).

## Key files referenced

- `server/server_integration_test.go` — the two flaky tests + all wait helpers
- `.github/workflows/build.yml` lines 119-260 — `test` job: coverage-gated `-race` run
  (line 155), advisory `-tags integration` run (line 251), coverage gate config (161-166)
- `Makefile` lines 436-441, 495, 541 — `make test` (`-short`, no `-race`), `make race-test`,
  `make ci`
- `session/tmux/server_registry_integration_test.go`,
  `server/mcp/server_integration_test.go`, `session/mcp_integration_test.go`,
  `session/headless/integration_test.go` — existing `//go:build integration` precedent
