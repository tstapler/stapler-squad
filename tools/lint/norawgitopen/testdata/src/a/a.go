// Package a contains test fixtures for the norawgitopen analyzer.
package a

import (
	git "github.com/go-git/go-git/v5"
)

// BAD1: bare PlainOpen — no commondir support at all.
func bad1(path string) {
	_, _ = git.PlainOpen(path) // want `direct call to git\.PlainOpen`
}

// BAD2: PlainOpenWithOptions without EnableDotGitCommonDir.
func bad2(path string) {
	_, _ = git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true}) // want `direct call to git\.PlainOpenWithOptions`
}

// GOOD1: PlainOpenWithOptions with EnableDotGitCommonDir — still flagged, since the
// wrapper (session/git.OpenRepo) is the required single source of truth, not just
// the right option set repeated at every call site.
func good1(path string) {
	_, _ = git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true}) // want `direct call to git\.PlainOpenWithOptions`
}

// GOOD2: nolint comment on the same line.
func good2(path string) {
	_, _ = git.PlainOpen(path) //nolint:norawgitopen test fixture, not a real open
}
