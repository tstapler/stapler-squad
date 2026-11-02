package session

import (
	"testing"
	"time"
)

// A detection that blocks (e.g. open() on a TCC-protected path) must not stall
// the caller past the timeout, other paths must still resolve, and a repeat
// attempt on the stuck path must fail fast without spawning another goroutine.
func TestDetectWorktreeBounded_BlockedPathDoesNotStall(t *testing.T) {
	release := make(chan struct{})
	calls := make(chan string, 8)
	orig := detectWorktreeFn
	detectWorktreeFn = func(path string) (*WorktreeInfo, error) {
		calls <- path
		if path == "/stuck" {
			<-release
		}
		return &WorktreeInfo{}, nil
	}
	t.Cleanup(func() {
		detectWorktreeFn = orig
		stuckWorktreeDetections.Delete("/stuck")
	})

	start := time.Now()
	if _, err := detectWorktreeBounded("/stuck", 20*time.Millisecond); err == nil {
		t.Fatal("expected timeout error for blocked path")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("blocked path stalled caller for %s", d)
	}

	if info, err := detectWorktreeBounded("/ok", 20*time.Millisecond); err != nil || info == nil {
		t.Fatalf("readable path: info=%v err=%v", info, err)
	}

	before := len(calls)
	if _, err := detectWorktreeBounded("/stuck", 20*time.Millisecond); err == nil {
		t.Fatal("expected fast failure for still-stuck path")
	}
	if len(calls) != before {
		t.Fatal("stuck path spawned another detection goroutine")
	}

	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, stuck := stuckWorktreeDetections.Load("/stuck"); !stuck {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stuck marker never cleared after detection returned")
		}
		time.Sleep(time.Millisecond)
	}
}

// LoadInstances-equivalent: FromInstanceData for a session whose detection
// blocks returns promptly, and a second session still restores.
func TestFromInstanceData_BlockedDetectionDoesNotStallLoad(t *testing.T) {
	release := make(chan struct{})
	orig, origTimeout := detectWorktreeFn, startupWorktreeDetectTimeout
	detectWorktreeFn = func(path string) (*WorktreeInfo, error) {
		if path == "/stuck-repo" {
			<-release
		}
		return &WorktreeInfo{}, nil
	}
	startupWorktreeDetectTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		close(release)
		detectWorktreeFn, startupWorktreeDetectTimeout = orig, origTimeout
		stuckWorktreeDetections.Delete("/stuck-repo")
	})

	now := time.Now()
	mk := func(title, path string) InstanceData {
		return InstanceData{Title: title, Path: path, Status: Paused, CreatedAt: now, UpdatedAt: now, Program: "claude"}
	}
	start := time.Now()
	for _, d := range []InstanceData{mk("stuck", "/stuck-repo"), mk("fine", "/fine-repo")} {
		inst, err := FromInstanceData(d)
		if err != nil || inst == nil {
			t.Fatalf("%s: inst=%v err=%v", d.Title, inst, err)
		}
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("load stalled for %s", d)
	}
}
