package session

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/tstapler/stapler-squad/github"
	ssqlog "github.com/tstapler/stapler-squad/log"
	gitutil "github.com/tstapler/stapler-squad/session/git"
)

// stapler-squad#152 security review: GitHub owner/repo regexes only exclude
// "/" from the captured segments, so "." and ".." pass through syntactically.
// GetRepoPath joins Owner/Repo directly into a filesystem path, so a crafted
// input like "https://github.com/../.." must be rejected before it ever
// reaches EnsureRepoCloned, not allowed to resolve outside the intended
// ~/.stapler-squad/repos/github.com/ subtree.
func TestParseGitHubURL_RejectsPathTraversalSegments(t *testing.T) {
	t.Parallel()
	cases := []string{
		"https://github.com/../..",
		"https://github.com/../../etc",
		"https://github.com/owner/..",
		"https://github.com/../repo",
		"https://github.com/owner/../repo/tree/main",
		"https://github.com/owner/../repo/pull/1",
		"owner/..",
		"../repo",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			ref, err := ParseGitHubURL(in)
			if err == nil {
				t.Errorf("ParseGitHubURL(%q) = %+v, want error (traversal segment)", in, ref)
			}
		})
	}
}

func TestParseGitHubURL_HappyPath(t *testing.T) {
	t.Parallel()
	ref, err := ParseGitHubURL("https://github.com/owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Owner != "owner" || ref.Repo != "repo" {
		t.Errorf("got Owner=%q Repo=%q, want Owner=owner Repo=repo", ref.Owner, ref.Repo)
	}
}

func TestParseGitHubURL_ShorthandHappyPath(t *testing.T) {
	t.Parallel()
	ref, err := ParseGitHubURL("owner/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Owner != "owner" || ref.Repo != "repo" {
		t.Errorf("got Owner=%q Repo=%q, want Owner=owner Repo=repo", ref.Owner, ref.Repo)
	}
}

func TestGetRepoPath_StaysWithinBaseDir(t *testing.T) {
	t.Parallel()
	m := NewRepoPathManagerWithBase("/tmp/repos-base")
	path := m.GetRepoPath(&GitHubRef{Owner: "owner", Repo: "repo"})
	want := "/tmp/repos-base/github.com/owner/repo"
	if path != want {
		t.Errorf("GetRepoPath() = %q, want %q", path, want)
	}
}

// stapler-squad: registering a GitHub Enterprise domain must make the omnibar
// able to parse URLs from it, e.g. https://github.example-corp.com/engineering/widget-service/pull/370
func TestParseGitHubURLWithHosts_RecognizesEnterprisePRURL(t *testing.T) {
	t.Parallel()
	hosts := []string{"github.example-corp.com"}
	ref, err := ParseGitHubURLWithHosts("https://github.example-corp.com/engineering/widget-service/pull/370", hosts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Host != "github.example-corp.com" || ref.Owner != "engineering" || ref.Repo != "widget-service" || ref.PRNumber != 370 || ref.Type != GitHubRefTypePR {
		t.Errorf("got %+v, want Host=github.example-corp.com Owner=engineering Repo=widget-service PRNumber=370 Type=PR", ref)
	}
}

// TestParseGitHubURLWithHosts_MatchesRegardlessOfHostCase is the regression
// test for the round-2 review finding: GHE hosts are free-text admin/user
// input (unlike the hardcoded "github.com"), so a registered host typed in
// one case must still match a URL pasted in another case.
func TestParseGitHubURLWithHosts_MatchesRegardlessOfHostCase(t *testing.T) {
	t.Parallel()
	hosts := []string{"Github.Example-Corp.com"}
	ref, err := ParseGitHubURLWithHosts("https://GITHUB.EXAMPLE-CORP.COM/engineering/widget-service/pull/370", hosts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Host != "github.example-corp.com" || ref.Owner != "engineering" || ref.Repo != "widget-service" || ref.PRNumber != 370 {
		t.Errorf("got %+v, want Host=github.example-corp.com Owner=engineering Repo=widget-service PRNumber=370", ref)
	}
}

func TestParseGitHubURLWithHosts_RejectsUnregisteredHost(t *testing.T) {
	t.Parallel()
	// Without the host registered, an enterprise URL must not be silently
	// mis-parsed against github.com.
	_, err := ParseGitHubURLWithHosts("https://github.example-corp.com/engineering/widget-service/pull/370", nil)
	if err == nil {
		t.Errorf("expected error for unregistered enterprise host, got nil")
	}
}

func TestParseGitHubURLWithHosts_RejectsPathTraversalSegments(t *testing.T) {
	t.Parallel()
	hosts := []string{"github.example-corp.com"}
	cases := []string{
		"https://github.example-corp.com/../..",
		"https://github.example-corp.com/owner/../repo/pull/1",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			ref, err := ParseGitHubURLWithHosts(in, hosts)
			if err == nil {
				t.Errorf("ParseGitHubURLWithHosts(%q) = %+v, want error (traversal segment)", in, ref)
			}
		})
	}
}

func TestIsGitHubURLWithHosts(t *testing.T) {
	t.Parallel()
	hosts := []string{"github.example-corp.com"}
	if !IsGitHubURLWithHosts("https://github.example-corp.com/engineering/widget-service/pull/370", hosts) {
		t.Errorf("expected true for registered enterprise host URL")
	}
	if IsGitHubURLWithHosts("https://github.example-corp.com/engineering/widget-service/pull/370", nil) {
		t.Errorf("expected false for unregistered enterprise host URL")
	}
}

func TestGetRepoPath_UsesHostSpecificSubdirectory(t *testing.T) {
	t.Parallel()
	m := NewRepoPathManagerWithBase("/tmp/repos-base")
	path := m.GetRepoPath(&GitHubRef{Host: "github.example-corp.com", Owner: "engineering", Repo: "widget-service"})
	want := "/tmp/repos-base/github.example-corp.com/engineering/widget-service"
	if path != want {
		t.Errorf("GetRepoPath() = %q, want %q", path, want)
	}
}

func TestGetCloneURL_UsesHostSpecificURL(t *testing.T) {
	t.Parallel()
	m := NewRepoPathManagerWithBase("/tmp/repos-base")
	url := m.GetCloneURL(&GitHubRef{Host: "github.example-corp.com", Owner: "engineering", Repo: "widget-service"})
	want := "https://github.example-corp.com/engineering/widget-service.git"
	if url != want {
		t.Errorf("GetCloneURL() = %q, want %q", url, want)
	}
}

// TestGetCloneURL_EmbedsKeychainToken_When_HostHasStoredAccount locks in the
// token-injection branch of GetCloneURL, which previously had no test
// coverage — the exact code path that EnsureRepoCloned's credential-logging
// fix (host/owner/repo-only logging, post-clone `git remote set-url`
// stripping, sanitizeCloneOutput) depends on actually firing.
func TestGetCloneURL_EmbedsKeychainToken_When_HostHasStoredAccount(t *testing.T) {
	// Not t.Parallel(): SetKeychainTokenForAccount below mutates process-global
	// keychain mock state (TestMain's keyring.MockInit()) for
	// "github.example-corp.com" — the same host literal
	// TestGetCloneURL_UsesHostSpecificURL uses, so running both in parallel
	// makes that test's GetCloneURL call nondeterministically pick up this
	// test's stored token and fail. The t.Cleanup below removes the token
	// again afterward — without it, the leak persists past this (non-parallel)
	// test's own execution window into the batch of t.Parallel() tests that
	// run once the sequential pass finishes, causing the same failure even
	// with the two tests never truly running concurrently.
	host := "github.example-corp.com"
	if err := github.SetKeychainTokenForAccount(host, "octocat", "token-abc123"); err != nil {
		t.Fatalf("SetKeychainTokenForAccount failed: %v", err)
	}
	// The keychain mock (TestMain's keyring.MockInit()) is one process-global
	// store shared by every test in this binary — not reset between tests.
	// Without this cleanup, the stored token for this host leaks into
	// TestGetCloneURL_UsesHostSpecificURL (same host literal), which resumes
	// as a t.Parallel() test only after this non-parallel test has already
	// completed and its GetCloneURL call would then nondeterministically pick
	// up this test's token.
	t.Cleanup(func() {
		if err := github.DeleteKeychainTokenForAccount(host, "octocat"); err != nil {
			t.Logf("cleanup: DeleteKeychainTokenForAccount(%q): %v", host, err)
		}
	})

	m := NewRepoPathManagerWithBase("/tmp/repos-base")
	url := m.GetCloneURL(&GitHubRef{Host: host, Owner: "engineering", Repo: "widget-service"})
	want := "https://x-access-token:token-abc123@github.example-corp.com/engineering/widget-service.git"
	if url != want {
		t.Errorf("GetCloneURL() = %q, want %q", url, want)
	}
}

// TestParseGitHubURLWithHosts_RejectsLocalLookingPaths is the regression test
// for the dropped path-vs-shorthand guard: the "owner/repo" shorthand regex
// can't distinguish a real shorthand from a relative/absolute/home-relative
// local path, since both are just "segment/segment". Concretely,
// ParseGitHubURL(".git/repo") used to be rejected (see the pre-refactor
// implementation's `!strings.HasPrefix(input, ".")` guard) but started
// succeeding with Owner=".git" once shorthand parsing moved into the shared
// github package, which has no equivalent guard.
func TestParseGitHubURLWithHosts_RejectsLocalLookingPaths(t *testing.T) {
	t.Parallel()
	cases := []string{
		".git/repo",
		"./relative/path",
		"/absolute/path",
		"~/home/path",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			ref, err := ParseGitHubURL(in)
			if err == nil {
				t.Errorf("ParseGitHubURL(%q) = %+v, want error (local-looking path)", in, ref)
			}
		})
	}
}

// TestGitHubRefPRURL_should_BuildCanonicalURL_When_PRNumberSet is the
// regression test for the PR #463 bug: CreateSession parsed a GitHub PR URL
// into a GitHubRef but never populated GitHubPRURL on the created session,
// leaving the VCS panel's PR link broken. PRURL() is the single function both
// CreateSession (server/services/session_service.go) and the startup backfill
// migration (github_pr_url_backfill.go) now call instead of formatting the
// URL inline in two places.
func TestGitHubRefPRURL_should_BuildCanonicalURL_When_PRNumberSet(t *testing.T) {
	t.Parallel()
	ref := &GitHubRef{Host: "github.com", Owner: "tstapler", Repo: "stapler-squad", PRNumber: 463}

	got := ref.PRURL()

	want := "https://github.com/tstapler/stapler-squad/pull/463"
	if got != want {
		t.Errorf("PRURL() = %q, want %q", got, want)
	}
}

// TestGitHubRefPRURL_should_NormalizeHost_When_HostIsGitHubEnterprise proves
// PRURL() routes through github.NormalizeHost rather than embedding the raw
// host string, so GHES hosts are handled the same way whether the ref came
// from CreateSession's live parse or the backfill's path-recovered host.
func TestGitHubRefPRURL_should_NormalizeHost_When_HostIsGitHubEnterprise(t *testing.T) {
	t.Parallel()
	ref := &GitHubRef{Host: "github.mycorp.com", Owner: "acme", Repo: "widgets", PRNumber: 7}

	got := ref.PRURL()

	want := "https://" + github.NormalizeHost("github.mycorp.com") + "/acme/widgets/pull/7"
	if got != want {
		t.Errorf("PRURL() = %q, want %q", got, want)
	}
}

// TestGitHubRefPRURL_should_ReturnEmpty_When_PRNumberNotSet guards the
// zero-value case (a repo/branch ref, not a PR ref) — must not fabricate a
// "/pull/0" URL.
func TestGitHubRefPRURL_should_ReturnEmpty_When_PRNumberNotSet(t *testing.T) {
	t.Parallel()
	ref := &GitHubRef{Host: "github.com", Owner: "tstapler", Repo: "stapler-squad"}

	if got := ref.PRURL(); got != "" {
		t.Errorf("PRURL() = %q, want empty string when PRNumber is unset", got)
	}
}

// TestGitHubRefPRURL_should_ReturnEmpty_When_RefIsNil guards the nil-receiver
// case — CreateSession's gitHubRef is a *GitHubRef that can be nil for
// non-GitHub session creation, so PRURL() must not panic if ever called on it.
func TestGitHubRefPRURL_should_ReturnEmpty_When_RefIsNil(t *testing.T) {
	t.Parallel()
	var ref *GitHubRef

	if got := ref.PRURL(); got != "" {
		t.Errorf("PRURL() = %q, want empty string for a nil ref", got)
	}
}

// TestIsCorruptedClone_ReturnsFalse_When_RepoIsHealthy guards against false
// positives: a normal repo with at least one commit must never be flagged as
// corrupted, or EnsureRepoCloned would delete and re-clone perfectly good repos.
func TestIsCorruptedClone_ReturnsFalse_When_RepoIsHealthy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit failed: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := wt.Add("f.txt"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com"}}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if isCorruptedClone(dir) {
		t.Errorf("isCorruptedClone(%q) = true, want false for a healthy repo", dir)
	}
}

// TestIsCorruptedClone_ReturnsTrue_When_HeadIsUnresolvable reproduces the
// exact corruption signature left behind by an interrupted `git clone`: git's
// internal bootstrap phase writes a placeholder symbolic HEAD
// ("ref: refs/heads/.invalid") before the clone completes and rewrites it to
// the real default branch (see builtin/clone.c / refs.c write_file call). If
// the clone subprocess is killed in between, HEAD points at a ref that will
// never exist.
func TestIsCorruptedClone_ReturnsTrue_When_HeadIsUnresolvable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := git.PlainInit(dir, false); err != nil {
		t.Fatalf("PlainInit failed: %v", err)
	}
	headPath := filepath.Join(dir, ".git", "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/.invalid\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if !isCorruptedClone(dir) {
		t.Errorf("isCorruptedClone(%q) = false, want true for an unresolvable-HEAD repo", dir)
	}
}

// TestIsCorruptedClone_ReturnsTrue_When_NotAGitRepo guards the other failure
// mode isCorruptedClone must catch: a directory that has a ".git" entry
// os.Stat can see but that go-git can't open at all (e.g. a partially-written
// or truncated clone) should also be treated as corrupted, not panic or be
// silently treated as healthy.
func TestIsCorruptedClone_ReturnsTrue_When_NotAGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if !isCorruptedClone(dir) {
		t.Errorf("isCorruptedClone(%q) = false, want true for a non-git directory", dir)
	}
}

// TestRepairCorruptedGitRepo_ReRepairs_When_HeadIsUnresolvable is the
// regression test for CreateBacklogWorktree's repair gap: EnsureRepoCloned
// self-heals a corrupted clone, but backlog-triage-reached repos never went
// through EnsureRepoCloned, so RepairCorruptedGitRepo generalizes the same
// self-heal to a plain on-disk repo path. This clones a real local "origin"
// repo, corrupts the clone's HEAD the same way TestIsCorruptedClone_* does,
// then verifies repair re-clones it into a healthy state with the original
// commit intact.
func TestRepairCorruptedGitRepo_ReRepairs_When_HeadIsUnresolvable(t *testing.T) {
	t.Parallel()
	originDir := t.TempDir()
	originRepo, err := git.PlainInit(originDir, false)
	if err != nil {
		t.Fatalf("PlainInit(origin) failed: %v", err)
	}
	wt, err := originRepo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originDir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := wt.Add("f.txt"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	commitSHA, err := wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com"}})
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	clonePath := filepath.Join(t.TempDir(), "clone")
	if _, err := git.PlainClone(clonePath, false, &git.CloneOptions{URL: originDir}); err != nil {
		t.Fatalf("PlainClone failed: %v", err)
	}
	headPath := filepath.Join(clonePath, ".git", "HEAD")
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/.invalid\n"), 0644); err != nil {
		t.Fatalf("WriteFile(HEAD) failed: %v", err)
	}
	if !isCorruptedClone(clonePath) {
		t.Fatalf("test setup failed: clone at %q should be corrupted before repair", clonePath)
	}

	if err := RepairCorruptedGitRepo(clonePath); err != nil {
		t.Fatalf("RepairCorruptedGitRepo failed: %v", err)
	}

	if isCorruptedClone(clonePath) {
		t.Errorf("RepairCorruptedGitRepo(%q): repo still corrupted after repair", clonePath)
	}
	repairedRepo, err := gitutil.OpenRepo(clonePath)
	if err != nil {
		t.Fatalf("PlainOpen(repaired clone) failed: %v", err)
	}
	head, err := repairedRepo.Head()
	if err != nil {
		t.Fatalf("Head() failed after repair: %v", err)
	}
	if head.Hash() != commitSHA {
		t.Errorf("repaired clone HEAD = %s, want %s", head.Hash(), commitSHA)
	}
}

// TestRepairCorruptedGitRepo_NoOp_When_RepoIsHealthy guards against
// unnecessarily deleting and re-cloning a perfectly good repo.
func TestRepairCorruptedGitRepo_NoOp_When_RepoIsHealthy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit failed: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := wt.Add("f.txt"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com"}}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if err := RepairCorruptedGitRepo(dir); err != nil {
		t.Fatalf("RepairCorruptedGitRepo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.txt")); err != nil {
		t.Errorf("RepairCorruptedGitRepo deleted/re-cloned a healthy repo: %v", err)
	}
}

// TestRepairCorruptedGitRepo_NoOp_When_NotAGitRepo guards the other no-op
// case: a plain directory (e.g. a resolved path that isn't actually a git
// repo) must not be treated as corrupted and deleted.
func TestRepairCorruptedGitRepo_NoOp_When_NotAGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := RepairCorruptedGitRepo(dir); err != nil {
		t.Fatalf("RepairCorruptedGitRepo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.txt")); err != nil {
		t.Errorf("RepairCorruptedGitRepo deleted a non-git directory: %v", err)
	}
}

// TestGetMainRepoPath_ResolvesWorktreeToMainRepo is the regression test for
// CreateBacklogWorktree's anchor-path fix: a BacklogItem's stored RepoPath can be a
// worktree's own directory rather than the main checkout (e.g. an agent running inside
// one filed the item using its own CWD). git worktree add still succeeds when run from
// such a worktree, but every later operation anchored at that same path — WithRepoWorktreeLock's
// lock key, RepairCorruptedGitRepo's target — would then be keyed to a directory that
// might not outlive the call (e.g. an ephemeral triage worktree). GetMainRepoPath must
// resolve a worktree path back to the real main repo root so CreateBacklogWorktree can
// anchor there instead.
func TestGetMainRepoPath_ResolvesWorktreeToMainRepo(t *testing.T) {
	t.Parallel()
	mainDir := t.TempDir()
	repo, err := git.PlainInit(mainDir, false)
	if err != nil {
		t.Fatalf("PlainInit failed: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := wt.Add("f.txt"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com"}}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	worktreeDir := filepath.Join(t.TempDir(), "extra-worktree")
	runGitOrFail(t, mainDir, "worktree", "add", "-b", "extra-branch", worktreeDir)

	resolved, err := GetMainRepoPath(worktreeDir)
	if err != nil {
		t.Fatalf("GetMainRepoPath(worktreeDir) failed: %v", err)
	}

	// Compare canonicalized paths — t.TempDir() can itself sit under a symlink
	// (e.g. macOS's /tmp -> /private/tmp), which would break a literal string compare.
	wantMain, err := filepath.EvalSymlinks(mainDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(mainDir) failed: %v", err)
	}
	gotMain, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		t.Fatalf("EvalSymlinks(resolved) failed: %v", err)
	}
	if gotMain != wantMain {
		t.Errorf("GetMainRepoPath(%q) = %q, want %q (the main repo, not the worktree itself)", worktreeDir, gotMain, wantMain)
	}
}

// TestCreateBacklogWorktree_AnchorsAtMainRepo_When_RepoPathIsAWorktree verifies
// CreateBacklogWorktree end-to-end when given a worktree's path (rather than the main
// checkout) as repoPath: it must still succeed, branching from the main repo's default
// branch tip, rather than erroring or misbehaving because GetMainRepoPath's resolution
// wasn't wired in.
func TestCreateBacklogWorktree_AnchorsAtMainRepo_When_RepoPathIsAWorktree(t *testing.T) {
	// Not t.Parallel(): this test swaps the log package's injectable slog seam
	// to capture CreateBacklogWorktree's log output, which would race other
	// parallel tests' logging.
	origin := t.TempDir()
	originRepo, err := git.PlainInit(origin, false)
	if err != nil {
		t.Fatalf("PlainInit(origin) failed: %v", err)
	}
	originWT, err := originRepo.Worktree()
	if err != nil {
		t.Fatalf("Worktree failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(origin, "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := originWT.Add("f.txt"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	mainTip, err := originWT.Commit("init", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.com"}})
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	runGitOrFail(t, origin, "branch", "-M", "main")

	mainRepo := cloneWithOrigin(t, origin)

	// Simulate a backlog item whose RepoPath got stored as a worktree's own directory —
	// an agent running inside one filed the item using its own CWD instead of the main
	// checkout. First create a stand-in worktree via a real backlog spawn.
	anchorWorktree, err := CreateBacklogWorktree(mainRepo, "anchor-item")
	if err != nil {
		t.Fatalf("CreateBacklogWorktree(mainRepo) failed: %v", err)
	}
	wantMain, err := filepath.EvalSymlinks(mainRepo)
	if err != nil {
		t.Fatalf("EvalSymlinks(mainRepo) failed: %v", err)
	}

	// Capture slog output (at Debug level, since the line asserted on below logs at
	// Debug) to verify which repo path CreateBacklogWorktree actually operated
	// against — the resulting branch's commit history alone isn't a reliable signal:
	// go-git's Head() resolution against a *linked worktree* path (rather than the
	// main repo) is unreliable enough that, pre-fix, this exact scenario was
	// misdetected as an unborn (zero-commit) repo and took the ambient-HEAD fallback
	// instead — which still happened to branch from the right commit here only
	// because that fallback shells out to real git, not because the resolution was
	// actually correct.
	var logBuf bytes.Buffer
	prevLogger := ssqlog.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer ssqlog.SetSlogDefaultForTest(prevLogger)

	childWorktree, err := CreateBacklogWorktree(anchorWorktree, "child-item")
	if err != nil {
		t.Fatalf("CreateBacklogWorktree(anchorWorktree) failed: %v", err)
	}

	logOutput := logBuf.String()
	if strings.Contains(logOutput, "no default branch found and repo has no commits yet") {
		t.Errorf("CreateBacklogWorktree(anchorWorktree) took the unborn-repo/ambient-HEAD fallback — GetMainRepoPath resolution did not take effect; log:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, `defaultBranch=main`) {
		t.Errorf("expected a debug log confirming resolution against the main branch; log:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, "repoPath="+wantMain) {
		t.Errorf("CreateBacklogWorktree did not operate against the resolved main repo path %q; log:\n%s", wantMain, logOutput)
	}

	base := strings.TrimSpace(runGitOutputOrFail(t, childWorktree, "merge-base", "HEAD", mainTip.String()))
	if base != mainTip.String() {
		t.Errorf("child worktree's branch point = %s, want origin's main tip %s (must branch from the resolved main repo, not the anchor worktree)", base, mainTip)
	}
}

// TestCreateBacklogWorktree_should_Error_When_RepoPathIsEmpty guards the production
// (not test-only) mechanism that can reach this exact corruption: a BacklogItem can
// legitimately have an empty RepoPath (CreateBacklogItem never requires one, and
// TransitionBacklogItemStatus's guards never check it — Idea->Ready is a directly
// legal transition per session/domain/backlog.go's validTransitions map), so an item
// can reach SpawnSessionFromItem with RepoPath still "". Without this guard,
// ResolveSessionPath("") would silently resolve to the calling process's own cwd via
// filepath.Abs("") — for the live server process, that's this repo's own real
// checkout. Asserts on the error directly rather than chdir'ing into a fake repo: this
// guard returns before ResolveSessionPath is ever called, so no real cwd interaction
// happens for it to observe.
func TestCreateBacklogWorktree_should_Error_When_RepoPathIsEmpty(t *testing.T) {
	_, err := CreateBacklogWorktree("", "empty-repopath-item")
	if err == nil {
		t.Fatal("CreateBacklogWorktree(\"\", ...) succeeded; want an error rather than silently operating against cwd")
	}
	if !strings.Contains(err.Error(), "repoPath must not be empty") {
		t.Errorf("CreateBacklogWorktree(\"\", ...) error = %q, want it to mention repoPath must not be empty", err.Error())
	}
}
