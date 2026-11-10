#!/usr/bin/env bash
# Task 0.1e: record the current Go test pass set into baseline-test-set.json.
#
# Runs `make test`'s two gotestsum invocations (same flags and package split as
# the Makefile recipe) with --jsonfile in a throwaway worktree of HEAD, after
# `make proto-gen ent-gen ./bin/tmux` there (generated code and tmux are
# prerequisites, as for `make test`). Then writes the sorted unique
# `Package/Test` names that passed and, separately, those that failed
# (pre-existing failures / BUG-087..090 flakes). The file is the Epic 0
# reference only; P1 compares live runs (Task 4.8a).
#
# Heavy (a full test run): refuses while the 1-minute load average exceeds 2.0,
# no override. --dry-run is cheap.
# usage: test-set.sh [--out <json>] [--dry-run]
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
OUT="$BZL_REPO_ROOT/project_plans/bazel-conversion/implementation/baseline-test-set.json" DRY=0
while (($#)); do
  case "$1" in
    --out) OUT="$2"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done
need jq; need git; need go
if ((DRY)); then
  jq -n --arg out "$OUT" --arg l "$(loadavg1)" '{dry_run:true, out:$out, loadavg1_now:($l|tonumber), max_loadavg1:2.0,
    steps:["require_quiet_host","git worktree add --detach <tmp>/wt HEAD","make proto-gen ent-gen bin/tmux",
           "gotestsum --jsonfile (session,session/mux,session/tmux,testutil with -p 1)","gotestsum --jsonfile (rest)","write JSON"]}'
  exit 0
fi
command -v gotestsum >/dev/null || die "gotestsum not installed (make test auto-installs it)"
require_quiet_host

TMP="$(mktemp -d /tmp/bzlts.XXXXXX)"
trap 'git -C "$BZL_REPO_ROOT" worktree remove --force "$TMP/wt" 2>/dev/null || true; rm -rf "$TMP"; git -C "$BZL_REPO_ROOT" worktree prune' EXIT
git -C "$BZL_REPO_ROOT" worktree add --quiet --detach "$TMP/wt" HEAD
cd "$TMP/wt"
unset npm_config_user_agent
make proto-gen ent-gen bin/tmux >"$TMP/prep.log" 2>&1 || die "prep failed; see $TMP/prep.log"
export STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30 TMUX_BIN="$PWD/bin/tmux"
set +e
gotestsum --jsonfile "$TMP/a.json" --format pkgname-and-test-fails -- -short -timeout=20m -p 1 ./session ./session/mux ./session/tmux ./testutil >"$TMP/a.log" 2>&1
# shellcheck disable=SC2046
gotestsum --jsonfile "$TMP/b.json" --format pkgname-and-test-fails -- -short -timeout=20m \
  $(go list ./... | grep -vE '^github\.com/tstapler/stapler-squad/(session|session/mux|session/tmux|testutil)$') >"$TMP/b.log" 2>&1
set -e
cat "$TMP/a.json" "$TMP/b.json" | jq -s --arg head "$(git rev-parse --short HEAD)" --arg d "$(date -u +%FT%TZ)" '
  map(select(.Test != null and (.Action=="pass" or .Action=="fail")))
  | { head:$head, run_date:$d,
      passed:(map(select(.Action=="pass")|"\(.Package)/\(.Test)")|unique|sort),
      failed:(map(select(.Action=="fail")|"\(.Package)/\(.Test)")|unique|sort) }
  | .passed_count=(.passed|length) | .failed_count=(.failed|length)
  | .not_quiet=($nq=="1") | .load1_at_start=($l0|tonumber) | .load1_at_end=($l1|tonumber)' \
  --arg nq "${BZL_NOT_QUIET:-0}" --arg l0 "${BZL_LOAD1_AT_START:-0}" --arg l1 "$(loadavg1)" > "$OUT"
jq -c '{head,passed_count,failed_count}' "$OUT"
