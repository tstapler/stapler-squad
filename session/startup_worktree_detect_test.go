package session

import (
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// withWatchdog fails the test cleanly instead of hanging if fn blocks.
func withWatchdog(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("watchdog: test body blocked")
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}

func stubDetect(t *testing.T, fn func(string) (*WorktreeInfo, error)) {
	t.Helper()
	orig := detectWorktreeFn
	detectWorktreeFn = fn
	t.Cleanup(func() { detectWorktreeFn = orig })
}

func TestDetectWorktreeBounded_BlockedPathDoesNotStall(t *testing.T) {
	release := make(chan struct{})
	var stuckCalls atomic.Int32
	stubDetect(t, func(path string) (*WorktreeInfo, error) {
		if path == "/stuck" {
			stuckCalls.Add(1)
			<-release
		}
		return &WorktreeInfo{}, nil
	})
	t.Cleanup(func() { inflightWorktreeDetections.Delete("/stuck") })

	withWatchdog(t, func() {
		if _, err := detectWorktreeBounded("/stuck", 20*time.Millisecond); err == nil {
			t.Error("expected timeout error")
		}
		if info, err := detectWorktreeBounded("/ok", 20*time.Millisecond); err != nil || info == nil {
			t.Errorf("readable path: info=%v err=%v", info, err)
		}
		if _, err := detectWorktreeBounded("/stuck", 20*time.Millisecond); err == nil {
			t.Error("expected fast failure for still-stuck path")
		}
	})
	if n := stuckCalls.Load(); n != 1 {
		t.Fatalf("stuck path detections = %d, want 1", n)
	}
	close(release)
	waitUntil(t, func() bool { _, ok := inflightWorktreeDetections.Load("/stuck"); return !ok })
}

func TestDetectWorktreeBounded_ConcurrentCallersSpawnOneGoroutine(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	stubDetect(t, func(string) (*WorktreeInfo, error) {
		calls.Add(1)
		<-release
		return &WorktreeInfo{}, nil
	})
	t.Cleanup(func() { inflightWorktreeDetections.Delete("/c") })

	withWatchdog(t, func() {
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := detectWorktreeBounded("/c", 30*time.Millisecond); err == nil {
					t.Error("expected error")
				}
			}()
		}
		wg.Wait()
	})
	if n := calls.Load(); n != 1 {
		t.Fatalf("detections = %d, want exactly 1", n)
	}
	close(release)
	waitUntil(t, func() bool { _, ok := inflightWorktreeDetections.Load("/c"); return !ok })
}

func TestStatBounded_TimesOut(t *testing.T) {
	release := make(chan struct{})
	orig := statFn
	var calls atomic.Int32
	statFn = func(string) (fs.FileInfo, error) { calls.Add(1); <-release; return nil, nil }
	t.Cleanup(func() {
		close(release)
		waitUntil(t, func() bool { _, ok := inflightStats.Load("/x"); return !ok })
		statFn = orig
	})
	withWatchdog(t, func() {
		for range 3 {
			if _, timedOut := statBounded("/x", 20*time.Millisecond); !timedOut {
				t.Error("expected timeout")
			}
		}
	})
	if n := calls.Load(); n != 1 {
		t.Fatalf("stat calls = %d, want 1 (single-flight)", n)
	}
}

// A session whose detection blocks loads promptly with GitHub info empty and
// unpersisted; once the abandoned call returns it has filled detectWorktreeCache,
// so the next load applies the result instantly.
func TestFromInstanceData_BlockedDetectionDoesNotStallLoad(t *testing.T) {
	const repo = "/stuck-repo"
	release := make(chan struct{})
	origTimeout, origUncached := startupWorktreeDetectTimeout, detectWorktreeUncachedFn
	startupWorktreeDetectTimeout = 20 * time.Millisecond
	detectWorktreeUncachedFn = func(path string) (*WorktreeInfo, error) {
		if path == repo {
			<-release
			return &WorktreeInfo{GitHubOwner: "late", GitHubRepo: "repo"}, nil
		}
		return &WorktreeInfo{}, nil
	}
	t.Cleanup(func() {
		waitUntil(t, func() bool { _, ok := inflightWorktreeDetections.Load(repo); return !ok })
		startupWorktreeDetectTimeout, detectWorktreeUncachedFn = origTimeout, origUncached
		detectWorktreeCache.Lock()
		delete(detectWorktreeCache.m, repo)
		delete(detectWorktreeCache.m, "/fine-repo")
		detectWorktreeCache.Unlock()
	})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	now := time.Now()
	mk := func(title, path string) InstanceData {
		return InstanceData{Title: title, Path: path, Status: Paused, CreatedAt: now, UpdatedAt: now, Program: "claude"}
	}
	var stuck, fine, again *Instance
	withWatchdog(t, func() {
		var err error
		if stuck, err = FromInstanceData(mk("stuck", repo)); err != nil {
			t.Errorf("stuck: %v", err)
		}
		if fine, err = FromInstanceData(mk("fine", "/fine-repo")); err != nil {
			t.Errorf("fine: %v", err)
		}
	})
	if stuck == nil || fine == nil {
		t.Fatal("instances not restored")
	}
	if d := stuck.ToInstanceData(); d.GitHubOwner != "" || d.GitHubRepo != "" {
		t.Fatalf("GitHub info set after timeout: %q/%q", d.GitHubOwner, d.GitHubRepo)
	}

	close(release)
	waitUntil(t, func() bool { _, ok := inflightWorktreeDetections.Load(repo); return !ok })
	waitUntil(t, func() bool {
		detectWorktreeCache.RLock()
		defer detectWorktreeCache.RUnlock()
		_, ok := detectWorktreeCache.m[repo]
		return ok
	})
	withWatchdog(t, func() {
		var err error
		if again, err = FromInstanceData(mk("again", repo)); err != nil {
			t.Errorf("again: %v", err)
		}
	})
	if d := again.ToInstanceData(); d.GitHubOwner != "late" || d.GitHubRepo != "repo" {
		t.Fatalf("next load got %q/%q, want late/repo", d.GitHubOwner, d.GitHubRepo)
	}
}
