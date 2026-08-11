# Stack Research: flaky-hook-url-tests

Date: 2026-08-01

## 1. Cost of `go test -race` and known GitHub Actions isolation patterns

**Overhead.** Go's race detector is ThreadSanitizer-based instrumentation of every
memory access. Google's own race-detector docs and the academic literature on
dynamic race detectors converge on the same order-of-magnitude figures: **~2-20x
CPU/wall-time slowdown and ~5-10x memory overhead** versus a non-`-race` build.
This is a range, not a constant — it depends heavily on how much real concurrency
(goroutines, syscalls, scheduling churn) the code under test exercises. Real
process spin-up (fork/exec of tmux, PTY attach) is exactly the kind of
syscall-heavy, goroutine-heavy workload where the top of that range shows up,
because TSan has to track happens-before edges across every goroutine touching
shared instance state, not just do extra bookkeeping in hot loops.

**Isolation patterns available in GitHub Actions / `go test`, cheapest to most
invasive:**

| Pattern | Mechanism | Cost | Fit here |
|---|---|---|---|
| `-run` regex in a dedicated step | `go test -race -run 'TestServer_should_Write.*HookURL'` as its own invocation | Free (no infra change) | Isolates from *other tests in the same job*, but the job still shares the runner's 4 vCPUs with everything else in the `Test` job's steps if run sequentially — actually a *net win* since steps run serially, not concurrently, in one job |
| `-p 1` (limit package parallelism) | `go test -race -p 1 ./server/... ./session/... ./config/...` | Slower wall-clock (packages run one at a time instead of up to `-p N` concurrently) | Directly attacks the stated root cause (CPU contention from concurrently-running package binaries) without splitting CI structure |
| Build tag (`//go:build integration`) | Move the 2 flaky tests behind a tag, run in a **separate step** (still same job, still `-race`) | Free | Matches the pattern the rest of the repo already uses (see §4) — the odd one out is that these two tests currently have *no* tag |
| Matrix job (separate runner) | New job in `build.yml`, e.g. `test-hook-integration`, `needs: prepare`, own `runs-on: ubuntu-latest` | +1 runner-minute billing, +job startup overhead (~10-20s: checkout, setup-go, artifact download, tmux cache) | Full CPU isolation — no contention from the other 3 packages' tests at all — but heaviest structural change |
| Larger runner (`ubuntu-latest-8-core` or similar GitHub-hosted sized runner) | `runs-on:` label change | Requires GitHub Team/Enterprise plan + billing; doubles or quadruples available vCPUs for the whole job | Addresses root cause broadly (less contention for *everything* in the job, not just these 2 tests) but is an infra/cost decision, not purely a code fix |

None of these require a different CI provider, OS, or Go version, so all are
inside the stated constraints. The cheapest fix that directly targets the
diagnosed root cause (CPU contention under `-race` while `./server/... ./session/...
./config/...` compile and run their package binaries concurrently) is `-p 1` or
moving the two tests behind a build tag into their own step — both keep the
existing single job, keep full `-race` coverage, and don't change runner size.

## 2. Current `.github/workflows/build.yml` — `Test` job

- **Runner:** plain `ubuntu-latest` for every job in the file (`test`, `prepare`,
  `build`, `install-check`, `benchmark-gate`, `web-build-smoke`) — no larger/paid
  runner labels anywhere in this workflow or any other workflow in
  `.github/workflows/`. Confirmed via `grep -rn runs-on .github/workflows/*.yml`.
  Standard GitHub-hosted `ubuntu-latest` = 4 vCPU / 16 GB RAM (as of 2026).
- **Job structure:** `prepare` (generates protos + builds web UI once, uploads as
  artifact) → `test` (`needs: prepare`) runs in parallel with the `build` matrix
  and `install-check` (both also `needs: prepare` only, not `needs: test` — test
  failures don't block binary publishing by design, per the job's own header
  comment).
- **The single test invocation that matters here:**
  ```bash
  TMUX_BIN="$(pwd)/bin/tmux" go test -race -coverprofile=coverage.out \
    -covermode=atomic ./server/... ./session/... ./config/... \
    || (TMUX_BIN="$(pwd)/bin/tmux" go test -race -v ./...; exit 1)
  ```
  This is **one `go test` invocation covering three whole package trees**
  (`./server/...`, `./session/...`, `./config/...`), not per-package or
  per-file. `go test` compiles a separate test binary per package and, by
  default, runs up to `GOMAXPROCS` of those binaries concurrently (the `-p`
  flag, default = `NumCPU`). On a 4-vCPU `ubuntu-latest` runner that means up to
  4 package test binaries — each independently paying the `-race` CPU/memory
  tax — competing for the same 4 cores at once. `server/server_integration_test.go`
  spawning a *real* tmux server + shell process is one of many concurrent
  consumers of that shared, already-`-race`-taxed CPU budget.
  - On failure, it re-runs `go test -race -v ./...` (the **entire** repo, every
    package) just for verbose failure output — this doesn't run tests twice in
    the success path, only on failure, so it's not a source of the flake itself,
    but it does mean a flaky failure produces a second full-repo `-race` run
    (expensive, and could itself intermittently fail/timeout for the same
    contention reason, muddying CI logs further).
  - No `-p` flag, no `-parallel` flag, no `GOMAXPROCS` override anywhere in this
    step or job — Go's defaults apply throughout.
- **Coverage gate immediately follows** (`vladopajic/go-test-coverage@v2`,
  60% global threshold) — reads `coverage.out` from the same step, so splitting
  the flaky tests into a separate invocation needs to either merge coverage
  profiles (`go test -coverprofile` supports multiple `-coverpkg`/merge via
  `go tool covdata` or simple file concatenation for the text format) or accept
  a coverage-profile gap for those two tests specifically.
- **No existing stress/repeat step** in this job (`-count=N` appears only in
  `benchmark-gate`, for benchmark statistical significance, not for flake
  detection — see §4).

## 3. Deterministic async-completion patterns (stdlib / repo precedent)

The requirement is to replace or firm up a fixed sleep-poll timeout for two
sequential async completions: (a) the instance becoming "live" in the poller,
and (b) `InjectHookConfig`'s file write landing on disk after `instance.Start(true)`
returns.

**What the current code does** (`server/server_integration_test.go`):
- `waitForLiveInstance` — polls `deps.SessionService.FindLiveInstance(sessionID)`
  every 20ms for up to `timeout`.
- `waitForPermissionRequestHookCommand` — polls-and-parses
  `.claude/settings.local.json` every 50ms for up to `timeout`.
- **Both call sites pass `30*time.Second`** (lines 338 and 409). Notably, the
  doc comment directly above `waitForPermissionRequestHookCommand` (lines
  458-462) says: *"Callers pass 60s, not the ~instant time InjectHookConfig
  itself takes... Observed CI flakiness at 30s... motivated the wider budget."*
  This comment describes a 60s budget that **does not exist in the code** — both
  call sites still pass 30s. This is either a stale comment left behind after a
  fix was reverted/not applied, or a fix that was written but never wired
  through to the call sites. Either way it's a concrete, low-risk starting
  point: making the call sites match the comment's stated intent (60s) is a
  one-line-per-call-site change consistent with what whoever wrote that comment
  already decided was correct.
- These are genuinely *two sequential* waits (`waitForLiveInstance` then
  `waitForPermissionRequestHookCommand`), so worst-case combined budget today is
  60s (30+30), not 30s — but each individual wait's budget is what's marginal
  under `-race` + contention, since either one alone can blow its 30s if
  `instance.Start(true)`'s real tmux spin-up is slow enough.

**More deterministic alternatives than fixed sleep-poll:**
1. **`context.Context` with cancellation propagated from the test's own
   deadline** — `t.Context()` (Go 1.24+, this repo is on Go 1.25 per
   `build.yml`'s `go-version: '1.25.0'`) already carries a deadline tied to the
   test's own `-timeout`; polling loops could select on `<-ctx.Done()` instead
   of an independent `time.After`/deadline calculation, unifying "test timed
   out" and "wait timed out" into one signal and one error message instead of
   two separate ad hoc deadlines that can disagree.
2. **Event-driven signaling instead of poll+sleep** — the codebase already has
   an `events.EventBus` (`s.eventBus.Publish(events.NewSessionUpdatedEvent(...))`,
   used in `session_service.go` around the same async goroutine that calls
   `InjectHookConfig`). The test could subscribe to session-updated events
   and block on a channel receive (with a `select` against a timeout) rather
   than polling the instance/file every 20-50ms. This trades "poll until
   state matches" for "block until producer signals," which is both faster
   (no polling granularity latency) and immune to the specific race where a
   fixed budget is marginal — a channel receive doesn't degrade under CPU
   contention the way a sleep-based poll loop does (a starved goroutine still
   wakes on channel send; it just wakes late, whereas a sleep-poll loop can
   miss its own wakeup slot entirely and burn through iterations).
   Caveat: no such "hook injected" event currently exists — `InjectHookConfig`
   is called synchronously inside the goroutine with no event published after
   it succeeds (only failure is logged, at `log.Warn`, not published to
   `eventBus`). Adding one would be a small, well-scoped change to
   `session_service.go`'s existing goroutine (right after the `InjectHookConfig`
   call around line 1547).
3. **`testutil/wait` package** (`testutil/wait/wait.go`) — the repo already has
   a generic, context-based polling helper (`WaitForCondition`,
   `WaitForConditionWithError`, with `FastTimeout`/`DefaultTimeout`/`SlowTimeout`
   presets of 2s/10s/30s and configurable poll interval) built specifically
   because "tests... cannot import the top-level `testutil` package due to
   import cycles with the session package." **`server_integration_test.go`
   does not use this package** — it hand-rolls its own `deadline := time.Now().Add(timeout)`
   loops instead. This is a duplication, not a new pattern to introduce: the
   two flaky tests could switch to `wait.WaitForConditionWithError` with (say)
   `wait.SlowTimeout` today with zero new abstractions, and it already
   supports the `context.WithTimeout`-based approach in point 1.
4. **`sync.WaitGroup` / callback hook exposed for tests only** — a
   testing-only hook (e.g. an unexported callback field settable via a
   test-only setter, mirroring `SetOnExitCallback` already used at line 850 of
   `instance.go` for process-exit callbacks) that fires exactly when
   `InjectHookConfig` finishes inside the goroutine would let the test `Add(1)`/
   block on `Wait()` deterministically instead of polling the file at all. This
   is the most invasive of the four options (touches `session_service.go`'s
   internals for test-observability) but eliminates the file-polling timeout
   entirely rather than just widening it.

Given the "no architecture redesign" scope boundary, options 1 (context deadline
unification) and 3 (reuse `testutil/wait`) are the cheapest fits; option 2
(event-driven) is a moderate, well-scoped addition; option 4 is the most
invasive and probably out of scope for this fix.

## 4. Existing stress-test / flake-detection tooling in this repo

- **No `-count=N` stress-test make target exists for flaky *tests*.** The only
  `-count=` usages in the Makefile are for **benchmarks**, not flake repro:
  - `make bench-*` targets: `go test -bench=. -benchmem -count=8 -timeout=30m ./...`
    (lines ~820, 829, 840) — statistical significance for benchmark comparison
    via `benchstat`, unrelated to test flakiness.
  - `benchmark-gate` job in `build.yml`: `-count=5` for the same reason
    (documented inline: "count=5 + utest for statistical significance").
- **Existing build-tag convention for real-process integration tests**: several
  files already use `//go:build integration` to gate tests that spawn real
  processes, run only via the separate `make test-integration` target
  (`go test -race -tags integration ./...`), **not** the main `make test`
  (`-short`) or the main CI `Test` job's package-tree invocation:
  - `server/mcp/server_integration_test.go`
  - `session/mcp_integration_test.go`
  - `session/tmux/server_registry_integration_test.go`
  - `session/headless/integration_test.go`
  - `**server/server_integration_test.go` is the odd one out**: despite its
    filename matching this exact naming convention, it has **no `//go:build
    integration` tag and no `testing.Short()` skip guard** (confirmed: `grep -n
    "testing.Short()\|t.Skip" server/server_integration_test.go` returns
    nothing). It always runs as part of the main `./server/...` tree in every
    `go test` invocation — including the CI `Test` job's `-race` run and local
    `make test`/`make test-race` (both of which pass `-short` and would
    otherwise skip it if it had the same guard as its siblings). This is a
    structural inconsistency worth flagging for planning: the fix could either
    (a) leave it in the main run but harden the waits per §3, or (b) bring it
    in line with the repo's own established convention by adding the
    `integration` build tag (or a `testing.Short()` guard) so it only runs via
    `make test-integration` / a dedicated CI step — which would also solve the
    contention problem for free by moving it out of the shared `-race
    ./server/... ./session/... ./config/...` invocation entirely.
- **`test-triage-harness` targets** (`test-triage-harness`, `test-triage-gate`,
  etc., lines ~500-517) show the repo's precedent for scoping a `-run` regex to
  a narrow, expensive test group in its own make target + `-tags=harness` — the
  same shape of solution (tag + dedicated invocation) already exists for a
  different flaky/slow-test problem (triage harness tests), reinforcing that
  build-tag isolation is the repo's established idiom, not a new one this fix
  would be introducing.
- **Local repro path**: no existing script artificially induces CPU contention
  to reproduce the flake locally (e.g. no `stress-ng` wrapper, no parallel
  `go test ./... &` background load script). A stress-run to validate any fix
  would need to either (a) run `go test -race -count=N ./server/... -run
  'TestServer_should_Write.*HookURL'` under artificial load (e.g. `stress-ng
  --cpu $(nproc)` running concurrently, or `go test -race ./...` running in the
  background to simulate "rest of the suite" contention), or (b) add a
  throwaway CI workflow_dispatch job that runs the full `-race` suite
  `-count=5` to check for intermittent timeouts before merging — matching the
  constraint that verification must be a real repro/stress-run, not just local
  "it passed once for me."

## Key files referenced

- `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/.github/workflows/build.yml` (`test` job, lines 121-244)
- `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/Makefile` (lines 436-556, 748-756, 820-840)
- `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/server/server_integration_test.go` (lines 281-500, esp. `waitForLiveInstance` 441-452, `waitForPermissionRequestHookCommand` 454-500, `installFakeClaudeBinary` 36-60)
- `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/server/services/approval_handler.go` (`InjectHookConfig`, lines 672-794)
- `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/server/services/session_service.go` (async `CreateSession` goroutine, lines 1524-1560)
- `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/testutil/wait/wait.go` (existing, unused-by-these-tests context-based poll helper)
- `/home/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/session/mcp_integration_test.go`, `session/headless/integration_test.go`, `session/tmux/server_registry_integration_test.go`, `server/mcp/server_integration_test.go` (sibling files using the `//go:build integration` convention this file lacks)
