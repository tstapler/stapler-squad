# Research: Stack (Agent 1)

Scope: `.github/workflows/{benchmark,ux-analysis,build,e2e-video,registry-validation}.yml`.
All findings VERIFIED via `grep`/`Read` against this worktree's checked-in files
(commit context: branch `tstapler/triage-2375ca2e-2155-4165-a38a-214f1fd80e39`).

## 1. `actions/github-script` version — pinning is inconsistent (new finding)

| File | Pin | Notes |
|---|---|---|
| `benchmark.yml:108,334,548` | `@f28e40c7f34bde8b3046d885e986cb6290c5673b # v7` | SHA-pinned, all 3 jobs |
| `ux-analysis.yml:151` | `@v7` | **floating tag, not SHA-pinned** |
| `build.yml:218` | `@v7` | **floating tag, not SHA-pinned** |
| `e2e-video.yml:246` | `@v7` | **floating tag, not SHA-pinned** |
| `registry-validation.yml:75` | `@v7` | **floating tag, not SHA-pinned** (the reference file itself) |

All resolve to major version `v7`, which is the current major of `actions/github-script`
(latest is `v7.x`) — so no version bump is needed for this fix. But only `benchmark.yml` is
SHA-pinned; the other four files (including the reference `registry-validation.yml`) use an
unpinned `@v7` tag. This is a pre-existing inconsistency, **not required to fix for this
item's acceptance criteria** (out of scope: AC list only concerns comment-gating logic), but
worth flagging to the planner as a one-line drive-by opportunity per
`.claude/rules/fix-flaky-tests-dont-defer.md`'s sibling principle
(`feedback_fix_collateral_debt.md` in memory) — low-risk, mechanical, and touches the exact
lines this change will already be editing in 4 of the 5 files.

**Recommendation:** Don't block the main fix on this. If the planner wants to include it,
each `@v7` site can be swapped for the same SHA already used in `benchmark.yml`
(`f28e40c7f34bde8b3046d885e986cb6290c5673b # v7`) as a 1-line diff per site, no behavior
change (same major version already in use).

## 2. GitHub Script API surface — confirmed available, `deleteComment` is NOT used anywhere

All 5 files already use the sticky-comment pattern via `github.rest.issues.*`:

```
listComments   → benchmark.yml (x3), ux-analysis.yml, build.yml, e2e-video.yml
updateComment  → benchmark.yml (x3), ux-analysis.yml, build.yml, e2e-video.yml
createComment  → benchmark.yml (x3), ux-analysis.yml, build.yml, e2e-video.yml, registry-validation.yml
```

`registry-validation.yml` is notably the *only* file that does **not** list/update — it only
`createComment`s (line 111), because its gate (`return` early, lines 90-95) means it never
needs to reconcile against a prior comment; the comment only exists when the condition is
currently true. This matters for **AC #2** (stale-regression cleanup): the other four files
already have the `listComments`/`updateComment` machinery in place, so implementing
"update to cleared state" is a small diff (change the update branch's body when the
gate is false-but-a-marker-comment-exists); implementing "delete" requires calling
`github.rest.issues.deleteComment({ comment_id })`, which is a real, available Octokit
method but **has zero precedent anywhere in this repo's workflows today** — it would be a
net-new pattern, not a copy of existing code. This is a data point for the open question in
requirements.md, not a resolution of it (that's a planning-phase decision, not this agent's
call) — but the two options aren't equally "free": updating-in-place fits the existing code
shape with zero new API surface; deleting is technically trivial (single Octokit call) but
introduces a comment-thread side effect (a comment vanishing) that has no existing pattern
in this repo to model UX expectations on.

## 3. `$GITHUB_STEP_SUMMARY` precedent — used exactly once, only in `benchmark.yml`'s go-tier1 job

```
benchmark.yml:100  echo "## Go Benchmarks (Tier 1)" >> $GITHUB_STEP_SUMMARY
benchmark.yml:101  cat benchstat-tier1.txt >> $GITHUB_STEP_SUMMARY
```

This is the **only** `GITHUB_STEP_SUMMARY` usage across all 5 in-scope files (confirmed via
repo-wide grep of `.github/workflows/*.yml` — zero hits outside `benchmark.yml:100-101`).
Two things follow:

- It's a real, working precedent for "always-visible, not comment-thread noise" — but only
  for the **go-tier1** job. `frontend-throughput` (lines 334-360) and `e2e-latency`
  (lines 548-575) jobs in the same file do **not** write to step-summary today, despite
  being near-identical in structure to go-tier1. If the plan adopts step-summary as the
  home for raw benchstat output (per requirements.md's Open Question), that's 2 new
  step-summary writes to add for consistency, not a copy-paste of an already-uniform
  pattern.
- `ux-analysis.yml`, `build.yml`, and `e2e-video.yml` have **no** step-summary usage at all
  today — adopting step-summary there would be a first-time pattern in those files, though
  mechanically identical (`>> $GITHUB_STEP_SUMMARY` is a shell built-in, not an action
  capability).

## 4. Check Runs (`checks: write` / `github.rest.checks.create`) — NOT used anywhere in this repo

Repo-wide grep of `.github/workflows/*.yml` for `checks.create` and `checks:` (as a
permissions key) returns **zero hits**. Confirmed permissions blocks that exist in the
5 in-scope files:

```
benchmark.yml:57-59    contents: read, pull-requests: write   (go-tier1 job)
benchmark.yml:204-206  contents: read                          (job with no comment step)
benchmark.yml:251-253  contents: read, pull-requests: write   (frontend-throughput job)
benchmark.yml:387-389  contents: read                          (job with no comment step)
benchmark.yml:443-445  contents: read, pull-requests: write   (e2e-latency job)
ux-analysis.yml:15-16  pull-requests: write                    (workflow-level)
build.yml:38-40        contents: read, pull-requests: write   (workflow-level)
e2e-video.yml:14-15    pull-requests: write                    (workflow-level)
registry-validation.yml:16-18  contents: read, pull-requests: write
```

None grant `checks: write`. A real custom Check Run (the stretch option named in
requirements.md's Open Questions) would be a **fresh capability** for this repo — new
permission scope, new Octokit surface (`github.rest.checks.create`), no existing pattern to
copy. This confirms the requirements doc's framing of it as "worth naming as a stretch
option, not required" — it's meaningfully more effort than the baseline gate-copy fix, which
only needs `pull-requests: write` (already granted everywhere it's needed) and JS control
flow already demonstrated in `registry-validation.yml:90-95`.

## 5. New dependencies — none needed, confirmed

The baseline fix (mirroring `registry-validation.yml`'s early-return gate into the other 4
files' existing `actions/github-script` steps) requires:
- No new GitHub Action (`actions/github-script@v7` already present in all 5 files).
- No new Octokit method beyond what's already imported implicitly via `github.rest.issues.*`
  (`listComments`/`updateComment`/`createComment` all already called in the target files).
- No new workflow permission (`pull-requests: write` already granted in every job that has a
  comment step — confirmed above).
- Pure JS-in-YAML diff: an `if (!actionable) { return; }`-style early return added to each
  comment step's script block, following `registry-validation.yml:90-95`'s exact shape.

This holds **only** for the baseline fix (ACs #1, #3, #4, #6, #7). If the plan pursues the
stretch Check-Run option (Open Question #2) or the SHA-pinning drive-by (§1 above), those
add, respectively: a new `checks: write` permission grant + `checks.create` call (fresh
pattern, no in-repo precedent), and a set of 4 mechanical SHA substitutions (no new
capability, just pinning what's already resolved).

## Summary for planner

- Gate logic: 100% precedented, zero new deps — `registry-validation.yml:90-95` copies
  directly into the 6 gate sites (3 in `benchmark.yml`, 1 each in the other 3).
- AC #2 (stale comment cleanup): `updateComment`-to-cleared-state fits existing code shape
  with no new API; `deleteComment` is available but unprecedented in this repo — flag for
  planning-phase decision, don't default to it silently.
- Step-summary migration (Open Question): only 1 of 6 gate sites has any existing
  step-summary usage; adopting it elsewhere is a first-time pattern per file, though trivial
  shell redirection.
- Check Runs (Open Question): confirmed zero precedent anywhere in repo; real new capability
  (permission + API), correctly scoped as a stretch option, not baseline.
- Drive-by opportunity (not requested, not required): 4 of 5 files use floating `@v7` instead
  of the SHA pin `benchmark.yml` already uses — worth a one-line-per-site mention to the
  planner, not a blocker.
