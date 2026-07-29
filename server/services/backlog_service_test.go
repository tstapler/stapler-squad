package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// mockSessionCreator records CreateDirectorySession calls for inspection.
type mockSessionCreator struct {
	calls []mockCreateCall
	err   error
}

// mockSessionStopper implements SessionStopper for tests.
type mockSessionStopper struct {
	liveUUIDs map[string]bool
}

func (m *mockSessionStopper) IsSessionLive(uuid string) bool {
	return m.liveUUIDs[uuid]
}

func (m *mockSessionStopper) StopSessionByUUID(_ context.Context, _ string) error { return nil }

func (m *mockSessionStopper) KillTmuxSessionByTitle(_ context.Context, _ string) error {
	return nil
}

type mockCreateCall struct {
	title   string
	path    string
	prompt  string
	tags    []string
	oneShot bool
}

func (m *mockSessionCreator) CreateDirectorySession(_ context.Context, title, path, prompt string, tags []string, oneShot bool, _ bool) (*session.Instance, error) {
	m.calls = append(m.calls, mockCreateCall{title: title, path: path, prompt: prompt, tags: tags, oneShot: oneShot})
	if m.err != nil {
		return nil, m.err
	}
	return &session.Instance{Title: title}, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newBacklogService(t *testing.T) *BacklogService {
	t.Helper()
	return NewBacklogService(createTestStorage(t), nil, nil, nil)
}

func newBacklogServiceNilStorage() *BacklogService {
	return NewBacklogService(nil, nil, nil, nil)
}

// ─── CreateBacklogItem ────────────────────────────────────────────────────────

// UT-010: Happy path — title, description, AC, priority=3, status="idea"
func TestCreateBacklogItem_Success(t *testing.T) {
	svc := newBacklogService(t)

	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:       "Implement login flow",
		Description: "Add OAuth2 login",
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "User can log in", Status: "pending"},
		},
		Priority: 3,
	}))
	require.NoError(t, err)

	item := resp.Msg.Item
	assert.NotEmpty(t, item.Id)
	assert.Equal(t, "Implement login flow", item.Title)
	assert.Equal(t, "Add OAuth2 login", item.Description)
	assert.Equal(t, "idea", item.Status)
	assert.Equal(t, int32(3), item.Priority)
	require.Len(t, item.AcceptanceCriteria, 1)
	assert.Equal(t, "User can log in", item.AcceptanceCriteria[0].Text)
}

// UT-011: Empty title → CodeInvalidArgument
func TestCreateBacklogItem_EmptyTitle(t *testing.T) {
	svc := newBacklogService(t)

	_, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "",
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

// UT-012: Nil storage → CodeUnavailable
func TestCreateBacklogItem_NilStorage(t *testing.T) {
	svc := newBacklogServiceNilStorage()

	_, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "some item",
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeUnavailable, connErr.Code())
}

// ─── ListBacklogItems ─────────────────────────────────────────────────────────

// UT-013: Default filter hides done and archived items
func TestListBacklogItems_DefaultFilterHidesTerminalStatuses(t *testing.T) {
	svc := newBacklogService(t)

	// Create three items — all start as "idea"
	for _, title := range []string{"idea item", "done item", "archived item"} {
		_, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
			Title: title,
		}))
		require.NoError(t, err)
	}

	// List all to get IDs.
	listAll, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{
		IncludeTerminal: true,
	}))
	require.NoError(t, err)
	require.Len(t, listAll.Msg.Items, 3)

	idByTitle := map[string]string{}
	for _, it := range listAll.Msg.Items {
		idByTitle[it.Title] = it.Id
	}

	// Archive "archived item".
	archiveResp, err := svc.ArchiveBacklogItem(t.Context(), connect.NewRequest(&sessionv1.ArchiveBacklogItemRequest{
		ItemId: idByTitle["archived item"],
	}))
	require.NoError(t, err)
	// Pre-check: verify the archive transition actually happened before testing the list filter.
	require.Equal(t, "archived", archiveResp.Msg.Item.Status, "item should be in archived status before testing list filter")

	// Default list should exclude archived items.
	listDefault, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{}))
	require.NoError(t, err)

	returnedTitles := make([]string, 0, len(listDefault.Msg.Items))
	for _, it := range listDefault.Msg.Items {
		returnedTitles = append(returnedTitles, it.Title)
	}
	assert.NotContains(t, returnedTitles, "archived item")
	assert.Contains(t, returnedTitles, "idea item")
	assert.Contains(t, returnedTitles, "done item")
}

// AC9: duplicate-status items are excluded from default/active results, but
// still returned when explicitly filtered by status (matching archived/done behavior).
func TestListBacklogItems_ExcludesDuplicateByDefault(t *testing.T) {
	svc := newBacklogService(t)

	// Create three items — all start as "idea".
	for _, title := range []string{"idea item", "duplicate target item", "duplicate item"} {
		_, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
			Title: title,
		}))
		require.NoError(t, err)
	}

	// List all to get IDs.
	listAll, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{
		IncludeTerminal: true,
	}))
	require.NoError(t, err)
	require.Len(t, listAll.Msg.Items, 3)

	idByTitle := map[string]string{}
	for _, it := range listAll.Msg.Items {
		idByTitle[it.Title] = it.Id
	}

	// Transition "duplicate item" to duplicate status, pointing at "duplicate target item".
	transResp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:        idByTitle["duplicate item"],
		TargetStatus:  "duplicate",
		DuplicateOfId: idByTitle["duplicate target item"],
	}))
	require.NoError(t, err)
	// Pre-check: verify the transition actually happened before testing the list filter.
	require.Equal(t, "duplicate", transResp.Msg.Item.Status, "item should be in duplicate status before testing list filter")

	// Default list (ExcludeTerminal) should exclude the duplicate item.
	listDefault, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{}))
	require.NoError(t, err)

	returnedTitles := make([]string, 0, len(listDefault.Msg.Items))
	for _, it := range listDefault.Msg.Items {
		returnedTitles = append(returnedTitles, it.Title)
	}
	assert.NotContains(t, returnedTitles, "duplicate item")
	assert.Contains(t, returnedTitles, "idea item")
	assert.Contains(t, returnedTitles, "duplicate target item")

	// Explicit status filter should override the default exclusion and return the duplicate item.
	listExplicit, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{
		Status: []string{"duplicate"},
	}))
	require.NoError(t, err)

	explicitTitles := make([]string, 0, len(listExplicit.Msg.Items))
	for _, it := range listExplicit.Msg.Items {
		explicitTitles = append(explicitTitles, it.Title)
	}
	assert.Contains(t, explicitTitles, "duplicate item")
}

// ─── ApprovePlan ──────────────────────────────────────────────────────────────

// UT-032a: ApprovePlan when plan_artifacts_path is empty → CodeFailedPrecondition
func TestApprovePlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition(t *testing.T) {
	svc := newBacklogService(t)

	// Create item with no plan artifacts path.
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item without plan",
	}))
	require.NoError(t, err)

	_, err = svc.ApprovePlan(t.Context(), connect.NewRequest(&sessionv1.ApprovePlanRequest{
		ItemId: createResp.Msg.Item.Id,
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code())
}

// UT-032b: ApprovePlan happy path — sets plan_approved=true and plan_approved_at
func TestApprovePlan_HappyPath_SetsPlanApprovedAndTimestamp(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	// Create item.
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item with plan",
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Simulate TriggerTriage by directly setting plan_artifacts_path via storage.
	// os.Stat check in ApprovePlan requires the path to exist on disk.
	artifactsPath := t.TempDir()
	planApproved := false
	_, err = storage.UpdateBacklogItem(t.Context(), itemID, session.BacklogItemUpdate{
		PlanArtifactsPath: &artifactsPath,
		PlanApproved:      &planApproved,
	}, nil)
	require.NoError(t, err)

	// Now approve the plan.
	approveResp, err := svc.ApprovePlan(t.Context(), connect.NewRequest(&sessionv1.ApprovePlanRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)
	assert.True(t, approveResp.Msg.Item.PlanApproved)
	assert.NotNil(t, approveResp.Msg.Item.PlanApprovedAt)
}

// ─── TriggerReReview ──────────────────────────────────────────────────────

// UT-040a: TriggerReReview on item not in review status → CodeFailedPrecondition
func TestTriggerReReview_NotInReviewStatus_ReturnsFailedPrecondition(t *testing.T) {
	svc := newBacklogService(t)

	// Create item (starts as "idea").
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "test item",
	}))
	require.NoError(t, err)

	// Try to trigger re-review on item in "idea" status.
	_, err = svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{
		ItemId: createResp.Msg.Item.Id,
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code())
	assert.Contains(t, connErr.Error(), "review")
}

// UT-040b: TriggerReReview on item with no repo_path → CodeFailedPrecondition
func TestTriggerReReview_MissingRepoPath_ReturnsFailedPrecondition(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	// Create item with AC so it can transition to ready.
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "test item",
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipPlanning: true, // Skip planning gate for simpler transition
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Transition: idea → ready → in_progress → review.
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: string(session.BacklogStatusReady),
	}))
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: string(session.BacklogStatusInProgress),
	}))
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: string(session.BacklogStatusReview),
	}))
	require.NoError(t, err)

	// Try to trigger re-review without repo_path.
	_, err = svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{
		ItemId: itemID,
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code())
	assert.Contains(t, connErr.Error(), "repo_path")
}

// UT-040c: TriggerReReview happy path — item in review, no SessionCreator returns placeholder
func TestTriggerReReview_HappyPath_NoSessionCreator_ReturnsPlaceholder(t *testing.T) {
	svc := newBacklogService(t)

	// Create item with repo_path and AC.
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "test item",
		RepoPath: "/tmp/test-repo",
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Transition through states to reach review.
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: string(session.BacklogStatusReady),
	}))
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: string(session.BacklogStatusInProgress),
	}))
	require.NoError(t, err)

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: string(session.BacklogStatusReview),
	}))
	require.NoError(t, err)

	// Trigger re-review without a SessionCreator.
	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)
	assert.NotNil(t, resp.Msg.ItemSession)
	assert.Equal(t, itemID, resp.Msg.ItemSession.Id)
	assert.Equal(t, "re-review-triggered", resp.Msg.ItemSession.SessionRole)
}

// ─── T-11: Auto-triage, double-trigger guard, itemSessionToProto ─────────────

// TestCreateBacklogItem_SkipsTriageWhenSkipTriageTrue: skip_triage=true → triage_triggered=false,
// no CreateDirectorySession call.
func TestCreateBacklogItem_SkipsTriageWhenSkipTriageTrue(t *testing.T) {
	creator := &mockSessionCreator{}
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil)

	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:      "item with repo",
		RepoPath:   "/tmp/some-repo",
		SkipTriage: true,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.TriageTriggered, "triage_triggered should be false when skip_triage=true")
	assert.Empty(t, creator.calls, "CreateDirectorySession should not be called when skip_triage=true")
}

// TestCreateBacklogItem_SkipsTriageWhenRepoPathEmpty: no repo_path → triage_triggered=false.
func TestCreateBacklogItem_SkipsTriageWhenRepoPathEmpty(t *testing.T) {
	creator := &mockSessionCreator{}
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil)

	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "item without repo",
		RepoPath: "", // empty — triage auto-trigger skipped
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.TriageTriggered, "triage_triggered should be false when repo_path is empty")
	assert.Empty(t, creator.calls, "CreateDirectorySession should not be called when repo_path is empty")
}

// TestTriggerTriage_DoubleTriggerGuard: when a triage session already exists with no ended_at,
// TriggerTriage returns CodeAlreadyExists.
func TestTriggerTriage_DoubleTriggerGuard(t *testing.T) {
	storage := createTestStorage(t)
	const liveUUID = "00000000-0000-0000-0000-000000000001"
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{liveUUID: true}}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetSessionStopper(stopper)

	// Create an item with a repo path so TriggerTriage can reach the guard.
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:      "item for double-trigger test",
		RepoPath:   t.TempDir(),
		SkipTriage: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Manually insert a triage ItemSession with no ended_at (simulates a running triage).
	is, isErr := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: liveUUID,
		SessionRole: string(session.SessionRoleTriage),
	})
	require.NoError(t, isErr)
	// Mark it as started so the orphan guard treats it as genuinely live.
	require.NoError(t, storage.UpdateItemSessionStarted(t.Context(), is.ID.String(), time.Now()))

	// TriggerTriage should refuse because a triage session is already running.
	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: itemID,
	}))
	require.Error(t, trigErr)
	var connErr *connect.Error
	require.ErrorAs(t, trigErr, &connErr)
	assert.Equal(t, connect.CodeAlreadyExists, connErr.Code())
}

// TestItemSessionToProto_MapsTriageResult: valid triage_result JSON on ItemSession →
// populated TriageResult proto fields.
func TestItemSessionToProto_MapsTriageResult(t *testing.T) {
	storage := createTestStorage(t)

	// Create a backing item so we can create an ItemSession.
	itemData := session.BacklogItemData{
		Title:    "proto mapping test",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
	}
	item, err := storage.CreateBacklogItem(t.Context(), itemData)
	require.NoError(t, err)

	isData := session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "00000000-0000-0000-0000-000000000002",
		SessionRole: string(session.SessionRoleTriage),
	}
	is, isErr := storage.CreateItemSession(t.Context(), isData)
	require.NoError(t, isErr)

	// Store triage result JSON.
	triageJSON := `{"summary":"looks good","suggestions":[{"text":"Add error handling","rationale":"robustness"}],"clarifying_questions":["Is this P1?"]}`
	require.NoError(t, storage.UpdateItemSessionTriageResult(t.Context(), is.ID.String(), triageJSON))

	// Re-load and convert.
	sessions, loadErr := storage.ListItemSessions(t.Context(), item.ID)
	require.NoError(t, loadErr)
	require.Len(t, sessions, 1)

	proto := itemSessionToProto(sessions[0])
	require.NotNil(t, proto.TriageResult, "TriageResult should be populated")
	assert.Equal(t, "looks good", proto.TriageResult.Summary)
	require.Len(t, proto.TriageResult.Suggestions, 1)
	assert.Equal(t, "Add error handling", proto.TriageResult.Suggestions[0].Text)
	assert.Equal(t, "robustness", proto.TriageResult.Suggestions[0].Rationale)
	require.Len(t, proto.TriageResult.ClarifyingQuestions, 1)
	assert.Equal(t, "Is this P1?", proto.TriageResult.ClarifyingQuestions[0])
}

// TestItemSessionToProto_HandlesInvalidTriageResultJSON: malformed JSON → no panic,
// TriageResult is nil in the returned proto.
func TestItemSessionToProto_HandlesInvalidTriageResultJSON(t *testing.T) {
	storage := createTestStorage(t)

	itemData := session.BacklogItemData{
		Title:    "invalid json test",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
	}
	item, err := storage.CreateBacklogItem(t.Context(), itemData)
	require.NoError(t, err)

	isData := session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "00000000-0000-0000-0000-000000000003",
		SessionRole: string(session.SessionRoleTriage),
	}
	is, isErr := storage.CreateItemSession(t.Context(), isData)
	require.NoError(t, isErr)

	// Store malformed JSON.
	require.NoError(t, storage.UpdateItemSessionTriageResult(t.Context(), is.ID.String(), "{not valid json"))

	// Must not panic.
	require.NotPanics(t, func() {
		sessions, _ := storage.ListItemSessions(t.Context(), item.ID)
		if len(sessions) > 0 {
			proto := itemSessionToProto(sessions[0])
			// TriageResult should be nil because JSON was invalid.
			assert.Nil(t, proto.TriageResult)
		}
	})
}

// errSessionCreator always returns an error from CreateDirectorySession.
type errSessionCreator struct{ err error }

func (e *errSessionCreator) CreateDirectorySession(_ context.Context, _, _, _ string, _ []string, _ bool, _ bool) (*session.Instance, error) {
	return nil, e.err
}

// TestCreateBacklogItem_AutoTriggersTriageWhenRepoPathSet: when repo_path is set and
// sessionCreator is present, CreateBacklogItem calls TriggerTriage. Because TriggerTriage
// needs a real filesystem path for artifact creation, we verify the attempt is made
// by using an errSessionCreator that rejects the spawn — auto-triage still fails
// gracefully and TriageTriggered is false.
func TestCreateBacklogItem_AutoTriggersTriageWhenRepoPathSet(t *testing.T) {
	creator := &errSessionCreator{err: errors.New("no tmux in tests")}
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil)

	// The auto-trigger code path is gated on: !SkipTriage && RepoPath != "" && sessionCreator != nil.
	// TriggerTriage will reach MkdirAll on a real path and then try CreateDirectorySession.
	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "item triggers triage",
		RepoPath: t.TempDir(), // real path so TriggerTriage can create artifact dir
	}))
	require.NoError(t, err, "CreateBacklogItem must succeed even if auto-triage fails")
	// Session spawn failed, so triage_triggered is false, but the item was created.
	assert.False(t, resp.Msg.TriageTriggered, "triage_triggered should be false when session spawn errors")
	assert.NotEmpty(t, resp.Msg.Item.Id, "item should still be created")
}

// ─── TransitionBacklogItemStatus (duplicate status, Epic 2.2) ────────────────

// faultyRepo wraps a real session.Repository and injects a specific error for
// GetBacklogItem calls matching a target id. Used to simulate an infra failure
// (as opposed to a genuine not-found) on the second, duplicate_of_id lookup —
// the seam Task 2.2.5f's test needs to distinguish CodeInternal from
// CodeFailedPrecondition.
type faultyRepo struct {
	session.Repository
	failGetBacklogItemID string
	failErr              error
}

func (f *faultyRepo) GetBacklogItem(ctx context.Context, id string) (*session.BacklogItemData, error) {
	if id == f.failGetBacklogItemID {
		return nil, f.failErr
	}
	return f.Repository.GetBacklogItem(ctx, id)
}

// Task 2.2.5a: happy path — the atomic write sets status AND duplicate_of_id in one Save.
func TestTransitionBacklogItemStatus_ToDuplicate_SetsStatusAndDuplicateOfIdAtomically(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	canonicalResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "canonical item",
	}))
	require.NoError(t, err)
	canonicalID := canonicalResp.Msg.Item.Id

	dupResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "duplicate item",
	}))
	require.NoError(t, err)
	dupID := dupResp.Msg.Item.Id

	resp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:        dupID,
		TargetStatus:  string(session.BacklogStatusDuplicate),
		DuplicateOfId: canonicalID,
	}))
	require.NoError(t, err)
	assert.Equal(t, "duplicate", resp.Msg.Item.Status)
	assert.Equal(t, canonicalID, resp.Msg.Item.DuplicateOfId)

	// Re-fetch to prove the write actually landed (one Save, not two round trips).
	refetched, err := storage.GetBacklogItem(t.Context(), dupID)
	require.NoError(t, err)
	assert.Equal(t, "duplicate", refetched.Status)
	assert.Equal(t, canonicalID, refetched.DuplicateOfID)
}

// Task 2.2.5b: a stale expected-status precondition returns ErrPreconditionFailed,
// which surfaces at the RPC layer as CodeAborted.
func TestTransitionBacklogItemStatus_ConcurrentStatusChange_ReturnsPreconditionFailed(t *testing.T) {
	svc := newBacklogService(t)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "test item",
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Transition idea → ready for real, changing the item's actual status.
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: string(session.BacklogStatusReady),
	}))
	require.NoError(t, err)

	// Attempt a second transition (ready → in_progress is structurally and
	// guard-wise valid since SkipPlanning=true) but with a stale expected_status
	// ("idea") that no longer matches the item's actual current status ("ready").
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:         itemID,
		TargetStatus:   string(session.BacklogStatusInProgress),
		ExpectedStatus: string(session.BacklogStatusIdea),
	}))
	require.Error(t, err)
	assert.ErrorIs(t, err, session.ErrPreconditionFailed)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeAborted, connErr.Code())
}

// Task 2.2.5d: reopening a duplicate item (duplicate → idea) clears the stale
// duplicate_of_id atomically with the status write.
func TestTransitionBacklogItemStatus_Reopen_ClearsDuplicateOfId(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	canonicalResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "canonical item",
	}))
	require.NoError(t, err)
	canonicalID := canonicalResp.Msg.Item.Id

	dupResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "duplicate item",
	}))
	require.NoError(t, err)
	dupID := dupResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:        dupID,
		TargetStatus:  string(session.BacklogStatusDuplicate),
		DuplicateOfId: canonicalID,
	}))
	require.NoError(t, err)

	// Reopen: duplicate → idea.
	reopenResp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       dupID,
		TargetStatus: string(session.BacklogStatusIdea),
	}))
	require.NoError(t, err)
	assert.Equal(t, "idea", reopenResp.Msg.Item.Status)
	assert.Empty(t, reopenResp.Msg.Item.DuplicateOfId)

	refetched, err := storage.GetBacklogItem(t.Context(), dupID)
	require.NoError(t, err)
	assert.Equal(t, "idea", refetched.Status)
	assert.Empty(t, refetched.DuplicateOfID)
}

// Task 2.2.5e: a non-duplicate target status with a populated duplicate_of_id
// in the request must NOT persist it — closes the pre-mortem P2 finding that
// opts/SetDuplicateOfID were previously gated only on duplicate_of_id != "" with
// no check on the target status. Exercises both the RPC-layer construction
// guard (Task 2.2.4c, via the full RPC round trip) and the write-layer guard
// (Task 2.2.3a's toStatus == BacklogStatusDuplicate condition, via a direct
// storage-layer call that bypasses the RPC's own tightened construction so the
// write layer's own gate is what's actually under test).
func TestTransitionBacklogItemStatus_NonDuplicateTargetWithDuplicateOfId_DoesNotPersistIt(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	otherResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "other item",
	}))
	require.NoError(t, err)
	otherID := otherResp.Msg.Item.Id

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "test item",
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// RPC round trip: TargetStatus "ready" with a populated DuplicateOfId.
	resp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:        itemID,
		TargetStatus:  string(session.BacklogStatusReady),
		DuplicateOfId: otherID,
	}))
	require.NoError(t, err)
	assert.Equal(t, "ready", resp.Msg.Item.Status)
	assert.Empty(t, resp.Msg.Item.DuplicateOfId)

	refetched, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, "ready", refetched.Status)
	assert.Empty(t, refetched.DuplicateOfID)

	// Direct storage-layer call with an explicit opts, bypassing the RPC's own
	// construction guard, to prove the write-layer gate (Task 2.2.3a) is the
	// authoritative check regardless of what a caller passes in opts.
	updated, err := storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusInProgress, nil,
		&session.TransitionOptions{DuplicateOfID: otherID})
	require.NoError(t, err)
	assert.Equal(t, "in_progress", updated.Status)
	assert.Empty(t, updated.DuplicateOfID)
}

// Task 2.2.5f: an infra failure (non-not-found) on the duplicate_of_id lookup
// must surface as CodeInternal, not CodeFailedPrecondition — mirrors Task
// 3.1.3c's MCP-side assertion for the structurally identical RPC-handler lookup.
func TestTransitionBacklogItemStatus_DuplicateOfIdLookupInfraError_ReturnsInternal_NotFailedPrecondition(t *testing.T) {
	testDir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(testDir + "/sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })

	realStorage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	realSvc := NewBacklogService(realStorage, nil, nil, nil)

	createResp, err := realSvc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "test item",
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	targetResp, err := realSvc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "canonical item",
	}))
	require.NoError(t, err)
	targetID := targetResp.Msg.Item.Id

	faulty := &faultyRepo{
		Repository:           repo,
		failGetBacklogItemID: targetID,
		failErr:              errors.New("simulated db failure"),
	}
	faultyStorage, err := session.NewStorageWithRepository(faulty)
	require.NoError(t, err)
	faultySvc := NewBacklogService(faultyStorage, nil, nil, nil)

	_, err = faultySvc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:        itemID,
		TargetStatus:  string(session.BacklogStatusDuplicate),
		DuplicateOfId: targetID,
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInternal, connErr.Code())
	assert.NotEqual(t, connect.CodeFailedPrecondition, connErr.Code())
}

// Fix 1 (sdd:6-verify Go idioms SUGGEST #1): the duplicate_of_id target lookup
// must be gated on the target status (to == duplicate), not merely on
// DuplicateOfId being non-empty. Proves the lookup is skipped entirely for a
// non-duplicate target: DuplicateOfId points at an item whose lookup would
// fail with an infra error, but since the transition targets "ready" (not
// "duplicate"), the lookup must never fire and the transition must succeed.
func TestTransitionBacklogItemStatus_NonDuplicateTargetWithFailingDuplicateOfId_SkipsLookupAndSucceeds(t *testing.T) {
	testDir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(testDir + "/sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })

	realStorage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	realSvc := NewBacklogService(realStorage, nil, nil, nil)

	createResp, err := realSvc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "test item",
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	targetResp, err := realSvc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "other item",
	}))
	require.NoError(t, err)
	targetID := targetResp.Msg.Item.Id

	faulty := &faultyRepo{
		Repository:           repo,
		failGetBacklogItemID: targetID,
		failErr:              errors.New("simulated db failure"),
	}
	faultyStorage, err := session.NewStorageWithRepository(faulty)
	require.NoError(t, err)
	faultySvc := NewBacklogService(faultyStorage, nil, nil, nil)

	// TargetStatus "ready" (not "duplicate") with a DuplicateOfId that would
	// blow up if looked up. Must succeed because the lookup is skipped.
	resp, err := faultySvc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:        itemID,
		TargetStatus:  string(session.BacklogStatusReady),
		DuplicateOfId: targetID,
	}))
	require.NoError(t, err)
	assert.Equal(t, "ready", resp.Msg.Item.Status)
	assert.Empty(t, resp.Msg.Item.DuplicateOfId)
}

// Task 2.2.5g: done → duplicate is a structurally invalid edge (not in
// validTransitions), so it must be rejected with CodeInvalidArgument at the
// !s.engine.CanTransition(from, to) gate — not CodeFailedPrecondition, which is
// reserved for TransitionGuard's business-rule rejections on a structurally
// valid edge. Closes the gap where this rejection was previously proven only
// at the pure-function level (session/backlog_test.go), never through the
// actual RPC handler.
func TestTransitionBacklogItemStatus_DoneToDuplicate_RejectsWithInvalidArgument(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	itemData := session.BacklogItemData{
		Title:  "already done item",
		Status: string(session.BacklogStatusDone),
	}
	created, err := storage.CreateBacklogItem(t.Context(), itemData)
	require.NoError(t, err)

	canonicalResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "canonical item",
	}))
	require.NoError(t, err)
	canonicalID := canonicalResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:        created.ID,
		TargetStatus:  string(session.BacklogStatusDuplicate),
		DuplicateOfId: canonicalID,
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())

	// No row mutation occurred.
	refetched, err := storage.GetBacklogItem(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), refetched.Status)
	assert.Empty(t, refetched.DuplicateOfID)
}
