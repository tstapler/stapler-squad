# Bazel targets, configs and exceptions

Reference for the Bazel build (provisional until cutover). Commands here are listed as they exist on this branch;
the ones not yet run are marked `<!-- proposal -->` for `tools/bzl/check-skill-proposals.sh`.

## Configs (`.bazelrc`)

Run Bazel through `tools/bazel`. `--config=agent` is added by the wrapper for non-TTY and Claude callers.

| Config | Effect |
|---|---|
| `agent` | Presentation only: no color or curses, no progress, `--test_output=errors`, `--test_summary=terse`, `--verbose_failures`. Never added to `bazel query` (it rejects it). |
| `dev` | `--test_output=errors`. |
| `ci` | `--lockfile_mode=error`, no progress, verbose failures, errors only. |
| `ci-cache-read`, `ci-cache-write` | Remote-cache failure policy (3 s timeout, one retry, circuit breaker, compression). The two must keep identical policy lines. |
| `linux` | `--@rules_go//go/config:pure`; auto-applied on Linux (pure Go, ADR-006). |
| `race` | cgo on plus `--@rules_go//go/config:race`; needs a host C compiler. |
| `release` | Same tag set as `embed_tymux`. |

Startup options the wrapper passes (`--max_idle_secs=30`, `-Xmx2g`) are not config lines; `startup:<config>` lines are ignored.
Shared caches: disk cache `~/.cache/stapler-squad-bazel/disk` (GC cap 100 GB, 30 days), repository cache
`~/.cache/stapler-squad-bazel/repo`.

## TagConfigs

Each config lists its full Go build-tag set in one line because rules_go's `tags` flag is last-wins, not additive
(spike S7). They therefore cannot be combined pairwise; a combination needs its own named config. The sealed list is
`tools/bzl/configs.txt`, checked by `tools/bzl/check-configs.sh`.

| Config | Go tags |
|---|---|
| `embed_tmux` | `embed_tmux` |
| `embed_tymux` | `embed_tmux,embed_tymux` |
| `sinkguard` | `sinkguard` |
| `gitoracle` | `gitoracle` |
| `integration` | `integration` |

## Make-to-Bazel map

| Make target | Behaviour |
|---|---|
| `make gazelle` | `bazel run //:gazelle` through `tools/bazel` <!-- proposal --> |
| `make gazelle-check` | Gazelle `-mode=diff`; fails if BUILD files are stale <!-- proposal --> |
| `make sync` | Generates `gen/` and `session/ent` for plain go/gopls (`make proto-gen ent-gen` while `BUILD_SYSTEM=make`) <!-- proposal --> |
| `make gen-freshness` | Fails with `run: make sync` when generated code is stale; only under `BUILD_SYSTEM=bazel` <!-- proposal --> |
| `make go-build`, `go-vet`, `go-test` | Native Go with auto-sync; not authoritative (`PKG=`, `RUN=` select package and `-run`) <!-- proposal --> |
| `make bazel-doctor` | Nine host and worktree health checks <!-- proposal --> |
| `make ready-bazel-guards` | Repo guardrail scripts that need only sh and git <!-- proposal --> |
| `make setup-git` | Per-clone git config: hooks path, lockfile merge driver; re-run after every fresh clone <!-- proposal --> |

`make build`, `make test`, `FORCE=1` and `RUN=` as Bazel wrappers arrive with Story 8.1 and Task 8.2e; until then the
skill lists them as proposals.

## PermanentExceptions

What stays outside Bazel, from [ADR-005](../../project_plans/bazel-conversion/decisions/ADR-005-permanent-exceptions-outside-bazel.md):

| Item | Interface to Bazel |
|---|---|
| macOS codesign and TCC identity | The install path signs a copy of the Bazel output at a stable path |
| Diff- and history-based gates (`dupl --new-from-rev`, complexity gate, `proto-breaking`, jscpd, actionlint, shellcheck) | Wrapped; run after `make sync` |
| golangci-lint | Wrapped; run after `make sync` |
| The 17 custom analyzers (`tools/lint`) | Run twice: as `nogo` on every Go target in the Bazel build (twin libraries, [below](#custom-analyzers-nogo)), and by `make lint-custom`, which stays the parity oracle until the Story 8.6 flip |
| release-please, website and demo deploys, benchmarks, RealClaude harness | Unchanged |
| goreleaser | Keeps its own `CGO_ENABLED=0 go build`; parity checked by `goreleaser-check.yml` |
| Homebrew formula | Written by goreleaser |
| Feature registry | Stays a make target; CI fails on a non-empty `git diff -- docs/registry` |
| Windows | Cross-compile only (owner decision 2026-11-01); no Windows tests under Bazel |

## Rules forks register

Rules repositories forked under ADR-008 are listed with upstream link, removal condition and age in
[bazel-forks.md](bazel-forks.md).

## Agent output validation (Task 11.2b)

Recorded 2026-11-03 on this branch with `--config=agent` on `//config/...` (one worktree, warm disk cache, host
load about 11, `--jobs=4 --local_resources=memory=3000`): the output had no ANSI escape sequences
(`grep -c $'\x1b'` printed `0`) and 35 lines. That run was a failing run (`//config:config_test` exited 1 in the
sandbox with `claude command not found in aliases or PATH`), so 35 lines is a failure view, which is longer than a
passing one. A passing-run line count is not recorded yet.
||||||| 0d1f727a1


Reference for the Bazel labels that replace each `make` binary target, the `--config` set, and the one build that stays outside Bazel. Decisions: ADR-003 (make wrapper), ADR-005 (permanent exceptions), ADR-007 (embeds). Run Bazel through `BAZEL_REAL=$(command -v bazelisk) tools/bazel ...`.

## Binaries

| `make` target | Output | Bazel label | Notes |
|---|---|---|---|
| `build`, `build-embedded`, `build-embedded-tymux` | `stapler-squad` (otelc-woven) | `//:stapler-squad` | The one shipped binary is always otelc-woven (sandboxed, remote-cacheable action, see "otel weave" below). `--config=release` stamps the version and embeds tmux and tymuxd. `//:stapler-squad_plain` is the uninstrumented `go_binary`, used only by the parity harness. |
| `install-hooks` | `ssq-hooks` | `//cmd/ssq-hooks:ssq-hooks` | Makefile passes no ldflags, so no `x_defs`. |
| `build-mux`, `install-mux` | `ssq-mux` | `//cmd/ssq-mux:ssq-mux` | `cmd/claude-mux` was renamed to `cmd/ssq-mux`; `make build-mux` builds `./cmd/ssq-mux` to `./ssq-mux`. |
| `registry-generate-backend` | `tools/scanner/backend/cmd/scanner` | `//tools/scanner/backend/cmd:cmd` | Bazel names the output `cmd` (the package name); `make` names it `scanner`. Callers use the label, not the path. |

`bazel build //cmd/... //tools/scanner/...` builds every row that has a label.

## Architecture guards: `forbidden_deps_test`

`//tools/bzl:forbidden_deps.bzl` replaces the four `go list -deps` guards that skip under Bazel (`SkipGoListUnderBazel`). An aspect walks `deps` and `embed` of `target` (point it at a `go_library`; a `go_test` would also count its test deps) and the rule calls `fail()` at analysis time, naming the shortest dep chain, when a `forbidden` label pattern appears or a `required` one is missing. Patterns: `//pkg`, `//pkg:name`, `//pkg/...`, `@repo//...`. `direct_only = True` checks direct deps only. Instances: `//:forbidden_deps_test`, `//jules:forbidden_deps_test`, `//server/prwatch:forbidden_deps_test` (direct only), `//server/wakes:forbidden_deps_test`. The Go tests still run under `go test`. `bazel test //tools/bzl/forbidden_deps_test_data/...` proves each form (transitive, direct only, package prefix, missing required) fails when seeded.

## Release stamping

`--config=release` sets `--stamp`, `--compilation_mode=opt`, `--workspace_status_command=tools/bzl/workspace_status.sh` and the embed flags. The script emits `STABLE_GIT_VERSION`, `STABLE_GIT_BRANCH`, `STABLE_GIT_COMMIT` and `STABLE_GIT_WORKTREE` with the same sanitisers as the Makefile. `//:stapler-squad` maps them to `main.version` and `pkg/buildinfo.{Version,Branch,Commit,Worktree}` through `-ldflags -X` inside the weave script, so a commit that touches no Go file re-runs only the weave (the stamped status file is its input).

`go.mod` declares `godebug netdns=go`. rules_go does not read it, so `//:stapler-squad_plain` sets `runtime.godebugDefault=netdns=go` through `x_defs`. `go version -m` shows no `DefaultGODEBUG` line for a rules_go binary (cmd/go writes that line); `bash tools/bzl/parity/compare-binary.sh` checks the string in the binary instead.

## Parity check

`bash tools/bzl/parity/compare-binary.sh` builds the uninstrumented release binary (`//:stapler-squad_plain`) and the plain `go build` command (the otelc wrapper is stripped from the line) `make -n build-embedded-tymux` prints, then compares `version` output, the `go version -m` module set and tags, `netdns=go`, and the embedded tmux and tymuxd bytes. Allowed differences: `CGO_ENABLED` (Linux Bazel builds are pure Go, ADR-006) and the module versions listed in `tools/bzl/parity/allowed-binary-deltas.txt`.

## otel weave: the default binary (spike S15b, FallbackWorkaround)

`//:stapler-squad` is an `otel_weave_hermetic` rule (`tools/bzl/otel_weave_hermetic.bzl`, `tools/bzl/otel_weave_hermetic.sh`), not a `go_binary`: the shipped binary is always `otelc` auto-instrumented (ADR-005 amendment). The action is sandboxed and remote-cacheable (`no-remote-exec` only): its inputs are the Go sources of the `:stapler-squad_lib` archive closure (plus the otelc custom rule package), the Bazel-built proto and ent code, the web dist, the embed staging, the Go SDK, a module-cache tarball and the committed overlay in `tools/bzl/otel_overlay/`. The overlay is the pre-computed output of `otelc setup` (go.mod/go.sum with the otelc pins, the two generated Go files, the rule bundle, `matched.json` with path placeholders), so the action runs only the `-toolexec=otelc` build, offline (`GOPROXY=off`, `-mod=readonly`), with a private `GOCACHE` and an empty `HOME`. The repo's `go.mod` and `go.sum` are never touched. Version stamping is `-ldflags -X` from the stamped status file; `--platforms=` cross builds work (the script reads the target GOOS/GOARCH from rules_go and turns cgo off), and `//tools/bzl/windows:stapler-squad_windows_*` are `otel_cross_binary` targets over `//:stapler-squad`.

Maintenance: after any change to `go.mod`, `go.sum`, the otelc version or `instrumentation/otelc/safeexec`, run `GO=<rules_go sdk go> tools/bzl/otel_overlay/gen.sh` and commit the result. `CHECK=1` diffs instead (CI runs it); the generator canonicalizes otelc's order-nondeterministic output so the check is stable.

Remaining non-hermetic parts: the module cache comes from the `OtelModcache` action, a go.sum-verified `go mod download` that needs the network (the result is cached and re-run only when the overlay changes; a `go_deps`-based replacement is S15b-4); cgo packages use the host `gcc` (S15b-7); a cache miss is a cold `GOCACHE` (S15b-5); darwin was not built. The work directory is fixed (`/tmp/ssq-otel-work` under Bazel's hermetic `/tmp`) because a per-sandbox path leaks into the Go build IDs; with it two real executions are `cmp`-identical.

## Custom analyzers (nogo)

`go_sdk.nogo` in `MODULE.bazel` registers one nogo, `//tools/bzl/nogo:nogo`, for every Go target in the repo. It
runs all 17 analyzers from `tools/lint` (the rules in `.claude/rules/*.md`); a finding fails the build of that target.

| Piece | What it is |
|---|---|
| `//tools/bzl/nogo:<analyzer>` | Twin library: the same `tools/lint/<analyzer>/analyzer.go` (referenced, never copied) compiled against the root `x/tools`. nogo links the root `x/tools`, and the isolated `tools/lint` module's copy would be a second one at link time. |
| `ANALYZERS` in `tools/bzl/nogo/BUILD.bazel` | The twin list. `make nogo-twins-check` (also run by `make lint-custom`) fails when it, the `tools/lint/*/analyzer.go` directories and `cmd/linter`'s imports disagree. |
| `tools/bzl/nogo/nogo_config.json` | `_base.exclude_files`: `external/`, `bazel-out/` (generated code) and `tools/lint/` (not linted by `make lint-custom` either). The analyzers scope themselves by package path and file name. |
| `//tools/bzl/nogo/testdata:seed_<analyzer>` | One seeded violation per analyzer, all `manual`. `make nogo-seed-check` builds them and requires each to fail with its own analyzer's finding. |
| `//tools/lint/<analyzer>:<analyzer>_test` | The analysistest fixtures; `gotool.Setup(t)` lets them run under Bazel instead of skipping. |

To add an analyzer: create `tools/lint/<name>/`, add `exports_files(["analyzer.go"])` to its `BUILD.bazel`, add the name to
`ANALYZERS` and to `cmd/linter`, add a seed under `testdata/seed/<name>/` and a `SEEDS` or `TEST_SEEDS` row, then run
`make nogo-twins-check nogo-seed-check`. Suppress a finding the same way as under make: `//nolint:<analyzer> <reason>`.
Action cost: [spike-results.md, "8.4 nogo twins"](../../project_plans/bazel-conversion/implementation/spike-results.md).
