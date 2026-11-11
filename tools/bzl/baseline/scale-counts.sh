#!/usr/bin/env bash
# Task 0.1d: recount the repo scale numbers from git, one `key: value` per line
# (grep-friendly), run at HEAD of the current checkout.
#   tracked_go_files       git ls-files '*.go'
#   tracked_test_files     git ls-files '*_test.go'
#   test_packages          distinct directories holding a tracked *_test.go (all modules)
#   test_packages_root_module  same, excluding nested modules and project_plans/
#   go_mod_files           tracked go.mod files (all)
#   go_mod_files_live      go.mod files outside project_plans/ (spike copies)
#   ignored_generated_go_files   gitignored .go files present in this checkout
#                                (generated: gen/proto, session/ent; zero in a fresh worktree)
# usage: scale-counts.sh [--repo <dir>]   (default: this checkout)
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
R="$BZL_REPO_ROOT"
[[ "${1:-}" == --repo ]] && R="$2"
g() { git -C "$R" "$@"; }
nested='^project_plans/|^tools/lint/|^tools/scanner/'
echo "tracked_go_files: $(g ls-files '*.go' | wc -l)"
echo "tracked_test_files: $(g ls-files '*_test.go' | wc -l)"
echo "test_packages: $(g ls-files '*_test.go' | xargs -n1 dirname | sort -u | wc -l)"
echo "test_packages_root_module: $(g ls-files '*_test.go' | grep -vE "$nested" | xargs -n1 dirname | sort -u | wc -l)"
echo "go_mod_files: $(g ls-files | grep -cE '(^|/)go\.mod$')"
echo "go_mod_files_live: $(g ls-files | grep -E '(^|/)go\.mod$' | grep -vc '^project_plans/')"
echo "ignored_generated_go_files: $(g ls-files --others --ignored --exclude-standard '*.go' | grep -vc '^\.claude/worktrees/')"
