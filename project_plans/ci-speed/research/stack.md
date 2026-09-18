# Research: Stack — CI Tooling for ci-speed

Grounded against `.github/workflows/*.yml`, `.github/actions/prepare/action.yml`, `Makefile`,
`go.mod`, `web-app/package.json`, and `tests/e2e/` as they exist on this branch
(`stapler-squad-ci-speed`) on 2026-08-27, plus 2026-era web research (search results below).

## 1. Go build/test caching: is `actions/setup-go`'s `cache: true` enough?

**Yes, it's still the right default here — but two repo-specific gaps make it underperform.**

- `setup-go`'s `cache: true` caches both `GOCACHE` (build cache) and `GOMODCACHE` (module
  cache), keyed on a hash of `go.sum` by default. This has been the built-in, GitHub-endorsed
  default since March 2023 and is confirmed still current in 2026 sources — no dedicated
  third-party Go cache action has displaced it. ([GitHub Changelog](https://github.blog/changelog/2023-03-24-github-actions-the-setup-go-action-now-enables-caching-by-default/), [actions/setup-go](https://github.com/actions/setup-go))
- **Gap 1 — go-version pin mismatch forces a toolchain download on every job.**
  `go.mod:3` pins `go 1.26.4`, but every `actions/setup-go` call in this repo pins
  `go-version: '1.25.0'` (`build.yml` ×7, `lint.yml`, `mcp-integration.yml`,
  `registry-validation.yml`, `release.yml`, `release-please.yml`), and two workflows
  (`demo-publish.yml`, `ux-analysis.yml`) pin `'1.23'` on `actions/setup-go@v4`. With
  `GOTOOLCHAIN=auto` (the Go default, unset anywhere in this repo — verified via
  `grep -rn GOTOOLCHAIN`), any `go build`/`go test`/`go vet` invocation on a runner whose
  installed toolchain is older than the `go` directive auto-downloads the 1.26.4 toolchain
  from `go.dev` before doing anything else. That download lands under `GOMODCACHE`
  (`$(go env GOPATH)/pkg/mod/golang.org/toolchain@go1.26.4...`), so `cache: true` *does*
  cache it after the first successful run per job — but every one of the ~13
  `actions/setup-go` call sites has its own independent cache scope (see Gap 2), so this
  network fetch pays out repeatedly rather than once. Fix is mechanical: bump every
  `go-version:` to `'1.26.4'` (or switch to `go-version-file: go.mod`, which
  `benchmark.yml`'s `go-tier1` job already does at line 68 — the only call site using it).
- **Gap 2 — 9+ independent cache scopes with no sharing.** `cache: true` computes cache key
  scope per-job (each `actions/setup-go` step gets its own `Linux-go-<hash>` style key), and
  there are 16 separate `actions/setup-go` invocations across the 13 workflow files (7 in
  `build.yml` alone: `web-build-smoke`, `test`, `pty-race-regression`, `integration-coverage`,
  `build` (×1 shared by the 5-way OS/arch matrix, cache restored 5x), `install-check`,
  `benchmark-gate`). Each restores the module+build cache independently; since `go.sum` is
  identical across all of them, they should all get cache **hits** in steady state (once the
  key exists), but GitHub Actions cache has a **200 uploads/minute, 1500 downloads/minute
  per-repo rate limit**, and "even a small Go program can require 3k+ cache entries" per
  restore according to 2026 measurements — with this many concurrent jobs restoring
  simultaneously on every PR push, rate-limit-induced cache misses (not stale keys) are a
  plausible, checkable cause of the tail latency the requirements doc flags (81.5m max vs
  ~18m avg on `build.yml`). ([Dan Peterson, "Better GitHub Actions caching for Go"](https://danp.net/posts/github-actions-go-cache/), [GitHub Community Discussion #198210](https://github.com/orgs/community/discussions/198210))
- **Cache hit-rate reality check**: 2026 measurements (RunsOn, Jan 2026) show 70-90% hit rates
  achievable with well-designed keys; `cache: true`'s default key (branch + `go.sum` hash) is
  already well-scoped for this repo (`go.sum` is 565 lines, one module, no workspace split),
  so the key *design* is not the bottleneck — restore volume/contention is. **Actionable
  check**: pull actual cache hit/miss timing from a sample of recent `build.yml` run logs
  (`Restore cache` step duration + hit/miss annotation) before assuming this is the tail-
  latency driver — this doc infers plausibility from documented GHA behavior, it does not
  confirm it from this repo's own run logs.
- **No GOCACHE-size/eviction problem specific to this repo is evident**: the 10GB
  per-repository cache cap now supports an opt-in size increase beyond 10GB (Nov 2025 GitHub
  changelog) and eviction is LRU by last-access, not a hard problem this repo's single-module
  Go cache is likely to hit — `go.sum` is small (565 lines) relative to typical GOCACHE
  bloat complaints, which come from many divergent branches/lockfile hashes coexisting, not
  from this scenario. ([GitHub Changelog: cache size can exceed 10GB](https://github.blog/changelog/2025-11-20-github-actions-cache-size-can-now-exceed-10-gb-per-repository/))

## 2. pnpm caching (web-app + tests/e2e) and Playwright browser caching

- **`web-app/` is already on best practice**: `actions/setup-node`'s `cache: 'pnpm'` +
  `cache-dependency-path: web-app/pnpm-lock.yaml` is used consistently everywhere `web-app`
  deps are installed (`.github/actions/prepare/action.yml`, `lint.yml`, `build.yml`'s
  `web-build-smoke`). This is still the documented, GitHub-endorsed pattern in 2026 — no
  reason to switch to a hand-rolled `actions/cache` on the pnpm store dir for a single-package
  (non-workspace: confirmed no `web-app/pnpm-workspace.yaml` exists) install.
- **Gap — `tests/e2e/` is a *second*, separately-versioned pnpm project with inconsistent
  handling.** `tests/e2e/package.json` + `tests/e2e/pnpm-lock.yaml` (and a stray
  `tests/e2e/package-lock.json`, presumably stale from before a pnpm migration — worth
  confirming and deleting if unused) exist independently of `web-app/`. Every workflow that
  installs it does so *without* wiring `cache-dependency-path` to
  `tests/e2e/pnpm-lock.yaml`, so `setup-node`'s pnpm cache (configured earlier in the same job
  for `web-app/pnpm-lock.yaml`) never covers it:
  - `demo-publish.yml:45` — `npm install` (not even pnpm — inconsistent with the rest of the
    repo, no cache at all, and downloads `node_modules` fresh every run).
  - `e2e-video.yml:102`, `ux-analysis.yml:73` — `pnpm install --frozen-lockfile` with no
    caching wired up (each of these jobs runs `pnpm install` cold on every single PR run).
  - `benchmark.yml:252,312,498,573` — same pattern, `cd web-app && pnpm install
    --frozen-lockfile` with no dedicated cache-dependency-path beyond whatever `setup-node`
    picked up earlier for the primary `web-app` lockfile.
  Fix: add a second `cache-dependency-path` entry (it accepts a list/glob) covering both
  lockfiles in one `setup-node` call, or a dedicated `actions/cache` step keyed on
  `tests/e2e/pnpm-lock.yaml`.
- **Playwright browser binaries are never cached anywhere in this repo.** All 4 call sites
  (`demo-publish.yml:46`, `e2e-video.yml:103`, `ux-analysis.yml:74`,
  `benchmark.yml:343,614`) run `npx playwright install chromium --with-deps` (or
  `--with-deps chromium`) with no preceding cache step. 2026 guidance: cache
  `~/.cache/ms-playwright` keyed on a hash of the lockfile (proxy for the pinned Playwright
  version) + the Playwright version itself in the key; **on a cache hit, still run
  `playwright install-deps` (the apt-level system libraries) but skip re-downloading the
  browser binary itself** — a documented gotcha, since the two are decoupled (browser binary
  vs. system shared libs). Measured savings in a comparable 2026 write-up: ~1m43s → ~45s
  compute + ~17s cache load/save, i.e. roughly 40s saved per job just on the browser download,
  before counting the apt-get dependency install this repo also pays separately in
  `e2e-video.yml`/`demo-publish.yml` for `ffmpeg` (uncached, `sudo apt-get install -y ffmpeg`
  every run — a much smaller cost than Chromium but stackable across 4+ jobs).
  ([Justin Poehnelt, "Caching Playwright Binaries in GitHub Actions"](https://justin.poehnelt.com/posts/caching-playwright-in-github-actions/), [QASkills.sh](https://qaskills.sh/blog/github-actions-cache-playwright-browsers), [playwrightsolutions.com](https://playwrightsolutions.com/playwright-github-action-to-cache-the-browser-binaries/))
- Since `demo-publish.yml`, `e2e-video.yml`, `ux-analysis.yml`, and `benchmark.yml`'s
  browser/e2e jobs all install the *same* pinned Playwright version independently, one
  well-scoped `actions/cache` step (or a small composite action mirroring
  `.github/actions/prepare`) would deduplicate this across all 4 workflows at once.

## 3. `concurrency:`/`cancel-in-progress`, path filters, and required-check gating

**Current state in this repo (grepped, not assumed):**

| Workflow | `concurrency:` block | `cancel-in-progress` |
|---|---|---|
| `benchmark.yml` | workflow-level | `${{ github.ref != 'refs/heads/main' }}` |
| `ux-analysis.yml` | workflow-level | `true` |
| `e2e-video.yml` | workflow-level | `true` |
| `build.yml` | **job-level only**, on `benchmark-gate` | `false` (deliberate, see below) |
| `lint.yml`, `mcp-integration.yml`, `registry-validation.yml`, `goreleaser-check.yml`, `backlog-scaffolding-guard.yml`, `generated-proto-guard.yml` | **none** | — |

- **Gap**: `lint.yml` and `mcp-integration.yml` — both PR-triggered, both pure
  compute-and-report with no persisted state — have no `concurrency:` group at all. Every
  push to a PR branch lets the *previous* run keep burning runner-minutes to completion
  instead of being cancelled, which directly wastes Actions minutes (the requirements doc's
  stated cost concern) on superseded commits — the highest-value, lowest-risk fix available
  in this whole survey, mirroring the pattern `ux-analysis.yml`/`e2e-video.yml` already use
  (`group: <workflow>-${{ github.event.pull_request.number }}`, `cancel-in-progress: true`).
- **`build.yml`'s existing job-level exception is correct and should not be copied blindly**:
  `benchmark-gate` (main-push only) deliberately sets `cancel-in-progress: false` because it
  persists a baseline to the Actions cache (`actions/cache/save@v4`) — cancelling mid-save
  would leave a corrupt or half-written baseline for the next PR to compare against. 2026
  guidance confirms this is the standard gotcha: any job doing `cache/save` or a git-push of
  derived state (this repo's `demo-publish.yml` git-commits GIFs back to `main`) must not sit
  inside a `cancel-in-progress: true` group scoped broadly enough to catch it mid-write.
  ([starsling.dev cancel-in-progress explainer](https://starsling.dev/best-practices/github-actions/cancel-superseded-runs))
- **The `detect-changes`-job-instead-of-`paths:`-filter pattern already used in `build.yml`
  and `mcp-integration.yml` is the documented correct workaround, not a stopgap to replace.**
  Both files' own comments (citing PR #618/#619) correctly identify that GitHub's native
  `paths:`/`paths-ignore:` filters on `pull_request` triggers never post *any* status for a
  required check when they don't match, which leaves branch protection blocked forever. 2026
  sources confirm this is still an open, unresolved GitHub limitation — `paths-ignore` "does
  not work for required checks" — with GitHub's own recommended mitigation being exactly this
  pattern (a job that always runs and reports skip-as-pass via `if:`).
  ([oneuptime.com concurrency guide](https://oneuptime.com/blog/post/2026-01-25-github-actions-concurrency-control/view))
  There is **no newer 2026 GitHub-native replacement** for this found in research — merge
  queue's `merge_group` trigger addresses a different problem (cancelling stale queue entries
  via the `destroyed` event), and is not adopted anywhere in this repo's workflows
  currently. Merge queue is a real candidate worth a design decision in the plan phase (it
  would let multiple PRs' required-check runs be batched/deduplicated at merge time), but it's
  a branch-protection/repo-settings change, not a workflow-file change — flagging for the
  plan phase rather than resolving here.
- **`lint.yml` and every other workflow *do* still carry `paths:`/`paths-ignore:` filters on
  `push`** (correctly — the required-check trap above is specific to `pull_request`, since a
  push to `main` isn't gating a merge). These push-side filters look reasonably scoped already
  (e.g. `lint.yml`'s `web-app/**.css`, `**.ts`, `**.tsx`, `tests/e2e/**`, `**.sh`).

## 4. Bazel + bzlmod for this Go+TS stack: 2026 survey

- **bzlmod is the stable, default dependency system in 2026** — WORKSPACE is deprecated
  (consistent with the requirements doc's note that the prior narrow tmux/`rules_foreign_cc`
  integration broke specifically because it was WORKSPACE-mode-only under Bazel 9).
- **Go support (`rules_go` + Gazelle)**: mature and bzlmod-native; multiple 2026 write-ups
  (including a Bazelcon talk on "How Uber Manages Go Dependencies with Bzlmod") describe
  production Go+bzlmod monorepos. This is the lower-risk half of a hypothetical migration.
  ([dev.to Go/Bazel/Gazelle/bzlmod walkthrough](https://dev.to/nikhildev/building-with-go-in-a-monorepo-using-bazel-gazelle-and-bzlmod-4on0))
- **TypeScript/pnpm support (`aspect_rules_js`)**: also bzlmod-native via
  `npm_translate_lock`/`npm_import` extensions in `MODULE.bazel`, which directly consume a
  pnpm lockfile. **`rules_js` 3.0 shipped 2026-02-09** and explicitly **dropped support for
  older Bazel/pnpm/Node versions** to simplify internals — current stable is `aspect_rules_js`
  3.2.3. The stated prerequisite for a low-friction adopt is "Bazel 7+, pnpm 9+" — this repo
  is on `pnpm@10.27.0` (`web-app/package.json`'s `packageManager` field) and would need to
  confirm current Bazel-version compatibility at plan time, not assume it from this survey.
  ([Aspect blog: rules_js 3.0](https://blog.aspect.build/rules-js-3), [rules_js releases](https://github.com/aspect-build/rules_js/releases))
  Framework-specific note not covered by the searches above: this repo's web-app is a
  **Next.js** app (`web-app/next.config.ts`), and `aspect_rules_js`'s Next.js integration
  story is generally reported as less turnkey than a plain Vite/webpack SPA — worth a spike,
  not an assumption, if Bazel is shortlisted in the plan phase.
- **Remote caching options**:
  - **BuildBuddy free tier**: "100 GB of cache transfer" on the Personal/free tier per its
    pricing page (exact monthly-vs-total framing not confirmed in this pass — verify directly
    against `buildbuddy.io/pricing` at plan time before committing to it as the remote-cache
    backend). Team/enterprise tiers add unlimited transfer. ([BuildBuddy pricing](https://www.buildbuddy.io/pricing/))
  - **GitHub Actions cache as a Bazel remote-cache backend**: technically possible via a
    `bazel-remote`-compatible HTTP shim or a community action, but this is a noticeably less
    trodden path than BuildBuddy/EngFlow's native gRPC remote-cache protocol — most 2026
    write-ups configure `--remote_cache` against a purpose-built cache server (BuildBuddy,
    EngFlow), not GitHub's cache API. This repo's own free-runner constraint (no self-hosted)
    doesn't block using an external SaaS remote cache like BuildBuddy's free tier — it's
    orthogonal to runner hosting.
- **Rough edges / migration cost for *this* repo specifically** (synthesizing the survey
  against this repo's actual shape, not generic Bazel complaints):
  - The repo already has one documented, recent, negative data point:
    `rules_foreign_cc`'s WORKSPACE-only tmux integration had to be ripped out
    (`b51b60eb1`, 2026-07-15) purely because of the WORKSPACE→bzlmod transition — a full
    bzlmod adoption avoids that specific trap, but demonstrates the team has already spent
    non-trivial effort on a narrow Bazel integration that didn't pay off.
  - `ent` (entgo.io) code generation, `buf` protobuf generation, and Next.js's own build
    pipeline (`next build` with `output: export`-style static output, per
    `.github/actions/prepare/action.yml`'s `cp -r web-app/out server/web/dist`) would all need
    Bazel-rule wrapping (`rules_go`'s `go_generate`-style targets for `ent generate`, a custom
    `buf` genrule, and `aspect_rules_js`'s Next.js support) before Bazel could replace even the
    `prepare` composite action, let alone the full `make ci` graph — this is a multi-week
    surface, consistent with the requirements doc's "Rabbit Hole" flag on full Bazel adoption.
  - **Net read for this survey**: Bazel/bzlmod is technically viable for this stack in 2026
    (unlike the "permanently broken under Bazel 9" WORKSPACE-mode failure that killed the
    prior integration), but the migration surface (ent + buf + Next.js + existing
    Make-based dev workflow that `CLAUDE.md` documents extensively) is large enough that the
    plan phase should treat it as a large, separately-sequenced bet — not something to fold
    into the same effort as the caching/concurrency/path-filter fixes above, which are
    low-risk and independently shippable this week.

## 5. Lighter alternatives: Turborepo/Nx, and Go-side cross-job cache sharing

- **Turborepo vs Nx for `web-app/`**: both are 2026-current, both support GitHub Actions
  remote caching (Turborepo via Vercel's remote cache, free for hobby/personal use; Nx via Nx
  Cloud). Community framing in 2026: Turborepo suits a single JS/TS package wanting fast task
  orchestration without adopting a new workspace model; Nx adds affected-graph analysis,
  generators, and better polyglot (Java/.NET) support that's irrelevant here. **Caveat specific
  to this repo**: `web-app/` is confirmed **not** a pnpm workspace (`web-app/pnpm-workspace.yaml`
  does not exist) — it's a single package with one `package.json`. Turborepo/Nx's core value
  (task-graph caching *across multiple packages*) doesn't apply until/unless `web-app/` and
  `tests/e2e/` are unified into one pnpm workspace with `turbo run` orchestrating both. Today,
  the only caching lever available on the JS side is plain pnpm-store/lockfile caching (§2) —
  Turborepo wouldn't add value without that workspace restructuring first, which is itself a
  non-trivial, separately-scoped change.
  ([nx.dev "Nx vs Turborepo"](https://nx.dev/docs/kb/nx-vs-turborepo), [WarpBuild monorepo GH Actions guide](https://www.warpbuild.com/blog/github-actions-monorepo-guide))
- **Go-side `sccache`/`ccache` equivalents**: none needed or found — Go's own `GOCACHE` (build
  cache) already does content-addressed, per-package incremental caching, which is what
  `sccache`/`ccache` bolt onto C/C++/Rust toolchains that lack it natively. The actionable gap
  here isn't a smarter cache mechanism, it's the *scope/pinning* issues in §1 (version-pin
  mismatch, redundant per-job restores under rate limits).
- **Cross-job Go build-artifact sharing within one workflow run** (as distinct from
  cross-*run* caching, which `cache: true` already covers): no dedicated marketplace action
  found for this — the generic mechanism is `actions/upload-artifact`/`download-artifact`,
  which this repo already uses for exactly this purpose (`build.yml`'s `prepare` job uploads
  `generated-files` — `gen/`, `session/ent/`, `server/web/dist/` — and `test`,
  `pty-race-regression`, `integration-coverage`, `build`, `install-check`, `benchmark-gate` all
  download it rather than regenerating). **Caveat surfaced by 2026 sources**: artifact
  upload/download has meaningfully higher fixed latency than cache restore ("upload ~5 min /
  download ~2 min" vs. cache "a few seconds" in one comparable write-up) — for `build.yml`'s
  `generated-files` artifact specifically, this is likely still the right tool (contents are
  build outputs generated fresh each run, not a stable dependency cache), but it's worth
  timing the actual upload/download step durations in this repo's own run logs before
  assuming the artifact hop is free.
  ([echobind.com artifacts-vs-cache](https://echobind.com/post/difference-between-artifacts-and-cache-in-GitHub-Actions), [Thinkmill: sharing build artifacts across jobs](https://www.thinkmill.com.au/blog/faster-ci-pipelines-share-build-artifacts-across-independent-jobs))
- **Bigger structural gap this section surfaces (flagging for the workflow-structure research
  agent, not resolving here)**: unlike `build.yml`'s jobs, **`mcp-integration.yml`,
  `e2e-video.yml`, and `ux-analysis.yml` each call `./.github/actions/prepare` independently**
  rather than consuming `build.yml`'s `generated-files` artifact — because they're separate
  *workflows* (GitHub Actions artifacts don't cross workflow-run boundaries without an
  explicit `workflow_run`/`workflow_call` wiring). That means the full
  proto-gen + ent-gen + Next.js build sequence runs **up to 4 independent times per PR push**
  (once each in `build.yml`, `mcp-integration.yml`, `e2e-video.yml`, `ux-analysis.yml`, when
  all are triggered). A GitHub-native fix exists for this class of problem — **reusable
  workflows (`workflow_call`)** — which would let one workflow run `prepare` once and have
  the others consume it via `needs:`/artifact-passing within a single workflow run instead of
  four independent ones. This is a workflow-graph restructuring decision (which jobs get
  merged into one workflow) rather than a tooling swap, so it belongs in the architecture/plan
  phase — recorded here because it surfaced directly from this stack survey's own artifact
  vs. cache comparison.

## 6. Additional direct-inspection findings (not from web research)

- **Pinned tmux 3.4 binary is built/cached 4 separate times per `build.yml` run.** The exact
  same cache key (`tmux-3.4-${{ runner.os }}-v1`) and `apt-get install automake libevent-dev
  libncurses-dev pkg-config [socat]` + conditional `./scripts/build-tmux.sh` sequence appears
  independently in 4 jobs: `test` ([build.yml:238-247](../../../.github/workflows/build.yml#L238-L247)),
  `pty-race-regression` (L461-470), `integration-coverage` (L552-561), and `benchmark-gate`
  (L714-723). On a cache hit this is just 4 redundant `actions/cache` restores + 4 redundant
  `apt-get install` calls (small but stacked); on a cache **miss** it's 4 full serial tmux
  source builds instead of one. Since all 4 jobs already download the same `generated-files`
  artifact from `prepare`, building tmux once in `prepare` and folding the binary into that
  same artifact would collapse this to a single build + 4 cheap artifact-download reuses —
  same consolidation logic as the `workflow_call` idea in §5, but resolvable entirely within
  `build.yml` without a cross-workflow restructure.
- **`go.mod` vendors the entire `buf` CLI as a Go library dependency**
  (`github.com/bufbuild/buf v1.57.2`, [go.mod:16](../../../go.mod#L16)) in addition to every
  workflow separately downloading/caching the standalone `buf` *binary* via
  `bufbuild/buf-action` or a raw `curl` (`lint.yml:84-86`). This roughly doubles buf-related
  weight in the dependency graph (module cache entry + separate tool-cache binary) for a tool
  only invoked via CLI (`buf generate`) — worth confirming at plan time whether the Go-module
  dependency on `buf` is load-bearing (e.g. a library import somewhere) or leftover, since
  removing an unused direct dependency shrinks `go.sum` and the module-cache restore payload
  for all 16 `setup-go` call sites.
- **`deploy-pages.yml` targets a `web/` directory that does not exist in this repo** (only
  `web-app/` does — confirmed via `ls -d web` returning nothing). Its `paths: ['web/**', ...]`
  filter means it structurally can never trigger on `push`/`pull_request` today, so it isn't
  costing any CI minutes — but it's dead workflow config that should be deleted or repointed
  at `web-app/` as part of any cleanup pass, since it's easy to mistake for a live check when
  auditing `.github/workflows/`.
- **`tests/e2e/` carries a stray `package-lock.json` alongside its authoritative
  `pnpm-lock.yaml`** (no `packageManager` field in `tests/e2e/package.json` to confirm intent,
  unlike `web-app/package.json`'s explicit `pnpm@10.27.0` pin) — likely leftover from a pre-pnpm
  migration. Harmless to CI time directly, but worth deleting so a future `npm install` in that
  directory (as `demo-publish.yml` already does today) doesn't silently diverge from the pnpm
  lockfile everyone else uses.

## Summary of concretely actionable, low-risk findings (for the plan phase)

1. Fix the `go.mod` (`1.26.4`) vs. every `actions/setup-go` `go-version:` pin (`1.25.0`/`1.23`)
   mismatch — switch to `go-version-file: go.mod` everywhere (already the pattern in
   `benchmark.yml`'s `go-tier1` job) to stop paying repeated toolchain-download overhead.
2. Add `concurrency:`/`cancel-in-progress: true` groups to `lint.yml` and
   `mcp-integration.yml` (both currently have none) — mirrors the pattern already proven in
   `ux-analysis.yml`/`e2e-video.yml`.
3. Cache `~/.cache/ms-playwright` (with the install-deps-on-hit caveat) across
   `demo-publish.yml`, `e2e-video.yml`, `ux-analysis.yml`, `benchmark.yml` — currently
   uncached in all four.
4. Wire `cache-dependency-path` to cover `tests/e2e/pnpm-lock.yaml` (in addition to
   `web-app/pnpm-lock.yaml`) wherever both are installed in the same job; switch
   `demo-publish.yml`'s stray `npm install` to `pnpm install --frozen-lockfile` for
   consistency and to get caching at all.
5. Treat Bazel/bzlmod adoption as a separately-sequenced, large bet (per the requirements
   doc's own Rabbit Hole flag) — not a near-term fix; the caching/concurrency/pin fixes above
   are shippable independently and immediately.
6. Flag the 4x independent `prepare` (proto-gen/ent-gen/web-build) duplication across
   `build.yml`/`mcp-integration.yml`/`e2e-video.yml`/`ux-analysis.yml` for the
   workflow-structure research agent — a `workflow_call` consolidation is the GitHub-native
   fix, but it's a job-graph decision, not a caching-tool decision.
7. Build pinned tmux 3.4 once in `build.yml`'s `prepare` job and fold it into the
   `generated-files` artifact instead of rebuilding/re-caching it independently in `test`,
   `pty-race-regression`, `integration-coverage`, and `benchmark-gate` (4x today).
8. Delete or repoint the dead `deploy-pages.yml` (targets a nonexistent `web/` directory) and
   the stray `tests/e2e/package-lock.json` as low-effort cleanup alongside the above.
