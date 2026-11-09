#!/usr/bin/env bash
# Tests tools/ci/check-tymux-pin.sh against a local bare git repo (no network).
# Usage: tools/ci/check-tymux-pin_test.sh   (also: make test-tymux-pin)
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$here/check-tymux-pin.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

module="github.com/tstapler/tymux/clients/go"
failures=0
cases=0

git_q() { git -c user.name=t -c user.email=t@example.com -c commit.gpgsign=false -c tag.gpgsign=false "$@"; }

# Fixture remote: v1.0.0 (annotated, with clients/go/v1.0.0) on commit A; v2.0.0 on commit B.
src="$work/src"
remote="$work/remote.git"
git init -q "$src"
git_q -C "$src" commit -q --allow-empty -m a
commit_a="$(git -C "$src" rev-parse HEAD)"
git_q -C "$src" tag -a v1.0.0 -m v1.0.0
git_q -C "$src" tag -a clients/go/v1.0.0 -m client
git_q -C "$src" commit -q --allow-empty -m b
commit_b="$(git -C "$src" rev-parse HEAD)"
git_q -C "$src" tag -a v2.0.0 -m v2.0.0
git clone -q --bare "$src" "$remote"

# make_tree <name> <Makefile text> <go.mod pin or "">: a repo root holding a copy of the script.
make_tree() {
  local dir="$work/$1"
  mkdir -p "$dir/tools/ci"
  cp "$script" "$dir/tools/ci/check-tymux-pin.sh"
  printf '%s\n' "$2" >"$dir/Makefile"
  printf 'module example.com/x\n\nrequire (\n%s\n)\n' "$3" >"$dir/go.mod"
  echo "$dir"
}

# run_case <description> <tree> <want exit> <want stderr/stdout substring> <remote> [CI value]
run_case() {
  local desc="$1" tree="$2" want_rc="$3" want_msg="$4" rem="$5" ci="${6:-}" out rc
  cases=$((cases + 1))
  if [[ -n "$ci" ]]; then
    out="$(env CI="$ci" TYMUX_REMOTE="$rem" "$tree/tools/ci/check-tymux-pin.sh" 2>&1)"
  else
    out="$(env -u CI TYMUX_REMOTE="$rem" "$tree/tools/ci/check-tymux-pin.sh" 2>&1)"
  fi
  rc=$?
  if [[ "$rc" -ne "$want_rc" || "$out" != *"$want_msg"* ]]; then
    failures=$((failures + 1))
    echo "FAIL: $desc (exit $rc, want $want_rc; want output containing: $want_msg)" >&2
    printf '    %s\n' "${out//$'\n'/$'\n    '}" >&2
  else
    echo "ok: $desc"
  fi
}

pin() { printf '\t%s %s' "$module" "$1"; }
pseudo_a="v1.0.1-0.20260101000000-${commit_a:0:12}"
pseudo_b="v2.0.1-0.20260102000000-${commit_b:0:12}"

run_case "pseudo-version pin matches daemon tag" \
  "$(make_tree pseudo 'TYMUX_VERSION ?= v1.0.0' "$(pin "$pseudo_a")")" 0 "tymux pin consistent" "$remote"

run_case "release-tag pin matches daemon tag" \
  "$(make_tree release 'TYMUX_VERSION ?= v1.0.0' "$(pin v1.0.0)")" 0 "tymux pin consistent" "$remote"

run_case "pseudo-version pin from a different commit exits 1" \
  "$(make_tree mismatch_pseudo 'TYMUX_VERSION ?= v1.0.0' "$(pin "$pseudo_b")")" 1 "go.mod pins the client at" "$remote"

run_case "release pin vs different daemon tag exits 1" \
  "$(make_tree mismatch_release 'TYMUX_VERSION ?= v2.0.0' "$(pin v1.0.0)")" 1 "go.mod pins the client at" "$remote"

run_case "release pin without a clients/go tag exits 1" \
  "$(make_tree no_client_tag 'TYMUX_VERSION ?= v1.0.0' "$(pin v9.9.9)")" 1 "has no clients/go/v9.9.9 tag" "$remote"

run_case "daemon tag missing from the remote exits 1" \
  "$(make_tree no_daemon_tag 'TYMUX_VERSION ?= v9.9.9' "$(pin v1.0.0)")" 1 "is not a tag on" "$remote"

run_case "missing TYMUX_VERSION exits 1" \
  "$(make_tree no_version 'OTHER ?= 1' "$(pin v1.0.0)")" 1 "TYMUX_VERSION not found in Makefile" "$remote"

run_case "missing go.mod client require exits 1" \
  "$(make_tree no_pin 'TYMUX_VERSION ?= v1.0.0' '')" 1 "not found in go.mod" "$remote"

unreachable="$work/does-not-exist.git"
unreachable_tree="$(make_tree unreachable 'TYMUX_VERSION ?= v1.0.0' "$(pin v1.0.0)")"

run_case "unreachable remote warns and passes outside CI" \
  "$unreachable_tree" 0 "skipped, cannot reach" "$unreachable"

run_case "unreachable remote fails with CI=true" \
  "$unreachable_tree" 1 "cannot reach" "$unreachable" true

# --- Task 3.3d: MODULE.bazel / PIN / checksums / gitlink cross-checks (offline: remote unreachable). ---
repo_root="$(cd "$here/../.." && pwd)"
# Derived from the repo so a TYMUX_VERSION bump does not stale the mutations below.
daemon_tag="$(sed -nE 's/^TYMUX_VERSION \?= (v[^[:space:]]+).*/\1/p' "$repo_root/Makefile" | head -1)"
first_sha_prefix="$(awk '/name = "tymuxd_/ { f = 1 } f && /sha256 = "/ { split($0, a, "\""); print substr(a[2], 1, 8); exit }' "$repo_root/MODULE.bazel")"

# make_pin_tree <name>: a git repo with the real MODULE.bazel, PIN, checksums, Makefile pin line and go.mod
# client pin, plus a third_party/tmux gitlink equal to the PIN commit. Callers then mutate one file.
make_pin_tree() {
  local dir="$work/$1" commit
  mkdir -p "$dir/tools/ci" "$dir/tools/bzl/tmux" "$dir/scripts"
  cp "$script" "$dir/tools/ci/check-tymux-pin.sh"
  cp "$repo_root/MODULE.bazel" "$dir/MODULE.bazel"
  cp "$repo_root/tools/bzl/tmux/PIN" "$dir/tools/bzl/tmux/PIN"
  cp "$repo_root/scripts/tymuxd-checksums.txt" "$dir/scripts/tymuxd-checksums.txt"
  grep -E '^TYMUX_VERSION \?=' "$repo_root/Makefile" >"$dir/Makefile"
  printf 'module example.com/x\n\nrequire (\n\t%s %s\n)\n' "$module" \
    "$(awk -v m="$module" '$1 == m { print $2; exit }' "$repo_root/go.mod")" >"$dir/go.mod"
  commit="$(sed -n 's/^commit=//p' "$dir/tools/bzl/tmux/PIN")"
  git init -q "$dir"
  git -C "$dir" update-index --add --cacheinfo "160000,$commit,third_party/tmux"
  git_q -C "$dir" add -- Makefile go.mod MODULE.bazel scripts tools
  git_q -C "$dir" commit -q -m fixture
  echo "$dir"
}

amend() { git_q -C "$1" add -- Makefile go.mod MODULE.bazel scripts tools; git_q -C "$1" commit -q --amend --no-edit; }

run_case "pins consistent across MODULE.bazel, PIN, checksums and gitlink" \
  "$(make_pin_tree pins_ok)" 0 "skipped, cannot reach" "$unreachable"

t="$(make_pin_tree mod_tymuxd_sha)"
sed -i "0,/sha256 = \"$first_sha_prefix/s//sha256 = \"00000000/" "$t/MODULE.bazel"; amend "$t"
run_case "mutated tymuxd sha256 in MODULE.bazel fails" "$t" 1 "differs from" "$unreachable"

t="$(make_pin_tree mod_tmux_sha)"
sed -i 's/sha256 = "e79c699c/sha256 = "00000000/' "$t/MODULE.bazel"; amend "$t"
run_case "mutated tmux_src sha256 in MODULE.bazel fails" "$t" 1 "tmux_src sha256" "$unreachable"

t="$(make_pin_tree pin_sha)"
sed -i 's/^sha256=e79c699c/sha256=00000000/' "$t/tools/bzl/tmux/PIN"; amend "$t"
run_case "mutated PIN sha256 fails" "$t" 1 "tmux_src sha256" "$unreachable"

t="$(make_pin_tree pin_commit)"
sed -i 's/^commit=7f2a35ad/commit=00000000/' "$t/tools/bzl/tmux/PIN"; amend "$t"
run_case "PIN commit differing from the gitlink fails" "$t" 1 "gitlink" "$unreachable"

t="$(make_pin_tree checksums_line)"
sed -i "/^$daemon_tag x86_64-unknown-linux-musl/d" "$t/scripts/tymuxd-checksums.txt"; amend "$t"
run_case "missing checksums line for a MODULE.bazel tymuxd repo fails" "$t" 1 "has no" "$unreachable"

t="$(make_pin_tree url_tag)"
sed -i "s#download/$daemon_tag/tymux-aarch64-apple#download/v0.0.1/tymux-aarch64-apple#" "$t/MODULE.bazel"; amend "$t"
run_case "MODULE.bazel url for a different release fails" "$t" 1 "is not release" "$unreachable"

echo "$((cases - failures))/$cases passed"
[[ "$failures" -eq 0 ]]
