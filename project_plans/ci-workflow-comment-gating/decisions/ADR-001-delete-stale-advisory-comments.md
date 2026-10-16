# ADR-001: Delete stale advisory sticky comments when the underlying condition clears

**Status**: Proposed — pending the dry-run verification named below
**Date**: 2026-08-12

## Context

`benchmark.yml` (×3 jobs), `ux-analysis.yml`, and `e2e-video.yml` post a sticky PR comment
found via an HTML marker (`listComments` → `find(c => c.body.includes(marker))` →
`updateComment`/`createComment`). Requirements AC #2 requires that when a prior regression
comment exists and the current run no longer shows a regression, the stale comment is either
removed or updated to reflect the cleared state — it must not be left showing outdated
information indefinitely.

Research (features.md §2, pitfalls.md §2) confirmed:
- No workflow in this repo has ever called `github.rest.issues.deleteComment` — grepped
  `.github/workflows/*.yml` for `deleteComment` across all 12 workflow files, zero hits.
- Both delete-on-clean and update-in-place are established, documented patterns elsewhere in
  the GitHub Actions ecosystem (`marocchino/sticky-pull-request-comment`'s `delete: true` mode;
  `mshick/add-pr-comment`'s delete-on-status option), so neither is a novel invention — but
  neither has in-repo precedent either.
- AC #6 requires a fully green PR to produce "zero or near-zero advisory comments." An
  update-in-place approach (rewriting the comment body to a "✅ cleared" state) still leaves a
  permanent comment artifact on the PR thread on every clean run once one has ever been posted
  — which does not satisfy "near-zero."

## Decision

Use **delete-on-clean**: when the gate's actionability signal (`hasRegression` for
`benchmark.yml`, `isActionable` for `ux-analysis.yml`, `videoAnomaly` for `e2e-video.yml`) is
false and a marker comment exists, call
`github.rest.issues.deleteComment({ owner, repo, comment_id: existing.id })` and return,
instead of updating the comment body to a "resolved" state.

`build.yml`'s feature-coverage comment is explicitly **excluded** from this decision — see the
Pattern Decisions table in `plan.md` ("`build.yml` 'coverage unchanged' resolution"): an
unchanged coverage percentage is not a "resolved problem" the way a cleared performance
regression or a cleared Lighthouse/Axe failure is, so delete-on-clean doesn't apply there.

## Consequences

- Comment threads return to a genuinely clean state matching AC #6's "near-zero" bar — better
  than update-in-place, which always leaves at least one permanent comment once triggered once.
- This introduces a net-new Octokit call pattern to this repo. Its permission-model behavior
  under the already-granted `pull-requests: write` scope is **INFERRED, not verified**
  (pitfalls.md §5): `createComment`/`updateComment` already work under that scope today because
  all three operations belong to the same Issues Comments REST API family
  (`/repos/{owner}/{repo}/issues/comments/{comment_id}`), and GitHub's documented permission
  model gates create/update/delete on that endpoint family together. But nothing in this repo
  has actually called `deleteComment` yet.
- **Required before shipping to production**: a real dry run — either a scratch
  `workflow_dispatch` step against a throwaway comment on a test PR, or simply observing the
  first live scratch-PR validation run in Phase 1 of `plan.md`'s rollout — to confirm
  `deleteComment` succeeds and does not 403. If it does 403, fall back to update-in-place
  (rewrite the body to a "✅ Resolved as of `<sha>`" state) for the affected workflow(s) only;
  the gate's actionability computation itself is unaffected by this fallback, only the
  clean-branch action changes.

## Alternatives Considered

- **Update-in-place to a "cleared" body.** Rejected as the primary approach because it
  contradicts AC #6's "near-zero" framing (a permanent comment artifact remains on every clean
  PR once triggered once) — but retained as the documented fallback if the `deleteComment` dry
  run reveals a permission problem.
