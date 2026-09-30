package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

const hookSlug = "gh-signal-events" // the slug mgmtHarness.reconcileReq registers

func (h *mgmtHarness) lifecycleReq(requestID string, expected int64) WebhookLifecycleRequest {
	return WebhookLifecycleRequest{RequestID: requestID, ExpectedVersion: expected, Compat: h.reconcileReq("unused", 0, "").Compat}
}

func (h *mgmtHarness) emergencyReq(requestID string, expected int64, action string) WebhookEmergencyRequest {
	l := h.lifecycleReq(requestID, expected)
	return WebhookEmergencyRequest{RequestID: l.RequestID, ExpectedVersion: l.ExpectedVersion, Compat: l.Compat, Action: action}
}

func (h *mgmtHarness) act(t *testing.T, token, action string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return h.do(t, http.MethodPost, mgmtBase+testInstance+"/"+action, token, body)
}

func (h *mgmtHarness) seeded(t *testing.T) *mgmtHarness {
	t.Helper()
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, h.reconcileReq("req-seed-0001", 0, testMgmtSecret)).Code)
	return h
}

func (h *mgmtHarness) stillRegistered(t *testing.T) bool {
	t.Helper()
	return h.do(t, http.MethodGet, mgmtBase+testInstance, h.token, nil).Code == http.StatusOK
}

func TestWebhookLifecycle_should_StopAcceptingDeliveriesAndBumpVersion_When_Disabled(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	require.Equal(t, http.StatusOK, h.deliver(t, hookSlug, testMgmtSecret).Code)

	rec := h.act(t, h.token, "disable", h.lifecycleReq("req-dis-0001", 1))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	requireValid(t, "reconcile-response.schema.json", rec.Body.Bytes())
	got := decodeBody[WebhookReconcileResponse](t, rec)
	assert.Equal(t, "disabled", got.Outcome)
	assert.EqualValues(t, 2, got.Registration.Version)
	assert.False(t, *got.Registration.Webhook.Enabled)
	assert.Equal(t, http.StatusNotFound, h.deliver(t, hookSlug, testMgmtSecret).Code, "a disabled webhook must not accept deliveries")
	assert.Equal(t, int32(1), h.infra.sessionSvc.callCount.Load())

	replay := h.act(t, h.token, "disable", h.lifecycleReq("req-dis-0001", 1))
	assert.True(t, decodeBody[WebhookReconcileResponse](t, replay).Replayed)

	again := h.act(t, h.token, "disable", h.lifecycleReq("req-dis-0002", 2))
	require.Equal(t, http.StatusOK, again.Code)
	assert.Equal(t, "unchanged", decodeBody[WebhookReconcileResponse](t, again).Outcome)
}

func TestWebhookLifecycle_should_RefuseWithoutMutation_When_VersionStaleOrInvalid(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)

	stale := h.act(t, h.token, "disable", h.lifecycleReq("req-dis-0001", 9))
	zero := h.act(t, h.token, "delete", h.lifecycleReq("req-del-0001", 0))

	require.Equal(t, http.StatusConflict, stale.Code)
	assert.Equal(t, "VERSION_CONFLICT", errCode(t, stale))
	requireValid(t, "error-response.schema.json", stale.Body.Bytes())
	assert.Equal(t, http.StatusBadRequest, zero.Code)
	assert.Equal(t, http.StatusOK, h.deliver(t, hookSlug, testMgmtSecret).Code, "nothing may have changed")
}

func TestWebhookLifecycle_should_EnforceImmutableProvenance_When_TupleDiffersFromCreation(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	other := h.lifecycleReq("req-dis-0001", 1)
	other.Compat.ClientVersion = "9.9.9"

	disable := h.act(t, h.token, "disable", other)
	other.RequestID = "req-del-0001"
	del := h.act(t, h.token, "delete", other)

	for _, rec := range []*httptest.ResponseRecorder{disable, del} {
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Equal(t, "COMPAT_TUPLE_MISMATCH", errCode(t, rec))
	}
	assert.Equal(t, http.StatusOK, h.deliver(t, hookSlug, testMgmtSecret).Code)
	assert.True(t, h.stillRegistered(t))
}

func TestWebhookLifecycle_should_RemoveEverythingAndAllowRecreate_When_Deleted(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)

	rec := h.act(t, h.token, "delete", h.lifecycleReq("req-del-0001", 1))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	requireValid(t, "reconcile-response.schema.json", rec.Body.Bytes())
	assert.Equal(t, "deleted", decodeBody[WebhookReconcileResponse](t, rec).Outcome)
	assert.False(t, h.stillRegistered(t))
	assert.Zero(t, workflowCount(t, h))
	assert.Equal(t, http.StatusNotFound, h.deliver(t, hookSlug, testMgmtSecret).Code)

	replay := h.act(t, h.token, "delete", h.lifecycleReq("req-del-0001", 1))
	require.Equal(t, http.StatusOK, replay.Code)
	assert.True(t, decodeBody[WebhookReconcileResponse](t, replay).Replayed)

	missing := h.act(t, h.token, "delete", h.lifecycleReq("req-del-0002", 1))
	assert.Equal(t, http.StatusNotFound, missing.Code, "a fresh delete of a missing registration is not found")

	recreate := h.reconcile(t, h.token, h.reconcileReq("req-seed-0002", 0, testMgmtSecret))
	assert.Equal(t, http.StatusCreated, recreate.Code, recreate.Body.String())
	assert.Equal(t, http.StatusOK, h.deliver(t, hookSlug, testMgmtSecret).Code)
}

func TestWebhookLifecycle_should_HideRegistrationFromOtherPrincipals_When_TheyDisableOrDelete(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	intruder, err := h.infra.entRepo.IssueIntegrationCredential(context.Background(),
		session.IntegrationCredentialInfo{PrincipalID: "intruder", WorkspaceID: testMgmtWorkspace, AllowedDirRoot: h.root})
	require.NoError(t, err)

	for _, action := range []string{"disable", "delete"} {
		rec := h.act(t, intruder, action, h.lifecycleReq("req-x-0000001-"+action, 1))
		assert.Equal(t, http.StatusNotFound, rec.Code, action)
		assert.Equal(t, "NOT_FOUND", errCode(t, rec))
	}
	rec := h.act(t, intruder, "emergency-cleanup", h.emergencyReq("req-x-emergency", 1, "delete"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, http.StatusOK, h.deliver(t, hookSlug, testMgmtSecret).Code)
	assert.True(t, h.stillRegistered(t))
}

func TestWebhookLifecycle_should_RequireAuthentication_When_AnyLifecycleRouteCalled(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	for _, action := range []string{"disable", "delete", "emergency-cleanup"} {
		for _, token := range []string{"", "bogus"} {
			rec := h.act(t, token, action, h.lifecycleReq("req-anon-0001", 1))
			assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s token=%q", action, token)
			requireValid(t, "error-response.schema.json", rec.Body.Bytes())
		}
	}
	assert.True(t, h.stillRegistered(t))
}

func TestWebhookLifecycle_should_ReachEmergencyCleanupButNotOrdinaryOps_When_CapabilityRevisionRevoked(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	h.svc.SetSupportedCapabilityRevisions(2) // revision 1, which the registration was created under, is revoked

	reconcile := h.reconcile(t, h.token, h.reconcileReq("req-rec-0001", 1, ""))
	disable := h.act(t, h.token, "disable", h.lifecycleReq("req-dis-0001", 1))
	del := h.act(t, h.token, "delete", h.lifecycleReq("req-del-0001", 1))
	for _, rec := range []*httptest.ResponseRecorder{reconcile, disable, del} {
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Equal(t, "UNSUPPORTED_CAPABILITY", errCode(t, rec))
	}
	assert.True(t, h.stillRegistered(t), "ordinary operations must not have run")

	emergency := h.act(t, h.token, "emergency-cleanup", h.emergencyReq("req-emg-0001", 1, "delete"))

	require.Equal(t, http.StatusOK, emergency.Code, emergency.Body.String())
	requireValid(t, "reconcile-response.schema.json", emergency.Body.Bytes())
	assert.Equal(t, "deleted", decodeBody[WebhookReconcileResponse](t, emergency).Outcome)
	assert.False(t, h.stillRegistered(t))
	assert.Zero(t, workflowCount(t, h))
}

func TestWebhookLifecycle_should_ReturnBoundedNotFoundAndNotMutate_When_EmergencyProvenanceOrScopeIsWrong(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	wrongTuple := h.emergencyReq("req-emg-0001", 1, "delete")
	wrongTuple.Compat.SignerKeyID = "someone-else"

	mismatch := h.act(t, h.token, "emergency-cleanup", wrongTuple)
	badAction := h.act(t, h.token, "emergency-cleanup", h.emergencyReq("req-emg-0002", 1, "reconcile"))
	stale := h.act(t, h.token, "emergency-cleanup", h.emergencyReq("req-emg-0003", 5, "delete"))

	assert.Equal(t, http.StatusNotFound, mismatch.Code)
	assert.Equal(t, "NOT_FOUND", errCode(t, mismatch), "a provenance failure must look exactly like a missing registration")
	assert.NotContains(t, mismatch.Body.String(), "version", "and must reveal nothing about it")
	assert.Equal(t, http.StatusBadRequest, badAction.Code)
	assert.Equal(t, http.StatusConflict, stale.Code)
	assert.Equal(t, "VERSION_CONFLICT", errCode(t, stale))
	assert.True(t, h.stillRegistered(t))
	assert.Equal(t, http.StatusOK, h.deliver(t, hookSlug, testMgmtSecret).Code)
}

func TestWebhookLifecycle_should_RejectReuse_When_RequestIDSpansActions(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	require.Equal(t, http.StatusOK, h.act(t, h.token, "emergency-cleanup", h.emergencyReq("req-shared-01", 1, "disable")).Code)

	rec := h.act(t, h.token, "emergency-cleanup", h.emergencyReq("req-shared-01", 2, "delete"))

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", errCode(t, rec))
	assert.True(t, h.stillRegistered(t), "the conflicting delete must not have run")
}

func TestWebhookLifecycle_should_StillWork_When_ReceiverIsDisabled(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	svc := NewWebhookRegistrationService(h.infra.entRepo, h.infra.cfg, testMgmtWorkspace, false)
	mux := http.NewServeMux()
	NewWebhookRegistrationHandler(svc).RegisterRoutes(mux)
	h.mux = mux

	rec := h.act(t, h.token, "delete", h.lifecycleReq("req-del-0001", 1))

	assert.Equal(t, http.StatusOK, rec.Code, "cleanup must not depend on the receiver being enabled")
	assert.Zero(t, workflowCount(t, h))
}

func TestWebhookLifecycle_should_PublishWorkflowEvents_When_DisabledAndDeleted(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)
	bus := events.NewEventBus(10)
	h.svc.SetEventBus(bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)
	next := func() *events.Event {
		select {
		case ev := <-ch:
			return ev
		case <-time.After(2 * time.Second):
			t.Fatal("expected a workflow event")
			return nil
		}
	}

	require.Equal(t, http.StatusOK, h.act(t, h.token, "disable", h.lifecycleReq("req-dis-0001", 1)).Code)
	disabled := next()
	assert.Equal(t, events.WorkflowChangeUpdated, disabled.WorkflowPayload.Kind)
	require.NotNil(t, disabled.WorkflowPayload.Workflow)
	assert.False(t, disabled.WorkflowPayload.Workflow.Enabled)

	rec := h.act(t, h.token, "delete", h.lifecycleReq("req-del-0001", 2))
	require.Equal(t, http.StatusOK, rec.Code)
	deleted := next()
	assert.Equal(t, events.WorkflowChangeDeleted, deleted.WorkflowPayload.Kind)
	assert.Equal(t, decodeBody[WebhookReconcileResponse](t, rec).Registration.WorkflowID, deleted.WorkflowPayload.WorkflowID)

	h.act(t, h.token, "delete", h.lifecycleReq("req-del-0001", 2)) // replay
	select {
	case ev := <-ch:
		t.Fatalf("a replay must not publish, got %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestWebhookLifecycle_should_RejectUnknownFields_When_BodyIsNotStrictlyTheContract(t *testing.T) {
	h := newMgmtHarness(t).seeded(t)

	rec := h.act(t, h.token, "disable", []byte(`{"request_id":"req-dis-0001","expected_version":1,"surprise":true}`))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.True(t, h.stillRegistered(t))
}
