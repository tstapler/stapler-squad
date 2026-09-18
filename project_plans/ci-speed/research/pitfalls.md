# Research: Pitfalls — What Goes Wrong Speeding Up GitHub Actions CI

Scope: what commonly breaks when teams optimize GHA CI, checked against this repo's actual
`.github/workflows/*.yml`, `.github/actions/prepare/action.yml`, and live repo state (`gh api`,
`gh run list`). Every repo-specific claim below was verified against a file, a `gh`/`git` command
output, or both — not assumed from the requirements doc's baseline table.

## 0. Headline finding: the requirements.md baseline for `demo-publish.yml` is stale

The requirements doc frames `demo-publish.yml`'s 136m avg / 202m max as "a clear outlier, root
cause unconfirmed" and lists it in scope for systematic fixing. Both parts of that framing need
correcting before any plan spends appetite on it:

- **Root cause is not mysterious.** `demo-publish.yml`'s "Record demo flows" step runs
  `npx playwright test --reporter=list || true` with **no spec argument** — the full 92-file
  `tests/e2e/*.spec.ts` suite — while `tests/e2e/playwright.config.ts` sets `workers: 1` and
  `retries: 1` (`fullyParallel: false`). That's the entire E2E suite, serially, with a retry
  multiplier, run twice (`demo-publish.yml:70`). Contrast `e2e-video.yml`, which explicitly
  filters to 5 `FEATURE_SPECS` (`e2e-video.yml:30-35`) and shards 2-way — the fix pattern already
  exists in the same repo, one workflow just doesn't use it.
- **It's already been effectively neutralized.** `git log --follow -p -- .github/workflows/demo-publish.yml`
  shows commit `be238fa6811c3d39698fe5f017e1eec575201013` ("chore(ci): make demo GIFs
  manual-only", 2026-07-04) changed the trigger from `push: {branches: [main]}` to
  `workflow_dispatch:` only. `gh run list --workflow=demo-publish.yml --limit 15` confirms the
  last 15 runs — the exact window the baseline table's "last ~15 runs each" is built from — are
  **all `push`-triggered, dated 2026-06-30 through 2026-07-04**, i.e. entirely from before the
  cutover. There have been **zero** `workflow_dispatch` runs of this workflow since. It does not
  fire on PRs, does not fire on push to main today, and has not run at all in the ~7.5 weeks
  before this project's baseline was captured.

**Pitfall to design against:** treating a baseline number as current behavior without checking
whether the workflow's trigger config changed after the measurement window. A plan that spent
part of its 3–6 week appetite "fixing" `demo-publish.yml`'s duration would be solving an already-solved
problem for a workflow that no longer runs automatically — the actual gap (if any) is a one-line
`--spec demo.spec.ts` scoping fix for whenever someone next dispatches it manually, not a
research project. This also means the "cut typical per-PR wall-clock time-to-green ~50%"
success metric should exclude `demo-publish.yml` from its baseline entirely — it isn't part of
per-PR time-to-green today. Re-verify the other 10 workflows' "last 15 runs" windows the same
way (`gh run list --workflow=<file> --json createdAt,event,conclusion`) before trusting any of
the baseline table for planning purposes; this is the general form of the pitfall, demo-publish
is just the worst instance.

## 1. GitHub Actions cache pitfalls

**Verified in this repo:**

- **11 of 13 workflow files call `actions/cache`/`setup-go cache:true`/`setup-node cache:'pnpm'`
  independently** (`build.yml` alone has 9 separate `setup-go`+cache and 4 separate "Cache pinned
  tmux binary" steps, one per job — `test`, `pty-race-regression`, `integration-coverage`,
  `build`). Each is its own cache scope, so a Go-module cache warmed in one job is invisible to
  the others unless the key is identical — worth auditing whether these 9 setup-go calls could
  collapse to one `prepare`-style shared artifact instead of 9 independent Go module/build caches
  competing for the same 10GB repo-wide pool.
- **The "pinned tmux binary" cache key is `tmux-3.4-${{ runner.os }}-v1`** (`build.yml:243`,
  repeated verbatim in 3 more jobs) — static, no lockfile/source hash component. That's
  intentional (it's a pinned upstream tarball build, not derived from repo source), but it means
  a cache poisoning or corruption bug in that entry would silently serve a bad `bin/tmux` to
  every job forever until the `-v1` suffix is manually bumped — the only invalidation lever is
  human memory, not a hash of `scripts/build-tmux.sh` or the pinned tmux version string. If either
  ever changes without someone remembering to bump `-v1`, every consuming job restores stale
  tmux silently (`cache-hit == 'true'` still short-circuits the rebuild step at
  `build.yml:246`).
- **`benchmark.yml`'s 4 baseline caches (`bench-tier1-`, `bench-tier2-`, `bench-frontend-throughput-`,
  `bench-e2e-latency-`) use the documented immutable-key workaround** — `restore-keys:
  bench-<name>-` (prefix match, always resolves to the most recent) paired with `key:
  bench-<name>-${{ github.run_id }}` on save (`benchmark.yml:88-94`, `173-178`, etc.). This is the
  *correct* pattern for actions/cache's write-once semantics, but it means **every push to main
  creates a new, never-explicitly-deleted cache entry** in 4 independent accumulating series, on
  top of every other cache in the repo (buf binary, tmux binary, pnpm store, Next.js `.next/cache`,
  Go module/build cache × 9). All of it draws from one shared 10GB-per-repo pool with LRU
  eviction. If a large, frequently-invalidated cache (pnpm store, Go build cache) balloons, it can
  evict an older-but-still-referenced `bench-*` baseline entry — the workflow won't fail (restore
  with no match just runs without a baseline: no explicit `if: steps.cache.outputs.cache-hit`
  gate on the restore side), it will **silently compare against whatever's left**, degrading the
  regression gate's signal without any red X to notice. GitHub raised the default limit past 10GB
  in Nov 2025 ([GitHub Changelog](https://github.blog/changelog/2025-11-20-github-actions-cache-size-can-now-exceed-10-gb-per-repository/)),
  so this is a real lever the plan can pull, but the default is still 10GB until raised.
- **Cache poisoning via `pull_request_target`: not applicable here** — grepped all workflow
  `on:` blocks; nothing in this repo uses `pull_request_target`, so the TanStack-incident class
  of attack (fork PR writes into the base-branch cache scope) isn't a live risk. Worth stating
  explicitly so a future contributor doesn't "fix" a non-problem, and worth a standing constraint
  that any new workflow **must not** introduce `pull_request_target` + `actions/cache` write
  together.
- **Scoping rule to design against:** a cache written on a PR branch is readable only by that
  same branch and (read-only) the base/default branch's cache — a PR run can never see another
  PR's cache. For this repo's per-job Go/pnpm caches that's fine (the default-branch fallback
  covers the common "first run on a new PR branch" cold-start), but any *new* cache key the plan
  introduces that's meant to be shared across all PRs (e.g., a cross-PR build-artifact cache) will
  silently miss 100% of the time for exactly this reason — a well-known gotcha, not a bug to
  chase.

Sources: [GitHub cache eviction policy changelog](https://github.blog/changelog/2025-09-29-new-date-for-enforcement-of-cache-eviction-policy/), [cache size >10GB changelog](https://github.blog/changelog/2025-11-20-github-actions-cache-size-can-now-exceed-10-gb-per-repository/), [GitHub dependency caching reference](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching), [TanStack cache-poisoning incident writeup](https://safedep.io/tanstack-github-actions-cache-poisoning/), [GitHub cache isolation discussion #194493](https://github.com/orgs/community/discussions/194493).

## 2. Concurrency / path-filter pitfalls

**Verified in this repo:**

- **Branch protection is currently NOT configured on `main`.** `gh api
  repos/tstapler/stapler-squad/branches/main/protection` returns `404 "Branch not protected"`,
  and `gh api repos/tstapler/stapler-squad/rulesets` returns `[]`. This changes the *severity* of
  every pitfall in this section: today, nothing on GitHub's side actually blocks a merge on a
  missing/pending check, so a path-filter or job-rename mistake would not currently produce the
  classic "PR permanently unmergeable" failure mode — it would just silently reduce coverage
  (a PR merges without ever having run a check it should have). **If/when branch protection or a
  ruleset is added** (a reasonable thing for this plan to recommend, since "green means
  mergeable" is stated as a hard constraint in requirements.md even though nothing enforces it
  yet), every pitfall below becomes immediately live. The plan should treat "add branch
  protection with required checks" and "make sure required checks survive path filters/job
  renames" as one unit of work, not sequence them with a gap in between.
- **Only 3 of 13 workflows set `concurrency:`**: `benchmark.yml` (`cancel-in-progress:
  github.ref != 'refs/heads/main'` — correctly never cancels on main, where baseline-save steps
  must finish), `ux-analysis.yml` and `e2e-video.yml` (both unconditional
  `cancel-in-progress: true`, scoped per-PR-number). `build.yml`, `lint.yml`,
  `mcp-integration.yml`, and the rest have no `concurrency:` block at all, meaning two pushes to
  the same PR within the run window queue and both run to completion today — wasted spend, but
  not a correctness risk. Adding `concurrency: cancel-in-progress: true` to these is an easy win,
  but must exclude any job whose failure-to-complete would corrupt shared state (see `benchmark.yml`'s
  own comment at line 27-30 for why it deliberately excludes main-branch runs from cancellation —
  a baseline-save cache write that gets killed mid-flight leaves a half-written or missing cache
  entry, not a corrupt one, since `actions/cache/save` uploads atomically, but a *skipped* one).
- **6 of 13 workflows use `paths:`/`paths-ignore:` filters** (`benchmark.yml`, `build.yml`,
  `lint.yml`, `deploy-pages.yml`, `goreleaser-check.yml`, `mcp-integration.yml`,
  `registry-validation.yml`, `ux-analysis.yml`). None of these are currently wired as required
  checks (since none are required at all yet — see above), so today a path-filtered workflow
  simply not running is invisible. The moment any of these becomes a required check, GitHub's
  own documented failure mode applies directly: *"if a job is skipped because its workflow wasn't
  triggered [by a path filter], making the job status 'required' will block merges"* — the check
  sits at "Expected — Waiting for status to be reported" forever. `build.yml` already works
  around exactly this for its internal `prepare`/downstream jobs using an explicit
  `detect-changes` job + `if: always() && (... || needs.detect-changes.outputs.relevant ==
  'true')` pattern (`build.yml:75-86`, with a comment citing `actions/runner#2205` for why
  `always()` is required) — that pattern is the template to extend to any workflow the plan wants
  to make a required check while keeping its `paths:` filter, rather than reaching for GitHub's
  own `on.paths` filter for a workflow meant to gate merges.
- **Recommended pattern when this plan does add required checks with path filters:** either (a)
  give every path-filtered workflow a cheap always-runs "detect changes → conditionally run real
  job, always run a trivial pass-through job" shape (as `build.yml` already does internally), or
  (b) require a single aggregate "CI passed" job that itself has no path filter and fans out to
  the real jobs — the second is what GitHub's own troubleshooting doc and multiple community
  threads converge on as the durable fix, since it also sidesteps the job-rename pitfall in
  Section 4 (one required check name, never renamed, regardless of how the underlying job graph
  is restructured).

Sources: [GitHub troubleshooting required status checks](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks), [community discussion #54877 — branch protections when actions use paths-ignore](https://github.com/orgs/community/discussions/54877), [community discussion #13690 — required actions should only be required if run](https://github.com/orgs/community/discussions/13690), [community discussion #26698 — stuck "Waiting for status to be reported"](https://github.com/orgs/community/discussions/26698), [GitHub concurrency docs](https://docs.github.com/en/enterprise-cloud@latest/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency), [community discussion #32376 — concurrency group cancelled instead of pending](https://github.com/orgs/community/discussions/32376).

## 3. Bazel / bzlmod pitfalls (for evaluating, not yet adopting)

Repo-specific context: the only prior Bazel usage here was a narrow, never-CI-exercised tmux
build path, removed because "WORKSPACE-mode is broken under Bazel 9" (per requirements.md). A
repo-wide grep confirms **no `WORKSPACE`, `WORKSPACE.bazel`, `MODULE.bazel`, or `.bazelrc` files
exist anywhere in the tree today** — so there's no latent WORKSPACE-mode config left to trip over
a second time; a bzlmod evaluation would start from zero, not from a half-migrated state.

- **Bazel 7 made bzlmod the default; Bazel 8 disabled WORKSPACE by default; Bazel 9 removed
  WORKSPACE support entirely** — confirming the stated root cause of the prior removal was a
  real, dated platform shift, not a one-off misconfiguration, and that any future adoption must
  target bzlmod from day one (no WORKSPACE fallback exists to lean on if bzlmod hits a wall).
- **Go via Gazelle**: `bazel-gazelle`'s `go_deps` module extension does Minimum Version Selection
  over `go.mod`'s transitive graph, which is the easy part — this repo's `go.mod`/`go.sum` stay
  the source of truth and Gazelle regenerates `BUILD.bazel` files from them. The **maintenance
  burden is running Gazelle on every dependency or package-layout change** (a CI check, not a
  one-time cost) — for a repo already running `make build && make test`, `make lint`,
  `make ent-gen`, and `make registry-generate` as generation steps that must be re-run and
  committed (or CI-checked) on every relevant change, this is a 5th such treadmill.
- **No equivalent of Gazelle for the pnpm/TypeScript side is standard or mature** — `web-app/`'s
  `pnpm-lock.yaml` has no widely-adopted Bazel BUILD-file generator comparable to Gazelle for Go;
  teams doing Bazel+JS typically hand-maintain `rules_js`/`aspect_rules_js` targets or use
  `pnpm`'s own workspace tooling instead of Bazel for the frontend half, which would mean a
  **split build system** (Bazel for Go, pnpm-native for web-app) rather than the unified graph
  the requirements doc's "full bzlmod adoption for the whole build graph" framing implies — that
  framing itself may not be achievable cleanly given current tooling maturity, which is worth
  surfacing to the planning phase as a scope-narrowing candidate (Go-only Bazel, or no Bazel).
- **Cache/CI-specific pitfalls reported for 2026 adoptions**: "cache collision when many jobs
  append to the same `--disk_cache` path while downloading into `--repository_cache` on one
  SSD — latency spikes look like 'slow Bazel' but are queue depth," and "git depth mismatch"
  bugs when CI does a shallow checkout but a `git_repository` rule expects full history/tags.
  Both are directly relevant to `ubuntu-latest` free runners, which have limited, shared local
  SSD — remote caching (BuildBuddy/EngFlow-style, `--remote_cache`) becomes close to
  load-bearing infrastructure once adopted, which is new operational surface (an external
  service, auth tokens, availability dependency) this repo doesn't currently have for any other
  part of its build.
- **Runner disk/memory**: `ubuntu-latest` free runners give ~14GB RAM and ~14GB free disk
  (SSD-backed but shared with the OS/toolchain image) — a from-scratch Bazel remote-cache-cold
  build of a repo with protobuf codegen + ent ORM codegen + a native tmux build + a full
  Next.js/pnpm frontend is a plausible way to blow that budget on a cache-miss run, exactly the
  tail-latency problem (18m avg / 81.5m max) this project is trying to fix for `build.yml` today
  — bzlmod doesn't remove that risk, it relocates it into Bazel's own remote-cache hit rate.

**Net for the plan**: treat bzlmod as a large, standalone spike with its own explicit go/no-go
gate (per the requirements doc's own "Rabbit Holes" list), not a default direction — the
concrete pitfalls above (split build system for JS, remote-cache as new infra, Gazelle as a 5th
codegen treadmill) are the reasons a lighter build-cache tool (e.g., simply better `actions/cache`
keying, `sccache`/`ccache`-equivalent for Go build cache, or Turborepo/Nx-style remote caching
scoped to just `web-app/`) should be evaluated side-by-side before committing appetite to Bazel.

Sources: [rules_go bzlmod docs](https://github.com/bazelbuild/rules_go/blob/master/docs/go/core/bzlmod.md), [bazel-gazelle](https://github.com/bazel-contrib/bazel-gazelle), [2026 Bazel remote-cache CI matrix pitfalls](https://macpull.com/blog/articles/2026-remote-mac-bazel-external-deps-remote-cache-ci-matrix.html), [Bazel caching/remote execution overview](https://www.incredibuild.com/blog/bazel-caching-remote-execution-and-the-build-supply-chain).

## 4. Job/matrix consolidation pitfalls

- **Required-check-by-name breakage is the single most common self-inflicted "why won't my PR
  merge" bug** in the GHA ecosystem: *"required status checks match by exact reported name, not
  by workflow file identity — renames silently break the match... a required check that doesn't
  get triggered (because it has been renamed) blocks forever."* Since this repo has no required
  checks configured today (Section 2), this is currently a **zero-consequence** mistake to make —
  but it becomes a live landmine the instant the plan (correctly) recommends turning branch
  protection on. **Sequencing matters**: any job renames/merges this plan wants to do (e.g.
  collapsing `build.yml`'s 9 separate `setup-go` jobs, or merging `test`/`pty-race-regression`/
  `integration-coverage`) should happen *before* required checks are configured, or be
  accompanied in the same change as updating the ruleset/branch-protection required-check list —
  never left as a follow-up.
- **The single-aggregate-check pattern is the standard mitigation** cited across multiple sources:
  one required check (e.g. a final `ci-passed` job with `needs: [...]` and `if: always()`) that
  fans out internally, so future job renames never require a branch-protection edit. Given this
  repo will likely add its first-ever required checks as part of this plan (Section 2), it's worth
  adopting this pattern from the start rather than pointing individual job names (`test`, `lint`,
  `build`) at branch protection and inheriting the rename-fragility immediately.
- **Consolidation can unmask flakiness previously hidden by generous per-job timeouts.**
  `benchmark.yml` alone has per-job timeouts ranging 20m–50m (`timeout-minutes: 20/45/50/40`,
  lines 56/189/231/289/476/544) and `build.yml`'s `benchmark-gate` is 45m — collapsing jobs that
  each currently get their own generous, independently-tuned timeout into one merged job means
  the merged job's timeout has to be re-derived (not just summed — that would make total-suite
  hangs take even longer to fail loud), and any test that was marginally flaky under one job's
  resource contention may become reliably flaky once it's sharing a runner with what used to be a
  separate job's workload. Per `.claude/rules/fix-flaky-tests-dont-defer.md`, any flake newly
  exposed by consolidation must be root-caused/fixed or filed immediately — not re-excused as
  "consolidation side effect."
- **`generated-proto-guard.yml`'s baseline (35.6m max on a ~5m avg workflow) is itself a tail-latency
  outlier worth flagging as a candidate root-cause target**, since a guard checking generated-file
  drift ballooning to 7x its average smells like exactly the kind of cache-miss/retry pattern this
  plan is meant to find — confirm via `gh run list --workflow=generated-proto-guard.yml` before
  assuming a fix, following the same stale-baseline-checking discipline as Section 0.

Sources: [GitHub troubleshooting required status checks](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks), [devopsinterviewkb — required check stuck "Expected"](https://devopsinterviewkb.com/questions/github/branch-protection/required-status-check-stuck-expected-forever), [community discussion #42874 — renamed workflow keeps running old name](https://github.com/orgs/community/discussions/42874).

## 5. Test-splitting/sharding pitfalls

**Verified in this repo:**

- **`e2e-video.yml` already shards 2-way** (`matrix.shard: [1, 2]`, `--shard=${{ matrix.shard
  }}/2`). Playwright's default sharding splits **by test file count, not by historical
  duration** — with only 5 `FEATURE_SPECS` split across 2 shards, an uneven file-size split (e.g.
  `demo.spec.ts` being much longer than the other 4 combined) would leave one shard dominating
  wall-clock while the other finishes early and idles — the matrix doesn't shrink total time in
  that case, it just wastes a runner. Playwright 1.49+ supports duration-aware sharding (feeding
  prior run durations back in) — worth checking whether the installed Playwright version
  (`tests/e2e/package.json`) supports it before assuming naive 2-way sharding is balanced;
  `fail-fast: false` is already set correctly so one shard's failure doesn't cancel the other.
- **Shared server + shared temp dir per shard, not shared across shards**: each `e2e-video.yml`
  shard runs its own `mktemp -d` (`DEMO_DIR`) and its own `./stapler-squad --test-mode --test-dir
  "$DEMO_DIR"` server instance on the same fixed `TEST_SERVER_URL: http://localhost:8543` port —
  this is safe *because* each shard is a separate runner/VM (no port collision), but it's a
  pattern that would break immediately if this plan ever tried to run multiple shards on one
  runner (e.g. to save runner-minutes via self-hosted-style co-location, which is out of scope
  per requirements.md's constraints, but worth flagging as a reason not to "optimize" toward
  it) — `tests/e2e/global-setup.ts`'s own dynamic `findFreePort()` pattern (per this repo's
  CLAUDE.md) is the one that's actually safe for co-located parallel servers; `e2e-video.yml` and
  `demo-publish.yml` both hardcode `:8543` instead, which only works because they're one-server-
  per-runner today.
- **`tests/e2e/playwright.config.ts` sets `workers: 1` globally** — this is the same config
  `demo-publish.yml` inherits (Section 0's root cause) and that `e2e-video.yml` overrides
  per-shard via the matrix, not via `workers`. Any future attempt to add real intra-runner
  parallelism (raising `workers` above 1) needs the shared-server-and-port pattern above fixed
  first — running `workers: 2+` against one hardcoded `:8543` server with one shared `DEMO_DIR`
  would hit exactly the "shared test state breaks under parallel sharding" failure mode the
  research question calls out, since tests would race on the same seeded session/backlog data.

Sources: [Playwright sharding guide — file-count vs duration-aware](https://www.mindbowser.com/playwright-sharding-guide/), [QASkills — Playwright test sharding and parallel CI guide 2026](https://qaskills.sh/blog/playwright-test-sharding-parallel-ci-guide), [Bug0 — Playwright sharding 60min→8min](https://bug0.com/blog/playwright-test-sharding-guide).

## 6. Metrics/observability pitfalls

- **The baseline table itself is the concrete instance of "false confidence from
  wall-clock-only, stale-trigger-blind metrics"** — see Section 0. Any duration-tracking
  automation this plan builds must record the triggering event (`push`/`pull_request`/
  `workflow_dispatch`) and the workflow file's *current* `on:` block alongside each run's
  duration, or it will keep quietly averaging-in runs from a since-changed trigger config the way
  the current baseline did for `demo-publish.yml`.
- **`gh api` secondary rate limits are real and already hit this session** (per the memory note
  referenced in this task's context) — GitHub's secondary limits cap REST calls at roughly 900
  points/minute and 100 concurrent requests, separate from the primary 5,000/hr (or 15,000/hr for
  GHEC-resource) per-token limit. A duration-tracking job that polls `gh api
  repos/.../actions/runs` plus per-run `jobs` endpoints for every run, on a schedule, across 13
  workflows, is exactly the kind of chatty polling pattern that trips secondary limits — batch
  requests (the `workflow_runs` list endpoint returns run-level `run_started_at`/`updated_at`
  timing without a per-job call; only reach for `/actions/runs/{id}/jobs` when per-job
  granularity is actually needed) and add backoff on `403`/`Retry-After` rather than retrying in
  a tight loop.
- **Actions "Usage" UI vs. API-derived numbers can disagree** because the UI aggregates billed
  minutes (rounded up per job, multiplied by the OS billing multiplier — 1x for Linux) while the
  API's `run_started_at`→`updated_at` delta is wall-clock, not billed-minutes; for a plan whose
  success metric is "wall-clock time-to-green," the API-derived number is the right one to track,
  but any cost-framed comparison (this repo is a **public** repo — confirmed via `gh api
  repos/tstapler/stapler-squad --jq .private` returning `false` — so GitHub Actions minutes on
  hosted runners are free/unbilled regardless of volume) should not lean on the billing-oriented
  Usage UI at all; there is no cost pressure here, only velocity/tail-latency pressure, which
  changes what "success" should be measured against (queue+run wall-clock, not minutes spent).
- **Queued/waiting time is invisible in a wall-clock-only view.** `run_started_at` (when the
  runner actually picked up the job) vs. `created_at` (when the run was queued) can diverge under
  runner contention (e.g. many workflows firing on the same push, each needing a fresh
  `ubuntu-latest` runner) — a plan built only on job *duration* averages would miss a real
  contributor to "time-to-green" that no amount of per-job caching fixes; the requirements doc's
  own "tail-latency" framing (18m avg vs 81.5m max for `build.yml`) is consistent with either
  cache misses *or* queue contention, and only the `created_at`→`run_started_at` delta
  distinguishes them — worth pulling before deciding which mechanism to optimize for.

Sources: [GitHub REST API rate limits docs](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api), [Actions limits docs](https://docs.github.com/en/actions/reference/limits), [cazzulino — avoiding GitHub API rate limits in CI workflows](https://www.cazzulino.com/github-actions-rate-limiting.html).
