# Pitfalls & Risks — ci-hookurl-race-flake

Research pass for SDD Phase 2. Scope per `requirements.md`: the two hook-URL/MCP-URL
integration tests in `server/server_integration_test.go`
(`TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort`,
`TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`),
the CI `test` job in `.github/workflows/build.yml`, and (only if research shows it's the
actual bottleneck) `server/services/approval_handler.go`'s `InjectHookConfig`.

## 0. Critical finding first: this exact problem already has a full, unshipped SDD plan

Before any new pitfall analysis: **`project_plans/flaky-hook-url-tests/`** is a complete,
prior SDD pass — `requirements.md`, `research/{stack,features,architecture,pitfalls,
build-vs-buy,ux}.md`, `implementation/{plan,pre-mortem,validation,adversarial-review,
architecture-review}.md`, and `decisions/ADR-001-p1-flag-over-isolated-invocation.md` —
targeting the **identical** bug: these same two tests, the same root cause (`-race`
CPU contention across `./server/... ./session/... ./config/...` on a 4-vCPU
`ubuntu-latest` runner), and the same "Race A vs Race B" distinction this task's
requirements doc also names.

**It was never implemented.** Verified directly against the current tree:
- `server/server_integration_test.go` still passes `30*time.Second` at both
  `waitForPermissionRequestHookCommand` call sites (lines 338, 409) — the prior plan's
  Task 1.1.1 (widen to 60s) was never applied, even though the helper's own doc comment
  (lines 458-462) already claims "callers pass 60s."
- None of the four poll loops (`waitForResolvedAddr`, `waitForLiveInstance`,
  `waitForPermissionRequestHookCommand`, `waitForTmuxTeardown`) were converted to
  `require.Eventually`/`testutil/wait.WaitForCondition` (prior Task 1.2.1-1.2.4).
- `.github/workflows/build.yml`'s gating step (lines 153-157) has no `-p 1` flag — ADR-001's
  chosen mitigation was never merged. ADR-001's own `## Status` field still reads
  `Proposed`, not `Accepted`.
- `git log` on both files shows no commit referencing this fix; only `chore(sdd):` commits
  added the planning artifacts themselves.

**Implication for this task**: the highest-leverage first step is *not* re-deriving pitfalls
from scratch — it's reading `project_plans/flaky-hook-url-tests/implementation/plan.md`
(chosen approach: 30s→60s fix + `require.Eventually` conversion + `-p 1` in CI, ADR-001)
and its own `pre-mortem.md` (5 failure modes already identified, including the exact
"conflating Race A and Race B" trap this task's own requirements doc also flags), then
deciding whether to **execute that existing plan** rather than re-plan the same ground.
Re-planning from zero risks silently contradicting decisions that prior research already
made carefully (e.g. why option 3 — a second isolated `-race` invocation — was rejected).
If this task's planner instead reaches a *different* conclusion than ADR-001, that
divergence should be argued explicitly against ADR-001's stated reasoning, not arrived at
independently in ignorance of it.

One thing this task's requirements do that the prior one didn't as explicitly: acceptance
criterion 4 requires *measuring* actual hook-injection latency under `-race` + load before
choosing "raise timeout" vs "make pipeline faster" — the prior plan's Task 2.1.2 only
measured `-p 1`'s wall-clock *cost*, never captured a numeric flake-rate or latency
baseline (this gap is pre-mortem failure #2 in the prior plan, rated P2, and was never
closed). Closing that measurement gap is this task's clearest concrete addition over the
prior pass, not a wheel to reinvent.

## 1. The "raise the timeout again" anti-pattern — why it's already flagged as insufficient here

This file has raised this exact budget once already (per its own comments): the
`waitForPermissionRequestHookCommand` doc comment states the intent was already to move
30s → 60s, and that move was never even applied to the call sites. Requirements
correctly treats "raise it again" as suspect. The specific failure pattern:

- **Diminishing, then negative, returns.** Each bump buys headroom against a fixed
  distribution of tail latencies, but `-race` + shared-runner contention has a long,
  runner-load-dependent tail — there's no number that provably eliminates timeouts short
  of "so large it defeats the purpose of a CI gate" (a test that takes 3+ minutes to fail
  is barely better than one that hangs). Each bump also compounds: these two tests already
  chain two sequential waits (`waitForLiveInstance` then
  `waitForPermissionRequestHookCommand`), so a "small" per-call bump doubles in the worst
  case combined budget.
- **Runner-dependent thresholds don't transfer.** A number tuned to pass reliably on
  today's `ubuntu-latest` (4 vCPU) silently stops being "generous" the moment GitHub
  changes default runner specs, the job matrix grows (more concurrent jobs/steps stealing
  scheduler time), or the repo adds more tests to the same package trees — nobody
  re-derives the number when any of those change; it just quietly becomes marginal again
  years later, and the flake reappears looking "new."
- **Masking a real regression.** A budget generous enough to survive *any* plausible CI
  load is also generous enough to hide an actual regression that makes the hook-injection
  path permanently slower (e.g. a newly-introduced lock held across the tmux spin-up, an
  accidentally-serialized I/O call). Requirements' own acceptance criterion 2 ("documented
  rationale, not a bare number bump") and criterion 4 ("measured, not assumed") exist
  precisely to force a distinction between "we measured X ms and chose a budget with Y%
  headroom" and "we guessed bigger is safer."
- **The two-races conflation trap** (already identified in the prior project's
  `pitfalls.md` and worth restating because it's the single easiest way to get this wrong):
  this file's `installFakeClaudeBinary` comment documents a *different*, already-mitigated
  race ("Race A" — the fake `claude` binary exiting before tmux's readiness check observes
  it live, causing `CreateSession`'s goroutine to `return` *before* ever calling
  `InjectHookConfig`) that **no timeout value fixes** — the file is never written, full stop.
  A future investigator who sees "timed out again" without checking server logs for
  `"[CreateSession] async start failed"` (Race A's signature; its absence indicates the
  load-sensitive Race B this task targets) could easily "fix" a Race A recurrence by
  bumping the same timeout yet again, burning CI time on a hang no budget resolves.

## 2. `go test -race` pitfalls in CI specifically

- **Overhead is workload-shaped, not constant.** ThreadSanitizer-based race detection
  costs ~2-20x CPU/wall-time and ~5-10x memory versus a non-`-race` build (order-of-magnitude
  figures, not a fixed multiplier) — and the top of that range shows up precisely for
  syscall-heavy, goroutine-heavy workloads (real process fork/exec, PTY attach, the exact
  shape of `instance.Start(true)`'s tmux spin-up), because TSan must track happens-before
  edges across every goroutine touching shared state, not just add flat per-call overhead.
  This is not a coincidence for this bug — it's *why* this specific test pair is the one
  that blows a fixed budget instead of some other, less-concurrent test in the same suite.
- **Package-level parallelism multiplies the tax.** By default `go test` runs up to
  `GOMAXPROCS` (= `NumCPU`, 4 on `ubuntu-latest`) package test binaries concurrently. The
  current gating invocation spans three package trees (`./server/... ./session/...
  ./config/...`) in one command with no `-p` override — meaning up to 4 independently
  `-race`-taxed binaries compete for the same 4 cores at once. This is the diagnosed root
  cause both this task's requirements doc and the prior `flaky-hook-url-tests` project
  name explicitly. `-p 1` (serialize package binaries) is the cheapest lever that targets
  it without touching test code or timeouts — but it has a real, non-hypothetical cost:
  total wall-clock for the step increases because packages no longer overlap build/test
  time (must be measured and stated explicitly per acceptance criterion 2, not silently
  absorbed).
- **`t.Parallel()` interacts badly with an already-contended `-race` run.** This repo's
  affected tests don't currently call `t.Parallel()` (confirmed: no `t.Parallel` in
  `server/server_integration_test.go`), which is good — `t.Parallel()` would let Go's test
  runner interleave this test's goroutines with *other* parallel subtests within the same
  package binary, adding a second axis of scheduling contention on top of the
  cross-package one `-p` controls. If any future refactor of this file (or of tests added
  to `./server/...`) reaches for `t.Parallel()` to "speed things up," it directly fights
  against the fix being considered here — flag this as an anti-pattern to explicitly avoid
  introducing while working this ticket, not just something to check is already absent.
- **GOMAXPROCS defaults on constrained runners.** GitHub-hosted `ubuntu-latest` is 4 vCPU;
  Go's default `GOMAXPROCS` equals the container's visible CPU count, which the Go runtime
  reads correctly in the cgroup-aware versions this repo uses (Go 1.25). The risk here
  isn't a *misdetected* CPU count — it's that 4 vCPUs is already a small, easily-saturated
  budget, and every additional concurrent consumer (package test binaries, the fallback
  `-v ./...` re-run on failure, any background service steps) divides an already-thin
  pool further. There's no code fix for "the runner is small" — only reducing the number
  of things competing for it (fewer concurrent package binaries) or increasing the budget
  each waiting test can tolerate.
- **Race detector state is genuinely global-ish per binary, not per-test.** TSan's shadow
  memory and detector state apply across the whole test binary process, so *unrelated*
  tests running in the same package binary (not just the two flaky ones) add to the same
  CPU/memory pressure pool during the run — meaning any other slow/heavy test recently
  added to `./server/...` is a silent contributor to this flake even though it never
  appears in the failing test's own name. This argues for treating the *whole* `test` job's
  design (not just these two tests) as the object of any contention-reduction fix, which is
  exactly why ADR-001 chose a job-wide `-p 1` over an isolated-just-these-two-tests
  invocation.
- **The failure-path doubles the cost.** The gating step's own fallback
  (`|| (TMUX_BIN=... go test -race -v ./...; exit 1)`) re-runs the *entire* repo under
  `-race -v` whenever the primary invocation fails for any reason, including this flake.
  That's not itself a cause of the original timeout, but it means every occurrence of this
  flake pays for a second full-repo `-race` run just to produce verbose failure output —
  worth knowing when reasoning about "wall-clock cost of the flake," separate from
  "wall-clock cost of the fix."

## 3. Real-subprocess + filesystem-poll test pitfalls (tmux + async JSON write)

- **Fire-and-forget teardown outliving the test.** `DeleteSession` intentionally tears
  down tmux/git resources in an unawaited goroutine (correct for production UX — the RPC
  returns immediately), but that means a test's `t.Cleanup(DeleteSession)` can return
  before real teardown finishes. This repo already hit and partially fixed this:
  `waitForTmuxTeardown` polls `inst.TmuxSessionExists()` post-cleanup specifically because,
  without it, a still-tearing-down session from an earlier test piles up on the **shared,
  process-scoped tmux socket** (`testSocketOnce`, `session/tmux/tmux.go:333-338`, gated by
  `config.IsTestMode()`) — every integration test in one `go test` binary shares one tmux
  server socket by design. The code comment on `waitForTmuxTeardown` (lines 503-526)
  documents this was reproduced locally with `go test -race -count=10` and got
  **monotonically worse** the more create/delete cycles ran in the same process — i.e. this
  is a compounding, not a one-shot, contention source, and it is a *different* mechanism
  from the CI-load timeout this task is nominally about. Any new fix here risks either (a)
  not accounting for this residual, already-known-but-unfixed contributor (the prior
  project's pre-mortem P1 item explicitly warns a future recurrence could be misdiagnosed
  as "the new fix didn't work" when it's actually this pre-existing mechanism), or (b)
  scope-creeping into fixing it when the task explicitly says not to redesign the
  lifecycle.
- **Zombie/orphaned processes across retries.** The fake `claude` stand-in
  (`installFakeClaudeBinary`) is a `sleep 60` script deliberately kept alive past the
  readiness check window. If a test times out and its cleanup doesn't run to completion
  (or `waitForTmuxTeardown`'s own 5s cleanup budget also expires), the underlying tmux pane
  and its `sleep 60` child can outlive the test process — in a CI environment (no login
  shell reaping orphans the way an interactive session might), a burst of timeouts across
  retries could leave more zombie sleep processes and tmux sessions than a single run
  would produce, compounding cost run over run within a job. Any retry/backoff design added
  to work around timeouts needs to be paired with a bounded, verified-successful teardown —
  not just "try the test again," because that risks stacking teardown debt on the
  already-shared socket rather than resolving it.
- **Non-atomic writes being read mid-write.** `InjectHookConfig` writes
  `.claude/settings.local.json` via a temp file + `os.Rename` (atomic on POSIX
  filesystems) — this is *already correct* and confirmed by the prior project's research
  (`architecture.md`/`stack.md`) as "not the bottleneck; no torn-read window." This is worth
  re-verifying (not re-deriving) rather than assuming as a first check when investigating
  this task's acceptance criterion 4 (measure the actual pipeline latency) — a torn-read
  bug would look superficially similar to a slow-write timeout in the JSON-parse-failure
  path (`waitForPermissionRequestHookCommand`'s inner `json.Unmarshal` failures are
  silently swallowed and retried, so a torn read wouldn't even produce a distinguishable
  error — it would just look like "not written yet"). If new measurement work reopens this
  question, confirm via the rename call site directly, not by inference from timing alone.
- **Poll-interval / retry-backoff design risk.** The existing poll loops use fixed,
  fairly tight intervals (10-50ms) with a hard deadline and no backoff. Under CPU
  contention, a tight poll interval itself consumes scheduler time competing with the very
  process it's waiting on (each `time.Sleep(10*time.Millisecond)` wakeup is itself a
  scheduling event under an already-4-vCPU-constrained, `-race`-taxed runner). This is a
  second-order effect, not the primary bottleneck (`instance.Start(true)`'s own tmux
  spin-up dominates), but a "make it more robust" pass that further tightens poll
  intervals (reasoning "check more often to react faster") would make contention *worse*,
  not better — the opposite of the intuitive fix. Any retry-loop redesign should prefer
  fewer, better-spaced polls (or a true blocking wait — e.g. via `events.EventBus`, which
  this codebase already has and which a channel-receive doesn't degrade the way a
  sleep-poll loop does under contention) over a tighter poll interval.
- **Test process, not just target process, is a resource consumer.** Each of these two
  tests boots a full `*Server` (`BuildDependencies()`, `NewServerWithDeps`, `srv.Start(ctx)`)
  in addition to the tmux subprocess — meaning the "real subprocess" pitfall here is
  two-layered: a real HTTP server goroutine tree *and* a real tmux server + shell process,
  both under `-race` instrumentation simultaneously. Contention analysis that only accounts
  for "the tmux fork/exec is slow" undercounts the actual concurrent load these two tests
  place on the runner.

## 4. Prior flaky-test / tmux patterns already documented in this repo

Beyond `project_plans/flaky-hook-url-tests/` (covered in §0), repo-wide `.claude/`
docs/rules were checked for tmux teardown and CI-timing precedent:

- **`.claude/rules/tmux-keep-server-on-restart.md`** — documents a *related but distinct*
  tmux lifecycle hazard: restarting the *production* systemd service (`make
  install-service` / `systemctl --user restart`) kills the tmux server and every live
  session unless `--tmux-keep-server` is passed, because the Linux `ExecStart` historically
  lacked that flag (macOS's LaunchAgent already had it). **This is not directly the same
  mechanism as the CI flake** — that rule is about killing sessions on a real deployed
  instance during a service restart; this task's tests run their own isolated,
  `config.IsTestMode()`-gated tmux socket (`testSocketOnce`) inside a `go test` binary, not
  the production systemd-managed tmux server. The relevance is narrower than it first
  looks: both bugs share the *theme* of "tmux server/session lifecycle assumptions
  silently violated by a code path that tears things down faster/differently than callers
  expect" (unawaited teardown goroutines, server-wide side effects from what looks like a
  scoped operation) — worth keeping in mind as a *pattern* to watch for if this task's fix
  touches tmux spin-up/teardown code, but it is not itself the root cause of this CI flake
  and doesn't need to be "fixed" as part of this task.
- **`.claude/rules/prefer-go-git-over-subshells.md`** — establishes the repo's general
  preference for native Go integrations over shelling out, with an explicit carve-out:
  "a subshell is still fine ... any operation needing a credential helper... the rule is
  'prefer go-git when it can do the job,' not 'never shell out.'" tmux itself has no native
  Go client equivalent in this codebase (`session/tmux/tmux.go` shells out via
  `safeexec.CommandContext` throughout) — this rule doesn't apply to the tmux spin-up path
  and shouldn't be read as license to "go-native-ify" tmux invocations as a speed fix; that
  would be a large, out-of-scope rewrite for a CI-timeout ticket.
- **No other `.claude/docs/*.md` or `.claude/rules/*.md` file mentions flaky tests, CI
  timing, or `-race` CPU contention specifically** — `concurrency-patterns.md` and
  `go-double-checked-locking.md` are about correctness (stale-slot reads), not
  timing/scheduling flakiness, though the prior project's `pitfalls.md` correctly flags
  their relevance *if* any fix touches how `session_service.go`'s async goroutine
  sets/reads instance state (see the pattern below).
- **Double-checked-locking anti-pattern is a real risk *if* someone tries to "speed up"
  `Start(true)`.** Per `.claude/rules/go-double-checked-locking.md` and the prior project's
  pitfalls analysis: `session_service.go`'s async `CreateSession` goroutine reads/writes
  shared instance state (`SetCreationProgress`, `ForceStatus`) that `waitForLiveInstance`'s
  poller also reads concurrently. A "faster" refactor that caches/snapshots instance state
  before `Start(true)` resolves and returns that cached value on an error path would
  reproduce exactly this rule's warned-against bug — a waiter observing a stale/foreign
  value instead of what the goroutine that actually did the work computed. This is a
  concrete, named risk for acceptance criterion 5 ("no behavior change to
  approval_handler.go or the hook-injection path unless research shows the pipeline itself
  is the bottleneck") — if research *does* justify touching this path, the double-checked
  locking rule and the `go-concurrency`/`go-development` skills should be invoked per this
  project's CLAUDE.md guidance on any Go concurrency change.

## 5. Risk ranking of the "obvious" fixes

Ranked from **highest risk of masking a real bug / lowest genuine fix value** to
**lowest risk / most durable**, incorporating both general CI-flake literature patterns
and this repo's own prior analysis (ADR-001, `build-vs-buy.md`'s "3-for-3 precedent"
finding that this exact test pair has been patched via wait-tuning three times before,
never via CI topology changes):

1. **Just raising the timeout again (highest risk).** Already attempted once in spirit
   (the stale 60s comment) and never fully applied; requirements' own acceptance criteria
   2 and 4 exist specifically to block a bare number bump. Masks regressions, doesn't
   scale with runner-load variance, and risks the Race A/Race B conflation trap in §1.
   Only acceptable *as part of* a fix that also includes a measured rationale (criterion 4)
   — never as the sole lever.
2. **Reducing CI job concurrency / runner sizing changes.** Requires GitHub
   org-level access this session likely doesn't have (confirmed as a rabbit hole in the
   prior project's requirements doc); even if actionable, it's an infra/billing decision
   with a cost trade-off outside this repo's code, and doesn't reduce the *within-job*
   package-level contention (`-p` default) that's the actually-diagnosed cause — a bigger
   runner helps everything equally but is the least targeted, least repo-controllable
   lever, and doesn't produce a durable, reviewable code artifact the way a workflow-file
   change does.
3. **Splitting `-race` scope (isolating this file/these tests into a separate
   invocation or job).** Looks like the "purest" fix but is a heavier, riskier lever than
   it appears: coverage-profile-merge correctness for a second `-coverprofile` output is
   unverified and would need either a new dependency (violates "no new external
   dependencies") or fragile manual profile concatenation; job-setup duplication (tmux
   build/cache, Go setup) is an ongoing drift risk; and it still wouldn't fix the
   *within-binary* shared-tmux-socket contention (`testSocketOnce`) since other tests in
   the same package/binary would still share it. ADR-001 rejected this option on exactly
   these grounds with a 3-for-3 precedent against CI-topology changes for this test pair.
   Legitimate only if a measurement (criterion 4) proves cross-package contention persists
   even after `-p 1`/timeout fixes and no cheaper lever remains.
4. **Making the pipeline itself faster (most durable, but narrowest applicability).**
   Genuinely addresses the bottleneck rather than the budget around it — but prior
   research already found `InjectHookConfig`'s own file write is *not* the bottleneck (atomic
   rename, near-instant); the real latency is `instance.Start(true)`'s tmux spin-up, which
   is Race A's territory and this task's own scope explicitly warns against touching
   without evidence ("no behavior change ... unless research shows the pipeline itself is
   the bottleneck"). This is the right fix *if and only if* acceptance criterion 4's
   measurement actually locates a concrete bottleneck in the pipeline (not just "CPU
   contention exists") — pursuing it without that evidence risks the double-checked-locking
   and Race-A-reintroduction hazards named in §4, for a change that may not even be needed
   if `-p 1` alone resolves the marginal-timeout symptom.
5. **`-p 1` (serialize package binaries in the existing gating invocation) — lowest
   risk, already vetted.** This is ADR-001's chosen mitigation: single-flag change, zero
   new dependencies, zero coverage-merge risk (same command produces the same
   `coverage.out`), fully and independently reversible, and directly targets the
   specifically-diagnosed root cause (cross-package `-race` CPU contention on a shared
   4-vCPU runner). Its only real cost — increased job wall-clock — is knowable in advance
   via local measurement and must be reported explicitly, not hidden, per acceptance
   criterion 2. Combined with a *measured* (not guessed) timeout value and the
   `require.Eventually` mechanism swap (readability/consistency, not a behavior change),
   this is the combination the prior project's research already converged on as lowest-risk.

## Key files referenced

- `project_plans/flaky-hook-url-tests/` (full prior SDD pass — requirements, research,
  plan, ADR-001, pre-mortem, validation, adversarial-review, architecture-review) — read
  this before writing new planning artifacts for this task.
- `server/server_integration_test.go` (lines 36-60 `installFakeClaudeBinary`/Race A;
  281-420 the two flaky tests; 422-540 the four poll helpers)
- `.github/workflows/build.yml` (lines 119-268, the `test` job; no `-p` flag, no job-level
  `timeout-minutes` on `test` itself — only `benchmark-gate` sets one, at 30 min)
- `session/tmux/tmux.go` (lines 333-409: `testSocketOnce`, `Socket`, `ResolveSocket` —
  the shared-socket/test-isolation mechanism referenced in §3)
- `server/services/session_service.go` (async `CreateSession` goroutine, ~lines 1524-1560)
- `server/services/approval_handler.go` (`InjectHookConfig`, atomic write via temp file +
  `os.Rename`)
- `.claude/rules/tmux-keep-server-on-restart.md`, `.claude/rules/go-double-checked-locking.md`,
  `.claude/rules/prefer-go-git-over-subshells.md`, `.claude/docs/concurrency-patterns.md`
