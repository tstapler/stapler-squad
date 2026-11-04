#!/usr/bin/env bash
# Bazel --workspace_status_command: same sanitisers as Makefile (VERSION, GIT_BRANCH, GIT_COMMIT,
# GIT_WORKTREE). STABLE_ keys only: a volatile key would relink on every build.
set -u

version=$( (git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev) | tr -cd 'A-Za-z0-9.+_-')
branch=$( (git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown) | tr -cd 'A-Za-z0-9.+_/-')
commit=$( (git rev-parse --short HEAD 2>/dev/null || echo unknown) | tr -cd 'A-Za-z0-9')
if [ "$(git rev-parse --git-dir 2>/dev/null)" != "$(git rev-parse --git-common-dir 2>/dev/null)" ]; then
  worktree=true
else
  worktree=false
fi

echo "STABLE_GIT_VERSION ${version:-dev}"
echo "STABLE_GIT_BRANCH ${branch:-unknown}"
echo "STABLE_GIT_COMMIT ${commit:-unknown}"
echo "STABLE_GIT_WORKTREE ${worktree}"
