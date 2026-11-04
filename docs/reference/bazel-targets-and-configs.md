# Bazel targets and configs

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
