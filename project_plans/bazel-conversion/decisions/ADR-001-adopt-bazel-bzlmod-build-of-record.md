# ADR-001: Adopt Bazel (bzlmod) as the build of record

**Date**: 2026-11-01
**Status**: Accepted (owner decision, requirements.md 2026-11-01)
**Supersedes**: `project_plans/ci-speed/decisions/ADR-001-reject-bazel-bzlmod-adoption.md`
**Context**: bazel-conversion. Drivers (co-equal): CI speed via affected-target selection, hermeticity, worktree cost.

## Decision

Bazel 9.x with bzlmod only (no WORKSPACE) becomes the build of record for Go, proto, ent, tmux/tymuxd and the web-app. `make` targets become thin wrappers. Migration is a strangler: non-blocking CI first, parity gate P1-P10, then cutover, `BUILD_SYSTEM=make` escape for one release cycle, install-service last. Rollback is one PR until the make recipes are removed (Epic 10).

## The ci-speed ADR-001 rejection reasons, point by point

| ci-speed ADR-001 reason | Status now | Response in this plan |
|---|---|---|
| 1. Migration surface vs a 3-6 week appetite; ent and buf output need custom wiring; tmux needs re-wiring | **Appetite cap superseded** (requirements.md Size: XL, full scope, owner-comfortable). **Technical claim still true** and is the reason Epic 0 exists. | Epic 0 turns each hard item (ent, proto managed mode, embed tree artifacts, tmux, bun.lock) into a pass/fail spike with a decided fallback; Gazelle owns every Go BUILD file so 3,700 files are not hand-written. |
| 2. Free runners remove remote *execution*; remote cache is a weak win and a new dependency | **Still holds for RBE; plan assumes none.** The cache is no longer the only driver: hermeticity and worktree cost are won by pinned toolchains and a shared local `--disk_cache`, with no remote service. | Remote cache = self-hosted `bazel-remote` on `onyx` (ADR-004), server-enforced read-only for PRs, graceful fallback if unreachable. CI speed claims are measured against Epic 0 baselines, not assumed. |
| 3. No Gazelle equivalent for the pnpm/Next half; split build system likely | **Still the largest technical risk, but the premise changed**: the web build is now Bun (`bun build/build.ts`), not Next/pnpm, and aspect_rules_js cannot read `bun.lock`. | ADR-003: wrap Bun install+build in a repo-owned repository rule and one `dist/` action (one lockfile, no per-package Gazelle needed). A fork of aspect_rules_js is costed as the alternative in the Epic 0 spike (ADR-008). Result is one Bazel graph, not a split system. |
| 4. Scar tissue: `b51b60eb1` removed a WORKSPACE-mode rules_foreign_cc path when Bazel 9 broke it | **Root cause does not apply** (WORKSPACE was the failure; this plan is bzlmod-only) but **version-churn risk remains**. | Pin every module and Bazel (`.bazelversion` 9.3.1, `MODULE.bazel.lock` committed unless spike S13 (Task 0.10d) decides otherwise with owner approval, `--lockfile_mode=error` in CI); scheduled upgrade-canary job; `bazel-version-upgrades` skill; accepted risk recorded here. Make fallback is kept and canary-tested so it cannot rot (Epic 8.5). |
| 5. Cheaper alternatives (build-once/fan-out) capture most of the win | **Delivered by ci-speed and insufficient for drivers 2 and 3**: fan-out does not give a hermetic fresh-clone build or cheaper Nth worktree, and the three hand-maintained selection mechanisms remain (a `grep -qE` regex in `build.yml`, `tools/ci/detect-feature-changes.sh`, and native `paths:` filters in `mcp-integration.yml`) a "two sources of truth" risk. | Affected-target selection (rdeps + global-inputs list) retires all three mechanisms; verified against 30 historical PRs before deletion (Story 7.3). |

Also from ADR-001 "Consequences": agents must reason in Bazel. Addressed by Epic 11 (project skill, path-scoped rule, CLAUDE.md section, Diataxis docs, `make` wrappers as the primary agent interface).

## Measured pain (why now)

The pain numbers are recorded by Task 0.1f in the "Measured pain" section of `project_plans/bazel-conversion/implementation/baselines.md` (CI p50/p95 time-to-green, cold worktree build time, disk per worktree, manual regen-codegen steps per agent session, warm one-package test latency, first-error legibility under make). Until that task runs, no pain number is asserted here. Improvement targets T1-T7 are derived from those numbers at Task 0.1g (`tools/bzl/baseline/targets.yaml`) and enforced as P10 improvement rows; the economic abort criterion (Task 0.14e) is evaluated at Story 0.14.

## Priority rationale (RICE; every input UNVERIFIED until Task 0.1g)

*Pointer (2026-11-02, not a rescore)*: the measured RA2 of 0.987 (e2e is most of the critical path) is why ADR-009 and Epic 12 add selective e2e; the scores below are unchanged and are re-scored at Task 0.1g.

RICE = Reach x Impact x Confidence / Effort. Reach is agent sessions per week that build or test (estimate, scale 100 = this repo's current order of magnitude), Impact uses the 0.25/0.5/1/2/3 scale, Confidence is a probability, Effort is the verified CU total in millions from the plan's Effort estimate (Effort for Go-only is Epics 0, 1, 2 and 4 with the same x1.75 multiplier).

| Option | Reach | Impact | Confidence | Effort (M CU) | RICE | Note |
|---|---|---|---|---|---|---|
| Full Bazel (this plan) | 100 | 2 | 0.5 | 28.8 | 3.5 | covers all three drivers incl. hermeticity |
| Go-only Bazel, web native | 100 | 1 | 0.6 | 16.2 | 3.7 | no web hermeticity, split build system |
| Status quo + ci-speed fixes | 100 | 0.25 | 0.9 | 0.5 | 45.0 | already delivered, misses drivers 2 and 3 |

Reading: RICE alone does not favour the full conversion, because it weights Effort heavily and cannot price hermeticity or worktree cost. The owner chose the full end state (requirements.md), and RICE is recorded so the choice is explicit rather than implied. The safeguards are the Epic 0 spike gate, the economic abort criterion at Story 0.14 (decision gate), the improvement targets, and the epic-end exit rule; Task 0.1g re-scores this table with measured Reach and Impact.

## Consequences

- One mental model after cutover; two during the strangler period (mitigated by wrappers).
- Permanent exceptions stay outside Bazel (ADR-005).
- Bazel-only source changes are banned until the retention window ends, so revert stays a single PR.
- ci-speed ADR-001 gets a "Superseded by bazel-conversion ADR-001" header line (Task 0.14c).

## Alternatives considered

- **Bazel as a thin cache wrapper around `make`/`go build`** (genrule per make target): cheap, but no target graph, so no affected-target selection and no hermeticity gain. Rejected.
- **Go-only Bazel, web native**: kept as a sequencing stage (Epic 1), not an end state (owner chose full end state).
- **Nix / Pants / Buck2 / Turborepo-Nx**: scored in `research/build-vs-buy.md` section 5; Nix loses on incremental Go and affected targets, the others have smaller Go ecosystems or cover only JS.
