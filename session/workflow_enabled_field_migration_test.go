package session

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(v bool) *bool { return &v }

func TestWorkflowEnabledColumnPreexisted_should_ReturnFalse_When_TableDoesNotExist(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	assert.False(t, workflowEnabledColumnPreexisted(db))
}

func TestWorkflowEnabledColumnPreexisted_should_ReturnFalse_When_TableExistsWithoutEnabledColumn(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, slug TEXT, cron_enabled BOOLEAN)`)
	require.NoError(t, err)

	assert.False(t, workflowEnabledColumnPreexisted(db))
}

func TestWorkflowEnabledColumnPreexisted_should_ReturnTrue_When_EnabledColumnAlreadyExists(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE workflows (id TEXT PRIMARY KEY, slug TEXT, cron_enabled BOOLEAN, enabled BOOLEAN)`)
	require.NoError(t, err)

	assert.True(t, workflowEnabledColumnPreexisted(db))
}

// TestRunWorkflowEnabledFieldBackfill_should_CorrectLegacyDisabledRowsOnly verifies the
// correction logic itself: a row that predates the enabled field (backfilled to the
// schema default true) but had cron_enabled=false under the old overloaded "cron_enabled
// is also the generic enable flag" semantics is corrected to enabled=false; a row with
// cron_enabled=true is left at enabled=true untouched.
func TestRunWorkflowEnabledFieldBackfill_should_CorrectLegacyDisabledRowsOnly(t *testing.T) {
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	wfRepo := NewEntWorkflowRepository(repo.GetEntClient())

	// Simulates a pre-migration row: cron_enabled=false meant "disabled" under the old
	// semantics, but ent's auto-migration backfilled enabled to its schema default
	// (true) uniformly.
	legacyDisabled, err := wfRepo.Create(ctx, WorkflowCreateInput{
		Slug: "legacy-disabled", Name: "Legacy Disabled", Command: "cmd", TargetDirectory: "/tmp/test",
		CronEnabled: false, Enabled: boolPtr(true),
	})
	require.NoError(t, err)

	legacyEnabled, err := wfRepo.Create(ctx, WorkflowCreateInput{
		Slug: "legacy-enabled", Name: "Legacy Enabled", Command: "cmd", TargetDirectory: "/tmp/test",
		CronEnabled: true, Enabled: boolPtr(true),
	})
	require.NoError(t, err)

	runWorkflowEnabledFieldBackfill(ctx, repo)

	got, err := wfRepo.GetByID(ctx, legacyDisabled.ID)
	require.NoError(t, err)
	assert.False(t, got.Enabled, "a legacy row with cron_enabled=false must be corrected to enabled=false")

	got2, err := wfRepo.GetByID(ctx, legacyEnabled.ID)
	require.NoError(t, err)
	assert.True(t, got2.Enabled, "a row with cron_enabled=true must be left at enabled=true")
}

// TestNewEntRepository_should_NotReDisableLegitimatelyEnabledTrigger_When_ReopenedAcrossRestart
// is the regression test for the BLOCKER found during sdd:6-verify's architecture review:
// backfillEnabledField's old, unconditional-on-every-Start() invocation could not
// distinguish "row still at the migration-artifact default" from "row a legitimately
// enabled webhook/github_push trigger settles into permanently" — both look identical
// (enabled=true, cron_enabled=false) — so it silently re-disabled every enabled trigger
// on every server restart, forever. This proves the fix: opening the SAME database file
// a second time (simulating a restart) must not touch a row created after the first open,
// because workflowEnabledColumnPreexisted now sees the enabled column already exists and
// skips the backfill entirely on the second (and every subsequent) open.
func TestNewEntRepository_should_NotReDisableLegitimatelyEnabledTrigger_When_ReopenedAcrossRestart(t *testing.T) {
	dbPath := t.TempDir() + "/restart_test.db"

	// First "boot": schema.Create() adds the enabled column for the first time in this
	// database's life. workflowEnabledColumnPreexisted is false here, so the backfill
	// runs — harmlessly, since no rows exist yet.
	first, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)

	wfRepo := NewEntWorkflowRepository(first.GetEntClient())
	created, err := wfRepo.Create(context.Background(), WorkflowCreateInput{
		Slug: "legit-enabled-webhook", Name: "Legit Enabled", Command: "cmd", TargetDirectory: "/tmp/test",
		TriggerType: "webhook", WebhookSlug: "legit-enabled-webhook",
		CronEnabled: false,         // vestigial for a webhook trigger — the real gate is Enabled
		Enabled:     boolPtr(true), // the operator's actual, intended, permanent state
	})
	require.NoError(t, err)
	require.True(t, created.Enabled)
	require.NoError(t, first.client.Close())

	// Second "boot": reopen the SAME database file, simulating a server restart. The
	// enabled column now already exists, so workflowEnabledColumnPreexisted must return
	// true and the backfill must be skipped entirely.
	second, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer second.client.Close()

	reloaded, err := NewEntWorkflowRepository(second.GetEntClient()).GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.True(t, reloaded.Enabled,
		"a legitimately-enabled trigger must survive a restart — the enabled backfill must not re-run once the column already exists")
}
