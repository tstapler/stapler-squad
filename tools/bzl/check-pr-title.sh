#!/usr/bin/env bash
# Repo rule C7: PR titles are Conventional Commits. Usage: check-pr-title.sh '<title>'
# (bazel.yml passes github.event.pull_request.title).
title="${1:-}"
if printf '%s' "$title" | grep -qE '^(feat|fix|chore|docs|refactor|test)(\(.+\))?!?: '; then
  exit 0
fi
echo "check-pr-title: '$title' is not a Conventional Commit title (feat|fix|chore|docs|refactor|test)(scope)?: ..." >&2
exit 1
