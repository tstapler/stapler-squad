# Research: Build vs. Buy — ci-speed

Repo scale (measured, 2026-08-27, this worktree):
- Go: `find . -name "*.go" -not -path "./vendor/*" | wc -l` → **1392 files**, `wc -l` total → **423,927 LOC**.
- `web-app/`: `find web-app/src -type f \( -name "*.ts" -o -name "*.tsx" \)` → **1281 files**.
- `.github/workflows/`: 14 workflow files. Only 1 existing composite action (`.github/actions/prepare/action.yml`, generates protos/ent/web build) — most cross-cutting setup is still copy-pasted per job.
- `build.yml` has 9 jobs (`detect-changes`, `prepare`, `web-build-smoke`, `test`, `pty-race-regression`, `integration-coverage`, `build`, `install-check`, `benchmark-gate`); `grep -rl setup-go .github/workflows/` → **10 files** call `setup-go` independently, each with its own `cache: true` (Go's built-in per-job module/build cache via `actions/setup-go`, not shared across jobs).
- `timeout-minutes` is set in only **8 places across 3 files** (`benchmark.yml` ×6, `ux-analysis.yml` ×1, `build.yml` ×1) — 11 of 14 workflow files have no job-level timeout at all, so a hang defaults to GitHub's 6-hour cap.
- The tmux submodule build (`scripts/build-tmux.sh`) is cached independently in at least 3 separate jobs (`build.yml` ×2, `benchmark.yml`), each with its own `actions/cache@v4` step keyed similarly — a candidate for the artifact-sharing option below rather than N separate cache lookups.

---

## 1. Build-graph / incremental-build system

### Option A: Bazel + bzlmod (full adoption, Go + web-app)

**Maturity (2026, verified via search):** bzlmod is Bazel's default and now the *only* supported external-dependency system — Bazel 7 defaulted to it, Bazel 8 disabled legacy WORKSPACE by default, and **Bazel 9 removed WORKSPACE support entirely** ([bazel-contrib/rules_go bzlmod docs](https://github.com/bazelbuild/rules_go/blob/master/docs/go/core/bzlmod.md)). `rules_go` and Gazelle both support bzlmod. Remote caching is a documented, supported feature (`--remote_cache` against BuildBuddy/EngFlow-style backends), and GHA integration typically goes through `bazel-contrib/setup-bazel` plus a GitHub Actions cache backend for the Bazel disk/repo cache.

This project's own history is direct negative evidence: Bazel was previously used for a narrow tmux build path via `rules_foreign_cc` and was **removed in `b51b60eb1`** specifically because WORKSPACE-mode broke under the Bazel 9 migration. That is exactly the failure mode bzlmod fixes, but it also means whoever re-adopts Bazel here is doing so with zero in-repo track record of it working end-to-end — the removal wasn't "Bazel is bad," it was "we used it in the one mode that just got deleted."

**Pros:**
- The only option that gives true whole-graph incrementality across Go + protobuf + ent + tmux native build + web-app in one dependency graph, with correct cross-language caching (a change to `web-app/` wouldn't force Go retests, and vice versa, without hand-maintained `paths:` filters).
- Bzlmod + rules_go + Gazelle is a real, current, actively maintained combination, not a 2023-era dead end.
- Remote caching would help wall-clock and cost, though this project's runner constraint (`ubuntu-latest` only, free tier) rules out a paid remote-execution/cache backend — a GHA-cache-backed disk cache is available but far weaker than BuildBuddy-style RBE, so most of Bazel's biggest win (remote execution) is off the table under this project's constraints anyway.
- rules_js/aspect-build exists for the TS/pnpm side, so the ecosystem gap that used to block "Bazel for JS" is closed.

**Cons:**
- Migration effort is large relative to this project's appetite. At 1392 Go files / 424k LOC and 1281 web-app TS/TSX files, hand-writing BUILD files is a non-starter; Gazelle auto-generates Go BUILD files reasonably well, but ent's generated code (`session/ent/*.go`, explicitly gitignored and regenerated per-build per this repo's own CLAUDE.md) and buf-generated proto code are exactly the kind of codegen-output-as-build-input case that is fiddly to wire into Gazelle correctly (need custom Gazelle extensions or manual `genrule`s for `buf generate` and the ent codegen step, plus the native tmux submodule build that was already tried and pulled out once).
- rules_js/aspect-build for a 1281-file pnpm-based Next.js-style web-app is a much rougher, less-traveled path than rules_go — pnpm-specific tooling (workspace protocol, `pnpm-lock.yaml` translation into Bazel's lockfile model) has more sharp edges and fewer worked examples than the Go side.
- Every engineer and every LLM-driven agent working in this repo (this repo is "written almost entirely with Claude Code" per its own CLAUDE.md) now has to reason in Bazel's model in addition to `go build`/`pnpm`, raising the ongoing cognitive/tooling tax for a repo that isn't primarily optimizing for polyglot-monorepo build-graph correctness.
- The project's own prior removal of Bazel for a *narrower* use case than "the whole build graph" is strong evidence the cost of keeping a Bazel setup current with upstream migrations (bzlmod, and whatever comes after) recurs and has already bitten this repo once.
- 3–6 week appetite for the *whole* CI-speed initiative — a full Bazel migration alone is plausibly a multi-week project on its own, leaving little budget for the other requirements (duration gate, path-triggering, concurrency tuning).

**Verdict: Not recommended** for this project's scope/appetite. The win (whole-graph incrementality) is real but this repo already has direct scar tissue from a narrower Bazel adoption breaking under a Bazel-version migration, the free-runner constraint neuters Bazel's best feature (remote execution), and full adoption at this scale would likely consume most or all of the 3–6 week appetite on tooling migration risk rather than measured CI-time reduction. Revisit only if a future initiative has its own dedicated appetite and a stronger multi-team monorepo-governance driver, not as a sub-task of a CI-speed project.

### Option B: Turborepo/Nx for `web-app/` + Go's native build cache via `actions/cache`

**Maturity (2026, verified via search):** Turborepo remains the simpler, lower-config option for a single pnpm workspace; its free-tier remote caching depends on a Vercel account (not required, since GHA's `actions/cache` can back Turborepo's cache instead, at the cost of losing cross-run analytics). Nx is positioned for multi-team/multi-app-type repos needing project-boundary enforcement and its own cloud caching — a heavier tool than this repo's single `web-app/` package currently needs (`ls web-app/` shows a single `package.json`/`src/`, not a multi-package pnpm workspace with several publishable packages).

**Pros:**
- Scoped to exactly the piece of the repo (`web-app/`) with real task-level caching opportunity (typecheck, lint, jest, build, Playwright bundling) — no need to touch the Go/proto/ent/tmux side of the graph at all.
- Go already has its own incremental build cache built into the toolchain (`GOCACHE`, `GOMODCACHE`) at zero cost — the deficiency here (per requirements.md's own observation about `build.yml`'s ~9 jobs each independently calling `setup-go`+`cache: true`) is that this cache isn't being *shared* well across jobs, not that Go lacks one. Fixing that is Option D, and it's orthogonal to whatever the web-app does.
- Low blast radius: adopting Turborepo (or Nx) in `web-app/` doesn't touch the Go build, proto/ent codegen, or the tmux native build — it only changes how `pnpm` scripts are invoked and cached.
- Turborepo's config surface (`turbo.json` task graph + `outputs` globs) is small enough to land within days, not weeks, given a single-package `web-app/`.

**Cons:**
- `web-app/` is (from `ls web-app/`) a single Next.js-style app, not a true multi-package pnpm workspace — Turborepo/Nx's core value proposition (affected-package detection, cross-package task graphs) has much less to bite on here than in a repo with many `packages/*`. The main win reduces to "cache jest/lint/typecheck/build outputs keyed on file hashes," which is a real but narrower win than the marketing case for either tool.
- Adds a new dependency + config file + mental model (`turbo.json` or `nx.json`) for a benefit that a well-tuned `actions/cache` step keyed on `web-app/pnpm-lock.yaml` + source hash can approximate for this specific (non-multi-package) repo shape.

**Verdict: Viable**, but only worth it if `web-app/`'s CI time (not separately broken out in the baseline table, folded into `build.yml`'s ~18m average and `ux-analysis.yml`/`lint.yml`) is shown by the root-cause phase to be a material contributor *and* the team plans to grow `web-app/` into a real multi-package workspace. Otherwise Option C/D capture most of the available win at far lower cost for a single-package frontend.

### Option C: Fix cache-key correctness + job/artifact sharing in existing GHA workflows (no new tool)

**Pros:**
- Zero new dependencies, zero new mental model, and directly targets the concrete inefficiency requirements.md and this repo's own workflow files already show: 10 separate `setup-go` invocations each independently downloading/rebuilding the Go module+build cache, and the tmux submodule rebuilt+cached independently in 3+ jobs.
- Matches this repo's own engineering-discipline norms (`.claude/rules/prefer-go-git-over-subshells.md` etc.) of preferring the simplest correct fix over introducing new tooling layers, and the existing `.github/actions/prepare/action.yml` composite action shows the pattern is already half-adopted — extending it (or adding a "build once, `actions/upload-artifact`, download in dependent jobs" step) is incremental, not a new paradigm.
- Directly actionable this week — no learning curve, no new marketplace-action supply-chain surface.

**Cons:**
- Doesn't solve the *architectural* problem Option A would (whole-graph, path-aware incrementality) — it's a bounded, job-topology fix, not a build-graph rethink. If the root-cause research (Agent 1's job) finds most of the cost is genuinely wasted redundant work rather than legitimately-needed matrix breadth, this alone may not hit the "cut typical PR wall-clock ~50%" target without also doing concurrency tuning, matrix reduction, and path-based triggering (all separately in scope per requirements.md, not competitors to this option).

**Verdict: Recommended** as the baseline, must-do action regardless of what else ships — it's the lowest-risk, most repo-idiomatic fix for the specific redundancy requirements.md already identified, and it doesn't preclude adopting Option B or D on top of it.

### Option D: Dedicated Go-cache-sharing / build-once-reuse GitHub Action

**Verified via search:** `actions/setup-go`'s built-in `cache: true` already implements the `~/.cache/go-build` + `~/go/pkg/mod` caching pattern keyed on `go.sum` by default, with a `cache-dependency-path` input for multi-module/monorepo layouts ([actions/setup-go](https://github.com/actions/setup-go), [Better GitHub Actions caching for Go](https://danp.net/posts/github-actions-go-cache/)). Marketplace wrappers like `magnetikonline/action-golang-cache` exist but mostly just combine `setup-go` + `actions/cache` — i.e. they package Option D's own idea, not something meaningfully beyond it.

**Pros:**
- The "shared build cache" half of this is not something to build or buy separately — it's already a built-in `actions/setup-go` feature; the fix is *using it correctly* (consistent `cache-dependency-path`, not re-downloading modules fresh in every one of the 10 jobs that call it) rather than adding a new tool.
- The "build once, reuse binary" half (`actions/upload-artifact` / `download-artifact` to compile once in `prepare`/`build` and reuse the binary in `install-check`, `benchmark-gate`, etc.) is a first-party GitHub Action already available, zero supply-chain risk beyond what's already trusted.

**Cons:**
- Artifact upload/download has its own overhead (compression, transfer) that can eat into the savings for small binaries/short jobs — needs measuring per job, not assumed universally net-positive.

**Verdict: Recommended**, folded into Option C's implementation (this is the mechanism, not a separate product decision) — use first-party `actions/cache` + `actions/upload-artifact`/`download-artifact`, not a third-party wrapper action, since the wrappers add a supply-chain dependency for no capability beyond what's already free and native.

**Overall Decision 1 recommendation:** C+D now (cache-key correctness, `cache-dependency-path` fix, and build-once/reuse-artifact across `build.yml`'s 9 jobs); B (Turborepo) only if root-cause data shows `web-app/`'s own tasks are a material, currently-uncached cost center; A (Bazel) not recommended for this initiative's appetite.

---

## 2. CI-duration budget/gate mechanism

### Option A: Custom shell/Go script step checking `steps.x.outputs.time` per workflow

**Verdict: Not recommended.** Requires hand-instrumenting every job with start/end timestamp capture and a comparison step, duplicated per workflow (14 files) — this is exactly the kind of bespoke, per-file logic requirements.md's own "duration budget/gate" goal should avoid reinventing when GitHub already exposes `timeout-minutes` natively (see Option C) for the hard-fail case, and the GitHub Actions REST API for the trend/soft case (see Option D). A custom implementation here is also the highest-maintenance option: it has to keep working across all 14 workflow files as they change, with no upstream keeping it current.

### Option B: Existing GitHub Marketplace action for step/job timing + budget enforcement

**Verified via search:** `komiya-atsushi/action-enforce-timeout-minutes` ([GitHub Marketplace listing](https://github.com/marketplace/actions/enforce-timeout-minutes)) exists and does roughly the inverse of a duration gate — it's a **lint-time** check that every job declares a `timeout-minutes:` at all (catching the "11 of 14 workflow files have no job timeout" gap found above), not a runtime "this run took too long, fail/flag it" gate. It's a single-maintainer action (KOMIYA Atsushi) with no verified update/star signal found in this search pass — for a CI-critical-path dependency, that's a supply-chain risk worth naming explicitly (single point of failure, unclear response time to a breaking Actions runtime change) even though the action's *scope* (checking YAML for a missing key) is low-risk to what it can do if compromised.

**Verdict: Viable only for the narrow "did every job set a timeout" lint**, and even then, an equivalent check is few enough lines of YAML/`yq`/`grep` (this research already used `grep -rn timeout-minutes .github/workflows/*.yml` to compute the "8 of 14 files" gap in under a second) that writing a 10-line repo-owned check (Option A-lite, but scoped to *this one job* rather than a full custom timing system) carries less supply-chain exposure than a single-maintainer marketplace action for equivalent capability.

### Option C: `timeout-minutes:` tightened to realistic per-job budgets

**Pros:**
- Zero new dependencies — native GHA feature already partially in use in this repo (`benchmark.yml`, `ux-analysis.yml`, `build.yml`).
- Directly closes the gap this research found: 11 of 14 workflow files currently have **no** job-level timeout, defaulting to GitHub's 6-hour cap — meaning a hung job (e.g., a flaky e2e test that never resolves) can silently burn up to 6 hours of billed runner time with zero automated signal, which directly undermines the "tame tail latency" success metric (the baseline table's own Max column shows workflows like `demo-publish.yml` at 202m and `build.yml` at 81.5m — tail runs already several multiples of the average, exactly the pattern an absent or loose timeout allows to run unchecked).
- Sets a hard ceiling per job derived from the baseline table's own Max column (e.g., `build.yml`'s `test` job budgeted at ~2x its historical max, not an arbitrary round number) — this *is* the "CI-duration budget/gate" requirement, expressed the way GitHub's own model already supports it, satisfying requirements.md's explicit ask for a gate without inventing new infrastructure.

**Cons:**
- A hard `timeout-minutes` only catches runaway/hung jobs, not gradual regression (a job that's slowly creeping from 10m to 18m over months, still well under any reasonable timeout ceiling) — it's a ceiling, not a trend detector. That's a real gap relative to "tame tail latency" as a *statistical* property, not just a worst-case cap.

**Verdict: Recommended** as the hard-fail half of the gate — lowest cost, native, directly closes a real, measured gap (11/14 files with no timeout) that is itself a tail-latency risk independent of the CI-speed initiative's other goals.

### Option D: Scheduled workflow pulling historical run durations via `gh api`/REST API for trend regression

**Pros:**
- Directly complements Option C: catches the slow-creep case Option C's hard ceiling cannot (a job trending upward across weeks while staying under its timeout). GitHub's REST API (`GET /repos/{owner}/{repo}/actions/runs`) already exposes per-run duration data with no new external service.
- A scheduled workflow (e.g. weekly) using `gh api` matches this repo's own stated preference for using GitHub's native APIs over bespoke instrumentation (see Decision 3 below) and can post a job summary or open/update a tracking issue on regression — no new marketplace action needed, `gh` is already a trusted, first-party CLI.

**Cons:**
- Genuinely new code to write and maintain (a script that queries the API, computes a rolling baseline, and decides "regression" vs. noise) — this is the one place in Decision 2 where *some* custom logic is justified, because there is no existing action doing "trend-detect against this repo's own historical run data" at the narrow scope needed, and the logic (compare, don't gate) is low-risk if it has bugs — worst case it's a false/missed alert, not a broken required check.

**Verdict: Recommended** as the soft/trend-detection half, implemented as a small, repo-owned script (justified custom code per Decision 3's criteria) rather than a marketplace dependency, since no well-fitted existing tool was found for "regression-detect this specific repo's own run-duration history."

**Overall Decision 2 recommendation:** C (tightened `timeout-minutes` per job, sized off the baseline table's Max column) for the hard gate + D (a small scheduled `gh api`-based trend script) for the soft/regression signal. Skip A (redundant custom instrumentation) and treat B as optional/low-priority (a "lint that timeout-minutes is set" check whose value is mostly subsumed by actually setting them all in C).

---

## 3. LLM-generated vs. battle-tested for custom tooling

Any custom script this project ends up writing (the Decision 2/Option D trend script is the one clear case) should be scoped as narrowly as possible and built on GitHub's own primitives rather than reimplementing what they already provide:

- **Prefer `timeout-minutes`, job summaries (`$GITHUB_STEP_SUMMARY`), and `::warning::`/`::error::` annotations over bespoke pass/fail logic.** These are maintained by GitHub itself, versioned with the Actions runner, and every contributor/agent working in this repo already understands them — there's no correctness risk to own, because there's no logic to get wrong (a `timeout-minutes: 20` either does or doesn't fire; a hand-rolled "did this step take too long" bash comparison can have off-by-one/timezone/formatting bugs that a native feature can't).
- **Custom code is justified only where no native/existing feature does the job at all** — per Decision 2, that's exactly the "detect a multi-week trend against this repo's own run-history" case, because GitHub's REST API gives raw data but no analysis. That script should be small (a query + a rolling-average comparison), read-only (it should never be able to fail a PR's required check — trend detection is advisory, not blocking, so a bug in it can't block shipping), and covered by this repo's own `fix-flaky-tests-dont-defer.md`-style discipline: if it's ever wrong, root-cause and fix it rather than silencing it.
- **Reckless territory:** hand-rolling a duration *gate* (blocking, not advisory) in custom code when `timeout-minutes` already exists natively — that duplicates a native feature, adds a maintenance surface, and any bug in the custom version (e.g., a race reading `steps.x.outputs.time` before the step finishes) could either silently fail to gate (defeating the purpose) or falsely block an unrelated PR (violating the "must not break branch-protection required checks" constraint from requirements.md). The custom code's blast radius here is strictly worse than the native feature's, for no capability gain.

**Verdict:** custom code is justified for the one case with no native equivalent (historical-trend analysis), and should be advisory-only; everything else in Decision 2 should be native GHA features, not custom implementations.

---

## 4. Fork/adapt: a close-enough OSS repo's CI setup to study

Searches for a public repo combining Go backend + React/TypeScript frontend + protobuf(buf) + Playwright e2e + GHA did not turn up a single clean, verified match with all four elements confirmed simultaneously in this research pass (search results surfaced smaller/partial matches like `sudorandom/protodocs`, which has Go+pnpm+buf+Playwright but is a small single-purpose doc-generation tool, not comparable in CI complexity to this repo's 14-workflow, 9-job `build.yml` setup).

**INFERRED (not verified in this search pass, flagging for follow-up rather than asserting as fact):** `coder/coder` (github.com/coder/coder) is a large, actively-maintained OSS project with a Go backend, a React/TypeScript frontend (`site/`), Playwright e2e tests (`site/e2e/`), and protobuf usage for its agent API — a plausible shape match for "Go+TS+protobuf+e2e" at meaningfully larger CI complexity than the small repos this search surfaced. This is based on general knowledge of the project, not a verified read of its current `.github/workflows/` — before treating it as a reference, actually clone/read `coder/coder`'s live `.github/workflows/` directory to confirm it still matches (protobuf tooling choice, job topology, caching strategy) rather than relying on this inference.

**Verdict:** **Not confidently recommended as a ready-made template** on the evidence gathered here — no verified match was found. If the planning phase wants a fork/adapt input, spend a small, timeboxed spike (a few hours, not a research track) actually opening `coder/coder`'s current `.github/workflows/` (and optionally `temporalio/temporal`'s, another Go+TS-adjacent large OSS project) to check for transferable patterns — job-splitting strategy, cache-key design, path-based triggers — rather than treating either as validated in this document. Absent a confirmed match, root-cause analysis of *this* repo's own workflows (already partially done above) is the more reliable input to the plan than pattern-matching an unverified external repo.
