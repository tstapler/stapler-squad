package unfinished

// Tests for WatchDirWatcher's directory-discovery logic (see watcher.go's
// doc comment on why it owns no fsnotify watcher of its own — Scanner.AddRepo
// already registers one per repo). These are the enforcement for wiring the
// previously-dead WatchDirs() setting (Settings → Unfinished Work Sources)
// into a live scanner.

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestScannerForWatchDir() *Scanner {
	return &Scanner{scanQueue: make(chan scanTask, 50)}
}

func makeRepoDir(t *testing.T, parent, name string) string {
	t.Helper()
	repoPath := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return repoPath
}

// TestWatchDirWatcher_WalkDir_DiscoversRepoAndRegistersWithScanner proves the
// core wiring that makes watch dirs functional: a repo found under a watch
// dir actually reaches Scanner.AddRepo, which is what makes it show up in scans.
func TestWatchDirWatcher_WalkDir_DiscoversRepoAndRegistersWithScanner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repoPath := makeRepoDir(t, root, "my-repo")

	s := newTestScannerForWatchDir()
	w := NewWatchDirWatcher(s, nil)
	w.walkDir(root)

	if _, tracked := s.repoSet.Load(repoPath); !tracked {
		t.Fatalf("repo %q discovered under watch dir was never registered with the scanner", repoPath)
	}
}

// TestWatchDirWatcher_WalkDir_SkipsSkipDirs proves a .git directory nested
// under a skip-listed directory (e.g. node_modules, matching a vendored
// dependency's own repo) is never surfaced as a discovered repo.
func TestWatchDirWatcher_WalkDir_SkipsSkipDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nodeModules := filepath.Join(root, "node_modules")
	vendoredRepo := makeRepoDir(t, nodeModules, "some-dep")

	s := newTestScannerForWatchDir()
	w := NewWatchDirWatcher(s, nil)
	w.walkDir(root)

	if _, tracked := s.repoSet.Load(vendoredRepo); tracked {
		t.Fatalf("repo %q under node_modules should have been skipped, but was registered", vendoredRepo)
	}
}

// TestWatchDirWatcher_WalkDir_RespectsDepthLimit proves a repo deeper than
// depth 5 under the watch dir is not discovered — matching walkDir's
// documented depth <= 5 contract.
func TestWatchDirWatcher_WalkDir_RespectsDepthLimit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	deep := root
	for i := 0; i < 6; i++ {
		deep = filepath.Join(deep, "level")
	}
	repoPath := makeRepoDir(t, deep, "too-deep-repo")

	s := newTestScannerForWatchDir()
	w := NewWatchDirWatcher(s, nil)
	w.walkDir(root)

	if _, tracked := s.repoSet.Load(repoPath); tracked {
		t.Fatalf("repo %q beyond depth 5 should not have been discovered", repoPath)
	}
}

// TestWatchDirWatcher_RemoveWatchDir_UnregistersDiscoveredRepos proves
// removing a watch dir releases exactly the repos it contributed, via
// Scanner.RemoveRepo — the discovered-repos index this refactor added to
// replace the old (never-functional) per-repo fsnotify.Watcher.Remove call.
func TestWatchDirWatcher_RemoveWatchDir_UnregistersDiscoveredRepos(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repoPath := makeRepoDir(t, root, "my-repo")

	s := newTestScannerForWatchDir()
	w := NewWatchDirWatcher(s, nil)
	w.walkDir(root)
	if _, tracked := s.repoSet.Load(repoPath); !tracked {
		t.Fatalf("setup: repo %q was not registered before RemoveWatchDir", repoPath)
	}

	w.RemoveWatchDir(root)

	if _, tracked := s.repoSet.Load(repoPath); tracked {
		t.Fatalf("repo %q should have been unregistered after RemoveWatchDir(%q)", repoPath, root)
	}
}

// TestWatchDirWatcher_RemoveWatchDir_DoesNotAffectOtherWatchDirsRepos proves
// removing one watch dir doesn't unregister a repo discovered under a
// different watch dir — the discovered index is keyed per watch dir, not global.
func TestWatchDirWatcher_RemoveWatchDir_DoesNotAffectOtherWatchDirsRepos(t *testing.T) {
	t.Parallel()
	rootA := t.TempDir()
	rootB := t.TempDir()
	repoA := makeRepoDir(t, rootA, "repo-a")
	repoB := makeRepoDir(t, rootB, "repo-b")

	s := newTestScannerForWatchDir()
	w := NewWatchDirWatcher(s, nil)
	w.walkDir(rootA)
	w.walkDir(rootB)

	w.RemoveWatchDir(rootA)

	if _, tracked := s.repoSet.Load(repoA); tracked {
		t.Errorf("repo %q under removed watch dir %q should be unregistered", repoA, rootA)
	}
	if _, tracked := s.repoSet.Load(repoB); !tracked {
		t.Errorf("repo %q under untouched watch dir %q should still be registered", repoB, rootB)
	}
}
