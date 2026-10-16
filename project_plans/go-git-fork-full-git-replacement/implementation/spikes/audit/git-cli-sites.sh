#!/bin/sh
# Reproduces the git-CLI call-site counts for Story 0.1.1. Run from anywhere inside the repo.
# Uses `git grep` (tracked files only) so .claude/worktrees copies do not inflate counts.
cd "$(git rev-parse --show-toplevel)" || exit 1
echo "HEAD: $(git rev-parse --short HEAD)  branch: $(git rev-parse --abbrev-ref HEAD)"
prod() { git grep "$@" -- "*.go" ":!*_test.go" ":!.claude" ":!third_party"; }
test_() { git grep "$@" -- "*_test.go" ":!.claude" ":!third_party"; }
c() { printf '%-62s %s\n' "$1" "$2"; }
c 'product "git", sites (claimed 62)'            "$(prod -nE '"git",' | wc -l | tr -d ' ')"
c 'product "git", files (claimed 29)'            "$(prod -lE '"git",' | wc -l | tr -d ' ')"
c 'test "git", sites (claimed 223)'              "$(test_ -nE '"git",' | wc -l | tr -d ' ')"
c 'test "git", sites under session/ (claimed 121)' "$(git grep -nE '"git",' -- 'session/*_test.go' | wc -l | tr -d ' ')"
c 'product exec/safeexec constructor sites (claimed 36)' "$(prod -nE '(safeexec|exec)\.Command(Context)?\([^)]*"git"' | wc -l | tr -d ' ')"
c 'product exec/safeexec constructor files (claimed 19)' "$(prod -lE '(safeexec|exec)\.Command(Context)?\([^)]*"git"' | wc -l | tr -d ' ')"
c 'non-test go-git importers (claimed 30)'       "$(git grep -lE 'go-git/go-git/v5' -- '*.go' ':!*_test.go' ':!.claude' | wc -l | tr -d ' ')"
c 'product runner sites (claimed 25)'            "$(prod -nE '\.Run\([^)]*"git"|commandRunner\(\)\.Run|runGitCommand\(' | wc -l | tr -d ' ')"
c 'test direct exec.Command("git" sites (claimed 10)' "$(test_ -nE 'exec\.Command(Context)?\([^)]*"git"' | wc -l | tr -d ' ')"
c 'LookPath("git") total (claimed 30)'           "$(git grep -nE 'LookPath\("git"\)' -- '*.go' ':!.claude' | wc -l | tr -d ' ')"
c 'LookPath("git") test sites (claimed 29)'      "$(test_ -nE 'LookPath\("git"\)' | wc -l | tr -d ' ')"
c 'LookPath("git") test files (claimed 10)'      "$(test_ -lE 'LookPath\("git"\)' | wc -l | tr -d ' ')"
c 'Setenv("PATH" test sites (claimed 20)'        "$(test_ -nE 'Setenv\("PATH"' | wc -l | tr -d ' ')"
c 'Setenv("PATH" test files (claimed 12)'        "$(test_ -lE 'Setenv\("PATH"' | wc -l | tr -d ' ')"
echo; echo "Product subcommand histogram (first arg after \"git\",):"
prod -hoE '"git", *"[a-z-]+"' | sed -E 's/.*, *//' | sort | uniq -c | sort -rn
