package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkflow_EnabledField_DefaultsTrue verifies the ent schema default for the new
// enabled field (webhook-triggers verify follow-ups AC0-3): a row inserted without
// explicitly setting enabled (bypassing EntWorkflowRepository.Create, which always sets
// it explicitly) picks up the schema's Default(true) at the SQL layer.
func TestWorkflow_EnabledField_DefaultsTrue(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	wf, err := storage.GetEntClient().Workflow.Create().
		SetSlug("schema-default-wf").
		SetName("Schema Default WF").
		SetCommand("cmd").
		SetTargetDirectory("/tmp/test").
		Save(t.Context())
	require.NoError(t, err)

	assert.True(t, wf.Enabled, "enabled must default to true for a row that never explicitly sets it")
}

// TestEntWorkflowRepository_Update_should_ClearWebhookSlugWithoutUniqueConstraintViolation_When_TwoWorkflowsBothClearedInSequence
// covers the sdd:6-verify finding: Update previously called
// u.SetWebhookSlug(*w.WebhookSlug) unconditionally whenever the pointer was
// non-nil, including when the caller sent "" to clear it. webhook_slug is
// .Optional().Unique() but not .Nillable() (session/ent/schema/workflow.go), so
// two workflows both set to the literal empty string collide under the unique
// index — while Create correctly leaves the column NULL for an empty slug
// (guarded by `if w.WebhookSlug != ""`), Update did not mirror that guard. The
// fix routes an empty-string update through ClearWebhookSlug (NULL) instead of
// SetWebhookSlug(""), so clearing two separate workflows' webhook_slug in
// sequence must both succeed.
func TestEntWorkflowRepository_Update_should_ClearWebhookSlugWithoutUniqueConstraintViolation_When_TwoWorkflowsBothClearedInSequence(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntWorkflowRepository(storage.GetEntClient())

	wf1, err := repo.Create(t.Context(), WorkflowCreateInput{
		Slug:            "clear-slug-wf-1",
		Name:            "Clear Slug WF 1",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		WebhookSlug:     "wf1-hook",
	})
	require.NoError(t, err)

	wf2, err := repo.Create(t.Context(), WorkflowCreateInput{
		Slug:            "clear-slug-wf-2",
		Name:            "Clear Slug WF 2",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
		WebhookSlug:     "wf2-hook",
	})
	require.NoError(t, err)

	empty := ""

	// Clearing the first workflow's webhook_slug must succeed and leave the
	// column NULL (not "").
	updated1, err := repo.Update(t.Context(), wf1.ID, WorkflowUpdateInput{WebhookSlug: &empty})
	require.NoError(t, err)
	require.Empty(t, updated1.WebhookSlug)

	// Clearing the second workflow's webhook_slug must ALSO succeed — before the
	// fix, this would fail with a unique-constraint violation because the first
	// clear left "" (not NULL) in the column.
	updated2, err := repo.Update(t.Context(), wf2.ID, WorkflowUpdateInput{WebhookSlug: &empty})
	require.NoError(t, err, "clearing a second workflow's webhook_slug must not collide with the first workflow's cleared slug")
	require.Empty(t, updated2.WebhookSlug)
}

// TestEntWorkflowRepository_Update_should_SetWebhookSlug_When_NonEmptyValueProvided
// is the companion happy-path case: a non-empty webhook_slug update still goes
// through SetWebhookSlug and round-trips correctly.
func TestEntWorkflowRepository_Update_should_SetWebhookSlug_When_NonEmptyValueProvided(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntWorkflowRepository(storage.GetEntClient())

	wf, err := repo.Create(t.Context(), WorkflowCreateInput{
		Slug:            "set-slug-wf",
		Name:            "Set Slug WF",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)
	require.Empty(t, wf.WebhookSlug)

	newSlug := "newly-set-hook"
	updated, err := repo.Update(t.Context(), wf.ID, WorkflowUpdateInput{WebhookSlug: &newSlug})
	require.NoError(t, err)
	require.Equal(t, newSlug, updated.WebhookSlug)
}
