package session

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The webhook-management tables are purely additive: ent's Schema.Create adds them on
// startup and nothing is backfilled. This test starts from the shape of a database
// that predates them (the three new tables dropped, every existing row kept), reopens
// it with current code, and proves the upgrade recreates the tables without touching
// the workflows -- including webhook workflows -- that already existed.
func TestWebhookManagementTables_should_BeAddedWithoutTouchingExistingWorkflows_When_DatabasePredatesThem(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	user, err := repo.client.Workflow.Create().
		SetSlug("users-own-hook").SetName("user's hook").SetCommand("user command").
		SetTargetDirectory("/home/user").SetTriggerType("webhook").SetWebhookSlug("users-own-hook").
		SetWebhookSecretEncrypted("user-encrypted-secret").SetEventFilter("push").Save(ctx)
	require.NoError(t, err)
	plain, err := repo.client.Workflow.Create().
		SetSlug("plain-cron").SetName("cron").SetCommand("c").SetTargetDirectory("/tmp").
		SetTriggerType("cron").SetCronExpression("0 * * * *").SetCronEnabled(true).Save(ctx)
	require.NoError(t, err)
	managed, err := repo.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	raw, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	for _, table := range []string{"webhook_request_ledgers", "webhook_registrations", "integration_credentials"} {
		_, err = raw.Exec("DROP TABLE " + table)
		require.NoError(t, err, "dropping %s to simulate a pre-upgrade database", table)
	}
	require.NoError(t, raw.Close())

	for attempt := 1; attempt <= 2; attempt++ { // the second open proves the migration is idempotent
		upgraded, err := NewEntRepository(WithDatabasePath(dbPath))
		require.NoError(t, err, "open attempt %d", attempt)

		for name, count := range map[string]func() (int, error){
			"webhook_registrations":   func() (int, error) { return upgraded.client.WebhookRegistration.Query().Count(ctx) },
			"webhook_request_ledgers": func() (int, error) { return upgraded.client.WebhookRequestLedger.Query().Count(ctx) },
			"integration_credentials": func() (int, error) { return upgraded.client.IntegrationCredential.Query().Count(ctx) },
		} {
			n, err := count()
			require.NoError(t, err, "%s must exist after the upgrade", name)
			assert.Zero(t, n, "%s starts empty", name)
		}

		got, err := upgraded.client.Workflow.Get(ctx, user.ID)
		require.NoError(t, err)
		assert.Equal(t, "user-encrypted-secret", got.WebhookSecretEncrypted)
		assert.Equal(t, "push", got.EventFilter)
		assert.Equal(t, "webhook", got.TriggerType)
		cron, err := upgraded.client.Workflow.Get(ctx, plain.ID)
		require.NoError(t, err)
		assert.True(t, cron.CronEnabled)

		// A workflow whose registration row is gone is now simply user-managed: the API
		// must neither adopt nor modify it.
		_, err = upgraded.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-after-upgrade"))
		assert.ErrorIs(t, err, ErrWebhookSlugUnavailable)
		orphan, err := upgraded.client.Workflow.Get(ctx, managed.Registration.WorkflowID)
		require.NoError(t, err)
		assert.Equal(t, "gh-signal main", orphan.Name)

		require.NoError(t, upgraded.Close())
	}
}
