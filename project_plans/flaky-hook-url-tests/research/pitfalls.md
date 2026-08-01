# Pitfalls & Risks — flaky-hook-url-tests

Research pass for SDD Phase 2 (stapler-squad). Scope: `server/server_integration_test.go`
(`TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
and `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`), the async
hook-injection path in `server/services/session_service.go` (CreateSession's goroutine) and
`server/services/approval_handler.go` (`InjectHookConfig`).

## 1. General failure modes when "fixing" a `-race` CI timeout

- **Widening the timeout and calling it done.** A longer budget (30s → 60s → 90s) makes a
  scheduling-contention flake pass more often, but it is indistinguishable, from the CI log
  alone, from silently tolerating a real regression that makes the code path permanently
  slower (e.g. a lock now held across an I/O call). Any timeout change needs an explicit
  before/after latency measurement, not just "it passes now."
- **Retries masking a real deadlock.** Wrapping the flaky assertion in a retry loop (or
  `-count` re-run until green) converts a hang into "eventually passes," which hides the
  difference between (a) benign scheduling variance and (b) an actual deadlock/goroutine
  leak that happens to resolve on a second attempt for unrelated reasons (e.g. GC, another
  goroutine's timeout firing). This is exactly the shape of bug `waitForTmuxTeardown`'s own
  comment already documents for a *different* flake in this same file — the fix there was
  understanding the fire-and-forget teardown contract, not retrying blindly.
- **Over-isolating a test out of the `-race` job to "fix" the flake.** Moving the two hook
  tests to a separate, non-`-race` invocation (or a `-short`-skipped path) makes the flake
  disappear from the log but removes race coverage for the exact code this test exists to
  regression-test: the lazily-evaluated `baseURLFn` closures threaded through
  `NewApprovalHandler`/`SetHookBaseURLFn` (per the test's own doc comment, this is the
  regression test for a real, previously-shipped bug in `architecture-review.md`). Isolating
  it from `-race` is a scope violation against this project's own constraint ("must not
  weaken `-race` coverage for `./server/... ./session/... ./config/...`").
- **Stress-testing locally with `-race -count=N` and declaring victory.** Local stress runs
  reproduce *contention within one machine's scheduler*, not GitHub Actions runner-specific
  contention (shared vCPU throttling, noisy-neighbor CPU steal, cgroup limits). A fix that
  makes `go test -race -count=20 ./server/...` pass 20/20 locally is necessary but not
  sufficient evidence — the requirements doc's own Feasibility Risk #1 already flags this
  ("Marginal-timeout hypothesis may not be conclusively provable without exact CI
  contention"). Local pass rate should be reported as supporting evidence, not proof.
- **Fixing the symptom instead of naming the mechanism.** The project's own engineering
  discipline rule ("No fix without root cause") applies directly here: "increase the
  timeout" is not a root-cause statement. The root-cause hypothesis on record is
  "CI runner scheduling contention under the full `-race` suite delays `instance.Start(true)`
  well past what an idle machine sees" — any fix should be traceable to that specific
  mechanism (e.g. reducing lock hold time in the hot path, reducing goroutine fan-out during
  test setup) not just "give it more time."

## 2. Specific interaction risk: hook-injection latency changes vs. the `installFakeClaudeBinary` early-exit race

The two races are causally chained through the *same* async goroutine
(`server/services/session_service.go` lines ~1524–1549):

```go
go func() {
    ...
    if startErr := instance.Start(true); startErr != nil {   // race A lives here
        ...
        return   // <-- InjectHookConfig is never reached
    }
    ...
    if err := InjectHookConfig(instanceRootDir, instanceTitle); err != nil {  // race B's target
        log.Warn(...)
    }
    ...
}()
```

- **Race A** (already documented, in `installFakeClaudeBinary`'s comment,
  `server/server_integration_test.go` lines 36–60): if tmux's readiness check doesn't see the
  fake `claude` process as live before it exits, `instance.Start(true)` returns an error and
  the goroutine `return`s *before* ever calling `InjectHookConfig`. When this happens,
  `waitForPermissionRequestHookCommand` times out **no matter how large its budget is** — the
  hook file is never written, so it isn't a race that a bigger timeout can paper over.
- **Race B** (this ticket's target): once `Start(true)` *does* succeed, `InjectHookConfig`
  itself runs almost instantly (the comment on `waitForPermissionRequestHookCommand`, line
  ~458, says "not the ~instant time InjectHookConfig itself takes") — the *test's* wait is
  actually mostly waiting on `Start(true)` to return under scheduling contention, not on
  `InjectHookConfig`'s own I/O.
- **The specific danger**: any change aimed at "reducing hook-injection latency" that touches
  poll intervals or timeouts *inside* `instance.Start(true)`'s own readiness-check loop (as
  opposed to something purely inside `InjectHookConfig`) directly changes race A's timing
  window, not just race B's. Concretely:
  - **Shortening the tmux readiness poll interval** (to make `Start(true)` return faster,
    thinking it will help race B) tightens the window in which the fake binary's `sleep 60`
    must still be alive — sounds safe since 60s ≫ any poll interval, but if a future change
    also shortens `sleep 60` (e.g. someone "optimizes" the test binary to `sleep 5` reasoning
    "tests only need it briefly"), a faster poll makes it *more* likely to observe liveness
    before the process table entry updates, which is a *good* interaction — but the reverse
    is not true: **lengthening** the readiness-check's own internal timeout to "fix" race B
    could mask race A actually firing, because the outer test-level `waitForLiveInstance`
    timeout (30s) might still return normally (session appears live) while `Start(true)`'s
    return value/error path is never observed by the test at all — the two failure surfaces
    (test-level timeout vs. `Start`'s own internal error) are decoupled, so "fixing" one
    doesn't validate the other.
  - **Any refactor that reorders `SetCreationProgress`/`ForceStatus` calls relative to
    `Start(true)`'s error return** risks a classic **double-checked-locking / stale-read**
    bug per `.claude/rules/go-double-checked-locking.md`: if a "faster" code path caches or
    snapshots instance state before `Start(true)` resolves and then returns that cached
    value on the error path, waiters could observe a session as "live" (from a stale
    snapshot) when it actually failed, producing exactly the kind of contradiction that rule
    warns about ("re-reading the slot returns another goroutine's observation, which may
    contradict the current goroutine's own computation").
  - **Net risk**: a change motivated by "make hook injection faster" that instead speeds up
    or slows down `Start(true)`'s own internal polling changes the *already-documented* race
    window for a *different, more severe* failure mode (a hang that no timeout budget fixes)
    without that being the change's stated intent — exactly the kind of unintentional
    interaction the requirements doc's Feasibility Risks section already calls out ("changes
    risk reintroducing/interacting with that race").
- **Practical implication for scope**: any candidate fix must be evaluated against *both*
  tests' failure logs, not just the marginal-timeout one — and ideally exercised with the
  fake binary's `sleep` duration deliberately shortened in a throwaway local run, to confirm
  the change doesn't shrink race A's safety margin.

## 3. Risk of splitting `-race` into a separate CI job/invocation for this file

Current state (`.github/workflows/build.yml` lines 153–157, single `test` job):

```yaml
- name: Run tests with coverage (pinned tmux 3.4)
  run: |
    TMUX_BIN="$(pwd)/bin/tmux" go test -race -coverprofile=coverage.out \
      -covermode=atomic ./server/... ./session/... ./config/... \
      || (TMUX_BIN="$(pwd)/bin/tmux" go test -race -v ./...; exit 1)
```

followed by:

```yaml
- name: Coverage gate
  uses: vladopajic/go-test-coverage@v2
  with:
    profile: coverage.out
    global-threshold: 60
    local-threshold: 0
```

This is **one `go test` invocation** covering three package trees; Go's own test tooling
merges per-package coverage into a single `coverage.out` for free. Splitting the two flaky
tests (or all of `server/server_integration_test.go`) into a second invocation/job breaks that
free merge and introduces real risk:

- **Coverage merge correctness.** A second `go test -race -coverprofile=coverage2.out ...`
  run produces a *second* profile that nothing currently combines. `vladopajic/go-test-coverage`
  reads a single `profile:` path — pointing it at only one of the two files silently drops
  coverage for whichever package tree isn't in that file (the gate would still pass/fail, just
  against incomplete data, which is worse than an honest failure). Combining them correctly
  requires either concatenating `mode:` headers correctly (naive `cat` duplicates the `mode:
  atomic` line, which breaks `go tool cover`) or a merge tool (e.g. `gocovmerge`, not currently
  a project dependency — the ticket's own constraint says "no new deps"). This is a real
  correctness trap, not just extra CI config.
- **`make registry-generate` / `docs/registry` step** (build.yml lines 223–238) and the
  **Coverage gate** step both currently run once, downstream of the single combined
  `coverage.out`. Splitting the test invocation means deciding which job produces the
  artifact the coverage gate reads, and whether `actions/upload-artifact` +
  `actions/download-artifact` needs to shuttle a second profile between jobs — new
  cross-job wiring, new failure surface if the artifact name/path drifts.
- **CI job matrix complexity.** A new job means: its own `Set up Go`, `Download generated
  files`, `Install tmux build dependencies`, `Cache pinned tmux binary` / `Build pinned tmux
  3.4` steps (build.yml lines 128–151) all have to be duplicated or the new job has to
  depend on `prepare`/`test` in a way that doesn't already exist. Every duplicated step is
  itself a place for the two jobs' tmux binaries, cache keys, or Go toolchain versions to
  drift out of sync over time.
- **Added CI wall-clock from a new job's cold start.** A separate job pays the full runner
  cold-start cost (VM provisioning + checkout + Go module cache warm + tmux build/cache-restore)
  independently of the existing `test` job — likely tens of seconds to a couple of minutes of
  pure overhead per PR, to isolate what is currently two tests inside one file. Given the
  ticket's own scope framing ("marginal-timeout-under-load flakiness," not a structural
  problem with this test file), that overhead is disproportionate unless the underlying fix
  genuinely requires isolation from `-race` contention from the *other* packages under test
  (which itself would need to be demonstrated, not assumed).
- **Net take**: splitting into a separate job is a heavier, riskier lever than it looks,
  specifically because of the free coverage-merge property the single-invocation design
  currently gets for free. Prefer test-local fixes (longer/adaptive budgets scoped to just
  these two tests, reducing incidental contention the test itself creates — e.g. the
  documented shared-tmux-socket pattern in `waitForTmuxTeardown`'s comment) before reaching
  for a new CI job.

## 4. House-documented Go concurrency gotchas relevant to `approval_handler.go`

From `.claude/docs/concurrency-patterns.md` and `.claude/rules/go-double-checked-locking.md`
(same content, the doc is the canonical source, the rule is the enforced summary):

- **Double-checked locking must return the locally-computed value, not re-read the shared
  slot.** Pattern: `read-lock → miss → compute → write-lock → conditional store → return`.
  The wrong version returns whatever ended up in the shared slot after the lock is dropped,
  which may be a *different* goroutine's result if this goroutine lost the write race. The
  canonical correct implementation referenced is `session/git/worktree_git.go`'s `IsDirty`.
- **Applicability to this ticket**: `InjectHookConfig` itself doesn't do double-checked
  locking, but the broader async goroutine in `session_service.go` reads/writes shared
  `instance` state (`SetCreationProgress`, `ForceStatus`, `SetStatusManager`) that other
  goroutines (the live-instance poller `waitForLiveInstance` polls, and any status-change
  event consumers) read concurrently. Any change to *when* those calls happen relative to
  `Start(true)`'s return — e.g. attempting to short-circuit "wait faster" logic by caching an
  intermediate progress/status value and returning it from a different goroutine's read —
  would reproduce exactly the anti-pattern this rule exists to prevent: a waiter observing a
  stale/foreign value instead of the value the goroutine that actually did the work computed.
  Any fix here should keep the existing pattern (state is set once, in-order, by the single
  async goroutine; readers poll and observe, they don't independently reconstruct state) and
  avoid introducing any new lock-protected cache in this path.
- **No other house pattern doc (mutex-vs-atomic selection, singleflight, lock-free structures)
  is directly implicated** — this bug is fundamentally about wall-clock scheduling delay
  under contention, not about a missing synchronization primitive. The main risk is
  *introducing* an unneeded primitive (e.g. a new mutex-guarded cache to "speed up" hook
  injection) where none is needed, which is also flagged by
  `.claude/rules/interface-pollution-checklist.md`'s "unjustified generic" /
  "speculative" smells in spirit — added synchronization is itself a smell here unless a
  specific data race is identified.

## 5. Risk of conflating the two distinct races via a single "just raise the timeout" fix

Yes — this is a real and likely reviewer trap, and the requirements doc is already careful to
distinguish the two, but a shallow fix could still merge them:

- **Race A (early-exit, already mitigated via `sleep 60` in the fake binary)**: per the code
  comment, this race causes `waitForPermissionRequestHookCommand` to fail **at both 30s AND
  60s** — i.e., it is a genuine hang (the file is never written because `InjectHookConfig` is
  never called), not a slow-but-eventually-successful path. No timeout value fixes it; it
  is fixed by the test correctly keeping the fake binary alive long enough for
  `instance.Start(true)`'s readiness check to observe liveness before the fake process could
  exit.
- **Race B (this ticket, marginal timeout under `-race` load)**: this is a genuine
  eventually-succeeds-given-enough-time case — `InjectHookConfig` *does* get called, it's just
  delayed by scheduler contention, so an appropriately larger (or adaptive) budget is a
  legitimate fix *for this specific race*.
- **The conflation risk**: a reviewer (or an engineer reproducing CI flakiness) who sees "test
  intermittently times out" without reading the `installFakeClaudeBinary` comment carefully
  could reason "we already tried 30s and 60s and it still failed once, so let's go to 90s or
  120s" — treating the two failure reports as one continuous "just needs more time" problem.
  If race A's specific trigger (fake binary exiting before readiness check) reappears (e.g.
  from a future change to the fake binary's script, or an environment where `sh -c "sleep 60"`
  gets killed early by a differently-configured PID 1 / process-reaper), a blanket timeout
  increase would silently paper over that regression too — the test would just take longer
  to report the *same* hang, wasting CI wall-clock without ever succeeding, and the failure
  would look superficially identical to "we didn't wait long enough."
- **Mitigation for this ticket**: any fix should (a) explicitly distinguish which race a given
  test failure log corresponds to before choosing a timeout value (race A: `Start(true)`
  itself failed/erred, visible via `log.Error("[CreateSession] async start failed", ...)` in
  server logs; race B: `Start(true)` succeeded but slowly), and (b) avoid picking a timeout
  number "because the comment already tried 30 and 60" without re-deriving why those numbers
  were insufficient for *this* ticket's specific failure (load contention) as opposed to *that*
  documented failure (early exit).
