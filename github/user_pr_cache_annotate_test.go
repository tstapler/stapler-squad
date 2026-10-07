package github

import (
	"testing"
	"time"
)

// refFromRemote builds a RepoRef the way GetOwnerRepoFromRemote does, from a
// real-shaped remote URL.
func refFromRemote(t *testing.T, remote string) RepoRef {
	t.Helper()
	parsed, err := ParseGitHubRefWithHosts(remote, []string{"ghe.corp"})
	if err != nil {
		t.Fatalf("parse %q: %v", remote, err)
	}
	ref, err := NewRepoRefWithHost(parsed.Owner, parsed.Repo, parsed.Host)
	if err != nil {
		t.Fatalf("ref %q: %v", remote, err)
	}
	return ref
}

func seedPRs(t *testing.T, prs ...UserPR) *UserPRCache {
	t.Helper()
	c := NewUserPRCache()
	c.snapshot.Store(&userPRSnapshot{prs: prs, capturedAt: time.Now()})
	return c
}

func linkedIDs(pr UserPR) map[string]bool {
	out := map[string]bool{}
	for _, id := range pr.SessionIDs {
		out[id] = true
	}
	return out
}

// Characterization (Task 1.3.1d): pins which sessions link to github.com
// acme/api#42 (head fix-ci). Expectations were captured on the owner-only
// keys; rows marked "changes in 1.3.1a" are the intended fixes.
func TestAnnotate_should_PinCurrentLinks_When_RealShapedRepoRefsHttpsSshDotGitEmptyHostGHEFork(t *testing.T) {
	t.Parallel()
	empty, err := NewRepoRef("acme", "api")
	if err != nil {
		t.Fatal(err)
	}
	sessions := []PRAnnotationSession{
		{ID: "https", Branch: "fix-ci", Repo: refFromRemote(t, "https://github.com/acme/api")},
		{ID: "ssh-dotgit", Branch: "fix-ci", Repo: refFromRemote(t, "git@github.com:acme/api.git")},
		{ID: "https-dotgit", Branch: "fix-ci", Repo: refFromRemote(t, "https://github.com/acme/api.git")},
		{ID: "empty-host", Branch: "fix-ci", Repo: empty},
		{ID: "ghe-same-path", Branch: "fix-ci", Repo: refFromRemote(t, "https://ghe.corp/acme/api")},
		{ID: "same-owner-other-repo", Branch: "fix-ci", Repo: refFromRemote(t, "https://github.com/acme/web")},
		{ID: "fork-other-owner", Branch: "fix-ci", Repo: refFromRemote(t, "https://github.com/me/api")},
		{ID: "fork-same-owner-renamed", Branch: "fix-ci", Repo: refFromRemote(t, "https://github.com/acme/api-fork")},
		{ID: "branch-case", Branch: "Fix-CI", Repo: refFromRemote(t, "https://github.com/acme/api")},
		{ID: "by-number", Branch: "other", Repo: refFromRemote(t, "https://github.com/acme/api"), PRNumber: 42},
		{ID: "by-number-other-repo", Branch: "other", Repo: refFromRemote(t, "https://github.com/acme/web"), PRNumber: 42},
	}
	worktrees := []PRAnnotationWorktree{
		{Branch: "fix-ci", Repo: refFromRemote(t, "https://github.com/acme/web"), WorktreePath: "/wt/web"},
	}
	c := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 42, HeadRef: "fix-ci", Host: "github.com"})
	c.Annotate(sessions, worktrees)
	pr := c.GetAll()[0]

	got := linkedIDs(pr)
	want := map[string]bool{
		"https": true, "ssh-dotgit": true, "https-dotgit": true, "empty-host": true,
		"ghe-same-path":           true, // changes in 1.3.1a: cross-host link dropped
		"same-owner-other-repo":   true, // changes in 1.3.1a: cross-repo link dropped
		"fork-same-owner-renamed": true, // changes in 1.3.1a: becomes legacy-fallback link
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("session %q linked=%v, want %v", id, got[id], w)
		}
	}
	// by-number* have branch "other": the number index is consulted only when
	// no session matches the branch, and here several do.
	for _, id := range []string{"fork-other-owner", "branch-case", "by-number", "by-number-other-repo"} {
		if got[id] {
			t.Errorf("session %q linked today, expected not", id)
		}
	}
	if pr.LocalWorktreePath != "/wt/web" { // changes in 1.3.1a: cross-repo worktree dropped
		t.Errorf("LocalWorktreePath = %q, want /wt/web (owner-only key)", pr.LocalWorktreePath)
	}

	// PR-number index applies when no session matches the branch.
	c2 := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 42, HeadRef: "no-session-branch", Host: "github.com"})
	c2.Annotate(sessions, nil)
	got2 := linkedIDs(c2.GetAll()[0])
	if !got2["by-number"] || !got2["by-number-other-repo"] { // by-number-other-repo changes in 1.3.1a
		t.Errorf("PR-number links = %v, want by-number and by-number-other-repo", got2)
	}
}
