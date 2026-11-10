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

# --- Docs-derived rules (D1, D20, D21, E9) ---
for bad in local_ram_resources local_cpu_resources incompatible_sandbox_hermetic_tmp experimental_guard_against_concurrent_changes; do
  if rcbody "${rcfiles[@]}" | grep -nE -- "--${bad}([^a-z_]|$)"; then
    fail "--${bad} does not exist on the pinned Bazel (verified absent)"
  fi
done
if rcbody "${rcfiles[@]}" | grep -nE -- '--digest_function'; then
  fail "--digest_function changes every cache key; every cache sharer must keep the default"
fi
if rcbody "${rcfiles[@]}" | grep -nE -- '--action_env=PATH([[:space:]]|$)'; then
  fail "--action_env=PATH changes the action key per user/worktree"
fi
if rcbody "${rcfiles[@]}" | grep -nE -- '--profile='; then
  fail "--profile=<path> in a committed rc disables Bazel's automatic profile GC; pass it per CI job"
fi
if rcbody "${rcfiles[@]}" | grep -qE -- '--remote_local_fallback([^_a-z]|$)' \
   && ! rcbody "${rcfiles[@]}" | grep -qE -- '--incompatible_remote_local_fallback_for_remote_cache'; then
  fail "--remote_local_fallback is a no-op for a cache-only setup without --incompatible_remote_local_fallback_for_remote_cache"
fi

# build:agent / test:agent is presentation only (a changed analysis-affecting flag discards the cache)
agent_allow='color|curses|show_progress|show_loading_progress|verbose_failures|test_output|test_summary'
while IFS= read -r flag; do
  n=${flag#--}; n=${n#no}
  case "$n" in
    color|curses|show_progress|show_loading_progress|verbose_failures|test_output|test_summary) ;;
    *) fail "$flag in build:agent/test:agent is outside the presentation allow-list ($agent_allow)" ;;
  esac
done < <(rcbody "${rcfiles[@]}" | grep -E '^[[:space:]]*(build|test|common):agent[[:space:]]' | grep -oE -- '(^|[[:space:]])--[a-z_]+' | tr -d ' \t')

# ci-cache-read and ci-cache-write carry identical failure-policy lines
policy() {
  rcbody "${rcfiles[@]}" | grep -E "^[[:space:]]*(build|common):$1[[:space:]]" \
    | grep -E -- '--remote_timeout|--remote_retries|--experimental_circuit_breaker|--experimental_remote_failure|--remote_cache_compression|--remote_download' \
    | sed -E 's/^[[:space:]]*[a-z]+:[a-z-]+[[:space:]]+//' | sort
}
[ "$(policy ci-cache-read)" = "$(policy ci-cache-write)" ] \
  || fail "ci-cache-read and ci-cache-write failure-policy lines differ"

# every flag must exist in `bazel help <command> --long` for the pinned version (cached in .cache/).
# BZL_CHECK_HELP_DIR=<dir with build.txt and startup.txt> replaces the live probe (tests).
help_file() { # help_file <build|startup_options>
  local cmd=$1 ver cache
  if [ -n "${BZL_CHECK_HELP_DIR:-}" ]; then
    [ -f "$BZL_CHECK_HELP_DIR/$cmd.txt" ] && { echo "$BZL_CHECK_HELP_DIR/$cmd.txt"; return 0; }
    return 1
  fi
  ver=$(cat .bazelversion 2>/dev/null) || return 1
  cache=".cache/bazel-help-$ver-$cmd.txt"
  if [ ! -s "$cache" ]; then
    command -v bazel >/dev/null 2>&1 || return 1
    mkdir -p .cache
    # shellcheck disable=SC2015  # intentional: any failure of help or mv removes the temp file
    bazel help "$cmd" --long > "$cache.tmp" 2>/dev/null && mv "$cache.tmp" "$cache" || { rm -f "$cache.tmp"; return 1; }
  fi
  echo "$cache"
}
if [ -z "${BZL_CHECK_SKIP_HELP:-}" ]; then
  while IFS=' ' read -r cmd flag; do
    [ -n "$flag" ] || continue
    case "$cmd" in
      startup*) helpcmd=startup_options ;;
      try-import|import) continue ;;
      *) helpcmd=build ;;
    esac
    hf=$(help_file "$helpcmd") || fail "cannot read 'bazel help $helpcmd --long': flag existence cannot be evaluated (treated as failure)"
    n=${flag#--}
    grep -qE -- "^ *--(\[no\])?${n}( |\$)" "$hf" \
      || { n2=${n#no}; grep -qE -- "^ *--\[no\]${n2}( |\$)" "$hf"; } \
      || fail "$flag ($cmd) is not in 'bazel help $helpcmd --long' for $(cat .bazelversion 2>/dev/null)"
  done < <(rcbody "${rcfiles[@]}" | awk '{c=$1; sub(/:.*/,"",c); for(i=2;i<=NF;i++){f=$i; sub(/=.*/,"",f); if (f ~ /^--[a-z]/) print c, f}}' | sort -u)
fi

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
