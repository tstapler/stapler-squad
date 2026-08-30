package session

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

// TestToInstanceData_PreservesBackend verifies that Instance.Backend (a
// per-session ProcessManager backend pin, e.g. BackendTymux) survives the
// ToInstanceData -> FromInstanceData round trip (Epic 5.1's persistence
// story) instead of silently reverting to the process-wide default across a
// process restart.
func TestToInstanceData_PreservesBackend(t *testing.T) {
	t.Parallel()
	now := time.Now()

	instance := &Instance{
		Title:     "backend-roundtrip",
		Path:      "/path/to/repo",
		Status:    Paused,
		CreatedAt: now,
		UpdatedAt: now,
		Program:   "claude",
		Backend:   BackendTymux,
	}

	data := instance.ToInstanceData()
	if data.Backend != BackendTymux {
		t.Fatalf("ToInstanceData(): expected Backend %q, got %q", BackendTymux, data.Backend)
	}

	restored, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData: %v", err)
	}
	if restored.Backend != BackendTymux {
		t.Fatalf("FromInstanceData(): expected Backend %q, got %q", BackendTymux, restored.Backend)
	}
}

// TestFromInstanceData_OldJSONWithoutBackendFieldDefaultsEmpty is the
// backward-compatibility regression test named by Epic 5.1's doc comment
// update: an InstanceData/sessions.json entry written before the "backend"
// field existed has no "backend" key at all. Confirm it unmarshals to the Go
// zero value ("") rather than failing or defaulting to some other value, and
// that a restored instance carries that empty pin through — i.e. it falls
// through to whatever NewProcessManager's process-wide default resolves to,
// with no explicit migration code required.
func TestFromInstanceData_OldJSONWithoutBackendFieldDefaultsEmpty(t *testing.T) {
	t.Parallel()

	// Hand-constructed legacy JSON blob: a real pre-Epic-5.1 sessions.json
	// entry would never contain a "backend" key.
	oldJSON := `{
		"title": "legacy-session",
		"path": "/path/to/repo",
		"status": 2,
		"program": "claude"
	}`

	var data InstanceData
	if err := json.Unmarshal([]byte(oldJSON), &data); err != nil {
		t.Fatalf("Unmarshal legacy InstanceData JSON: %v", err)
	}

	if data.Backend != "" {
		t.Fatalf("expected Backend to default to \"\" for legacy JSON without a backend key, got %q", data.Backend)
	}

	restored, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("FromInstanceData: %v", err)
	}
	if restored.Backend != "" {
		t.Fatalf("expected restored instance Backend to be \"\" (falls through to process-wide default), got %q", restored.Backend)
	}
}

// TestWorktreeMissingLevel_WarnsOnFirstDebugsOnRepeat guards the dedup
// decision fromInstanceData uses when it detects a missing worktree
// directory: Warn the first time a session title has been seen
// (alreadyLogged=false), Debug on any repeat (alreadyLogged=true).
func TestWorktreeMissingLevel_WarnsOnFirstDebugsOnRepeat(t *testing.T) {
	t.Parallel()

	if got := worktreeMissingLevel(false); got != slog.LevelWarn {
		t.Errorf("worktreeMissingLevel(false) = %v, want %v", got, slog.LevelWarn)
	}
	if got := worktreeMissingLevel(true); got != slog.LevelDebug {
		t.Errorf("worktreeMissingLevel(true) = %v, want %v", got, slog.LevelDebug)
	}
}

// TestLoggedMissingWorktree_DedupesAcrossSeparateInstanceObjects guards the
// actual reason this is a package-level map rather than an Instance field:
// session/health.go's ~15s LoadInstances() tick constructs a brand-new
// Instance object from disk every time, so dedup must survive across
// distinct Instance objects sharing the same title, not just repeated calls
// on one object.
func TestLoggedMissingWorktree_DedupesAcrossSeparateInstanceObjects(t *testing.T) {
	title := "dedup-test-" + t.Name()
	t.Cleanup(func() { clearLoggedMissingWorktree(title) })

	_, firstSeen := loggedMissingWorktree.LoadOrStore(title, struct{}{})
	if firstSeen {
		t.Fatalf("expected title %q to be unseen on first check", title)
	}

	// A second, entirely separate check for the same title (simulating a new
	// throwaway Instance object from the next health-check tick) must see it
	// as already logged.
	_, secondSeen := loggedMissingWorktree.LoadOrStore(title, struct{}{})
	if !secondSeen {
		t.Fatal("expected title to be seen as already-logged on the second check")
	}
}

// TestClearLoggedMissingWorktree_AllowsReWarnAfterSessionRecreated verifies
// the cleanup hook Storage.DeleteInstance/DeleteAllInstances call: once a
// title's entry is cleared, the next check for that title warns again,
// rather than leaking forever or permanently suppressing a legitimately new
// session that happens to reuse an old title.
func TestClearLoggedMissingWorktree_AllowsReWarnAfterSessionRecreated(t *testing.T) {
	title := "dedup-clear-test-" + t.Name()
	t.Cleanup(func() { clearLoggedMissingWorktree(title) })

	loggedMissingWorktree.LoadOrStore(title, struct{}{})

	clearLoggedMissingWorktree(title)

	_, alreadyLogged := loggedMissingWorktree.LoadOrStore(title, struct{}{})
	if alreadyLogged {
		t.Fatal("expected title to warn again after its entry was cleared")
	}
}
