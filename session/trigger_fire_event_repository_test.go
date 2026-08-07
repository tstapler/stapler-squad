package session

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntTriggerFireEventRepository_Create_should_PersistAndRoundTrip_When_ValidInput(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	wfRepo := NewEntWorkflowRepository(storage.GetEntClient())
	wf, err := wfRepo.Create(t.Context(), WorkflowCreateInput{
		Slug:            "trigger-fire-wf",
		Name:            "Trigger Fire WF",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	repo := NewEntTriggerFireEventRepository(storage.GetEntClient())
	err = repo.Create(t.Context(), TriggerFireEventInput{
		WorkflowID: &wf.ID,
		Outcome:    "fired_success",
		DeliveryID: "delivery-1",
		SessionID:  "sess-1",
	})
	require.NoError(t, err)

	events, err := repo.ListByWorkflow(t.Context(), wf.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "fired_success", events[0].Outcome)
	assert.Equal(t, "delivery-1", events[0].DeliveryID)
	assert.Equal(t, "sess-1", events[0].SessionID)
	require.NotNil(t, events[0].WorkflowID)
	assert.Equal(t, wf.ID, *events[0].WorkflowID)
}

// TestEntTriggerFireEventRepository_Create_should_ReturnErrDuplicateDelivery_When_SameWorkflowAndDeliveryIDRepeats
// verifies AC12 / pre-mortem P1 #1: a second Create for the SAME (workflow_id,
// delivery_id) pair fails with the typed ErrDuplicateDelivery via the composite
// unique index, not a silent duplicate insert.
func TestEntTriggerFireEventRepository_Create_should_ReturnErrDuplicateDelivery_When_SameWorkflowAndDeliveryIDRepeats(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	wfRepo := NewEntWorkflowRepository(storage.GetEntClient())
	wf, err := wfRepo.Create(t.Context(), WorkflowCreateInput{
		Slug:            "dedup-wf",
		Name:            "Dedup WF",
		Command:         "cmd",
		TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	repo := NewEntTriggerFireEventRepository(storage.GetEntClient())
	input := TriggerFireEventInput{
		WorkflowID: &wf.ID,
		Outcome:    "fired_success",
		DeliveryID: "dup-delivery",
	}
	require.NoError(t, repo.Create(t.Context(), input))

	err = repo.Create(t.Context(), input)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateDelivery)

	events, err := repo.ListByWorkflow(t.Context(), wf.ID, 10)
	require.NoError(t, err)
	assert.Len(t, events, 1, "the duplicate insert must not have created a second row")
}

// TestEntTriggerFireEventRepository_Create_should_AllowSameDeliveryIDAcrossDifferentWorkflows
// verifies the composite index's scoping (pre-mortem P1 #1 correction): two different
// Workflow rows matching the SAME inbound delivery (e.g. two github_push triggers both
// watching main) must each be able to record their own fire event — a bare global
// unique index on delivery_id alone would incorrectly collide here.
func TestEntTriggerFireEventRepository_Create_should_AllowSameDeliveryIDAcrossDifferentWorkflows(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	wfRepo := NewEntWorkflowRepository(storage.GetEntClient())
	wfA, err := wfRepo.Create(t.Context(), WorkflowCreateInput{
		Slug: "watcher-a", Name: "Watcher A", Command: "cmd", TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)
	wfB, err := wfRepo.Create(t.Context(), WorkflowCreateInput{
		Slug: "watcher-b", Name: "Watcher B", Command: "cmd", TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	repo := NewEntTriggerFireEventRepository(storage.GetEntClient())
	require.NoError(t, repo.Create(t.Context(), TriggerFireEventInput{
		WorkflowID: &wfA.ID, Outcome: "fired_success", DeliveryID: "shared-delivery",
	}))
	require.NoError(t, repo.Create(t.Context(), TriggerFireEventInput{
		WorkflowID: &wfB.ID, Outcome: "fired_success", DeliveryID: "shared-delivery",
	}))

	eventsA, err := repo.ListByWorkflow(t.Context(), wfA.ID, 10)
	require.NoError(t, err)
	assert.Len(t, eventsA, 1)

	eventsB, err := repo.ListByWorkflow(t.Context(), wfB.ID, 10)
	require.NoError(t, err)
	assert.Len(t, eventsB, 1)
}

// TestEntTriggerFireEventRepository_Create_should_AllowMultipleRowsWithNoDeliveryID
// verifies that rows without a meaningful delivery ID (e.g. cron fires) never collide
// with each other — delivery_id is left unset (NULL), and SQL treats distinct NULLs as
// non-colliding under the composite unique index.
func TestEntTriggerFireEventRepository_Create_should_AllowMultipleRowsWithNoDeliveryID(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	wfRepo := NewEntWorkflowRepository(storage.GetEntClient())
	wf, err := wfRepo.Create(t.Context(), WorkflowCreateInput{
		Slug: "cron-only-wf", Name: "Cron Only", Command: "cmd", TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	repo := NewEntTriggerFireEventRepository(storage.GetEntClient())
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(t.Context(), TriggerFireEventInput{
			WorkflowID: &wf.ID,
			Outcome:    "fired_success",
		}))
	}

	events, err := repo.ListByWorkflow(t.Context(), wf.ID, 10)
	require.NoError(t, err)
	assert.Len(t, events, 3)
}

// TestEntTriggerFireEventRepository_Create_should_RejectExactlyOneConcurrentDuplicate
// exercises the same (workflow_id, delivery_id) pair from two real goroutines
// concurrently — the scenario the unique index (not a pre-check-then-insert) exists to
// make safe: exactly one of the two racing Create calls must succeed.
func TestEntTriggerFireEventRepository_Create_should_RejectExactlyOneConcurrentDuplicate(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	wfRepo := NewEntWorkflowRepository(storage.GetEntClient())
	wf, err := wfRepo.Create(t.Context(), WorkflowCreateInput{
		Slug: "race-wf", Name: "Race WF", Command: "cmd", TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	repo := NewEntTriggerFireEventRepository(storage.GetEntClient())

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = repo.Create(t.Context(), TriggerFireEventInput{
				WorkflowID: &wf.ID,
				Outcome:    "fired_success",
				DeliveryID: "race-delivery",
			})
		}(i)
	}
	wg.Wait()

	successCount := 0
	dupCount := 0
	for _, e := range errs {
		if e == nil {
			successCount++
		} else if errors.Is(e, ErrDuplicateDelivery) {
			dupCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one concurrent Create must succeed")
	assert.Equal(t, 1, dupCount, "the other must fail with ErrDuplicateDelivery")

	events, err := repo.ListByWorkflow(t.Context(), wf.ID, 10)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

// TestEntTriggerFireEventRepository_Create_should_AllowNilWorkflowID_When_RejectedBeforeResolution
// verifies a rejected webhook request (e.g. unknown slug) can still be recorded with no
// resolved Workflow.
func TestEntTriggerFireEventRepository_Create_should_AllowNilWorkflowID_When_RejectedBeforeResolution(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	repo := NewEntTriggerFireEventRepository(storage.GetEntClient())
	err := repo.Create(t.Context(), TriggerFireEventInput{
		Outcome:      "rejected",
		ErrorMessage: "unknown webhook slug",
	})
	require.NoError(t, err)

	// Not fetchable via ListByWorkflow (no workflow_id) — just confirm it inserted
	// without error and without a WorkflowID.
	client := storage.GetEntClient()
	all, err := client.TriggerFireEvent.Query().All(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Nil(t, all[0].WorkflowID)
	assert.Equal(t, "rejected", all[0].Outcome)
}

// TestEntTriggerFireEventRepository_ListByWorkflow_should_DefaultLimitAndOrderNewestFirst
// verifies the default limit (100) and newest-first ordering.
func TestEntTriggerFireEventRepository_ListByWorkflow_should_DefaultLimitAndOrderNewestFirst(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	wfRepo := NewEntWorkflowRepository(storage.GetEntClient())
	wf, err := wfRepo.Create(t.Context(), WorkflowCreateInput{
		Slug: "order-wf", Name: "Order WF", Command: "cmd", TargetDirectory: "/tmp/test",
	})
	require.NoError(t, err)

	repo := NewEntTriggerFireEventRepository(storage.GetEntClient())
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(t.Context(), TriggerFireEventInput{
			WorkflowID: &wf.ID,
			Outcome:    "fired_success",
			DeliveryID: uuid.New().String(),
		}))
	}

	events, err := repo.ListByWorkflow(t.Context(), wf.ID, 0)
	require.NoError(t, err)
	require.Len(t, events, 3)
	for i := 1; i < len(events); i++ {
		assert.True(t, !events[i-1].CreatedAt.Before(events[i].CreatedAt), "events must be ordered newest first")
	}
}
