#!/usr/bin/env bash
# Retry a command with exponential backoff. Use for CI/dev steps that hit
# external networks (e.g. `next/font` fetching from fonts.gstatic.com) where
# a transient failure shouldn't fail the whole build.
#
# Usage:
#   ./scripts/retry-with-backoff.sh <command> [args...]
#   ./scripts/retry-with-backoff.sh -n 5 -s 3 -- <command> [args...]
#
# Options:
#   -n  max attempts (default 3)
#   -s  base delay in seconds before the first retry; doubles each attempt (default 5)
set -euo pipefail

max_attempts=3
base_delay=5

while getopts ":n:s:" opt; do
  case "$opt" in
    n) max_attempts="$OPTARG" ;;
    s) base_delay="$OPTARG" ;;
    *) ;;
  esac
done
shift $((OPTIND - 1))
[ "${1:-}" = "--" ] && shift

if [ "$#" -eq 0 ]; then
  echo "usage: $0 [-n max_attempts] [-s base_delay] [--] <command> [args...]" >&2
  exit 2
fi

attempt=1
while true; do
  if "$@"; then
    exit 0
  fi
  if [ "$attempt" -ge "$max_attempts" ]; then
    echo "retry-with-backoff: '$*' failed after $attempt attempts" >&2
    exit 1
  fi
  delay=$((base_delay * (2 ** (attempt - 1))))
  echo "retry-with-backoff: attempt $attempt/$max_attempts failed, retrying in ${delay}s..." >&2
  sleep "$delay"
  attempt=$((attempt + 1))
done
