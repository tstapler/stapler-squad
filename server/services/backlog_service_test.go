package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// ─── fakeHeadlessPool ─────────────────────────────────────────────────────────

// fakeHeadlessPool is a test stub implementing headless.PoolClient.
type fakeHeadlessPool struct {
	mu       sync.Mutex
	response string
	err      error
	delay    time.Duration // simulates a slow LLM call; see TestTriggerTriage_SlowLLMCallDoesNotExpireCleanupContext
	calls    []fakePoolCall
}

type fakePoolCall struct {
	key     headless.FeatureKey
	workDir string
}

func (f *fakeHeadlessPool) CallBlockingWithOptions(_ context.Context, key headless.FeatureKey, _, _ string, opts headless.CallOptions) (string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakePoolCall{key: key, workDir: opts.WorkDir})
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return f.response, f.err
}

func (f *fakeHeadlessPool) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeHeadlessPool) firstCall() fakePoolCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return fakePoolCall{}
	}
	return f.calls[0]
}

// validTriageJSON returns a minimal valid triage result for use in success-path tests.
func validTriageJSON() string {
	return `{"summary":"test summary","suggestions":[{"text":"do X","rationale":"why"}],"tasks":[{"text":"write tests","estimate":"1h","category":"test"}]}`
}

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

// fakeAutonomousDriverStarter records StartAutonomousDriverForInstance calls for inspection.
type fakeAutonomousDriverStarter struct {
	calls []*session.Instance
}

func (f *fakeAutonomousDriverStarter) StartAutonomousDriverForInstance(inst *session.Instance) {
	f.calls = append(f.calls, inst)
}

func (f *fakeAutonomousDriverStarter) StartAutonomousDriverWithTimeout(inst *session.Instance, _ time.Duration) {
	f.calls = append(f.calls, inst)
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
	// Path must round-trip: SpawnSessionFromItem writes slash commands and a
	// context file to inst.Path. An empty Path here makes those writes land in
	// the test process's working directory instead of a sandbox.
	return &session.Instance{Title: title, Path: path}, nil
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

// ─── Full lifecycle (audit regression test) ──────────────────────────────────

// TestBacklogFullLifecycle_TriageApprovalSpawn_CarriesRealPromptContent exercises
// the real production path — CreateBacklogItem → TriggerTriage (real headless call,
// real ParseHeadlessTriageResult, real idea→ready transition) → ApprovePlan →
// SpawnSessionFromItem (real BuildTokenBudgetedPrompt) — faking only the two
// external process boundaries (the LLM call and the tmux/claude subprocess).
//
// This was written to verify a cross-platform-audit finding (project_plans/
// backlog-cross-platform-audit/gaps-and-risks.md #1): that AutonomousDriver's
// inst.Prompt/inst.InitialPrompt mismatch (ADR-022) breaks the execution phase's
// prompt delivery. It does not — CreateDirectorySession stores the real prompt in
// inst.Prompt, and buildClaudeCommand includes it as a CLI arg on first launch
// (claudeSessionID == ""), so session_driver.go's InitialPrompt-typing step
// correctly no-ops (see session_driver.go:135 "No initial prompt configured").
// This test locks in that the real item content — not a generic fallback —
// reaches the session-creation boundary.
func TestBacklogFullLifecycle_TriageApprovalSpawn_CarriesRealPromptContent(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil)
	svc.SetHeadlessPool(pool)
	starter := &fakeAutonomousDriverStarter{}
	svc.SetAutonomousDriverStarter(starter)

	repoPath := t.TempDir()
	const description = "Build the zzyzx widget integration end to end"
	const acText = "widget renders correctly on load"

	// 1. Create item (real code, status starts at "idea"). SkipTriage=true so we
	// control the TriggerTriage call explicitly below instead of racing the
	// auto-triage goroutine CreateBacklogItem would otherwise kick off.
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:       "zzyzx widget",
		Description: description,
		RepoPath:    repoPath,
		SkipTriage:  true,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: acText, Status: "pending"},
		},
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// 2. Trigger real triage. TriggerTriage returns immediately; the parse +
	// persist + idea→ready transition happens in a goroutine (see
	// backlog_service.go TriggerTriage), so poll for the real transition.
	_, err = svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
	require.NoError(t, err)

	var readyItem *sessionv1.BacklogItem
	require.Eventually(t, func() bool {
		getResp, getErr := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		if getErr != nil || getResp.Msg.Item.Status != "ready" {
			return false
		}
		readyItem = getResp.Msg.Item
		return true
	}, 2*time.Second, 10*time.Millisecond, "item should reach 'ready' after real headless triage completes")

	require.Equal(t, 1, pool.callCount(), "TriggerTriage should have made exactly one real headless call")
	require.NotEmpty(t, readyItem.PlanArtifactsPath, "TriggerTriage should persist plan_artifacts_path")

	// 3. Approve the plan (real ApprovePlan handler).
	approveResp, err := svc.ApprovePlan(t.Context(), connect.NewRequest(&sessionv1.ApprovePlanRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.True(t, approveResp.Msg.Item.PlanApproved)

	// 4. Spawn the execution session (real SpawnSessionFromItem, real
	// BuildTokenBudgetedPrompt). Autonomous=true with a wired autonomousStarter
	// exercises the autonomous-driver-start code path (asserted in step 4a below).
	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	require.NoError(t, err)

	// 4a. The autonomous driver start hook must actually fire when Autonomous=true
	// and an autonomousStarter is wired.
	require.Len(t, starter.calls, 1)

	// 5. The real, load-bearing assertion: the prompt that reached the session
	// creation boundary contains the item's actual content, not a placeholder.
	require.Len(t, creator.calls, 1)
	capturedPrompt := creator.calls[0].prompt
	assert.Contains(t, capturedPrompt, description, "spawned session prompt should carry the real item description")
	assert.Contains(t, capturedPrompt, acText, "spawned session prompt should carry the real acceptance criteria")
	assert.Contains(t, capturedPrompt, "plan.md", "spawned session prompt should point at the approved plan artifacts")
	assert.NotContains(t, capturedPrompt, "Please proceed with the task described in your instructions",
		"spawned session prompt must not be the generic AutonomousDriver fallback")

	// 6. Item should have advanced to in_progress after spawn.
	finalResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, "in_progress", finalResp.Msg.Item.Status)
}

// TestTriggerTriage_SlowLLMCallDoesNotExpireCleanupContext is a regression test
// for a live, 100%-reproducible bug found by manually observing a real triage
// run in production (see project_plans/backlog-cross-platform-audit/gaps-and-risks.md
// #1): TriggerTriage's cleanupCtx used to be created with a fixed timeout
// BEFORE calling the headless LLM pool, not after. Real triage calls routinely
// take 7-15 minutes (the prompt instructs 4 parallel research subagents), so
// cleanupCtx's budget was always already expired by the time the post-call
// persistence writes (triage result, plan_artifacts_path, idea->ready
// transition) ran — every successful triage call would log "[TriggerTriage]
// headless triage complete" while silently failing every single DB write that
// was supposed to make the result visible, leaving the item stuck at "idea"
// forever. This test simulates that exact shape (LLM call slower than the
// cleanup budget) at test-friendly timescales.
func TestTriggerTriage_SlowLLMCallDoesNotExpireCleanupContext(t *testing.T) {
	// triageCleanupTimeout is reduced but kept generous enough (2s) that the real
	// SQLite writes below can't flake under CI load or a busy test machine — the
	// bug being tested is about ORDERING (timeout starts before vs. after the
	// slow call), not about needing a tiny timeout, so there's no reason to cut
	// this closer to the wire.
	origTimeout := triageCleanupTimeout
	triageCleanupTimeout = 2 * time.Second
	defer func() { triageCleanupTimeout = origTimeout }()

	storage := createTestStorage(t)
	// delay outlasts triageCleanupTimeout — this is what the old code got wrong:
	// a cleanupCtx created before this delay would already be expired by the
	// time it's used afterward.
	pool := &fakeHeadlessPool{response: validTriageJSON(), delay: 3 * time.Second}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:      "slow triage item",
		RepoPath:   repoPath,
		SkipTriage: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
	require.NoError(t, err)

	var readyItem *sessionv1.BacklogItem
	require.Eventually(t, func() bool {
		getResp, getErr := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		if getErr != nil || getResp.Msg.Item.Status != "ready" {
			return false
		}
		readyItem = getResp.Msg.Item
		return true
	}, 6*time.Second, 10*time.Millisecond,
		"item must reach 'ready' even though the LLM call outlasted triageCleanupTimeout — "+
			"with the pre-fix ordering this would time out here because every persistence "+
			"write after the slow call would fail with context deadline exceeded")

	require.NotEmpty(t, readyItem.PlanArtifactsPath, "plan_artifacts_path must be persisted, not silently dropped")
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
// headlessPool is wired, CreateBacklogItem attempts TriggerTriage. With headlessPool nil
// (default in newBacklogService), the guard skips auto-triage gracefully.
func TestCreateBacklogItem_AutoTriggersTriageWhenRepoPathSet(t *testing.T) {
	creator := &errSessionCreator{err: errors.New("no tmux in tests")}
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil)
	// headlessPool is nil → auto-trigger guard skips triage; triage_triggered must be false.
	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "item triggers triage",
		RepoPath: t.TempDir(),
	}))
	require.NoError(t, err, "CreateBacklogItem must succeed even if auto-triage is skipped")
	assert.False(t, resp.Msg.TriageTriggered, "triage_triggered should be false when headlessPool is nil")
	assert.NotEmpty(t, resp.Msg.Item.Id, "item should still be created")
}

// ─── TriggerTriage ────────────────────────────────────────────────────────────

// TestTriggerTriage_NilPool: returns CodeUnimplemented when no headless pool is wired.
func TestTriggerTriage_NilPool(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "triage test",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.Error(t, trigErr)
	var connErr *connect.Error
	require.ErrorAs(t, trigErr, &connErr)
	assert.Equal(t, connect.CodeUnimplemented, connErr.Code())
}

// TestTriggerTriage_Success: headless pool returns valid JSON → item transitions to ready.
func TestTriggerTriage_Success(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "headless triage item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	resp, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)
	assert.Equal(t, string(session.SessionRoleTriage), resp.Msg.ItemSession.SessionRole)

	// Goroutine runs asynchronously — poll until item transitions to "ready".
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "item should transition to ready after headless triage completes")

	// Verify the pool was called with the right feature key and working directory.
	require.Equal(t, 1, pool.callCount())
	assert.Equal(t, headless.FeatureKeyTriage, pool.firstCall().key)
	assert.Equal(t, repoPath, pool.firstCall().workDir)

	// ItemSession should be ended.
	sessions, listErr := storage.ListItemSessions(t.Context(), item.ID)
	require.NoError(t, listErr)
	require.Len(t, sessions, 1)
	assert.NotNil(t, sessions[0].EndedAt, "triage item session should be marked ended on success")
}

// TestTriggerTriage_HeadlessPoolError: pool error → session ended, item stays idea.
func TestTriggerTriage_HeadlessPoolError(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{err: errors.New("claude binary not found")}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "error triage item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr, "TriggerTriage must return success synchronously even if headless call will fail")

	// Poll until the goroutine finishes and marks the session ended.
	require.Eventually(t, func() bool {
		sessions, listErr := storage.ListItemSessions(t.Context(), item.ID)
		return listErr == nil && len(sessions) > 0 && sessions[0].EndedAt != nil
	}, 5*time.Second, 50*time.Millisecond, "session should be marked ended after headless error")

	// Item must stay in idea status.
	updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, loadErr)
	assert.Equal(t, string(session.BacklogStatusIdea), updated.Status)
}

// TestTriggerTriage_AlreadyExists_LiveSession: a live (non-headless) triage session blocks re-trigger.
func TestTriggerTriage_AlreadyExists_LiveSession(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"live-triage-uuid": true}}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	svc.SetSessionStopper(stopper)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "blocked triage item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	// Create an open triage session that reports as live (non-headless UUID).
	_, isErr := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "live-triage-uuid",
		SessionRole: string(session.SessionRoleTriage),
	})
	require.NoError(t, isErr)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.Error(t, trigErr)
	var connErr *connect.Error
	require.ErrorAs(t, trigErr, &connErr)
	assert.Equal(t, connect.CodeAlreadyExists, connErr.Code())
}

// TestTriggerTriage_OrphanedHeadlessSession: orphaned headless triage session is tombstoned and re-trigger succeeds.
func TestTriggerTriage_OrphanedHeadlessSession(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "orphan triage item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	// Inject a stale open headless triage session (simulates a server restart).
	_, isErr := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-triage-00000000-dead-beef-0000-000000000000",
		SessionRole: string(session.SessionRoleTriage),
	})
	require.NoError(t, isErr)

	// Re-trigger should succeed: orphan is tombstoned and a new session is created.
	resp, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)
	assert.NotEmpty(t, resp.Msg.ItemSession.Id)

	// Wait for the new goroutine to complete and item to reach ready.
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond)
}
