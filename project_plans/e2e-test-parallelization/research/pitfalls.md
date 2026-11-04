# Research: Known Pitfalls and Risks

Agent 4 (Pitfalls), Phase 2 — e2e-test-parallelization

## 1. This session's own pitfalls (verified against this repo's git/PR history)

These four were hit directly in this session's #1159 → #1165 → #1166 saga and are the
highest-confidence evidence available, since they're reproduced against a backend server
exactly like the one this project is about to shard further.

### 1a. Missing runtime dependency only discovered in CI (`ts-node`, exit 127)

Commit [`fb54c63cd`](https://github.com/tstapler/stapler-squad/commit/fb54c63cd17b9b94a046e66b2300c64601a0222e) —
"fix(ci): install web-app deps in e2e-bun-report before running the comparator":

> The comparator runs via `web-app/node_modules/.bin/ts-node`. The pre-sharding `e2e-bun`
> job got this installed as a side effect of its full `make stapler-squad` build; the new
> `e2e-bun-report` job never ran that, so ts-node was missing (exit 127, confirmed on this
> PR's own first run).

**Pitfall:** splitting one monolithic job (which happened to install a dependency as a side
effect of an unrelated build step) into N smaller jobs/shards silently drops that side-effect
install for any job that doesn't do the full build — e.g. a lightweight merge/report job. Local
verification can't catch this because the full local flow still does the complete build. This is
exactly the shape of risk a future "share build artifacts across shards" change would reintroduce
if the report/merge job is assumed to inherit the same toolchain as the shard jobs that did a full
`make stapler-squad`.

### 1b. Hardcoded gate-membership default broke main's CI immediately after merge

PR #1165 body and its close comment — [issue comment](https://github.com/tstapler/stapler-squad/issues/1165#issuecomment)
— plus commit history around `DEFAULT_MEMBERS` in `web-app/scripts/__tests__/web-build-gate.test.mjs`:

> `web-build-gate`'s `DEFAULT_MEMBERS` hardcoded `e2e-next`, whose job result is raw Playwright
> exit code, not a baseline-aware comparator verdict. The real baseline... shows ~529 of ~2400
> specs genuinely failing today — pre-existing, not a regression — so `e2e-next` can never exit 0
> as a default member. Confirmed live: main's very first post-merge `build.yml` run (`e2e-next`,
> `e2e-bun`, `web-build-gate` all `failure`) broke immediately.

**Pitfall:** a gate/membership list that encodes "which jobs must pass" as a hardcoded default
is untested until it actually runs on `main` post-merge — PR CI for the gate-definition change
itself doesn't exercise the failure mode, because the new job hasn't run with real (known-failing)
data yet. The fix pattern used here — give new report-only jobs an explicit opt-in gate variable
(`WEB_BUILD_GATE_MEMBERS`) rather than a hardcoded default — generalizes to any future change that
adds a new CI job whose pass/fail isn't yet proven to correlate with real regressions.

### 1c. Concurrency-group cancellation didn't take effect for ~7 hours on a long-running main push

`build.yml`'s workflow-level concurrency block (lines 55-57):
```yaml
concurrency:
  group: build-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}
```
This is deliberate — the preceding comment (lines 49-51) explains `cancel-in-progress` is `false`
for `main` so `benchmark-gate`'s cache-save step is never killed mid-write. But it has a real cost,
confirmed directly: push `0b0ae53ea` (PR #1159's merge, the pre-sharding unsharded e2e jobs) kicked
off [run 40126199824](https://github.com/tstapler/stapler-squad/actions/runs/40126199824) at
19:38Z. `e2e (Next build)` ran 19:40Z→23:03Z (hit its 200m soft timeout), then `e2e (Bun build)`
ran 23:03Z→02:28Z the *next calendar day* — the whole run took ~6h50m, even though the sharding
fix commits (`b954d9e70`, `1f60795c5`, `d6f4625b2`) that superseded this exact problem were
authored and pushed to the branch within that same window. Because this was a `push`-to-`main`
event, no later commit's workflow run was able to cancel it — `cancel-in-progress` is scoped off
for `main` by design, so the obsolete run kept consuming a runner and reporting a stale/misleading
`main` red status for hours after the actual fix had already landed on a branch.

**Pitfall:** any CI-timing change (new shard count, new per-job timeout, a job that might run very
long before failing) needs to be considered against workflows that disable `cancel-in-progress`
for `main`. A long-running job that goes wrong on `main` can't be cancelled by a subsequent push —
only by someone manually cancelling it via `gh run cancel` or the UI. This matters directly for the
"what's the right shard count/timeout" research question: a too-large per-shard soft timeout on
`main` is not just "slow," it's uncancellable for its full duration.

### 1d. Stale merge-base / stale `refs/pull/N/merge` caused unrelated-looking test failures

Two independent instances of the same bug shape:

- Commit [`352174457`](https://github.com/tstapler/stapler-squad/commit/352174457) (PR #1146) —
  "fix(ci): pin web-lint/go-lint checkout to the PR head, not the stale merge ref":
  > `actions/checkout` defaults to GitHub's synthetic `refs/pull/N/merge`... which tracks main's
  > current tip independently of `base.sha`... Once main advances past `base.sha`, each job's
  > `git diff base.sha...HEAD` silently widens to include every file from every PR merged to main
  > in between. Surfaced 2026-10-22: PR #1144's web-lint failed on unrelated pre-existing warnings
  > in files the PR never touched (from PR #1143, merged to main after #1144's last sync).
- The repo's own memory note on PR #905
  (`instinct_ci_tests_against_live_base_not_stale_merge.md`): a `make registry-generate`-based
  check failed in CI with "Registry out of date" even though local verification showed no diff,
  because `main` had gained 4 more commits (including a file-splitting refactor) between the
  local merge and the CI run.

**Pitfall:** any new CI logic in this project that does a diff/comparison against a base
ref/commit (e.g., a future "only run affected shards" optimization, or a baseline-diff check for
the comparator) must pin to the PR's actual head SHA and a *fixed* base SHA captured at the right
time — not trust that GitHub's default checkout ref or a stale local merge represents what CI will
actually see. This is a recurring bug class in this exact repo (hit at least 3 times:
#905, #1146, and implicitly relevant to any future shard-affected-tests logic), so a
sharding/parallelization redesign that adds new base-ref comparisons should budget for this
specific review step up front rather than discover it the hard way again.

## 2. General Playwright/CI sharding gotchas (external research)

### Blob report merging
- Blob report filenames already embed the shard index, so copying all shards' `*.zip` files into
  one directory for `playwright merge-reports` is safe — they don't clash
  ([Playwright blob reporter guide](https://qaskills.sh/blog/playwright-blob-reporter-guide);
  [Merging sharded reports into one HTML report](https://timdeschryver.dev/blog/merging-your-playwright-sharded-reports-into-a-single-html-report)).
  This repo's own `e2e-next-report`/`e2e-bun-report` merge jobs already rely on exactly this
  property — confirmed independently by PR #1165's test plan ("Verified Playwright's blob
  reporter auto-suffixes each shard's output filename by shard index").
- The merge step can use a *different* reporter config than the shard-generation step (e.g.
  generate blob during shards, merge to JSON for the comparator) — intentional, not a bug, per
  the [official docs via the sharding guide](https://www.mindbowser.com/playwright-sharding-guide/).
- Artifact upload paths must be updated in lockstep with any reporter-output-directory rename
  (`playwright-report` → `blob-report`), or the upload step silently stops picking up the right
  files.

### Shard balancing by count vs. duration
- Playwright's native `--shard` splits by file/test *count*, not measured duration — a shard that
  happens to collect several slow spec files becomes the long pole for the whole run regardless of
  shard count chosen
  ([webcrawlerapi.com glossary on shard imbalance](https://webcrawlerapi.com/glossary/playwright/how-to-fix-playwright-shard-imbalance);
  [Currents parallelization guide](https://docs.currents.dev/guides/parallelization-guide/pw-parallelization/playwright-orchestration)).
  This directly matches this repo's own baseline note that shard timing is "~25-40 min/shard" —
  i.e., already observed to vary shard-to-shard, which is this exact phenomenon, not noise.
- Duration-aware balancing exists only via third-party reporters (e.g. the `@flakiness/playwright`
  reporter's `shardBalancing`, which packs shards by expected runtime using a timings file from a
  prior run) — not built into Playwright core as of this research
  ([flakiness README](https://cdn.jsdelivr.net/npm/@flakiness/playwright@2.0.0/README.md)). A
  timings file that goes stale (tests added/removed/renamed) falls back to a default weight per
  missing test, so a stale hint file degrades gracefully rather than breaking the split — worth
  knowing if the plan phase considers this route for shard-count tuning.
- CI-index-to-shard-index base mismatches are a recurring gotcha on other CI providers (CircleCI's
  `CIRCLE_NODE_INDEX` is 0-based vs. Playwright's `--shard` being 1-based)
  ([mindbowser sharding guide](https://www.mindbowser.com/playwright-sharding-guide/)). GitHub
  Actions' own `strategy.matrix` index is also 0-based in the same way this repo's `build.yml`
  shard jobs must already account for — worth re-verifying explicitly if shard count changes,
  since an off-by-one here silently drops or duplicates a shard's worth of tests rather than
  erroring loudly.

### Artifact-upload flakiness scaling with job count
- `actions/upload-artifact@v4` no longer merges same-named uploads across matrix/shard jobs the
  way v3 did — every matrix job uploading under one static artifact name causes failures for all
  but the first job to finish, confirmed by a reported 12-job matrix incident
  ([Joomla CMS issue tracker](https://issues.joomla.org/tracker/joomla-cms/46925); GitHub's own
  "multiple uploads to the same named artifact" v4 migration-guide section).
- `overwrite: true` is not a reliable workaround — it's been reported as racy/intermittent under
  genuinely parallel jobs writing to the same artifact name
  ([upload-artifact issue #506](https://redirect.github.com/actions/upload-artifact/issues/506)).
- Practical implication for this project: this repo's shard jobs almost certainly already
  shard-qualify their blob-report artifact names (worth confirming in the plan/implementation
  phase against the live `build.yml`), but **any increase in shard count from 10 upward should
  re-verify the artifact-name uniqueness pattern still holds**, since this is exactly the class of
  bug that gets more likely, not less, as job count rises — and it manifests as a report-merge job
  silently working with fewer shards' data than expected (a quiet undercount), not a loud failure.

### General stateful-backend parallelization pitfalls
- The dominant failure mode in e2e suites run against a real (non-static) backend is shared
  mutable state across "parallel" tests — shared test records, shared accounts, leaked
  cookies/localStorage — not infrastructure flakiness per se
  ([QA Wolf: speed up tests with parallelization](https://qawolf.com/blog/speed-up-tests-with-parallelization);
  [QA Wolf: writing tests for full parallelization](https://www.qawolf.com/blog/how-to-write-tests-for-full-parallelization)).
- Hidden order dependencies between tests are a well-documented academic finding as a primary
  blocker to safe parallelization of e2e suites
  ([Olianas thesis via iris.unige.it](https://iris.unige.it/handle/11567/1118975)).
- The standard mitigation for a stateful backend is either per-test/per-worker data isolation
  (fresh account/UUID per test) or a dedicated backend instance per parallel unit rather than one
  shared non-production server
  ([parallel-ui-testing-with-docker example](https://github.com/cdaringe/parallel-ui-testing-with-docker);
  [dev.to: optimizing e2e strategies](https://dev.to/quave/optimizing-end-to-end-testing-strategies-for-speed-reliability-and-efficiency-1idh)).
  This is the exact tradeoff the requirements doc's "shared-server rearchitecture" rabbit hole is
  about — external sources corroborate that "N servers, one per worker" is the standard fix, but
  only once per-test data isolation within each server is also handled; adding N servers without
  fixing cross-test state coupling *within* each server's test sequence doesn't remove the
  underlying risk, it just changes its blast radius from "whole suite" to "whole shard."
- Retries are explicitly called out as a mask, not a fix, for state-coupling flakiness — worth
  noting since this repo's `playwright.config.ts` already uses `retries: 1`
  ([dev.to: flaky test root causes](https://dev.to/manavshiplightai/how-to-fix-flaky-tests-root-causes-and-permanent-fixes-41f4)).

## 3. Bun-specific stability risks for CI tooling use

Per `docs/explanation/bun-migration-feasibility.md` (already in this repo) and this session's own
requirements doc, the closed decision is: **do not re-propose `bun test` as the test runner** —
that's settled and out of scope. The question here is narrower: are there known Bun stability
issues relevant to using Bun more aggressively for *install/build/tooling* steps in the e2e CI path
(which `build.yml` already does today via `bun install` and, for `e2e-bun`, `make stapler-squad`
with `WEB_BUILD=bun`)?

- **Already documented in this repo:** `bun test` segfaulted after ~90s with 125 failures on the
  currently-installed Bun version (1.4.3) — `docs/explanation/bun-migration-feasibility.md` §2,
  §9's "Risks added by this choice." This is scoped to the *test runner*, not install/build, and
  is explicitly out of scope per the requirements doc — included here only for completeness, not
  as new evidence.
- **`Bun.build` itself has not crashed** in the feasibility study's spikes (§9: "Bun-specific bugs
  in a young bundler... `Bun.build` itself has not crashed in these spikes") — this is the
  relevant data point for the *tooling* angle this project cares about, and it's reassuring but
  explicitly scoped to spikes, not sustained CI load.
- **`vanilla-extract` incompatibility under Bun** (§3.1 of the feasibility doc): Bun's `require`
  shim returns `undefined` for a Node-internal (`Module._load`) that the `eval` npm package
  depends on, breaking `vanilla-extract`'s CSS compile step unless patched with a `vm` +
  `createRequire` shim. This matters if any future e2e-CI speedup idea involves running
  `make stapler-squad`'s Bun build path more aggressively (e.g., building once and sharing across
  shards) — the existing Bun build path already carries this patched-dependency risk, which is a
  real (if currently working) fragility point to be aware of before leaning on it harder.
- **Bun's CSS parser is stricter than webpack's**, rejecting malformed nested `@keyframes` blocks
  that webpack silently passed through (§3.2) — already fixed upstream (PR #1097) but illustrates
  that Bun's tooling surfaces latent bugs other tools didn't; a heavier reliance on Bun in CI
  increases the chance of hitting another such latent-bug discovery mid-pipeline.
- **No official Bun GitHub issues found** on install/lockfile/cache instability specifically from
  2026 web search — the one concrete report found (an archgate CLI PR fixing a
  `--frozen-lockfile` failure after a release script updated `package.json` without regenerating
  the lockfile) was a project-process gap, not a Bun engine bug
  ([agentskills.so bun-lockfile-update skill description](https://agentskills.so/skills/laurigates-claude-plugins-bun-lockfile-update)).
  Bun's own docs recommend `bun ci` over `bun install` in CI specifically for frozen-lockfile
  enforcement ([bun.com/docs/pm](https://bun.com/docs/pm)) — worth checking whether `build.yml`'s
  existing `bun install --frozen-lockfile` steps should switch to `bun ci` if that command exists
  in the pinned Bun version, as a low-risk hardening item adjacent to this project (not required by
  it).
- **Net assessment:** no new Bun-stability landmine was found beyond what
  `bun-migration-feasibility.md` already documents (the test-runner segfault, out of scope here).
  For the in-scope angle — install/build/tooling steps — the evidence is "works, but on a patched
  dependency and a stricter-than-webpack CSS parser," i.e., every additional Bun-build invocation
  in the e2e CI path inherits that same, currently-contained fragility. Any plan to build once and
  share across shards (the "fixed per-shard overhead" rabbit hole) should treat a `make
  stapler-squad WEB_BUILD=bun` artifact as coming from a path with a known patched dependency, not
  a fully stock toolchain — relevant to how confidently that artifact can be cached/reused.

## 4. Design-against list if the shared-server constraint is lifted

If the plan recommends N servers per job (one per Playwright worker) instead of one shared server,
these failure modes should be explicitly designed against — each framed by how it would actually
manifest if missed, since none of them crash loudly:

| Failure mode | Mechanism | How it manifests if missed |
|---|---|---|
| **Port exhaustion / collision** | N servers each need a free dynamic port; `tests/e2e/helpers/test-server.ts`'s `findFreePort()` already does this for the single-server case, but a race between N near-simultaneous `findFreePort()` calls at worker-startup time (TOCTOU: port is free when checked, taken by the time `bind()` happens) is more likely at N>1. GitHub Actions runners also have the full ephemeral port range available, so outright exhaustion is unlikely, but transient same-port collision during the narrow check-then-bind window is not. | **Not a crash** — a worker's server fails to start, Playwright's fixture setup for that worker times out, and the resulting test failures look like ordinary app-level flakiness (timeouts waiting for the app to respond) scattered across whatever specs happened to run on that worker, with no obvious common cause pointing back to port allocation. Hard to attribute without specifically logging the bound port per worker and cross-referencing failure clustering by worker index. |
| **Resource contention on one runner's fixed CPU/memory** | GitHub-hosted runners have a fixed core/memory budget (this repo's current jobs already note CPU contention concerns elsewhere, e.g. the `build.yml` comment about splitting `TMUX_TEST_PKGS`/`REST_TEST_PKGS` to avoid "sharing a runner's CPUs with... real tmux subprocesses"). N full `stapler-squad` server processes plus N browser instances plus N tmux subprocesses all competing for the same fixed vCPU count will slow every server down non-uniformly depending on scheduling luck. | **Not a crash** — tests get slower and more timing-sensitive assertions (anything with a fixed-ish wait, or any spec relying on the TOFU-dialog-skip workaround the requirements doc already flags as fragile) start failing intermittently under load, in a pattern that looks exactly like ordinary test flakiness and gets misdiagnosed as "this spec is flaky" rather than "this runner is oversubscribed," especially since the failure rate would vary by which other specs happen to land on the same worker that run. |
| **Incomplete test isolation / cross-contamination between per-worker servers** | The repo's own state-isolation design (`docs/reference/state-isolation.md`, `STAPLER_SQUAD_INSTANCE`, `STAPLER_SQUAD_TMUX_SOCKET`) already documents multiple axes that must each be separately isolated — config dir, tmux socket, workspace/session state — and the project's own `CLAUDE.md` explicitly warns that `STAPLER_SQUAD_INSTANCE` and `--test-mode`/`--test-dir` do **not** isolate tmux; only `STAPLER_SQUAD_TMUX_SOCKET` does, and an instance started without it "shares the live service's default tmux server, sees its sessions, and its leaked-control-mode sweeper... can kill the live service's control clients." A from-scratch per-worker server design that misses even one of these axes (e.g. isolates the port and test-dir but reuses a derived-but-not-unique tmux socket name across workers) reproduces this exact class of bug between *worker* N and worker M instead of between a test instance and the live service. | **Not a crash** — two workers' sessions interfere (one worker's leaked-session sweeper kills another worker's active tmux pane, or two workers' `global-setup.ts` both resolve to the same fallback testdir under some edge-case env-var precedence), producing test failures that look like each spec is independently flaky, are not reproducible when that spec is run alone, and can only be attributed by correlating failure timing across workers — exactly the kind of bug the existing single-TOFU-dialog-skip workaround already hints the current architecture is fragile around. |

**Common thread:** none of these three failure modes crash the test run outright — they all
degrade into "flaky-looking" failures that are individually hard to reproduce and get attributed to
"that spec is just flaky" rather than the shared root cause (contention or incomplete isolation).
Any shared-server-lifting design should therefore budget for an explicit verification step beyond
"the suite passed once" — e.g., deliberately running the full per-worker-server suite multiple
times and checking whether failures cluster by worker index or by co-scheduled spec, which is the
kind of multi-cycle validation the requirements doc's own "Rabbit Holes" section already flags as
expected for this specific sub-effort.
