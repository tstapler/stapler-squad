package session

import (
	"context"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func enrichWatchdog(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("watchdog: test body blocked")
	}
}

func enrichWaitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}

func enrichInstance(t *testing.T, title, path string, status Status) *Instance {
	t.Helper()
	now := time.Now()
	inst, err := FromInstanceDataDeferred(InstanceData{Title: title, Path: path, Status: status, CreatedAt: now, UpdatedAt: now, Program: "claude"})
	if err != nil {
		t.Fatalf("FromInstanceData(%s): %v", title, err)
	}
	return inst
}

func owner(i *Instance) string { return i.Snapshot().GitHub.GitHubOwner }

func ghInfo(string) WorktreeProbeResult {
	return WorktreeProbeResult{Info: &WorktreeInfo{GitHubOwner: "o", GitHubRepo: "r"}}
}

func startEnricher(t *testing.T, cfg WorktreeEnricherConfig) *WorktreeEnricher {
	t.Helper()
	e := NewWorktreeEnricher(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e.Start(ctx)
	return e
}

// LoadInstances' restore step must do no filesystem probing: a session whose
// worktree path does not exist stays un-paused and un-enriched until the
// enricher runs.
func TestFromInstanceData_DoesNoFilesystemProbing(t *testing.T) {
	t.Parallel()
	now := time.Now()
	data := InstanceData{Title: "noprobe", Path: "/nonexistent/repo", Status: Ready, CreatedAt: now, UpdatedAt: now, Program: "claude"}
	data.Worktree.WorktreePath = "/nonexistent/worktree"
	data.Worktree.RepoPath = "/nonexistent/repo"
	inst, err := FromInstanceDataDeferred(data)
	if err != nil {
		t.Fatal(err)
	}
	if inst.Paused() {
		t.Fatal("load paused a session by probing the filesystem")
	}
	if owner(inst) != "" {
		t.Fatal("load populated GitHub info")
	}
}

func TestWorktreeEnricher_StuckPathDoesNotBlockOthers(t *testing.T) {
	release := make(chan struct{})
	var stuckProbes atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Workers: 2, Deadline: 20 * time.Millisecond,
		Probe: func(path string) WorktreeProbeResult {
			if path == "/stuck" {
				stuckProbes.Add(1)
				<-release
			}
			return ghInfo(path)
		},
		AfterFunc: func(time.Duration, func()) {}, // retries driven manually elsewhere
	})
	t.Cleanup(func() { close(release) })

	stuck := enrichInstance(t, "stuck", "/stuck", Paused)
	a := enrichInstance(t, "a", "/a", Paused)
	b := enrichInstance(t, "b", "/b", Paused)
	enrichWatchdog(t, func() {
		for _, i := range []*Instance{stuck, a, b} {
			e.Enqueue(i)
		}
		for _, i := range []*Instance{stuck, a, b} {
			e.Await(i, 5*time.Second)
		}
	})
	if owner(a) != "o" || owner(b) != "o" {
		t.Fatalf("healthy paths not enriched: %q %q", owner(a), owner(b))
	}
	if owner(stuck) != "" {
		t.Fatal("stuck session enriched")
	}
	if st := e.Stats(); st.TimedOut != 1 || st.Completed != 2 {
		t.Fatalf("stats = %+v, want 1 timed out, 2 completed", st)
	}
	if n := stuckProbes.Load(); n != 1 {
		t.Fatalf("stuck path probed %d times, want 1", n)
	}
}

func TestWorktreeEnricher_RetryAppliesLateResultThroughActor(t *testing.T) {
	release := make(chan struct{})
	var now atomic.Int64
	base := time.Now()
	var retry atomic.Pointer[func()]
	var persisted atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Deadline: 20 * time.Millisecond,
		Persist:  func(*Instance) error { persisted.Add(1); return nil },
		Now:      func() time.Time { return base.Add(time.Duration(now.Load())) },
		Probe: func(path string) WorktreeProbeResult {
			<-release
			return ghInfo(path)
		},
		AfterFunc: func(_ time.Duration, f func()) { retry.Store(&f) },
	})
	inst := enrichInstance(t, "late", "/late", Paused)
	li := NewLiveInstance(inst)
	t.Cleanup(li.Stop)

	enrichWatchdog(t, func() {
		e.Enqueue(inst)
		e.Await(inst, 5*time.Second)
	})
	if owner(inst) != "" || retry.Load() == nil {
		t.Fatal("expected a timeout with a scheduled retry")
	}
	close(release)
	now.Store(int64(time.Hour))
	(*retry.Load())()
	enrichWaitUntil(t, func() bool { return owner(inst) == "o" })
	if persisted.Load() != 1 {
		t.Fatalf("persisted %d times, want 1", persisted.Load())
	}
}

func TestWorktreeEnricher_SkipsArchivedAndTerminalSessions(t *testing.T) {
	var probes atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Probe: func(path string) WorktreeProbeResult { probes.Add(1); return ghInfo(path) },
	})
	archived := enrichInstance(t, "archived", "/arch", Paused)
	at := time.Now()
	archived.SetArchivedAt(&at)
	stopped := enrichInstance(t, "stopped", "/stopped", Stopped)
	live := enrichInstance(t, "live", "/live", Paused)
	enrichWatchdog(t, func() {
		for _, i := range []*Instance{archived, stopped, live} {
			e.Enqueue(i)
		}
		e.Await(live, 5*time.Second)
	})
	if probes.Load() != 1 {
		t.Fatalf("probes = %d, want 1 (only the live session)", probes.Load())
	}
	if owner(archived) != "" || owner(stopped) != "" {
		t.Fatal("archived/terminal session was touched")
	}
}

func TestWorktreeEnricher_PausesSessionWithMissingWorktree(t *testing.T) {
	now := time.Now()
	data := InstanceData{Title: "gone", Path: "/repo", Status: Ready, CreatedAt: now, UpdatedAt: now, Program: "claude"}
	data.Worktree.WorktreePath = "/gone/worktree"
	data.Worktree.RepoPath = "/repo"
	inst, err := FromInstanceDataDeferred(data)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var probed []string
	e := startEnricher(t, WorktreeEnricherConfig{
		Probe: func(path string) WorktreeProbeResult {
			mu.Lock()
			probed = append(probed, path)
			mu.Unlock()
			return WorktreeProbeResult{Info: &WorktreeInfo{}, StatErr: fs.ErrNotExist}
		},
	})
	enrichWatchdog(t, func() { e.Enqueue(inst); e.Await(inst, 5*time.Second) })
	if !inst.Paused() || !inst.Started() {
		t.Fatalf("paused=%v started=%v, want both true", inst.Paused(), inst.Started())
	}
	if len(probed) != 1 || probed[0] != "/gone/worktree" {
		t.Fatalf("probed %v, want the worktree path", probed)
	}
}

func TestWorktreeEnricher_ParkedPathWaitsForUnpark(t *testing.T) {
	var ready atomic.Bool
	var probes atomic.Int32
	e := startEnricher(t, WorktreeEnricherConfig{
		Ready: func(string) bool { return ready.Load() },
		Probe: func(path string) WorktreeProbeResult { probes.Add(1); return ghInfo(path) },
	})
	inst := enrichInstance(t, "parked", "/parked", Paused)
	enrichWatchdog(t, func() { e.Enqueue(inst); e.Await(inst, 5*time.Second) })
	if probes.Load() != 0 || owner(inst) != "" {
		t.Fatal("parked path was probed")
	}
	ready.Store(true)
	e.Unpark("/parked")
	enrichWaitUntil(t, func() bool { return owner(inst) == "o" })
}
