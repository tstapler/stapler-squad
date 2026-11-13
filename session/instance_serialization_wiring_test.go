package session

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

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
		status      domain.Status
		archived    bool
		wantStatus  domain.Status
		wantStarted bool
	}{
		{name: "active_archived_heals_to_stopped", status: domain.Active, archived: true, wantStatus: domain.Stopped, wantStarted: true},
		{name: "creating_archived_heals_to_stopped", status: domain.Creating, archived: true, wantStatus: domain.Stopped, wantStarted: true},
		{name: "restoring_archived_preserves_status", status: domain.Restoring, archived: true, wantStatus: domain.Restoring, wantStarted: true},
		{name: "permanently_failed_archived_preserves_status", status: domain.PermanentlyFailed, archived: true, wantStatus: domain.PermanentlyFailed, wantStarted: true},
		{name: "failed_archived_preserves_status", status: domain.Failed, archived: true, wantStatus: domain.Failed, wantStarted: true},
		// Controls: an unarchived row is untouched, and Paused is handled by its
		// own earlier branch so the new guard must not reach it.
		{name: "active_not_archived_control", status: domain.Active, archived: false, wantStatus: domain.Active, wantStarted: false},
		{name: "paused_archived_control", status: domain.Paused, archived: true, wantStatus: domain.Paused, wantStarted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data := domain.InstanceData{
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
			if tb, ok := restored.ProcessManagerForTest().(*TmuxBackend); ok {
				if tb.TmuxManager() == nil {
					t.Error("expected the tmux session object to be wired even for an archived session")
				}
			}
		})
	}
}
