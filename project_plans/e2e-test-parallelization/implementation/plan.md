# Implementation Plan: e2e-test-parallelization

**Feature**: Reduce `e2e-next`/`e2e-bun` wall-clock beyond the #1166 10-way-shard baseline via duration-neutral native rebalancing, browser-install isolation, and a bounded test-content triage, while deferring the higher-risk per-worker-server rearchitecture as a documented seam.
**Date**: 2026-10-25
**Status**: Ready for implementation
**ADRs**: [ADR-001: Isolate via seam — defer N-server rearchitecture](../decisions/ADR-001-isolate-via-seam-defer-n-server-rearchitecture.md), [ADR-002: fullyParallel:true before any third-party shard balancer](../decisions/ADR-002-fullyparallel-before-third-party-balancer.md)

---

## Step 0.5 — Creative pass: overall project shape

Three candidate shapes for how this round's work lands, evaluated before choosing:

1. **All wins in one PR** (fullyParallel flip + browser-install isolation + Bun experiment + sample-triage, all in one `build.yml`/`playwright.config.ts` commit).
   - *Strength*: one CI cycle to prove everything out; minimal coordination overhead.
   - *Weakness*: if CI regresses, the specific cause is unclear across four simultaneous changes, and `build.yml`'s own `concurrency.cancel-in-progress: false` for `main` pushes (pitfalls.md §1c, confirmed: push `0b0ae53ea` ran uncancelled 19:38Z→02:28Z next day) means a bad combined change can occupy a runner for hours on `main` with no way to cancel it via a follow-up push.

2. **Staged PRs by risk tier** (spikes first as read-only/no-workflow-change branches, then one PR per implementation lever, N-server seam as a doc-only PR with zero `build.yml` risk).
   - *Strength*: matches requirements.md's own Risk Control section ("stage it behind its own PR... so a problem in the risky change doesn't block or get conflated with the safer wins") and directly repeats the lesson of this session's #1165 (bundled sharding + gate-membership change, broke `main` immediately) being superseded by the simpler #1166. Each PR gets an independent revert path (`git revert` on one small commit) and an independently attributable CI cycle.
   - *Weakness*: more total CI cycles (each `build.yml` run is ~25-40 min per shard, run in parallel — but still N separate pushes to validate instead of one), more PR-review overhead.

3. **Measure-first-then-implement** (run the GH concurrency-ceiling check and the Bun global-store timing spike as pure investigation before touching any workflow file).
   - *Strength*: avoids committing to a shard-count or Bun-install change the data doesn't support.
   - *Weakness*: on its own this is a sequencing rule, not a project shape — it doesn't say whether the resulting implementation stories land together or separately, so it has to be combined with (1) or (2) anyway.

**Chosen: (2) Staged PRs by risk tier, with the (3) measure-first spikes run first inside that staging.** The four implementation levers (fullyParallel flip, browser-install isolation, Bun experiment, sample-triage) are architecturally independent — none of their diffs overlap in `build.yml`/`playwright.config.ts` in a way that requires them to land together — so there is no efficiency argument for bundling them, and the #1165/#1166 history is direct, repo-specific evidence that bundling topology changes increases both blast radius and debugging cost. Rejected: (1) all-in-one (too much blast radius for zero coupling benefit, given pitfall 1c's uncancellable-`main`-push risk); (3) alone (doesn't decide packaging). Recorded as a Pattern Decision row below.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| shard | A horizontal partition of the 193-spec Playwright suite assigned to one CI job via `--shard=N/M`. | Currently 10-way per job, 20 shard jobs total (`e2e-next-shard` + `e2e-bun-shard`). |
| blob report | Playwright's compact per-shard binary report format (`--reporter=blob`), merged across shards via `playwright merge-reports`. | Filenames auto-embed shard index; already safe to co-locate (pitfalls.md §2). |
| worker | A Playwright OS process that executes tests in-process within one job. | Today always exactly 1 per job (`workers: 1`, `playwright.config.ts:65`). |
| `workerIndex` vs `parallelIndex` | `workerIndex` increases monotonically across worker restarts (a crashed/retried worker gets a new value, never reused); `parallelIndex` stays stable in `[0, workers)`. | `parallelIndex` is the safer key for any future per-worker resource (e.g. a fixed port range); `workerIndex`-derived ports can drift to unexpectedly high numbers on a flaky run. |
| `fullyParallel` | Playwright config flag controlling whether tests *within* one file can run across multiple workers. | Currently `false` (`playwright.config.ts:59`). This project flips it to `true` while keeping `workers: 1` — a no-op on actual concurrency, but it changes `--shard`'s balancing granularity from file-count to test-count. |
| `DEFAULT_MEMBERS` | Hardcoded array in `web-app/scripts/ci/web-build-gate.mjs` (`["detect-changes", "web-build-smoke-next", "e2e-next"]`) naming which CI job results gate a PR by default when `WEB_BUILD_GATE_MEMBERS` isn't set. | The #1165 incident (pitfalls.md §1b) came from touching this without the gate ever running against real (known-failing) data first. This plan's stories must not add any new job to this list. |
| `WEB_BUILD_GATE_MEMBERS` | A GitHub repo variable that overrides `DEFAULT_MEMBERS` without a workflow-file edit (e.g. to opt `e2e-bun` into gating later). | Not touched by this plan — the new browser-isolation job is informational, like `storybook-build`, and is never added to gate membership. |
| `TestServer` | The TypeScript class in `tests/e2e/helpers/test-server.ts` that spawns, health-checks, and tears down one `stapler-squad` backend instance for e2e tests. | Already per-instance-safe on port, tmux-socket, and PID-scoped test-dir/bin-dir (architecture.md §1) — the blocker for per-worker use is lifecycle/discovery (`global-setup.ts`'s single call + unindexed `process.env`), not `TestServer` itself. |
| `global-setup.ts` / `global-teardown.ts` | Playwright lifecycle hooks that run once in the main process before/after all tests; today call `startGlobalTestServer()`/`stopGlobalTestServer()` exactly once and publish `TEST_SERVER_URL`/`TEST_SERVER_TESTDIR` into `process.env` for every worker to inherit. | The single point that makes the suite single-server-shaped by construction. |
| `e2e-next-blob-${shard}` / `e2e-bun-blob-${shard}` | Per-shard artifact names (`build.yml:418`, `build.yml:585`) already shard-index-qualified. | Confirmed already unique per shard — pitfalls.md §2's "re-verify artifact-name uniqueness if shard count changes" check passes at the current count; must be re-checked if shard count ever changes. |
| baseline-classification comparator | `scripts/e2e-baseline-compare.ts`, invoked by the `e2e-bun` job, classifies each spec pass/flaky/fail against the recorded Next-build baseline. | Must not regress — this plan touches only install steps and config flags upstream of it, never its classification logic. |
| `detect-changes` / `prepare` | `build.yml` jobs: `detect-changes` gates whether shard jobs run at all on a PR based on changed-file relevance; `prepare` produces the shared Next-mode build artifact every shard job downloads. | Both are untouched by this plan's stories. |
| `continue-on-error` project | A Playwright project (`desktop-firefox`, `desktop-webkit`, `webkit-mobile`) whose failures don't fail the job — informational only. | These are exactly what the new dedicated browser-install job pair (Story 3.2.1) exists to serve; `chromium`, `chromium-dom`, and `mobile-chromium` remain the only gating projects and stay Chromium-only. |
| `webkit-mobile` vs `mobile-webkit` | Two distinct, similarly-named Playwright projects: `webkit-mobile` (`playwright.config.ts`'s `webkitMobileProjects()`, gated by `fs.existsSync(webkit.executablePath())`, `testMatch: '**/mobile-webkit.spec.ts'`) carries the 6 `next-e2e-baseline.json` firefox/webkit/webkit-mobile baseline-"pass" entries Story 3.2.1 must not drop; `mobile-webkit` (gated by `process.env.PW_WEBKIT === '1'`, `testMatch: MOBILE_SPECS`) is a separate, unrelated Files-viewer-mobile coverage project. | Adversarial-review finding: this naming collision is plausibly why an earlier draft of Story 3.2.1 named only "mobile-webkit" and never mentioned `webkit-mobile` at all, which is exactly how the baseline-dropping bug (see Story 3.2.1) was missed during planning. |
| Bun global virtual store | Bun 1.4's opt-in install-cache mode (`linker = "isolated"` + `globalStore = true` in `bunfig.toml`, or `BUN_INSTALL_GLOBAL_STORE=1 bun install --linker isolated`). | Advertised 7x win is for large dependency trees on a warm cache; `tests/e2e`'s own tree is 7 packages, so the win here is unverified and small — treated as a measured spike, not an assumed win. |
| shared-server rearchitecture (aka N-server rearchitecture, per-worker-server rearchitecture) | Lifting the single-shared-backend-server constraint so each Playwright worker gets its own isolated `TestServer` instance, unlocking true in-process `fullyParallel`/multi-worker execution. Canonical name per requirements.md's Rabbit Holes section ("Shared-server rearchitecture"). | This plan also uses "N-server rearchitecture" and "per-worker-server rearchitecture" for the same concept (e.g. the feature summary line, Pattern Decisions, Phase 5's header) — all three names refer to the one deferred Phase 5 item; prose instances elsewhere in this plan may still vary, but they all resolve to this row. |
| CU (cost unit) | Price-weighted token estimate: `in + 1.25·cache_write + 0.1·cache_read + 5·out`, per `~/.claude/skills/sdd/skills/ESTIMATION.md`. | Used in the Effort Estimate section below instead of human-time units. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall project staging shape | Staged PRs by risk tier, spikes run first | This plan's Step 0.5 | All-in-one PR; measure-first-only (no packaging decision) | #1165/#1166 precedent: bundling topology changes increased blast radius and debugging cost for no coupling benefit; `build.yml`'s `cancel-in-progress: false` on `main` makes an uncancelled bad combined change costly (pitfalls.md §1c). |
| Shard rebalancing | None (infra config, not OOP domain modeling) — native `fullyParallel: true` + Playwright's own `--shard` | build-vs-buy.md §1 | Third-party timing-based shard balancer (NSXBet/test-orchestrator: 4 stars, single org, 9 months old) or Currents' paid SaaS dashboard | build-vs-buy.md §1 verdict: "Not recommended as a dependency; the pattern (switch `fullyParallel: true`...) is cheap, native, and worth trying first." A 4-star single-org tool breaking the main gate is a worse failure mode than today's uneven-but-correct shards. |
| Browser-install isolation | None (infra config) — mirrors `e2e-video.yml`'s existing single-browser-install-per-job precedent | stack.md §"Summary of unused-but-available levers" item 2; `e2e-video.yml:171` (`npx playwright install chromium --with-deps`) | Leave Firefox/WebKit installed in all 20 gating shard jobs (status quo) | stack.md: a real, measurable per-shard cost (3x browser download/install) "paid on every shard for coverage that's informational on all but one project." |
| Deferred N-server seam | Worker-scoped Playwright fixture adapting the existing `TestServer` class (closest to an Adapter/extension-point shape, not a named GoF pattern) | architecture.md §2-3; build-vs-buy.md §3-4 | Bespoke N-server orchestration from scratch; a managed/hosted e2e runner (BrowserStack/Sauce Labs/Playwright Cloud) | build-vs-buy.md §3: "re-derives work `TestServer` already does correctly." build-vs-buy.md §2: none of these services isolate *this repo's own* backend per shard — the actual unsolved constraint. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| Single-shared-backend-server constraint (`global-setup.ts`'s one `TestServer` singleton, `playwright.config.ts`'s whole-file `fullyParallel`/`workers` settings) | Blocks true in-process multi-worker parallelism; `playwright.config.ts:59,65` pin `fullyParallel: false`/`workers: 1` for the whole suite. | **Isolate via seam.** Design the worker-scoped-fixture extension point now (Phase 5, doc-only); do not implement N-server orchestration in this round's Phase 3 implementation scope. Story 3.3.1 is the forcing function that re-evaluates this disposition once Phase 3's real numbers land. | Per architecture.md §7: duration-aware rebalancing (this round's Phase 3 scope) is cheaper and safer and attacks the dominant measured cost directly (>2x shard-duration spread, e.g. 15m49s vs. 31m36s on the same 10-way `e2e-next-shard` split, `gh run view 40184788547`); the correctness risk of per-spec shared-state assumptions is substantially but not fully de-risked by the already-running 20-job matrix (a bounded grep, not a full 193-file audit, per architecture.md §5); the resource-contention risk of N server+tmux stacks sharing one CI runner's CPU/RAM is genuinely unverified and needs iterative, multi-CI-cycle validation this round doesn't budget for (mirroring the #1165→#1166 saga's own cost). |
| Cross-shard build-artifact caching (fixed per-shard rebuild cost; requirements.md's "Fixed per-shard overhead" Rabbit Hole) | Each of the 20 shard jobs independently repeats Go/Bun setup and (for `e2e-bun`) a full `make stapler-squad` rebuild — not shared/cached across shards. | **Not pursued this round.** No sharing scheme is implemented; the existing `prepare`-job-shared Next-mode artifact is left as-is and `e2e-bun`'s per-shard rebuild is left as-is. | architecture.md §6 found fixed per-shard overhead (~3-3.5 min) is only ~10-20% of the slowest shard's total time, while shard-duration imbalance (>2x spread) is the dominant cost (architecture.md §6, "the dominant driver of the slowest-shard tail is shard imbalance, not fixed overhead") — so a cross-shard artifact-sharing scheme, which would add real complexity (concurrent-write hazards on a single repo-root binary shared by multiple jobs, and the `WEB_BUILD_STAMP` seam's same-build-different-seed concerns requirements.md's Rabbit Hole itself names), isn't worth pursuing this round given its ROI is low relative to the already-prioritized duration-rebalancing lever (Story 3.1.1). |

---

## Migration Plan

Omitted — no schema/data changes. This project touches CI workflow config, Playwright config, and documentation only.

## Observability Plan

- **Logs**: Standard GitHub Actions job logs (Actions UI, `gh run view <id>`, `gh api repos/tstapler/stapler-squad/actions/jobs/<id>`). No new logging infrastructure.
- **Metrics**: Per-shard job start/end timestamps (already captured by GitHub Actions and already used in this plan's own before/after Given-When-Then examples, e.g. `gh run view 40184788547`). No new metrics pipeline — mirrors how PR #1166's own body cited exact run/job IDs and timestamps as evidence.
- **Alerts**: None — matches requirements.md's Observability Requirements section ("Standard CI signal... is sufficient").

## Risk Control

- **Feature flag**: N/A for `playwright.config.ts`/`build.yml` edits — a config-file revert is the flag. `WEB_BUILD_GATE_MEMBERS` (the existing repo variable) remains the one real gating lever in this area and is not touched by any story here.
- **Rollback procedure**: Each story in Phase 3 lands as its own PR/commit; rollback is `git revert <commit>` on that one PR, per requirements.md's Risk Control section ("Rollback for any CI change is trivial by construction"). No story in this plan depends on another story's `build.yml` diff, so any one can be reverted independently.
- **Staged rollout**: Per Step 0.5's chosen shape — spikes (Phase 2) first and read-only, then one PR per Phase 3 story, each validated by its own full CI cycle before the next is opened. Phase 5 (N-server seam) is a documentation-only PR with zero `build.yml`/`playwright.config.ts` diff and zero CI-cycle risk.

## Unresolved Questions

- [ ] Does the account's actual GitHub Actions concurrency ceiling (Free=20/Pro=40/Team=60) leave headroom above the current 20 concurrently-running shard jobs plus `build.yml`'s ~15 other jobs? — blocks Story 2.1.1's go/no-go, which in turn blocks any future shard-count increase (not itself a Phase 3 story, since Phase 3 keeps shard count at 10). Owner: Tyler (billing-plan lookup only he can run against his own account UI/API token).
- [ ] Does Bun's global virtual store produce a measurable win on `tests/e2e`'s small (7-package) devDependency tree, or is the 7x figure (a large-tree number) simply inapplicable here? — blocks Story 2.2.1's go/no-go for whether to touch `bunfig.toml` at all. Owner: implementing agent (empirical, no human judgment call needed beyond reading the number).
- [ ] What sample size and selection criteria make the bounded test-content triage (Story 4.1.1) defensible as "not rubber-stamping, not exhaustive," per requirements.md's explicit warning against both failure modes? — blocks Story 4.1.1a. Resolved in this plan (see Story 4.1.1 below) but flagged here since it was an open question in requirements.md at hand-off; owner: this plan's methodology, confirmed by Tyler before the triage agent runs if he wants to adjust the sample size.

## Dependency Visualization

```
Phase 1: Sync
  1.1.1 Rebase onto origin/main
        │
        ├────────────┬─────────────────┬──────────────────┐
        ▼            ▼                 ▼                  ▼
Phase 2: Spikes (parallel, read-only, no build.yml edits)
  2.1.1 GH concurrency   2.2.1 Bun global-store
  ceiling check          timing spike
        │                      │
        │ (informs future      │ (go/no-go for
        │  shard-count          │  bunfig.toml touch,
        │  changes only;        │  independent of 3.1/3.2)
        │  does not block       │
        │  Phase 3)             │
        ▼                      ▼
Phase 3: Safe wins (each its own PR, independently revertable)
  3.1.1/3.1.2 fullyParallel:true        3.2.1/3.2.2 Browser-install isolation
  (depends on 1.1.1 only)               (depends on 1.1.1 only; independent of 3.1)
        │                                       │
        └───────────────┬───────────────────────┘
                         ▼
                3.3.1 Post-Phase-3 go/no-go checkpoint
                (depends on 3.1.2 AND 3.2.2 both merged;
                 ALSO depends on 5.1.1 (Phase 5) having
                 landed its doc, since 3.3.1 appends a
                 dated decision note to the top of it)
                         ▲
                         │ (5.1.1's doc must exist before 3.3.1 can append to it —
                         │  but 5.1.1 itself has no dependency on Phase 3 or 3.3.1)
Phase 4: Bounded test-content triage      Phase 5: Deferred design-only
  (depends on 1.1.1 only;                   (fully independent of Phase 3's
   independent of Phase 3 and 3.3.1)         build.yml/playwright.config.ts;
  4.1.1 Sample-triage methodology +          independent of Phase 4)
  execution + candidate list                5.1.1 N-server worker-scoped-fixture
                                             seam doc — should land before 3.3.1
                                             runs, so the append target exists
```

---

## Phase 1: Sync & Baseline

### Epic 1.1: Rebase onto the real, live `build.yml`

**Goal**: Every subsequent workflow-file edit in this plan is made against `origin/main`'s actual post-#1166 `build.yml` (confirmed at commit `0551de0b858b334a943fec89667ced4eb5f791be` at plan time), not the stale local checkout, which research confirmed is 26 commits behind and predates the 10-way shard matrix entirely.

#### Story 1.1.1: Create the feature branch from `origin/main`, not stale local `main`
**As a** worker agent implementing any story in Phase 3, **I want** my branch to start from `origin/main`'s current `build.yml`/`playwright.config.ts`, **so that** my diff applies cleanly and doesn't silently resurrect pre-#1166 unsharded jobs.
**Acceptance Criteria**:
- A new branch exists whose `build.yml` contains the `e2e-next-shard`/`e2e-bun-shard` 10-way matrix jobs.
  - *Given* `origin/main` is at `0551de0b858b334a943fec89667ced4eb5f791be` (or later), *When* a worker runs `git fetch origin main && git switch -c <branch> origin/main`, *Then* `grep -c "matrix:" .github/workflows/build.yml` finds the `e2e-next-shard`/`e2e-bun-shard` `strategy.matrix.shard: [1, 2, ..., 10]` blocks, and `git log --oneline -1 -- .github/workflows/build.yml` shows a commit at or after `5a7279bdb8` (PR #1166).
**Files**: none changed — this is a `git` operation task.

##### Task 1.1.1a: Fetch and branch from `origin/main` (cost class XS)
- Run `git fetch origin main`.
- Create the working branch from `origin/main`, e.g. `git switch -c e2e-parallel-spikes origin/main` for Phase 2, and a separate branch per Phase 3 story (e.g. `e2e-fullyparallel`, `e2e-browser-install-isolation`) each also based on `origin/main`, not on each other — this keeps the "independently revertable" property from the Risk Control section.
- Files: none (git metadata only).

---

## Phase 2: Spikes (measure before implementing)

### Epic 2.1: GitHub Actions concurrency ceiling verification

**Goal**: Confirm whether the account's actual billing plan leaves headroom above the 20 shard jobs `build.yml` already runs concurrently, before any future shard-count change (not itself part of this round's scope) is ever proposed.

#### Story 2.1.1: Verify the account's GitHub Actions concurrency ceiling
**As a** plan owner, **I want** to know the account's actual concurrent-job cap, **so that** a future shard-count increase isn't recommended on the unverified "not binding at 8-10 shards" anecdote alone (stack.md's explicit flag).
**Acceptance Criteria**:
- The account's plan tier (Free=20, Pro=40, Team=60, Enterprise Cloud=500 concurrent standard-runner jobs) is recorded with a command and its output.
  - *Given* the repo is `tstapler/stapler-squad` (public, personal), *When* a worker runs `gh api /user --jq '.plan.name // "NOT_EXPOSED"'` and cross-references `build.yml`'s current job count (20 shard jobs + ~15 other concurrent jobs per stack.md), *Then*, if the output is `NOT_EXPOSED` (architecture review live-tested this exact account and confirmed its `/user` response has no `plan` key at all, so the bare `.plan.name` query silently prints nothing rather than erroring — a 403 is not the only failure shape) *or* the call returns a non-2xx status, the worker falls back to reading `https://github.com/settings/billing` directly and records which path produced the answer; the finding is written into a short note (see Task 2.1.1a) stating the plan tier and whether 20 shard jobs + ~15 other jobs is within or near that cap.
- No `build.yml` edit is made as part of this story — it is read-only.
  - *Given* this story's only output is a finding note, *When* `git diff --stat` is run against the spike branch, *Then* it shows zero changes to any `.github/workflows/*.yml` or `tests/e2e/*.ts` file.
**Files**: `project_plans/e2e-test-parallelization/decisions/concurrency-ceiling-finding.md` (new, short note — not an ADR, since this is a measurement, not a design decision).

##### Task 2.1.1a: Run the billing/plan check and record the finding (cost class XS)
- Run `gh api /user --jq '.plan.name // "NOT_EXPOSED"'`. Architecture review live-tested this account and confirmed the plain `.plan.name` query returns a silent empty result (no `plan` key in the response at all, not a 403) — checking only for an HTTP error misses this case, so the `// "NOT_EXPOSED"` fallback is required, not optional.
- If the result is `NOT_EXPOSED` or the call errors, fall back to reading `https://github.com/settings/billing` directly (ask Tyler if the agent has no browser access) rather than writing a blank or guessed plan tier into the finding note.
- Write a 5-10 line finding note: plan tier (and which method produced it — API field or billing-UI fallback), concurrent-job cap for that tier, and a one-line verdict on whether the current 20+~15 concurrent jobs in `build.yml` leaves headroom.
- Files: `project_plans/e2e-test-parallelization/decisions/concurrency-ceiling-finding.md`.

### Epic 2.2: Bun 1.4 global-store linker experiment

**Goal**: Measure, don't assume, whether `linker = "isolated"` + `globalStore = true` meaningfully speeds up `tests/e2e`'s `bun install --frozen-lockfile` step, given stack.md's flag that the advertised 7x figure is a large-dependency-tree number and `tests/e2e`'s own tree is only 7 packages.

**Scope note — the other two Bun sub-levers requirements.md named are already closed out, not unaddressed gaps**: requirements.md's in-scope bullet names three Bun sub-levers (install/cache strategy, `bunx` vs `npx` overhead, and "Bun's own HTTP server or process-spawning primitives if relevant to `global-setup.ts`'s server lifecycle"). This epic covers only the first. The second — `bunx` vs `npx` — was already confirmed **not** a lever by Phase 2 research: stack.md's explicit finding (`research/stack.md:74-86`) is that both just resolve/exec the same installed `node_modules/.bin/playwright` once Playwright is a committed `devDependency`, so "switching `npx` → `bunx` in these jobs is cosmetic, not a performance lever" — don't recommend it, and this plan doesn't. The third — Bun's own HTTP/process-spawning primitives for `global-setup.ts`'s server lifecycle — is subsumed by Phase 5's deferred shared-server rearchitecture work (Story 5.1.1), since that's precisely the effort that would redesign `global-setup.ts`'s server lifecycle; it isn't a separate lever this round skipped, it's folded into the one deferred item.

#### Story 2.2.1: Time `bun install` with and without the global store, decide go/no-go
**As a** plan owner, **I want** an empirical before/after install-time comparison, **so that** a `bunfig.toml` change isn't shipped on an assumed win that doesn't transfer to this repo's small dependency tree.
**Acceptance Criteria**:
- A timing comparison exists for `tests/e2e`'s `bun install --frozen-lockfile`, with and without the global store, on a cold `node_modules`.
  - *Given* `tests/e2e/node_modules` is deleted, *When* a worker runs `time bun install --frozen-lockfile` (baseline) and then, with `bunfig.toml` set to `[install]\nlinker = "isolated"\nglobalStore = true`, deletes `node_modules` again and re-runs `time bun install --frozen-lockfile`, *Then* both wall-clock times are recorded in the finding note, each run at least twice to account for filesystem-cache noise.
- A go/no-go verdict is recorded based on the measured delta, not the vendor's 7x claim.
  - *Given* the two timings from the step above, *When* the delta is less than, say, 1-2 seconds (plausible given the 7-package tree per stack.md), *Then* the verdict is "no-go, cosmetic win not worth a `bunfig.toml` change"; *When* the delta is materially larger, *Then* the verdict is "go" and a follow-up task (not in this plan — a new, separately-opened PR, per the staging decision) adds the `bunfig.toml` change.
**Files**: `project_plans/e2e-test-parallelization/decisions/bun-global-store-spike-finding.md` (new).

##### Task 2.2.1a: Run the local timing comparison and record the verdict (cost class S)
- In a scratch copy of `tests/e2e` (or a throwaway branch), run the two timed `bun install --frozen-lockfile` comparisons described above.
- Write the finding note with both timings, the delta, and the go/no-go verdict.
- Do not edit `tests/e2e/bunfig.toml` as part of this task regardless of verdict — that's explicitly out of this plan's Phase 3 scope (a "go" verdict becomes its own future story/PR, per the staging decision in Step 0.5).
- Files: `project_plans/e2e-test-parallelization/decisions/bun-global-store-spike-finding.md`.

---

## Phase 3: Safe, Cheap Wins — implement now, each as its own PR

### Epic 3.1: `fullyParallel: true` shard rebalancing

**Goal**: Attack the measured >2x shard-duration variance (architecture.md §6: `e2e-next-shard` 15m49s fastest vs. 31m36s slowest, same 10-way split, run `40184788547`) by having Playwright's `--shard` balance by test count instead of file count — a config flip that does not touch the shared-server constraint at all (`workers: 1` is unchanged) and so carries none of the correctness/resource-contention risk flagged for the deferred N-server item.

#### Story 3.1.1: Flip `fullyParallel: true` in `playwright.config.ts`, keep `workers: 1`
**As a** contributor waiting on `e2e-next`/`e2e-bun`, **I want** the 10-way shard split to balance by individual test rather than whole file, **so that** no single shard gets stuck with several slow files while another finishes early.
**Acceptance Criteria**:
- `playwright.config.ts:59` reads `fullyParallel: true`; `playwright.config.ts:65` (`workers: 1`) is unchanged.
  - *Given* the current `origin/main` file has `fullyParallel: false,` at line 59, *When* the edit is applied, *Then* `grep -n "fullyParallel\|workers:" tests/e2e/playwright.config.ts` shows `fullyParallel: true,` and `workers: 1,` with no other line in the `defineConfig` block changed.
- `actionlint` and the existing `web-build-gate.test.mjs` suite both pass against the edited tree (requirements.md's own constraint).
  - *Given* no `build.yml` job name or `needs:` graph changes in this story, *When* `node --test web-app/scripts/__tests__/web-build-gate.test.mjs` runs, *Then* every existing test (e.g. "fails for a member Next job failure or cancellation") still passes unchanged, since this story touches zero workflow-file content.
**Rollback trigger**: Story 3.1.2's comparator check (below) reports any new `severity: "fail"` finding, or its cross-shard file-split check finds a file from the 54-file `beforeAll`/`afterAll` pool now split across shards without a documented "safe because X" review — revert this commit (`fullyParallel: false` restored) immediately rather than attempting a targeted per-file exclusion first.
**Files**: `tests/e2e/playwright.config.ts`.

##### Task 3.1.1a: Edit `playwright.config.ts` (cost class XS)
- Change line 59 from `fullyParallel: false,` to `fullyParallel: true,`. Leave `workers: 1,` (line 65) untouched.
- Add a one-line comment above the changed field citing why (balances `--shard` by test count, not file count; `workers: 1` keeps the single-shared-server constraint unchanged) — per this repo's Code Comments convention (why, not what).
- Files: `tests/e2e/playwright.config.ts`.

##### Task 3.1.1b: Open the PR, run local gates (cost class XS)
- Run `actionlint .github/workflows/build.yml` locally (no change expected here, but the requirements doc requires it be checked for any PR touching this area — confirms the PR's base is clean).
- Run `cd web-app && node --test scripts/__tests__/web-build-gate.test.mjs` to confirm the gate test suite is green pre-push.
- Open the PR against `main` (ready for review, not draft, per this repo's `tstapler/stapler-squad` override).
- Files: none (PR metadata only).

#### Story 3.1.2: Validate the before/after shard-duration variance on real CI
**As a** plan owner, **I want** a real CI run's before/after shard-timing comparison, **so that** the fix is proven, not assumed, per requirements.md's Success Metrics and Risk Control sections.
**Acceptance Criteria**:
- The PR's CI run produces 10 `e2e-next-shard` job durations and 10 `e2e-bun-shard` job durations, and the spread between fastest and slowest is measurably smaller than the baseline run's 2.0x/2.2x spread (architecture.md §6).
  - *Given* baseline run `40184788547` recorded `e2e-next-shard` fastest=15m49s/slowest=31m36s (2.0x) and `e2e-bun-shard` fastest=15m48s/slowest=34m4s (2.2x), *When* this story's PR's CI run completes, *Then* `gh api repos/tstapler/stapler-squad/actions/runs/<new-run-id>/jobs --jq '.jobs[] | select(.name | startswith("e2e (Next build) shard")) | .name + " " + (.started_at) + " " + (.completed_at)'` is used to compute the new fastest/slowest spread, and the result is recorded in the PR body (mirroring how PR #1166's own body cited exact run/job IDs).
- The slowest-shard-to-final-verdict wall-clock time (the literal requirements.md Success Metric, not the duration-spread proxy above) is measured before and after.
  - *Given* baseline run `40184788547`'s shard start timestamps and its `web-build-gate` job's completion timestamp are both readable via `gh api repos/tstapler/stapler-squad/actions/runs/40184788547/jobs`, *When* this story's PR's CI run completes, *Then* the same slowest-shard-start-to-`web-build-gate`-completion delta is computed for the new run and recorded alongside the spread numbers in the PR body — a spread improvement with no improvement (or a regression) in this number is not sufficient to call the story done.
- The comparator (`scripts/e2e-baseline-compare.ts`, run inside the `e2e-bun` job's "Run the baseline comparator" step, `build.yml:644-655`) reports zero new `severity: "fail"` findings on this PR's CI run relative to the pre-flip baseline.
  - *Given* `main`'s pre-flip `e2e-bun` job runs green today (no open `fail`-severity findings — the gate is not currently red), *When* this story's PR's CI run completes, *Then* `gh run view <new-run-id> --log | grep -A5 "Run the baseline comparator"` (or the job's `$GITHUB_STEP_SUMMARY`) shows the comparator's exit code is 0 and its printed findings list contains zero `severity: "fail"` entries; any new `fail`-severity finding is this story's go/no-go blocker, not a follow-up to triage later.
- The comparator's `flaky`-classified finding count does not increase relative to the pre-flip baseline (pre-mortem P2 item #5: a "fail"-only check would miss `fullyParallel`'s test-redistribution quietly converting solid passes into flaky results without tripping any blocker).
  - *Given* the pre-flip baseline's comparator output on `main` lists its current count of `severity: "flaky"`/baseline-class-`flaky` findings (recorded alongside the zero-`fail` confirmation above), *When* this story's PR's CI run completes, *Then* the new run's comparator output's flaky-finding count is compared against that baseline count, and any increase is treated the same as a new `fail`-severity finding — this story's go/no-go blocker, not a follow-up to triage later.
- Any of the 54 files architecture.md §5 flagged as using only file-scoped `beforeAll`/`afterAll` that land split across more than one distinct shard job post-flip are identified and individually reviewed before the flip is called safe — this is the direct remediation for the risk `fullyParallel: true` introduces (it changes `--shard`'s splitting unit from whole-file to individual-test, removing the "whole file always lands in one job" premise architecture.md §5's safety argument for these 54 files depends on).
  - *Given* Playwright's merged JSON report (the same `tests/e2e/all-blobs` → `report.json` merge the comparator already consumes, one per build mode) is produced per this run, *When* a worker re-derives the current 54-file list (re-run architecture.md §5's grep against this PR's branch, since the count may have shifted) and cross-references each file's tests against which shard's blob produced each result, *Then* the PR body includes a table "file: shard indices observed" for all 54, and for any file showing more than one distinct shard index, the PR body states, file by file, either "safe because `<specific reason tied to what the beforeAll/afterAll actually does>`" or "needs `test.describe.configure({ mode: 'serial' })` / explicit shard-pinning / exclusion before this flip can be merged" — an unreviewed split file blocks the merge; it is not an acceptable known-gap.
- This is an explicit wall-clock blocker, not an agent-token cost — see Effort Estimate below.
**Rollback trigger**: the comparator check above reports any new `fail`-severity finding, or the 54-file split review above concludes a file is unsafe — revert Story 3.1.1's `playwright.config.ts` commit immediately rather than attempting a targeted per-file exclusion as a first response.
**Files**: none (CI run + PR body text only).

##### Task 3.1.2a: Push, wait for CI, record the comparison (cost class XS — agent time; wall-clock blocker — CI time)
- Push the branch, let the full `build.yml` run complete (~25-40 min, per existing baseline).
- Pull the new run's shard job timestamps via `gh api`, compute fastest/slowest/spread and the slowest-shard-to-`web-build-gate`-verdict wall-clock delta, exactly as the Given-When-Then above specifies.
- Run the comparator check: read the `e2e-bun` job's "Run the baseline comparator" step output for this run and confirm zero new `fail`-severity findings.
- Run the 54-file shard-split check: re-derive the file list, cross-reference shard assignment, and write the "file: shard indices observed" table; manually review any file that split.
- Append all of the above to the PR description.
- Files: none (PR body text).

### Epic 3.2: Isolate Firefox/WebKit installs out of the 20 gating shard jobs

**Goal**: Remove the Firefox/WebKit browser-install cost (stack.md: "a real, measurable per-shard install-time cost... paid on every shard... for coverage that's informational on all but one project") from every `e2e-next-shard`/`e2e-bun-shard` job, moving it to a dedicated job pair that mirrors `e2e-video.yml`'s existing Chromium-only install pattern — without touching `web-build-gate`'s membership semantics (pitfalls.md §1b), and without silently dropping the 6 `next-e2e-baseline.json` baseline-"pass" entries that depend on Firefox/WebKit (2× `desktop-firefox`, 2× `desktop-webkit`, 2× `webkit-mobile`) out of the baseline comparator's view. **Root cause this plan must not repeat** (see the Domain Glossary's `webkit-mobile` vs `mobile-webkit` row): an earlier draft of this epic named only "mobile-webkit" and never mentioned `webkit-mobile` — the project that actually carries those 6 baseline entries — which is exactly how the baseline-dropping risk below went unnoticed during planning.

#### Story 3.2.1: Chromium-only installs in the 20 gating shard jobs; new dedicated job pair for Firefox/WebKit-only projects, feeding the same comparator
**As a** contributor waiting on `e2e-next`/`e2e-bun`, **I want** the 20 gating shard jobs to stop paying for Firefox/WebKit installs that only serve a handful of informational Playwright projects, **so that** every gating shard's fixed per-job overhead shrinks without losing the informational cross-browser coverage or silently breaking the baseline comparator.
**Acceptance Criteria**:
- `e2e-next-shard` and `e2e-bun-shard`'s "Install Playwright + test dependencies" steps install only Chromium, AND their test-run commands are restricted to the Chromium-based projects so they stop attempting (and failing on) Firefox/WebKit-dependent tests entirely.
  - *Given* `build.yml:392`/`build.yml:559` currently read `npx playwright install chromium firefox webkit --with-deps`, and `build.yml:404`/`build.yml:571` currently run `npx playwright test --reporter=list,blob --shard=${{ matrix.shard }}/10` with **no `--project` filter** (so today all configured projects, including `desktop-firefox`/`desktop-webkit`/`webkit-mobile`, are sharded together using the browsers the install step just installed), *When* both install lines are changed to `npx playwright install chromium --with-deps` **and** both test-run lines gain `--project=chromium --project=chromium-dom --project=mobile-chromium`, *Then* `grep -n "playwright install\|playwright test" .github/workflows/build.yml` shows the chromium-only install form and the three-project filter in both `e2e-next-shard` and `e2e-bun-shard`. The install change without the run-command change is called out here as unsafe on its own: without the `--project` filter, Playwright still tries to launch the now-uninstalled Firefox/WebKit binaries for `desktop-firefox`/`desktop-webkit`/`webkit-mobile` and records those as hard launch-failures in every shard's blob report, instead of cleanly excluding them.
- A new job pair — `e2e-secondary-browsers-next` and `e2e-secondary-browsers-bun`, mirroring the existing `e2e-next-shard`/`e2e-bun-shard` twin-job-not-matrix precedent — installs Firefox+WebKit once each and runs `desktop-firefox`, `desktop-webkit`, and **`webkit-mobile`** (not just the differently-gated `mobile-webkit` — see the Domain Glossary row and this Epic's Goal), and each uploads its blob report under a name the existing merge steps already sweep up with no further edit.
  - *Given* no such job exists in `origin/main`'s `build.yml` today, and `e2e-next`'s own merge step (`build.yml:448-454`) downloads artifacts matching `pattern: e2e-next-blob-*` while `e2e-bun`'s merge step (`build.yml:615-621`) downloads `pattern: e2e-bun-blob-*`, *When* `e2e-secondary-browsers-next` runs `npx playwright install firefox webkit --with-deps`, then `npx playwright test --reporter=list,blob --project=desktop-firefox --project=desktop-webkit --project=webkit-mobile` (plus `--project=mobile-webkit` when `PW_WEBKIT=1` is set on the job, preserving the original scope item for `MOBILE_SPECS` coverage) against the Next build, and uploads its blob report as `e2e-next-blob-secondary`; and `e2e-secondary-browsers-bun` does the same against the Bun build (`make stapler-squad`'s Bun-mode build, mirroring `e2e-bun-shard`'s own build step) and uploads as `e2e-bun-blob-secondary`, *Then* both artifact names match their respective job's existing `e2e-*-blob-*` download glob with zero change needed to the download/merge steps themselves, and `e2e-next`'s `needs:` (`build.yml:426`) gains `e2e-secondary-browsers-next` while `e2e-bun`'s `needs:` (`build.yml:593`) gains `e2e-secondary-browsers-bun` — ordering-only additions, not new `if:` conditions, since both new jobs are `continue-on-error: true` at the job level and must never block the merge step from running even when they fail.
- The comparator (`scripts/e2e-baseline-compare.ts`) reports all 6 of `next-e2e-baseline.json`'s firefox/webkit/webkit-mobile baseline-"pass" entries as non-`fail` severity on a real CI run after this story ships.
  - *Given* `next-e2e-baseline.json` carries 2× `desktop-firefox`, 2× `desktop-webkit`, and 2× `webkit-mobile` entries with `class: "pass"` (confirmed via `grep -c 'desktop-firefox\|desktop-webkit\|webkit-mobile' tests/e2e/baseline/next-e2e-baseline.json` against `origin/main`), *When* this story's PR's CI run completes and the `e2e-bun` job's "Run the baseline comparator" step output is read, *Then* none of those 6 specs appears with `severity: "fail"` — each is either absent from the findings list (passed cleanly) or appears with `severity: "report"`/`"warn"` only. If any appears as `"fail"`, that is this story's go/no-go blocker, and the fix is either (a) confirming the secondary job's blob genuinely reached the merge (re-check the artifact-name match above), or (b) explicitly removing that spec from `next-e2e-baseline.json` in this same PR with a called-out comment — never silently.
- `route-coverage-manifest.ts` reports no coverage drop caused by this story's change.
  - *Given* `scripts/route-coverage-manifest.ts` maps each `web-app/src/app/route-meta.ts` route to the specs that visited it via `next-e2e-baseline.json`'s `routesVisited` field, disqualifying any route whose only visitors are baseline-`fail`/`flaky`/expected-diff-listed specs, *When* a worker runs it (`ts-node scripts/route-coverage-manifest.ts --baseline tests/e2e/baseline/next-e2e-baseline.json --expected-diff tests/e2e/baseline/bun-expected-diff.json --route-meta web-app/src/app/route-meta.ts --out /tmp/route-coverage-before.json` on `origin/main`, then the same command on this story's branch into `/tmp/route-coverage-after.json`), *Then* `diff <(jq -S '.[] | select(.covered==false) | .route' /tmp/route-coverage-before.json) <(jq -S '.[] | select(.covered==false) | .route' /tmp/route-coverage-after.json)` shows no newly-uncovered route — if it does, that route's only real coverage came from one of the 6 firefox/webkit/webkit-mobile specs and needs the same go/no-go treatment as the comparator check above.
- `web-build-gate`'s `DEFAULT_MEMBERS` is unchanged, and neither new job is ever referenced in `web-build-gate`'s `needs:` — now enforced by a durable test, not just a one-time grep.
  - *Given* `DEFAULT_MEMBERS = ["detect-changes", "web-build-smoke-next", "e2e-next"]` in `web-app/scripts/ci/web-build-gate.mjs:6`, *When* the diff for this story is complete, *Then* that line is byte-identical to before; `web-build-gate`'s `needs:` list (`build.yml:670`, currently `[detect-changes, web-build-smoke-next, web-build-smoke-bun, e2e-next, e2e-bun]`) does not include `e2e-secondary-browsers-next`/`e2e-secondary-browsers-bun`; and a new `node:test` case added to `web-app/scripts/__tests__/web-build-gate.test.mjs` in this same PR (Task 3.2.1d) fails if either new job name is ever added to `DEFAULT_MEMBERS` or to `web-build-gate`'s `needs:` in the future — the PR #1165-shaped mistake this story must not let a later PR repeat silently.
- `web-build-gate.test.mjs` still passes, including the new regression case.
  - *Given* this story adds two jobs but makes no change to `web-build-gate`'s own job block or to any pre-existing `strategy.matrix` block's `fail-fast` setting, *When* `node --test web-app/scripts/__tests__/web-build-gate.test.mjs` runs from `web-app/`, *Then* every existing test plus the new Task 3.2.1d case passes, and the pre-existing "every matrix sets `fail-fast: false`" assertion still passes trivially for the two new jobs (neither has a `strategy.matrix`, by design, to avoid needing it).
- No side-effect dependency is silently dropped from the jobs the install/run-command edits touch (pitfalls.md §1a's `ts-node` lesson).
  - *Given* the only things the removed `firefox webkit` install arguments and the added `--project` filter touch are the browser binaries and which projects run (no other side effect — confirmed by reading both steps in isolation), *When* the chromium-only shard jobs run, *Then* no other step in `e2e-next-shard`/`e2e-bun-shard` references a Firefox- or WebKit-only artifact or project name, confirmed by `grep -n "firefox\|webkit\|desktop-firefox\|desktop-webkit\|webkit-mobile" .github/workflows/build.yml` showing no remaining reference inside either shard job's steps after this story's edit.
**Rollback trigger**: the comparator check or the `route-coverage-manifest.ts` diff above shows any new `fail`-severity finding or newly-uncovered route — revert this story's `build.yml` commit; do not attempt a live patch to `next-e2e-baseline.json` under gate pressure.
**Files**: `.github/workflows/build.yml`, `web-app/scripts/__tests__/web-build-gate.test.mjs`.

##### Task 3.2.1a: Make the 20 gating shard jobs Chromium-only, including the test-run command (cost class S)
- Edit `build.yml:392` (inside `e2e-next-shard`) and `build.yml:559` (inside `e2e-bun-shard`): change `npx playwright install chromium firefox webkit --with-deps` to `npx playwright install chromium --with-deps` in both places.
- Edit `build.yml:404` and `build.yml:571` (the `npx playwright test --reporter=list,blob --shard=...` lines): add `--project=chromium --project=chromium-dom --project=mobile-chromium` to both, so these shards stop attempting to run Firefox/WebKit-dependent projects at all rather than failing on a now-missing browser executable.
- Add a one-line comment at each site: "Firefox/WebKit (desktop-firefox, desktop-webkit, webkit-mobile) moved to the dedicated e2e-secondary-browsers-next/-bun jobs below — see their own comment for why."
- Files: `.github/workflows/build.yml`.

##### Task 3.2.1b: Add the `e2e-secondary-browsers-next`/`e2e-secondary-browsers-bun` job pair (cost class M)
- Add two new jobs (not a matrix — mirroring the existing `e2e-next-shard`/`e2e-bun-shard` pattern of separate jobs per build mode), named `e2e-secondary-browsers-next` and `e2e-secondary-browsers-bun`, `runs-on: ubuntu-latest`, `needs: [detect-changes, prepare]`, same `if:` relevance guard as the shard jobs, `continue-on-error: true` at the job level, no `strategy.matrix`.
- `e2e-secondary-browsers-next` steps: checkout, Go setup, download `prepare`'s shared Next-mode artifact, `go build`, Bun/Node setup, `npx playwright install firefox webkit --with-deps`, then `npx playwright test --reporter=list,blob --project=desktop-firefox --project=desktop-webkit --project=webkit-mobile` (and, conditionally, `--project=mobile-webkit` if `PW_WEBKIT=1` is set as an env var on the job) with `WEB_BUILD: next` set explicitly (architecture-review nitpick: don't rely on the implicit unset-defaults-to-next behavior, for consistency with `test-server.ts`'s own stated "so a typo fails here" rationale for setting it explicitly elsewhere), then upload the blob report as `e2e-next-blob-secondary` with `if: always()` (matching the main shards' own always-upload pattern, since a Firefox/WebKit test failure must still produce a blob for the comparator to see it).
- `e2e-secondary-browsers-bun` is the same but builds via `make stapler-squad`'s Bun-mode seam (mirroring `e2e-bun-shard`'s own build step) and uploads as `e2e-bun-blob-secondary`.
- Add `e2e-secondary-browsers-next` to `e2e-next`'s `needs:` array (`build.yml:426`) and `e2e-secondary-browsers-bun` to `e2e-bun`'s `needs:` array (`build.yml:593`) — ordering only, so the blob-download step doesn't race ahead of the upload; neither job's existing `if:` condition changes, since both new jobs' `continue-on-error` status must never gate the merge step.
- Include a header comment (mirroring `e2e-next-shard`'s own comment block style) explaining: this job pair exists so the 20 gating shard jobs don't pay a 3x browser-install cost for coverage that's informational on all but the Chromium projects; that `webkit-mobile` (not `mobile-webkit`) is the project carrying the 6 baseline-tracked specs and must not be dropped; and that the blob artifact names are chosen to match the existing `e2e-next-blob-*`/`e2e-bun-blob-*` download globs with zero merge-step changes. Cite this plan's Story 3.2.1 and stack.md directly (one line, not a narrative — per this repo's comment-proportionality convention).
- Files: `.github/workflows/build.yml`.

##### Task 3.2.1c: Verify gate membership, baseline coverage, and run `web-build-gate.test.mjs` locally (cost class S)
- Run `node --test web-app/scripts/__tests__/web-build-gate.test.mjs` from `web-app/` and confirm all existing tests plus the new Task 3.2.1d case pass.
- Run `actionlint .github/workflows/build.yml` and fix any new findings (e.g. the new jobs' `continue-on-error`/`if:` syntax).
- Confirm `grep -n "DEFAULT_MEMBERS" web-app/scripts/ci/web-build-gate.mjs` still shows the 3-entry array unchanged, and that `web-build-gate`'s own `needs:` line names neither new job.
- Run the `route-coverage-manifest.ts` before/after diff described in the Acceptance Criteria above and confirm zero newly-uncovered routes.
- Files: none (verification only).

##### Task 3.2.1d: Add the `DEFAULT_MEMBERS`/`needs:` regression test to `web-build-gate.test.mjs` (cost class XS)
- Add one `node:test` case asserting `DEFAULT_MEMBERS` (imported from `web-build-gate.mjs`) does not contain `"e2e-secondary-browsers-next"` or `"e2e-secondary-browsers-bun"`, and a second assertion — using the same regex-extraction style the existing `"build.yml: web-build-gate has if: always()..."` test already uses to pull the job's body text — that the `web-build-gate` job's `needs:` list does not contain either new job name.
- This is the architecture-review fix for the gap where Task 3.2.1c's own check was a one-time manual grep: this test makes it durable so a future PR can't silently add either job to gate membership, the same mistake PR #1165 made.
- Files: `web-app/scripts/__tests__/web-build-gate.test.mjs`.

#### Story 3.2.2: Validate on real CI
**As a** plan owner, **I want** to confirm the chromium-only shard jobs run measurably faster per-job, the new dedicated job pair's Firefox/WebKit results stay informational and still reach the comparator, and overall time-to-verdict improves, **so that** the change is proven on real CI timing and correctness, not just locally.
**Acceptance Criteria**:
- The chromium-only shard jobs' "Install Playwright + test dependencies" step duration drops relative to baseline.
  - *Given* baseline shard `e2e-next-shard #6` (run `40184788547`) spent 45s on "Playwright-browser cache+install" as part of its 2m52s fixed overhead (architecture.md §6), *When* this story's PR's CI run completes, *Then* the equivalent step duration on any shard is measurably below 45s (installing 1 browser instead of 3), recorded via the same `gh api .../actions/jobs/<id>` step-timestamp method architecture.md §6 used.
- The slowest-shard-to-final-verdict wall-clock time improves, or at minimum does not regress, relative to the pre-Story-3.2.1 baseline, using the same measurement method Story 3.1.2 establishes.
  - *Given* Story 3.1.2 already recorded a slowest-shard-start-to-`web-build-gate`-completion delta for its own PR's run, *When* this story's PR's CI run completes, *Then* the same delta is computed for this run and compared against Story 3.1.2's number (not the original pre-#1166 baseline, since Story 3.1.1/3.1.2 may already have landed) and the comparison is recorded in the PR body — a regression here, even alongside a faster install step, means the two new secondary-browser jobs' overhead (or their insertion into `e2e-next`/`e2e-bun`'s `needs:`) costs more than the install-step savings, and that is itself a finding to report, not silently absorb.
- `web-build-gate` still passes/fails based only on `e2e-next`/`e2e-bun`'s existing semantics — an `e2e-secondary-browsers-next`/`-bun` failure does not turn the gate red.
  - *Given* both new jobs are deliberately `continue-on-error: true` and absent from `web-build-gate`'s `needs:`, *When* a Firefox or WebKit spec genuinely fails inside either job, *Then* `web-build-gate`'s own job result is unaffected (still determined solely by `detect-changes`, `web-build-smoke-next`, `web-build-smoke-bun`, `e2e-next`, `e2e-bun`), confirmed by inspecting the PR's own `web-build-gate` step summary table.
- The baseline comparator and `route-coverage-manifest.ts` checks defined in Story 3.2.1's acceptance criteria are actually run against this real CI run's output, not just locally against a simulated report.
  - *Given* Story 3.2.1's AC specifies the exact `gh run view <run-id> --log` / `route-coverage-manifest.ts` before/after procedure, *When* this story's PR's CI run completes, *Then* both checks are run against this run's real artifacts and their results (zero new `fail`-severity findings, zero newly-uncovered routes) are recorded in the PR body — this story is the first time those checks run against real CI data rather than a hypothesis.
- The comparator's `flaky`-classified finding count does not increase relative to whichever baseline is current when this PR's run completes (pre-mortem P2 item #5, same check as Story 3.1.2's).
  - *Given* the current flaky-finding count on `main` at the time this PR's CI run completes (which may already reflect Story 3.1.1/3.1.2 having merged first, per the staging order), *When* this story's PR's CI run completes, *Then* the new run's flaky-finding count is compared against that current count, and any increase is this story's go/no-go blocker.
**Rollback trigger**: the wall-clock-to-verdict delta regresses, the comparator shows a new `fail`-severity finding, or `route-coverage-manifest.ts` shows a newly-uncovered route — revert this story's `build.yml` commit.
**Files**: none (CI run + PR body text only).

##### Task 3.2.2a: Push, wait for CI, record the comparison (cost class XS — agent time; wall-clock blocker — CI time)
- Push the branch, let the full `build.yml` run complete.
- Record the install-step-duration before/after, the slowest-shard-to-verdict wall-clock delta, the gate-membership confirmation, the comparator findings, and the route-coverage diff in the PR body.
- Files: none (PR body text).

### Epic 3.3: Post-Phase-3 go/no-go checkpoint against requirements.md's Success Metric

**Goal**: Pre-mortem P1 finding: Phase 5 defers this plan's own "highest-potential-value" lever (the shared-server rearchitecture) to a doc-only artifact with no backlog item, re-review date, or checkpoint tied to whether Phase 3's incremental wins actually moved requirements.md's Success Metric — so the project could be called "done" by its own criteria while the underlying "CI feels slow" problem goes essentially unsolved. This epic is the forcing function: once Stories 3.1.2 and 3.2.2's real-CI measurements exist, explicitly compare them against the Success Metric and record a decision on Phase 5, rather than letting the seam doc sit indefinitely dormant with no trigger to revisit it.

#### Story 3.3.1: Compare the realized wall-clock-to-verdict delta against the Success Metric; record an explicit escalate/defer decision for Phase 5
**As a** plan owner, **I want** an explicit go/no-go checkpoint after Phase 3's real CI data lands, **so that** the project isn't called "done" while the suite is still slow, and Phase 5's seam work either stays a justifiably low-priority reference or gets scheduled as a real follow-up.
**Acceptance Criteria**:
- The combined post-Phase-3 wall-clock-to-verdict delta is computed from Story 3.1.2's and Story 3.2.2's own recorded measurements and compared against requirements.md's Success Metrics baseline (the post-#1166 ~25-40 min/shard range this plan's Problem Statement and Baseline both cite).
  - *Given* Story 3.1.2 recorded a slowest-shard-start-to-`web-build-gate`-completion delta for its PR's run (against baseline run `40184788547`'s own such delta), and Story 3.2.2 recorded the same delta for its PR's run (against Story 3.1.2's number, per Story 3.2.2's own AC), *When* this checkpoint runs after both PRs have merged to `main`, *Then* a worker computes `percent_improvement = (baseline_delta - final_delta) / baseline_delta` using the three real numbers already on record (baseline_delta from run `40184788547`, final_delta from Story 3.2.2's PR body), and records all three numbers plus the computed percentage in this story's finding note.
- A threshold-based verdict is recorded, not a vague "keep an eye on it": if `percent_improvement < 10%`, the verdict is **(b) insufficient — escalate**; if `percent_improvement >= 10%`, the verdict is **(a) sufficient — stays deferred**. (10% is chosen as a floor comfortably above single-run measurement noise — architecture.md §6 shows shard timings vary 2.0x-2.2x *within the same 10-way split on one run*, so a true before/after win needs to clear a bar well above what one noisy run could produce by chance; it is deliberately conservative, not tuned to produce either answer.)
  - *Given* the computed `percent_improvement` from the step above, *When* it is `< 10%`, *Then* this story's worker opens a tracked backlog item (`mcp__stapler-squad__create_backlog_item`) titled "Schedule shared-server rearchitecture (e2e-test-parallelization Phase 5 follow-up)", linking `docs/explanation/e2e-per-worker-server-seam.md` and this checkpoint's finding note, and records the backlog item's ID in the finding note; *When* it is `>= 10%`, *Then* no backlog item is opened, and the finding note instead states "Phase 5 remains a low-priority deferred reference; re-check only if `e2e-next`/`e2e-bun` wall-clock complaints recur" as the explicit decision.
- The two Phase 3 levers (`fullyParallel: true` and chromium-only shard installs) are validated together on one real `main` CI run, not only independently on their own separate PR branches (pre-mortem P2 item #3: each story validates its own diff alone; the two changes interact on shard test-distribution — a chromium-only shard now runs a different, smaller project set than the `fullyParallel`-rebalanced split was tuned against — and nothing before this checkpoint exercises both together).
  - *Given* both Story 3.1.1's and Story 3.2.1's PRs have merged to `main` (in either order, per the independent-staging decision in Step 0.5), *When* the first `build.yml` run on `main` after both merges completes (not a new PR — the regular post-merge `main` run), *Then* this checkpoint reads that run's shard-duration spread, comparator fail-count, and comparator flaky-count, and confirms none regresses relative to whichever of the two stories' own individually-validated numbers is more recent; a regression found only in this combined run (not visible in either story's own isolated validation) is recorded as a new finding in this checkpoint's note, and — because `cancel-in-progress: false` on `main` pushes means a bad run here can't be cancelled by a later push (pitfalls.md §1c) — triggers an immediate revert of whichever of the two merged commits is the more likely cause, not a wait-and-see.
- The decision (either branch) is recorded in a durable, discoverable place, not just left in an agent's own output.
  - *Given* the finding note is written, *When* this story's task completes, *Then* the decision is appended, dated, to **both** `project_plans/e2e-test-parallelization/implementation/plan.md`'s own status (a new line under this plan's header `**Status**` field, e.g. "Phase 3 shipped; post-Phase-3 checkpoint (Story 3.3.1) recorded on `<date>`: `<verdict>`") **and** to the top of `docs/explanation/e2e-per-worker-server-seam.md` as a dated note (e.g. "2026-XX-XX: Story 3.3.1 checkpoint ran; verdict: `<verdict>`; see `<finding note path>`") — so a future reader of either artifact sees the decision without having to know this checkpoint story existed.
**Files**: `project_plans/e2e-test-parallelization/decisions/post-phase3-checkpoint-finding.md` (new), `project_plans/e2e-test-parallelization/implementation/plan.md` (status line), `docs/explanation/e2e-per-worker-server-seam.md` (dated note at top).

##### Task 3.3.1a: Compute the delta, apply the threshold, record the decision (cost class S)
- After Stories 3.1.2 and 3.2.2's PRs have both merged, pull the three wall-clock numbers (baseline from run `40184788547`, Story 3.1.2's PR body, Story 3.2.2's PR body) and compute `percent_improvement` as specified above.
- Apply the 10% threshold; if insufficient, open the backlog item via `mcp__stapler-squad__create_backlog_item` and record its ID; if sufficient, record the "stays deferred" decision instead.
- Pull the first post-both-merges `main` CI run's shard-duration spread, comparator fail-count, and comparator flaky-count; compare against each story's own isolated-validation numbers per the combined-interaction acceptance criterion above; record the comparison (or an immediate-revert action, if a regression is found) in the same finding note.
- Write the finding note, then append the dated decision line to `plan.md`'s Status field and to the top of `docs/explanation/e2e-per-worker-server-seam.md` (this task necessarily runs after Phase 5's Story 5.1.1 has created that file).
- Files: `project_plans/e2e-test-parallelization/decisions/post-phase3-checkpoint-finding.md`, `project_plans/e2e-test-parallelization/implementation/plan.md`, `docs/explanation/e2e-per-worker-server-seam.md`.

---

## Phase 4: Bounded Test-Content Triage

### Epic 4.1: Sample-triage methodology for lighter-tier conversion candidates

**Goal**: Per the owner's explicit 2026-10-25 decision in requirements.md's Open Questions ("scope a bounded sample triage as an explicit story in this round's plan, not deferred... and not an exhaustive 193-spec retriage"), identify a reasoned, bounded sample of the 193 e2e specs that might not need a real backend+browser, producing a reviewed candidate list — not a mass conversion.

#### Story 4.1.1: Define methodology, execute the bounded sample, produce a reviewed candidate list
**As a** plan owner, **I want** a defensible, bounded sample-triage methodology and its results, **so that** the suite's content gets re-judged without either rubber-stamping everything or reading all 193 files (requirements.md's own named failure modes).
**Acceptance Criteria**:
- The sample-selection methodology is concrete and stated before triage begins: **stratify by Playwright project assignment and file size, then sample 20 files (≈10% of 193)** — specifically: all files already flagged by architecture.md §5's bounded grep as low-risk-to-move candidates (the 54 files using only file-scoped `beforeAll`/`afterAll`, 28% of 193, the single largest "might not need true cross-worker state" category) get priority; from that 54-file pool, select the 20 largest-by-line-count files (a proxy for "most assertion surface to re-examine," per this repo's own hotspot-analysis convention of using size/complexity as a triage signal) rather than a uniform random sample, since a larger file has more to gain from conversion if it qualifies and more to lose if it doesn't.
  - *Given* architecture.md §5's grep found 54 files in the "file-scoped `beforeAll`/`afterAll`" category, *When* a worker runs `git show origin/main:tests/e2e/<each-of-the-54-files> | wc -l` (or an equivalent single `wc -l` pass) and sorts descending, *Then* the top 20 by line count form the sample, and this selection is written down in the methodology note before any file is read for triage content (so the sample isn't adjusted after seeing results).
- **Methodology caveat, stated explicitly rather than implied**: this sample draws exclusively from the 54-file pool and is biased toward its largest files. A large file in that pool plausibly correlates with *heavier* backend/session setup (more reason the file-scoped `beforeAll`/`afterAll` exists at all), which may anti-correlate with "doesn't need a real backend" — the opposite of this methodology's own "more to gain, more to lose" rationale. The sample intentionally includes no file from outside the 54-pool as a counterexample/sanity check.
  - *Given* this competing hypothesis is not resolved by the sample design, *When* the finding note (Task 4.1.1a) is written, *Then* it states this caveat in its own sentence alongside the methodology, so a reviewer of the candidate list sees the limitation rather than inferring "largest-first" was the only consideration.
- Each of the 20 sampled files gets a per-file verdict: "full e2e justified" (with the one-line reason: real backend/browser interaction the assertions depend on) or "lighter-tier candidate" (with the specific lighter tier proposed — e.g. a Jest/RTL component test, a pure-function unit test — and what coverage would need to be preserved).
  - *Given* a sampled file like (placeholder — actual filenames are determined by the line-count sort above, not fixed in this plan) is read in full, *When* the triage agent classifies it, *Then* the verdict table has exactly 20 rows, each with a file path, line count, verdict, and one-line justification — no row left blank or deferred.
- The output is a reviewed list, not an applied conversion: no spec file is actually moved/rewritten as part of this story.
  - *Given* the Acceptance Criteria above produce a verdict table, *When* this story's task completes, *Then* `git diff --stat -- tests/e2e/` shows zero changes to any `*.spec.ts` file — the candidate list is the deliverable, any actual conversion is explicitly out of this round's scope (a future story, contingent on Tyler reviewing the list).
**Files**: `project_plans/e2e-test-parallelization/decisions/test-content-triage-sample.md` (new).

##### Task 4.1.1a: Select the sample and write the methodology section (cost class S)
- Re-derive architecture.md §5's 54-file "file-scoped `beforeAll`/`afterAll`" list against the current `origin/main` tree (the grep may have shifted slightly since 2026-10-25; re-run it, don't trust the cached count blindly).
- Sort by line count descending, take the top 20, and write the methodology + file list + the largest-file-selection-bias caveat (above) into the finding note before reading any file's content.
- Files: `project_plans/e2e-test-parallelization/decisions/test-content-triage-sample.md`.

##### Task 4.1.1b: Read and classify the 20 sampled files (cost class M)
- Read each of the 20 files in full; classify per the Acceptance Criteria's verdict schema.
- Append the 20-row verdict table to the finding note.
- **Sizing note (triad-review Engineering-lens gap)**: this task reads 20 files, exceeding this plan's usual 3-5-file-per-task norm elsewhere. Not split into sub-batches deliberately: the norm exists to bound blast radius and review burden for *code-editing* tasks; this is a read-only classification task writing to one shared finding-note table, where splitting into sub-batches would (a) lose the ability to compare candidates against each other for a consistent verdict standard across the sample, and (b) multiply context-switch cost for zero blast-radius benefit, since no code changes result from this task either way (confirmed by this story's own "zero `*.spec.ts` changes" acceptance criterion).
- Files: `project_plans/e2e-test-parallelization/decisions/test-content-triage-sample.md`.

##### Task 4.1.1c: Summarize and flag for owner review (cost class XS)
- Write a short (3-5 sentence) summary at the top of the finding note: how many of the 20 were "lighter-tier candidate" vs. "full e2e justified," and a one-line recommendation on whether a follow-up conversion story is worth opening.
- Do not open that follow-up story in this plan — it's contingent on Tyler reviewing the list, per the Acceptance Criteria above.
- **Delivery mechanism (pre-mortem P2 item #1 fix)**: a prose summary alone has no forcing function to ever get acted on. If ≥1 file is classified "lighter-tier candidate," open a tracked backlog item via `mcp__stapler-squad__create_backlog_item` titled "Review e2e test-content triage candidates (e2e-test-parallelization Phase 4 follow-up)", linking this finding note and listing the candidate count, and record the backlog item's ID in the finding note. If zero files qualify, record that explicitly instead ("no candidates found — no backlog item opened") rather than silently omitting the step.
- Files: `project_plans/e2e-test-parallelization/decisions/test-content-triage-sample.md`.

---

## Phase 5: Deferred Design-Only — N-Server Worker-Scoped-Fixture Seam

**This phase is design documentation only. No code in `tests/e2e/`, `.github/workflows/`, or anywhere else is written or changed in this round. It exists so a future project doesn't re-derive architecture.md §1-§4 and build-vs-buy.md §3-4 from scratch.**

**This phase does not decide its own priority.** Story 3.3.1 (Phase 3) is the forcing function: it runs after Phase 3's real CI data lands, compares the measured wall-clock win against requirements.md's Success Metric, and records an explicit escalate-or-defer decision — including a dated note appended to the top of this phase's own doc (Task 3.3.1a). Until that checkpoint runs, this phase's doc is a reference, not a commitment either way.

### Epic 5.1: Document the extension point

#### Story 5.1.1: Write the seam design doc
**As a** future implementer of true in-process multi-worker e2e parallelism, **I want** a written design for the worker-scoped-fixture seam, grounded in this round's research, **so that** I start from a known, bounded design rather than re-reading the whole research corpus.
**Acceptance Criteria**:
- A doc exists describing: the worker-scoped-fixture mechanism (`test.extend<TestFixtures, WorkerFixtures>({ scope: 'worker' })` overriding the built-in `baseURL` fixture), the two known call sites that would need to change (`connection-count-indicator.spec.ts:39`, `launcher-presets.spec.ts:21`, both reading `process.env.TEST_SERVER_TESTDIR` directly), the "separate config file, don't branch the existing one" precedent from `isolated-dev-stacks`, and the three unverified risk categories from pitfalls.md §4 (port-allocation TOCTOU, CPU/RAM resource contention, incomplete tmux/instance isolation) stated as open risks a future implementer must validate, not as solved problems.
  - *Given* architecture.md §2 names the worker-scoped-fixture mechanism and §1.2 names the two call sites, *When* the doc is written, *Then* it quotes or directly links both (not paraphrases from memory), and explicitly states "Isolate via seam — not implemented this round" as its status line, matching ADR-001.
- The doc is cross-referenced from the repo's documentation index.
  - *Given* `CLAUDE.md`'s "Reference Documents Index" table lists every `docs/` doc with a one-line description, *When* this story's task completes, *Then* a new row exists for this doc, following the existing table's format exactly (topic | file path).
**Files**: `docs/explanation/e2e-per-worker-server-seam.md` (new), `/home/tstapler/Programming/stapler-squad/CLAUDE.md` (Reference Documents Index table row).

##### Task 5.1.1a: Write the seam design doc (cost class M)
- Write `docs/explanation/e2e-per-worker-server-seam.md` covering: (1) why `TestServer`'s internals (port/tmux-socket/testDir/binDir) are already per-instance-safe (architecture.md §1 table) but the lifecycle/discovery layer (`global-setup.ts`'s single call + unindexed `process.env`) is not; (2) the concrete worker-scoped-fixture shape (code sketch, not implementation) overriding `baseURL`; (3) the two known call sites needing a parallel `testServerDir` fixture; (4) the "separate config, don't branch `playwright.config.ts`" rationale, citing `isolated-dev-stacks/research/architecture.md` §3-4 directly; (5) the three design-against risk categories from pitfalls.md §4, stated as open, each with "how it manifests if missed" preserved verbatim since that framing is the useful part; (6) an explicit "not implemented, see ADR-001" status line.
- Files: `docs/explanation/e2e-per-worker-server-seam.md`.

##### Task 5.1.1b: Add the Reference Documents Index row (cost class XS)
- Add one row to `CLAUDE.md`'s "Reference Documents Index" table: `| Per-worker e2e backend server seam (deferred N-server design) | docs/explanation/e2e-per-worker-server-seam.md |`.
- Files: `/home/tstapler/Programming/stapler-squad/CLAUDE.md`.

---

## Effort Estimate

Per `~/.claude/skills/sdd/skills/ESTIMATION.md`: cost classes and CU, not human time. Bands are INFERRED; recalibrate after the first real implementer run.

| Story | Tasks | Cost class | Est. raw tokens | Assumed output share | Est. CU (≈) | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 1.1.1 | 1.1.1a | XS | ~10k | 5% | ~10k | none |
| 2.1.1 | 2.1.1a | XS | ~15k | 10% | ~16k | none (API call is near-instant) |
| 2.2.1 | 2.2.1a | S | ~40k | 15% | ~46k | none (local timing runs, seconds each) |
| 3.1.1 | 3.1.1a, 3.1.1b | XS | ~20k | 10% | ~21k | none |
| 3.1.2 | 3.1.2a | S (agent) — grew from XS to cover the comparator check + 54-file shard-split review added for adversarial Blocker 1 | ~35k | 10% | ~37k | **Yes — one full `build.yml` CI cycle (~25-40 min) to produce the before/after shard timings, the comparator findings, and the shard-split table** |
| 3.2.1 | 3.2.1a, 3.2.1b, 3.2.1c, 3.2.1d | M (sum of S+M+S+XS) — grew from 3 to 4 tasks and from a single job to a job pair for adversarial Blocker 2 + architecture Concern 2 | ~130k | 15% | ~150k | none |
| 3.2.2 | 3.2.2a | S (agent) — grew from XS to cover the wall-clock-to-verdict, comparator, and route-coverage checks added for the same blocker/concerns | ~30k | 10% | ~32k | **Yes — one full `build.yml` CI cycle to confirm install-step duration, time-to-verdict, gate membership, and baseline/route coverage** |
| 4.1.1 | 4.1.1a, 4.1.1b, 4.1.1c | M (sum of S+M+XS) | ~140k | 10% | ~151k | none (reading 20 files + classifying, no CI needed) |
| 5.1.1 | 5.1.1a, 5.1.1b | S (sum of M+XS) | ~70k | 15% | ~81k | none |
| 3.3.1 | 3.3.1a | S | ~42k | 10% | ~45k | **Yes — gated on Story 3.1.2's and Story 3.2.2's PRs both being merged (not just CI-green), plus the first post-merge `main` run existing to read the combined-interaction numbers from (triad-review/pre-mortem P2 item #3 addition) — no new CI run of its own is triggered by this story, it reads one that already happened** |

**Sum of task estimates**: ~532k raw tokens / ~589k CU (revised after the triad-review follow-up pass adding the flaky-delta checks, the combined-CI-run interaction check, and the Phase-4 backlog-item delivery mechanism; was ~525k/~581k).
**Verification multiplier (×1.5-2 per ESTIMATION.md, for spec-compliance/code-quality review and `sdd:6-verify`)**: ~886k-1.18M CU.
**Resulting size band**: **Medium** (≤1.1M CU at the low end, now crossing slightly past it at the high end), per the Scope-default table — still a config/doc-level project with no new application code, but the comparator/route-coverage/regression-test additions plus this checkpoint push it past the **Small** threshold used in the pre-repair estimate.

**Critical path in agent waves**:
1. Wave 1: Task 1.1.1a (branch setup) — blocks everything else.
2. Wave 2 (parallel): Story 2.1.1, Story 2.2.1 (spikes, independent of each other and of Phase 3/4), Story 3.1.1's tasks, Story 3.2.1's tasks, Story 4.1.1's tasks, Story 5.1.1's tasks — all five tracks can fan out concurrently once Wave 1 completes, since none share a file.
3. Wave 3 (serialized on CI, not tokens): Story 3.1.2's CI cycle, then Story 3.2.2's CI cycle — these are the only two stories with a real wall-clock dependency, and they can run as two separate pushes (either sequentially or concurrently as two separate PRs, since neither PR's diff overlaps the other's).
4. Wave 4: Story 3.3.1 — runs only after both Wave 3 PRs are merged to `main` (not just CI-green on their own branches) and after Story 5.1.1 has landed its doc (3.3.1 appends a dated note to it).

**Overrun checkpoint**: if total spend across all 10 stories exceeds **~1.48M CU** (25% over the ~1.18M high-end estimate), pause and report the delta with cause — informational, not a scope cut.

**Wall-clock blockers** (the only calendar-time items in this plan):
- Story 3.1.2: one full `build.yml` CI run (~25-40 min observed baseline) to prove the `fullyParallel: true` shard-rebalancing win, the comparator shows zero new fail-severity findings, and no file from the 54-file pool split unsafely across shards. Owner: whoever merges/reviews that PR (Tyler).
- Story 3.2.2: one full `build.yml` CI run to prove the browser-install-isolation win, confirm gate-membership is unaffected, confirm the comparator/route-coverage checks are clean, and confirm wall-clock-to-verdict didn't regress. Owner: Tyler.
- Story 3.3.1: not a new CI run, but gated on Tyler actually merging both Phase 3 PRs first — a calendar dependency on review bandwidth, not agent tokens.
- Story 2.1.1's finding may itself surface a wall-clock blocker (if the account is on a tier with less headroom than assumed) — but that would only matter for a future shard-count-increase proposal, which is explicitly out of this round's implementation scope.

**Basis note**: bands, weights, and the output-share assumptions are INFERRED per ESTIMATION.md; recalibrate against the first real implementer-agent run's `subagent_tokens`.
