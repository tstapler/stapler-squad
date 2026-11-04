#!/usr/bin/env bash
# Task 11.1d: keep agent-facing Bazel docs honest about which commands have actually been run.
#   check-skill-proposals.sh [--docs FILE...]
# Checks the skill (default .claude/skills/bazel-stapler-squad/SKILL.md) and every --docs file:
#   1. each bazel/make command (skill: fenced line or inline `code`; docs: fenced lines only) is listed in verified-commands.txt or marked
#      proposal (same-line `<!-- proposal -->`, a `<!-- proposal -->` line right before the fence, or a
#      heading ending in "(proposal - not spike-verified)" for its whole section);
#   2. the dependency-add section keeps that heading suffix exactly while unverified (spike S11 Fail/absent,
#      or any of its commands unlisted) and loses it once verified;
#   3. each `<!-- errors.yaml:E<n> -->` quotation equals the row's hint text;
#   4. each errors.yaml row's Remedy: commands are verified unless the row is `status: proposal`, and a
#      proposal row whose commands are all verified fails (flip it to verified).
# Overrides for the test: SKILL_FILE, VERIFIED_FILE, ERRORS_YAML, SPIKE_RESULTS. bash 3.2 compatible.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../.." && pwd)
skill="${SKILL_FILE:-$root/.claude/skills/bazel-stapler-squad/SKILL.md}"
verified="${VERIFIED_FILE:-$here/verified-commands.txt}"
errors="${ERRORS_YAML:-$here/errors.yaml}"
spike="${SPIKE_RESULTS:-$root/project_plans/bazel-conversion/implementation/spike-results.yaml}"
export ERRORS_YAML="$errors"
PROPOSAL_SUFFIX='(proposal - not spike-verified)'

docs=()
if [ "${1:-}" = "--docs" ]; then shift; docs=("$@"); fi

fail=0
note() { echo "FAIL: $*" >&2; fail=1; }

verified_cmds=$(awk '/^[[:space:]]*(#|$)/ {next} {sub(/[[:space:]]+#.*$/,""); sub(/[[:space:]]+$/,""); print}' "$verified" 2>/dev/null)
is_verified() { printf '%s\n' "$verified_cmds" | grep -Fxq -- "$1"; }

# Prints "<line>\t<marked 0|1>\t<section 0|1>\t<command>" for each bazel/make command, where section is 1
# inside the dependency-add section (heading mentioning "dependency"), and a "H\t<line>\t<has-suffix>" row for it.
extract() { # <file> <inline 1|0>
  awk -v suffix="$PROPOSAL_SUFFIX" -v inline="$2" '
    function cmdstart(s) { return s ~ /^(bazel|make|bazelisk|tools\/bazel) / }
    function clean(s) {
      sub(/^[$][ ]+/, "", s)
      while (s ~ /^[A-Z_][A-Z0-9_]*=[^ ]+ /) sub(/^[A-Z_][A-Z0-9_]*=[^ ]+ /, "", s)
      sub(/[ ]*<!-- proposal -->.*$/, "", s)
      sub(/[ ]+#.*$/, "", s); sub(/[ ]+$/, "", s)
      return s
    }
    function emit(c, ln, mark) { printf "%d\t%d\t%d\t%s\n", ln, mark, indep, c }
    { line=$0 }
    /^```/ {
      if (!fence) { fence=1; fmark = (prev ~ /^<!-- proposal -->[ ]*$/) } else { fence=0 }
      prev=line; next
    }
    fence {
      c=clean(line); if (cmdstart(c)) emit(c, NR, (fmark || secmark || line ~ /<!-- proposal -->/))
      prev=line; next
    }
    /^#+ / {
      secmark = (index(line, suffix) > 0)
      indep = (tolower(line) ~ /dependency/)
      if (indep) printf "H\t%d\t%d\n", NR, secmark
      prev=line; next
    }
    inline {
      rest=line
      while (match(rest, /`[^`]+`/)) {
        span=substr(rest, RSTART+1, RLENGTH-2); rest=substr(rest, RSTART+RLENGTH)
        c=clean(span); if (cmdstart(c)) emit(c, NR, (secmark || line ~ /<!-- proposal -->/))
      }
      prev=line
      next
    }
    { prev=line }' "$1"
}

s11_verdict() {
  awk '/^- spike:/ {on=($3=="S11")} on && /^  verdict:/ {print $2; exit}' "$spike" 2>/dev/null
}

check_file() {
  local f="$1" inline="$2" n mark insec cmd unverified_in_section=0 hline="" hmark=0
  [ -f "$f" ] || { note "$f: file not found"; return; }
  while IFS=$'\t' read -r a b c d; do
    if [ "$a" = "H" ]; then hline="$b"; hmark="$c"; continue; fi
    n="$a"; mark="$b"; insec="$c"; cmd="$d"
    if is_verified "$cmd"; then
      continue
    fi
    [ "$insec" = 1 ] && unverified_in_section=1
    if [ "$mark" != 1 ]; then
      note "$f:$n: command is neither in verified-commands.txt nor marked <!-- proposal -->: $cmd"
    fi
  done < <(extract "$f" "$inline")

  if [ -n "$hline" ]; then
    local verdict; verdict=$(s11_verdict)
    local unverified=$unverified_in_section
    case "$verdict" in ""|Fail) unverified=1 ;; esac
    if [ "$unverified" = 1 ] && [ "$hmark" != 1 ]; then
      note "$f:$hline: dependency-add heading must end with '$PROPOSAL_SUFFIX' while unverified (S11 verdict '${verdict:-none}')"
    fi
    if [ "$unverified" = 0 ] && [ "$hmark" = 1 ]; then
      note "$f:$hline: dependency-add heading still says '$PROPOSAL_SUFFIX' but S11 passed and every command is verified; remove it"
    fi
  fi

  # errors.yaml quotations: the next non-empty line after the marker must equal the hint text.
  while IFS=$'\t' read -r ln id; do
    want=$(bash "$here/translate-errors.sh" --text "$id")
    got=$(awk -v s="$ln" 'NR>s && NF {print; exit}' "$f")
    if [ -z "$want" ]; then note "$f:$ln: errors.yaml has no hint for $id"
    elif [ "$got" != "$want" ]; then note "$f:$ln: quotation of $id differs from errors.yaml"; fi
  done < <(awk '{ if (match($0, /<!-- errors[.]yaml:E[0-9]+ -->/)) { t=substr($0,RSTART,RLENGTH); sub(/.*yaml:/,"",t); sub(/ .*/,"",t); print NR "\t" t } }' "$f")
}

check_errors_table() {
  local id status cmds cmd allverified any hint
  while read -r id; do
    status=$(awk -v id="$id" '/^- id:/ {on=($3==id)} on && /^  status:/ {print $2; exit}' "$errors")
    hint=$(bash "$here/translate-errors.sh" --text "$id")
    cmds=$(printf '%s\n' "${hint#*Remedy:}" | grep -o "'[^']*'" | sed "s/^'//;s/'\$//" | sed 's/ && /\n/g' | grep -E '^(bazel|make|bazelisk) ' || true)
    [ -n "$cmds" ] || continue
    allverified=1; any=0
    while IFS= read -r cmd; do
      [ -n "$cmd" ] || continue
      any=1
      is_verified "$cmd" || allverified=0
    done <<EOF
$cmds
EOF
    [ "$any" = 1 ] || continue
    if [ "$status" = proposal ] && [ "$allverified" = 1 ]; then
      note "$errors: $id is status: proposal but every Remedy command is verified; mark it verified"
    elif [ "$status" != proposal ] && [ "$allverified" = 0 ]; then
      note "$errors: $id is not status: proposal but a Remedy command is not in verified-commands.txt"
    fi
  done < <(awk '/^- id:/ {print $3}' "$errors")
}

[ -f "$skill" ] && check_file "$skill" 1
for d in ${docs[@]+"${docs[@]}"}; do check_file "$d" 0; done
check_errors_table

if [ "$fail" = 0 ]; then echo PASS; else echo FAIL >&2; fi
exit "$fail"
