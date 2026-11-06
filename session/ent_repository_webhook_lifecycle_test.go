package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// consistentTuple returns provenance whose digest really is the SHA-256 of its bytes, as the
// service always produces. Lifecycle requires this: it verifies the persisted row's integrity.
func consistentTuple() (tuple, digest string) {
	tuple = `{"contract_version":"v1","capability_revision":1}`
	sum := sha256.Sum256([]byte(tuple))
	return tuple, hex.EncodeToString(sum[:])
}

func seedConsistent(t *testing.T, r *EntRepository) *WebhookReconcileResult {
	t.Helper()
	in := baseReconcileInput(scopeA, "seed-req-1")
	in.CompatTuple, in.CompatTupleSHA256 = consistentTuple()
	res, err := r.ReconcileWebhookRegistration(context.Background(), in)
	require.NoError(t, err)
	return res
}

func consistentLifecycle(requestID string, action WebhookLifecycleAction, expected int64) WebhookLifecycleInput {
	tuple, digest := consistentTuple()
	return WebhookLifecycleInput{
		Scope: scopeA, RequestID: requestID, Fingerprint: "fp-" + requestID + "-" + string(action),
		InstanceID: "inst-1", Action: action, ExpectedVersion: expected,
		CompatTuple: tuple, CompatTupleSHA256: digest,
	}
}

func TestApplyWebhookLifecycle_should_DisableOnceAndBumpVersion_When_Requested(t *testing.T) {
	r := NewTestEntRepository(t)
	seedConsistent(t, r)

	res, err := r.ApplyWebhookLifecycle(context.Background(), consistentLifecycle("d-1", WebhookActionDisable, 1))
	require.NoError(t, err)

	assert.Equal(t, WebhookOutcomeDisabled, res.Outcome)
	assert.EqualValues(t, 2, res.Registration.Version)
	assert.False(t, res.Registration.Workflow.Enabled)
	require.NotNil(t, res.ChangedWorkflow)

	again, err := r.ApplyWebhookLifecycle(context.Background(), consistentLifecycle("d-2", WebhookActionDisable, 2))
	require.NoError(t, err)
	assert.Equal(t, WebhookOutcomeUnchanged, again.Outcome, "disabling a disabled registration is a no-op")
	assert.EqualValues(t, 2, again.Registration.Version)
	assert.Nil(t, again.ChangedWorkflow)
}

func TestApplyWebhookLifecycle_should_ReplayStoredResult_When_SameRequestRepeatedEvenAfterStateMoved(t *testing.T) {
	r := NewTestEntRepository(t)
	seedConsistent(t, r)
	in := consistentLifecycle("d-1", WebhookActionDisable, 1)
	first, err := r.ApplyWebhookLifecycle(context.Background(), in)
	require.NoError(t, err)

	replay, err := r.ApplyWebhookLifecycle(context.Background(), in)

	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	assert.Equal(t, first.Outcome, replay.Outcome)
	assert.EqualValues(t, first.Registration.Version, replay.Registration.Version)
	_, _, l := countRows(t, r)
	assert.Equal(t, 2, l, "seed reconcile + one disable; the replay must not write a ledger row")
}

func TestApplyWebhookLifecycle_should_RejectReuse_When_RequestIDCarriesADifferentAction(t *testing.T) {
	r := NewTestEntRepository(t)
	seedConsistent(t, r)
	_, err := r.ApplyWebhookLifecycle(context.Background(), consistentLifecycle("shared-id", WebhookActionDisable, 1))
	require.NoError(t, err)

	_, err = r.ApplyWebhookLifecycle(context.Background(), consistentLifecycle("shared-id", WebhookActionDelete, 2))

	assert.ErrorIs(t, err, ErrIdempotencyKeyReused)
	w, g, _ := countRows(t, r)
	assert.Equal(t, []int{1, 1}, []int{w, g}, "the conflicting delete must not have run")
}

func TestApplyWebhookLifecycle_should_ReturnConflictWithoutMutation_When_ExpectedVersionIsStale(t *testing.T) {
	r := NewTestEntRepository(t)
	seedConsistent(t, r)

	_, err := r.ApplyWebhookLifecycle(context.Background(), consistentLifecycle("d-1", WebhookActionDisable, 7))

	var conflict *WebhookVersionConflictError
	require.True(t, errors.As(err, &conflict))
	assert.EqualValues(t, 1, conflict.CurrentVersion)
	view, err := r.InspectWebhookRegistration(context.Background(), scopeA, "inst-1")
	require.NoError(t, err)
	assert.True(t, view.Workflow.Enabled)
	assert.EqualValues(t, 1, view.Version)
}

func TestApplyWebhookLifecycle_should_RefuseWithoutMutation_When_ProvenanceDiffersOrIsCorrupted(t *testing.T) {
	ctx := context.Background()

	t.Run("request tuple differs from the persisted one", func(t *testing.T) {
		r := NewTestEntRepository(t)
		seedConsistent(t, r)
		in := consistentLifecycle("d-1", WebhookActionDelete, 1)
		in.CompatTuple = `{"contract_version":"v1","tampered":true}`

		_, err := r.ApplyWebhookLifecycle(ctx, in)

		assert.ErrorIs(t, err, ErrCompatTupleMismatch)
		w, g, _ := countRows(t, r)
		assert.Equal(t, []int{1, 1}, []int{w, g})
	})

	t.Run("request hash differs from the persisted one", func(t *testing.T) {
		r := NewTestEntRepository(t)
		seedConsistent(t, r)
		in := consistentLifecycle("d-1", WebhookActionDelete, 1)
		in.CompatTupleSHA256 = "0000"

		_, err := r.ApplyWebhookLifecycle(ctx, in)

		assert.ErrorIs(t, err, ErrCompatTupleMismatch)
	})

	t.Run("persisted tuple no longer hashes to its stored digest", func(t *testing.T) {
		r := NewTestEntRepository(t)
		seedConsistent(t, r)
		require.NoError(t, r.client.WebhookRegistration.Update().SetCompatTuple(`{"corrupted":true}`).Exec(ctx))
		in := consistentLifecycle("d-1", WebhookActionDelete, 1)
		in.CompatTuple = `{"corrupted":true}` // even a request that matches the corrupted bytes must fail closed

		_, err := r.ApplyWebhookLifecycle(ctx, in)

		assert.ErrorIs(t, err, ErrCompatTupleMismatch)
		w, g, _ := countRows(t, r)
		assert.Equal(t, []int{1, 1}, []int{w, g})
	})
}

func TestApplyWebhookLifecycle_should_ReturnNotFound_When_ScopeDoesNotOwnTheRegistration(t *testing.T) {
	r := NewTestEntRepository(t)
	seedConsistent(t, r)
	in := consistentLifecycle("d-1", WebhookActionDelete, 1)
	in.Scope = scopeB

	_, err := r.ApplyWebhookLifecycle(context.Background(), in)

	assert.ErrorIs(t, err, ErrNotFound)
	w, g, _ := countRows(t, r)
	assert.Equal(t, []int{1, 1}, []int{w, g})
}

func TestApplyWebhookLifecycle_should_DeleteWorkflowAndRegistrationButKeepLedger_When_Deleted(t *testing.T) {
	r := NewTestEntRepository(t)
	seed := seedConsistent(t, r)
	in := consistentLifecycle("del-1", WebhookActionDelete, 1)

	res, err := r.ApplyWebhookLifecycle(context.Background(), in)
	require.NoError(t, err)

	assert.Equal(t, WebhookOutcomeDeleted, res.Outcome)
	assert.Equal(t, seed.Registration.WorkflowID, res.Registration.WorkflowID)
	assert.Equal(t, "gh-signal main", res.Registration.Workflow.Name, "the result describes the state immediately before deletion")
	w, g, l := countRows(t, r)
	assert.Equal(t, []int{0, 0, 2}, []int{w, g, l})
	_, err = r.InspectWebhookRegistration(context.Background(), scopeA, "inst-1")
	assert.ErrorIs(t, err, ErrNotFound)

	replay, err := r.ApplyWebhookLifecycle(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, replay.Replayed, "a replayed delete answers from the ledger even though the registration is gone")
	assert.Equal(t, WebhookOutcomeDeleted, replay.Outcome)

	_, err = r.ApplyWebhookLifecycle(context.Background(), consistentLifecycle("del-2", WebhookActionDelete, 1))
	assert.ErrorIs(t, err, ErrNotFound, "a fresh delete of a missing registration is not found, not success")
}

func TestApplyWebhookLifecycle_should_CleanUpOrphanedRegistration_When_WorkflowWasDeletedOutOfBand(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	seed := seedConsistent(t, r)
	require.NoError(t, r.client.Workflow.DeleteOneID(seed.Registration.WorkflowID).Exec(ctx))

	_, errDisable := r.ApplyWebhookLifecycle(ctx, consistentLifecycle("d-1", WebhookActionDisable, 1))
	res, errDelete := r.ApplyWebhookLifecycle(ctx, consistentLifecycle("del-1", WebhookActionDelete, 1))

	assert.ErrorIs(t, errDisable, ErrRegistrationOrphaned)
	require.NoError(t, errDelete)
	assert.Equal(t, WebhookOutcomeDeleted, res.Outcome)
	_, g, _ := countRows(t, r)
	assert.Zero(t, g)
}

func TestApplyWebhookLifecycle_should_AllowRecreate_When_InstanceWasDeleted(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	seedConsistent(t, r)
	_, err := r.ApplyWebhookLifecycle(ctx, consistentLifecycle("del-1", WebhookActionDelete, 1))
	require.NoError(t, err)

	again := baseReconcileInput(scopeA, "recreate-1")
	again.CompatTuple, again.CompatTupleSHA256 = consistentTuple()
	res, err := r.ReconcileWebhookRegistration(ctx, again)

	require.NoError(t, err)
	assert.Equal(t, WebhookOutcomeCreated, res.Outcome)
	assert.EqualValues(t, 1, res.Registration.Version)
}

func TestApplyWebhookLifecycle_should_ApplyExactlyOnce_When_DeleteRacesConcurrently(t *testing.T) {
	ctx := context.Background()

	t.Run("same request id", func(t *testing.T) {
		r := NewTestEntRepository(t)
		seedConsistent(t, r)
		in := consistentLifecycle("race-same", WebhookActionDelete, 1)
		const racers = 12
		results := make([]*WebhookReconcileResult, racers)
		errs := make([]error, racers)
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func(i int) { defer wg.Done(); results[i], errs[i] = r.ApplyWebhookLifecycle(ctx, in) }(i)
		}
		wg.Wait()

		applied := 0
		for i := 0; i < racers; i++ {
			require.NoError(t, errs[i])
			if !results[i].Replayed {
				applied++
			}
		}
		assert.Equal(t, 1, applied)
	})

	t.Run("different request ids", func(t *testing.T) {
		r := NewTestEntRepository(t)
		seedConsistent(t, r)
		const racers = 12
		errs := make([]error, racers)
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, errs[i] = r.ApplyWebhookLifecycle(ctx, consistentLifecycle(fmt.Sprintf("race-%d", i), WebhookActionDelete, 1))
			}(i)
		}
		wg.Wait()

		winners, notFound := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				winners++
			case errors.Is(err, ErrNotFound):
				notFound++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		assert.Equal(t, 1, winners)
		assert.Equal(t, racers-1, notFound)
	})
}
