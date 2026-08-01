# Build vs. Buy: CI Infrastructure Options for the Hook-URL/MCP-URL Race Flake

Research pass for `project_plans/ci-hookurl-race-flake/requirements.md`. This is a CI/test-infra
problem, not a product feature, so "build vs. buy" here means: use an existing GitHub/Go
ecosystem tool or convention vs. write bespoke tooling to solve it.

## Context gathered from the repo itself

Before evaluating external options, the repo already has two directly relevant conventions in
place — both are strong evidence for what the "idiomatic" answer looks like here:

- **`testing.Short()` gating** is used extensively (`session/tmux/exec_gate_test.go`,
  `session/native_process_manager_test.go`, `session/integration_test.go`,
  `session/instance_cold_restore_test.go`, `session/session_restart_test.go`, etc.) to skip slow
  tests under `make test` (`go test -short ./...`, `Makefile:437`) and `make test-race`
  (`go test -race -short ./...`, `Makefile:495`).
- **`//go:build integration` build tags** already isolate a class of slow/heavy integration tests
  from the default `go test` run: `server/mcp/server_integration_test.go`,
  `session/mcp_integration_test.go`, `session/headless/integration_test.go`,
  `session/tmux/server_registry_integration_test.go`. These are only compiled in under
  `make test-integration` (`go test -race -tags integration ./...`, `Makefile:498`), and are
  **excluded** from CI's `test` job today because that job's `go test` invocation
  (`.github/workflows/build.yml`) passes neither `-short` nor `-tags integration`.
- **The two flaky tests are the anomaly, not the norm**: `server/server_integration_test.go`'s
  `TestServer_should_WriteUnchangedHookURL_When_StartedOnExplicitPort` and
  `TestServer_should_WriteRealPortIntoSessionHooksAndMCPURL_When_StartedWithPortZeroThenSessionCreated`
  have neither a `testing.Short()` guard nor a `//go:build integration` tag, so CI's
  `go test -race ... ./server/... ./session/... ./config/...` sweeps them into the same run as
  every fast unit test, under `-race`, with no isolation.

This matters directly for point 2 below: the repo doesn't need a *new* pattern, it needs the
*existing* pattern applied consistently.

---

## 1. GitHub-hosted vs. self-hosted/larger runners

**Question**: does `ubuntu-latest` have a paid larger-runner option that removes CPU contention?

- Larger runners exist, but GitHub restricts them to **organizations on the GitHub Team plan or
  GitHub Enterprise Cloud**. They are not purchasable on a personal/free GitHub account, and this
  repo (`tstapler/stapler-squad`, personal fork/dotfiles-adjacent) has no such billing entity
  behind it.
- Standard GitHub-hosted runner minutes (any size available to a personal account) are **free on
  public repositories** regardless of runner size class within what a personal account can access
  — but the larger-runner *tiers* themselves simply aren't offered to personal accounts at all, so
  "pay for more CPU" isn't an available lever here, cost aside.
- Self-hosting a bigger runner (e.g., on a home box or a cloud VM) is technically possible via
  GitHub's self-hosted runner registration, but that trades a CI flake for new maintenance surface
  (patching, security exposure of a runner with repo access, uptime) — disproportionate for fixing
  a test-timeout flake.

**Pros**: would genuinely remove CPU contention if it were available; zero code changes.
**Cons**: not purchasable on this account tier; self-hosting shifts cost from "occasional flaky
CI run" to "ongoing infrastructure to secure and maintain."
**Verdict**: **Not recommended.** Larger hosted runners are gated behind Team/Enterprise billing
this repo doesn't have, and self-hosting is disproportionate. This option is out of reach for
reasons of account access, not just cost — don't spend further effort here.

---

## 2. `go test -race` scope tools: build tags/`testing.Short()` vs. a bespoke runner script

**Question**: standard Go idioms for separating "fast unit, no `-race`" from "slow integration,
`-race` optional" vs. a custom test-runner script.

- **`testing.Short()`** (stdlib, `go test -short`) is the standard idiom for "skip this test when
  running the fast suite." Already used pervasively in this repo (see Context above). Zero new
  dependencies; a one-line `if testing.Short() { t.Skip(...) }` guard per test.
- **Build tags (`//go:build integration`)** are the standard idiom for "compile this test only
  into a separate, explicitly-invoked binary." Already used in this repo for exactly this class of
  problem (tmux/MCP integration tests that need real infra). Requires no new tooling — it's a
  first line comment plus a blank line, recognized natively by `go build`/`go test`.
- **A bespoke custom test-runner script** (e.g., a shell/Go script that greps test names, forks
  separate `go test` invocations per "tier") would duplicate what `-short`/build tags already do
  natively, add a new artifact to maintain, and diverge from every other slow test already gated
  the standard way in this codebase — inconsistent with existing conventions for no added benefit.

**Pros of the standard idiom**: already proven in-repo, zero new dependencies, `go vet`/tooling
understands it natively, any Go developer recognizes it instantly.
**Cons**: requires touching the two test functions (trivial) and deciding whether they run under
`-race` at all in CI, or in a separate non-blocking job.
**Verdict**: **Recommended.** Apply the existing `//go:build integration` tag (or a
`testing.Short()` guard, consistent with sibling tests in the same package) to the two flaky
hook-URL/MCP-URL tests, rather than inventing a custom runner. This directly satisfies AC #2
("documented rationale, not a bare number bump") and AC #3 (explicit about what's narrowed) since
the existing convention's rationale is already documented at each of its other call sites.

---

## 3. Flaky test retry tooling

**Question**: off-the-shelf retry solutions vs. hand-rolled retry logic, and whether retry-on-flake
is even appropriate given AC #2/#3 (must not mask a real bug).

- **`gotestsum --rerun-fails`** (gotestyourself/gotestsum) is the standard, well-maintained
  (`gotest.tools/gotestsum`) Go-native option: reruns only the *failed* tests (not the whole
  suite), configurable attempt count (default 2), and is explicitly designed for this use case.
  Used by other Go projects (moby/dapr issues reference adopting it for the same reason). Would
  require adding `gotestsum` as a CI tool (single Go install, no source changes) and swapping the
  `go test` invocation in `build.yml` for `gotestsum --rerun-fails=N --packages=...`.
- **`go test -count=N`** re-runs the whole test binary N times but does not selectively retry only
  failures, and doesn't distinguish "flaky infra timeout" from "genuinely broken" — worse fit than
  gotestsum's targeted rerun.
- **`nick-fields/retry`** (formerly `nick-invision/retry`; ownership transferred Feb 2022) is a
  generic GitHub Action step-level retry — reruns the *entire* `go test` step/command on failure
  or timeout. Coarser than gotestsum (retries everything in the step, not just the failed test),
  but requires zero Go tooling changes, just a workflow YAML wrapper.
- **Hand-rolled retry inside the test** (e.g., a manual loop around the hook-injection wait) would
  bake CI-specific flakiness handling into product test code, coupling test logic to CI resource
  constraints — worse separation of concerns than either tool option.

**Is retry-on-flake appropriate here at all?** The requirements' own root-cause diagnosis says the
timeout is infra contention (`-race` CPU overhead under a shared runner), not a logic bug in the
hook-injection pipeline — so a bounded retry (via gotestsum, scoped to just these tests) is
defensible *as a stopgap*, provided AC #4's actual latency measurement confirms the timeout margin
is simply too tight for `-race` load, not that the pipeline itself is broken. Retry should not be
the primary or only fix — it should pair with (a) the build-tag isolation from point 2, so the
retry surface is small and intentional, and (b) a documented note that retry exists to absorb CI
runner variance, not to mask a real race in the hook-injection code.

**Pros of `gotestsum --rerun-fails`**: targeted (only failed tests), well-supported, minimal
integration cost, has an off-ramp (can be removed once timeout tuning + build-tag isolation prove
sufficient).
**Cons**: adds one more CI tool dependency; any retry tool risks masking a real intermittent bug
if adopted without the AC #4 measurement step first.
**Verdict**: **Viable, but conditional.** Recommend `gotestsum --rerun-fails` only as a
belt-and-suspenders addition *after* isolating the two tests via build tags and confirming (per
AC #4) that the remaining flakiness is infra-timing noise, not a real bug. Do not reach for it as
the first or only fix — AC #5 explicitly rules out treating a race-condition symptom with a
retry-shaped band-aid unless research shows the test infra, not the pipeline, is the bottleneck.

---

## 4. Observability: measuring actual pipeline latency under `-race` + CI load

**Question**: low-effort way to get real timing data out of CI vs. building custom instrumentation.

- **`go test -json`** (stdlib flag) emits structured per-test-event output including timestamps;
  this is the standard, zero-dependency way to get machine-readable timing.
- **`gotestsum --jsonfile=<path>`** captures that same `test2json` stream to a file, and
  **`gotestsum tool slowest --jsonfile <path> --threshold 500ms`** (a companion subcommand,
  already ships with the same tool from point 3) reports the slowest tests directly — exactly the
  "measure before choosing a fix" data AC #4 asks for, with no bespoke script.
  Recommend running one debug CI invocation (or a temporary workflow job) with a wider timeout that
  captures `--jsonfile` output, then feeding it to `gotestsum tool slowest` to get the actual
  hook-injection pipeline latency distribution under `-race` + concurrent-suite load — this is the
  single clearest way to satisfy AC #4 empirically rather than guessing.
- **GitHub Actions Job Summary (`$GITHUB_STEP_SUMMARY`)** is already used elsewhere in this repo's
  workflows (`.github/workflows/benchmark.yml:100-101`, feeding benchstat output into the summary)
  — the same pattern (pipe `gotestsum`'s human-readable summary or `tool slowest` output into
  `$GITHUB_STEP_SUMMARY`) would surface timing data directly on the PR/run page with no new
  infrastructure.
- **Custom instrumentation** (e.g., adding manual `time.Now()`/logging calls inside
  `CreateSession`'s hook-injection goroutine, or a bespoke log-scraping script) would work but
  duplicates what `go test -json` + `gotestsum tool slowest` already provides for free, and risks
  becoming permanent production-code instrumentation for what should be a one-time diagnostic pass.

**Pros of `go test -json` + `gotestsum tool slowest`**: zero custom code, reuses the same tool
already recommended for point 3 (one dependency, two uses), output can be captured once and
archived/attached to the requirements doc as the AC #4 evidence.
**Cons**: requires one throwaway/diagnostic CI run (or local repro with `-race` and background
load) to collect real numbers before deciding on a fix — this is a small amount of extra work but
is exactly what AC #4 mandates, not a workaround to avoid.
**Verdict**: **Recommended.** Use `go test -json` (optionally via `gotestsum --jsonfile` +
`gotestsum tool slowest`) for a one-off measurement pass, and pipe a summary into
`$GITHUB_STEP_SUMMARY` (a pattern this repo's `benchmark.yml` already establishes) rather than
building custom instrumentation.

---

## Summary Table

| # | Question | Verdict |
|---|---|---|
| 1 | GitHub larger/self-hosted runners | **Not recommended** — gated behind Team/Enterprise billing this repo doesn't have; self-hosting disproportionate |
| 2 | Build tags/`testing.Short()` vs. bespoke runner | **Recommended** — repo already has this exact idiom in use; apply it to the two untagged flaky tests instead of inventing new tooling |
| 3 | `gotestsum --rerun-fails` vs. hand-rolled retry | **Viable, conditional** — only after build-tag isolation + AC #4 measurement confirm this is infra noise, not a real bug |
| 4 | `go test -json`/`gotestsum tool slowest` vs. custom instrumentation | **Recommended** — zero-dependency stdlib flag plus one companion tool command gives the AC #4 latency data directly |

## Sources

- [GitHub Actions Pricing: Price Cuts, Backlash, and a Rapid Retreat](https://samexpert.com/github-actions-pricing-backlash-2026/)
- [Pricing changes for GitHub Actions · GitHub](https://github.com/resources/insights/2026-pricing-changes-for-github-actions)
- [Actions runner pricing - GitHub Docs](https://docs.github.com/en/billing/reference/actions-runner-pricing)
- [gotestsum/README.md at main · gotestyourself/gotestsum](https://github.com/gotestyourself/gotestsum/blob/main/README.md)
- [gotestsum command - gotest.tools/gotestsum - Go Packages](https://pkg.go.dev/gotest.tools/gotestsum)
- [8 Ways To Retry: Finding Flaky Tests - Semaphore](https://semaphore.io/blog/flaky-test-retry)
- [Bring in gotestsum and use --rerun-fails flag in CI and automation · Issue #6578 · dapr/dapr](https://github.com/dapr/dapr/issues/6578)
- [GitHub - nick-fields/retry: Retries a GitHub Action step on failure or timeout](https://github.com/nick-fields/retry)
- [Using GitHub Actions to summarise your Go tests | Chris Reddington](https://chrisreddington.com/blog/githubactions-testsummary-go/)
