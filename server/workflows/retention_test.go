package workflows

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
)

// newTestInfra opens a fresh SQLite database for a test and returns the
// ent client plus a workflow repository backed by the same database.
func newTestInfra(t *testing.T) (repo *session.EntRepository, wfRepo session.WorkflowRepository) {
	t.Helper()
	entRepo, err := session.NewEntRepository(session.WithDatabasePath(t.TempDir() + "/retention_test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { entRepo.Close() })
	return entRepo, session.NewEntWorkflowRepository(entRepo.GetEntClient())
}

// createWorkflowWithRetention creates a workflow with the given retention settings.
func createWorkflowWithRetention(t *testing.T, wfRepo session.WorkflowRepository, slug string, keepSessions, archiveAfterHours int) {
	t.Helper()
	_, err := wfRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:              slug,
		Name:              slug,
		Command:           "echo test",
		TargetDirectory:   "/tmp",
		CronEnabled:       false,
		KeepSessions:      &keepSessions,
		ArchiveAfterHours: &archiveAfterHours,
	})
	require.NoError(t, err)
}

// insertStoppedSessionForWorkflow inserts a stopped session directly via ent.
func insertStoppedSessionForWorkflow(t *testing.T, entRepo *session.EntRepository, workflowID, title string, updatedAt time.Time) {
	t.Helper()
	ctx := context.Background()
	client := entRepo.GetEntClient()
	_, err := client.Session.Create().
		SetTitle(title).
		SetPath("/tmp").
		SetStatus(int(session.Stopped)).
		SetWorkflowID(workflowID).
		SetUpdatedAt(updatedAt).
		SetProgram("claude").
		Save(ctx)
	require.NoError(t, err)
}

// countArchivedSessions returns the number of sessions for the given workflow that have been archived.
func countArchivedSessions(t *testing.T, entRepo *session.EntRepository, workflowID string) int {
	t.Helper()
	ctx := context.Background()
	n, err := entRepo.GetEntClient().Session.Query().
		Where(
			entsession.WorkflowID(workflowID),
			entsession.ArchivedAtNotNil(),
		).
		Count(ctx)
	require.NoError(t, err)
	return n
}

// countActiveSessions returns the number of non-archived sessions for the given workflow.
func countActiveSessions(t *testing.T, entRepo *session.EntRepository, workflowID string) int {
	t.Helper()
	ctx := context.Background()
	n, err := entRepo.GetEntClient().Session.Query().
		Where(
			entsession.WorkflowID(workflowID),
			entsession.ArchivedAtIsNil(),
		).
		Count(ctx)
	require.NoError(t, err)
	return n
}

func TestRunRetentionSweep_ArchiveAfterHours_ArchivesOldSessions(t *testing.T) {
	entRepo, wfRepo := newTestInfra(t)
	ctx := context.Background()

	createWorkflowWithRetention(t, wfRepo, "test-wf", 0, 2) // archive after 2 hours
	wf, err := wfRepo.GetBySlug(ctx, "test-wf")
	require.NoError(t, err)

	// Insert a session stopped 3 hours ago — should be archived.
	old := time.Now().Add(-3 * time.Hour)
	insertStoppedSessionForWorkflow(t, entRepo, wf.ID.String(), "old-run", old)

	// Insert a session stopped 30 minutes ago — should NOT be archived.
	recent := time.Now().Add(-30 * time.Minute)
	insertStoppedSessionForWorkflow(t, entRepo, wf.ID.String(), "recent-run", recent)

	RunRetentionSweep(ctx, entRepo.GetEntClient(), wfRepo)

	assert.Equal(t, 1, countArchivedSessions(t, entRepo, wf.ID.String()), "only old session should be archived")
	assert.Equal(t, 1, countActiveSessions(t, entRepo, wf.ID.String()), "recent session should remain")
}

func TestRunRetentionSweep_KeepSessions_ArchivesExcessSessions(t *testing.T) {
	entRepo, wfRepo := newTestInfra(t)
	ctx := context.Background()

	createWorkflowWithRetention(t, wfRepo, "keep-wf", 2, 0) // keep only 2
	wf, err := wfRepo.GetBySlug(ctx, "keep-wf")
	require.NoError(t, err)

	// Insert 4 stopped sessions. keep_sessions=2 → oldest 2 should be archived.
	for i := 0; i < 4; i++ {
		ts := time.Now().Add(-time.Duration(4-i) * time.Hour)
		insertStoppedSessionForWorkflow(t, entRepo, wf.ID.String(), "run-"+string(rune('A'+i)), ts)
	}

	RunRetentionSweep(ctx, entRepo.GetEntClient(), wfRepo)

	assert.Equal(t, 2, countArchivedSessions(t, entRepo, wf.ID.String()), "should archive oldest 2")
	assert.Equal(t, 2, countActiveSessions(t, entRepo, wf.ID.String()), "should keep newest 2")
}

func TestRunRetentionSweep_Disabled_NothingArchived(t *testing.T) {
	entRepo, wfRepo := newTestInfra(t)
	ctx := context.Background()

	// Both 0 = disabled
	createWorkflowWithRetention(t, wfRepo, "no-retention-wf", 0, 0)
	wf, err := wfRepo.GetBySlug(ctx, "no-retention-wf")
	require.NoError(t, err)

	old := time.Now().Add(-100 * time.Hour)
	insertStoppedSessionForWorkflow(t, entRepo, wf.ID.String(), "very-old-run", old)

	RunRetentionSweep(ctx, entRepo.GetEntClient(), wfRepo)

	assert.Equal(t, 0, countArchivedSessions(t, entRepo, wf.ID.String()), "no sessions should be archived when retention is disabled")
}

func TestRunRetentionSweep_NilGuard_DoesNotPanic(t *testing.T) {
	// Passing nil ent client or nil repo should not panic (StartRetentionEnforcer guards these).
	assert.NotPanics(t, func() {
		StartRetentionEnforcer(context.Background(), nil, nil, time.Second)
	})
}
