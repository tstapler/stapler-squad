package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// recordingGuidanceStore is the second GuidanceStore implementation: it records
// every call's arguments and returns canned results, so the tests pin what the
// handlers pass the store and reach store errors real Ent cannot produce.
type recordingGuidanceStore struct {
	createCalls []domain.CreateGuidanceRequestInput
	createRes   *domain.GuidanceRequestData
	createErr   error

	mirroredReqs []domain.GuidanceRequest
	mirroredCaps []int
	mirroredRes  *domain.GuidanceRequestData
	mirroredErr  error

	getCalls []uuid.UUID
	getRes   *domain.GuidanceRequestData
	getErr   error

	cancelIDs     []uuid.UUID
	cancelReasons []string
	cancelErr     error

	linkCalls [][2]string
	linkErr   error

	itemCalls []string
	item      backlogdto.BacklogItemData
	itemErrs  []error // consumed one per GetBacklogItem call; empty means nil
}

func newRecordingGuidanceStore() *recordingGuidanceStore {
	return &recordingGuidanceStore{
		createRes:   &domain.GuidanceRequestData{ID: uuid.NewString(), Scope: domain.RequestScopeStandalone, QuestionType: domain.QuestionTypeYesNo},
		mirroredRes: &domain.GuidanceRequestData{ID: uuid.NewString(), Scope: domain.RequestScopeSession, QuestionType: domain.QuestionTypeMultipleChoice},
	}
}

func (f *recordingGuidanceStore) CreateGuidanceRequest(_ context.Context, in domain.CreateGuidanceRequestInput) (*domain.GuidanceRequestData, error) {
	f.createCalls = append(f.createCalls, in)
	return f.createRes, f.createErr
}

func (f *recordingGuidanceStore) CreateMirroredGuidanceRequest(_ context.Context, req domain.GuidanceRequest, capLimit int) (*domain.GuidanceRequestData, error) {
	f.mirroredReqs = append(f.mirroredReqs, req)
	f.mirroredCaps = append(f.mirroredCaps, capLimit)
	return f.mirroredRes, f.mirroredErr
}

func (f *recordingGuidanceStore) GetGuidanceRequest(_ context.Context, id uuid.UUID) (*domain.GuidanceRequestData, error) {
	f.getCalls = append(f.getCalls, id)
	return f.getRes, f.getErr
}

func (f *recordingGuidanceStore) CancelGuidanceRequest(_ context.Context, id uuid.UUID, reason string) (bool, error) {
	f.cancelIDs = append(f.cancelIDs, id)
	f.cancelReasons = append(f.cancelReasons, reason)
	return f.cancelErr == nil, f.cancelErr
}

func (f *recordingGuidanceStore) GetItemSessionBySessionAndItem(_ context.Context, sessionUUID, itemID string) (backlogdto.ItemSessionSummary, error) {
	f.linkCalls = append(f.linkCalls, [2]string{sessionUUID, itemID})
	return backlogdto.ItemSessionSummary{BacklogItemID: itemID}, f.linkErr
}

func (f *recordingGuidanceStore) GetBacklogItem(_ context.Context, id string) (*backlogdto.BacklogItemData, error) {
	f.itemCalls = append(f.itemCalls, id)
	var err error
	if len(f.itemErrs) > 0 {
		err, f.itemErrs = f.itemErrs[0], f.itemErrs[1:]
	}
	item := f.item
	item.ID = id
	return &item, err
}

func (f *recordingGuidanceStore) GetItemSessionBySessionUUID(context.Context, string) (backlogdto.ItemSessionSummary, error) {
	return backlogdto.ItemSessionSummary{}, domain.ErrNotFound
}

var _ GuidanceStore = (*recordingGuidanceStore)(nil)

func guidanceHandlersOn(f *recordingGuidanceStore) (*backlogHandlers, context.Context) {
	return &backlogHandlers{guidance: f}, WithSessionUUID(context.Background(), "caller-1")
}

// --- backlogHandlers: create_guidance_request / get_guidance_request ---

func TestCreateGuidanceRequest_should_PassScopeAndQuestionToTheStore_When_Standalone(t *testing.T) {
	t.Parallel()
	f := newRecordingGuidanceStore()
	h, ctx := guidanceHandlersOn(f)

	res, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]any{"scope": "standalone", "question_text": "Ship it?", "question_type": "yes-no"}))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "%v", m)
	assert.Equal(t, f.createRes.ID, m["id"])
	require.Len(t, f.createCalls, 1)
	assert.Equal(t, domain.RequestScopeStandalone, f.createCalls[0].Scope)
	assert.Equal(t, "Ship it?", f.createCalls[0].QuestionText)
	assert.Equal(t, domain.QuestionTypeYesNo, f.createCalls[0].QuestionType)
	assert.Empty(t, f.createCalls[0].SessionUUID, "standalone carries no session")
	assert.Empty(t, f.mirroredReqs)
}

func TestCreateGuidanceRequest_should_ScopeToTheCaller_When_SessionScope(t *testing.T) {
	t.Parallel()
	f := newRecordingGuidanceStore()
	h, ctx := guidanceHandlersOn(f)

	res, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]any{"scope": "session", "question_text": "Which?", "question_type": "short-answer"}))
	require.NoError(t, err)
	require.True(t, parseResult(t, res)["success"].(bool))
	require.Len(t, f.createCalls, 1)
	assert.Equal(t, "caller-1", f.createCalls[0].SessionUUID)
}

func TestCreateGuidanceRequest_should_MapStoreErrors_When_CreateFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
	}{
		{"cap exceeded", domain.ErrPendingCapExceeded, ErrPendingCapExceeded, domain.ErrPendingCapExceeded.Error()},
		{"other", errors.New("disk full"), ErrInternalError, "create guidance request: disk full"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordingGuidanceStore()
			f.createErr = tt.err
			h, ctx := guidanceHandlersOn(f)
			res, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]any{"scope": "standalone", "question_text": "Q?", "question_type": "yes-no"}))
			require.NoError(t, err)
			m := parseResult(t, res)
			assert.Equal(t, tt.wantCode, resultCode(m))
			assert.Contains(t, resultErrField(m, "message"), tt.wantMsg)
		})
	}
}

func TestCreateGuidanceRequest_should_UseTheMirroredWriteWithItsCap_When_QuestionsGiven(t *testing.T) {
	t.Parallel()
	f := newRecordingGuidanceStore()
	h, ctx := guidanceHandlersOn(f)
	args := map[string]any{
		"scope": "standalone",
		"questions": []any{map[string]any{
			"text":    "Region?",
			"options": []any{map[string]any{"label": "us"}, map[string]any{"label": "eu"}},
		}},
	}

	res, err := h.createGuidanceRequest(ctx, makeToolReq(args))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "%v", m)
	assert.Equal(t, f.mirroredRes.ID, m["id"])
	require.Len(t, f.mirroredReqs, 1)
	assert.Empty(t, f.createCalls, "structured questions go through the mirrored write only")
	assert.Equal(t, "Region?", f.mirroredReqs[0].Params().QuestionText)
	assert.Equal(t, 0, f.mirroredCaps[0], "no cap is set by the tool; 0 selects the default")
}

func TestCreateGuidanceRequest_should_MapStoreErrors_When_MirroredCreateFails(t *testing.T) {
	t.Parallel()
	f := newRecordingGuidanceStore()
	f.mirroredErr = domain.ErrPendingCapExceeded
	h, ctx := guidanceHandlersOn(f)
	res, err := h.createGuidanceRequest(ctx, makeToolReq(map[string]any{
		"scope":     "standalone",
		"questions": []any{map[string]any{"text": "Region?", "options": []any{map[string]any{"label": "us"}, map[string]any{"label": "eu"}}}},
	}))
	require.NoError(t, err)
	assert.Equal(t, ErrPendingCapExceeded, resultCode(parseResult(t, res)))
}

func TestGetGuidanceRequest_should_ReadTheParsedIDAndMapErrors(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"ok", nil, ""},
		{"not found", domain.ErrNotFound, ErrItemNotFound},
		{"other", errors.New("boom"), ErrInternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordingGuidanceStore()
			f.getRes = &domain.GuidanceRequestData{ID: id.String(), Scope: domain.RequestScopeStandalone, QuestionText: "Q?", QuestionType: domain.QuestionTypeYesNo}
			f.getErr = tt.err
			h, ctx := guidanceHandlersOn(f)
			res, err := h.getGuidanceRequest(ctx, makeToolReq(map[string]any{"question_id": id.String()}))
			require.NoError(t, err)
			m := parseResult(t, res)
			assert.Equal(t, []uuid.UUID{id}, f.getCalls)
			if tt.wantCode == "" {
				require.True(t, m["success"].(bool), "%v", m)
				assert.Equal(t, id.String(), m["id"])
				return
			}
			assert.Equal(t, tt.wantCode, resultCode(m))
		})
	}
}

// --- peerHelpHandlers: request_help ---

func newPeerHelpRig(t *testing.T) (*toolRig, *recordingGuidanceStore) {
	t.Helper()
	f := newRecordingGuidanceStore()
	r := newToolRigWithGuidance(t, func(*session.Storage) GuidanceStore { return f })
	r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), liveOwner()
	return r, f
}

func TestRequestHelp_should_WriteAShortAnswerSessionRow_When_PeerIsHuman(t *testing.T) {
	t.Parallel()
	r, f := newPeerHelpRig(t)

	m := r.call(as("A"), toolRequestHelp, askArgs("human"))

	require.True(t, m["success"].(bool), "%v", m)
	assert.Equal(t, f.createRes.ID, m["request_id"])
	require.Len(t, f.createCalls, 1)
	assert.Equal(t, domain.RequestScopeSession, f.createCalls[0].Scope)
	assert.Equal(t, "A", f.createCalls[0].SessionUUID)
	assert.Equal(t, domain.QuestionTypeShortAnswer, f.createCalls[0].QuestionType)
	assert.Equal(t, "How do I run the linter?", f.createCalls[0].QuestionText)
	assert.Empty(t, f.mirroredReqs)
}

func TestRequestHelp_should_WriteAMirroredPeerRowWithThePerAskerCap_When_PeerIsASession(t *testing.T) {
	t.Parallel()
	r, f := newPeerHelpRig(t)

	m := r.call(as("A"), toolRequestHelp, askArgs("B"))

	require.True(t, m["success"].(bool), "%v", m)
	assert.Equal(t, f.mirroredRes.ID, m["request_id"])
	require.Len(t, f.mirroredReqs, 1)
	assert.Empty(t, f.createCalls)
	req := f.mirroredReqs[0].Params()
	assert.Equal(t, "A", req.AskerSessionUUID)
	assert.Equal(t, "B", req.AnswererSessionUUID)
	assert.Equal(t, domain.QuestionSourcePeer, req.Source)
	assert.Equal(t, []int{r.backend.Limits().PeerOpenPerAsker}, f.mirroredCaps)
}

func TestRequestHelp_should_MapStoreErrors_When_CreateFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		peer     string
		setup    func(*recordingGuidanceStore)
		wantCode string
	}{
		{"human cap", "human", func(f *recordingGuidanceStore) { f.createErr = domain.ErrPendingCapExceeded }, CodeCapExceeded},
		{"human other", "human", func(f *recordingGuidanceStore) { f.createErr = errors.New("boom") }, ErrInternalError},
		{"peer cap", "B", func(f *recordingGuidanceStore) { f.mirroredErr = domain.ErrPendingCapExceeded }, CodeCapExceeded},
		{"peer other", "B", func(f *recordingGuidanceStore) { f.mirroredErr = errors.New("boom") }, ErrInternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, f := newPeerHelpRig(t)
			tt.setup(f)
			m := r.call(as("A"), toolRequestHelp, askArgs(tt.peer))
			assert.Equal(t, tt.wantCode, resultCode(m))
			assert.Empty(t, r.backend.scheduleCalls(), "no wake is armed when the row was not created")
			assert.Empty(t, f.cancelIDs)
		})
	}
}

func TestRequestHelp_should_WithdrawTheRow_When_TheAnswerWakeCannotBeArmed(t *testing.T) {
	t.Parallel()
	for _, cancelErr := range []error{nil, errors.New("cancel failed")} {
		r, f := newPeerHelpRig(t)
		f.cancelErr = cancelErr
		r.backend.scheduleErr = func(int, wakes.ScheduleRequest) error { return errors.New("no wake") }

		m := r.call(as("A"), toolRequestHelp, askArgs("human"))

		assert.False(t, m["success"].(bool), "%v", m)
		id, err := uuid.Parse(f.createRes.ID)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{id}, f.cancelIDs)
		assert.Equal(t, []string{helpAbortReason}, f.cancelReasons)
	}
}

// --- peerHelpHandlers: request_owner_action ---

func ownerActionRig(t *testing.T) (*toolRig, *recordingGuidanceStore, string) {
	t.Helper()
	r, f := newPeerHelpRig(t)
	f.item = backlogdto.BacklogItemData{Title: "item"}
	return r, f, uuid.NewString()
}

func TestRequestOwnerAction_should_CheckLinkThenItemThenCreateAnItemScopedRow(t *testing.T) {
	t.Parallel()
	r, f, itemID := ownerActionRig(t)

	m := r.call(as("A"), toolRequestOwnerAction, ownerActionArgs(itemID))

	require.True(t, m["success"].(bool), "%v", m)
	assert.Equal(t, [][2]string{{"A", itemID}}, f.linkCalls)
	assert.Equal(t, []string{itemID}, f.itemCalls)
	require.Len(t, f.createCalls, 1)
	in := f.createCalls[0]
	assert.Equal(t, domain.RequestScopeBacklogItem, in.Scope)
	require.NotNil(t, in.ItemID)
	assert.Equal(t, itemID, in.ItemID.String())
	assert.Equal(t, domain.QuestionTypeMultipleChoice, in.QuestionType)
	assert.JSONEq(t, `["record waiver","provide evidence"]`, in.Options)
	assert.Equal(t, f.createRes.ID, m["request_id"])
}

func TestRequestOwnerAction_should_MapLinkAndItemStoreErrors_And_CreateNothing(t *testing.T) {
	t.Parallel()
	archived := time.Now()
	tests := []struct {
		name      string
		setup     func(*recordingGuidanceStore)
		wantCode  string
		wantItems int
	}{
		{"link store failure", func(f *recordingGuidanceStore) { f.linkErr = errors.New("db down") }, ErrInternalError, 0},
		{"link not found then item missing", func(f *recordingGuidanceStore) {
			f.linkErr, f.itemErrs = domain.ErrNotFound, []error{domain.ErrNotFound}
		}, ErrItemNotFound, 1},
		{"link not found, item exists", func(f *recordingGuidanceStore) { f.linkErr = domain.ErrNotFound }, ErrPermissionDenied, 1},
		{"item load failure", func(f *recordingGuidanceStore) { f.itemErrs = []error{errors.New("boom")} }, ErrInternalError, 1},
		{"archived item", func(f *recordingGuidanceStore) { f.item.ArchivedAt = &archived }, ErrPermissionDenied, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, f, itemID := ownerActionRig(t)
			tt.setup(f)
			m := r.call(as("A"), toolRequestOwnerAction, ownerActionArgs(itemID))
			assert.Equal(t, tt.wantCode, resultCode(m), "%v", m)
			assert.Len(t, f.itemCalls, tt.wantItems)
			assert.Empty(t, f.createCalls)
			assert.Empty(t, r.backend.scheduleCalls())
		})
	}
}

func TestRequestOwnerAction_should_ReadTheItemAgainForAWaiver_And_FailOnItsError(t *testing.T) {
	t.Parallel()
	r, f, itemID := ownerActionRig(t)
	f.itemErrs = []error{nil, errors.New("boom")}
	args := ownerActionArgs(itemID)
	args["waive_criterion_index"] = float64(0)

	m := r.call(as("A"), toolRequestOwnerAction, args)

	assert.Equal(t, ErrInternalError, resultCode(m))
	assert.Equal(t, []string{itemID, itemID}, f.itemCalls, "link check item read, then the waiver read")
	assert.Empty(t, f.createCalls)
}

func TestRequestOwnerAction_should_MapCreateErrors_And_WithdrawTheRowWhenNoWakeCanBeArmed(t *testing.T) {
	t.Parallel()
	t.Run("cap exceeded", func(t *testing.T) {
		t.Parallel()
		r, f, itemID := ownerActionRig(t)
		f.createErr = domain.ErrPendingCapExceeded
		assert.Equal(t, ErrPendingCapExceeded, resultCode(r.call(as("A"), toolRequestOwnerAction, ownerActionArgs(itemID))))
		assert.Empty(t, f.cancelIDs)
	})
	t.Run("other create error", func(t *testing.T) {
		t.Parallel()
		r, f, itemID := ownerActionRig(t)
		f.createErr = errors.New("boom")
		assert.Equal(t, ErrInternalError, resultCode(r.call(as("A"), toolRequestOwnerAction, ownerActionArgs(itemID))))
	})
	t.Run("wake refused", func(t *testing.T) {
		t.Parallel()
		r, f, itemID := ownerActionRig(t)
		r.backend.scheduleErr = func(int, wakes.ScheduleRequest) error { return errors.New("no wake") }
		m := r.call(as("A"), toolRequestOwnerAction, ownerActionArgs(itemID))
		assert.False(t, m["success"].(bool), "%v", m)
		id, err := uuid.Parse(f.createRes.ID)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{id}, f.cancelIDs)
		assert.Equal(t, []string{helpAbortReason}, f.cancelReasons)
	})
}
