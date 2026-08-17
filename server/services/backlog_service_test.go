package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// TestMain pre-seeds headless.DefaultCapabilitySelfCheck as passed before any test
// runs. NewBacklogService defaults every instance's capabilityCheck field to that
// package-level singleton (guarded by sync.Once, deliberately cached for the whole
// process lifetime in production — see capability_check.go). Left unseeded, the
// first test in this binary to reach the codebase-read gate without calling
// SetCapabilityCheck "wins" the once.Do race and permanently resolves the
// singleton based on whether ITS OWN fakeHeadlessPool response happens to contain
// the capability marker string (it doesn't — the fakes return scripted verdict
// JSON) — poisoning it to failed for every other test in the package for the rest
// of the process, regardless of test order or -count. That was the actual root
// cause behind TestAutoRespawnReview_DeadWorkSession_TombstonedThenRespawns'
// order-dependent flake (reliably 1-pass-then-every-subsequent-run-fails under
// -count=N in one process). Tests that specifically exercise the capability-check
// failure/success path still override it per-instance via SetCapabilityCheck.
//
// It also clears GITHUB_TOKEN/GH_TOKEN for the whole test run, mirroring
// github/main_test.go's TestMain: this package drives UserPRCache directly
// (e.g. TestPreviewDestinationPath_GitHubURL_EnterpriseHostViaCachedAccount_ReturnsExactPath),
// and collectAllTokens reads both env vars straight from the environment, so
// a developer machine or CI runner with either set would otherwise leak a
// real token into the cache and dial the real GitHub API mid-suite.
func TestMain(m *testing.M) {
	headless.DefaultCapabilitySelfCheck = headless.NewPassedCapabilitySelfCheckForTesting()
	restore := envtest.ClearAmbientGitHubTokenEnv()
	code := m.Run()
	restore()
	os.Exit(code)
}

// ─── fakeHeadlessPool ─────────────────────────────────────────────────────────

// fakeHeadlessPool is a test stub implementing headless.PoolClient.
type fakeHeadlessPool struct {
	mu        sync.Mutex
	response  string
	responses []string // if set, returned in order (one per call) instead of response
	err       error
	delay     time.Duration // simulates a slow LLM call; see TestTriggerTriage_SlowLLMCallDoesNotExpireCleanupContext
	cost      float64       // returned as CallBlocking's cost; see TestTriggerReReview_HappyPath_ThreadsCallCostIntoItemSession
	calls     []fakePoolCall
	onCall    func(workDir string) // optional: simulates the LLM writing files into WorkDir before the response returns
}

type fakePoolCall struct {
	key            headless.FeatureKey
	workDir        string
	userPrompt     string
	systemPrompt   string
	allowedTools   string
	permissionMode string
	ctxDeadline    time.Time
	hasDeadline    bool
}

func (f *fakeHeadlessPool) CallBlocking(ctx context.Context, key headless.FeatureKey, systemPrompt, userPrompt string, opts headless.CallOptions) (string, float64, error) {
	f.mu.Lock()
	callIndex := len(f.calls)
	deadline, hasDeadline := ctx.Deadline()
	f.calls = append(f.calls, fakePoolCall{
		key:            key,
		workDir:        opts.WorkDir,
		userPrompt:     userPrompt,
		systemPrompt:   systemPrompt,
		allowedTools:   opts.AllowedTools,
		permissionMode: opts.PermissionMode,
		ctxDeadline:    deadline,
		hasDeadline:    hasDeadline,
	})
	delay := f.delay
	resp := f.response
	if callIndex < len(f.responses) {
		resp = f.responses[callIndex]
	}
	onCall := f.onCall
	f.mu.Unlock()
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return "", 0, ctx.Err()
		}
	}
	if onCall != nil {
		onCall(opts.WorkDir)
	}
	return resp, f.cost, f.err
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
	liveUUIDs         map[string]bool
	killedPaneUUIDs   []string
	archivedUUIDs     []string
	archiveErrForUUID map[string]error
	// staleFor maps a session UUID to the "time since last meaningful output"
	// TimeSinceLastMeaningfulOutput should report for it. A UUID present in
	// liveUUIDs but absent here reports (0, true) — live and fresh.
	staleFor map[string]time.Duration
	// tslmoOverrideNotLive forces TimeSinceLastMeaningfulOutput to report
	// live=false for a UUID even though it's present (true) in liveUUIDs, so
	// IsSessionLive still reports it as live. Models the real SessionService
	// implementation's rare edge case where a session is deregistered from the
	// live poller between an earlier IsSessionLive check (e.g.
	// tombstoneOrphanWorkSessions' orphan sweep) and a later
	// TimeSinceLastMeaningfulOutput call in the same request — both are backed
	// by the same FindLiveInstance lookup in production, so they normally
	// agree, but a caller enriching an error message with the progress signal
	// must still handle the disagreement gracefully rather than assume it. Nil
	// (the default) preserves the old coupled behavior for every other test.
	tslmoOverrideNotLive map[string]bool
	// onKillTmuxPaneOnly, if set, is invoked synchronously from
	// KillTmuxPaneOnly before it records the call — lets a test observe
	// storage state at the exact moment the pane would be killed in
	// production (BUG-064: RemediateStaleWorkSession must end the
	// ItemSession row before calling KillTmuxPaneOnly, not after, so that
	// onSessionExited's already-ended guard has something to observe once
	// the (real) tmux kill asynchronously fires the exit event).
	onKillTmuxPaneOnly func(uuid string)
}

func (m *mockSessionStopper) IsSessionLive(uuid string) bool {
	return m.liveUUIDs[uuid]
}

func (m *mockSessionStopper) TimeSinceLastMeaningfulOutput(uuid string) (time.Duration, bool) {
	if !m.liveUUIDs[uuid] || m.tslmoOverrideNotLive[uuid] {
		return 0, false
	}
	return m.staleFor[uuid], true
}

func (m *mockSessionStopper) StopSessionByUUID(_ context.Context, _ string) error { return nil }

func (m *mockSessionStopper) KillTmuxSessionByTitle(_ context.Context, _ string) error {
	return nil
}

func (m *mockSessionStopper) KillTmuxPaneOnly(_ context.Context, uuid string) error {
	if m.onKillTmuxPaneOnly != nil {
		m.onKillTmuxPaneOnly(uuid)
	}
	m.killedPaneUUIDs = append(m.killedPaneUUIDs, uuid)
	return nil
}

func (m *mockSessionStopper) ArchiveSessionByUUID(_ context.Context, uuid string) error {
	if m.archiveErrForUUID != nil {
		if err, ok := m.archiveErrForUUID[uuid]; ok {
			return err
		}
	}
	m.archivedUUIDs = append(m.archivedUUIDs, uuid)
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
	// UUID must be unique per call — SpawnSessionFromItem's ItemSession row is
	// keyed on inst.UUID, so archival-tracking tests need distinct values per
	// spawn to tell rounds apart (see mockSessionStopper.archivedUUIDs).
	inst := &session.Instance{Title: title, Path: path, UUID: fmt.Sprintf("mock-session-%d", len(m.calls))}
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
	inst := &session.Instance{Title: title, Path: worktreePath, UUID: fmt.Sprintf("mock-session-%d", len(m.calls))}
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
	return NewBacklogService(createTestStorage(t), nil, nil, nil, nil, nil)
}

func newBacklogServiceNilStorage() *BacklogService {
	return NewBacklogService(nil, nil, nil, nil, nil, nil)
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

// TestUpdateBacklogItem_PopulatesUserModifiedFieldsOnTitleEdit is the Epic 0.3
// prerequisite test (plan.md Task 0.3.2c): a genuinely different Title
// populates the item's UserModifiedFields with "title".
func TestUpdateBacklogItem_PopulatesUserModifiedFieldsOnTitleEdit(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "Original Title",
	}))
	require.NoError(t, err)

	_, err = svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId: created.Msg.Item.Id,
		Title:  "New Title",
	}))
	require.NoError(t, err)

	refetched, err := svc.storage.GetBacklogItem(t.Context(), created.Msg.Item.Id)
	require.NoError(t, err)
	assert.Equal(t, "New Title", refetched.Title)
	assert.True(t, session.ContainsModifiedField(session.ParseUserModifiedFields(refetched.UserModifiedFields), "title"),
		"UserModifiedFields must contain \"title\" after a genuine title edit, got %q", refetched.UserModifiedFields)
}

// TestUpdateBacklogItem_DoesNotMarkTitleModifiedWhenValueUnchanged is the
// regression test for the pre-mortem P1 #2 value-diff correction (plan.md
// Task 0.3.2b/0.3.2c): the frontend edit form always resubmits the current
// Title verbatim, so a presence-only check would falsely mark it as
// user-modified on nearly every edit. Resubmitting the unchanged Title
// alongside a genuinely different Priority must mark only "priority".
func TestUpdateBacklogItem_DoesNotMarkTitleModifiedWhenValueUnchanged(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "Unchanged Title",
		Priority: 3,
	}))
	require.NoError(t, err)

	_, err = svc.UpdateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UpdateBacklogItemRequest{
		ItemId:   created.Msg.Item.Id,
		Title:    "Unchanged Title", // resubmitted verbatim, mirroring the real edit form
		Priority: 5,                 // genuinely changed
	}))
	require.NoError(t, err)

	refetched, err := svc.storage.GetBacklogItem(t.Context(), created.Msg.Item.Id)
	require.NoError(t, err)
	assert.Equal(t, 5, refetched.Priority)
	modified := session.ParseUserModifiedFields(refetched.UserModifiedFields)
	assert.True(t, session.ContainsModifiedField(modified, "priority"), "priority genuinely changed, must be marked modified")
	assert.False(t, session.ContainsModifiedField(modified, "title"), "title was resubmitted unchanged, must NOT be marked modified")
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

// ─── UnarchiveBacklogItem ───────────────────────────────────────────────────

// PR #499 code review, MODERATE finding: no RPC-level test existed for the
// UnarchiveBacklogItem handler (server/services/backlog_service_lifecycle.go).
// Mirrors ArchiveBacklogItem's coverage shape: a success path that restores an
// archived item to "idea" and reappears in the default list, plus a not-found
// error path mapped to connect.CodeNotFound.
func TestUnarchiveBacklogItem_Success(t *testing.T) {
	svc := newBacklogService(t)

	created, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item to unarchive",
	}))
	require.NoError(t, err)
	itemID := created.Msg.Item.Id

	archiveResp, err := svc.ArchiveBacklogItem(t.Context(), connect.NewRequest(&sessionv1.ArchiveBacklogItemRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)
	require.Equal(t, "archived", archiveResp.Msg.Item.Status, "item should be archived before testing unarchive")

	unarchiveResp, err := svc.UnarchiveBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UnarchiveBacklogItemRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)
	assert.Equal(t, itemID, unarchiveResp.Msg.Item.Id)
	assert.Equal(t, "idea", unarchiveResp.Msg.Item.Status)

	// The unarchived item should reappear in the default (non-terminal) list.
	listDefault, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{}))
	require.NoError(t, err)
	returnedTitles := make([]string, 0, len(listDefault.Msg.Items))
	for _, it := range listDefault.Msg.Items {
		returnedTitles = append(returnedTitles, it.Title)
	}
	assert.Contains(t, returnedTitles, "item to unarchive")
}

func TestUnarchiveBacklogItem_ReturnsNotFoundForMissingItem(t *testing.T) {
	svc := newBacklogService(t)

	_, err := svc.UnarchiveBacklogItem(t.Context(), connect.NewRequest(&sessionv1.UnarchiveBacklogItemRequest{
		ItemId: "00000000-0000-0000-0000-000000000000",
	}))
	require.Error(t, err)
	var connErr *connect.Error
	require.ErrorAs(t, err, &connErr)
	assert.Equal(t, connect.CodeNotFound, connErr.Code())
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

// TestListBacklogItems_DefaultView_ShowsDoneButHidesArchived is the RPC-level
// regression test for the leak fixed alongside the auto-archive sweep: the
// backlog page's default fetch (IncludeTerminal:true, IncludeArchived
// omitted/false) must show "done" items but exclude "archived" ones — before
// the ExcludeDone/ExcludeArchived split, IncludeTerminal was the only knob
// and toggling it on to show done items also silently showed archived items,
// with no way to hide just the latter. IncludeArchived:true must then reveal
// the archived item.
func TestListBacklogItems_DefaultView_ShowsDoneButHidesArchived(t *testing.T) {
	svc := newBacklogService(t)

	create := func(title string) string {
		resp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
			Title:        title,
			SkipPlanning: true,
			AcceptanceCriteria: []*sessionv1.AcCriterion{
				{Index: 0, Text: "it works", Status: "pending"},
			},
		}))
		require.NoError(t, err)
		return resp.Msg.Item.Id
	}
	transition := func(itemID, to string) {
		_, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:         itemID,
			TargetStatus:   to,
			OverrideReason: "test override",
		}))
		require.NoError(t, err, "transition to %s", to)
	}

	doneID := create("done item for default view")
	archivedID := create("archived item for default view")

	transition(doneID, "ready")
	transition(doneID, "in_progress")
	transition(doneID, "review")
	transition(doneID, "done")

	transition(archivedID, "ready")
	transition(archivedID, "in_progress")
	transition(archivedID, "review")
	transition(archivedID, "done")
	transition(archivedID, "archived")

	// The exact request the backlog list page sends by default (Part 2 fix):
	// include done items, but not archived ones.
	defaultView, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{
		IncludeTerminal: true,
		IncludeArchived: false,
	}))
	require.NoError(t, err)
	defaultTitles := make([]string, 0, len(defaultView.Msg.Items))
	for _, it := range defaultView.Msg.Items {
		defaultTitles = append(defaultTitles, it.Title)
	}
	assert.Contains(t, defaultTitles, "done item for default view", "done items must be visible by default")
	assert.NotContains(t, defaultTitles, "archived item for default view", "archived items must be hidden by default")

	// The "Show Archived" toggle re-fetches with IncludeArchived:true.
	withArchived, err := svc.ListBacklogItems(t.Context(), connect.NewRequest(&sessionv1.ListBacklogItemsRequest{
		IncludeTerminal: true,
		IncludeArchived: true,
	}))
	require.NoError(t, err)
	archivedTitles := make([]string, 0, len(withArchived.Msg.Items))
	for _, it := range withArchived.Msg.Items {
		archivedTitles = append(archivedTitles, it.Title)
	}
	assert.Contains(t, archivedTitles, "archived item for default view", "IncludeArchived:true must reveal archived items")
}

// ─── backlogItemToProto ───────────────────────────────────────────────────────

// TestBacklogItemToProto_should_IncludePipelineMode_When_ItemHasNonDefaultMode
// verifies backlogItemToProto maps a non-default PipelineMode onto the proto
// BacklogItem's optional pipeline_mode field (Story 1.4.5).
func TestBacklogItemToProto_should_IncludePipelineMode_When_ItemHasNonDefaultMode(t *testing.T) {
	item := &session.BacklogItemData{
		ID:           "item-1",
		Title:        "item using quick mode",
		PipelineMode: "quick",
	}

	p := backlogItemToProto(item, nil)

	require.NotNil(t, p.PipelineMode)
	assert.Equal(t, "quick", *p.PipelineMode)
}

// TestBacklogItemToProto_should_IncludeAuditTrail_When_StatusEventsAndProgressNotesPresent
// is the regression test for closing the "reviewer/implementer decisions must be visible
// in detail" gap: BacklogStatusEvent.Note (the human-readable reason for a transition,
// e.g. "auto-reopened after FAIL verdict") and the BacklogProgressNote history (the
// implementer's report_progress audit trail) were both durably persisted already but
// never made it onto the wire — backlogItemToProto must now include both.
func TestBacklogItemToProto_should_IncludeAuditTrail_When_StatusEventsAndProgressNotesPresent(t *testing.T) {
	note := "auto-reopened after FAIL verdict"
	item := &session.BacklogItemData{
		ID:    "item-1",
		Title: "item with audit history",
		StatusEvents: []session.BacklogStatusEventData{
			{ID: "ev-1", FromStatus: "review", ToStatus: "in_progress", TriggeredBy: "system", Note: &note},
			{ID: "ev-2", FromStatus: "ready", ToStatus: "in_progress", TriggeredBy: "user"},
		},
		ProgressNotes: []session.ProgressNoteData{
			{ID: "pn-1", CriterionIndex: 0, Note: "implemented the dedent fix", Status: "done"},
		},
	}

	p := backlogItemToProto(item, nil)

	require.Len(t, p.StatusEvents, 2)
	require.NotNil(t, p.StatusEvents[0].Note)
	assert.Equal(t, note, *p.StatusEvents[0].Note)
	assert.Nil(t, p.StatusEvents[1].Note, "an event with no recorded reason must not synthesize one")

	require.Len(t, p.ProgressNotes, 1)
	assert.Equal(t, "pn-1", p.ProgressNotes[0].Id)
	assert.Equal(t, int32(0), p.ProgressNotes[0].CriterionIndex)
	assert.Equal(t, "implemented the dedent fix", p.ProgressNotes[0].Note)
	assert.Equal(t, "done", p.ProgressNotes[0].Status)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

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

// TestApprovePlan_ClearsExistingRejectionReason is the symmetry-fix regression
// test: approving a plan that carries a stale rejection reason (from a prior
// RejectPlan call) must clear that reason, not leave both an approval and a
// rejection reason coexisting. See ADR-001.
func TestApprovePlan_ClearsExistingRejectionReason(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item with a stale rejection",
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	artifactsPath := t.TempDir()
	_, err = storage.UpdateBacklogItem(t.Context(), itemID, session.BacklogItemUpdate{
		PlanArtifactsPath: &artifactsPath,
	}, nil)
	require.NoError(t, err)

	rejectResp, err := svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: itemID,
		Reason: "missing caching plan",
	}))
	require.NoError(t, err)
	require.Equal(t, "missing caching plan", rejectResp.Msg.Item.PlanRejectionReason)
	require.NotNil(t, rejectResp.Msg.Item.PlanRejectedAt)

	approveResp, err := svc.ApprovePlan(t.Context(), connect.NewRequest(&sessionv1.ApprovePlanRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)
	assert.True(t, approveResp.Msg.Item.PlanApproved)
	assert.Empty(t, approveResp.Msg.Item.PlanRejectionReason)
	assert.Nil(t, approveResp.Msg.Item.PlanRejectedAt, "plan_rejected_at must be cleared symmetrically with plan_rejection_reason on approval")
}

// ─── RejectPlan ───────────────────────────────────────────────────────────────

// TestRejectPlan_HappyPath_SetsReasonAndTimestamp: a non-empty reason persists
// plan_rejection_reason/plan_rejected_at and clears plan_approved.
func TestRejectPlan_HappyPath_SetsReasonAndTimestamp(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item with plan",
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	artifactsPath := t.TempDir()
	_, err = storage.UpdateBacklogItem(t.Context(), itemID, session.BacklogItemUpdate{
		PlanArtifactsPath: &artifactsPath,
	}, nil)
	require.NoError(t, err)

	rejectResp, err := svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: itemID,
		Reason: "missing caching plan",
	}))
	require.NoError(t, err)
	assert.Equal(t, "missing caching plan", rejectResp.Msg.Item.PlanRejectionReason)
	assert.NotNil(t, rejectResp.Msg.Item.PlanRejectedAt)
	assert.False(t, rejectResp.Msg.Item.PlanApproved)
}

// TestRejectPlan_EmptyReason_ReturnsInvalidArgument: an empty or whitespace-only
// reason must be rejected server-side, not just in the UI (AC4).
func TestRejectPlan_EmptyReason_ReturnsInvalidArgument(t *testing.T) {
	svc := newBacklogService(t)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item with plan",
	}))
	require.NoError(t, err)

	_, err = svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: createResp.Msg.Item.Id,
		Reason: "",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestRejectPlan_WhitespaceOnlyReason_ReturnsInvalidArgument: whitespace-only
// text is trimmed and treated identically to an empty reason.
func TestRejectPlan_WhitespaceOnlyReason_ReturnsInvalidArgument(t *testing.T) {
	svc := newBacklogService(t)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item with plan",
	}))
	require.NoError(t, err)

	_, err = svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: createResp.Msg.Item.Id,
		Reason: "   \n\t  ",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestRejectPlan_ReasonExceedsMaxLength_ReturnsInvalidArgument verifies that a
// reason longer than maxRejectReasonLength is rejected with InvalidArgument,
// mirroring the empty/whitespace-only guards above.
func TestRejectPlan_ReasonExceedsMaxLength_ReturnsInvalidArgument(t *testing.T) {
	svc := newBacklogService(t)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item with plan",
	}))
	require.NoError(t, err)

	tooLong := strings.Repeat("a", maxRejectReasonLength+1)
	_, err = svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: createResp.Msg.Item.Id,
		Reason: tooLong,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestRejectPlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition mirrors
// ApprovePlan's equivalent guard: rejecting a plan that was never generated
// makes no sense.
func TestRejectPlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition(t *testing.T) {
	svc := newBacklogService(t)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title: "item without plan",
	}))
	require.NoError(t, err)

	_, err = svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: createResp.Msg.Item.Id,
		Reason: "no plan yet, but a reason anyway",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

// TestRejectPlan_ClearsExistingApproval is the symmetry-fix regression test:
// approving a plan then rejecting it must clear plan_approved (round-tripped
// through a fresh GetBacklogItem read), and the spawn-gate precondition check
// must still block a spawn afterward — the concrete case the symmetry fix
// prevents, not just a field-value check.
func TestRejectPlan_ClearsExistingApproval(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "item to approve then reject",
		RepoPath: t.TempDir(),
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	artifactsPath := t.TempDir()
	_, err = storage.UpdateBacklogItem(t.Context(), itemID, session.BacklogItemUpdate{
		PlanArtifactsPath: &artifactsPath,
	}, nil)
	require.NoError(t, err)

	approveResp, err := svc.ApprovePlan(t.Context(), connect.NewRequest(&sessionv1.ApprovePlanRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)
	require.True(t, approveResp.Msg.Item.PlanApproved)

	rejectResp, err := svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: itemID,
		Reason: "actually, reconsider the approach",
	}))
	require.NoError(t, err)
	assert.False(t, rejectResp.Msg.Item.PlanApproved, "PlanApproved must be false in the RejectPlanResponse itself")

	// Read-after-write round trip through the real repository/ent layer.
	fetched, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.False(t, fetched.PlanApproved, "PlanApproved must be false on a fresh GetBacklogItem read")

	// Move the item to ready so the spawn-gate precondition check below is the
	// planning gate itself, not the earlier status gate.
	_, err = storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusReady, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
		ItemId: itemID,
	}))
	require.Error(t, err, "spawn must still be blocked after reject-following-approve")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "approve the plan")
}

// TestTransitionBacklogItemStatus_SendBackToIdea_ClearsRejectionReason extends
// the existing backward-transition reset block (which already clears
// plan_approved/plan_artifacts_path) to also clear plan_rejection_reason —
// sending a rejected item back to idea/refining must not leave a stale
// rejection reason attached to what is now effectively a new planning round.
// See ADR-001.
func TestTransitionBacklogItemStatus_SendBackToIdea_ClearsRejectionReason(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "rejected item sent back to idea",
		Status:   string(session.BacklogStatusReady),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	artifactsPath := t.TempDir()
	_, err = storage.UpdateBacklogItem(t.Context(), item.ID, session.BacklogItemUpdate{
		PlanArtifactsPath: &artifactsPath,
	}, nil)
	require.NoError(t, err)

	rejectResp, err := svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: item.ID,
		Reason: "needs a different approach entirely",
	}))
	require.NoError(t, err)
	require.Equal(t, "needs a different approach entirely", rejectResp.Msg.Item.PlanRejectionReason)
	require.NotNil(t, rejectResp.Msg.Item.PlanRejectedAt)

	transResp, err := svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       item.ID,
		TargetStatus: "idea",
	}))
	require.NoError(t, err)
	assert.Empty(t, transResp.Msg.Item.PlanRejectionReason)
	assert.Nil(t, transResp.Msg.Item.PlanRejectedAt, "plan_rejected_at must be cleared symmetrically with plan_rejection_reason on backward transition")
	assert.False(t, transResp.Msg.Item.PlanApproved)

	fetched, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Empty(t, fetched.PlanRejectionReason, "plan_rejection_reason must be cleared on a fresh read too")
	assert.Nil(t, fetched.PlanRejectedAt, "plan_rejected_at must be cleared on a fresh read too")
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
	// Force an isolated worktree base dir — without this, config.GetConfigDirForDir's
	// IsTestMode() branch scopes it by OS PID only (shared by every test in this binary),
	// so a stale worktree/branch left by another server/services test can be "reused" by
	// findExistingWorktreeForBranch, silently failing the async triage goroutine's git
	// status check and leaving the item stuck below (never reaching "ready"). Same fix as
	// TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession, below.
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

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

// TestSpawnSessionFromItem_Reopen_ReusesBranch is the regression test for the bug
// where every reopen/rework spawn minted a brand-new "backlog/<slug>-rN" branch off
// current HEAD instead of resuming the item's existing branch — see the -rN suffix
// in buildRevisionTitle leaking into the worktree/branch slug. The fix derives the
// worktree slug from baseTitle (stable across reopens), not title (which varies).
// This test drives two real spawns through the real git worktree path (not mocked)
// and asserts both land on the identical branch.
func TestSpawnSessionFromItem_Reopen_ReusesBranch(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "reuse branch item")

	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)
	firstBranch := currentBranch(t, creator.calls[0].path)

	// End the work session so the reopen isn't blocked by the active-session guard.
	sessions, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), sessions[0].ID, time.Now()))

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 2)
	secondBranch := currentBranch(t, creator.calls[1].path)

	assert.Equal(t, firstBranch, secondBranch, "reopen must reuse the same branch, not mint a new -rN branch")
	assert.NotContains(t, secondBranch, "-r2", "branch name must not pick up the session title's revision suffix")
}

// TestSpawnSessionFromItem_Reopen_ReusesWorktreeInPlace is a regression test for a
// real bug: reopen used to force-remove and recreate the worktree at the reused
// path (git.GitWorktree.setupFromExistingBranch always ran `worktree remove -f`
// before `worktree add`), and cleanupItemWorktrees then ran a second time against
// that same identical path via priorSessions — either step alone could wipe the
// worktree the brand-new session had just started using, leaving a still
// in_progress/review item with no worktree on disk at all (empty diffs, degraded
// re-review). Both the worktree path and an uncommitted file written before reopen
// must survive a reopen unchanged.
func TestSpawnSessionFromItem_Reopen_ReusesWorktreeInPlace(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "reuse worktree item")

	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)
	firstPath := creator.calls[0].path

	uncommitted := filepath.Join(firstPath, "uncommitted.txt")
	require.NoError(t, os.WriteFile(uncommitted, []byte("not yet committed\n"), 0o644))

	// End the work session so the reopen isn't blocked by the active-session guard.
	sessions, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), sessions[0].ID, time.Now()))

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 2)
	secondPath := creator.calls[1].path

	assert.Equal(t, firstPath, secondPath, "reopen must reuse the same worktree path, not mint a new one")
	assert.FileExists(t, uncommitted, "reopen must not wipe the existing worktree — the file written before reopen must survive")
}

// TestSpawnSessionFromItem_Reopen_ArchivesSupersededWorkSession is the regression
// test for the bug where backlog work sessions accumulated forever because rework
// respawns never archived the prior round's session — see
// docs/tasks/backlog-feature-improvement.md and the live-data finding of up to 13
// unarchived work sessions piled up on a single still-open item. A reopen spawn must
// archive every prior work-role session it supersedes (but must not touch the
// brand-new session it just created).
func TestSpawnSessionFromItem_Reopen_ArchivesSupersededWorkSession(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}}
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "archive superseded item")

	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 1)
	firstUUID := creator.calls[0].inst.UUID

	// End the work session so the reopen isn't blocked by the active-session guard.
	sessions, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), sessions[0].ID, time.Now()))

	require.Empty(t, stopper.archivedUUIDs, "nothing should be archived before the reopen spawn")

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.Len(t, creator.calls, 2)
	secondUUID := creator.calls[1].inst.UUID

	assert.Equal(t, []string{firstUUID}, stopper.archivedUUIDs,
		"reopen must archive exactly the superseded first-round work session")
	assert.NotContains(t, stopper.archivedUUIDs, secondUUID,
		"reopen must not archive the brand-new session it just created")
	// Regression guard for the 2026-07-29 OOM: archiving alone only hides the
	// session from the UI — its tmux pane (and the claude process/MCP subprocess
	// fleet behind it) must also be killed, or superseded rework sessions pile up
	// as live, resource-consuming zombies indefinitely.
	assert.Contains(t, stopper.killedPaneUUIDs, firstUUID,
		"reopen must also kill the superseded first-round session's tmux pane, not just archive it")
	assert.NotContains(t, stopper.killedPaneUUIDs, secondUUID,
		"reopen must not kill the tmux pane of the brand-new session it just created")
}

// currentBranch returns the checked-out branch name at path via the real git CLI.
func currentBranch(t *testing.T, path string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD") //nolint:norawexec // test helper, blocking CombinedOutput, no zombie risk
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git rev-parse failed: %s", out)
	return strings.TrimSpace(string(out))
}

// createReadyItemForSpawn creates a SkipPlanning backlog item and advances it to
// "ready", returning its ID. Reduces boilerplate for WIP-limit and spawn tests
// that don't care about the triage/planning flow.
func createReadyItemForSpawn(t *testing.T, svc *BacklogService, repoPath, title string) string {
	t.Helper()
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    title,
		RepoPath: repoPath,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)
	return itemID
}

// createReadyItemWithPriority is createReadyItemForSpawn plus an explicit priority
// (1 = P1/highest ... 5 = P5/lowest), for tests asserting dequeue ordering.
func createReadyItemWithPriority(t *testing.T, svc *BacklogService, repoPath, title string, priority int32) string {
	t.Helper()
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    title,
		RepoPath: repoPath,
		Priority: priority,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: "ready",
	}))
	require.NoError(t, err)
	return itemID
}

// TestSpawnSessionFromItem_RecordsTriggeredByFromAutonomousFlag verifies that the
// in_progress transition SpawnSessionFromItem fires on a fresh spawn records
// TriggeredBy="user" for a manual (non-autonomous) spawn and TriggeredBy="system"
// for an autonomous spawn — SpawnSessionFromItem is the RPC the frontend calls
// directly for the dominant "click Spawn Session" path, so a hardcoded value here
// would defeat the point of the status-audit-trail fix.
func TestSpawnSessionFromItem_RecordsTriggeredByFromAutonomousFlag(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	t.Run("manual spawn records user", func(t *testing.T) {
		itemID := createReadyItemForSpawn(t, svc, repoPath, "manual spawn triggeredBy")
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
			ItemId: itemID,
		}))
		require.NoError(t, err)

		getResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		require.NoError(t, err)
		require.NotEmpty(t, getResp.Msg.Item.StatusEvents)
		last := getResp.Msg.Item.StatusEvents[len(getResp.Msg.Item.StatusEvents)-1]
		require.Equal(t, "in_progress", last.ToStatus)
		require.Equal(t, "user", last.TriggeredBy)
	})

	t.Run("autonomous spawn records system", func(t *testing.T) {
		itemID := createReadyItemForSpawn(t, svc, repoPath, "autonomous spawn triggeredBy")
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{
			ItemId:     itemID,
			Autonomous: true,
		}))
		require.NoError(t, err)

		getResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		require.NoError(t, err)
		require.NotEmpty(t, getResp.Msg.Item.StatusEvents)
		last := getResp.Msg.Item.StatusEvents[len(getResp.Msg.Item.StatusEvents)-1]
		require.Equal(t, "in_progress", last.ToStatus)
		require.Equal(t, "system", last.TriggeredBy)
	})
}

// testWIPCap mirrors config.Config.MaxConcurrentBacklogWorkItemsOrDefault's
// default (cfg=nil in these tests, so the default applies).
const testWIPCap = 2

// TestBacklogService_Admit_AllowsUnderCapRejectsAtCap verifies Admit (webhook-triggers
// Task 1.3.1b) implements the same WIP cap SpawnSessionFromItem's own gate enforces —
// this is the method server/workflows.Scheduler consults before every trigger-fired
// CreateSession call (Epic 1.3).
func TestBacklogService_Admit_AllowsUnderCapRejectsAtCap(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	admitted, err := svc.Admit(t.Context())
	require.NoError(t, err)
	assert.True(t, admitted, "Admit should allow when no work sessions are live")

	// Fill the WIP cap with successful spawns.
	for i := 0; i < testWIPCap; i++ {
		id := createReadyItemForSpawn(t, svc, repoPath, fmt.Sprintf("admit item %d", i))
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: id}))
		require.NoError(t, err)
	}

	admitted, err = svc.Admit(t.Context())
	require.NoError(t, err)
	assert.False(t, admitted, "Admit should reject once the WIP cap is reached")
}

// TestSpawnSessionFromItem_WIPLimit_QueuesInsteadOfRejecting verifies that a
// fresh spawn is queued (not rejected) once testWIPCap items are already
// in_progress: the response carries Queued=true with no error, and the item
// transitions to "queued" with QueuedAt set.
func TestSpawnSessionFromItem_WIPLimit_QueuesInsteadOfRejecting(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Fill the WIP cap with successful spawns.
	for i := 0; i < testWIPCap; i++ {
		id := createReadyItemForSpawn(t, svc, repoPath, fmt.Sprintf("wip item %d", i))
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: id}))
		require.NoError(t, err, "spawn %d must succeed while under the cap", i)
	}
	require.Len(t, creator.calls, testWIPCap)

	// One more fresh spawn, at cap, must be queued rather than rejected.
	overCapID := createReadyItemForSpawn(t, svc, repoPath, "over cap item")
	resp, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: overCapID}))
	require.NoError(t, err, "spawn at the WIP cap must not return an error")
	assert.True(t, resp.Msg.Queued, "response must indicate the item was queued")
	assert.Len(t, creator.calls, testWIPCap, "no session should have been spawned for the queued item")

	// The queued item must reflect the new status and have queued_at set.
	getResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: overCapID}))
	require.NoError(t, err)
	assert.Equal(t, "queued", getResp.Msg.Item.Status)
}

// TestSpawnSessionFromItem_WIPLimit_AllowsReopenAtCap verifies that a reopen
// (revision) spawn for an item that's already in_progress is NOT blocked by the
// WIP limit, since it doesn't add a new concurrent item — it's already counted.
func TestSpawnSessionFromItem_WIPLimit_AllowsReopenAtCap(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Fill the WIP cap.
	var reopenTargetID string
	for i := 0; i < testWIPCap; i++ {
		id := createReadyItemForSpawn(t, svc, repoPath, fmt.Sprintf("wip item %d", i))
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: id}))
		require.NoError(t, err)
		reopenTargetID = id // reopen the last one filled
	}

	// End the reopen target's work session — the pre-existing "duplicate active work
	// session" guard would otherwise block the reopen for an unrelated reason.
	sessions, err := storage.ListItemSessions(t.Context(), reopenTargetID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), sessions[0].ID, time.Now()))

	// Reopen (isReopen=true, since reopenTargetID is already in_progress) must
	// succeed even though the cap is reached.
	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: reopenTargetID}))
	require.NoError(t, err, "reopen spawn must not be blocked by the WIP limit")
	assert.Len(t, creator.calls, testWIPCap+1)
}

// TestSpawnSessionFromItem_should_RejectNotQueue_When_UnapprovedPlanHitsWIPCap is
// the regression test for PR #199 review F2/F3: previously the WIP-cap gate ran
// BEFORE the planning-approval gate, so an item that reached "ready" without an
// approved plan (idea->ready only requires non-empty acceptance criteria, not
// planning approval) could be queued instead of rejected once the cap was hit.
// DequeueNextQueuedItems later claims queued items and spawns a real work
// session with no planning check of its own — so queueing such an item let it
// bypass the plan-required invariant entirely. The planning gate must now run
// first: an unapproved-plan item must be rejected outright, even at the cap,
// and never transition to "queued".
func TestSpawnSessionFromItem_should_RejectNotQueue_When_UnapprovedPlanHitsWIPCap(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Fill the WIP cap with SkipPlanning items so the cap is genuinely reached.
	for i := 0; i < testWIPCap; i++ {
		id := createReadyItemForSpawn(t, svc, repoPath, fmt.Sprintf("wip item %d", i))
		_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: id}))
		require.NoError(t, err)
	}
	require.Len(t, creator.calls, testWIPCap)

	// Create an item that reached "ready" WITHOUT SkipPlanning or an approved
	// plan — idea->ready only requires non-empty acceptance criteria.
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:      "unapproved plan item",
		RepoPath:   repoPath,
		SkipTriage: true,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId: itemID, TargetStatus: "ready",
	}))
	require.NoError(t, err)

	// Sanity check: the item genuinely has no approved plan.
	loaded, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	require.False(t, loaded.SkipPlanning)
	require.False(t, loaded.PlanApproved)

	// Spawning at the WIP cap must be rejected by the planning gate — never queued.
	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.Error(t, err, "an unapproved-plan item must be rejected, not queued, even when the WIP cap is hit")
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "approve the plan")

	getResp, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, "ready", getResp.Msg.Item.Status, "item must remain ready, not be queued, when the plan is unapproved")
	assert.Len(t, creator.calls, testWIPCap, "no additional session should have been spawned")
}

// TestSpawnSessionFromItem_WIPLimit_CountsLiveReviewSessions is the regression test for
// the "WIP limit now undercounts live sessions" gap (docs/tasks/backlog-feature-improvement.md):
// AutoReopenAfterFailedReview intentionally leaves a work session running (polling for a
// verdict) after the item's status flips back to "review". A WIP count that only looks at
// "in_progress" status items misses this live session entirely, letting an operator exceed
// the cap the 2026-07-12 OOM incident motivated. countLiveBacklogWorkSessions must count it.
func TestSpawnSessionFromItem_WIPLimit_CountsLiveReviewSessions(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	// Fill the cap with one item still genuinely in_progress...
	inProgressID := createReadyItemForSpawn(t, svc, repoPath, "still in progress")
	_, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: inProgressID}))
	require.NoError(t, err)

	// ...and one item whose work session is still alive but whose status has moved to
	// "review" (the AutoReopenAfterFailedReview live-session-reuse case) — this must
	// still count toward the cap even though it is no longer "in_progress".
	reviewID := createReadyItemForSpawn(t, svc, repoPath, "alive but in review")
	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: reviewID}))
	require.NoError(t, err)
	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       reviewID,
		TargetStatus: string(session.BacklogStatusReview),
	}))
	require.NoError(t, err)
	require.Equal(t, testWIPCap, 2, "test assumes the default cap of 2; update the fixture if this changes")

	// A fresh spawn for a third item must now be queued, not rejected: two agents (one
	// in_progress, one live-in-review) are already running, at the cap.
	overCapID := createReadyItemForSpawn(t, svc, repoPath, "over cap item")
	resp, err := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: overCapID}))
	require.NoError(t, err, "spawn at the WIP cap must not return an error — the review-status session is still live")
	assert.True(t, resp.Msg.Queued, "response must indicate the item was queued")
}

// TestSpawnSessionFromItem_TombstonesDeadWorkSession_AllowsRespawn is the regression
// test for a live production bug: a work session that never reached its normal
// completion path (crash, kill, server restart) left an open (EndedAt nil) work-role
// ItemSession behind. Every subsequent spawn attempt (e.g. AutoReopenForPRFix retrying
// every ~60s) was rejected by hasActiveWorkSession's guard forever, since nothing ever
// checked whether that session was actually still alive — the item bounced
// in_progress<->pr_pending indefinitely with zero progress. tombstoneOrphanWorkSessions
// must clear a confirmed-dead session so a fresh spawn succeeds.
func TestSpawnSessionFromItem_TombstonesDeadWorkSession_AllowsRespawn(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}} // nothing is live
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "item with dead work session")

	// Simulate a work session that died without ever calling UpdateItemSessionEnded —
	// same shape as the live bug (created hours ago, EndedAt still nil).
	_, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: "dead-work-session-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	// TransitionBacklogItemStatus above already moved the item to "ready"; put it back
	// to "in_progress" the way a real reopen scenario would leave it (dead session, but
	// item still shows as actively worked).
	_, err = storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusInProgress, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	// Before the fix, this would fail with CodeAlreadyExists forever.
	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err, "a dead work session must not permanently block respawning")
	assert.Len(t, creator.calls, 1)

	sessions, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	var deadEnded, newOpen bool
	for _, is := range sessions {
		if is.SessionUUID == "dead-work-session-uuid" {
			deadEnded = is.EndedAt != nil
		} else if is.Role == string(session.SessionRoleWork) {
			newOpen = is.EndedAt == nil
		}
	}
	assert.True(t, deadEnded, "the dead session must be tombstoned (EndedAt set)")
	assert.True(t, newOpen, "the newly-spawned work session must be open")
}

// TestRemediateStaleWorkSession_should_killTombstoneAndRespawn_When_ActiveWorkSessionIsStale
// is the regression test for the stale_work auto-remediation gap: a work
// session with no EndedAt whose underlying tmux session/pane are genuinely
// still alive (mockSessionStopper.liveUUIDs marks it live, mirroring
// Instance.TmuxAlive()==true / PaneProcessDead()==false for an agent that
// finished and is idle at an interactive prompt) must still be killed,
// tombstoned, and replaced with a fresh work session — not left stranded
// forever just because it isn't a zombie. See
// session.StaleWorkRemediator/BacklogLifecycleListener.
// remediateStaleWorkWithBackoffGate (session/backlog_lifecycle.go) for the
// backoff-gated caller this implements.
func TestRemediateStaleWorkSession_should_killTombstoneAndRespawn_When_ActiveWorkSessionIsStale(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"stale-work-session-uuid": true}}
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "item with stale-but-alive work session")

	_, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: "stale-work-session-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	_, err = storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusInProgress, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	remediateErr := svc.RemediateStaleWorkSession(t.Context(), itemID)
	require.NoError(t, remediateErr)

	assert.Contains(t, stopper.killedPaneUUIDs, "stale-work-session-uuid", "the stale tmux pane must be killed even though it was reported live")
	require.Len(t, creator.calls, 1, "a fresh work session must be spawned")

	sessions, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	var staleEnded, newOpen bool
	for _, is := range sessions {
		if is.SessionUUID == "stale-work-session-uuid" {
			staleEnded = is.EndedAt != nil
		} else if is.Role == string(session.SessionRoleWork) {
			newOpen = is.EndedAt == nil
		}
	}
	assert.True(t, staleEnded, "the stale session must be tombstoned (EndedAt set)")
	assert.True(t, newOpen, "the newly-spawned work session must be open")
}

// TestRemediateStaleWorkSession_should_EndSessionBeforeKillingPane_When_ActiveWorkSessionIsStale
// is a regression test for BUG-064: killing the stale session's tmux pane
// (KillTmuxPaneOnly -> Instance.KillSession) fires the Instance's exit event
// asynchronously, which session.BacklogLifecycleListener.onSessionExited
// handles in its own goroutine — and, before this fix, unconditionally
// transitioned the (still in_progress) item straight to "review" the moment
// it observed the work session's EndedAt set, racing ahead of this
// function's own AutoRespawnAutonomousWork call and silently discarding the
// intended fresh work-session respawn (live repro: item 2d7fac56,
// 2026-08-06). Ending the ItemSession row before killing the pane turns that
// race into a guaranteed happens-before: onSessionExited's own "already
// ended by another path" guard (session/backlog_lifecycle.go) can only work
// if EndedAt is already non-nil by the time it reads the row, which requires
// this ordering. Verifies the ordering directly via a hook on
// KillTmuxPaneOnly that inspects storage at the exact moment the pane would
// be killed in production.
func TestRemediateStaleWorkSession_should_EndSessionBeforeKillingPane_When_ActiveWorkSessionIsStale(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	itemID := createReadyItemForSpawn(t, svc, repoPath, "item with stale-but-alive work session")

	createdIS, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: "stale-work-session-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	_, err = storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusInProgress, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	var (
		firstKillObserved     bool
		endedAtWhenKillCalled *time.Time
	)
	stopper := &mockSessionStopper{
		liveUUIDs: map[string]bool{"stale-work-session-uuid": true},
		onKillTmuxPaneOnly: func(uuid string) {
			if uuid != "stale-work-session-uuid" || firstKillObserved {
				// Only the first kill matters here: a real Instance.KillSession
				// no-ops (HasSession() check) on an already-dead pane and never
				// re-fires the exit event, but killEndedWorkSessionPanes
				// (called later, right before AutoRespawnAutonomousWork spawns
				// the replacement) legitimately re-invokes KillTmuxPaneOnly on
				// the now-ended session too — that later, redundant call is not
				// the one racing onSessionExited and must not overwrite what
				// this test is actually checking.
				return
			}
			firstKillObserved = true
			is, getErr := storage.GetItemSession(t.Context(), createdIS.ID)
			require.NoError(t, getErr)
			endedAtWhenKillCalled = is.EndedAt
		},
	}
	svc.SetSessionStopper(stopper)

	remediateErr := svc.RemediateStaleWorkSession(t.Context(), itemID)
	require.NoError(t, remediateErr)

	require.Contains(t, stopper.killedPaneUUIDs, "stale-work-session-uuid")
	require.NotNil(t, endedAtWhenKillCalled, "the ItemSession row must already have EndedAt set by the time KillTmuxPaneOnly is called — killing the pane fires onSessionExited asynchronously, which needs to observe EndedAt already non-nil to correctly skip its own status transition")
}

// TestRemediateStaleWorkSession_should_noop_When_ItemNoLongerInProgress verifies
// that if the item already moved off in_progress (a human acted manually, or
// another reconciler beat this call to it) by the time the gated remediation
// goroutine actually runs, RemediateStaleWorkSession is a no-op — no kill, no
// spawn.
func TestRemediateStaleWorkSession_should_noop_When_ItemNoLongerInProgress(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"stale-work-session-uuid": true}}
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "item that already moved on")

	_, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: "stale-work-session-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	// Left in "ready" (createReadyItemForSpawn's terminal status), not
	// in_progress — simulates the item having already moved on.

	remediateErr := svc.RemediateStaleWorkSession(t.Context(), itemID)
	require.NoError(t, remediateErr)

	assert.Empty(t, stopper.killedPaneUUIDs, "must not kill anything once the item is no longer in_progress")
	assert.Empty(t, creator.calls, "must not spawn a new session once the item is no longer in_progress")
}

// TestSpawnSessionFromItem_LiveWorkSession_StillBlocksSpawn verifies
// tombstoneOrphanWorkSessions does NOT reap a work session that IsSessionLive
// confirms is genuinely still running — the fix must not weaken the duplicate-spawn
// guard for the common case.
func TestSpawnSessionFromItem_LiveWorkSession_StillBlocksSpawn(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{"live-work-session-uuid": true}}
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "item with live work session")
	_, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: "live-work-session-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	_, err = storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusInProgress, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.Error(t, err, "a genuinely live work session must still block a duplicate spawn")
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))
	assert.Empty(t, creator.calls)
}

// TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated is the
// regression test for a live-observed race (2026-07-19, item d3227302): two
// literal overlapping work-role ItemSessions were created for the same backlog
// item because SpawnSessionFromItem's read (ListItemSessions) -> check
// (hasActiveWorkSession) -> write (CreateItemSession) sequence held no lock, so
// two concurrent callers (e.g. the autonomous-driver respawn path racing a
// periodic reconciliation sweep, or a rapid double-click) could both observe
// "no active work session" before either had written its row. Run with -race:
// BacklogService.spawnInFlight's LoadOrStore/Delete guard must serialize the
// whole function body per item ID so only one of N concurrent
// SpawnSessionFromItem calls for the SAME item succeeds — the rest must fail
// fast with CodeAlreadyExists instead of each creating their own work session.
func TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "concurrent spawn race")

	const concurrency = 8
	var (
		wg       sync.WaitGroup
		resultMu sync.Mutex
		errs     []error
	)
	start := make(chan struct{})
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines together to maximize the race window
			_, spawnErr := svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
			resultMu.Lock()
			errs = append(errs, spawnErr)
			resultMu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	var successes, conflicts int
	for _, spawnErr := range errs {
		if spawnErr == nil {
			successes++
			continue
		}
		require.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(spawnErr), "unexpected error: %v", spawnErr)
		conflicts++
	}
	assert.Equal(t, 1, successes, "exactly one concurrent spawn attempt must succeed")
	assert.Equal(t, concurrency-1, conflicts, "every other concurrent attempt must fail fast with CodeAlreadyExists")
	assert.Len(t, creator.calls, 1, "only one real session must have been spawned")

	sessions, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	openWork := 0
	for _, is := range sessions {
		if is.Role == session.SessionRoleWork && is.EndedAt == nil {
			openWork++
		}
	}
	assert.Equal(t, 1, openWork, "exactly one open work-role ItemSession must exist for the item — no duplicates")
}

// TestSpawnSessionFromItem_ReopenKillsEndedWorkSessionPane is the regression test for
// a real complaint: each rework round gets its own "-rN" title (buildRevisionTitle),
// but nothing ever closed a finished round's tmux pane — it sat around indefinitely as
// an idle "[exited]" pane, accumulating with every rework cycle. A fresh reopen spawn
// must close the previous round's pane via KillTmuxPaneOnly (not StopSessionByUUID,
// which would also delete the worktree the new round is about to reuse).
func TestSpawnSessionFromItem_ReopenKillsEndedWorkSessionPane(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	stopper := &mockSessionStopper{liveUUIDs: map[string]bool{}}
	svc.SetSessionStopper(stopper)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	itemID := createReadyItemForSpawn(t, svc, repoPath, "item with a finished rework round")

	// Simulate a normally-completed prior work session (round 1): EndedAt set, the way
	// handleReviewSessionExited leaves it once a review verdict is processed.
	endedAt := time.Now().Add(-time.Hour)
	priorSession, err := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: "round-1-uuid",
		SessionRole: session.SessionRoleWork,
	})
	require.NoError(t, err)
	require.NoError(t, storage.UpdateItemSessionEnded(t.Context(), priorSession.ID, endedAt))
	_, err = storage.TransitionBacklogItemStatus(t.Context(), itemID, session.BacklogStatusInProgress, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Len(t, creator.calls, 1, "round 2 must spawn since round 1 already ended")
	assert.Contains(t, stopper.killedPaneUUIDs, "round-1-uuid", "round 1's tmux pane must be closed once round 2 spawns")
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

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

	// A live Instance at its own dedicated directory (distinct from repoPath —
	// AttachSessionToItem rejects a session whose path IS the item's shared repo
	// checkout, see the isolation guard's regression test below), discoverable by
	// AttachSessionToItem's storage.LoadInstances() lookup.
	sessionDir := t.TempDir()
	const attachUUID = "attach-session-uuid"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title: "attach-target",
		UUID:  attachUUID,
		Path:  sessionDir,
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

	data, err := os.ReadFile(filepath.Join(sessionDir, ".backlog-context.md"))
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

// TestAttachSessionToItem_RejectsWhenSessionPathIsItemSharedRepoCheckout is the
// regression test for the 2026-07-21 finding: unlike SpawnSessionFromItem
// (which always tries a dedicated worktree first and only falls back to a
// *fresh, per-session* directory — never the shared checkout), attaching an
// arbitrary pre-existing session had no isolation guarantee at all. Confirmed
// live on item 635a373d (PR #206): its attached session's effective root dir
// was literally item.RepoPath — the shared main checkout used by unrelated
// work — so a re-review graded whatever unrelated commits happened to land in
// that directory between rounds, producing a wrong verdict rather than a
// stuck one, and the item's stale-session auto-remediation couldn't recover
// it either since the session was genuinely alive.
func TestAttachSessionToItem_RejectsWhenSessionPathIsItemSharedRepoCheckout(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoPath := t.TempDir()
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:      "shared-checkout attach attempt",
		RepoPath:   repoPath,
		SkipTriage: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	// The hazard: a session whose Path IS the item's own RepoPath, not a
	// dedicated worktree or directory.
	const attachUUID = "shared-checkout-session-uuid"
	require.NoError(t, storage.AddInstance(&session.Instance{
		Title:     "shared-checkout-session",
		UUID:      attachUUID,
		Path:      repoPath,
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))

	_, err = svc.AttachSessionToItem(t.Context(), connect.NewRequest(&sessionv1.AttachSessionToItemRequest{
		ItemId:      itemID,
		SessionUuid: attachUUID,
	}))
	require.Error(t, err, "attaching a session whose path is the item's shared repo checkout must be rejected")
	assert.Contains(t, err.Error(), "shared repo checkout")

	fetched, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusIdea), fetched.Status,
		"a rejected attach must not transition the item or leave a dangling ItemSession")

	sessions, err := storage.ListItemSessions(t.Context(), itemID)
	require.NoError(t, err)
	assert.Empty(t, sessions, "a rejected attach must not create an ItemSession row")
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

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

// TestTriggerReReview_HeadlessPassAutoTransitionsToDone verifies that a PASS
// verdict from the headless re-review path moves the item straight to "done"
// without requiring a manual "Approve — Mark Done" click — matching the
// behavior of the tmux-driven submit_review_verdict MCP tool and
// SubmitManualReview, which already auto-transition on PASS.
func TestTriggerReReview_HeadlessPassAutoTransitionsToDone(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	// No work session/diff exists in this fixture, so TriggerReReview's review call
	// takes the "codebase-read" (empty-diff) path and DegradeIfUnverified requires
	// tool_reads evidence pointing at a real, existing file under RepoPath, or it
	// downgrades PASS to UNVERIFIABLE regardless of what the fake pool returns.
	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(repoDir+"/README.md", []byte("hello\n"), 0o644))
	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"looks good","verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"verified"}],"tool_reads":["README.md"]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "test item",
		RepoPath: repoDir,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		// SkipTriage prevents CreateBacklogItem's auto-triage goroutine from racing
		// this test's own explicit idea->ready transition below.
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	for _, status := range []string{
		string(session.BacklogStatusReady),
		string(session.BacklogStatusInProgress),
		string(session.BacklogStatusReview),
	} {
		_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:       itemID,
			TargetStatus: status,
		}))
		require.NoError(t, err)
	}

	_, err = svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)

	updated, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusDone), updated.Msg.Item.Status,
		"PASS verdict from headless re-review should auto-transition item to done")
}

// TestTriggerReReview_HeadlessPassWithUnshippedCode_StaysInReviewForShipPR is the
// regression test for the 2026-07-18 finding (docs/tasks/backlog-feature-improvement.md):
// TriggerReReview's headless-PASS branch transitions review->done via the storage
// layer directly, which bypasses the TransitionBacklogItemStatus RPC handler's
// ErrPRRequired guard entirely. Before this fix, a PASS verdict here could mark an
// item "done" even though its work session had committed code that was never
// pushed or turned into a PR — silently losing the ship step. The item must now
// stay in review so the "Ship PR" action can recover it.
func TestTriggerReReview_HeadlessPassWithUnshippedCode_StaysInReviewForShipPR(t *testing.T) {
	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	_, repoDir := setupPRFixSyncRepo(t)
	require.NoError(t, os.WriteFile(repoDir+"/README.md", []byte("hello\n"), 0o644))
	runGitTestCmd(t, repoDir, "add", "README.md")
	runGitTestCmd(t, repoDir, "commit", "-m", "add README")
	runGitTestCmd(t, repoDir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(repoDir+"/feature.txt", []byte("unshipped work\n"), 0o644))
	runGitTestCmd(t, repoDir, "add", "feature.txt")
	runGitTestCmd(t, repoDir, "commit", "-m", "wip")
	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"looks good","verdicts":[{"criterion_index":0,"outcome":"PASS","evidence":"verified"}],"tool_reads":["README.md"]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "test item",
		RepoPath: repoDir,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		// SkipTriage prevents CreateBacklogItem's auto-triage goroutine from racing
		// this test's own explicit idea->ready transition below.
		SkipTriage:   true,
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	for _, status := range []string{
		string(session.BacklogStatusReady),
		string(session.BacklogStatusInProgress),
	} {
		_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
			ItemId:       itemID,
			TargetStatus: status,
		}))
		require.NoError(t, err)
	}

	// Seed a work session with committed-but-unpushed code — the scenario the
	// ErrPRRequired guard (and this replicated check) exists for. repoDir is
	// checked out on "feature" right now (the unshipped commit) — use it as
	// the work session's own worktree path so isCodeShippedToMain resolves
	// the commit from its live HEAD, not a stale LastCommitSha field.
	item, err := storage.GetBacklogItem(t.Context(), itemID)
	require.NoError(t, err)
	attachPRFixWorkSession(t, storage, repo, item, "work-session-uuid", repoDir, repoDir, "feature")

	_, err = svc.TransitionBacklogItemStatus(t.Context(), connect.NewRequest(&sessionv1.TransitionBacklogItemStatusRequest{
		ItemId:       itemID,
		TargetStatus: string(session.BacklogStatusReview),
	}))
	require.NoError(t, err)

	_, err = svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{
		ItemId: itemID,
	}))
	require.NoError(t, err)

	updated, err := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReview), updated.Msg.Item.Status,
		"PASS verdict with unshipped work-session code and no PR must stay in review for the Ship PR action, not silently transition to done")
}

// TestTriggerReReview_SetsBacklogCategory verifies that TriggerReReview with a
// SessionCreator wired spawns the re-review session with Category == "Backlog"
// so it groups correctly in the session list UI.
func TestTriggerReReview_SetsBacklogCategory(t *testing.T) {
	storage := createTestStorage(t)
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)

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

// ─── TriggerReReview: BuildReviewCallOptions wiring (Story 2.2.3) + timeout degrade (Story 2.2.4) ──

// setupItemInReview creates a backlog item with repoPath and no worktree/work session,
// transitions it ready -> in_progress -> review, and returns its ID. With no work
// session, TriggerReReview's getWorkSessionDiff returns "" unconditionally, guaranteeing
// the empty-diff (codebase-read) path for tests that don't need real diff content.
func setupItemInReview(t *testing.T, svc *BacklogService, repoPath string) string {
	t.Helper()
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:    "test item",
		RepoPath: repoPath,
		AcceptanceCriteria: []*sessionv1.AcCriterion{
			{Index: 0, Text: "test", Status: "pending"},
		},
		SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

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
	return itemID
}

// TestTriggerReReview_EmptyDiff_UsesWorkDirAndCodebaseAccessPrompt verifies that an
// empty-diff re-review call is routed through BuildReviewCallOptions' codebase-access
// branch: WorkDir is set to the resolved codebase work dir (item.RepoPath, since no
// work session/worktree is recorded), and the resulting PASS verdict (backed by real
// tool_reads evidence) is persisted as-is.
func TestTriggerReReview_EmptyDiff_UsesWorkDirAndCodebaseAccessPrompt(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))
	itemID := setupItemInReview(t, svc, repoDir)

	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	// Bypass the Story 2.2.6 capability self-check: it would otherwise consume the
	// fake pool's single scripted response for its own marker-file smoke test
	// before the real re-review call this test is asserting on ever runs.
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)

	require.Equal(t, 1, pool.callCount())
	call := pool.firstCall()
	assert.Equal(t, repoDir, call.workDir, "empty-diff re-review must be granted WorkDir access to the codebase")
	assert.Equal(t, headless.CodebaseReadAllowedTools, call.allowedTools)
	assert.Equal(t, session.PermissionModeBypassPermissions, call.permissionMode)
	assert.Equal(t, headless.HeadlessReviewSystemPromptWithCodebaseAccess(), call.systemPrompt)

	outcome, err := storage.GetMostRecentReviewVerdictForItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictPass, outcome)
}

// TestTriggerReReview_EmptyDiff_ContextExtrasReachPrompt is a wiring-level test
// proving that on the codebase-read (empty-diff) path, TriggerReReview actually
// fetches prior review sessions, the full notes history, and the item's Description,
// and that all of this reaches the actual prompt text sent to the pool.
func TestTriggerReReview_EmptyDiff_ContextExtrasReachPrompt(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))

	createdItem, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:              "Context extras re-review test",
		Description:        "Add OAuth2 login support end to end",
		AcceptanceCriteria: `[{"index":0,"text":"test","status":"pending"}]`,
		Status:             string(session.BacklogStatusReview),
		RepoPath:           repoDir,
	})
	require.NoError(t, err)
	itemID := createdItem.ID

	// A prior review session+verdict — must surface in "## Prior Review Attempts".
	_, err = storage.CreateItemSessionWithVerdict(t.Context(), session.ItemSessionData{
		ItemID:      itemID,
		SessionUUID: "prior-review-1",
		SessionRole: session.SessionRoleReview,
	}, session.ReviewVerdictData{
		OverallOutcome: session.ReviewVerdictUnverifiable,
		Summary:        "prior attempt could not locate satisfying evidence",
	})
	require.NoError(t, err)

	// A report_progress note — must surface in "## Full Notes History".
	require.NoError(t, storage.AppendProgressNote(t.Context(), itemID, 0, "investigated the auth package first", "in_progress"))

	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)

	require.Equal(t, 1, pool.callCount())
	prompt := pool.firstCall().userPrompt

	assert.Contains(t, prompt, "## Prior Review Attempts", "prior review sessions must be fetched and rendered")
	assert.Contains(t, prompt, "prior attempt could not locate satisfying evidence")
	assert.Contains(t, prompt, "## Full Notes History", "progress notes must be fetched and rendered")
	assert.Contains(t, prompt, "investigated the auth package first")
	assert.Contains(t, prompt, "## Item Context", "item description must reach the prompt")
	assert.Contains(t, prompt, "Add OAuth2 login support end to end")
}

// TestTriggerReReview_HappyPath_ThreadsCallCostIntoItemSession verifies (MUST FIX #3)
// that TriggerReReview's success path captures headlessPool.CallBlocking's returned
// cost and persists it as the ItemSession's EstimatedCostUsd, matching the sibling
// ReviewGateRunner.Run behavior — previously the cost was discarded via `_`.
func TestTriggerReReview_HappyPath_ThreadsCallCostIntoItemSession(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))
	itemID := setupItemInReview(t, svc, repoDir)

	const wantCost = 0.0421
	pool := &fakeHeadlessPool{
		response: `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`,
		cost:     wantCost,
	}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)

	assert.InDelta(t, wantCost, resp.Msg.ItemSession.EstimatedCostUsd, 1e-9,
		"TriggerReReview's success path must thread CallBlocking's cost into the persisted ItemSession")
}

// TestTriggerReReview_EmptyDiff_UsesShorterCodebaseReadTimeout verifies the empty-diff
// re-review call runs under headless.CodebaseReadCallTimeout (600s), not the plain
// headless.DefaultCallTimeout (900s).
func TestTriggerReReview_EmptyDiff_UsesShorterCodebaseReadTimeout(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	itemID := setupItemInReview(t, svc, repoDir)

	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"ok","tool_reads":[],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	before := time.Now()
	_, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)

	require.Equal(t, 1, pool.callCount())
	call := pool.firstCall()
	require.True(t, call.hasDeadline, "empty-diff re-review call must run under a bounded context deadline")
	remaining := call.ctxDeadline.Sub(before)
	assert.Less(t, remaining, headless.DefaultCallTimeout, "empty-diff call must use a timeout shorter than the plain DefaultCallTimeout")
	assert.InDelta(t, headless.CodebaseReadCallTimeout.Seconds(), remaining.Seconds(), 5, "empty-diff call must use CodebaseReadCallTimeout specifically")
}

// TestTriggerReReview_CodebaseReadTimeout_RecordsUnverifiableNotFail verifies that a
// context.DeadlineExceeded on the codebase-read (empty-diff) path degrades to
// UNVERIFIABLE — not a generic RPC error and not FAIL — per ADR-001's 2026-07-14
// Repair Pass Addendum.
func TestTriggerReReview_CodebaseReadTimeout_RecordsUnverifiableNotFail(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	itemID := setupItemInReview(t, svc, repoDir)

	// delay is deliberately longer than the short outer ctx below, so CallBlocking
	// naturally observes ctx.Done() (context.DeadlineExceeded) instead of completing.
	pool := &fakeHeadlessPool{delay: 5 * time.Second}
	svc.SetHeadlessPool(pool)
	// Bypass the Story 2.2.6 capability self-check so this test exercises the
	// call-timeout degrade path it's actually named for, not the (also-UNVERIFIABLE,
	// but different) capability-self-check-failure path.
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	shortCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	resp, err := svc.TriggerReReview(shortCtx, connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err, "a codebase-read timeout must not surface as an RPC error — it must be recorded as an UNVERIFIABLE verdict")
	require.NotNil(t, resp.Msg.ItemSession)

	outcome, err := storage.GetMostRecentReviewVerdictForItem(context.Background(), itemID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictUnverifiable, outcome, "a codebase-read timeout must degrade to UNVERIFIABLE, never be recorded as FAIL")
}

// TestTriggerReReview_CodebaseReadEmptyToolReads_DowngradesPassToUnverifiable verifies
// that a PASS verdict returned on the codebase-read path with an empty tool_reads list
// is downgraded to UNVERIFIABLE before being persisted.
func TestTriggerReReview_CodebaseReadEmptyToolReads_DowngradesPassToUnverifiable(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	itemID := setupItemInReview(t, svc, repoDir)

	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"trust me, it's already implemented","tool_reads":[],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)

	outcome, err := storage.GetMostRecentReviewVerdictForItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictUnverifiable, outcome, "a PASS with no tool_reads evidence on the codebase-read path must be downgraded")
}

// TestTriggerReReview_CodebaseReadFabricatedToolReadsPath_DowngradesPassToUnverifiable
// verifies that a PASS verdict citing a tool_reads path that does not actually exist
// under the codebase work dir is downgraded to UNVERIFIABLE.
func TestTriggerReReview_CodebaseReadFabricatedToolReadsPath_DowngradesPassToUnverifiable(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir() // deliberately empty — no files created here
	itemID := setupItemInReview(t, svc, repoDir)

	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"found it","tool_reads":["does/not/exist.go"],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewPassedCapabilitySelfCheckForTesting())

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)

	outcome, err := storage.GetMostRecentReviewVerdictForItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictUnverifiable, outcome, "a PASS citing a fabricated tool_reads path must be downgraded")
}

// TestTriggerReReview_CapabilitySelfCheckFails_RecordsUnverifiableWithoutAttemptingCodebaseReadCall
// verifies that when the codebase-read capability self-check has already failed,
// TriggerReReview records an UNVERIFIABLE verdict directly and never attempts the real
// AllowedTools/PermissionMode-bearing codebase-read call — shares
// headless.CodebaseReadCapabilitySelfCheck's contract with ReviewGateRunner (Story
// 2.2.6c: a failure discovered via either call site short-circuits the other).
func TestTriggerReReview_CapabilitySelfCheckFails_RecordsUnverifiableWithoutAttemptingCodebaseReadCall(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	repoDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("x"), 0o644))
	itemID := setupItemInReview(t, svc, repoDir)

	pool := &fakeHeadlessPool{response: `{"overall":"PASS","summary":"found it","tool_reads":["marker.txt"],"verdicts":[]}`}
	svc.SetHeadlessPool(pool)
	svc.SetCapabilityCheck(headless.NewFailedCapabilitySelfCheckForTesting())

	resp, err := svc.TriggerReReview(t.Context(), connect.NewRequest(&sessionv1.TriggerReReviewRequest{ItemId: itemID}))
	require.NoError(t, err, "a failed capability self-check must not surface as an RPC error — it must be recorded as an UNVERIFIABLE verdict")
	require.NotNil(t, resp.Msg.ItemSession)

	assert.Equal(t, 0, pool.callCount(), "no real codebase-read call should have been attempted once the capability self-check has failed")

	outcome, err := storage.GetMostRecentReviewVerdictForItem(t.Context(), itemID)
	require.NoError(t, err)
	assert.Equal(t, session.ReviewVerdictUnverifiable, outcome, "a failed capability self-check must degrade the re-review to UNVERIFIABLE")
}

// TestResolveACSnapshot_MergesLiveNoteIntoStaleWorkSessionSnapshot verifies that
// a Note written via report_progress onto the live item's AcceptanceCriteria after
// the work session's AcSnapshot was captured at spawn time is merged into the
// snapshot returned by resolveACSnapshot, so TriggerReReview doesn't hand the
// reviewer a stale, note-less AC list.
func TestResolveACSnapshot_MergesLiveNoteIntoStaleWorkSessionSnapshot(t *testing.T) {
	staleSnapshot := session.AcCriteriaJSON(`[{"index":0,"text":"Do the thing","status":"pending"}]`)
	liveAC := session.AcCriteriaJSON(`[{"index":0,"text":"Do the thing","status":"done","note":"finished via report_progress"}]`)

	workSession := &session.ItemSessionSummary{AcSnapshot: staleSnapshot}

	result := resolveACSnapshot(workSession, liveAC)

	require.Len(t, result, 1)
	assert.Equal(t, "finished via report_progress", result[0].Note)
	assert.Equal(t, session.AcStatusDone, result[0].Status)
}

// TestResolveACSnapshot_NoWorkSession_ReturnsLiveAC verifies the live AC criteria
// are returned unchanged when there is no work session snapshot to merge against.
func TestResolveACSnapshot_NoWorkSession_ReturnsLiveAC(t *testing.T) {
	liveAC := session.AcCriteriaJSON(`[{"index":0,"text":"Do the thing","status":"done","note":"live note"}]`)

	result := resolveACSnapshot(nil, liveAC)

	require.Len(t, result, 1)
	assert.Equal(t, "live note", result[0].Note)
	assert.Equal(t, session.AcStatusDone, result[0].Status)
}

// ─── T-11: Auto-triage, double-trigger guard, itemSessionToProto ─────────────

// TestCreateBacklogItem_SkipsTriageWhenSkipTriageTrue: skip_triage=true → triage_triggered=false,
// no CreateDirectorySession call.
func TestCreateBacklogItem_SkipsTriageWhenSkipTriageTrue(t *testing.T) {
	creator := &mockSessionCreator{}
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil, nil, nil)

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
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil, nil, nil)

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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(createTestStorage(t), creator, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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

	// ItemSession should be ended. This is a SEPARATE, LATER write than the
	// status transition polled above — TriggerTriage's cleanup goroutine
	// transitions the item to Ready first, then (after the auto-spawn
	// branch) calls UpdateItemSessionEnded — so polling only on status
	// leaves a real race window where EndedAt hasn't landed yet. Poll for
	// it too, the same way, rather than asserting immediately.
	require.Eventually(t, func() bool {
		sessions, listErr := storage.ListItemSessions(t.Context(), item.ID)
		return listErr == nil && len(sessions) == 1 && sessions[0].EndedAt != nil
	}, 5*time.Second, 50*time.Millisecond, "triage item session should be marked ended on success")
}

// TestTriggerTriage_RunsInIsolatedWorktree_When_RepoPathIsARealGitRepo guards the
// fix for triage writing planning docs directly into item.RepoPath — a routinely
// shared or actively-used checkout (an app-managed mirror other sessions touch, or
// a developer's own live working directory for items created with repo_path
// defaulted to the calling session's cwd). When repo_path is a real git repo,
// triage must run in a dedicated worktree, not repo_path itself.
func TestTriggerTriage_RunsInIsolatedWorktree_When_RepoPathIsARealGitRepo(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "isolated worktree triage item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		return pool.callCount() == 1
	}, 5*time.Second, 50*time.Millisecond)

	workDir := pool.firstCall().workDir
	assert.NotEqual(t, repoPath, workDir, "triage must not run directly in repo_path when repo_path is a real git repo")
	assert.NotEmpty(t, workDir)
	info, statErr := os.Stat(workDir)
	require.NoError(t, statErr, "the worktree directory triage was told to use must actually exist")
	assert.True(t, info.IsDir())
}

// TestTriggerTriage_FallsBackToRepoPathDirectly_When_RepoPathIsNotAGitRepo locks in
// the fallback: worktree creation only kicks in when repo_path is a real git repo —
// a repo_path that legitimately isn't one (a plain directory item) must not break
// triage, and must preserve the pre-existing behavior of running directly there.
func TestTriggerTriage_FallsBackToRepoPathDirectly_When_RepoPathIsNotAGitRepo(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir() // deliberately not git-initialized
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "non-git repo_path triage item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		return pool.callCount() == 1
	}, 5*time.Second, 50*time.Millisecond)

	assert.Equal(t, repoPath, pool.firstCall().workDir,
		"a non-git repo_path must fall back to running triage directly there, same as before this change")
}

// TestTriggerTriage_CommitsSDDArtifactsInWorktree_AndUpdatesPlanArtifactsPath is the
// end-to-end regression test for both halves of the fix: SDD-mode triage's
// project_plans/<name>/ output must land in the isolated worktree and get
// committed there (closing the gap .claude/rules/sdd-planning-artifacts-commit.md
// already names), and PlanArtifactsPath must point at the implementation/
// subdirectory the SDD skills actually write plan.md into — not artifactAbsPath,
// which SDD-mode never writes to at all.
func TestTriggerTriage_CommitsSDDArtifactsInWorktree_AndUpdatesPlanArtifactsPath(t *testing.T) {
	storage := createTestStorage(t)
	const slug = "my-test-slug"
	pool := &fakeHeadlessPool{
		response: `{"title":"` + slug + `","summary":"test summary"}`,
		onCall: func(workDir string) {
			// Simulate the SDD skills writing project_plans/<slug>/implementation/plan.md.
			dir := filepath.Join(workDir, "project_plans", slug, "implementation")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.md"), []byte("# Plan\n"), 0o644))
		},
	}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:        "sdd mode triage item",
		Status:       string(session.BacklogStatusIdea),
		Priority:     3,
		RepoPath:     repoPath,
		PipelineMode: session.DefaultSDDPipelineModeSlug,
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond)

	workDir := pool.firstCall().workDir
	require.NotEqual(t, repoPath, workDir)

	// The worktree must have a real commit for the simulated SDD output — not left
	// sitting uncommitted.
	logCmd := exec.Command("git", "-C", workDir, "log", "--oneline") //nolint:norawexec // test assertion, blocking CombinedOutput
	out, logErr := logCmd.CombinedOutput()
	require.NoError(t, logErr)
	assert.Contains(t, string(out), "chore(sdd): planning artifacts for "+slug)

	statusCmd := exec.Command("git", "-C", workDir, "status", "--porcelain") //nolint:norawexec // test assertion
	statusOut, statusErr := statusCmd.CombinedOutput()
	require.NoError(t, statusErr)
	assert.Empty(t, string(statusOut), "the worktree must be clean after triage commits its output")

	updated, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	expectedPath := filepath.Join(workDir, "project_plans", slug, "implementation")
	assert.Equal(t, expectedPath, updated.PlanArtifactsPath,
		"PlanArtifactsPath must point at the implementation/ subdir SDD's plan.md actually lives in")
}

// TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession is the
// end-to-end proof that the triage worktree isn't just isolated — it's the SAME
// worktree/branch the real work session ends up using. Runs the full real
// pipeline (CreateBacklogItem -> TriggerTriage -> ApprovePlan ->
// SpawnSessionFromItem, faking only the two external process boundaries: the
// headless LLM call and the tmux/claude subprocess), for an SDD-mode item, and
// asserts CreateWorktreeSession was called with the exact path TriggerTriage's
// worktree was created at — proving retitleTriageWorktreeToFinalBranch's branch
// rename actually lines up with spawnSessionAfterGates' independent branch-name
// computation (via triageShortTitle picking up the stored triage result's
// title), not just that each half compiles in isolation.
func TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession(t *testing.T) {
	// Force an isolated config/worktree base directory for this test invocation.
	// Without this, config.GetConfigDirForDir's IsTestMode() branch (config/config.go)
	// scopes the worktree base dir by OS PID only (~/.stapler-squad/test/test-<pid>/),
	// which is shared across every repetition of `go test -count=N` in the same
	// process. Combined with repoPath's TempDir-derived branch slug being byte-identical
	// across repetitions (createTestStorage's internal t.TempDir() call is always #1,
	// this test's own repoPath := t.TempDir() below is always #2, so both are always
	// named "002" regardless of repetition), a leftover worktree directory or git
	// worktree-admin entry from an earlier repetition could be discovered and "reused"
	// by session/git/worktree.go's findExistingWorktreeForBranch, which matches on
	// branch name only within git's own repo-local registry and never validates the
	// found worktree's gitlink still resolves to a live repo. That produced the
	// intermittent "Condition never satisfied" flake (require.Eventually never seeing
	// status flip to "ready", because the async triage goroutine's dirty-check failed
	// with "fatal: not a git repository: .../.git/worktrees/<stale-uuid>" against a
	// stale sibling repetition's already-torn-down worktree admin dir). Setting
	// STAPLER_SQUAD_TEST_DIR (config.GetConfigDirForDir's Priority 1, above the
	// PID-scoped IsTestMode() fallback) to this test's own t.TempDir() gives every
	// repetition a fully isolated worktree base dir, closing the collision at the
	// test level without touching the shared worktree-reuse production code.
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	storage := createTestStorage(t)
	const slug = "widget-integration"
	pool := &fakeHeadlessPool{
		response: `{"title":"` + slug + `","summary":"build the widget"}`,
		onCall: func(workDir string) {
			dir := filepath.Join(workDir, "project_plans", slug, "implementation")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.md"), []byte("# Plan\n"), 0o644))
		},
	}
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)

	pipelineMode := session.DefaultSDDPipelineModeSlug
	createResp, err := svc.CreateBacklogItem(t.Context(), connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:        "widget integration",
		RepoPath:     repoPath,
		SkipTriage:   true,
		PipelineMode: &pipelineMode,
	}))
	require.NoError(t, err)
	itemID := createResp.Msg.Item.Id

	_, err = svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: itemID}))
	require.NoError(t, err)

	// 5s/50ms matches the sibling SDD-mode triage tests immediately above
	// (TestTriggerTriage_CommitsSDDArtifactsInWorktree_AndUpdatesPlanArtifactsPath
	// et al.) rather than the tighter 2s/10ms this test previously used — under
	// -race, observed passing runs of this exact test already take 3.5-4.2s end to
	// end (worktree create+setup, fake headless call, commit, branch rename, DB
	// writes), so the old 2s budget was undersized independent of the worktree
	// test-isolation fix above.
	require.Eventually(t, func() bool {
		getResp, getErr := svc.GetBacklogItem(t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: itemID}))
		return getErr == nil && getResp.Msg.Item.Status == "ready"
	}, 5*time.Second, 50*time.Millisecond)

	require.Equal(t, 1, pool.callCount())
	triageWorktreePath := pool.firstCall().workDir
	require.NotEqual(t, repoPath, triageWorktreePath, "sanity: triage must have run in an isolated worktree")

	_, err = svc.ApprovePlan(t.Context(), connect.NewRequest(&sessionv1.ApprovePlanRequest{ItemId: itemID}))
	require.NoError(t, err)

	_, err = svc.SpawnSessionFromItem(t.Context(), connect.NewRequest(&sessionv1.SpawnSessionFromItemRequest{ItemId: itemID}))
	require.NoError(t, err)

	require.Len(t, creator.calls, 1)
	assert.Equal(t, triageWorktreePath, creator.calls[0].path,
		"SpawnSessionFromItem must reuse the exact worktree TriggerTriage created and committed its SDD docs into, not start a fresh one from main")

	// The committed planning docs must still be present — proving this is a
	// reuse-in-place, not a coincidental path match after the real content was
	// discarded.
	planPath := filepath.Join(triageWorktreePath, "project_plans", slug, "implementation", "plan.md")
	_, statErr := os.Stat(planPath)
	assert.NoError(t, statErr, "the SDD plan.md committed during triage must still exist in the reused worktree")
}

// TestTriggerTriage_should_ApplyAssessedPriorityAndCategory_When_LLMProvidesThem guards
// the fix making triage actually assign priority/category instead of leaving every item
// at DefaultBacklogPriority forever (which defeats DequeueNextQueuedItems' priority-order
// auto-spawn — every item tied at the same priority is effectively still FIFO). The LLM's
// assessed priority and item_category must land on the item once triage completes.
func TestTriggerTriage_should_ApplyAssessedPriorityAndCategory_When_LLMProvidesThem(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: `{"summary":"critical bug","priority":1,"item_category":"bugfix","suggestions":[{"text":"fix it","rationale":"why"}]}`}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "assessed priority item",
		Status:   string(session.BacklogStatusIdea),
		Priority: session.DefaultBacklogPriority,
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond)

	updated, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, updated.Priority, "the LLM's assessed priority must be applied to the item")
	assert.Equal(t, "bugfix", updated.Category, "the LLM's assessed item_category must be applied to the item")
}

// TestTriggerTriage_should_NotClobberExistingPriorityOrCategory_When_LLMOmitsThem
// guards the "don't clobber" half of the same fix: a triage result with no priority/
// item_category (the model didn't provide one, or ParseHeadlessTriageResult zero-values
// them) must leave whatever the item already had untouched, not reset it.
func TestTriggerTriage_should_NotClobberExistingPriorityOrCategory_When_LLMOmitsThem(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()} // no priority/item_category field
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "pre-set priority item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 2,
		Category: string(session.BacklogCategoryChore),
		RepoPath: repoPath,
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond)

	updated, err := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Priority, "an omitted priority must not clobber the item's existing priority")
	assert.Equal(t, string(session.BacklogCategoryChore), updated.Category, "an omitted item_category must not clobber the item's existing category")
}

// TestTriggerTriage_AutoSpawnSession_SpawnsWorkSessionWithoutManualClick verifies the
// opt-in auto-spawn-session toggle: when AutoSpawnSession is true, TriggerTriage's
// completion goroutine spawns a work session automatically (Autonomous: true, bypassing
// the planning-approval gate) once the item reaches ready — no manual "Spawn Session"
// click required.
func TestTriggerTriage_AutoSpawnSession_SpawnsWorkSessionWithoutManualClick(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:            "auto-spawn triage item",
		Status:           string(session.BacklogStatusIdea),
		Priority:         3,
		RepoPath:         repoPath,
		AutoSpawnSession: true,
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	// Poll for the final state directly (in_progress), not just the spawn call — the
	// creator call and the subsequent in_progress transition both happen inside the same
	// synchronous SpawnSessionFromItem call, but from a different goroutine than this
	// test, so checking creator.calls alone races with the transition that follows it.
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusInProgress)
	}, 5*time.Second, 50*time.Millisecond, "auto-spawn must carry the item all the way to in_progress, not leave it sitting at ready")

	assert.Len(t, creator.calls, 1, "a work session should be auto-spawned once triage completes")
}

// TestTriggerTriage_AutoSpawnSessionFalse_LeavesItemAtReadyForManualSpawn is the
// default-behavior guard: with AutoSpawnSession left false (the default), the existing
// manual-click flow must be completely unchanged.
func TestTriggerTriage_AutoSpawnSessionFalse_LeavesItemAtReadyForManualSpawn(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	creator := &mockSessionCreator{}
	svc := NewBacklogService(storage, creator, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	repoPath := t.TempDir()
	initGitRepoWithCommit(t, repoPath)
	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "manual-spawn triage item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: repoPath,
		// AutoSpawnSession left at its zero value (false).
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond)

	assert.Empty(t, creator.calls, "no session should be spawned without the opt-in toggle")
}

// TestTriggerTriage_PersistFailurePublishesNotification verifies the fix for the
// swallowed-persistence-error bug: when the final idea->ready status transition fails
// after a successful triage (e.g. a concurrent status change broke its precondition), the
// operator must get a notification — previously this only reached the log file, leaving
// the item stuck at 'idea' with zero operator-visible signal that anything went wrong.
func TestTriggerTriage_PersistFailurePublishesNotification(t *testing.T) {
	storage := createTestStorage(t)
	// Delay the fake LLM call so the test can race a status change in underneath it,
	// deterministically forcing the final TransitionBacklogItemStatus precondition to fail.
	pool := &fakeHeadlessPool{response: validTriageJSON(), delay: 200 * time.Millisecond}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)
	eventBus := events.NewEventBus(4)
	svc.SetEventBus(eventBus)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "persist failure test item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	subCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch, _ := eventBus.Subscribe(subCtx)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)

	// Move the item off 'idea' while the delayed headless call is still in flight.
	_, err = storage.TransitionBacklogItemStatus(t.Context(), item.ID, session.BacklogStatusReview, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	var notif *events.Event
	for i := 0; i < 5; i++ {
		select {
		case ev := <-ch:
			if ev.Type == events.EventNotification {
				notif = ev
			}
		case <-time.After(3 * time.Second):
			i = 5
		}
		if notif != nil {
			break
		}
	}
	require.NotNil(t, notif, "a persistence failure during triage completion must publish an operator notification")
	assert.Contains(t, notif.NotificationTitle, "save step failed")
	assert.Contains(t, notif.NotificationMessage, "advancing the item to Ready")

	// The item must remain wherever the test left it (review) — not silently ready.
	updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, loadErr)
	assert.Equal(t, string(session.BacklogStatusReview), updated.Status)
}

// TestTriggerTriage_RefineWithFeedback: a second TriggerTriage call with feedback
// set produces a distinct revised result (iteration 2), embeds the prior result and
// the feedback text in the prompt, and both triage ItemSessions are retained.
func TestTriggerTriage_RefineWithFeedback(t *testing.T) {
	storage := createTestStorage(t)
	secondResponse := `{"summary":"revised summary","suggestions":[],"tasks":[{"text":"revised task","estimate":"3h","category":"backend"}]}`
	pool := &fakeHeadlessPool{responses: []string{validTriageJSON(), secondResponse}}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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

// TestTriggerTriage_RefineWithFeedback_ResetsPlanApproved is the symmetry-fix
// regression test for the TriggerTriage completion write: a freshly
// regenerated plan must not carry forward a stale approval from before the
// refine — the newly generated plan is pending_review, not approved. See
// ADR-001.
func TestTriggerTriage_RefineWithFeedback_ResetsPlanApproved(t *testing.T) {
	storage := createTestStorage(t)
	secondResponse := `{"summary":"revised summary","suggestions":[],"tasks":[{"text":"revised task","estimate":"3h","category":"backend"}]}`
	pool := &fakeHeadlessPool{responses: []string{validTriageJSON(), secondResponse}}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "refine after approval",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "initial triage should mark item ready")

	approveResp, err := svc.ApprovePlan(t.Context(), connect.NewRequest(&sessionv1.ApprovePlanRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, err)
	require.True(t, approveResp.Msg.Item.PlanApproved)

	_, refineErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId:   item.ID,
		Feedback: "This missed the mobile case entirely.",
	}))
	require.NoError(t, refineErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady) && !updated.PlanApproved
	}, 5*time.Second, 50*time.Millisecond, "refine completion should reset plan_approved to false")
}

// TestTriggerTriage_RefineWithFeedback_ClearsRejectionReason is the paired
// symmetry-fix regression test: a freshly regenerated plan must not carry
// forward a stale rejection reason from before the refine that the
// regeneration was meant to address. See ADR-001.
func TestTriggerTriage_RefineWithFeedback_ClearsRejectionReason(t *testing.T) {
	storage := createTestStorage(t)
	secondResponse := `{"summary":"revised summary","suggestions":[],"tasks":[{"text":"revised task","estimate":"3h","category":"backend"}]}`
	pool := &fakeHeadlessPool{responses: []string{validTriageJSON(), secondResponse}}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "refine after rejection",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.NoError(t, trigErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady)
	}, 5*time.Second, 50*time.Millisecond, "initial triage should mark item ready")

	rejectResp, err := svc.RejectPlan(t.Context(), connect.NewRequest(&sessionv1.RejectPlanRequest{
		ItemId: item.ID,
		Reason: "This missed the mobile case entirely.",
	}))
	require.NoError(t, err)
	require.Equal(t, "This missed the mobile case entirely.", rejectResp.Msg.Item.PlanRejectionReason)
	require.NotNil(t, rejectResp.Msg.Item.PlanRejectedAt)

	_, refineErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId:   item.ID,
		Feedback: rejectResp.Msg.Item.PlanRejectionReason,
	}))
	require.NoError(t, refineErr)
	require.Eventually(t, func() bool {
		updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
		return loadErr == nil && updated.Status == string(session.BacklogStatusReady) && updated.PlanRejectionReason == ""
	}, 5*time.Second, 50*time.Millisecond, "refine completion should clear plan_rejection_reason")

	updated, loadErr := storage.GetBacklogItem(t.Context(), item.ID)
	require.NoError(t, loadErr)
	assert.Nil(t, updated.PlanRejectedAt, "plan_rejected_at must be cleared symmetrically with plan_rejection_reason on refine completion")
}

// TestTriggerTriage_RefineWithFeedback_RequiresPriorResult: feedback on an item
// with no completed triage result is rejected rather than silently running a
// fresh triage as if the feedback were ignored.
func TestTriggerTriage_RefineWithFeedback_RequiresPriorResult(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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

// TestTriggerTriage_AlreadyExists_LiveHeadlessSession guards the fix for BUG-054:
// before triageInFlight existed, a headless triage session with EndedAt == nil was
// *always* treated as dead (see the removed isHeadless-implies-notLive branch this
// test replaces the assumption behind), so retriggering triage for an item whose
// headless call was genuinely still running silently tombstoned the live session in
// the DB and started a fully redundant duplicate LLM call — confirmed live
// 2026-08-01 (docs/bugs/fixed/BUG-054): a manual "Retry now" click raced a still-running
// auto-respawned triage call, producing a real "concurrent modification detected"
// error when the older call finally finished and tried to also transition idea->ready.
// A headless triage session must now be treated as live exactly when this process's
// own triageInFlight record says so.
func TestTriggerTriage_AlreadyExists_LiveHeadlessSession(t *testing.T) {
	storage := createTestStorage(t)
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	svc.SetHeadlessPool(pool)

	item, err := storage.CreateBacklogItem(t.Context(), session.BacklogItemData{
		Title:    "live headless triage item",
		Status:   string(session.BacklogStatusIdea),
		Priority: 3,
		RepoPath: t.TempDir(),
	})
	require.NoError(t, err)

	// Open headless triage session, exactly like the genuinely-still-running case:
	// no EndedAt, headless-prefixed UUID.
	_, isErr := storage.CreateItemSession(t.Context(), session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "headless-triage-11111111-live-0000-000000000000",
		SessionRole: string(session.SessionRoleTriage),
	})
	require.NoError(t, isErr)

	// Simulate the goroutine actually still running this call, the way TriggerTriage
	// itself would have set it before launching that goroutine.
	svc.triageInFlight.Store(item.ID, struct{}{})

	_, trigErr := svc.TriggerTriage(t.Context(), connect.NewRequest(&sessionv1.TriggerTriageRequest{
		ItemId: item.ID,
	}))
	require.Error(t, trigErr, "a live headless triage call must block re-trigger instead of being silently tombstoned and duplicated")
	var connErr *connect.Error
	require.ErrorAs(t, trigErr, &connErr)
	assert.Equal(t, connect.CodeAlreadyExists, connErr.Code())

	// The original session must NOT have been tombstoned.
	sessions, listErr := storage.ListItemSessions(t.Context(), item.ID)
	require.NoError(t, listErr)
	require.Len(t, sessions, 1)
	assert.Nil(t, sessions[0].EndedAt, "a genuinely live headless session must not be marked ended")
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

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
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

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

// TestBacklogService_ScrollbackManager_ConcurrentSetAndGet_NoRace is a regression test
// for a real data race: BacklogService.scrollbackManager used to be a bare, unguarded
// field, set via SetScrollbackManager and read directly by TriggerReReview. In
// production, server/dependencies.go wires SetScrollbackManager well after the HTTP
// server can already be serving TriggerReReview RPCs (the server starts controllers
// and begins accepting requests before all dependency wiring, including
// SetScrollbackManager, completes), so the field is genuinely read and written
// concurrently. This test concurrently calls SetScrollbackManager and the internal
// getScrollbackManager (the same accessor TriggerReReview now uses) many times; run
// with `go test -race`, it fails on the pre-fix bare field and passes once the field
// is guarded by scrollbackMu.
func TestBacklogService_ScrollbackManager_ConcurrentSetAndGet_NoRace(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	sm1 := scrollback.NewScrollbackManager(scrollback.DefaultScrollbackConfig())
	sm2 := scrollback.NewScrollbackManager(scrollback.DefaultScrollbackConfig())

	var wg sync.WaitGroup
	const n = 100
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				svc.SetScrollbackManager(sm1)
			} else {
				svc.SetScrollbackManager(sm2)
			}
		}(i)
		go func() {
			defer wg.Done()
			_ = svc.getScrollbackManager()
		}()
	}
	wg.Wait()
}

// ─── Epic 0.5: per-source sync-direction settings (ForwardSyncEnabled,
// BackwardSyncEnabled, ForwardSyncCloseLabel) ──────────────────────────────

// TestUpdateItemSource_RoundTripsForwardBackwardSyncEnabled verifies the
// three new sync-direction fields flow all the way from the UpdateItemSource
// RPC through storage and back out via ListItemSources, mirroring how the
// pre-existing Enabled field already round-trips.
func TestUpdateItemSource_RoundTripsForwardBackwardSyncEnabled(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	created, err := svc.CreateItemSource(context.Background(), &connect.Request[sessionv1.CreateItemSourceRequest]{
		Msg: &sessionv1.CreateItemSourceRequest{
			PluginId:    "github_issues",
			DisplayName: "My GitHub",
		},
	})
	require.NoError(t, err)
	sourceID := created.Msg.Source.Id

	// Newly created sources default to both directions disabled and no close label.
	require.False(t, created.Msg.Source.ForwardSyncEnabled)
	require.False(t, created.Msg.Source.BackwardSyncEnabled)
	require.Equal(t, "", created.Msg.Source.ForwardSyncCloseLabel)

	_, err = svc.UpdateItemSource(context.Background(), &connect.Request[sessionv1.UpdateItemSourceRequest]{
		Msg: &sessionv1.UpdateItemSourceRequest{
			SourceId:              sourceID,
			DisplayName:           "My GitHub",
			Enabled:               true,
			ForwardSyncEnabled:    true,
			BackwardSyncEnabled:   true,
			ForwardSyncCloseLabel: "wontfix",
		},
	})
	require.NoError(t, err)

	listResp, err := svc.ListItemSources(context.Background(), &connect.Request[sessionv1.ListItemSourcesRequest]{})
	require.NoError(t, err)

	var found *sessionv1.ItemSource
	for _, s := range listResp.Msg.Sources {
		if s.Id == sourceID {
			found = s
			break
		}
	}
	require.NotNil(t, found, "updated source not found in ListItemSources")
	assert.True(t, found.ForwardSyncEnabled)
	assert.True(t, found.BackwardSyncEnabled)
	assert.Equal(t, "wontfix", found.ForwardSyncCloseLabel)
}

// TestUpdateItemSource_ClearsForwardSyncCloseLabel verifies that sending an
// empty ForwardSyncCloseLabel actually clears a previously-set label.
// UpdateItemSource is a full-state overwrite (see the frontend's
// useBacklogSourcesService.ts comment), so guarding the write on a non-empty
// string — as the handler previously did — silently ignored a user's attempt
// to clear the field via blur-triggered updates in BacklogSourcesSettings.tsx.
func TestUpdateItemSource_ClearsForwardSyncCloseLabel(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	created, err := svc.CreateItemSource(context.Background(), &connect.Request[sessionv1.CreateItemSourceRequest]{
		Msg: &sessionv1.CreateItemSourceRequest{
			PluginId:    "github_issues",
			DisplayName: "My GitHub",
		},
	})
	require.NoError(t, err)
	sourceID := created.Msg.Source.Id

	_, err = svc.UpdateItemSource(context.Background(), &connect.Request[sessionv1.UpdateItemSourceRequest]{
		Msg: &sessionv1.UpdateItemSourceRequest{
			SourceId:              sourceID,
			DisplayName:           "My GitHub",
			ForwardSyncCloseLabel: "wontfix",
		},
	})
	require.NoError(t, err)

	// Now clear it — this must actually persist as empty, not be silently
	// ignored because the RPC field is the empty string.
	_, err = svc.UpdateItemSource(context.Background(), &connect.Request[sessionv1.UpdateItemSourceRequest]{
		Msg: &sessionv1.UpdateItemSourceRequest{
			SourceId:              sourceID,
			DisplayName:           "My GitHub",
			ForwardSyncCloseLabel: "",
		},
	})
	require.NoError(t, err)

	listResp, err := svc.ListItemSources(context.Background(), &connect.Request[sessionv1.ListItemSourcesRequest]{})
	require.NoError(t, err)

	var found *sessionv1.ItemSource
	for _, s := range listResp.Msg.Sources {
		if s.Id == sourceID {
			found = s
			break
		}
	}
	require.NotNil(t, found, "updated source not found in ListItemSources")
	assert.Equal(t, "", found.ForwardSyncCloseLabel, "close label should have been cleared, not left unchanged")
}

// TestUpdateItemSource_ReturnsErrorForUnknownSourceId verifies UpdateItemSource
// surfaces a NotFound error (rather than silently succeeding) when the target
// source id does not exist — the error path counterpart to the round-trip test.
func TestUpdateItemSource_ReturnsErrorForUnknownSourceId(t *testing.T) {
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)

	_, err := svc.UpdateItemSource(context.Background(), &connect.Request[sessionv1.UpdateItemSourceRequest]{
		Msg: &sessionv1.UpdateItemSourceRequest{
			SourceId:           uuid.NewString(),
			DisplayName:        "Nonexistent",
			ForwardSyncEnabled: true,
		},
	})
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}
