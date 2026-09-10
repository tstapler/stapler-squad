# Requirements: ci-speed

**Date**: 2026-08-27
**Type**: cross-cutting infrastructure change
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement
`tstapler/stapler-squad`'s GitHub Actions CI is slow enough to hurt dev velocity (PRs sit yellow/red too long before merge feedback), and there's no systematic driver analysis of why — just accumulated per-workflow ad hoc timeouts and partial caching. The user wants to understand the actual drivers of the slowness and build a long-term, systematic plan to reduce it, not a one-off tweak. **Reconciled during Phase 2 research (2026-08-28)**: the initial framing also cited dollar cost (Actions minutes usage) as a driver, but `origin` is a public repo, so standard `ubuntu-latest` GitHub-hosted runner minutes are unbilled — this is a wall-clock/dev-velocity problem only, not a billing problem (confirmed directly with the user: see the "Both equally" → resolved-to-wall-clock-only framing in the Phase 1 ideation interview).

## Baseline
Measured via `gh run list`/`gh run view` against the last ~15 completed runs per workflow (2026-08-27, wall-clock per run, all workflows run on `ubuntu-latest`, no macOS/Windows multiplier in play):

| Workflow | Avg wall-clock | Max observed |
|---|---|---|
| `build.yml` | ~18m | 81.5m |
| `demo-publish.yml` | ~136m | 202m |
| `mcp-integration.yml` | ~13m | 46.8m |
| `e2e-video.yml` | ~12m | 45.9m |
| `lint.yml` | ~12m | 31.5m |
| `benchmark.yml` | ~9m | 18.2m |
| `ux-analysis.yml` | ~7m | 8.3m |
| `generated-proto-guard.yml` | ~5m | 35.6m |
| `backlog-scaffolding-guard.yml` | ~4m | 10.1m |
| `goreleaser-check.yml` | ~4m | 4.9m |
| `registry-validation.yml` | ~3m | 7.4m |

Notable structural facts already gathered:
- All workflows already use `actions/cache` / `setup-go`+`cache: true` / `setup-node`+`cache: 'pnpm'` — caching is not simply *absent*, but its effectiveness (hit rate, scope, invalidation) is unverified.
- `build.yml` runs ~9 separate jobs, most independently calling `setup-go`+`cache: true` and downloading/building from scratch — likely redundant work across jobs rather than one shared build reused downstream.
- The gap between average (~18m) and max (81.5m) on `build.yml`, and (~13m avg / 47m max) on `mcp-integration.yml`, points at tail-latency causes (cache misses, flaky retries, contention/queueing) as a distinct problem from median duration.
- `demo-publish.yml`'s ~136m average wall-clock is a clear outlier vs. every other workflow and needs root-cause investigation (may be dominated by non-billable wait/approval time rather than compute — unconfirmed).
- The `ci` Make target (`make ci`) already composes build → test → test-race → vet → lint → lint-css-tokens → test-integration → fmt-check → registry-generate → guards, run both locally and (presumably) mirrored into CI jobs — worth checking for 1:1 duplication between local and CI sequencing.
- **Prior Bazel context**: Bazel was previously introduced only narrowly, as an optional alternate build path for the bundled tmux native dependency via `rules_foreign_cc` (never used for the Go/web-app build graph, never exercised by CI — `build.yml` always called `scripts/build-tmux.sh` directly). It was removed in commit `b51b60eb1` (2026-07-15) because `rules_foreign_cc`'s WORKSPACE-mode setup is permanently broken under Bazel 9 (WORKSPACE support was dropped in the 9.0 LTS bzlmod migration). This is a negative signal about that narrow integration, not a verdict on whether a full bzlmod-based Bazel (or another build-graph/caching tool) adoption for the whole Go+TS build is worthwhile — that question is open and explicitly in scope for research.

## Users / Consumers
- Tyler (repo owner/primary committer) and any other human contributors opening PRs against `tstapler/stapler-squad`.
- Automated backlog/session-worker agents (stapler-squad's own AI session automation) that open PRs and wait on CI to iterate — CI latency directly throttles that automation's cycle time too.

## Success Metrics
- Cut **typical per-PR wall-clock time-to-green by ~50%** (not just a single workflow's duration — the end-to-end time from PR push to all required checks passing).
- Tame worst-case tail latency as a secondary signal (e.g. `build.yml`'s 81.5m max vs 18m avg, `mcp-integration.yml`'s 47m max vs 13m avg) — a regression here should be visible even if median improves.
- A CI-duration budget/gate exists after this ships so a newly added slow job or dependency creep is caught automatically rather than silently regressing the win.

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
- **Stay on free GitHub-hosted runners** (`ubuntu-latest` or other free-tier hosted runner types) — self-hosted runners and paid larger-runner tiers are explicitly out of scope for this effort.
- No hard calendar deadline; bounded by the Large appetite window instead.
- Must not break "green means mergeable" — required branch-protection status checks must keep working through any workflow/job restructuring. **Verified during Phase 2 research (2026-08-28)**: `main` currently has zero branch protection rules/rulesets configured (`gh api repos/tstapler/stapler-squad/branches/main/protection` → 404). This constraint is therefore forward-looking — job/check names should be kept stable and intentional so that turning on branch protection later (a separate repo-owner decision, not part of this plan) is a settings change, not a re-plan — rather than a live guardrail this plan risks breaking today.

## Non-functional Requirements
- **Performance SLO**: ~50% reduction in typical per-PR time-to-green (see Success Metrics).
- **Scalability**: not applicable — CI load scales with PR/commit volume, not a target to design for independently.
- **Security classification**: internal engineering tooling; no regulated data involved.
- **Data residency**: not applicable.

## Scope
### In Scope
- All workflows in `.github/workflows/` are fair game for restructuring, including required-check workflows (build, lint, test, e2e, mcp-integration, registry/proto guards) and lower-frequency ones (release-please, release, demo-publish, deploy-pages) — nothing is pre-emptively excluded.
- Root-cause analysis of current slowness drivers: cache effectiveness/hit-rate, job/matrix duplication, sequential-vs-parallel job graph structure, unnecessary re-builds, tail-latency causes (flaky retries, queueing, cache misses).
- Evaluating build-graph/caching tooling changes (including a fresh look at Bazel with bzlmod, or lighter alternatives like a Go build cache action, Turborepo/Nx-style task caching for the web-app, or a shared inter-job artifact/cache strategy) as a genuine option, not ruled out by the prior narrow/abandoned integration.
- Path-based/conditional workflow triggering (skip workflows unaffected by a given diff), concurrency/cancellation tuning, and matrix reduction where jobs are redundant.
- Designing and implementing a long-term CI-duration budget/gate so regressions are caught automatically going forward.

### Out of Scope
- Self-hosted runners or paid larger GitHub-hosted runner tiers.
- Reducing test *coverage* or *quality* as a shortcut to reduce CI time — this is about eliminating waste and improving parallelism/caching, not skipping validation.
- Non-GitHub-Actions CI systems (e.g. migrating off GitHub Actions entirely).

## Rabbit Holes
- **Full Bazel/bzlmod adoption** is exactly the kind of thing that sounds tractable but can spiral into a multi-week migration on its own (build file authoring for every Go package + the web-app, remote-cache infra decisions, CI runner compatibility) — Phase 2 research must scope this as an *option to evaluate*, with a clear go/no-go recommendation, not something Phase 5 starts implementing speculatively.
- **`demo-publish.yml`'s 136m average** needs root-causing before assuming it's a compute problem — it could be waiting on manual approval/environment gates, an external service, or an artifact upload bottleneck, each of which has a completely different fix.
- **Job/matrix consolidation in `build.yml`** risks silently changing which check names branch protection depends on — renaming or merging jobs needs care to keep required-status-check names intact (or a coordinated branch-protection settings update in the same change).
- **Cache invalidation correctness** — a cache-hit-rate fix that's too aggressive (over-broad cache keys) can produce stale-artifact bugs that are worse than the CI time it saves.

## Alternatives Considered
- **Self-hosted/larger runners** — explicitly rejected by the user for this effort (see Constraints); would trade money/infra ops burden for speed rather than eliminating waste.
- **Bazel (bzlmod)** — open question for Phase 2 research; prior narrow WORKSPACE-mode integration for tmux was abandoned as permanently broken under Bazel 9, but a full bzlmod-based adoption for the Go+web-app build graph is unevaluated.
- **Lighter build-cache tooling** (e.g. a dedicated Go build-cache GitHub Action, Turborepo/Nx for the web-app's pnpm workspace, `sccache`) — to be compared against Bazel in research as lower-risk alternatives that solve the same caching/incrementality problem with less migration cost.

## Feasibility Risks
- Root-causing tail latency (cache misses vs. queueing vs. flakiness) may require deeper GitHub Actions API access than currently available (billing/usage API calls hit secondary rate limits during this session and require an elevated `gh auth` scope) — Phase 2 research should establish what access it actually has before committing to a specific measurement plan.
- Any workflow-level restructuring (job splits/merges, trigger changes) is inherently a change to a shared, load-bearing system — a bad interaction with branch protection or a currently-passing-by-luck flaky job could block all merges until fixed.

## Observability Requirements
Ongoing CI-duration tracking so regressions are visible without manual archaeology: at minimum, a way to see per-workflow/per-job duration trends over time (e.g. a scheduled job querying the Actions API and recording history, or equivalent), and a CI-duration budget/gate (a workflow step or check) that fails when a job or overall run exceeds a defined time budget — the specific mechanism is a Phase 2/3 design decision.

## Risk Control
Given the Large appetite and cross-cutting blast radius: land changes incrementally (workflow-by-workflow or job-by-job) behind normal PR review rather than one big-bang rewrite of `.github/workflows/`, so a regression in one workflow doesn't block all CI. Any required-status-check name changes must be paired with a branch-protection settings update in the same change, verified before merge. No feature-flag equivalent applies to CI config itself, but each change should be independently revertible (small, focused commits/PRs per workflow).

## Open Questions
- ~~What is the actual current monthly Actions minutes usage/cost?~~ — **Resolved, moot**: `origin` is a public repo, so standard `ubuntu-latest` runner minutes are unbilled entirely (no billing API access needed) — see the Problem Statement's reconciliation note above.
- Is `demo-publish.yml`'s ~136m average wall-clock dominated by compute time or by non-compute wait (approvals, external service, upload)?
- What is actual cache hit-rate today across `actions/cache`/`setup-go`/`setup-node` usages — is the existing caching actually effective, or silently missing on most runs?
- Does the free-tier 20-concurrent-job limit ever cause queueing delays on this repo (multiple workflows firing per PR/push), independent of any single workflow's own duration?
- Is a full Bazel/bzlmod adoption worth its migration cost here, or does a lighter build-cache/task-runner tool capture most of the win for far less effort?
