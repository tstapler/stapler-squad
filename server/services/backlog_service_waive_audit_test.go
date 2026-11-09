package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/session"
)

type auditFixture struct {
	svc     *BacklogService
	storage *session.Storage
	itemID  string
}

func newAuditFixture(t *testing.T, status session.BacklogStatus) auditFixture {
	t.Helper()
	storage := createTestStorage(t)
	ac, err := session.SerializeAcCriteria([]session.AcCriterion{
		{Index: 0, Text: "pending one", Status: session.AcStatusPending},
		{Index: 1, Text: "done one", Status: session.AcStatusDone},
	})
	require.NoError(t, err)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title: "audit item", Status: string(status), AcceptanceCriteria: ac,
	})
	require.NoError(t, err)
	return auditFixture{NewBacklogService(storage, &mockSessionCreator{}, nil, nil, nil, nil), storage, item.ID}
}

func (f auditFixture) waive(t *testing.T, ctx context.Context, idx int32, reason string, hdr map[string]string) error {
	t.Helper()
	req := connect.NewRequest(&sessionv1.WaiveAcceptanceCriterionRequest{ItemId: f.itemID, CriterionIndex: idx, Reason: reason})
	for k, v := range hdr {
		req.Header().Set(k, v)
	}
	_, err := f.svc.WaiveAcceptanceCriterion(ctx, req)
	return err
}

func (f auditFixture) audit(t *testing.T) []session.AcAuditEventData {
	t.Helper()
	rows, err := f.storage.ListAcAudit(t.Context(), f.itemID)
	require.NoError(t, err)
	return rows
}

func TestWaiveAcceptanceCriterion_should_DenyAndAudit_When_RequestCarriesSessionIdentity(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t, session.BacklogStatusInProgress)

	err := f.waive(t, t.Context(), 0, "self waive", map[string]string{"X-Stapler-Session-UUID": "11111111-1111-4111-8111-111111111111"})

	require.Error(t, err)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	rows := f.audit(t)
	require.Len(t, rows, 1)
	assert.Equal(t, "denied:agent_session", rows[0].Outcome)
	assert.Equal(t, string(session.AcActorAgentSession), rows[0].ActorClass)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", rows[0].ActorSessionUUID)
	assert.False(t, rows[0].SessionVerified, "an unproven claim must not be recorded as verified")
	item, err := f.storage.GetBacklogItem(t.Context(), f.itemID)
	require.NoError(t, err)
	got, _ := session.ParseAcCriteria(item.AcceptanceCriteria)
	assert.Equal(t, session.AcStatusPending, got[0].Status)
}

func TestWaiveAcceptanceCriterion_should_RecordUnverifiedCaller_When_NoIdentitySignal(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t, session.BacklogStatusInProgress)

	require.NoError(t, f.waive(t, t.Context(), 0, "ok", map[string]string{"User-Agent": "curl/8", "Sec-Fetch-Site": "same-origin"}))

	rows := f.audit(t)
	require.Len(t, rows, 1)
	assert.Equal(t, "applied", rows[0].Outcome)
	assert.Equal(t, string(session.AcActorUnverified), rows[0].ActorClass)
	assert.Equal(t, "ui", rows[0].Source)
	assert.Equal(t, "curl/8", rows[0].UserAgent)
	item, err := f.storage.GetBacklogItem(t.Context(), f.itemID)
	require.NoError(t, err)
	got, _ := session.ParseAcCriteria(item.AcceptanceCriteria)
	assert.Equal(t, "unverified caller", got[0].WaivedBy)
}

func TestWaiveAcceptanceCriterion_should_RefuseAndAudit_When_ItemTerminalOrCriterionDoneOrReasonBlank(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		status  session.BacklogStatus
		idx     int32
		reason  string
		code    connect.Code
		outcome string
	}{
		{"done item", session.BacklogStatusDone, 0, "r", connect.CodeFailedPrecondition, "denied:item_terminal"},
		{"archived item", session.BacklogStatusArchived, 0, "r", connect.CodeFailedPrecondition, "denied:item_terminal"},
		{"blank reason", session.BacklogStatusInProgress, 0, " ", connect.CodeInvalidArgument, "rejected:reason_required"},
		{"done criterion", session.BacklogStatusInProgress, 1, "r", connect.CodeFailedPrecondition, "rejected:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newAuditFixture(t, tc.status)

			err := f.waive(t, t.Context(), tc.idx, tc.reason, nil)

			require.Error(t, err)
			assert.Equal(t, tc.code, connect.CodeOf(err))
			rows := f.audit(t)
			require.Len(t, rows, 1, "exactly one audit row per refused attempt")
			assert.Contains(t, rows[0].Outcome, tc.outcome)
		})
	}
}

func TestAcActorFromRequest_should_ClassifyOperator_When_PasskeyMiddlewareAuthenticated(t *testing.T) {
	t.Parallel()
	var got session.AcActor
	h := middleware.Auth(stubValidator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = acActorFromRequest(r.Context(), r.Header, r.RemoteAddr)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
	req.Header.Set("Authorization", "Bearer good")

	h.ServeHTTP(httptest.NewRecorder(), req)

	assert.Equal(t, session.AcActorAuthenticatedBearer, got.Class)
}

type stubValidator struct{}

func (stubValidator) ValidateAuthSession(token string) bool { return token == "good" }

func TestUpdateBacklogItem_should_NotIntroduceOrClearWaiver_When_ClientResubmitsCriteria(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t, session.BacklogStatusInProgress)
	require.NoError(t, f.waive(t, t.Context(), 0, "owner call", nil))

	// Resubmitting the waived criterion as pending must not clear the waiver.
	_, err := f.svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: f.itemID,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "pending one", Status: "pending"},
			{Index: 1, Text: "done one", Status: "waived", WaiverReason: "sneaky"},
		},
	}))

	require.Error(t, err, "introducing a waiver through UpdateBacklogItem must be refused")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = f.svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: f.itemID,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "pending one", Status: "pending"},
			{Index: 1, Text: "done one", Status: "done"},
		},
	}))
	require.NoError(t, err)
	item, err := f.storage.GetBacklogItem(t.Context(), f.itemID)
	require.NoError(t, err)
	got, _ := session.ParseAcCriteria(item.AcceptanceCriteria)
	assert.Equal(t, session.AcStatusWaived, got[0].Status)
	assert.Equal(t, "owner call", got[0].WaiverReason)
}

func TestListAcAuditEvents_should_ReturnTrailOldestFirst(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t, session.BacklogStatusInProgress)
	require.Error(t, f.waive(t, t.Context(), 0, "x", map[string]string{"X-Stapler-Session-UUID": "11111111-1111-4111-8111-111111111111"}))
	require.NoError(t, f.waive(t, t.Context(), 0, "y", nil))

	resp, err := f.svc.ListAcAuditEvents(t.Context(), connect.NewRequest(&sessionv1.ListAcAuditEventsRequest{ItemId: f.itemID}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.Events, 2)
	assert.Equal(t, "denied:agent_session", resp.Msg.Events[0].Outcome)
	assert.Equal(t, "applied", resp.Msg.Events[1].Outcome)
	assert.NotNil(t, resp.Msg.Events[1].CreatedAt)
}

func TestAcActorFromRequest_should_ClassifyCookieAndBearerSeparately(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		set  func(*http.Request)
		want session.AcActorClass
	}{
		"cookie": {func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "cs_auth", Value: "good"}) }, session.AcActorAuthenticatedSession},
		"bearer": {func(r *http.Request) { r.Header.Set("Authorization", "Bearer good") }, session.AcActorAuthenticatedBearer},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var got session.AcActor
			h := middleware.Auth(stubValidator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = acActorFromRequest(r.Context(), r.Header, r.RemoteAddr)
			}))
			req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
			tc.set(req)
			h.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tc.want, got.Class)
		})
	}
}

func TestAcActorFromRequest_should_MarkInvalidSessionIDAndCapRemoteAddr(t *testing.T) {
	t.Parallel()
	h := http.Header{}
	h.Set("X-Stapler-Session-UUID", "<script>not-a-uuid</script>")

	a := acActorFromRequest(t.Context(), h, strings.Repeat("x", 500))

	assert.Equal(t, session.AcActorAgentSession, a.Class)
	assert.Equal(t, "(invalid id)", a.SessionUUID)
	assert.False(t, a.SessionVerified)
	assert.Len(t, a.RemoteAddr, 100)
	assert.Contains(t, a.Label(), "unverified claim")
}

func TestWaiveAcceptanceCriterion_should_RefuseRewaiveAndOversizedReason(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t, session.BacklogStatusInProgress)
	require.NoError(t, f.waive(t, t.Context(), 0, "first", nil))

	err := f.waive(t, t.Context(), 0, "second", nil)
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	f2 := newAuditFixture(t, session.BacklogStatusInProgress)
	err = f2.waive(t, t.Context(), 0, strings.Repeat("r", maxWaiverReason+1), nil)
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestApplyTriageResultToUpdate_should_NeverStoreModelWaiver_And_RepoCarriesExistingWaiverByText(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t, session.BacklogStatusInProgress)
	require.NoError(t, f.waive(t, t.Context(), 0, "owner call", nil)) // "pending one"

	// Re-triage: reorders, inserts, and the model emits a bogus waived status plus waiver metadata.
	result := &session.HeadlessTriageResult{AcceptanceCriteria: []session.AcCriterion{
		{Text: "brand new", Status: session.AcStatusWaived, WaiverReason: "model says so", WaivedBy: "model"},
		{Text: "done one", Status: session.AcStatusDone},
		{Text: "Pending One", Status: session.AcStatusPending},
	}}
	var update session.BacklogItemUpdate
	applyTriageResultToUpdate(result, &update)
	_, err := f.storage.UpdateBacklogItem(session.WithAcActor(t.Context(), session.AcActor{Class: session.AcActorSystem, Source: "api"}), f.itemID, update, nil)
	require.NoError(t, err)

	item, err := f.storage.GetBacklogItem(t.Context(), f.itemID)
	require.NoError(t, err)
	got, err := session.ParseAcCriteria(item.AcceptanceCriteria)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, session.AcStatusPending, got[0].Status, "model-emitted waiver is normalised away")
	assert.Empty(t, got[0].WaivedBy)
	assert.Equal(t, session.AcStatusWaived, got[2].Status, "existing waiver follows its text to the new index")
	assert.Equal(t, "owner call", got[2].WaiverReason)
	assert.NotEmpty(t, got[2].WaivedBy)
}

func TestUpdateBacklogItem_should_ReturnClearedWaivers_When_CriterionTextChanges(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t, session.BacklogStatusInProgress)
	require.NoError(t, f.waive(t, t.Context(), 0, "owner call", nil)) // "pending one"

	resp, err := f.svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: f.itemID,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "pending one, reworded", Status: "pending"},
			{Index: 1, Text: "done one", Status: "done"},
		},
	}))

	require.NoError(t, err)
	require.Len(t, resp.Msg.ClearedWaivers, 1)
	assert.Equal(t, "pending one", resp.Msg.ClearedWaivers[0].Text)
	assert.Equal(t, "owner call", resp.Msg.ClearedWaivers[0].WaiverReason)
}
