#!/usr/bin/env bash
# Shared helpers for tools/bzl/baseline/*.sh (source, do not execute).

BZL_REPO="${BZL_REPO:-tstapler/stapler-squad}"
BZL_BASELINE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BZL_REPO_ROOT="${BZL_REPO_ROOT:-$(cd "$BZL_BASELINE_DIR/../../.." && pwd)}"
# Cache lives under the git dir, which git never tracks or lists.
BZL_CACHE_DIR="${BZL_CACHE_DIR:-$(git -C "$BZL_REPO_ROOT" rev-parse --absolute-git-dir)/bzl-baseline-cache}"
BZL_CACHE_TTL_S="${BZL_CACHE_TTL_S:-3600}"
BZL_MAX_LOAD1=2.0 # fixed on purpose: the limit itself has no override; only BZL_ALLOW_NOISY_HOST=1 skips the wait, loudly

die() { echo "error: $*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing required tool: $1"; }

loadavg1() { cut -d' ' -f1 /proc/loadavg; }

# require_quiet_host: refuse to start a heavy measurement while the 1-minute
# load average exceeds BZL_MAX_LOAD1 (default 2.0). There is deliberately no
# bypass flag; lower the load instead. The one exception is the owner-authorised
# BZL_ALLOW_NOISY_HOST=1: above the limit it warns loudly, exports BZL_NOT_QUIET=1
# and BZL_LOAD1_AT_START, and every JSON the caller writes must carry them (not_quiet).
require_quiet_host() {
  local l
  l="$(loadavg1)"
  export BZL_LOAD1_AT_START="$l" BZL_NOT_QUIET=0
  if awk -v l="$l" -v m="$BZL_MAX_LOAD1" 'BEGIN{exit !(l>m)}'; then
    if [[ "${BZL_ALLOW_NOISY_HOST:-}" == "1" ]]; then
      echo "WARNING: BZL_ALLOW_NOISY_HOST=1: 1-minute load $l exceeds $BZL_MAX_LOAD1; results are NOT QUIET-HOST numbers (owner-authorised)" >&2
      export BZL_NOT_QUIET=1
      return 0
    fi
    echo "refusing to start: 1-minute load average $l exceeds $BZL_MAX_LOAD1 (host is shared; wait for it to go quiet)" >&2
    exit 3
  fi
}

# gh_cached <cache-key> <gh api args...>: prints the JSON body, cached for
# BZL_CACHE_TTL_S seconds so reruns do not spend API quota.
gh_cached() {
  local key="$1"; shift
  local f="$BZL_CACHE_DIR/$key.json" now mt
  mkdir -p "$BZL_CACHE_DIR"
  now="$(date +%s)"
  if [[ -s "$f" ]]; then
    mt="$(stat -c %Y "$f")"
    if (( now - mt < BZL_CACHE_TTL_S )); then cat "$f"; return 0; fi
  fi
  gh api "$@" > "$f.tmp" || { rm -f "$f.tmp"; return 1; }
  mv "$f.tmp" "$f"
  cat "$f"
}
