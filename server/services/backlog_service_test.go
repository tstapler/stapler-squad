package services

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	mu        sync.Mutex
	response  string
	responses []string // if set, returned in order (one per call) instead of response
	err       error
	delay     time.Duration // simulates a slow LLM call; see TestTriggerTriage_SlowLLMCallDoesNotExpireCleanupContext
	calls     []fakePoolCall
}

type fakePoolCall struct {
	key        headless.FeatureKey
	workDir    string
	userPrompt string
}

func (f *fakeHeadlessPool) CallBlockingWithOptions(ctx context.Context, key headless.FeatureKey, _, userPrompt string, opts headless.CallOptions) (string, error) {
	f.mu.Lock()
	callIndex := len(f.calls)
	f.calls = append(f.calls, fakePoolCall{key: key, workDir: opts.WorkDir, userPrompt: userPrompt})
	delay := f.delay
	resp := f.response
	if callIndex < len(f.responses) {
		resp = f.responses[callIndex]
	}
	f.mu.Unlock()
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return resp, f.err
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

func (f *fakeHeadlessPool) callAt(i int) fakePoolCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.calls) {
		return fakePoolCall{}
	}
	return f.calls[i]
}

// validTriageJSON returns a minimal valid triage result for use in success-path tests.
func validTriageJSON() string {
	return `{"summary":"test summary","suggestions":[{"text":"do X","rationale":"why"}],"tasks":[{"text":"write tests","estimate":"1h","category":"test"}]}`
}

// fakeGitHubResolver is a test stub for BacklogService.resolveGitHubInput. It
// records every input it was called with so tests can assert whether a plain
// local path bypassed resolution entirely.
type fakeGitHubResolver struct {
	calls     []string
	localPath string
	err       error
}

func (f *fakeGitHubResolver) resolve(input string) (string, *session.GitHubRef, error) {
	f.calls = append(f.calls, input)
	if f.err != nil {
		return "", nil, f.err
	}
	return f.localPath, &session.GitHubRef{}, nil
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
	// contextFileExistedAtSpawn/slashCommandsExistedAtSpawn are captured at the
	// moment CreateDirectorySession fires — i.e. the moment the real claude process
	// would start executing. This is the regression guard for the write-before-spawn
	// ordering fix: without it, the file writes could silently move back to *after*
	// spawn (as they were before this PR) and no test would catch it, since checking
	// file existence only after SpawnSessionFromItem returns can't distinguish
	// "written before spawn" from "written after spawn but before the RPC returned."
	contextFileExistedAtSpawn   bool
	slashCommandsExistedAtSpawn bool
	// inst is the *session.Instance returned to the caller, captured here so
	// tests can inspect post-return mutations (e.g. SetCategory) that the
	// production code performs on the instance after CreateDirectorySession/
	// CreateWorktreeSession returns it.
	inst *session.Instance
}

func (m *mockSessionCreator) CreateDirectorySession(_ context.Context, title, path, prompt string, tags []string, oneShot bool, _ bool) (*session.Instance, error) {
	_, contextErr := os.Stat(filepath.Join(path, ".backlog-context.md"))
	_, slashErr := os.Stat(filepath.Join(path, ".claude", "commands", "backlog", "status.md"))
	if m.err != nil {
		m.calls = append(m.calls, mockCreateCall{
			title:                       title,
			path:                        path,
			prompt:                      prompt,
			tags:                        tags,
			oneShot:                     oneShot,
			contextFileExistedAtSpawn:   contextErr == nil,
			slashCommandsExistedAtSpawn: slashErr == nil,
		})
		return nil, m.err
	}
	// Path must round-trip: SpawnSessionFromItem writes slash commands and a
	// context file to inst.Path. An empty Path here makes those writes land in
	// the test process's working directory instead of a sandbox.
	inst := &session.Instance{Title: title, Path: path}
	m.calls = append(m.calls, mockCreateCall{
		title:                       title,
		path:                        path,
		prompt:                      prompt,
		tags:                        tags,
		oneShot:                     oneShot,
		contextFileExistedAtSpawn:   contextErr == nil,
		slashCommandsExistedAtSpawn: slashErr == nil,
		inst:                        inst,
	})
	return inst, nil
}

// CreateWorktreeSession records the call to the same calls slice as CreateDirectorySession,
// using worktreePath as the session path (that's where files are written before spawn).
func (m *mockSessionCreator) CreateWorktreeSession(_ context.Context, title, _, worktreePath, prompt string, tags []string, oneShot bool, _ bool) (*session.Instance, error) {
	_, contextErr := os.Stat(filepath.Join(worktreePath, ".backlog-context.md"))
	_, slashErr := os.Stat(filepath.Join(worktreePath, ".claude", "commands", "backlog", "status.md"))
	if m.err != nil {
		m.calls = append(m.calls, mockCreateCall{
			title:                       title,
			path:                        worktreePath,
			prompt:                      prompt,
			tags:                        tags,
			oneShot:                     oneShot,
			contextFileExistedAtSpawn:   contextErr == nil,
			slashCommandsExistedAtSpawn: slashErr == nil,
		})
		return nil, m.err
	}
	inst := &session.Instance{Title: title, Path: worktreePath}
	m.calls = append(m.calls, mockCreateCall{
		title:                       title,
		path:                        worktreePath,
		prompt:                      prompt,
		tags:                        tags,
		oneShot:                     oneShot,
		contextFileExistedAtSpawn:   contextErr == nil,
		slashCommandsExistedAtSpawn: slashErr == nil,
		inst:                        inst,
	})
	return inst, nil
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

// UT-014a: RepoPath that looks like a GitHub URL is resolved to a local clone path.
func TestCreateBacklogItem_ResolvesGitHubURL(t *testing.T) {
	svc := newBacklogService(t)
	resolver := &fakeGitHubResolver{localPath: "/tmp/fake-clone/owner/repo"}
	svc.SetGitHubResolver(resolver.resolve)

	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "clone this repo",
		RepoPath: "https://github.com/owner/repo",
	}))
	require.NoError(t, err)

	assert.Equal(t, "/tmp/fake-clone/owner/repo", resp.Msg.Item.RepoPath)
	require.Len(t, resolver.calls, 1)
	assert.Equal(t, "https://github.com/owner/repo", resolver.calls[0])
}

// UT-014b: A resolver failure (e.g. clone error) surfaces as CodeInvalidArgument
// with the original input in the message, not a silent failure downstream.
func TestCreateBacklogItem_GitHubResolveError_ReturnsInvalidArgument(t *testing.T) {
	svc := newBacklogService(t)
	resolver := &fakeGitHubResolver{err: errors.New("clone failed: repository not found")}
	svc.SetGitHubResolver(resolver.resolve)

	_, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "bad repo",
		RepoPath: "https://github.com/owner/does-not-exist",
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
	assert.Contains(t, connErr.Error(), "https://github.com/owner/does-not-exist")
}

// UT-014c: A plain local filesystem path is stored as-is and never passed to the resolver.
func TestCreateBacklogItem_PlainPath_DoesNotCallResolver(t *testing.T) {
	svc := newBacklogService(t)
	resolver := &fakeGitHubResolver{localPath: "should-not-be-used"}
	svc.SetGitHubResolver(resolver.resolve)

	resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "local repo",
		RepoPath: "/home/user/projects/my-repo",
	}))
	require.NoError(t, err)

	assert.Equal(t, "/home/user/projects/my-repo", resp.Msg.Item.RepoPath)
	assert.Empty(t, resolver.calls)
}

// ─── UpdateBacklogItem ────────────────────────────────────────────────────────

// UT-015a: Updating repo_path with a GitHub URL resolves it to a local clone path.
func TestUpdateBacklogItem_ResolvesGitHubURL(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item to fix up",
	}))
	require.NoError(t, err)

	resolver := &fakeGitHubResolver{localPath: "/tmp/fake-clone/owner/repo"}
	svc.SetGitHubResolver(resolver.resolve)

	resp, err := svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:   created.Msg.Item.Id,
		RepoPath: "https://github.com/owner/repo",
	}))
	require.NoError(t, err)

	assert.Equal(t, "/tmp/fake-clone/owner/repo", resp.Msg.Item.RepoPath)
	require.Len(t, resolver.calls, 1)
	assert.Equal(t, "https://github.com/owner/repo", resolver.calls[0])
}

// UT-015b: A resolver failure on update surfaces as CodeInvalidArgument — this is the
// fix-up path for an item created with a bad (unresolvable) repo_path.
func TestUpdateBacklogItem_GitHubResolveError_ReturnsInvalidArgument(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item to fix up",
	}))
	require.NoError(t, err)

	resolver := &fakeGitHubResolver{err: errors.New("clone failed")}
	svc.SetGitHubResolver(resolver.resolve)

	_, err = svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:   created.Msg.Item.Id,
		RepoPath: "https://github.com/owner/does-not-exist",
	}))
	require.Error(t, err)

	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
	assert.Contains(t, connErr.Error(), "https://github.com/owner/does-not-exist")
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

// TestListBacklogItems_DoneStatusIsTerminal verifies that an item actually in
// "done" status is hidden by the default list (includeTerminal=false) and visible
// with includeTerminal=true.
//
// Regression test for the bug where the board page omitted includeTerminal:true
// and completed work disappeared from view.
//
// Pre-fix behaviour: done items were excluded (test would FAIL on assert.Contains).
// Post-fix behaviour: board always sends includeTerminal:true (test passes).
func TestListBacklogItems_DoneStatusIsTerminal(t *testing.T) {
	svc := newBacklogService(t)

	create := func(title string) string {
		resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
			Title:        title,
			SkipPlanning: true,
			// AC criteria required for idea→ready transition.
			AcceptanceCriteria: []*sessionv1.AcCriterion{
				{Index: 0, Text: "it works", Status: "pending"},
			},
		}))
		require.NoError(t, err)
		return resp.Msg.Item.Id
	}
	transition := func(itemID, to string, extra ...*sessionv1.TransitionBacklogItemStatusRequest) {
		req := &sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:       itemID,
			TargetStatus: to,
		}
		if len(extra) > 0 {
			req.OverrideReason = extra[0].OverrideReason
		}
		_, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(req))
		require.NoError(t, err, "transition to %s", to)
	}

	activeID := create("active item")
	doneID := create("done item")

	// Walk "done item" through the state machine: idea → ready → in_progress → review → done.
	// review→done requires OverrideReason because no verdict has been recorded.
	transition(doneID, "ready")
	transition(doneID, "in_progress")
	transition(doneID, "review")
	transition(doneID, "done", &sessionv1.TransitionBacklogItemStatusRequest{OverrideReason: "test override"})
	_ = activeID // stays in idea

	// Default list (includeTerminal omitted) must hide done items.
	listDefault, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{}))
	require.NoError(t, err)
	defaultTitles := make([]string, 0, len(listDefault.Msg.Items))
	for _, it := range listDefault.Msg.Items {
		defaultTitles = append(defaultTitles, it.Title)
	}
	assert.Contains(t, defaultTitles, "active item", "non-terminal items must appear in default list")
	assert.NotContains(t, defaultTitles, "done item", "done items must be hidden in default list")

	// includeTerminal:true (what the board page sends) must show done items.
	listAll, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{
		IncludeTerminal: true,
	}))
	require.NoError(t, err)
	allTitles := make([]string, 0, len(listAll.Msg.Items))
	for _, it := range listAll.Msg.Items {
		allTitles = append(allTitles, it.Title)
	}
	assert.Contains(t, allTitles, "done item", "done items must appear when includeTerminal=true")
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

// initGitRepoWithCommit initialises a minimal git repository with an initial commit so
// that git worktree operations (which require at least one commit) work in tests.
// Skips the test if git is unavailable.
func initGitRepoWithCommit(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Repo\n"), 0o644); err != nil {
		t.Skipf("write README: %v", err)
	}
	for _, args := range [][]string{
		{"init", dir},
		{"-C", dir, "config", "user.email", "test@example.com"},
		{"-C", dir, "config", "user.name", "Test"},
		{"-C", dir, "add", "README.md"},
		{"-C", dir, "commit", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...) //nolint:norawexec // test helper, blocking CombinedOutput, no zombie risk
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v failed: %v (%s) — cannot run worktree test", args, err, out)
		}
	}
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
	// SpawnSessionFromItem now creates a real git worktree — requires a valid repo with at least one commit.
	initGitRepoWithCommit(t, repoPath)

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

	// 5a. The write-before-spawn ordering fix: slash commands and the context file
	// must already exist on disk at the moment CreateDirectorySession fires (i.e.
	// before the claude process would start), not written afterward.
	assert.True(t, creator.calls[0].contextFileExistedAtSpawn,
		".backlog-context.md must exist before the session is spawned, not written after")
	assert.True(t, creator.calls[0].slashCommandsExistedAtSpawn,
		"slash command files must exist before the session is spawned, not written after")

	// 5b. Backlog work sessions must be categorized "Backlog" so they group
	// correctly in the session list UI instead of falling into "Uncategorized".
	assert.Equal(t, session.CategoryBacklog, creator.calls[0].inst.Category,
		"backlog work session should have Category == Backlog")

	// 6. Item should have advanced to in_progress after spawn.
	finalResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, "in_progress", finalResp.Msg.Item.Status)
}

// TestSpawnSessionFromItem_AutonomousBypassesPlanningGate verifies that
// Autonomous=true allows spawning a ready item without plan approval.
// Regression for: "run autonomously not working on many sessions" — the planning
// gate was not bypassed for autonomous mode, so any ready item without an approved
// plan failed even though the autonomous driver handles its own planning loop.
func TestSpawnSessionFromItem_AutonomousBypassesPlanningGate(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil)
	starter := &fakeAutonomousDriverStarter{}
	svc.SetAutonomousDriverStarter(starter)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Create a ready item without plan approval (skip_planning=false, plan_approved=false).
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:      "autonomous gate test",
		RepoPath:   repoPath,
		SkipTriage: true,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "it works", Status: "pending"},
		},
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Manually advance to "ready" via TransitionBacklogItemStatus (skipping real triage).
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)

	// Verify the item has no plan approval yet.
	getResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.False(t, getResp.Msg.Item.PlanApproved, "setup: item must not have plan approval")
	require.False(t, getResp.Msg.Item.SkipPlanning, "setup: skip_planning must be false")

	// Non-autonomous spawn must still be blocked by the planning gate.
	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId: itemID,
	}))
	require.Error(t, err, "non-autonomous spawn without plan approval must fail")

	// Autonomous spawn must bypass the planning gate.
	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId:     itemID,
		Autonomous: true,
	}))
	require.NoError(t, err, "autonomous spawn must succeed without plan approval")
	require.Len(t, starter.calls, 1, "autonomous driver start hook must fire")
}

// TestSpawnSessionFromItem_Reopen_SetsBacklogCategory verifies that a
// revision-reopen spawn (item already in_progress, isReopen=true in
// SpawnSessionFromItem) also gets Category == "Backlog", not just the
// initial work-session spawn covered by TestBacklogFullLifecycle above.
func TestSpawnSessionFromItem_Reopen_SetsBacklogCategory(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "reopen item",
		RepoPath: repoPath,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Drive status straight to in_progress (no real first spawn) so
	// SpawnSessionFromItem below takes the isReopen=true branch.
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

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)

	require.Len(t, creator.calls, 1)
	assert.Contains(t, creator.calls[0].tags, session.TagBacklogRevision,
		"reopen spawn should carry the revision tag")
	assert.Equal(t, session.CategoryBacklog, creator.calls[0].inst.Category,
		"backlog revision-reopen session should have Category == Backlog")
}

// ─── AttachSessionToItem ──────────────────────────────────────────────────────

// TestAttachSessionToItem_WritesContextFileWithPlanArtifactsAndPriorSessions is a
// regression test for two architecture-review findings: (1) AttachSessionToItem's
// entItem previously omitted PlanArtifactsPath/PlanApproved/SkipPlanning, so the
// plan-artifacts reminder now living inside BuildSessionInitialPrompt could never
// render on the attach path even when the item had an approved plan; (2) prior
// sessions must actually reach the written context file the same way they do for
// SpawnSessionFromItem.
func TestAttachSessionToItem_WritesContextFileWithPlanArtifactsAndPriorSessions(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	repoPath := t.TempDir()
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:      "attach me",
		RepoPath:   repoPath,
		SkipTriage: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// Give the item an approved plan directly via storage (bypassing full triage),
	// matching the pattern used elsewhere in this file for ApprovePlan tests.
	artifactsPath := t.TempDir()
	planApproved := true
	_, err = storage.UpdateBacklogItem(t.Context(), itemID, session.BacklogItemUpdate{
		PlanArtifactsPath: &artifactsPath,
		PlanApproved:      &planApproved,
	}, nil)
	require.NoError(t, err)

	// A prior, already-ended session for this item.
	priorIS, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: "prior-session-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), priorIS.ID, time.Now().Add(-time.Hour)))

	// A live Instance at repoPath, discoverable by AttachSessionToItem's
	// storage.LoadInstances() lookup.
	const attachUUID = "attach-session-uuid"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title: "attach-target",
		UUID:  attachUUID,
		Path:  repoPath,
		// Paused (not Active) so LoadInstances doesn't attempt a real cold-restore
		// tmux/claude process start — AttachSessionToItem only needs UUID+Path to
		// match, not a live process, and a real restore attempt is slow/unreliable
		// in CI (no claude binary, no real tmux server) even if it eventually
		// succeeds locally.
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	_, err = svc.AttachSessionToItem(t.Context(), connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
		ItemId:      itemID,
		SessionUuid: attachUUID,
	}))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoPath, ".backlog-context.md"))
	require.NoError(t, err, "AttachSessionToItem must write .backlog-context.md to the instance's path")
	content := string(data)

	assert.Contains(t, content, artifactsPath+"/plan.md",
		"attach-flow context file must include the plan-artifacts reminder when the item has an approved plan")
	assert.Contains(t, content, "Prior Attempts",
		"attach-flow context file must include prior session history")

	// The just-created attach session itself must not appear as a second "prior
	// attempt" entry — it has no EndedAt yet, so BuildSessionInitialPrompt's filter
	// naturally excludes it, but count the rendered entries to guard against a
	// future change that stops filtering on EndedAt.
	assert.Equal(t, 1, strings.Count(content, "- Role:"),
		"only the one real prior (ended) session should be rendered, not the just-created attach session")
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
	storage := createTestStorage(t)
	// delay outlasts the cleanup timeout below — this is what the old code got
	// wrong: a cleanupCtx created before this delay would already be expired by
	// the time it's used afterward.
	pool := &fakeHeadlessPool{response: validTriageJSON(), delay: 3 * time.Second}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	// Reduced but kept generous enough (2s) that the real SQLite writes below
	// can't flake under CI load or a busy test machine — the bug being tested
	// is about ORDERING (timeout starts before vs. after the slow call), not
	// about needing a tiny timeout, so there's no reason to cut this closer to
	// the wire. Set on this instance only — no shared global state, no risk to
	// any other concurrently running test.
	svc.SetTriageCleanupTimeout(2 * time.Second)

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

// TestTriggerReReview_SetsBacklogCategory verifies that TriggerReReview with a
// SessionCreator wired spawns the re-review session with Category == "Backlog"
// so it groups correctly in the session list UI.
func TestTriggerReReview_SetsBacklogCategory(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil)

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

	_, err = svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)

	require.Len(t, creator.calls, 1)
	assert.Equal(t, session.CategoryBacklog, creator.calls[0].inst.Category,
		"re-review session should have Category == Backlog")
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
	require.NoError(t, storage.UpdateItemSessionStarted(t.Context(), is.ID, time.Now()))

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
	require.NoError(t, storage.UpdateItemSessionTriageResult(t.Context(), is.ID, triageJSON))

	// Re-load and convert.
	sessions, loadErr := storage.ListItemSessions(t.Context(), item.ID)
	require.NoError(t, loadErr)
	require.Len(t, sessions, 1)

	proto := itemSessionToProto(sessions[0], nil)
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
	require.NoError(t, storage.UpdateItemSessionTriageResult(t.Context(), is.ID, "{not valid json"))

	// Must not panic.
	require.NotPanics(t, func() {
		sessions, _ := storage.ListItemSessions(t.Context(), item.ID)
		if len(sessions) > 0 {
			proto := itemSessionToProto(sessions[0], nil)
			// TriageResult should be nil because JSON was invalid.
			assert.Nil(t, proto.TriageResult)
		}
	})
}

// errSessionCreator always returns an error from CreateDirectorySession and CreateWorktreeSession.
type errSessionCreator struct{ err error }

func (e *errSessionCreator) CreateDirectorySession(_ context.Context, _, _, _ string, _ []string, _ bool, _ bool) (*session.Instance, error) {
	return nil, e.err
}

func (e *errSessionCreator) CreateWorktreeSession(_ context.Context, _, _, _, _ string, _ []string, _ bool, _ bool) (*session.Instance, error) {
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

// TestTriggerTriage_RefineWithFeedback: a second TriggerTriage call with feedback
// set produces a distinct revised result (iteration 2), embeds the prior result and
// the feedback text in the prompt, and both triage ItemSessions are retained.
func TestTriggerTriage_RefineWithFeedback(t *testing.T) {
	storage := createTestStorage(t)
	secondResponse := `{"summary":"revised summary","suggestions":[],"tasks":[{"text":"revised task","estimate":"3h","category":"backend"}]}`
	pool := &fakeHeadlessPool{responses: []string{validTriageJSON(), secondResponse}}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "refine me",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	// Initial triage.
	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "initial triage should mark item ready")

	// Refine with feedback.
	_, refineErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId:   item.ID,
		Feedback: "This missed the mobile case entirely.",
	}))
	require.NoError(t, refineErr)
	require.Eventually(t, func() bool {
		return pool.callCount() == 2
	}, 5*time.Second, 50*time.Millisecond, "refine should make a second headless call")
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "refine should mark item ready again")

	// The refine prompt embeds the prior result and the feedback text.
	assert.Contains(t, pool.callAt(1).userPrompt, "test summary")
	assert.Contains(t, pool.callAt(1).userPrompt, "This missed the mobile case entirely.")

	// Both triage ItemSessions are retained, most recent carries the revised result + iteration 2.
	sessions, listErr := storage.ListItemSessions(t.Context(), item.ID)
	require.NoError(t, listErr)
	require.Len(t, sessions, 2)
	assert.Contains(t, sessions[1].TriageResult, "revised summary")
	assert.Contains(t, sessions[1].TriageResult, `"iteration":2`)
}

// TestTriggerTriage_RefineWithFeedback_RequiresPriorResult: feedback on an item
// with no completed triage result is rejected rather than silently running a
// fresh triage as if the feedback were ignored.
func TestTriggerTriage_RefineWithFeedback_RequiresPriorResult(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "no prior triage",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId:   item.ID,
		Feedback: "please improve this",
	}))
	require.Error(t, trigErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(trigErr))
	assert.Equal(t, 0, pool.callCount(), "no headless call should be made when there's nothing to refine")
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

// ─── TriggerSync / GetSyncHistory ──────────────────────────────────────────────

// fakeSourcePlugin is a minimal session.ItemSourcePlugin stub for TriggerSync tests.
type fakeSourcePlugin struct {
	items      []session.ExternalItem
	fetchErr   error
	lastConfig session.PluginConfig
}

func (f *fakeSourcePlugin) PluginID() string { return "fake_source" }

func (f *fakeSourcePlugin) Fetch(_ context.Context, cfg session.PluginConfig, cursor string) ([]session.ExternalItem, string, error) {
	f.lastConfig = cfg
	if f.fetchErr != nil {
		return nil, cursor, f.fetchErr
	}
	return f.items, cursor, nil
}

func (f *fakeSourcePlugin) MapToBacklogItem(item session.ExternalItem, sourceID string) session.BacklogItemData {
	return session.BacklogItemData{
		Title:      item.Title,
		Status:     string(session.BacklogStatusIdea),
		ExternalID: item.ExternalID,
		SourceID:   sourceID,
	}
}

func TestTriggerSync_ReturnsUnimplementedWithoutPluginRegistry(t *testing.T) {
	svc := newBacklogService(t)
	_, err := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{SourceId: "any"}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeUnimplemented, connErr.Code())
}

func TestTriggerSync_ReturnsFailedPreconditionWhenFeatureDisabled(t *testing.T) {
	svc := newBacklogService(t)
	registry := session.NewPluginRegistry()
	registry.Register(&fakeSourcePlugin{})
	svc.SetPluginRegistry(registry)
	svc.SetSyncFeatureEnabledCheck(func() bool { return false })

	_, err := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{SourceId: "any"}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeFailedPrecondition, connErr.Code())
}

func TestTriggerSync_SucceedsWhenFeatureEnabledCheckReturnsTrue(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)
	registry := session.NewPluginRegistry()
	plugin := &fakeSourcePlugin{items: []session.ExternalItem{{ExternalID: "e1", Title: "Item"}}}
	registry.Register(plugin)
	svc.SetPluginRegistry(registry)
	svc.SetSyncFeatureEnabledCheck(func() bool { return true })

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Fake Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	_, syncErr := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{SourceId: src.ID}))
	require.NoError(t, syncErr)
}

func TestTriggerSync_ReturnsInvalidArgumentWhenSourceIDEmpty(t *testing.T) {
	svc := newBacklogService(t)
	svc.SetPluginRegistry(session.NewPluginRegistry())
	_, err := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{SourceId: ""}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestTriggerSync_ReturnsNotFoundForMissingSource(t *testing.T) {
	svc := newBacklogService(t)
	registry := session.NewPluginRegistry()
	registry.Register(&fakeSourcePlugin{})
	svc.SetPluginRegistry(registry)

	_, err := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{
		SourceId: "00000000-0000-0000-0000-000000000000",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
}

func TestTriggerSync_ReturnsInvalidArgumentForMalformedSourceID(t *testing.T) {
	svc := newBacklogService(t)
	registry := session.NewPluginRegistry()
	registry.Register(&fakeSourcePlugin{})
	svc.SetPluginRegistry(registry)

	_, err := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{
		SourceId: "not-a-uuid",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestTriggerSync_SucceedsAndCreatesItems(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)
	registry := session.NewPluginRegistry()
	plugin := &fakeSourcePlugin{items: []session.ExternalItem{{ExternalID: "e1", Title: "Synced Item"}}}
	registry.Register(plugin)
	svc.SetPluginRegistry(registry)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Fake Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	_, syncErr := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{SourceId: src.ID}))
	require.NoError(t, syncErr)

	historyResp, histErr := svc.GetSyncHistory(t.Context(), connect.NewRequest(&sessionv1.GetSyncHistoryRequest{SourceId: src.ID}))
	require.NoError(t, histErr)
	require.Len(t, historyResp.Msg.Events, 1)
	assert.Equal(t, int32(1), historyResp.Msg.Events[0].ItemsCreated)
}

// TestTriggerSync_DecryptsTokenThroughServiceLayer exercises the
// SetSyncKeyFunc wiring end-to-end through the RPC handler — the unit-level
// SyncLoop tests in session/backlog_sync_test.go cover decryption but bypass
// BacklogService.TriggerSync entirely, so a regression in this wiring
// (wrong key func, or the branch removed) wouldn't be caught without this.
func TestTriggerSync_DecryptsTokenThroughServiceLayer(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)
	registry := session.NewPluginRegistry()
	plugin := &fakeSourcePlugin{}
	registry.Register(plugin)
	svc.SetPluginRegistry(registry)

	key := make([]byte, 32)
	svc.SetSyncKeyFunc(func() ([]byte, error) { return key, nil })

	encToken, err := session.EncryptToken(key, "plaintext-token")
	require.NoError(t, err)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Encrypted Source",
		Enabled:     true,
		Config:      `{"encrypted":true,"token":"` + encToken + `"}`,
	})
	require.NoError(t, err)

	_, syncErr := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{SourceId: src.ID}))
	require.NoError(t, syncErr)

	assert.Contains(t, plugin.lastConfig.Raw, "plaintext-token")
	assert.NotContains(t, plugin.lastConfig.Raw, "encrypted")
}

func TestTriggerSync_PropagatesFetchError(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)
	registry := session.NewPluginRegistry()
	plugin := &fakeSourcePlugin{fetchErr: errors.New("upstream boom")}
	registry.Register(plugin)
	svc.SetPluginRegistry(registry)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    plugin.PluginID(),
		DisplayName: "Fake Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	_, syncErr := svc.TriggerSync(t.Context(), connect.NewRequest(&sessionv1.TriggerSyncRequest{SourceId: src.ID}))
	require.Error(t, syncErr)
	var connErr *connect.Error
	require.ErrorAs(t, syncErr, &connErr)
	assert.Equal(t, connect.CodeInternal, connErr.Code())
}

func TestGetSyncHistory_ReturnsInvalidArgumentWhenSourceIDEmpty(t *testing.T) {
	svc := newBacklogService(t)
	_, err := svc.GetSyncHistory(t.Context(), connect.NewRequest(&sessionv1.GetSyncHistoryRequest{SourceId: ""}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

// A malformed (non-UUID) source_id must be rejected as CodeInvalidArgument,
// not surfaced as CodeInternal — the storage layer's parse error isn't a
// server-side failure, it's bad client input.
func TestGetSyncHistory_ReturnsInvalidArgumentForMalformedSourceID(t *testing.T) {
	svc := newBacklogService(t)
	_, err := svc.GetSyncHistory(t.Context(), connect.NewRequest(&sessionv1.GetSyncHistoryRequest{SourceId: "not-a-uuid"}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeInvalidArgument, connErr.Code())
}

func TestGetSyncHistory_ReturnsEmptyForSourceWithNoSyncRuns(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    "fake_source",
		DisplayName: "Never Synced",
		Enabled:     true,
	})
	require.NoError(t, err)

	resp, histErr := svc.GetSyncHistory(t.Context(), connect.NewRequest(&sessionv1.GetSyncHistoryRequest{SourceId: src.ID}))
	require.NoError(t, histErr)
	assert.Empty(t, resp.Msg.Events)
	assert.False(t, resp.Msg.Truncated)
}

// TestGetSyncHistory_SetsTruncatedWhenHistoryExceedsCap verifies the RPC surfaces
// the storage layer's truncation signal, so the settings UI can show a "history not
// fully shown" indicator instead of silently capping at 200 with no explanation.
func TestGetSyncHistory_SetsTruncatedWhenHistoryExceedsCap(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil)

	src, err := storage.CreateItemSource(t.Context(), session.ItemSourceData{
		PluginID:    "fake_source",
		DisplayName: "Chatty Source",
		Enabled:     true,
	})
	require.NoError(t, err)

	start := time.Now()
	for i := 0; i < 201; i++ {
		require.NoError(t, storage.CreateSourceSyncEvent(t.Context(), src.ID, "", 1, 0, 0, 0, "", start, start))
	}

	resp, histErr := svc.GetSyncHistory(t.Context(), connect.NewRequest(&sessionv1.GetSyncHistoryRequest{SourceId: src.ID}))
	require.NoError(t, histErr)
	assert.Len(t, resp.Msg.Events, 200)
	assert.True(t, resp.Msg.Truncated)
}
