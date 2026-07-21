package git

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
)

// ScaffoldingExcludePatterns are the git exclude patterns for files stapler-squad
// writes into worktrees (backlog automation scaffolding, build output) that must
// never be committed to the target repo, even if they were already tracked in
// that branch's history. This is the single source of truth: consumed by the
// worktree-write side (session.addWorktreeExcludes, session.selfHealWorktreeScaffolding),
// the commit/push staging guard (StageAllExceptScaffolding below), and
// QuickCommitPush (server/services/unfinished_work_service.go). Keep in sync
// with .gitignore — session/git/scaffolding_test.go asserts they match.
var ScaffoldingExcludePatterns = []string{
	".backlog-context.md",
	".claude/commands/backlog/",
	"web-app/.next/",
}

// UntrackScaffolding removes any git index entry matching patterns (git-rm-cached
// semantics: the working-tree file is left alone, only the index entry is dropped)
// and returns the list of paths it untracked. Uses go-git directly against the
// index rather than shelling out to `git rm --cached`, per
// .claude/rules/prefer-go-git-over-subshells.md.
//
// Returns (nil, nil) — not an error — when worktreePath isn't a git repository at
// all (e.g. a directory-mode session with no git backing), matching the
// best-effort, non-fatal handling the rest of this package uses for that case.
func UntrackScaffolding(worktreePath string, patterns []string) ([]string, error) {
	repo, err := git.PlainOpenWithOptions(worktreePath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, nil
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("read git index: %w", err)
	}

	var toRemove []string
	for _, e := range idx.Entries {
		for _, p := range patterns {
			if scaffoldingPatternMatches(e.Name, p) {
				toRemove = append(toRemove, e.Name)
				break
			}
		}
	}
	if len(toRemove) == 0 {
		return nil, nil
	}

	for _, name := range toRemove {
		if _, rmErr := idx.Remove(name); rmErr != nil {
			return nil, fmt.Errorf("remove %s from index: %w", name, rmErr)
		}
	}
	if err := repo.Storer.SetIndex(idx); err != nil {
		return nil, fmt.Errorf("write git index: %w", err)
	}
	return toRemove, nil
}

// scaffoldingPatternMatches reports whether entryName (a git index path, always
// forward-slash-separated regardless of OS) matches exclude pattern p. Patterns
// ending in "/" match any entry under that directory tree; other patterns match
// the full path exactly.
func scaffoldingPatternMatches(entryName, p string) bool {
	if strings.HasSuffix(p, "/") {
		return strings.HasPrefix(entryName, p)
	}
	return entryName == p
}
