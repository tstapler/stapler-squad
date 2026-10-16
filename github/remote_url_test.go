package github

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// initTestRepoWithRemote creates a bare-bones git repo at dir with an "origin"
// remote pointing at wantURL, using go-git only (no git subprocess).
func initTestRepoWithRemote(t *testing.T, dir, wantURL string) {
	t.Helper()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit(%s): %v", dir, err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{wantURL},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
}

// addLinkedWorktreeFixture creates a linked-worktree-shaped directory tree
// under repoDir/worktrees/<name> without shelling out to `git worktree add`:
// a worktreeDir/.git file pointing at repoDir/.git/worktrees/<name>, which in
// turn contains a "commondir" file pointing back at repoDir/.git — the same
// layout `git worktree add` produces, and what remoteCacheKey parses.
func addLinkedWorktreeFixture(t *testing.T, repoDir, name string) (worktreeDir string) {
	t.Helper()
	wtGitDir := filepath.Join(repoDir, ".git", "worktrees", name)
	if err := os.MkdirAll(wtGitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", wtGitDir, err)
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatalf("write commondir: %v", err)
	}
	// gitdir/HEAD are required for go-git's EnableDotGitCommonDir open path to
	// treat this as a valid linked worktree rather than a corrupt repo.
	if err := os.WriteFile(filepath.Join(wtGitDir, "gitdir"), []byte(filepath.Join(repoDir, ".git")+"\n"), 0o644); err != nil {
		t.Fatalf("write gitdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	worktreeDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(worktreeDir, ".git"), []byte("gitdir: "+wtGitDir+"\n"), 0o644); err != nil {
		t.Fatalf("write .git worktree pointer: %v", err)
	}
	return worktreeDir
}

// resetRemoteURLCache clears package-level cache state between test cases so
// they don't observe each other's cached entries.
func resetRemoteURLCache(t *testing.T) {
	t.Helper()
	remoteURLCache.Range(func(k, _ any) bool {
		remoteURLCache.Delete(k)
		return true
	})
}

func TestGetRemoteURL_ReadsViaGoGitNotSubprocess(t *testing.T) {
	resetRemoteURLCache(t)
	dir := t.TempDir()
	const wantURL = "https://github.com/tstapler/stapler-squad.git"
	initTestRepoWithRemote(t, dir, wantURL)

	got, err := GetRemoteURL(dir)
	if err != nil {
		t.Fatalf("GetRemoteURL: %v", err)
	}
	if got != wantURL {
		t.Errorf("GetRemoteURL = %q, want %q", got, wantURL)
	}
}

// TestRemoteCacheKey_SharedAcrossWorktrees asserts remoteCacheKey resolves
// every linked worktree of the same repo to the identical key (the shared
// common .git dir), not one key per worktree path — otherwise GetRemoteURL's
// cache never hits across a repo's worktrees.
func TestRemoteCacheKey_SharedAcrossWorktrees(t *testing.T) {
	repoDir := t.TempDir()
	initTestRepoWithRemote(t, repoDir, "https://github.com/tstapler/stapler-squad.git")

	wt1 := addLinkedWorktreeFixture(t, repoDir, "wt1")
	wt2 := addLinkedWorktreeFixture(t, repoDir, "wt2")

	key1 := remoteCacheKey(wt1)
	key2 := remoteCacheKey(wt2)
	wantKey := filepath.Join(repoDir, ".git")

	if key1 != wantKey {
		t.Errorf("remoteCacheKey(wt1) = %q, want %q", key1, wantKey)
	}
	if key2 != wantKey {
		t.Errorf("remoteCacheKey(wt2) = %q, want %q", key2, wantKey)
	}
	if key1 != key2 {
		t.Errorf("remoteCacheKey differs across worktrees of the same repo: %q vs %q", key1, key2)
	}
}

func TestGetRemoteURL_CacheHitAcrossWorktrees(t *testing.T) {
	resetRemoteURLCache(t)
	repoDir := t.TempDir()
	const wantURL = "https://github.com/tstapler/stapler-squad.git"
	initTestRepoWithRemote(t, repoDir, wantURL)

	wt1 := addLinkedWorktreeFixture(t, repoDir, "wt1")
	wt2 := addLinkedWorktreeFixture(t, repoDir, "wt2")

	if _, err := GetRemoteURL(wt1); err != nil {
		t.Fatalf("GetRemoteURL(wt1): %v", err)
	}

	// wt1's lookup must have populated the cache under wt2's own key too
	// (both resolve to the same shared common-dir key) — i.e. GetRemoteURL(wt2)
	// is a pure cache hit, not a fresh open, despite never having been called yet.
	if _, ok := remoteURLCache.Load(remoteCacheKey(wt2)); !ok {
		t.Fatalf("cache has no entry for wt2's key after only GetRemoteURL(wt1) ran — cache is not shared across worktrees")
	}

	got, err := GetRemoteURL(wt2)
	if err != nil {
		t.Fatalf("GetRemoteURL(wt2) after wt1 primed the shared cache entry: %v", err)
	}
	if got != wantURL {
		t.Errorf("GetRemoteURL(wt2) = %q, want %q (from shared cache)", got, wantURL)
	}
}
