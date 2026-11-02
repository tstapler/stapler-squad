#!/usr/bin/env bash
# Guardrails for the Bazel config (validation G5, pre-mortem #3/#13). Exits 1 naming the first
# offender. BZL_CHECK_ROOT overrides the tree to check (tests); BZL_CHECK_SKIP_MOD_GRAPH=1 skips
# the `bazel mod graph` probe.
set -u
root="${BZL_CHECK_ROOT:-$(git rev-parse --show-toplevel)}"
cd "$root" || exit 2
fail() { echo "check-bazelrc: $1" >&2; exit 1; }

for f in WORKSPACE WORKSPACE.bazel; do
  [ ! -e "$f" ] || fail "$f exists (bzlmod only; no WORKSPACE)"
done

rcfiles=()
for f in .bazelrc .bazelrc.* ; do
  [ -f "$f" ] || continue
  case "$f" in .bazelrc.user.example) ;; *) rcfiles+=("$f") ;; esac
done
[ "${#rcfiles[@]}" -gt 0 ] || fail "no .bazelrc"

# startup:<config> lines are silently ignored by Bazel (only platform configs apply)
if grep -nE '^[[:space:]]*startup:' "${rcfiles[@]}" | grep -vE 'startup:(linux|macos|windows|freebsd|openbsd)[[:space:]]'; then
  fail "startup:<config> line is ignored by Bazel; use an unconditional startup line or the tools/bazel wrapper"
fi

# strip comments
rcbody() { sed 's/[[:space:]]*#.*$//' "$@"; }

if rcbody "${rcfiles[@]}" | grep -nE -- '--enable_workspace|--enable_bzlmod=(false|0|no)|--noenable_bzlmod'; then
  fail "workspace mode flag in .bazelrc (above)"
fi
if rcbody "${rcfiles[@]}" | grep -nE -- '--spawn_strategy=local|--strategy=[A-Za-z]*=local'; then
  fail "--spawn_strategy=local is not allowed in committed rc files (breaks sandboxing)"
fi
if rcbody "${rcfiles[@]}" | grep -nE -- '--remote_exec(utor)?\b'; then
  fail "remote execution flag (no RBE)"
fi
if rcbody "${rcfiles[@]}" | grep -niE -- '--remote_header[= ].*(authorization|token|secret|passw|bearer|api[_-]?key|basic )'; then
  fail "--remote_header carries a secret; use --credential_helper"
fi
attempts=$(rcbody "${rcfiles[@]}" | grep -oE -- '--flaky_test_attempts=[0-9]+' | cut -d= -f2 | sort -n | tail -1)
if [ -n "${attempts:-}" ] && [ "$attempts" -gt 1 ]; then
  fail "--flaky_test_attempts=$attempts (retrying hides flakes)"
fi

# flaky = True in any BUILD/.bzl file
hit=$(git ls-files -co --exclude-standard -- '*BUILD' '*BUILD.bazel' '*.bzl' 2>/dev/null \
  | grep -v '^\.claude/worktrees/' \
  | while IFS= read -r f; do grep -Hn -E 'flaky[[:space:]]*=[[:space:]]*True' "$f" 2>/dev/null; done | head -1)
[ -z "$hit" ] || fail "flaky = True: $hit"

# every flag in the rc files needs a spike-verified row (Task 0.2b) in spike-results.md
results=project_plans/bazel-conversion/implementation/spike-results.md
if [ -f "$results" ]; then
  while IFS= read -r flag; do
    name=${flag#--}; name=${name#no}
    grep -qE "^\| --(no)?${name} \| spike-verified \|" "$results" \
      || grep -qE "^\| --\[no\]${name} \| spike-verified \|" "$results" \
      || fail "$flag has no 'spike-verified' row in $results"
  done < <(rcbody "${rcfiles[@]}" | grep -oE -- '(^|[[:space:]])--[a-z_]+' | tr -d ' \t' | sort -u)
elif [ -z "${BZL_CHECK_NO_RESULTS_OK:-}" ]; then
  fail "$results missing: cannot evaluate flag verification (treated as failure)"
fi

if [ -z "${BZL_CHECK_SKIP_MOD_GRAPH:-}" ]; then
  command -v bazel >/dev/null 2>&1 || fail "bazel not found: cannot evaluate 'bazel mod graph' (treated as failure)"
  bazel mod graph >/dev/null 2>&1 || fail "bazel mod graph failed"
fi
exit 0
