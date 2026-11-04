#!/usr/bin/env bash
# Seeds fixtures for check-skill-proposals.sh; prints PASS/FAIL per case and exits 1 if any case misbehaves.
# shellcheck disable=SC2016  # fixtures hold literal backticks
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/skillprop-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
failures=0
cp "$here/errors.yaml" "$T/errors.yaml"
printf 'bazel build //:stapler-squad  # run log: fixture\n' > "$T/verified.txt"
printf -- '- spike: S11\n  verdict: Pass\n' > "$T/spike.yaml"

# run <label> <want-exit 0|1> <skill-body> [docs-body]
run() {
  local label=$1 want=$2 body=$3 docbody=${4:-} rc args=()
  printf '%s\n' "$body" > "$T/SKILL.md"
  if [ -n "$docbody" ]; then printf '%s\n' "$docbody" > "$T/doc.md"; args=(--docs "$T/doc.md"); fi
  SKILL_FILE="$T/SKILL.md" VERIFIED_FILE="$T/verified.txt" ERRORS_YAML="$T/errors.yaml" SPIKE_RESULTS="$T/spike.yaml" \
    bash "$here/check-skill-proposals.sh" ${args[@]+"${args[@]}"} >"$T/out" 2>&1
  rc=$?
  if [ "$rc" = "$want" ]; then echo "PASS - $label"; else echo "FAIL - $label (exit $rc, wanted $want)"; sed 's/^/    /' "$T/out"; failures=$((failures + 1)); fi
}

run "verified command" 0 '`bazel build //:stapler-squad`'
run "marked unverified command" 0 '`make gazelle` <!-- proposal -->'
run "unmarked unverified command" 1 '`make gazelle`'
run "fenced unmarked command" 1 $'```\nbazel test //x\n```'
run "fenced block marked on the line before" 0 $'<!-- proposal -->\n```\nbazel test //x\n```'
run "docs fenced command unlisted and unmarked" 1 'nothing' $'```bash\nbazel test //x\n```'
run "docs fenced command marked" 0 'nothing' $'<!-- proposal -->\n```bash\nbazel test //x\n```'
run "docs inline mention is not checked" 0 'nothing' 'Prose may say `bazel test //x`.'

hint=$(bash "$here/translate-errors.sh" --text E1)
run "errors.yaml quotation equal" 0 $'<!-- errors.yaml:E1 -->\n'"$hint"
run "errors.yaml quotation differs" 1 $'<!-- errors.yaml:E1 -->\nbazel-hint: run make gazelle somehow'

dep='## Add a Go dependency '
run "dependency heading marked while unverified" 0 "$dep(proposal - not spike-verified)"$'\n```\nbazel mod tidy\n```'
run "dependency heading unmarked while unverified" 1 "$dep"$'\n`bazel mod tidy` <!-- proposal -->'
printf 'bazel mod tidy  # run log: fixture\n' >> "$T/verified.txt"
run "dependency heading still marked after verification" 1 "$dep(proposal - not spike-verified)"$'\n`bazel mod tidy`'
run "dependency heading unmarked after verification" 0 "$dep"$'\n`bazel mod tidy`'

# errors.yaml rule (c): a non-proposal row with an unverified Remedy command fails.
awk '/^- id:/ {e2=($3=="E2")} e2 && /^  status:/ {print "  status: verified"; next} {print}' "$T/errors.yaml" > "$T/errors2.yaml"
ERRORS_YAML="$T/errors2.yaml" SKILL_FILE="$T/none" VERIFIED_FILE="$T/verified.txt" SPIKE_RESULTS="$T/spike.yaml" \
  bash "$here/check-skill-proposals.sh" >"$T/out" 2>&1
if [ $? = 1 ] && grep -q 'not status: proposal' "$T/out"; then echo "PASS - verified row with unverified remedy"; else echo "FAIL - verified row with unverified remedy"; failures=$((failures + 1)); fi

exit $((failures > 0))
