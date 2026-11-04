# Baselines (Epic 0, Story 0.1)

Pre-Bazel numbers for the three drivers (CI speed, worktree cost, agent experience) plus hermeticity. Every number below was produced by a script in `tools/bzl/baseline/`; rerun it to refresh. Run date 2026-11-01 local (2026-11-02 UTC), repo `tstapler/stapler-squad`, collectors at branch `sdd/bazel-epic0-baselines`.

Status legend: **DONE** measured and verified; **PARTIAL** measured with a stated gap; **PENDING-QUIET-HOST** script authored and tested (guard refuses), measurement not run because the host load average was 7-34 (limit 2.0); never estimated.

| Task | Status |
|---|---|
| 0.1a CI time-to-green | DONE |
| 0.1b cold build and disk per worktree | DONE (2026-11-03, 5 runs at `7cfd08a69`, includes the otelc weave) |
| 0.1c fresh-clone hermeticity gap | DONE |
| 0.1d scale recount | DONE |
| 0.1e Go test pass set | PENDING-QUIET-HOST (`test-set.sh` not run in the 2026-11-03 window; warm one-package latency measured, see Measured pain) |
| 0.1f measured pain | DONE except the Go test pass set (0.1e `test-set.sh`, NOT RUN) |
| 0.1g targets | not started (needs 0.1b and owner confirmation); scaffold only in `tools/bzl/baseline/targets.yaml` |
| 0.1h seeded failure fixtures | DONE (make path) |
| 0.1i PR sampling and billed-minutes join | DONE |
| 0.1j hermeticity on the make path | DONE (2026-11-03, `7cfd08a69`) |

## Measured pain

Run 2026-11-02T04:41Z (UTC). Full object: `tools/bzl/baseline/measured.json` (produced by `pain-numbers.sh`; cold worktree, disk, warm latency and hermeticity filled 2026-11-03 from the collector outputs, see the quiet-window section).

| Quantity | Value | n | Source |
|---|---|---|---|
| PR time-to-green p50 / p90 / p95 | 2339 s (39.0 min) / 4298 s (71.6 min) / 5001 s (83.4 min) | 30 head SHAs | `ci-time-to-green.sh` |
| Cold worktree `make build` wall time (otelc-woven, cold `GOCACHE`) | 56.04 s median (min 55.17, max 57.28) | 5 of 5 | `worktree-cost.sh` |
| Disk per worktree (`disk_kb_worktree`) | 516316 KB median (identical in all 5 runs); `node_modules` 525388 KB is a symlink into the shared store, not per worktree; cold `GOCACHE` 1969496 KB | 5 of 5 | `worktree-cost.sh` |
| Manual regen steps per agent session, median / p90 | 0 / 0 (literal plan cohort); 0 / 2 (build-active cohort, n=16) | 30 / 16 | `regen-steps.sh` |
| Warm one-package `go test -short -count=1` latency (`./config`) | 2.687 s median (2.623 to 3.062) | 5 of 5 | `pain-numbers.sh` definition |
| RA1 median reverse-dependency fan-out | 0.259 (all 30 sampled PRs); 0.266 (the 17 Go-touching PRs); 29 of 30 PRs at or below 0.30 | 30 | `risky-assumptions.sh` |
| RA2 non-Go share of the critical path | 0.987 median (e2e median 1974 s, runner queue/setup 398 s, lint 0 s, other 34 s of a 2601 s median span) | 28 | `risky-assumptions.sh` |
| Legacy first-error legibility, 9 fixtures | 1 of 4 make-applicable fixtures names a correct remedy; 5 are `N/A-make` | 9 | `fixtures/run-make-path.sh` |
| Hermeticity (H1, H2) | H1: 38 distinct host executables, 1 non-loopback network destination (34.95.71.106:443); H2: `env -i PATH=/usr/bin:/bin make build` fails (exit 2, "go/buf/bun not found") | 1 | `hermeticity.sh --system make` |

### CI time-to-green (Task 0.1a)

Definition (plan.md Story 0.1): per PR head SHA, `max(updated_at) - min(run_started_at)` over the latest run of each required-in-practice workflow (`build.yml`, `lint.yml`, `mcp-integration.yml` when triggered); a SHA counts only if every such run ended `success` or `skipped`; the 30 most recent qualifying SHAs; nearest-rank percentiles. Skipped and short-path runs are included. Window of the 30 SHAs: 2026-10-31T23:55Z to 2026-11-02T03:59Z (about 28 hours, a busy period; the whole sample is one day of activity, so day-of-week and load effects are not averaged out).

```
cmd:  bash tools/bzl/baseline/ci-time-to-green.sh
n=30  p50_s=2339  p90_s=4298  p95_s=5001  min_s=859  max_s=5659  rerun_shas=2
per_workflow (span of each workflow run, over the SHAs where it ran):
  build.yml            n=30  p50=2230 s  p90=4245 s
  lint.yml             n=27  p50=1033 s  p90=1102 s
  mcp-integration.yml  n=12  p50=590 s   p90=675 s
sensitivity, every workflow that ran (adds GoReleaser Check, PR Diff Guards, UX Analysis):
  n=30  p50=2230  p90=4298  p95=5001
```

Spot check (VERIFIED by hand against `gh api actions/runs?head_sha=`): the newest SHA in the sample, Build ran 03:26:27Z to 03:59:55Z = 2008 s, matching the script's per-SHA value.

Known biases, stated rather than corrected: (1) the runs API reports only the latest attempt of a re-run workflow, so 2 of the 30 SHAs (`rerun_shas`) have a shortened span; (2) requiring all runs to succeed excludes PRs that needed a fix push, so this is the time to green of the clean path, not of a typical PR including failures.

### Fresh-clone hermeticity gap (Task 0.1c)

```
cmd:  bash tools/bzl/baseline/fresh-clone-gap.sh        (clone of HEAD d70173fe1 into /tmp, go build ./...)
go build ./... exit code: 1
missing packages reported by go (50): 2 under gen/proto/go, 48 under session/ent
  github.com/tstapler/stapler-squad/gen/proto/go/session/v1
  github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect
  github.com/tstapler/stapler-squad/session/ent  and 47 session/ent/<entity> packages
first error: session/instance_scroll_forward.go:11:2: no required module provides package
  github.com/tstapler/stapler-squad/gen/proto/go/session/v1; to add it: go get ...
```

Go's own suggested remedy (`go get`) is wrong for these packages: the fix is `make proto-gen` and `make ent-gen`. The 48 ent packages are those imported by tracked code; the plan (Story 0.4) states `go list ./session/ent/...` lists 53 in a generated checkout (not re-verified here).

### Scale recount (Task 0.1d)

```
cmd:  bash tools/bzl/baseline/scale-counts.sh
tracked_go_files: 3373
tracked_test_files: 1679
test_packages: 154
test_packages_root_module: 122
go_mod_files: 8
go_mod_files_live: 4
ignored_generated_go_files: 0        (fresh worktree; 370 in the main checkout)
```

Reconciliation of 3,373 versus about 3,743: **3,373 + 370 = 3,743 exactly**. The 370 are gitignored generated files present in the main checkout (333 under `session/ent`, 37 under `gen/proto`; `git -C <main> ls-files --others --ignored --exclude-standard '*.go'`), and none are untracked-but-unignored (0). So the research figure counted generated code, not stray files.

Further facts from the same command set: the 154 test packages (the `features.md` number) include 32 directories outside the root module (`project_plans/` spike copies, `tools/lint`, `tools/scanner`, `tuitest`); the root module has 122. Of the 8 tracked `go.mod` files, 4 are live (`go.mod`, `tools/lint`, `tools/scanner`, `tuitest`) and 4 are copies under `project_plans/go-git-fork-full-git-replacement/.../spikes/`. `go list -e ./...` in a fresh checkout lists 143 root-module packages (147 directories hold tracked non-nested `.go` files; the generated 2 + 48 are absent).

### Seeded failure fixtures on the make path (Task 0.1h)

Command: `bash tools/bzl/baseline/fixtures/run-make-path.sh` (results saved in `tools/bzl/baseline/fixtures/make-path-results.json`). Each fixture runs in its own throwaway worktree. "Names a remedy" means the output matches an actionable, correct remedy for that failure, not merely a restatement of the cause.

| Fixture | Row | Make-path result |
|---|---|---|
| 1-stale-build | E1 | `N/A-make` |
| 2-missing-external-dep | E2 | `true` (Go prints `go get rsc.io/quote`) |
| 3-missing-generated-code | E3 | `false` (Go suggests `go get`; the real remedy `make proto-gen` or `make ent-gen` is not named) |
| 4-oom | E4 | `false` (Go runtime `fatal error: failed to reserve page summary memory`, a 17-line stack, no remedy; reproduced with `ulimit -v 262144`) |
| 5-enospc | E5 | `false` (`write /dev/full: no space left`, cause only) |
| 6-cache-unreachable | E6 | `N/A-make` |
| 7-flock-queue-wait | E7 | `N/A-make` |
| 8-sandbox-failure | E8 | `N/A-make` |
| 9-cache-auth-rejected | E11 | `N/A-make` |

T5 baseline: 1 of the 4 make-applicable fixtures prints a correct remedy (25%). Bazel-mode bodies of the fixtures are provisional and unexercised until Task 1.6d (they need `tools/bazel` and Gazelle-generated BUILD files).

### PR sampling and billed-minutes join (Task 0.1i)

```
cmd:  bash tools/bzl/baseline/pr-sample.sh        (saved as tools/bzl/baseline/pr-sample.json)
population=100 merged PRs (gh sorts by creation time; re-sorted by mergedAt), class counts: partial 98, global 2
sample: the 30 most recent partial PRs; repr.ok=true, restratified=false
  median changed files: sample 3.5 vs population 5 (ratio ok, limit 2x)
  area share of PRs, sample vs population: server 27% vs 38%, session 37% vs 46%, web-app 20% vs 15%, other 67% vs 57%; max diff 11.3 pp (limit 20 pp)
  truncated PRs (gh returns at most 100 files): 1
```

The restratification path was exercised with smaller sample sizes (`--n 8`, `10`, `12` all took it and passed), so the fallback code runs.

```
cmd:  bash tools/bzl/baseline/ci-billed-minutes.sh
runner_min = sum of job (completed_at - started_at) / 60 x OS multiplier (Linux 1, macOS 10, Windows 2); the repository is public so GitHub billing is 0
class    n    runner_min p50   p90      ttg p50 (n)        ttg p90
partial  30   183.6            285.1    2704 s (27)        4298 s
global   2    334.1            364.0    2404 s (2)         19745 s
all      100  279.3            335.2    3629 s (84)        17834 s
```

`all` time-to-green (3629 s) is higher than the 30-SHA figure (2339 s) because it is taken over the head SHAs of the last 100 merged PRs (including slow, re-run and long-lived ones), not the 30 most recent clean SHAs. The two populations differ on purpose; T2 compares like with like. With the seed `GlobalInputs` list, only 2 of 100 PRs are `global` (only `go.mod`/`go.sum` exist before Bazel), so the `global` class has n=2 and its p50/p90 are not stable; the whole-population `all` row is the meaningful check.

### Risky assumptions (Task 0.1f)

RA1 (fan-out narrow): module graph from `go list -e` at HEAD, edges over `Imports + TestImports + XTestImports`, 143 module packages. Of the 30 sampled PRs, 13 touch no Go package (fan-out 0), 17 do; those 17 reach 37-44 packages each (0.259-0.308, median 0.266). `ra1_median_fanout = 0.259`; 29 of 30 PRs at or below 0.30. Caveats: the denominator excludes the 50 generated packages (adding them lowers every fraction, about 0.19-0.23), and the graph is HEAD's, not each PR's base. The Go-touching PRs all land at 0.259 or above (INFERRED cause: the changed packages sit under `session`/`server`, which many packages import; not traced per PR).

RA2 (time-to-green not dominated by parts Bazel does not touch): reconstructed from job timestamps (the jobs API has no `needs`): median share 0.987, i.e. e2e (median 1974 s) plus runner queue/setup (398 s) are essentially the whole critical path of the last-finishing required workflow. This exceeds the 0.60 threshold of EG5 in Task 0.14e. Not evaluated here as a gate verdict; it is an input.

### Regen steps per agent session (T6 input)

```
cmd:  bash tools/bzl/baseline/regen-steps.sh
transcripts matched by the plan's glob: 898 top-level .jsonl files
literal cohort (30 most recent): median 0, p90 0; 1 of 30 sessions had any invocation (1 proto-gen, 1 ent-gen)
build-active cohort (16 sessions that ran a make build/test/lint/ci/ready command; fewer than 30 exist): median 0, p90 2, totals proto-gen 4, ent-gen 2, sync 0, build-error hits 20
```

Only 20 of the 898 transcripts contain any `make` command at all (`grep -lE '"command":"[^"]*make '`); most are short Playwright/e2e agent runs. The count includes intentional runs, so it is an upper bound on steps run only to unblock a build.

## Not yet measured (host busy)

Load average at attempt: 12.7 to 34.3 (1-minute) over the session; the scripts refuse above 2.0 (VERIFIED: `bash tools/bzl/baseline/worktree-cost.sh --runs 5` prints `refusing to start: 1-minute load average 12.71 exceeds 2.0` and exits 3; same for `hermeticity.sh`, `test-set.sh`). Run these when `cut -d' ' -f1 /proc/loadavg` is 2.0 or below:

```
bash tools/bzl/baseline/worktree-cost.sh --runs 5 --out tools/bzl/baseline/baseline-worktree.json   # 0.1b
bash tools/bzl/baseline/hermeticity.sh --system make --out tools/bzl/baseline/hermeticity-make.json # 0.1j
bash tools/bzl/baseline/test-set.sh                                                                   # 0.1e
bash tools/bzl/baseline/pain-numbers.sh | tee tools/bzl/baseline/measured.json | jq -e '(.ci_p50_s|type=="number") and (.ci_p95_s|type=="number") and (.cold_worktree_wall_s|type=="number") and (.disk_kb_worktree|type=="number") and (.regen_steps_per_session_median|type=="number") and (.warm_one_pkg_test_s|type=="number") and (.ra1_median_fanout|type=="number") and (.ra2_non_go_share|type=="number")'
```

Then fill the PENDING rows above and run Task 0.1g.

## Deviations from the plan text found while implementing

- `make test PKG=./config` (Tasks 0.1f, T4) does not exist: the Makefile has no `PKG` variable (`grep -n PKG Makefile` shows only `BUILDINFO_PKG`). Warm one-package latency is measured with `go test -short -count=1 ./config`; the T4 collector text must change.
- Story 0.1 says the cold-build worktree has "no shared caches" while Task 0.1b says `GOCACHE` is "left as default". `worktree-cost.sh` supports both (`--gocache cold|default`, default `cold`) and records the mode.
- The regen-step baseline median is 0, so T6 ("at most 10% of baseline median") is degenerate (0 x 0.1 = 0) on the literal definition.
- The seed `GlobalInputs` classifies only 2 of 100 PRs as `global` (the Bazel files do not exist yet), so the global-class statistics have n=2.

## Quiet-window measurements (2026-11-03)

Host: the 1-minute load was at or below 2.0 immediately before every timed run (gate in the scripts / driver); the load average rises during and after a build because the build itself is the load, so a post-run value above 2.0 is self-inflicted and not a NOT QUIET flag. Free memory (`free -g` available column) was 35 to 36 GB throughout. Commit measured: `7cfd08a69` (`sdd/bazel-measure`, Epic 0 tip `883d92d6b` plus a TMPDIR fix to the two scripts). `make build` here always runs the otelc weave (`scripts/otel-auto-build.sh`; a build that exits 0 has woven, the script exits non-zero without `otelc`), and this is the plan's `baseline` definition.

- 0.1b `worktree-cost.sh --runs 1` five times (`--ref 7cfd08a69 --gocache cold`), pre-run loadavg1 0.99, 1.73, 1.75, 1.98, 1.87; wall_s 57.28, 56.95, 56.04, 55.72, 55.17 (median 56.04); `baseline-worktree.json`. The temp worktrees lived under `~/.cache/tmp/ssq-measure` (script edited to honour `TMPDIR`, not `/tmp`). `make build` also touches the shared `node_modules` store under `~/.stapler-squad/shared-node-modules/` (existing mechanism, read-only reuse).
- 0.1e (warm latency) `go test -short -count=1 ./config`, one warm-up then 5 timed runs: 2.687, 3.004, 3.062, 2.657, 2.623 s; load_pre/load_post 1.97/1.97, 1.97/1.97, 1.97/1.89, 1.89/1.89, 1.89/1.82; median 2.687 s.
- 0.1j `hermeticity.sh --system make`: pre-run load 1.77. A first invocation (start load 1.88) produced a failed strace build (`h1_make_exit` 1, 31 executables) and is discarded; a retry produced `h1_make_exit` 0 with 38 executables. The cause of the first failure was not captured (the script deletes its log); an unscripted strace repro build at load 1.98 exited 0, so it is treated as a transient flake, INFERRED.
- 0.1e `test-set.sh` (Go test pass set) was not run.
