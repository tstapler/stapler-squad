#!/usr/bin/env bash
# Build-time wrapper for the opt-in otelc auto-instrumentation path.
#
# Composes the Toolexec Injection GOFLAGS (ADR-004) around a caller-supplied
# `go build`/`go test` argv, then verifies otelc didn't silently mutate
# go.mod/go.sum during the build itself (Module Mutation Guard, Story 2.1.4 —
# pitfalls.md #1c). Build-time concerns ONLY — this process exits once the
# child build finishes and never sets any OTEL_* runtime var; see the Run
# Recipe in docs/how-to/enable-otel-auto-instrumentation.md for that.
#
# Runs `otelc setup` twice (auto-discover built-in rules, then inject this
# repo's own custom rule import and re-run) because otelc has no additive way
# to merge a repo-local rule into the same weave as its built-in rules in one
# call — `--rules`/`OTELC_RULES` replaces the whole ruleset instead. Restores
# go.mod/go.sum from its own byte backup (not `otelc cleanup`'s revert, which
# is unreliable across a two-pass setup) and deletes every generated
# otelc.runtime.go/otel.instrumentation.go, so a plain `go build ./...` is
# never affected by this having run. Full investigation trail, exact repro
# commands, and dated findings:
# project_plans/go-auto-instrumentation/implementation/spike-verdicts.md
# (Spike E) and docs/how-to/enable-otel-auto-instrumentation.md.
#
# Usage:
#   ./scripts/otel-auto-build.sh go build -ldflags "-X main.version=1.2.3" -o stapler-squad-otel .
#   ./scripts/otel-auto-build.sh go test ./... -timeout=20m
set -euo pipefail

CUSTOM_RULE_IMPORT='_ "github.com/tstapler/stapler-squad/instrumentation/otelc/safeexec"'
TOOL_FILE="otel.instrumentation.go"

# hash_cmd: sha256sum isn't on PATH by default on macOS (shasum -a 256 is,
# following this repo's uname -s Darwin-detection convention — Makefile:5,9).
# The Module Mutation Guard below depends on this producing a real hash on
# both sides, or a missing tool would silently compare two empty strings as
# equal and false-pass the one check pitfalls.md names as this feature's core
# safety concern.
hash_cmd() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$@"
  else
    echo "✗ no sha256sum/shasum on PATH — cannot run the Module Mutation Guard" >&2
    exit 1
  fi
}

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

# `otelc setup` with no args only wires hook linkage for the current
# directory's package — every other package silently loses its trampoline
# linkage (LINK failures) unless the real build target args are forwarded to
# it too, matching what `otelc go build`/`otelc go test` does internally.
# This script can't use that wrapper subcommand directly: it needs its own
# two-pass custom-rule injection between setup and the real build (below).
# See spike-verdicts.md (Spike E) for the full investigation trail.
if [ "${1:-}" = "go" ]; then
  verb_prefix=("$1" "$2") # "go" "build"/"test"/"install"
  build_target_args=("${@:3}")
else
  verb_prefix=("$1")
  build_target_args=("${@:2}")
fi

# instrumentation/otelc/safeexec/hook.go is gated behind the `otelcauto`
# build tag (see that file's doc comment) so a plain `go build ./...` never
# tries to compile it. The tag must be active for both `otelc setup` and the
# real build/test step below, merged with any caller-supplied `-tags` (an
# explicit CLI `-tags` always wins over one set via GOFLAGS, so they must be
# merged here rather than just exported).
#
# The loop below splits build_target_args into bare package-pattern
# positionals (setup_pkg_args — all `otelc setup`'s strict CLI parser
# accepts; the merged -tags value goes out via GOFLAGS instead, since
# `otelc setup`'s internal `go list` calls still pick that up) and every
# other flag (other_flag_args, preserved for the real build/test call below).
# Flags MUST precede package positionals on a `go build`/`go test` command
# line, or `go` misparses a trailing `-tags=...` as an import path. See
# spike-verdicts.md (Spike E) for the full investigation trail.
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
# shellcheck disable=SC2329,SC2317  # invoked indirectly via `trap ... EXIT` below
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
  # BSD/macOS `sed -i` requires an explicit backup-suffix argument (GNU's
  # doesn't) — the -i.bak form works on both, so use it unconditionally and
  # discard the backup.
  sed -i.bak "s#^)\$#\t${CUSTOM_RULE_IMPORT}\n)#" "$TOOL_FILE"
  rm -f "$TOOL_FILE.bak"
  echo "▶ otelc setup — pass 2 (merge custom rule into the matched ruleset)"
  otelc setup "${setup_pkg_args[@]}"
fi

export GOFLAGS="${GOFLAGS} '-toolexec=otelc toolexec'"
echo "▶ GOFLAGS=${GOFLAGS}"
echo "▶ ${run_args[*]}"

mod_sum_before="$(hash_cmd go.mod go.sum)"

set +e
"${run_args[@]}"
build_status=$?
set -e

mod_sum_after="$(hash_cmd go.mod go.sum)"

if [ "$mod_sum_before" != "$mod_sum_after" ]; then
  echo "✗ Module Mutation Guard: go.mod/go.sum changed during the build step." >&2
  echo "  This is the silent dependency upgrade documented in pitfalls.md §1c." >&2
  diff <(echo "$mod_sum_before") <(echo "$mod_sum_after") >&2 || true
  git diff -- go.mod go.sum >&2 || true
  exit 1
fi

exit "$build_status"
