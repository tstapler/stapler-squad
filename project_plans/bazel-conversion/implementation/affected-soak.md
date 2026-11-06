# Affected-targets soak: historical comparison with the existing selection mechanisms (Task 7.3c)

Replays the 30 sampled partial-change PRs of `tools/bzl/baseline/pr-sample.json` (Task 0.1i, run 2026-11-02) through
the three CI selection mechanisms that exist today and through `tools/ci/affected-targets`, and counts where a
mechanism would run work that `affected-targets` does not select. Tool: `tools/ci/compare-affected` (Go, tests in
`compare_test.go`).

## The three mechanisms (inventory)

| Mechanism | Source | What it gates | Replay |
|---|---|---|---|
| `regex` | `.github/workflows/build.yml`, step "Detect Go/proto/web-app source or build-tooling changes" (`grep -qE '...'` over the PR file list) | `relevant=true` runs the Go, web and e2e pipeline | the pattern is extracted from `build.yml` at run time and matched per path |
| `script` | `tools/ci/detect-feature-changes.sh`, called by `.github/workflows/e2e-video.yml` | `record_features=true` runs the e2e video recording jobs (registry change, or a changed file containing `// +feature:`, `// +api:` or `// @feature`) | the script itself, with `CHANGED_FILES` set |
| `paths` | native `on.pull_request.paths` of `.github/workflows/mcp-integration.yml` | the MCP integration suites | the patterns are read from the workflow and matched with GitHub semantics (`**` spans directories, `*` and `?` stay in a segment) |

## What counts as a miss

A miss is a PR where the mechanism fires and `affected-targets` does not select the work it stands for:

- `regex`: no Bazel target and none of the jobs `go-test`, `web-build`, `e2e` selected.
- `script`: neither job `registry-freshness` nor `e2e` selected, and not `//...`.
- `paths`: no target under `//server/mcp` or `//server/services` (or `//...`) and no `mcp-integration` job selected.

Extra selections (`affected-targets` selects, the mechanism stays silent) are reported as `extra_*` and are not misses.

## Run

```
go build -o "$SCRATCH/affected-targets" ./tools/ci/affected-targets
go run ./tools/ci/compare-affected -root . -affected-bin "$SCRATCH/affected-targets" [-set partial|all] [-non-bazel-inputs <file>]
```

Measured at commit `43933064f` (branch `sdd/bazel-wp2-ci`, from `sdd/bazel-epic0`), `bazel query` against that checkout.

## Result: checked-in inputs (`tools/bzl/non-bazel-inputs.txt` as it is)

```
prs: 30
misses_regex: 2
misses_script: 1
misses_paths: 1
extra_regex: 0
extra_script: 4
extra_paths: 7
```

The Verify condition of Task 7.3c (all three `misses_*` equal to 0) is **not met** by the checked-in inputs.

| Miss | PR | Files | Selected | Cause |
|---|---|---|---|---|
| regex | #1363, #1358 | `.github/workflows/build.yml` only | job `workflow-lint` | `non-bazel-inputs.txt` maps all of `.github/**` to `workflow-lint`; a `build.yml` edit makes the regex run the whole pipeline |
| paths | #1355 | many workflow files, including `mcp-integration.yml` | job `workflow-lint` and others | same: the workflow file is itself a `paths:` entry |
| script | #1371 | `website/**`, including `website/tests/e2e/landing.spec.ts`, which contains `// @feature` | job `website-ci` | the script's trigger is file content (a marker), which a glob table cannot express |

## Result: with a candidate `non-bazel-inputs.txt` (not applied)

Same replay, handing `affected-targets` a scratch copy of the file with three added lines:

```
.github/workflows/build.yml go-test,web-build,e2e
.github/workflows/_prepare.yml go-test,web-build,e2e
.github/workflows/mcp-integration.yml mcp-integration
```

```
projected_misses_regex: 0
projected_misses_script: 1
projected_misses_paths: 0
```

(30 PRs). The three lines clear the regex and paths misses. Over the wider 100-PR set (`-set all`, partial and global
PRs) the same candidate leaves `regex 0`, `paths 0` and `script 10`.

## Findings

1. No miss comes from the reverse-dependency graph. Every miss is a non-graph input (a workflow file, or a content
   marker). INFERRED: `bazel-diff` would not change them either, because it hashes Bazel targets only, so these
   results alone do not justify switching (`research/architecture.md` 6c). The owner decides; the plan's literal rule
   ("switch only if misses > 0") is triggered by the checked-in inputs.
2. The regex and paths misses are fixed by the three added `non-bazel-inputs.txt` lines above. `tools/bzl/` is outside
   this work package, so the lines are a proposal.
3. The script misses are structural: `detect-feature-changes.sh` fires on markers inside source files (`// +api:` in a
   Go handler, `// @feature` in a spec, and the text `// +api:` inside `CLAUDE.md`), so a glob list cannot reproduce it.
   Over the 100-PR set it fires on 10 PRs whose selection has neither `e2e` nor `registry-freshness`. The mechanism gates
   only the e2e video recording (`e2e-video.yml`), which Epic 12 does not replace. Options: keep the script for that job
   after the flip, or map the marker files to the `e2e` job in `affected-targets` (a content scan, not a glob).
4. Limits of the replay (the numbers above are bounded by them):
   - the Bazel graph and file contents are those of the current checkout, not of each PR's merge commit, so a path
     deleted since the PR merged is replayed as modified and can only widen the selection, and the script cannot see a
     marker in a file that no longer exists (it can under-report script firings);
   - `pr-sample.json` carries paths without git status letters, so renames are not replayed as delete plus add;
   - GitHub returns at most 100 files per PR, and one sampled PR was truncated (`truncated_prs: 1`).
