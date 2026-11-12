# Bazel parallelism in CI

What limits parallel work in the Bazel CI job, what we changed, and what to re-measure. Numbers come from the
`bazel-profile` artifact of `.github/workflows/bazel.yml` (analysed from `~/.cache/tmp/bzprof/profile.gz`, the build step
of the old two-step job) and step timings from `gh api repos/tstapler/stapler-squad/actions/runs/<id>/jobs`.

## Workers (persistent and multiplex): not usable here

Multiplex workers need the rule to set `supports-workers` or `supports-multiplex-workers` in `execution_requirements`
and a worker binary that speaks the protocol ([multiplex docs](https://bazel.build/remote/multiplex)). Checked:

| Rule set | Result | Command |
|---|---|---|
| rules_go 0.65.0 (`GoCompilePkg`, `GoLink`, `RunNogo`, `ValidateNogo`) | No worker support: the module source has no `supports-workers`, `persistent_worker` or `WorkRequest` string | `grep -rli "persistent_worker\|supports-workers\|WorkRequest"` over the rules_go repo in `~/.cache/stapler-squad-bazel/repo/contents/3eefb4af.../*/` returned nothing |
| Our rules in `tools/` (`bun_dist` `BunWebBuild`, `BufGenTs`, `EntGen`, `OtelWeaveHermetic`) | They run shell scripts or tools directly; the only `execution_requirements` are cache/sandbox tags | `grep -rnE "supports-workers\|supports-multiplex" tools` finds only the detector and its test fixture in `tools/bzl/s6measure` |
| protobuf 36.2 / `rules_cc` | Not checked for workers; neither is on the critical path in the profile | not measured |

So `--worker_multiplex`, `--worker_max_instances`, `--dynamic_local_strategy=worker,...` and
`--experimental_worker_multiplex_sandboxing` would change nothing. We do not set them. `tools/bzl/s6measure` already
reports worker mnemonics from `bazel aquery`; re-run it after a rules_go upgrade, since a future rules_go could add workers.

## Real parallelism before and after #1592

Profile of the 28 min full-graph build (2 CPUs, 3 GB): `action resource lock` spans were at concurrency 2 for 1096 s of
1594 s, 1 for 132 s, 0 for 366 s; time-weighted mean 1.46. The cap was binding: with `build --local_resources=cpu=HOST_CPUS*.5`
a 4-vCPU `ubuntu-latest` ran two actions at once. `build:ci` now sets `cpu=HOST_CPUS` and `memory=HOST_RAM*.75` (#1592), so the cap
is 4 CPUs and about 12 GB. Tests take their parallelism from the same CPU budget (`--local_test_jobs` defaults to it).

The largest single actions, from the same profile (seconds): `OtelWeaveHermetic stapler-squad` 928, `GoCompilePkg server/server.a` 392,
`Copying directory web-app/dist` 359, `OtelModcache` 138. One 928 s action is a floor that more CPUs cannot lower; the
only lever for it is to overlap other work with it.

## Changes and reasoning

- **One `bazel test //...` instead of `bazel build //...` then `bazel test //...`** (`.github/workflows/bazel.yml`). Step timings
  from run 43118980774: build 1438 s, test 508 s (run 43117190237: 1643 s and 550 s). With two invocations no test could start
  until the whole graph, including the 928 s weave, finished. In one invocation Bazel starts each test as its deps complete,
  so the 508 s of tests overlap the weave. `--keep_going` keeps the old semantics (all failures reported). The BEP and profile
  now include test results, which also gives per-test durations for shard tuning (the old BEP came from the build step only).
  `bazel-trend` reads `buildMetrics` from the BEP, which is still present. Not measured locally (a full-graph run is not allowed on the dev host).
- **No worker, `--jobs`, or sharding flag changes.** `--jobs` is already high enough: the limit is the local CPU resource, not job
  slots. `_SIZES` in `tools/bzl/go_test.bzl` gives `server/services` 4 shards and `session` 2, which fits 4 CPUs; with
  the old 2-CPU cap shard_count above 2 could not run concurrently, and now up to 4 can. Do not raise shard counts until the merged run
  shows which test targets are long.
- Every `bazel build|test|run` call in `.github/workflows/bazel*.yml` passes `--config=ci`, except the `bazel cquery` calls in
  `bazel-macos.yml` (they must match the `--config=release --config=darwin_amd64` build to find its output) and the `bazel mod` call
  in `bazel-autofix.yml` (`bazel mod` rejects `build:` configs, per the comment there).
- Disk cache: `bazel-contrib/setup-bazel` 0.19.0 `cache-save` defaults to `true` and `bazel.yml` does not override it, so PR runs also save
  the disk cache (`action.yml` at the 0.19.0 tag). The job still shows the cost: `Post Set up Bazel` took 52-61 s in the runs above.

## What is not parallelizable

- `OtelWeaveHermetic` is a single 928 s action (weave plus build of the instrumented binary). It is the critical path floor.
- The "Freshness gates" step (about 101 s) runs before the main invocation; it needs a gazelle run and a `go mod tidy` check and only shares the Bazel server.
- `actions/checkout` with `fetch-depth: 0` took 143-195 s. It exists for `affected-targets -base`; a shallow clone plus fetching the base SHA
  would save most of that, but history may be used by other steps, so it is recommended and not done here.

## Re-measure after changes

1. Compare `Build and test the full graph` against the old `Build the full graph` plus `Test the full graph` (about 1946 s before, 2193 s in the second run).
2. Download `bazel-profile` and recompute concurrency of `action resource lock` spans; expect mean above 1.46 and a peak of 4.
3. From `bep.json` rank `testResult` durations; shard only targets that dominate the tail.
4. If `OtelWeaveHermetic` still dominates, look at its inputs (`tools/bzl/otel_weave_hermetic.bzl`), not at parallelism flags.
