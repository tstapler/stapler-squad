#!/usr/bin/env bash
# Retry a command with exponential backoff. Use for CI/dev steps that hit
# external networks (e.g. `next/font` fetching from fonts.gstatic.com) where
# a transient failure shouldn't fail the whole build.
#
# By default any non-zero exit is retried. Pass -c to only retry specific
# exit codes (e.g. curl's 6=couldn't resolve host, 7=couldn't connect,
# 28=timeout) and fail fast on everything else -- useful for tools whose
# exit code actually distinguishes "transient" from "this will never work",
# unlike e.g. `pnpm run build`, which exits 1 for both a flaky font fetch
# and a real compile error.
#
# Usage:
#   ./scripts/retry-with-backoff.sh <command> [args...]
#   ./scripts/retry-with-backoff.sh -n 5 -s 3 -c 6,7,28 -- curl ...
#   RETRY_EXIT_CODES=6,7,28 ./scripts/retry-with-backoff.sh curl ...
#
# Options:
#   -n  max attempts (default 3)
#   -s  base delay in seconds before the first retry; doubles each attempt (default 5)
#   -c  comma/space-separated exit codes to retry (default: RETRY_EXIT_CODES env
#       var, or retry on any non-zero exit if neither is set)
set -euo pipefail

max_attempts=3
base_delay=5
retry_codes="${RETRY_EXIT_CODES:-}"

while getopts ":n:s:c:" opt; do
  case "$opt" in
    n) max_attempts="$OPTARG" ;;
    s) base_delay="$OPTARG" ;;
    c) retry_codes="$OPTARG" ;;
    *) ;;
  esac
done
shift $((OPTIND - 1))
[ "${1:-}" = "--" ] && shift

if [ "$#" -eq 0 ]; then
  echo "usage: $0 [-n max_attempts] [-s base_delay] [-c exit_codes] [--] <command> [args...]" >&2
  exit 2
fi

is_retryable() {
  local code="$1"
  [ -z "$retry_codes" ] && return 0 # no allowlist: retry on any failure
  local c
  for c in ${retry_codes//,/ }; do
    [ "$c" = "$code" ] && return 0
  done
  return 1
}

attempt=1
while true; do
  "$@" && exit 0
  code=$?
  if ! is_retryable "$code"; then
    echo "retry-with-backoff: '$*' exited $code, not in retryable set ($retry_codes) -- failing fast" >&2
    exit "$code"
  fi
  if [ "$attempt" -ge "$max_attempts" ]; then
    echo "retry-with-backoff: '$*' failed after $attempt attempts (exit $code)" >&2
    exit "$code"
  fi
  delay=$((base_delay * (2 ** (attempt - 1))))
  echo "retry-with-backoff: attempt $attempt/$max_attempts failed (exit $code), retrying in ${delay}s..." >&2
  sleep "$delay"
  attempt=$((attempt + 1))
done
