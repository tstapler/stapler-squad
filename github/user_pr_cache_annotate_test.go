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

// Characterization (Task 1.3.1d): which sessions link to github.com
// acme/api#42 (head fix-ci). Originally captured on the owner-only keys; the
// rows marked "changed in 1.3.1a" are the intended fixes from the host+repo key.
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

	// by-number* have branch "other": the number index is consulted only when
	// no session matches the branch, and here several do.
	want := map[string]bool{
		"https": true, "ssh-dotgit": true, "https-dotgit": true, "empty-host": true,
		"ghe-same-path":           false, // changed in 1.3.1a: was linked (cross-host)
		"same-owner-other-repo":   false, // changed in 1.3.1a: was linked (cross-repo)
		"fork-same-owner-renamed": false, // changed in 1.3.1a: was linked; strict hits suppress the fallback
		"fork-other-owner":        false,
		"branch-case":             false,
		"by-number":               false,
		"by-number-other-repo":    false,
	}
	got := linkedIDs(pr)
	for id, w := range want {
		if got[id] != w {
			t.Errorf("session %q linked=%v, want %v", id, got[id], w)
		}
	}
	if pr.LocalWorktreePath != "" { // changed in 1.3.1a: was /wt/web (cross-repo)
		t.Errorf("LocalWorktreePath = %q, want empty", pr.LocalWorktreePath)
	}

	// PR-number index applies when no session matches the branch.
	c2 := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 42, HeadRef: "no-session-branch", Host: "github.com"})
	c2.Annotate(sessions, nil)
	got2 := linkedIDs(c2.GetAll()[0])
	if !got2["by-number"] || got2["by-number-other-repo"] { // by-number-other-repo changed in 1.3.1a
		t.Errorf("PR-number links = %v, want only by-number", got2)
	}
}

func mustRef(t *testing.T, owner, repo, host string) RepoRef {
	t.Helper()
	ref, err := NewRepoRefWithHost(owner, repo, host)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func annotateOne(t *testing.T, pr UserPR, sessions []PRAnnotationSession, wts []PRAnnotationWorktree) (UserPR, AnnotateStats) {
	t.Helper()
	c := seedPRs(t, pr)
	stats := c.Annotate(sessions, wts)
	return c.GetAll()[0], stats
}

func TestAnnotate_should_LinkOnlyMatchingHost_When_SameRepoBranchOnTwoHosts(t *testing.T) {
	t.Parallel()
	pr, _ := annotateOne(t, UserPR{Owner: "acme", Repo: "api", Number: 1, HeadRef: "fix-ci", Host: "github.com"},
		[]PRAnnotationSession{
			{ID: "s-a", Branch: "fix-ci", Repo: mustRef(t, "acme", "api", "github.com")},
			{ID: "s-b", Branch: "fix-ci", Repo: mustRef(t, "acme", "api", "ghe.corp")},
		}, nil)
	if len(pr.LinkedSessions) != 1 || pr.LinkedSessions[0].SessionID != "s-a" {
		t.Fatalf("LinkedSessions = %+v, want only s-a", pr.LinkedSessions)
	}
}

func TestAnnotate_should_NotLinkSession_When_SameOwnerDifferentRepoSameBranch(t *testing.T) {
	t.Parallel()
	pr, _ := annotateOne(t, UserPR{Owner: "acme", Repo: "api", Number: 1, HeadRef: "fix-ci", Host: "github.com"},
		[]PRAnnotationSession{
			{ID: "s-api", Branch: "fix-ci", Repo: mustRef(t, "acme", "api", "")},
			{ID: "s-web", Branch: "fix-ci", Repo: mustRef(t, "acme", "web", "")},
		}, nil)
	if got := pr.SessionIDs; len(got) != 1 || got[0] != "s-api" {
		t.Fatalf("SessionIDs = %v, want [s-api]", got)
	}
}

func TestAnnotate_should_NotCrossLink_When_SamePRNumberInTwoRepos(t *testing.T) {
	t.Parallel()
	c := seedPRs(t,
		UserPR{Owner: "acme", Repo: "api", Number: 42, HeadRef: "a", Host: "github.com"},
		UserPR{Owner: "acme", Repo: "web", Number: 42, HeadRef: "b", Host: "github.com"})
	c.Annotate([]PRAnnotationSession{
		{ID: "s-api", Branch: "x", PRNumber: 42, Repo: mustRef(t, "acme", "api", "")},
		{ID: "s-web", Branch: "y", PRNumber: 42, Repo: mustRef(t, "acme", "web", "")},
	}, nil)
	prs := c.GetAll()
	if len(prs[0].SessionIDs) != 1 || prs[0].SessionIDs[0] != "s-api" {
		t.Errorf("api PR links = %v, want [s-api]", prs[0].SessionIDs)
	}
	if len(prs[1].SessionIDs) != 1 || prs[1].SessionIDs[0] != "s-web" {
		t.Errorf("web PR links = %v, want [s-web]", prs[1].SessionIDs)
	}
}

func TestAnnotate_should_SetWorktreeOnlyOnMatchingRepo_When_TwoReposShareBranch(t *testing.T) {
	t.Parallel()
	c := seedPRs(t,
		UserPR{Owner: "acme", Repo: "api", Number: 1, HeadRef: "fix-ci", Host: "github.com"},
		UserPR{Owner: "acme", Repo: "web", Number: 2, HeadRef: "fix-ci", Host: "github.com"})
	c.Annotate(nil, []PRAnnotationWorktree{
		{Branch: "fix-ci", Repo: mustRef(t, "acme", "api", ""), WorktreePath: "/wt/api"},
	})
	prs := c.GetAll()
	if prs[0].LocalWorktreePath != "/wt/api" || prs[1].LocalWorktreePath != "" {
		t.Fatalf("worktree paths = %q, %q", prs[0].LocalWorktreePath, prs[1].LocalWorktreePath)
	}
}

func TestAnnotate_should_LinkEmptyHostSessionToGithubComPRAndNotMergeBranchCase_When_FixCIVsFixCi(t *testing.T) {
	t.Parallel()
	sessions := []PRAnnotationSession{
		{ID: "empty-host", Branch: "fix-ci", Repo: mustRef(t, "Acme", "API", "")},
		{ID: "upper", Branch: "Fix-CI", Repo: mustRef(t, "acme", "api", "github.com")},
	}
	lower, _ := annotateOne(t, UserPR{Owner: "acme", Repo: "api", Number: 1, HeadRef: "fix-ci", Host: "github.com"}, sessions, nil)
	if len(lower.SessionIDs) != 1 || lower.SessionIDs[0] != "empty-host" {
		t.Errorf("fix-ci links = %v, want [empty-host]", lower.SessionIDs)
	}
	upper, _ := annotateOne(t, UserPR{Owner: "acme", Repo: "api", Number: 2, HeadRef: "Fix-CI", Host: "github.com"}, sessions, nil)
	if len(upper.SessionIDs) != 1 || upper.SessionIDs[0] != "upper" {
		t.Errorf("Fix-CI links = %v, want [upper]", upper.SessionIDs)
	}
}

func TestSortLinkedSessions_should_OrderByLastActiveDescThenTitleWithZeroLast_When_Annotated(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	ref := mustRef(t, "acme", "api", "")
	pr, _ := annotateOne(t, UserPR{Owner: "acme", Repo: "api", Number: 1, HeadRef: "b", Host: "github.com"},
		[]PRAnnotationSession{
			{ID: "none", Branch: "b", Repo: ref},
			{ID: "old", Branch: "b", Repo: ref, LastActiveAt: base.Add(time.Minute)},
			{ID: "new", Branch: "b", Repo: ref, LastActiveAt: base.Add(5 * time.Minute)},
			{ID: "also-new", Branch: "b", Repo: ref, LastActiveAt: base.Add(5 * time.Minute)},
		}, nil)
	var got []string
	for _, l := range pr.LinkedSessions {
		got = append(got, l.SessionID)
	}
	want := []string{"also-new", "new", "old", "none"}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestAnnotate_should_KeepSessionIDsTitlesAlongsideLinkedSessions_When_Annotated(t *testing.T) {
	t.Parallel()
	pr, _ := annotateOne(t, UserPR{Owner: "acme", Repo: "api", Number: 1, HeadRef: "fix-ci", Host: "github.com"},
		[]PRAnnotationSession{{ID: "s-a", Branch: "fix-ci", Repo: mustRef(t, "acme", "api", ""), Status: LinkedSessionRunning}}, nil)
	if len(pr.SessionIDs) != 1 || pr.SessionIDs[0] != "s-a" || pr.LinkedSessions[0].SessionID != "s-a" ||
		pr.LinkedSessions[0].Status != LinkedSessionRunning {
		t.Fatalf("SessionIDs=%v LinkedSessions=%+v", pr.SessionIDs, pr.LinkedSessions)
	}
}

func TestAnnotate_should_LinkViaLegacyFallbackIndexOnlyWhenNewKeyMisses_When_FallbackNeeded(t *testing.T) {
	t.Parallel()
	pr := UserPR{Owner: "acme", Repo: "api", Number: 1, HeadRef: "fix-ci", Host: "github.com"}
	renamed := PRAnnotationSession{ID: "renamed", Branch: "fix-ci", Repo: mustRef(t, "acme", "api-old", "github.com")}

	got, stats := annotateOne(t, pr, []PRAnnotationSession{renamed}, nil)
	if len(got.LinkedSessions) != 1 || !got.LinkedSessions[0].LegacyFallback || stats.LegacyFallbackLinks != 1 {
		t.Fatalf("fallback link = %+v stats=%+v", got.LinkedSessions, stats)
	}

	strict := PRAnnotationSession{ID: "exact", Branch: "fix-ci", Repo: mustRef(t, "acme", "api", "")}
	got, stats = annotateOne(t, pr, []PRAnnotationSession{renamed, strict}, nil)
	if len(got.LinkedSessions) != 1 || got.LinkedSessions[0].SessionID != "exact" ||
		got.LinkedSessions[0].LegacyFallback || stats.LegacyFallbackLinks != 0 {
		t.Fatalf("strict hit must not consult fallback: %+v stats=%+v", got.LinkedSessions, stats)
	}
}

func TestAnnotate_should_NotWidenMatches_When_LegacyFallbackWouldCrossLinkDifferentRepo(t *testing.T) {
	t.Parallel()
	c := seedPRs(t,
		UserPR{Owner: "acme", Repo: "api", Number: 1, HeadRef: "fix-ci", Host: "github.com"},
		UserPR{Owner: "acme", Repo: "web", Number: 2, HeadRef: "other", Host: "github.com"})
	c.Annotate([]PRAnnotationSession{
		{ID: "s-web", Branch: "fix-ci", Repo: mustRef(t, "acme", "web", "")},
		{ID: "s-ghe", Branch: "fix-ci", Repo: mustRef(t, "acme", "api", "ghe.corp")},
	}, nil)
	prs := c.GetAll()
	if len(prs[0].LinkedSessions) != 0 {
		t.Fatalf("api PR must not link s-web/s-ghe via fallback, got %+v", prs[0].LinkedSessions)
	}
}
