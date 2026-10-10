#!/bin/sh
# Story 0.1.3: who imports session/vc vs session/vcs (non-test and test), plus package-level duplication.
cd "$(git rev-parse --show-toplevel)" || exit 1
mod=$(go list -m)
for p in vc vcs; do
  echo "== importers of $mod/session/$p (go list, incl. tests) =="
  go list -f '{{.ImportPath}}: {{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... 2>/dev/null \
    | grep -E " $mod/session/$p( |\$)" | cut -d: -f1
  echo "== textual refs outside the package itself =="
  git grep -nE "session/$p\"" -- '*.go' ':!.claude' | grep -v "^session/$p/"
done
echo "== exported symbol counts =="
for p in vc vcs; do echo "$p: $(git grep -hE '^func (\([^)]*\) )?[A-Z]|^type [A-Z]' -- "session/$p/*.go" ":!session/$p/*_test.go" | wc -l | tr -d ' ')"; done
