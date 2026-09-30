package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

const (
	testMgmtWorkspace = "ws-test"
	testMgmtSecret    = "0123456789abcdef0123456789abcdef-secret"
	testMgmtSecret2   = "fedcba9876543210fedcba9876543210-rotated"
	testInstance      = "gh-signal-main"
	mgmtBase          = webhookManagementBasePath + "/registrations/"
)

type mgmtHarness struct {
	infra   *webhookTestInfra
	svc     *WebhookRegistrationService
	handler *WebhookRegistrationHandler
	mux     *http.ServeMux
	root    string
	token   string
	// remoteAddr, when set, is the peer address every request appears to come from (the
	// limiter keys on it); empty keeps httptest's default.
	remoteAddr string
}

func newMgmtHarness(t *testing.T) *mgmtHarness {
	t.Helper()
	infra := newWebhookTestInfra(t)
	svc := NewWebhookRegistrationService(infra.entRepo, infra.cfg, testMgmtWorkspace, true)
	mux := http.NewServeMux()
	handler := NewWebhookRegistrationHandler(svc)
	handler.RegisterRoutes(mux)
	NewGenericWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg).RegisterRoutes(mux)

	root := t.TempDir()
	token, err := infra.entRepo.IssueIntegrationCredential(context.Background(),
		session.IntegrationCredentialInfo{PrincipalID: "gh-signal", WorkspaceID: testMgmtWorkspace, AllowedDirRoot: root})
	require.NoError(t, err)
	return &mgmtHarness{infra: infra, svc: svc, handler: handler, mux: mux, root: root, token: token}
}

func (h *mgmtHarness) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		require.NoError(t, err)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	if h.remoteAddr != "" {
		req.RemoteAddr = h.remoteAddr
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

func (h *mgmtHarness) reconcileReq(requestID string, expected int64, secret string) WebhookReconcileRequest {
	enabled := true
	return WebhookReconcileRequest{
		RequestID:       requestID,
		ExpectedVersion: expected,
		Compat: WebhookCompat{
			ContractVersion: WebhookManagementContractVersion, CapabilityRevision: WebhookManagementCapabilityRevision,
			ManifestSHA256: strings.Repeat("a", 64), SignerKeyID: "release-key-1", ClientName: "gh-signal", ClientVersion: "0.1.0",
		},
		Webhook: WebhookSpecBody{
			Slug: "gh-signal-events", Name: "gh-signal events", Command: "handle the event",
			TargetDirectory: h.root, PromptTemplate: "event {{.type}}", Enabled: &enabled,
		},
		Secret: secret,
	}
}

func (h *mgmtHarness) reconcile(t *testing.T, token string, req WebhookReconcileRequest) *httptest.ResponseRecorder {
	t.Helper()
	return h.do(t, http.MethodPost, mgmtBase+testInstance+"/reconcile", token, req)
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), "body: %s", rec.Body.String())
	return v
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decodeBody[struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}](t, rec).Error.Code
}

func (h *mgmtHarness) deliver(t *testing.T, slug, secret string) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(`{"type":"pull_request.opened","event":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/"+slug, bytes.NewReader(body))
	req.Header.Set("X-Webhook-Signature", sign(secret, body))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

func workflowCount(t *testing.T, h *mgmtHarness) int {
	t.Helper()
	n, err := h.infra.entRepo.GetEntClient().Workflow.Query().Count(context.Background())
	require.NoError(t, err)
	return n
}

func TestWebhookManagement_should_Return401Uniformly_When_CredentialIsNotValid(t *testing.T) {
	h := newMgmtHarness(t)
	require.NoError(t, h.infra.entRepo.RevokeIntegrationCredential(context.Background(), "gh-signal"))
	otherWS, err := h.infra.entRepo.IssueIntegrationCredential(context.Background(),
		session.IntegrationCredentialInfo{PrincipalID: "elsewhere", WorkspaceID: "some-other-workspace", AllowedDirRoot: h.root})
	require.NoError(t, err)

	routes := []struct{ method, path string }{
		{http.MethodGet, webhookManagementBasePath + "/capability"},
		{http.MethodGet, mgmtBase + testInstance},
		{http.MethodPost, mgmtBase + testInstance + "/reconcile"},
	}
	peer := 0
	for _, token := range []string{"", "not-a-token", h.token /* revoked */, otherWS /* wrong workspace */} {
		for _, rt := range routes {
			peer++ // a distinct peer per request keeps this about the 401 shape, not the failure budget
			h.remoteAddr = fmt.Sprintf("192.0.2.%d:40000", peer)
			rec := h.do(t, rt.method, rt.path, token, h.reconcileReq("req-00000001", 0, testMgmtSecret))
			assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s token=%q", rt.method, rt.path, token)
			assert.Equal(t, "UNAUTHENTICATED", errCode(t, rec))
		}
	}
	assert.Zero(t, workflowCount(t, h), "an unauthenticated request must never mutate anything")
}

func TestWebhookManagement_should_ReportContractAndReceiverState_When_CapabilityRequested(t *testing.T) {
	h := newMgmtHarness(t)

	rec := h.do(t, http.MethodGet, webhookManagementBasePath+"/capability", h.token, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	got := decodeBody[WebhookCapabilityResponse](t, rec)
	assert.Equal(t, WebhookCapabilityResponse{
		ContractVersion: "v1", CapabilityRevision: 1, ReceiverEnabled: true,
		Operations: []string{"capability", "inspect", "reconcile", "disable", "delete", "emergency_cleanup"},
	}, got)
}

func TestWebhookManagement_should_ProvisionRegistrationThatAcceptsSignedDeliveries_When_ReconciledEndToEnd(t *testing.T) {
	h := newMgmtHarness(t)

	rec := h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), testMgmtSecret, "the secret is write-only and must never be returned")
	created := decodeBody[WebhookReconcileResponse](t, rec)
	assert.Equal(t, "created", created.Outcome)
	assert.EqualValues(t, 1, created.Registration.Version)
	assert.Equal(t, "/webhooks/gh-signal-events", created.Registration.EndpointPath)

	insp := h.do(t, http.MethodGet, mgmtBase+testInstance, h.token, nil)
	require.Equal(t, http.StatusOK, insp.Code)
	assert.NotContains(t, insp.Body.String(), testMgmtSecret)
	assert.Equal(t, created.Registration.RegistrationID, decodeBody[WebhookRegistrationBody](t, insp).RegistrationID)

	ok := h.deliver(t, "gh-signal-events", testMgmtSecret)
	assert.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	assert.Equal(t, int32(1), h.infra.sessionSvc.callCount.Load(), "a correctly signed delivery must fire the workflow")
	assert.Contains(t, h.infra.sessionSvc.LastRequest().InitialPrompt, "event pull_request.opened")

	bad := h.deliver(t, "gh-signal-events", "wrong-secret-wrong-secret-wrong-secret")
	assert.Equal(t, http.StatusUnauthorized, bad.Code)
	assert.Equal(t, int32(1), h.infra.sessionSvc.callCount.Load(), "a badly signed delivery must not fire")
}

func TestWebhookManagement_should_NeverStoreSecretInPlaintext_When_RegistrationCreated(t *testing.T) {
	h := newMgmtHarness(t)
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret)).Code)
	ctx := context.Background()
	c := h.infra.entRepo.GetEntClient()

	wfs, err := c.Workflow.Query().All(ctx)
	require.NoError(t, err)
	regs, err := c.WebhookRegistration.Query().All(ctx)
	require.NoError(t, err)
	ledger, err := c.WebhookRequestLedger.Query().All(ctx)
	require.NoError(t, err)

	for _, wf := range wfs {
		assert.NotContains(t, wf.WebhookSecretEncrypted, testMgmtSecret)
	}
	for _, r := range regs {
		assert.NotContains(t, r.SecretDigest, testMgmtSecret)
	}
	for _, l := range ledger {
		assert.NotContains(t, l.Result, testMgmtSecret)
		assert.NotContains(t, l.Fingerprint, testMgmtSecret)
	}
	assert.NotContains(t, h.reconcileReq("r", 0, testMgmtSecret).String(), testMgmtSecret)
}

func TestWebhookManagement_should_ReplayStoredResultWithoutSecondWorkflow_When_SameRequestRepeated(t *testing.T) {
	h := newMgmtHarness(t)
	req := h.reconcileReq("req-00000001", 0, testMgmtSecret)
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, req).Code)

	again := h.reconcile(t, h.token, req)

	require.Equal(t, http.StatusOK, again.Code)
	assert.True(t, decodeBody[WebhookReconcileResponse](t, again).Replayed)
	assert.NotContains(t, again.Body.String(), testMgmtSecret)
	assert.Equal(t, 1, workflowCount(t, h))
}

func TestWebhookManagement_should_Return409WithoutMutation_When_RequestIDReusedForDifferentRequest(t *testing.T) {
	h := newMgmtHarness(t)
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret)).Code)

	other := h.reconcileReq("req-00000001", 0, testMgmtSecret)
	other.Webhook.Name = "a different request"
	rec := h.reconcile(t, h.token, other)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", errCode(t, rec))
}

func TestWebhookManagement_should_Return409WithCurrentVersion_When_ExpectedVersionIsStale(t *testing.T) {
	h := newMgmtHarness(t)
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret)).Code)

	stale := h.reconcileReq("req-00000002", 0, testMgmtSecret)
	stale.Webhook.Name = "must not land"
	rec := h.reconcile(t, h.token, stale)

	require.Equal(t, http.StatusConflict, rec.Code)
	body := decodeBody[struct {
		Error struct {
			Code           string `json:"code"`
			CurrentVersion int64  `json:"current_version"`
		} `json:"error"`
	}](t, rec)
	assert.Equal(t, "VERSION_CONFLICT", body.Error.Code)
	assert.EqualValues(t, 1, body.Error.CurrentVersion)
	insp := decodeBody[WebhookRegistrationBody](t, h.do(t, http.MethodGet, mgmtBase+testInstance, h.token, nil))
	assert.Equal(t, "gh-signal events", insp.Webhook.Name)
}

func TestWebhookManagement_should_KeepAcceptingOldSecretUntilRotated_When_UpdatedWithOrWithoutSecret(t *testing.T) {
	h := newMgmtHarness(t)
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret)).Code)

	rename := h.reconcileReq("req-00000002", 1, "") // secret omitted: unchanged
	rename.Webhook.Name = "renamed"
	rec := h.reconcile(t, h.token, rename)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "updated", decodeBody[WebhookReconcileResponse](t, rec).Outcome)
	assert.Equal(t, http.StatusOK, h.deliver(t, "gh-signal-events", testMgmtSecret).Code, "omitted secret must leave the old one valid")

	rotate := h.reconcileReq("req-00000003", 2, testMgmtSecret2)
	rotate.Webhook.Name = "renamed"
	require.Equal(t, http.StatusOK, h.reconcile(t, h.token, rotate).Code)
	assert.Equal(t, http.StatusOK, h.deliver(t, "gh-signal-events", testMgmtSecret2).Code)
	assert.Equal(t, http.StatusUnauthorized, h.deliver(t, "gh-signal-events", testMgmtSecret).Code, "the rotated-out secret must stop working")
}

func TestWebhookManagement_should_ReportUnchanged_When_DesiredStateAlreadyHolds(t *testing.T) {
	h := newMgmtHarness(t)
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret)).Code)

	rec := h.reconcile(t, h.token, h.reconcileReq("req-00000002", 1, testMgmtSecret))

	require.Equal(t, http.StatusOK, rec.Code)
	got := decodeBody[WebhookReconcileResponse](t, rec)
	assert.Equal(t, "unchanged", got.Outcome)
	assert.EqualValues(t, 1, got.Registration.Version)
}

func TestWebhookManagement_should_HideAndProtectRegistration_When_AnotherPrincipalProbesIt(t *testing.T) {
	h := newMgmtHarness(t)
	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret)).Code)
	other, err := h.infra.entRepo.IssueIntegrationCredential(context.Background(),
		session.IntegrationCredentialInfo{PrincipalID: "intruder", WorkspaceID: testMgmtWorkspace, AllowedDirRoot: h.root})
	require.NoError(t, err)

	insp := h.do(t, http.MethodGet, mgmtBase+testInstance, other, nil)
	hijack := h.reconcileReq("req-0000000x", 0, testMgmtSecret2)
	hijack.Webhook.Name = "hijacked"
	rec := h.reconcile(t, other, hijack)

	assert.Equal(t, http.StatusNotFound, insp.Code)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "SLUG_UNAVAILABLE", errCode(t, rec))
	assert.Equal(t, http.StatusOK, h.deliver(t, "gh-signal-events", testMgmtSecret).Code, "the owner's secret must be untouched")
	owner := decodeBody[WebhookRegistrationBody](t, h.do(t, http.MethodGet, mgmtBase+testInstance, h.token, nil))
	assert.Equal(t, "gh-signal events", owner.Webhook.Name)
}

func TestWebhookManagement_should_PreserveUserManagedWebhook_When_RegistrationTargetsItsSlug(t *testing.T) {
	h := newMgmtHarness(t)
	user := newGenericWebhookWorkflow(t, h.infra, "gh-signal-events", "users-own-secret", "", "", "user template")

	rec := h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret))

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "SLUG_UNAVAILABLE", errCode(t, rec))
	after, err := h.infra.workflowRepo.GetByID(context.Background(), user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.WebhookSecretEncrypted, after.WebhookSecretEncrypted)
	assert.Equal(t, user.PromptTemplate, after.PromptTemplate)
	assert.Equal(t, http.StatusOK, h.deliver(t, "gh-signal-events", "users-own-secret").Code, "the pre-existing webhook must keep working")
	assert.Equal(t, 1, workflowCount(t, h))
}

func TestWebhookManagement_should_RefuseWithoutMutation_When_RequestIsOutOfBounds(t *testing.T) {
	h := newMgmtHarness(t)
	outside := h.reconcileReq("req-00000001", 0, testMgmtSecret)
	outside.Webhook.TargetDirectory = t.TempDir() // a real dir, but not under this credential's root
	traversal := h.reconcileReq("req-00000002", 0, testMgmtSecret)
	traversal.Webhook.TargetDirectory = h.root + "/../escape"
	weak := h.reconcileReq("req-00000003", 0, "too-short")
	noSecret := h.reconcileReq("req-00000004", 0, "")
	badContract := h.reconcileReq("req-00000005", 0, testMgmtSecret)
	badContract.Compat.ContractVersion = "v2"
	badCapability := h.reconcileReq("req-00000006", 0, testMgmtSecret)
	badCapability.Compat.CapabilityRevision = 99
	badSlug := h.reconcileReq("req-00000007", 0, testMgmtSecret)
	badSlug.Webhook.Slug = "Not A Slug"
	shortID := h.reconcileReq("short", 0, testMgmtSecret)

	cases := []struct {
		name   string
		req    WebhookReconcileRequest
		status int
		code   string
	}{
		{"directory outside root", outside, http.StatusForbidden, "FORBIDDEN_DIRECTORY"},
		{"non-clean traversal path", traversal, http.StatusBadRequest, "INVALID_REQUEST"},
		{"weak secret", weak, http.StatusBadRequest, "WEAK_SECRET"},
		{"no secret on create", noSecret, http.StatusBadRequest, "INVALID_REQUEST"},
		{"unsupported contract", badContract, http.StatusConflict, "UNSUPPORTED_CONTRACT"},
		{"unsupported capability", badCapability, http.StatusConflict, "UNSUPPORTED_CAPABILITY"},
		{"invalid slug", badSlug, http.StatusBadRequest, "INVALID_REQUEST"},
		{"short request id", shortID, http.StatusBadRequest, "INVALID_REQUEST"},
	}
	for _, tc := range cases {
		rec := h.reconcile(t, h.token, tc.req)
		assert.Equal(t, tc.status, rec.Code, tc.name)
		assert.Equal(t, tc.code, errCode(t, rec), tc.name)
	}
	assert.Zero(t, workflowCount(t, h), "no refused request may create anything")
}

func TestWebhookManagement_should_RejectMalformedBodies_When_StrictDecodingViolated(t *testing.T) {
	h := newMgmtHarness(t)
	path := mgmtBase + testInstance + "/reconcile"
	valid, err := json.Marshal(h.reconcileReq("req-00000001", 0, testMgmtSecret))
	require.NoError(t, err)

	unknownField := append(bytes.TrimSuffix(valid, []byte("}")), []byte(`,"surprise":1}`)...)
	trailing := append(append([]byte{}, valid...), []byte(`{"second":"object"}`)...)
	oversize := []byte(`{"request_id":"` + strings.Repeat("x", maxWebhookManagementBodyBytes) + `"}`)

	assert.Equal(t, http.StatusBadRequest, h.do(t, http.MethodPost, path, h.token, unknownField).Code)
	assert.Equal(t, http.StatusBadRequest, h.do(t, http.MethodPost, path, h.token, trailing).Code)
	assert.Equal(t, http.StatusBadRequest, h.do(t, http.MethodPost, path, h.token, []byte(`not json`)).Code)
	assert.Equal(t, http.StatusRequestEntityTooLarge, h.do(t, http.MethodPost, path, h.token, oversize).Code)

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(valid))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer "+h.token)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	assert.Zero(t, workflowCount(t, h))
}

func TestWebhookManagement_should_RefuseToCreate_When_ReceiverIsNotEnabled(t *testing.T) {
	h := newMgmtHarness(t)
	svc := NewWebhookRegistrationService(h.infra.entRepo, h.infra.cfg, testMgmtWorkspace, false)
	mux := http.NewServeMux()
	NewWebhookRegistrationHandler(svc).RegisterRoutes(mux)
	h.mux = mux

	rec := h.reconcile(t, h.token, h.reconcileReq("req-00000001", 0, testMgmtSecret))
	capRec := h.do(t, http.MethodGet, webhookManagementBasePath+"/capability", h.token, nil)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, "RECEIVER_DISABLED", errCode(t, rec))
	assert.False(t, decodeBody[WebhookCapabilityResponse](t, capRec).ReceiverEnabled)
	assert.Zero(t, workflowCount(t, h))
}

func TestWebhookManagement_should_PublishOneWorkflowEvent_When_CreatedAndNoneWhenReplayed(t *testing.T) {
	h := newMgmtHarness(t)
	bus := events.NewEventBus(10)
	h.svc.SetEventBus(bus)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := bus.Subscribe(ctx)
	req := h.reconcileReq("req-00000001", 0, testMgmtSecret)

	require.Equal(t, http.StatusCreated, h.reconcile(t, h.token, req).Code)
	select {
	case ev := <-ch:
		require.NotNil(t, ev.WorkflowPayload)
		assert.Equal(t, "created", string(ev.WorkflowPayload.Kind))
	case <-time.After(2 * time.Second):
		t.Fatal("no workflow event published for a created registration")
	}

	require.Equal(t, http.StatusOK, h.reconcile(t, h.token, req).Code) // replay
	select {
	case ev := <-ch:
		t.Fatalf("a replay must not publish an event, got %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}
}
