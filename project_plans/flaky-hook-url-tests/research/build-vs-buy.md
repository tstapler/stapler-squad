# Build vs. Buy: flaky-hook-url-tests

## Context

Two tests in `server/server_integration_test.go` —
`TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
and `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` — intermittently
time out in CI waiting (30s budget each) on two hand-rolled poll helpers:
`waitForLiveInstance` (polls `SessionService.FindLiveInstance` every 20ms) and
`waitForPermissionRequestHookCommand` (polls `.claude/settings.local.json` every 50ms
for a `PermissionRequest` command hook written by `InjectHookConfig`,
`server/services/approval_handler.go:672`). Both run inside the `test` job's single
`go test -race -coverprofile=... ./server/... ./session/... ./config/...` invocation
(`.github/workflows/build.yml:155`), not under the `-short`-gated `make test-race`
target — neither test checks `testing.Short()`. `runs-on: ubuntu-latest` = 4 vCPUs;
`nproc` on this dev box shows 24 for comparison, i.e. CI has materially less headroom
than local dev, and `-race` roughly doubles both CPU and memory cost per goroutine.
Existing in-file comments (lines 41–51, 313–318, 458–462, 506–520) already document
three distinct, previously-hit flake root causes on this exact test pair (fake-claude-
binary startup race, PID-keyed shared tmux socket across `-count` reruns, and
scheduling-induced timeout under contention) — this is a file with a known history of
CI-only timing sensitivity, not a one-off.

## Option 1 — Existing OSS/stdlib pattern already in the repo: `testify/require.Eventually`

**Finding**: `github.com/stretchr/testify v1.11.1` is already a direct dependency
(`go.mod`), and `require.Eventually` is **already used in 6+ other test files** in this
same package tree, including two integration-style suites in the same directory:
`server/services/approval_handler_integration_test.go` (3 call sites),
`server/services/approval_handler_secret_test.go` (2 call sites),
`server/services/capacity_monitor_test.go`, `server/services/backlog_service_pipeline_mode_test.go`,
`server/services/backlog_service_events_test.go` (9 call sites). Notably,
`approval_handler_integration_test.go` polls the *exact same kind of thing* — a
just-written `settings.local.json` hook — via `require.Eventually` rather than a
hand-rolled `for time.Now().Before(deadline) { ...; time.Sleep(x) }` loop.

`server_integration_test.go` itself uses **zero** testify calls — it's pure
`for`/`time.Sleep`/`t.Fatalf`, an inconsistency with its sibling package, not a
deliberate choice documented anywhere in the file's comments.

- **Pros**: Zero new dependency (already vendored and idiomatic in this codebase).
  `require.Eventually(condition, waitFor, tick, msgAndArgs...)` collapses the
  boilerplate deadline loop into one line, is already the established pattern next
  door, and — critically for the actual bug — makes the timeout **configurable
  independently of the poll interval** without hand-writing a new deadline/sleep pair
  each time. It does not, by itself, fix CPU contention, but it's strictly better
  ergonomics for the same semantics the code already has (still polling, still a
  budget), with no behavior change and no new race risk.
- **Cons**: Doesn't address the root cause (CPU contention under `-race`) — it's a
  drop-in replacement for the same polling strategy, not a different strategy. Minor
  loss of the very specific custom `t.Fatalf` messages currently baked into each
  helper (retained easily via `msgAndArgs`).
- **Verdict: Recommended** as a no-risk mechanical cleanup for consistency, but **not
  sufficient alone** — it doesn't change the CPU-contention math that's the actual
  timeout cause. Pair with Option 4/isolation below.

## Option 2 — SaaS/managed CI runner upgrade (larger or self-hosted GitHub Actions runners)

- **Pros**: GitHub-hosted larger runners (8/16/32 vCPU) would directly reduce
  `-race`-under-contention pressure with no code change; a real fix for a resource-
  bound flake if the hypothesis is correct.
- **Cons**: Requires an Actions billing plan change (larger runners are billed per-
  minute at a multiplier and require the repo/org to be on a paid plan — GitHub Free
  tier does not offer them); self-hosted runners add real ongoing infra to maintain
  (patching, availability, cost) that this repo does not currently have. Either path
  is an org-level/billing decision, not something this PR's diff can express, and
  requirements.md explicitly scopes "changing CI provider/runner" as **out of scope**
  and flags runner sizing as something to merely *assess*, not action.
- **Verdict: Not recommended for this item.** Confirmed viable in principle but
  outside this item's appetite and the repo's control (no existing paid-runner
  config or self-hosted fleet to extend) — matches the requirements.md prediction.
  Worth a one-line callout in the plan doc for whoever owns CI infra, nothing more.

## Option 3 — Hand-rolled completion-signal channel vs. tuning the existing poll

**The "replace poll with signal" idea**: have `CreateSession`'s async goroutine
(`server/services/session_service.go:1524-1549`) signal completion of
`InjectHookConfig` (line 1547) via a channel/context the test can block on, instead of
the test polling the file it wrote.

- **Is it correct/low-risk?** Mechanically, yes — `InjectHookConfig` calls
  `os.WriteFile`-style writes synchronously (`approval_handler.go`) and returns before
  the next line runs, so a signal sent *after* that call returns has a valid
  happens-before relationship with the write (Go's memory model + the OS write/close
  completing before the function returns) — a signal-fires-before-flush race is not
  actually possible here given the current code shape. **But** wiring it requires:
  new exported test-observable hooks on `SessionService`/`Instance` (a channel, a
  callback registration point, or a new event on `s.eventBus`) that exist *only* to
  serve this one test file — production code has no other consumer for "hook config
  was just injected." That is new production-code surface area purely to satisfy a
  test, which the `interface-pollution-checklist.md` house rule flags as exactly the
  kind of speculative, test-only addition to avoid; it also doesn't remove the
  `waitForLiveInstance` poll (there's no equivalent signal for "tmux session is live
  and registered in the poller" without similarly instrumenting the poller). Net: two
  poll loops become one poll + one signal, for real new production code to maintain.
- **Tuning the existing poll (`waitForLiveInstance`/waitForPermissionRequestHookCommand`)**:
  no production code changes, no new observable surface, strictly test-file-local.
  Already has a documented precedent of being *widened* once already (30s→60s
  discussed in the file's own comments) to absorb scheduling delay — i.e. the
  poll-tuning lever has been pulled successfully before on this exact file.
- **Verdict**: signal-based approach is **not recommended** — correct in the narrow
  happens-before sense, but it adds production-code surface for a test-only concern
  (violates this repo's own interface-pollution guidance) and only covers half the
  wait chain. **Tuning/replacing the poll (Option 1) is recommended** as the lower-
  risk, lower-effort option that matches the item's 1-2 day appetite.

## Option 4 — Fork or adapt a prior fix for this exact problem in this repo

**Finding**: this repo has **no prior instance of isolating a single test file's
`-race` invocation into its own CI job** — `git log` over `.github/workflows/build.yml`
shows CI hardening commits (`fix(ci): fetch the tmux submodule...`,
`d434b0d75 fix(concurrency): eliminate data races and add race detector to CI`, etc.)
but the `test` job has always run `-race` over the full `./server/... ./session/...
./config/...` tree in one shot; there is no existing "split -race into N jobs"
pattern to adapt. However, `git log` on `server_integration_test.go` itself shows this
exact pair of tests has already been patched **three separate times** for CI-only
timing flakiness (fake-claude-binary race → PID-keyed shared tmux socket →
scheduling-induced 30s timeout, each with an inline comment explaining the fix), all
via the same lever: **widen or add a poll/wait budget, or make cleanup wait rather
than fire-and-forget** — never via new signaling machinery or CI topology changes.
That is the closest thing to "a similar already-solved instance of this exact
problem," and it point at the same conclusion as Option 3: tune the poll, don't
invent a new mechanism.

- **Verdict**: **Recommended** as the strongest signal in this research — the repo's
  own history is 3-for-3 in favor of "widen the wait budget / fix a real ordering
  bug," 0-for-0 on signaling or CI-topology changes, for this specific test file.

## Overall Recommendation

1. **(Primary)** Replace the two hand-rolled deadline loops in
   `server_integration_test.go` with `require.Eventually` (already a dependency,
   already the pattern used one directory over for the identical
   settings.local.json-polling scenario in `approval_handler_integration_test.go`) —
   zero new deps, consistent with sibling test files, no behavior change.
2. **(Primary)** Investigate isolating this file's `-race` run (or the `test` job's
   `-race` step more broadly) into its own job/matrix shard to reduce CPU contention,
   per requirements.md scope item (1) — this is a CI-workflow change, not a code
   change, and needs no new dependency either.
3. **(Reject)** Do not hand-roll a completion-signal channel — it's correct in
   isolation but adds test-only production surface area this repo's own
   interface-pollution rule flags, and doesn't fully replace the poll pattern anyway.
4. **(Out of appetite, not actioned)** Larger/self-hosted runners — technically
   viable, correctly predicted by requirements.md as out of this item's control/
   appetite; confirmed, not pursued.
