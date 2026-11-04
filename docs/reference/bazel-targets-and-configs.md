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
| otel auto-instrumentation (`build-otel-auto*`, tag `otelcauto`) | Runs on `go build` after `make sync`; a guard proves the main build is unaffected |
| macOS codesign and TCC identity | The install path signs a copy of the Bazel output at a stable path |
| Diff- and history-based gates (`dupl --new-from-rev`, complexity gate, `proto-breaking`, jscpd, actionlint, shellcheck) | Wrapped; run after `make sync` |
| golangci-lint and the custom analyzers | Wrapped; `make lint-custom` stays the whole-suite path |
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
| `build`, `build-embedded`, `build-embedded-tymux` | `stapler-squad` | `//:stapler-squad` | `--config=release` stamps the version and embeds tmux and tymuxd. |
| `install-hooks` | `ssq-hooks` | `//cmd/ssq-hooks:ssq-hooks` | Makefile passes no ldflags, so no `x_defs`. |
| `build-mux` | `claude-mux` | none | `cmd/claude-mux` no longer exists, so `make build-mux` fails today. `//cmd/ssq-mux:ssq-mux` is the PTY multiplexer that replaced it. |
| `registry-generate-backend` | `tools/scanner/backend/cmd/scanner` | `//tools/scanner/backend/cmd:cmd` | Bazel names the output `cmd` (the package name); `make` names it `scanner`. Callers use the label, not the path. |
| `build-otel-auto*` | `stapler-squad-otel` | none | Permanent exception, see below. |

`bazel build //cmd/... //tools/scanner/...` builds every row that has a label.

## Release stamping

`--config=release` sets `--stamp`, `--compilation_mode=opt`, `--workspace_status_command=tools/bzl/workspace_status.sh` and the embed flags. The script emits `STABLE_GIT_VERSION`, `STABLE_GIT_BRANCH`, `STABLE_GIT_COMMIT` and `STABLE_GIT_WORKTREE` with the same sanitisers as the Makefile. `//:stapler-squad` maps them to `main.version` and `pkg/buildinfo.{Version,Branch,Commit,Worktree}` through `x_defs`, so a commit that touches no Go file re-runs only the status action and `GoLink`.

`go.mod` declares `godebug netdns=go`. rules_go does not read it, so `//:stapler-squad` sets `runtime.godebugDefault=netdns=go` through `x_defs`. `go version -m` shows no `DefaultGODEBUG` line for a rules_go binary (cmd/go writes that line); `bash tools/bzl/parity/compare-binary.sh` checks the string in the binary instead.

## Parity check

`bash tools/bzl/parity/compare-binary.sh` builds the release binary and the `go build` command `make -n build-embedded-tymux` prints, then compares `version` output, the `go version -m` module set and tags, `netdns=go`, and the embedded tmux and tymuxd bytes. Allowed differences: `CGO_ENABLED` (Linux Bazel builds are pure Go, ADR-006) and the module versions listed in `tools/bzl/parity/allowed-binary-deltas.txt`.

## otel variant: permanent exception

`make build-otel-auto` and `build-otel-auto-embedded` keep using `go build` through `otelc` (ADR-005: `otelc` rewrites the Go toolchain build and has no Bazel analogue). Both now depend on `sync` instead of `proto-gen ent-gen`: `make sync` generates `gen/` and `session/ent` with `make` (`BUILD_SYSTEM=make`) or `bazel run //:sync-gen` (`BUILD_SYSTEM=bazel`), so the otel build works with either. `scripts/otel-auto-isolation-guard.sh` still proves no default target reaches it.
