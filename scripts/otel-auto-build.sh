#!/usr/bin/env bash
# Build-time wrapper for the opt-in otelc auto-instrumentation path.
#
# Composes the Toolexec Injection GOFLAGS (ADR-004) around a caller-supplied
# `go build`/`go test` argv, then verifies otelc didn't silently mutate
# go.mod/go.sum during the build itself (Module Mutation Guard, Story 2.1.4 —
# pitfalls.md #1c).
#
# Scope — build-time concerns ONLY. This process exits once the child build
# finishes; it never sets OTEL_ENABLED, OTEL_TRACES_EXPORTER, or
# OTEL_METRICS_EXPORTER, since a later `stapler-squad-otel` invocation is a
# separate process that inherits nothing from this one. Runtime suppression
# is the Run Recipe's job — see .claude/docs/opentelemetry-auto-instrumentation.md.
#
# `otelc setup` bootstrap (run every invocation) + `otelc cleanup` trap:
#   `otelc setup` writes per-checkout scaffolding — `.otelc-build/`,
#   `otel.instrumentation.go`, `otelc.runtime.go` — required for the toolexec
#   weave to compile. These are gitignored (see .gitignore) rather than
#   committed, because `otelc.runtime.go` has no build tag and unconditionally
#   blank-imports otelc instrumentation packages: its mere presence breaks a
#   plain `go build .` / `make build` (confirmed empirically), so it must not
#   linger in the tree after this script exits — leaving it behind would leak
#   the opt-in build into the default one, which Epic 2.1's isolation goal
#   forbids.
#
#   `otelc setup` also ADDS `require`/`replace` lines to go.mod/go.sum for
#   otelc's instrumentation modules, and can bump unrelated shared transitive
#   deps as a side effect of re-resolving the module graph (confirmed: it
#   bumped github.com/gofrs/flock v0.12.1 -> v0.13.0 and otlptracegrpc
#   v1.39.0 -> v1.44.0 on first run) — a real instance of the pitfalls.md
#   #1c failure mode. It is not silent here, though: this script snapshots
#   go.mod/go.sum before calling `otelc setup` at all and restores that exact
#   snapshot in the cleanup trap (below), in addition to `otelc cleanup`
#   removing the generated files above. The restore is done by our own byte
#   backup/copy, not by relying on `otelc cleanup`'s state-manager revert —
#   confirmed empirically (2026-08-22) that revert alone is NOT reliable once
#   `otelc setup` runs more than once per cycle (this script's own two-pass
#   custom-rule injection, below, does exactly that) — see the cleanup
#   function's own comment for the mechanism. So a full `build-otel-auto` run
#   leaves go.mod/go.sum untouched end to end, and the Module Mutation Guard
#   below — bracketing only the child build, between setup and cleanup — is
#   checking the actual weave/compile step, the thing Story 2.1.4 asks it to
#   check, not otelc's own bootstrap/teardown.
#
# Custom rule injection (Story 5.1.2, Task 5.1.2c — Spike E, spike-verdicts.md):
#   otelc has no additive way to merge a repo-local custom rule (e.g. this
#   repo's own instrumentation/otelc/safeexec/otelc.yaml, hooking
#   executor/safeexec.CommandContext) into the SAME weave as its built-in
#   rules (net/http, database/sql, ...) in one `otelc setup` call:
#     - `--rules`/`OTELC_RULES` REPLACES the entire ruleset with just the
#       given file (confirmed by reading tool/internal/setup/setup.go's
#       loadRules: it returns early on either being set, skipping AutoPin
#       and the embedded-defaults path entirely) — using it here would drop
#       net/http/database/sql/etc. spans for the whole build.
#     - The additive path is otel.instrumentation.go (the "tool file"):
#       `otelc setup`'s Pin step (tool/internal/setup/pin.go) only
#       auto-discovers built-in rules when NO tool file exists yet
#       (generatePinnedProjects); if one already exists it takes the
#       validate-and-keep path instead (updatePinnedProjects), preserving
#       every import already listed — including a hand-added one, as long as
#       it resolves to a real package with a valid rule file. Confirmed
#       empirically (2026-08-22): run `otelc setup` once to get the
#       auto-discovered default tool file, append our custom package's
#       blank import, run `otelc setup` again — matched.json then carries
#       both the defaults AND our hook_command_context rule.
#   Hence the two-pass setup below instead of a single call.
#
# Usage:
#   ./scripts/otel-auto-build.sh go build -ldflags "-X main.version=1.2.3" -o stapler-squad-otel .
#   ./scripts/otel-auto-build.sh go test ./... -timeout=20m
set -euo pipefail

CUSTOM_RULE_IMPORT='_ "github.com/tstapler/stapler-squad/instrumentation/otelc/safeexec"'
TOOL_FILE="otel.instrumentation.go"

if ! command -v otelc >/dev/null 2>&1; then
  echo "✗ otelc not found on PATH." >&2
  echo "  Install it from open-telemetry/opentelemetry-go-compile-instrumentation:" >&2
  echo "  https://github.com/open-telemetry/opentelemetry-go-compile-instrumentation" >&2
  echo "  (see project_plans/go-auto-instrumentation/implementation/spike-verdicts.md" >&2
  echo "  for the exact install command used in this repo)." >&2
  exit 1
fi

if [ "$#" -eq 0 ]; then
  echo "✗ usage: $0 <go build|go test ...>" >&2
  exit 1
fi

# `otelc setup` (called bare, no args) only generates the per-package
# runtime/hook-linkage file (otelc.runtime.go equivalent, via
# generateRuntimePerPackage) for the CURRENT DIRECTORY's package ("."),
# because tool/internal/setup/setup.go's getBuildPackages() falls back to
# loading just "." when given an empty args slice. That silently starves
# every OTHER package of its hook linkage — confirmed empirically
# (2026-08-22): `go test ./executor/... ./session/git/...` under a bare
# `otelc setup` fails to LINK with "relocation target ... not defined" for
# EVERY rule's trampoline (built-in ones included, not just this repo's
# custom safeexec hook), because those packages' own compiled units never
# got the linkname-generating runtime file the trampolines need. Forwarding
# the actual build target args (same ones "$@" itself is about to build)
# to `otelc setup` fixes it, matching what `otelc go build`/`otelc go test`
# would do internally (see setup.go's GoBuild/runGoBuild, which always
# calls Setup with the real command args) — this script can't use that
# wrapper subcommand directly because it needs to run its own two-pass
# custom-rule injection between setup and the actual build (see below).
if [ "${1:-}" = "go" ]; then
  verb_prefix=("$1" "$2") # "go" "build"/"test"/"install"
  build_target_args=("${@:3}")
else
  verb_prefix=("$1")
  build_target_args=("${@:2}")
fi

# instrumentation/otelc/safeexec/hook.go is gated behind the `otelcauto`
# build tag (see that file's doc comment) so a plain `go build ./...` /
# `make build` / `make lint` never tries to compile it — it imports
# go.opentelemetry.io/otelc/pkg/hook, which only exists in go.mod during this
# script's own `otelc setup` window. Confirmed empirically (2026-08-22):
# without the tag, `go build ./...` failed with "no required module provides
# package .../pkg/hook" even on a build that never touches this package,
# because `./...` still tries to compile every directory under the module
# root. The tag must be active for BOTH `otelc setup` (so pin/rule-matching
# can resolve the package) and the real build/test step below — merged with
# any `-tags` the caller already passed (e.g. `make build-otel-auto-embedded`'s
# `-tags embed_tmux`), not overwritten, since GOFLAGS-level tag injection
# would lose a caller-supplied `-tags` outright (an explicit CLI `-tags`
# always wins over one set via GOFLAGS, they don't merge).
#
# Single classification pass over build_target_args splits it into:
#   - setup_pkg_args: bare package-pattern positionals only (e.g. ".",
#     "./executor/safeexec"). This is ALL `otelc setup`'s own CLI accepts —
#     it parses flags strictly and rejects anything it doesn't recognize
#     (confirmed empirically, 2026-08-22: `otelc setup -o /tmp/x .` fails
#     with "flag provided but not defined: -o", and likewise for `-tags`),
#     unlike `otelc go build`/`otelc go test`, whose command definition sets
#     SkipFlagParsing (cmd_go.go) precisely so it can pass through arbitrary
#     `go` flags. Since this script calls `otelc setup` directly (not
#     `otelc go ...`, so it can run its own two-pass custom-rule injection
#     between setup and the real build), the merged -tags value is instead
#     exported via GOFLAGS below rather than passed here — pkgload.LoadPackages's
#     `packages.Config` leaves `Env` unset, and golang.org/x/tools/go/packages
#     defaults that to the current process environment, so the `go list`
#     calls `otelc setup` makes internally still pick up GOFLAGS's -tags even
#     though `otelc setup`'s own CLI never sees it as an argument.
#   - other_flag_args: every other flag and its value (e.g. -ldflags "...",
#     -o path), preserved for the real build/test invocation below.
# The real invocation is then rebuilt as verb + other_flag_args + our merged
# -tags + setup_pkg_args, in that order — flags MUST precede package-pattern
# positionals on a `go build`/`go test` command line, or `go` treats a
# trailing "-tags=..." as another package pattern instead of a flag
# ("malformed import path ...: leading dash", confirmed empirically,
# 2026-08-22, from a first attempt that just appended -tags at the end).
OTELC_AUTO_TAG="otelcauto"
caller_tags=""
setup_pkg_args=()
other_flag_args=()
skip_next_value=false
skip_next_is_tags_value=false
for arg in "${build_target_args[@]}"; do
  if [ "$skip_next_is_tags_value" = true ]; then
    caller_tags="$arg"
    skip_next_is_tags_value=false
    continue
  fi
  if [ "$skip_next_value" = true ]; then
    other_flag_args+=("$arg")
    skip_next_value=false
    continue
  fi
  case "$arg" in
    -tags=*)
      caller_tags="${arg#-tags=}"
      ;;
    -tags)
      skip_next_is_tags_value=true # next arg is the tags value — captured above, not a plain flag value
      ;;
    -o | -ldflags | -gcflags | -asmflags | -timeout | -run | -count | -p)
      other_flag_args+=("$arg")
      skip_next_value=true
      ;;
    -*)
      other_flag_args+=("$arg") # combined-form flag (e.g. -timeout=20m)
      ;;
    *)
      setup_pkg_args+=("$arg")
      ;;
  esac
done
if [ -n "$caller_tags" ]; then
  merged_tags="${caller_tags},${OTELC_AUTO_TAG}"
else
  merged_tags="$OTELC_AUTO_TAG"
fi

run_args=("${verb_prefix[@]}" "${other_flag_args[@]}" "-tags=${merged_tags}" "${setup_pkg_args[@]}")

# Belt-and-suspenders go.mod/go.sum restore, independent of `otelc cleanup`'s
# own state-manager revert. That revert is unreliable across this script's
# two-pass `otelc setup` (confirmed empirically, 2026-08-22): each `otelc
# setup` invocation is its own process, and setupLocked's "state manager not
# found in context" branch (setup.go) stores a fresh, empty tracker instead
# of loading the one pass 1 persisted — so pass 2's AutoPin snapshots
# go.mod/go.sum as they stood AFTER pass 1's bump, and `otelc cleanup` then
# "reverts" to that already-bumped state, leaving it dirty. A plain byte
# backup/restore here doesn't depend on otelc's internal tracking at all.
# shellcheck disable=SC2329  # invoked indirectly via `trap ... EXIT` below
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  otelc cleanup >/dev/null 2>&1 || true
  cp "$mod_backup_dir/go.mod" "$mod_backup_dir/go.sum" .
  rm -rf "$mod_backup_dir"
  rm -f otel.instrumentation.go otelc.runtime.go
  # generateRuntimePerPackage (setup.go) writes an otelc.runtime.go into
  # EVERY package passed as a build target (see the build_target_args note
  # above), not just the repo root — confirmed empirically (2026-08-22):
  # `otelc setup ./executor/... ./session/git/...` left stray
  # executor/otelc.runtime.go, executor/safeexec/otelc.runtime.go, and
  # session/git/otelc.runtime.go behind after a failed build, since a plain
  # `rm -f otelc.runtime.go` only ever removed the root copy. A leaked one
  # breaks the *next* invocation (it hardcodes go.mod requires from the
  # otelc version that generated it) and, per this script's own isolation
  # goal, must never linger in the tree regardless of exit path.
  find . -name otelc.runtime.go -not -path './.otelc-build/*' -delete 2>/dev/null || true
  exit "$status"
}

# trap registered immediately after mktemp -d, before the cp backup step
# below: if cp itself fails, the temp dir still gets cleaned up instead of
# leaking (Story 2.1.4 repair-loop finding).
mod_backup_dir="$(mktemp -d)"
trap cleanup EXIT INT TERM
cp go.mod go.sum "$mod_backup_dir/"

# See the "instrumentation/otelc/safeexec/hook.go is gated..." comment above:
# exported (not passed as a CLI arg) so otelc setup's own internal `go list`
# calls resolve/validate that package under the right build tag, even though
# `otelc setup` itself never sees -tags as an argument.
export GOFLAGS="${GOFLAGS:-} -tags=${merged_tags}"

echo "▶ otelc setup — pass 1 (auto-discover built-in rules; see header comment on go.mod/go.sum impact)"
otelc setup "${setup_pkg_args[@]}"

if [ -f "$TOOL_FILE" ] && ! grep -qF "$CUSTOM_RULE_IMPORT" "$TOOL_FILE"; then
  echo "▶ injecting custom rule import into $TOOL_FILE (instrumentation/otelc/safeexec)"
  # Insert right before the closing ")" of the import block written by pass 1.
  sed -i "s#^)\$#\t${CUSTOM_RULE_IMPORT}\n)#" "$TOOL_FILE"
  echo "▶ otelc setup — pass 2 (merge custom rule into the matched ruleset)"
  otelc setup "${setup_pkg_args[@]}"
fi

export GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"
echo "▶ GOFLAGS=${GOFLAGS}"
echo "▶ ${run_args[*]}"

mod_sum_before="$(sha256sum go.mod go.sum 2>/dev/null || true)"

set +e
"${run_args[@]}"
build_status=$?
set -e

mod_sum_after="$(sha256sum go.mod go.sum 2>/dev/null || true)"

if [ "$mod_sum_before" != "$mod_sum_after" ]; then
  echo "✗ Module Mutation Guard: go.mod/go.sum changed during the build step." >&2
  echo "  This is the silent dependency upgrade documented in pitfalls.md §1c." >&2
  diff <(echo "$mod_sum_before") <(echo "$mod_sum_after") >&2 || true
  git diff -- go.mod go.sum >&2 || true
  exit 1
fi

exit "$build_status"
