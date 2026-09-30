package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	scopeA = WebhookScope{PrincipalID: "principal-a", WorkspaceID: "ws-1"}
	scopeB = WebhookScope{PrincipalID: "principal-b", WorkspaceID: "ws-1"}
)

func baseReconcileInput(scope WebhookScope, requestID string) WebhookReconcileInput {
	return WebhookReconcileInput{
		Scope:             scope,
		RequestID:         requestID,
		Fingerprint:       "fp-" + requestID,
		InstanceID:        "inst-1",
		ExpectedVersion:   0,
		CompatTuple:       `{"contract_version":"v1"}`,
		CompatTupleSHA256: "tuple-hash-1",
		SecretEncrypted:   "enc-secret-1",
		SecretDigest:      "digest-1",
		Workflow: WebhookWorkflowSpec{
			WebhookSlug:     "gh-signal-main",
			Name:            "gh-signal main",
			Command:         "handle the event",
			TargetDirectory: "/tmp/repo",
			PromptTemplate:  "{{.action}}",
			EventFilter:     "pull_request",
			Enabled:         true,
		},
	}
}

func countRows(t *testing.T, r *EntRepository) (workflows, registrations, ledger int) {
	t.Helper()
	ctx := context.Background()
	w, err := r.client.Workflow.Query().Count(ctx)
	require.NoError(t, err)
	g, err := r.client.WebhookRegistration.Query().Count(ctx)
	require.NoError(t, err)
	l, err := r.client.WebhookRequestLedger.Query().Count(ctx)
	require.NoError(t, err)
	return w, g, l
}

func TestReconcileWebhookRegistration_should_CreateWebhookWorkflowAndRegistration_When_NoneExists(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()

	res, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	assert.Equal(t, WebhookOutcomeCreated, res.Outcome)
	assert.False(t, res.Replayed)
	assert.EqualValues(t, 1, res.Registration.Version)
	require.NotNil(t, res.ChangedWorkflow)
	assert.Equal(t, "webhook", res.ChangedWorkflow.TriggerType)
	assert.Equal(t, "gh-signal-main", res.ChangedWorkflow.WebhookSlug)
	assert.Equal(t, "enc-secret-1", res.ChangedWorkflow.WebhookSecretEncrypted)

	got, err := r.InspectWebhookRegistration(ctx, scopeA, "inst-1")
	require.NoError(t, err)
	assert.Equal(t, res.Registration.RegistrationID, got.RegistrationID)
	assert.Equal(t, "tuple-hash-1", got.CompatTupleSHA256)
}

func TestReconcileWebhookRegistration_should_ReturnStoredResultWithoutMutating_When_SameRequestIDAndFingerprintReplayed(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	in := baseReconcileInput(scopeA, "req-1")

	first, err := r.ReconcileWebhookRegistration(ctx, in)
	require.NoError(t, err)
	// The replay deliberately carries a stale expected_version: a replay must answer from
	// the ledger, not re-evaluate against current state.
	again, err := r.ReconcileWebhookRegistration(ctx, in)
	require.NoError(t, err)

	assert.True(t, again.Replayed)
	assert.Equal(t, first.Registration.RegistrationID, again.Registration.RegistrationID)
	assert.Equal(t, WebhookOutcomeCreated, again.Outcome)
	w, g, l := countRows(t, r)
	assert.Equal(t, []int{1, 1, 1}, []int{w, g, l})
}

func TestReconcileWebhookRegistration_should_RejectWithoutMutation_When_RequestIDReusedWithDifferentFingerprint(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	in := baseReconcileInput(scopeA, "req-1")
	_, err := r.ReconcileWebhookRegistration(ctx, in)
	require.NoError(t, err)

	other := in
	other.Fingerprint = "a-different-request"
	other.Workflow.Name = "renamed"
	_, err = r.ReconcileWebhookRegistration(ctx, other)

	assert.ErrorIs(t, err, ErrIdempotencyKeyReused)
	got, err := r.InspectWebhookRegistration(ctx, scopeA, "inst-1")
	require.NoError(t, err)
	assert.Equal(t, "gh-signal main", got.Workflow.Name)
}

func TestReconcileWebhookRegistration_should_ReturnConflictAndNotMutate_When_ExpectedVersionStale(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	stale := baseReconcileInput(scopeA, "req-2")
	stale.ExpectedVersion = 0 // a create attempt against an existing registration
	stale.Workflow.Name = "should not land"
	_, err = r.ReconcileWebhookRegistration(ctx, stale)

	var conflict *WebhookVersionConflictError
	require.True(t, errors.As(err, &conflict))
	assert.EqualValues(t, 1, conflict.CurrentVersion)
	got, err := r.InspectWebhookRegistration(ctx, scopeA, "inst-1")
	require.NoError(t, err)
	assert.Equal(t, "gh-signal main", got.Workflow.Name)
	_, _, l := countRows(t, r)
	assert.Equal(t, 1, l, "a rejected request must not consume its request ID")
}

func TestReconcileWebhookRegistration_should_UpdateAndBumpVersion_When_ExpectedVersionMatchesAndStateDiffers(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	upd := baseReconcileInput(scopeA, "req-2")
	upd.ExpectedVersion = 1
	upd.SecretEncrypted, upd.SecretDigest = "", "" // leave the secret alone
	upd.Workflow.Name = "renamed"
	upd.Workflow.Enabled = false
	res, err := r.ReconcileWebhookRegistration(ctx, upd)
	require.NoError(t, err)

	assert.Equal(t, WebhookOutcomeUpdated, res.Outcome)
	assert.EqualValues(t, 2, res.Registration.Version)
	assert.Equal(t, "renamed", res.ChangedWorkflow.Name)
	assert.False(t, res.ChangedWorkflow.Enabled)
	assert.Equal(t, "enc-secret-1", res.ChangedWorkflow.WebhookSecretEncrypted, "omitted secret must be preserved")
}

func TestReconcileWebhookRegistration_should_RotateSecret_When_DigestDiffers(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	upd := baseReconcileInput(scopeA, "req-2")
	upd.ExpectedVersion = 1
	upd.SecretEncrypted, upd.SecretDigest = "enc-secret-2", "digest-2"
	res, err := r.ReconcileWebhookRegistration(ctx, upd)
	require.NoError(t, err)

	assert.Equal(t, WebhookOutcomeUpdated, res.Outcome)
	assert.Equal(t, "enc-secret-2", res.ChangedWorkflow.WebhookSecretEncrypted)
}

func TestReconcileWebhookRegistration_should_ReportUnchangedWithoutVersionBump_When_DesiredStateAlreadyHolds(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	same := baseReconcileInput(scopeA, "req-2")
	same.ExpectedVersion = 1
	res, err := r.ReconcileWebhookRegistration(ctx, same)
	require.NoError(t, err)

	assert.Equal(t, WebhookOutcomeUnchanged, res.Outcome)
	assert.EqualValues(t, 1, res.Registration.Version)
	assert.Nil(t, res.ChangedWorkflow, "an unchanged reconcile must not trigger a workflow-changed event")
}

func TestInspectWebhookRegistration_should_ReturnNotFound_When_CallerScopeDoesNotOwnIt(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	_, errOtherPrincipal := r.InspectWebhookRegistration(ctx, scopeB, "inst-1")
	_, errOtherWorkspace := r.InspectWebhookRegistration(ctx, WebhookScope{PrincipalID: scopeA.PrincipalID, WorkspaceID: "ws-2"}, "inst-1")
	_, errMissing := r.InspectWebhookRegistration(ctx, scopeA, "no-such-instance")

	for _, e := range []error{errOtherPrincipal, errOtherWorkspace, errMissing} {
		assert.ErrorIs(t, e, ErrNotFound)
	}
}

func TestReconcileWebhookRegistration_should_RefuseAndLeaveOwnerUntouched_When_ForeignScopeTargetsSameSlug(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	hostile := baseReconcileInput(scopeB, "req-x")
	hostile.Workflow.Name = "hijacked"
	_, err = r.ReconcileWebhookRegistration(ctx, hostile)

	assert.ErrorIs(t, err, ErrWebhookSlugUnavailable)
	got, err := r.InspectWebhookRegistration(ctx, scopeA, "inst-1")
	require.NoError(t, err)
	assert.Equal(t, "gh-signal main", got.Workflow.Name)
	w, g, l := countRows(t, r)
	assert.Equal(t, []int{1, 1, 1}, []int{w, g, l}, "a failed create must leave no partial rows or ledger entry")
}

func TestReconcileWebhookRegistration_should_NeverAdoptOrModify_When_SlugBelongsToUserManagedWorkflow(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	user, err := r.client.Workflow.Create().
		SetSlug("gh-signal-main").SetName("user's own").SetCommand("user command").
		SetTargetDirectory("/home/user").Save(ctx)
	require.NoError(t, err)

	_, err = r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))

	assert.ErrorIs(t, err, ErrWebhookSlugUnavailable)
	after, err := r.client.Workflow.Get(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "user's own", after.Name)
	assert.Equal(t, "user command", after.Command)
	w, g, l := countRows(t, r)
	assert.Equal(t, []int{1, 0, 0}, []int{w, g, l})
}

func TestReconcileWebhookRegistration_should_RefuseWithoutMutation_When_CompatTupleDiffersFromPersisted(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	upd := baseReconcileInput(scopeA, "req-2")
	upd.ExpectedVersion = 1
	upd.CompatTupleSHA256 = "a-different-tuple"
	upd.Workflow.Name = "should not land"
	_, err = r.ReconcileWebhookRegistration(ctx, upd)

	assert.ErrorIs(t, err, ErrCompatTupleMismatch)
	got, err := r.InspectWebhookRegistration(ctx, scopeA, "inst-1")
	require.NoError(t, err)
	assert.Equal(t, "gh-signal main", got.Workflow.Name)
	assert.Equal(t, "tuple-hash-1", got.CompatTupleSHA256)
}

func TestReconcileWebhookRegistration_should_RefuseSlugChange_When_UpdatingExistingRegistration(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	_, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)

	upd := baseReconcileInput(scopeA, "req-2")
	upd.ExpectedVersion = 1
	upd.Workflow.WebhookSlug = "a-new-slug"
	_, err = r.ReconcileWebhookRegistration(ctx, upd)

	assert.ErrorIs(t, err, ErrImmutableWebhookField)
}

func TestReconcileWebhookRegistration_should_ReportOrphaned_When_WorkflowWasDeletedOutOfBand(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	res, err := r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "req-1"))
	require.NoError(t, err)
	require.NoError(t, r.client.Workflow.DeleteOneID(res.Registration.WorkflowID).Exec(ctx))

	upd := baseReconcileInput(scopeA, "req-2")
	upd.ExpectedVersion = 1
	_, errReconcile := r.ReconcileWebhookRegistration(ctx, upd)
	_, errInspect := r.InspectWebhookRegistration(ctx, scopeA, "inst-1")

	assert.ErrorIs(t, errReconcile, ErrRegistrationOrphaned)
	assert.ErrorIs(t, errInspect, ErrRegistrationOrphaned)
}

func TestReconcileWebhookRegistration_should_ApplyExactlyOnce_When_SameRequestRacesConcurrently(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	const racers = 12

	var wg sync.WaitGroup
	results := make([]*WebhookReconcileResult, racers)
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, "same-req"))
		}(i)
	}
	wg.Wait()

	created := 0
	for i := 0; i < racers; i++ {
		require.NoError(t, errs[i])
		if !results[i].Replayed {
			created++
		}
		assert.Equal(t, results[0].Registration.RegistrationID, results[i].Registration.RegistrationID)
	}
	assert.Equal(t, 1, created)
	w, g, l := countRows(t, r)
	assert.Equal(t, []int{1, 1, 1}, []int{w, g, l})
}

func TestReconcileWebhookRegistration_should_AllowOnlyOneWinner_When_DifferentRequestsRaceOnSameVersion(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	const racers = 12

	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = r.ReconcileWebhookRegistration(ctx, baseReconcileInput(scopeA, fmt.Sprintf("req-%d", i)))
		}(i)
	}
	wg.Wait()

	winners, conflicts := 0, 0
	for _, err := range errs {
		var vc *WebhookVersionConflictError
		switch {
		case err == nil:
			winners++
		case errors.As(err, &vc):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	assert.Equal(t, 1, winners)
	assert.Equal(t, racers-1, conflicts)
	w, g, _ := countRows(t, r)
	assert.Equal(t, []int{1, 1}, []int{w, g})
}

func TestIntegrationCredential_should_AuthenticateOnlyWhileActive_When_IssuedAndRevoked(t *testing.T) {
	r := NewTestEntRepository(t)
	ctx := context.Background()
	info := IntegrationCredentialInfo{PrincipalID: "gh-signal", WorkspaceID: "ws-1", AllowedDirRoot: "/tmp/repo"}

	token, err := r.IssueIntegrationCredential(ctx, info)
	require.NoError(t, err)
	assert.Contains(t, token, integrationTokenPrefix)

	got, err := r.AuthenticateIntegrationToken(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, info, *got)

	stored, err := r.client.IntegrationCredential.Query().Only(ctx)
	require.NoError(t, err)
	assert.NotContains(t, stored.TokenSha256, token, "the plaintext token must never be stored")

	_, err = r.AuthenticateIntegrationToken(ctx, token+"x")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = r.AuthenticateIntegrationToken(ctx, "")
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = r.IssueIntegrationCredential(ctx, info)
	assert.ErrorIs(t, err, ErrConflict, "re-issuing must not silently rotate an existing principal")

	require.NoError(t, r.RevokeIntegrationCredential(ctx, "gh-signal"))
	_, err = r.AuthenticateIntegrationToken(ctx, token)
	assert.ErrorIs(t, err, ErrNotFound)
}
