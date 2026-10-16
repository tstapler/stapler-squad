# Requirements: Gate GitHub Actions PR Comments on Actionable Findings

Source: backlog item `2375ca2e-2155-4165-a38a-214f1fd80e39`. Generated directly from the
backlog item's title, description, and acceptance criteria — no interactive ideation
interview was run (no user present in this session).

**Scope note:** This is distinct from `project_plans/pr-comment-check-runs/`, which covers
the app's own agent/session-driven PR commenting (`server/services/github_service.go`,
backlog shepherding skills). This item is scoped entirely to the five `.github/workflows/*.yml`
files that post sticky PR comments via `actions/github-script`. The backlog item's own
description notes an earlier item mistakenly conflated these two mechanisms and has since
been corrected to this narrower CI-workflow scope. Do not merge or cross-reference the two
project plans as if they were the same effort.

## Problem Statement

Four of five GitHub Actions workflows that post PR comments (`benchmark.yml`, `ux-analysis.yml`,
`build.yml`, `e2e-video.yml`) post/update an advisory comment **unconditionally on every run**,
regardless of whether there's anything the user needs to act on. `registry-validation.yml`
already implements the desired pattern: it only posts when there's real divergence or low
coverage. The other four should adopt the same gate.

## Context Already Established (grounded in the actual workflow files — don't re-derive)

- **`registry-validation.yml:90-95`** — the reference pattern already in the repo:
  ```js
  const hasDivergence = output.includes('Added') || output.includes('Removed') || exitCode !== '0';
  const hasLowCoverage = parseFloat(pct) < 50;
  if (!hasDivergence && !hasLowCoverage) { return; }
  ```
  The job itself also fails when divergence exceeds 2% (`registry-validation.yml:47-58`) —
  that blocking check is correct today and out of scope to change.

- **`benchmark.yml`** — three near-identical jobs, each posting unconditionally via a
  sticky marker-comment pattern (`updateComment`/`createComment` keyed on an HTML marker):
  - go-tier1: marker `<!-- benchmark-go-tier1 -->`, comment step at lines 108-135
  - frontend-throughput: marker `<!-- benchmark-frontend-throughput -->`, lines 334-360
  - e2e-latency: marker `<!-- benchmark-e2e-latency -->`, lines 548-575
  None of the three check benstat output for an actual regression before posting.
  `build.yml:266-268` (comment) confirms these are explicitly documented as "advisory...
  never blocked."

- **`ux-analysis.yml:149-203`** — posts unconditionally (`if: always() && github.event_name
  == 'pull_request'`, line 150) a status table combining three signals:
  - Axe: already blocks via job failure (`continue-on-error: false`, line 93) — a green
    Axe result needs no comment.
  - Lighthouse: advisory, warns if score < 70 (`continue-on-error: true`, line 109;
    threshold check at line 165, `score < 70`).
  - A static "Claude UX Analysis: Advisory" pointer row — never itself an action item.
  Marker: `<!-- ux-analysis -->` (line 156).

- **`build.yml:216-247`** — posts a feature-coverage summary unconditionally on every PR
  (marker `<!-- feature-coverage -->`, line 223), not gated on coverage changing or
  dropping. "Check new RPCs have tests" is a separate, already-blocking job step
  (`build.yml:249-262`) — out of scope, already correct.

- **`e2e-video.yml`**'s `notify` job (lines 243-319, gated only on
  `needs.detect-feature-changes.outputs.record_features == 'true'` at line 243) posts
  unconditionally whenever video-recordable features changed — pure navigation aid
  (links to video/GIF artifacts), never itself an action item. Marker: `<!-- e2e-videos -->`
  (line 249).

- All five comment steps use `continue-on-error: true` on the comment-posting step itself
  (advisory; never fails the job if posting is denied) and the sticky
  find-by-marker → `updateComment`/`createComment` pattern — this shared shape is what
  makes `registry-validation.yml`'s gate copyable rather than a one-off rewrite.

## Kano Classification

**Basic expectation**, not a delighter — users don't credit "CI didn't spam me" as a bonus,
they only notice and lose trust when it's violated. Treat as a defect fix.

## RICE-style Signal (qualitative)

| Dimension | Rating | Justification |
|---|---|---|
| Reach | High | `build.yml` and `e2e-video.yml` trigger broadly; `benchmark`/`ux-analysis` are path-gated but still frequent on active development. |
| Impact | High | Directly matches the owner's stated PR-comment-noise complaint. |
| Confidence | High | Root cause directly observed in the 5 workflow files above (file:line cited); the fix pattern already exists working in-repo. |
| Effort | Low-Medium | Mechanical: add an actionability gate mirroring `registry-validation.yml:90-95` to each of the other 4 workflows' comment step(s) — 6 gate sites total (3 in benchmark.yml + 1 each in the other three). No new capability required for the baseline fix. |

## Acceptance Criteria

1. `benchmark.yml`'s three comment jobs (go-tier1, frontend-throughput, e2e-latency) only
   post/update their PR comment when benchstat output indicates a real regression — not on
   every run that touches Go/web-app files.
2. When a prior regression comment exists and the current run no longer shows a regression,
   the stale comment is either removed or updated to reflect the cleared state (not left
   permanently showing an old regression).
3. `ux-analysis.yml` only posts when Lighthouse score drops below the existing 70 threshold,
   Axe fails (already blocking — a green Axe result produces no comment), or the Claude UX
   analysis step flags a real finding — not as an always-present status table.
4. `build.yml`'s feature-coverage comment only posts when coverage changed or dropped
   relative to the prior state, not unconditionally on every PR.
5. `e2e-video.yml`'s video-links comment is scoped down (posts only when there's a genuine
   anomaly, e.g. zero videos produced when videos were expected) or removed in favor of a
   check-run/artifact-list surface, since its normal-case content is pure navigation, never
   itself actionable.
6. A fully green PR (no regressions, Axe passes, Lighthouse ≥ 70, coverage unchanged, videos
   produced normally) produces zero or near-zero advisory comments from these five workflows.
7. None of the five workflows' existing blocking checks (Axe job failure, registry divergence
   >2% job failure, "Check new RPCs have tests" job failure) change behavior — this item only
   touches the advisory-comment gating layered on top.

## Out of Scope

- Changing what Axe / registry-divergence / RPC-test-coverage block on — already correct,
  working blocking checks.
- Migrating comment content into custom Check Runs (`checks: write` +
  `github.rest.checks.create`) — worth naming as a stretch option (see Open Questions), not
  required; the minimum fix is applying `registry-validation.yml`'s existing gating pattern.
- `registry-validation.yml` itself — already implements the desired pattern, is the
  reference, not a target.
- The app's own backend/agent-driven PR commenting (`server/services/github_service.go`,
  backlog shepherding skills, `project_plans/pr-comment-check-runs/`) — a separate,
  already-tracked concern, different mechanism entirely from these CI workflow files.

## Open Questions (preserve for planning phase, do not resolve here)

- Should Lighthouse/benchmark results move to `$GITHUB_STEP_SUMMARY` (already used by
  `benchmark.yml` alongside its comment) as the "always visible, but not comment-thread
  noise" home, since they're not real pass/fail checks?
- Is a real custom Check Run (via `checks: write`) worth adding for the Lighthouse
  threshold so it shows in the PR's checks list, or does step-summary + gated-comment
  cover it well enough for this project?
- For `e2e-video.yml`: does "no video artifacts were produced" ever actually fire in
  practice? If it's a real failure signal, it should probably become a check rather than a
  comment too.
- For benchmark comment cleanup (AC #2): should a cleared-regression state delete the
  sticky comment outright, or update it to a "no longer regressed" state? Precedent search
  needed — does any workflow in this repo already delete a stale sticky comment?

## Suggested Entry Point

`/sdd:quick` — each fix is small and mirrors an existing in-repo pattern
(`registry-validation.yml`'s gate), so this doesn't need the full 7-phase cycle. This
triage nonetheless runs the fuller research/plan/validate phases per the pipeline-mode
task instructions.
