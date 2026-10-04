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

// If this fails, an archived row cold-restores a real claude process on boot,
// or the self-heal over-applies and overwrites the failure signal the session
// was archived with — irreversibly, since UnarchiveSession only restores
// ArchivedAt (ADR-001, superseded-rework-session-retirement). started=true is
// what suppresses the restore; server/dependencies.go's Step 6 skips every
// Started() instance.
func TestFromInstanceData_should_NotAutoRestoreAndNormalizeOnlyActiveCreating_When_Archived(t *testing.T) {
	t.Parallel()
	archivedAt := time.Now()

	tests := []struct {
		name        string
		status      Status
		archived    bool
		wantStatus  Status
		wantStarted bool
	}{
		{name: "active_archived_heals_to_stopped", status: Active, archived: true, wantStatus: Stopped, wantStarted: true},
		{name: "creating_archived_heals_to_stopped", status: Creating, archived: true, wantStatus: Stopped, wantStarted: true},
		{name: "restoring_archived_preserves_status", status: Restoring, archived: true, wantStatus: Restoring, wantStarted: true},
		{name: "permanently_failed_archived_preserves_status", status: PermanentlyFailed, archived: true, wantStatus: PermanentlyFailed, wantStarted: true},
		{name: "failed_archived_preserves_status", status: Failed, archived: true, wantStatus: Failed, wantStarted: true},
		// Controls: an unarchived row is untouched, and Paused is handled by its
		// own earlier branch so the new guard must not reach it.
		{name: "active_not_archived_control", status: Active, archived: false, wantStatus: Active, wantStarted: false},
		{name: "paused_archived_control", status: Paused, archived: true, wantStatus: Paused, wantStarted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data := InstanceData{
				Title:   "archived-restore-" + tt.name,
				Path:    "/tmp/test",
				Status:  tt.status,
				Program: "claude",
			}
			if tt.archived {
				data.ArchivedAt = &archivedAt
			}

			// Deferred entry point: wires the tmux session object but never
			// spawns a subprocess, the same path LoadInstances uses.
			restored, err := FromInstanceDataDeferred(data)
			if err != nil {
				t.Fatalf("FromInstanceDataDeferred: %v", err)
			}

			if got := restored.Snapshot().Status; got != tt.wantStatus {
				t.Errorf("Status = %v, want %v", got, tt.wantStatus)
			}
			if got := restored.Started(); got != tt.wantStarted {
				t.Errorf("Started() = %v, want %v", got, tt.wantStarted)
			}
			if tt.archived && restored.Snapshot().ArchivedAt == nil {
				t.Error("ArchivedAt must survive the restore")
			}
			// The tmux session object must still be wired for archived
			// sessions, so KillTmuxPaneOnly / findConfirmedLiveInstance's
			// shadow instance keep working through this same constructor.
			if tb, ok := restored.processManager.(*TmuxBackend); ok {
				if tb.TmuxManager() == nil {
					t.Error("expected the tmux session object to be wired even for an archived session")
				}
			}
		})
	}
}
