#!/usr/bin/env bash
# Fails the commit when staged Go/proto/module changes leave BUILD files stale.
# Runs `make gazelle` (override with GAZELLE_CMD) only when a staged change can alter BUILD files:
# added/deleted/renamed .go or .proto, a modified .go whose diff touches an import, or go.mod/go.sum/MODULE.bazel.
set -u
cd "$(git rev-parse --show-toplevel)" || exit 2

relevant() {
  local status path
  while IFS=$'\t' read -r status path _; do
    case "$path" in
      go.mod | go.sum | MODULE.bazel) return 0 ;;
      *.proto) return 0 ;;
      *.go)
        case "$status" in
          M*) git diff --cached -U0 -- "$path" | grep -Eq '^[+-][[:space:]]*(import[[:space:]]|([[:alnum:]_.]+[[:space:]]+)?"[^"]+"[[:space:]]*$)' && return 0 ;;
          *) return 0 ;;
        esac
        ;;
    esac
  done < <(git diff --cached --name-status --diff-filter=ACDMR)
  return 1
}

relevant || exit 0

cmd=${GAZELLE_CMD:-make gazelle}
if [ -z "${GAZELLE_CMD:-}" ] && ! command -v bazelisk >/dev/null && ! command -v bazel >/dev/null; then
  echo "pre-commit-gazelle: no bazel/bazelisk on PATH; skipping (CI gazelle-check still gates)" >&2
  exit 0
fi

snapshot() { { git diff -- '*BUILD.bazel' '*.bzl' MODULE.bazel; git ls-files -o --exclude-standard -- '*BUILD.bazel'; } | sha256sum; }
before=$(snapshot)
if ! sh -c "$cmd" >&2; then
  echo "pre-commit-gazelle: '$cmd' failed" >&2
  exit 1
fi
if [ "$before" != "$(snapshot)" ]; then
  echo "pre-commit-gazelle: BUILD files changed; review and 'git add' them, then commit again" >&2
  git status --short -- '*BUILD.bazel' >&2
  exit 1
fi
