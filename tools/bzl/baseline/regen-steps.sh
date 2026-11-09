#!/usr/bin/env bash
# Task 0.1f (T6 baseline): manual regen-codegen steps per agent session.
#
# Scans the most recent N (default 30) top-level agent-session transcripts
# matching ~/.claude/projects/-home-tstapler-Programming-stapler-squad*/*.jsonl
# (by mtime; sub-agent transcripts live in subdirectories and are not
# matched). Per session it counts Bash tool_use commands that invoke
# `make proto-gen`, `make ent-gen` or `make sync`, plus tool results matching
# `gen/.* not found` or `undefined: .*sessionv1`. Prints median and p90 of
# (invocations + build-error hits) per session.
#
# Limits: all invocations are counted, including intentional ones; the plan's
# T6 wants only steps run to unblock a build, which a transcript cannot
# distinguish, so this is an upper bound (labelled `counts_all_invocations`).
# Two cohorts are printed: `all` (literal plan definition) and `build_active`
# (the most recent N sessions that ran at least one make/go build or test
# command), because the newest transcripts are often non-coding agent runs.
#
# usage: regen-steps.sh [--n 30] [--glob '<glob>']
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need jq
N=30
GLOB="$HOME/.claude/projects/-home-tstapler-Programming-stapler-squad*/*.jsonl"
while (($#)); do
  case "$1" in
    --n) N="$2"; shift 2 ;;
    --glob) GLOB="$2"; shift 2 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done

# shellcheck disable=SC2206
files=( $GLOB )
# shellcheck disable=SC2015  # the || arm only handles "no transcripts"; the exit 0 inside it cannot run when A is true
((${#files[@]} > 0)) && [[ -e "${files[0]}" ]] || { echo '{"regen_steps_per_session_median":"UNMEASURED: no transcripts found"}'; exit 0; }
mapfile -t sorted < <(ls -t "${files[@]}")

per_session() { # per_session <file>: prints one JSON line
  local f="$1" cmds errs
  cmds="$(grep -F '"name":"Bash"' "$f" | grep -E 'make' | nice -n 19 jq -r \
    'select(.type=="assistant") | .message.content[]? | select(.type=="tool_use" and .name=="Bash") | .input.command // empty' 2>/dev/null || true)"
  errs="$(grep -E 'gen/[^"]* not found|undefined: [^"]*sessionv1' "$f" | nice -n 19 jq -r \
    'select(.type=="user") | .message.content | if type=="array" then .[]? | select(.type=="tool_result") | (.content|if type=="array" then map(.text? // "")|join(" ") else (.//"") end) else empty end' 2>/dev/null \
    | grep -cE 'gen/.* not found|undefined: .*sessionv1' || true)"
  jq -c -n --arg f "$(basename "$f")" --arg cmds "$cmds" --argjson errs "${errs:-0}" '
    ($cmds|split("\n")|map(select(length>0))) as $c
    | {session:$f,
       proto_gen:($c|map(select(test("make[^&;|]* proto-gen")))|length),
       ent_gen:($c|map(select(test("make[^&;|]* ent-gen")))|length),
       sync:($c|map(select(test("make[^&;|]* sync($|[ &;|])")))|length),
       build_active:($c|map(select(test("make[^&;|]* (build|test|lint|ci|ready)")))|length > 0),
       build_errors:$errs}
    | .total = (.proto_gen+.ent_gen+.sync+.build_errors)'
}

# Cohort "all": the most recent N sessions; cohort "build_active": first N build-active ones.
all_rows=()
active_rows=()
for f in "${sorted[@]}"; do
  row="$(per_session "$f")"
  if ((${#all_rows[@]} < N)); then all_rows+=("$row"); fi
  if [[ "$(jq -r .build_active <<<"$row")" == true ]] && ((${#active_rows[@]} < N)); then active_rows+=("$row"); fi
  if ((${#all_rows[@]} >= N && ${#active_rows[@]} >= N)); then break; fi
done

summ() { jq -s '
  def pct($p): map(.total)|sort as $s | if ($s|length)==0 then null else $s[((($p*($s|length))|ceil)-1)|if .<0 then 0 else . end] end;
  def med: map(.total)|sort as $s | ($s|length) as $l | if $l==0 then null elif $l%2==1 then $s[($l-1)/2] else (($s[$l/2-1]+$s[$l/2])/2) end;
  { n:length, median:med, p90:pct(0.9), sessions_with_any:(map(select(.total>0))|length),
    totals:{proto_gen:(map(.proto_gen)|add), ent_gen:(map(.ent_gen)|add), sync:(map(.sync)|add), build_errors:(map(.build_errors)|add)} }'; }

a="$(printf '%s\n' "${all_rows[@]}" | summ)"
b="$(printf '%s\n' "${active_rows[@]}" | summ)"
jq -n --argjson a "$a" --argjson b "$b" --arg d "$(date -u +%FT%TZ)" --argjson total "${#sorted[@]}" '
  { run_date:$d, transcripts_matched:$total, counts_all_invocations:true,
    regen_steps_per_session_median:$a.median, regen_steps_per_session_p90:$a.p90,
    all:$a, build_active:$b }'
