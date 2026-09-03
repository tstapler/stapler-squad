package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
)

// TestMaybeAutoArchive_ArchivesBacklogOriginatedSession is the regression test for the
// session-retention-cleanup fix: maybeAutoArchive previously only archived sessions
// spawned by the workflow system (WorkflowID != ""). Backlog-spawned sessions (tagged
// backlog:work / backlog:review) have no WorkflowID and were never archived by this
// fallback path, relying entirely on BacklogService.archiveItemWorkSessions firing on
// the owning item's terminal/rework transition — which never happens for an item stuck
// or abandoned before reaching either. Those sessions then accumulate forever, since
// SessionRetentionSweeper only ever considers sessions with ArchivedAt set.
func TestMaybeAutoArchive_ArchivesBacklogOriginatedSession(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	inst := &session.Instance{
		Title:     "backlog-work-session",
		Path:      "/tmp/test",
		Status:    session.Stopped,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Tags:      []string{session.TagBacklogWork, session.TagAutonomous},
	}

	fix.svc.maybeAutoArchive(inst)

	snap := inst.Snapshot()
	assert.NotNil(t, snap.ArchivedAt, "expected backlog-originated session to be auto-archived on exit")
}

// TestMaybeAutoArchive_IgnoresPlainSession guards the other side of the same change:
// a session with no WorkflowID and no backlog tag (a plain user-created session) must
// not be auto-archived just because it stopped.
func TestMaybeAutoArchive_IgnoresPlainSession(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	inst := &session.Instance{
		Title:     "plain-session",
		Path:      "/tmp/test",
		Status:    session.Stopped,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	fix.svc.maybeAutoArchive(inst)

	snap := inst.Snapshot()
	assert.Nil(t, snap.ArchivedAt, "expected a plain, non-backlog, non-workflow session to be left unarchived")
}
