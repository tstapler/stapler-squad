package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// recordingSessionRPC is the second SessionRPC implementation (the first is the composition
// root's adapter over *services.SessionService). It records every call and reaches branches the
// real service cannot: an await timeout, a vanished creation, an arbitrary RPC error.
type recordingSessionRPC struct {
	calls []string
	reqs  map[string]any
	errs  map[string]error

	programs      []string
	outcome       domain.CreationOutcome
	awaitErr      error
	awaitID       string
	awaitTimeout  time.Duration
	live          *instance.Instance
	findIDs       []string
	injectErr     error
	injectRoot    string
	injectTitle   string
	injectHooks   []string
	injected      int
	refuseErr     error
	refused       []*instance.Instance
	removedTitles []string
}

func newRecordingSessionRPC() *recordingSessionRPC {
	return &recordingSessionRPC{reqs: map[string]any{}, errs: map[string]error{}}
}

var _ SessionRPC = (*recordingSessionRPC)(nil)

func replyWith[Req, Resp any](f *recordingSessionRPC, name string, req *connect.Request[Req], resp *Resp) (*connect.Response[Resp], error) {
	f.calls = append(f.calls, name)
	f.reqs[name] = req.Msg
	if err := f.errs[name]; err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (f *recordingSessionRPC) ListProgramsConfig(_ context.Context, r *connect.Request[sessionv1.ListProgramsConfigRequest]) (*connect.Response[sessionv1.ListProgramsConfigResponse], error) {
	resp := &sessionv1.ListProgramsConfigResponse{}
	for _, id := range f.programs {
		resp.Programs = append(resp.Programs, &sessionv1.ProgramConfigProto{Id: id})
	}
	return replyWith(f, "ListProgramsConfig", r, resp)
}

func (f *recordingSessionRPC) CreateSession(_ context.Context, r *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error) {
	return replyWith(f, "CreateSession", r, &sessionv1.CreateSessionResponse{Session: &sessionv1.Session{Id: "created-id"}})
}

func (f *recordingSessionRPC) AwaitCreationTerminal(_ context.Context, id string, timeout time.Duration) (domain.CreationOutcome, error) {
	f.calls = append(f.calls, "AwaitCreationTerminal")
	f.awaitID, f.awaitTimeout = id, timeout
	return f.outcome, f.awaitErr
}

func (f *recordingSessionRPC) FindLiveInstance(id string) *instance.Instance {
	f.findIDs = append(f.findIDs, id)
	return f.live
}

func (f *recordingSessionRPC) InjectHooksConfig(rootDir, title string, hooks []string) error {
	f.injected++
	f.injectRoot, f.injectTitle, f.injectHooks = rootDir, title, hooks
	return f.injectErr
}

func (f *recordingSessionRPC) RefuseIfWorktreeSharedWithOtherLiveSession(inst *instance.Instance) error {
	f.refused = append(f.refused, inst)
	return f.refuseErr
}

func (f *recordingSessionRPC) RemoveFromAllPollers(title string) {
	f.removedTitles = append(f.removedTitles, title)
}

func (f *recordingSessionRPC) RefreshInstanceHookProof(*instance.Instance) {}

func (f *recordingSessionRPC) StopDriverForSession(string)    {}
func (f *recordingSessionRPC) TriggerReviewForSession(string) {}

func (f *recordingSessionRPC) CreateWorkflow(_ context.Context, r *connect.Request[sessionv1.CreateWorkflowRequest]) (*connect.Response[sessionv1.CreateWorkflowResponse], error) {
	return replyWith(f, "CreateWorkflow", r, &sessionv1.CreateWorkflowResponse{Workflow: &sessionv1.WorkflowProto{}})
}

func (f *recordingSessionRPC) UpdateWorkflow(_ context.Context, r *connect.Request[sessionv1.UpdateWorkflowRequest]) (*connect.Response[sessionv1.UpdateWorkflowResponse], error) {
	return replyWith(f, "UpdateWorkflow", r, &sessionv1.UpdateWorkflowResponse{Workflow: &sessionv1.WorkflowProto{}})
}

func (f *recordingSessionRPC) DeleteWorkflow(_ context.Context, r *connect.Request[sessionv1.DeleteWorkflowRequest]) (*connect.Response[sessionv1.DeleteWorkflowResponse], error) {
	return replyWith(f, "DeleteWorkflow", r, &sessionv1.DeleteWorkflowResponse{})
}

func (f *recordingSessionRPC) ListWorkflows(_ context.Context, r *connect.Request[sessionv1.ListWorkflowsRequest]) (*connect.Response[sessionv1.ListWorkflowsResponse], error) {
	return replyWith(f, "ListWorkflows", r, &sessionv1.ListWorkflowsResponse{})
}

func (f *recordingSessionRPC) RunWorkflow(_ context.Context, r *connect.Request[sessionv1.RunWorkflowRequest]) (*connect.Response[sessionv1.RunWorkflowResponse], error) {
	return replyWith(f, "RunWorkflow", r, &sessionv1.RunWorkflowResponse{SessionId: "run-session"})
}

func (f *recordingSessionRPC) ListApprovalRules(_ context.Context, r *connect.Request[sessionv1.ListApprovalRulesRequest]) (*connect.Response[sessionv1.ListApprovalRulesResponse], error) {
	return replyWith(f, "ListApprovalRules", r, &sessionv1.ListApprovalRulesResponse{})
}

func (f *recordingSessionRPC) UpsertApprovalRule(_ context.Context, r *connect.Request[sessionv1.UpsertApprovalRuleRequest]) (*connect.Response[sessionv1.UpsertApprovalRuleResponse], error) {
	return replyWith(f, "UpsertApprovalRule", r, &sessionv1.UpsertApprovalRuleResponse{})
}

func (f *recordingSessionRPC) DeleteApprovalRule(_ context.Context, r *connect.Request[sessionv1.DeleteApprovalRuleRequest]) (*connect.Response[sessionv1.DeleteApprovalRuleResponse], error) {
	return replyWith(f, "DeleteApprovalRule", r, &sessionv1.DeleteApprovalRuleResponse{})
}

func (f *recordingSessionRPC) ReloadClaudeSettingsRules(_ context.Context, r *connect.Request[sessionv1.ReloadClaudeSettingsRulesRequest]) (*connect.Response[sessionv1.ReloadClaudeSettingsRulesResponse], error) {
	return replyWith(f, "ReloadClaudeSettingsRules", r, &sessionv1.ReloadClaudeSettingsRulesResponse{Success: true, RuleCount: 7, Message: "reloaded"})
}

func (f *recordingSessionRPC) ListTaggingRules(_ context.Context, r *connect.Request[sessionv1.ListTaggingRulesRequest]) (*connect.Response[sessionv1.ListTaggingRulesResponse], error) {
	return replyWith(f, "ListTaggingRules", r, &sessionv1.ListTaggingRulesResponse{})
}

func (f *recordingSessionRPC) UpsertTaggingRule(_ context.Context, r *connect.Request[sessionv1.UpsertTaggingRuleRequest]) (*connect.Response[sessionv1.UpsertTaggingRuleResponse], error) {
	return replyWith(f, "UpsertTaggingRule", r, &sessionv1.UpsertTaggingRuleResponse{})
}

func (f *recordingSessionRPC) DeleteTaggingRule(_ context.Context, r *connect.Request[sessionv1.DeleteTaggingRuleRequest]) (*connect.Response[sessionv1.DeleteTaggingRuleResponse], error) {
	return replyWith(f, "DeleteTaggingRule", r, &sessionv1.DeleteTaggingRuleResponse{})
}

func (f *recordingSessionRPC) GetNotificationHistory(_ context.Context, r *connect.Request[sessionv1.GetNotificationHistoryRequest]) (*connect.Response[sessionv1.GetNotificationHistoryResponse], error) {
	return replyWith(f, "GetNotificationHistory", r, &sessionv1.GetNotificationHistoryResponse{})
}

func (f *recordingSessionRPC) SearchClaudeHistory(_ context.Context, r *connect.Request[sessionv1.SearchClaudeHistoryRequest]) (*connect.Response[sessionv1.SearchClaudeHistoryResponse], error) {
	return replyWith(f, "SearchClaudeHistory", r, &sessionv1.SearchClaudeHistoryResponse{})
}

func resultErrCode(t *testing.T, res *mcpgo.CallToolResult) string {
	t.Helper()
	m := parseResult(t, res)
	require.False(t, m["success"].(bool), "expected an error result, got %+v", m)
	return m["error"].(map[string]interface{})["code"].(string)
}

func liveCreated(t *testing.T) *instance.Instance {
	t.Helper()
	return &instance.Instance{Title: "created-title", UUID: "created-id", Path: t.TempDir(), Status: domain.Active}
}

func lifecycleOver(f *recordingSessionRPC) *lifecycleHandlers {
	return &lifecycleHandlers{store: &stubStore{}, svc: f}
}

func TestCreateSession_should_ReportStillCreatingAndSkipHooks_When_AwaitTimesOut(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.awaitErr = domain.ErrCreationAwaitTimeout

	res, err := lifecycleOver(f).createSessionWithAwaitTimeout(context.Background(), makeToolReq(createSessionArgs(t, "t-timeout", nil)), 7*time.Second)
	require.NoError(t, err)

	m := parseResult(t, res)
	assert.True(t, m["success"].(bool))
	assert.True(t, m["still_creating"].(bool))
	assert.Equal(t, "created-id", f.awaitID, "awaits the id CreateSession returned")
	assert.Equal(t, 7*time.Second, f.awaitTimeout)
	assert.Zero(t, f.injected, "no hook injection before the pipeline reached Active")
	assert.Empty(t, f.findIDs)
}

func TestCreateSession_should_MapCreationOutcomeToErrorCodes_When_AwaitDoesNotSucceed(t *testing.T) {
	tests := []struct {
		name     string
		outcome  domain.CreationOutcome
		awaitErr error
		wantCode string
	}{
		{"failed status", domain.CreationOutcome{Status: domain.Failed, FailureReason: "worktree boom"}, nil, ErrSessionCreationFailed},
		{"vanished", domain.CreationOutcome{}, domain.ErrCreationVanished, ErrSessionCreationCancelled},
		{"caller context ended", domain.CreationOutcome{}, context.Canceled, ErrSessionCreationAwaitCanceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordingSessionRPC()
			f.outcome, f.awaitErr = tc.outcome, tc.awaitErr

			res, err := lifecycleOver(f).createSession(context.Background(), makeToolReq(createSessionArgs(t, "t-"+tc.name, nil)))
			require.NoError(t, err)

			assert.Equal(t, tc.wantCode, resultErrCode(t, res))
			assert.Zero(t, f.injected)
		})
	}
}

func TestCreateSession_should_PassHooksAndRootToTheInjector_When_PipelineReachedActive(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.outcome = domain.CreationOutcome{Status: domain.Active, InstanceID: "created-id"}
	f.live = liveCreated(t)

	args := createSessionArgs(t, "t-hooks", map[string]interface{}{"hooks": []interface{}{"stop_notification", 7, "pre_tool_logging"}})
	res, err := lifecycleOver(f).createSession(context.Background(), makeToolReq(args))
	require.NoError(t, err)

	assert.True(t, parseResult(t, res)["success"].(bool))
	assert.Equal(t, []string{"created-id"}, f.findIDs)
	assert.Equal(t, 1, f.injected)
	assert.Equal(t, f.live.GetEffectiveRootDir(), f.injectRoot)
	assert.Equal(t, "created-title", f.injectTitle)
	assert.Equal(t, []string{"stop_notification", "pre_tool_logging"}, f.injectHooks, "non-string entries are dropped, names pass through verbatim")
	req := f.reqs["CreateSession"].(*sessionv1.CreateSessionRequest)
	assert.Equal(t, "t-hooks", req.Title)
	assert.Contains(t, req.Tags, "source:mcp")
}

func TestCreateSession_should_StillSucceed_When_HookInjectionFails(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.outcome = domain.CreationOutcome{Status: domain.Active}
	f.live = liveCreated(t)
	f.injectErr = errors.New("disk full")

	res, err := lifecycleOver(f).createSession(context.Background(), makeToolReq(createSessionArgs(t, "t-inject-fail", nil)))
	require.NoError(t, err)

	assert.True(t, parseResult(t, res)["success"].(bool), "a failed hook write is logged, not fatal")
	assert.Equal(t, 1, f.injected)
}

func TestCreateSession_should_ReportInternalError_When_ActiveSessionIsNoLongerFindable(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.outcome = domain.CreationOutcome{Status: domain.Active}

	res, err := lifecycleOver(f).createSession(context.Background(), makeToolReq(createSessionArgs(t, "t-gone", nil)))
	require.NoError(t, err)

	assert.Equal(t, ErrInternalError, resultErrCode(t, res))
	assert.Zero(t, f.injected)
}

func TestCreateSession_should_MapCreateRPCError_When_CreateSessionFails(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.errs["CreateSession"] = connect.NewError(connect.CodeInternal, errors.New("boom"))

	res, err := lifecycleOver(f).createSession(context.Background(), makeToolReq(createSessionArgs(t, "t-create-err", nil)))
	require.NoError(t, err)

	assert.Equal(t, ErrInternalError, resultErrCode(t, res))
	assert.Empty(t, f.awaitID, "no await after a failed create")
}

func TestCreateSession_should_WarnOnUnknownProgramOnly_When_ProgramListIsAvailable(t *testing.T) {
	tests := []struct {
		name         string
		programs     []string
		listErr      error
		wantWarnings bool
	}{
		{"unknown program warns", []string{"claude", "aider"}, nil, true},
		{"known program is silent", []string{"claude", "mystery"}, nil, false},
		{"list failure omits the warning", nil, errors.New("config unreadable"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordingSessionRPC()
			f.programs = tc.programs
			if tc.listErr != nil {
				f.errs["ListProgramsConfig"] = tc.listErr
			}
			f.awaitErr = domain.ErrCreationAwaitTimeout

			res, err := lifecycleOver(f).createSession(context.Background(), makeToolReq(createSessionArgs(t, "t-prog-"+tc.name, map[string]interface{}{"program": "mystery"})))
			require.NoError(t, err)

			warning, _ := parseResult(t, res)["program_warning"].(string)
			assert.Equal(t, tc.wantWarnings, warning != "", "warning=%q", warning)
			assert.Equal(t, "mystery", f.reqs["CreateSession"].(*sessionv1.CreateSessionRequest).Program)
		})
	}
}

func TestStopSession_should_RefuseBeforeRemovingFromPollers_When_WorktreeIsShared(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.live = &instance.Instance{Title: "stop-me", UUID: "stop-uuid", Path: t.TempDir(), Status: domain.Paused}
	f.refuseErr = connect.NewError(connect.CodeFailedPrecondition, errors.New("sibling still running"))

	res, err := lifecycleOver(f).stopSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "stop-uuid", "confirm": true}))
	require.NoError(t, err)

	assert.Equal(t, ErrConflict, resultErrCode(t, res))
	assert.Equal(t, []*instance.Instance{f.live}, f.refused)
	assert.Empty(t, f.removedTitles, "a refused stop must leave the session in its pollers")
}

func TestStopSession_should_RemoveFromPollersUnderTheCanonicalTitle_When_Allowed(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.live = &instance.Instance{Title: "stop-me", UUID: "stop-uuid", Path: t.TempDir(), Status: domain.Paused}

	res, err := lifecycleOver(f).stopSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": "stop-uuid", "confirm": true}))
	require.NoError(t, err)

	assert.True(t, parseResult(t, res)["success"].(bool))
	assert.Equal(t, []string{"stop-me"}, f.removedTitles, "pollers are keyed by title, not the UUID the caller passed")
}

func TestCreateSessionForPR_should_ReportStillCreatingAndSkipHooks_When_AwaitTimesOut(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.awaitErr = domain.ErrCreationAwaitTimeout
	gh := &githubHandlers{cache: &fakeUserPRSource{}, store: &stubStore{}, svc: f}

	res, err := gh.createSessionForPR(context.Background(), makeToolReq(createSessionForPRArgs(t, "pr-timeout", 11, nil)))
	require.NoError(t, err)

	assert.True(t, parseResult(t, res)["still_creating"].(bool))
	assert.Zero(t, f.injected)
	req := f.reqs["CreateSession"].(*sessionv1.CreateSessionRequest)
	assert.Equal(t, sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE, req.SessionType)
	assert.Contains(t, req.Tags, "pr:tstapler/stapler-squad#11")
}

func TestCreateSessionForPR_should_InjectOnlyTheDefaultHook_When_PipelineReachedActive(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.outcome = domain.CreationOutcome{Status: domain.Active}
	f.live = liveCreated(t)
	gh := &githubHandlers{cache: &fakeUserPRSource{}, store: &stubStore{}, svc: f}

	res, err := gh.createSessionForPR(context.Background(), makeToolReq(createSessionForPRArgs(t, "pr-active", 12, nil)))
	require.NoError(t, err)

	assert.True(t, parseResult(t, res)["success"].(bool))
	assert.Equal(t, 1, f.injected)
	assert.Nil(t, f.injectHooks, "the PR path requests no extra hooks")
	assert.Equal(t, f.live.GetEffectiveRootDir(), f.injectRoot)
}

func TestCreateSessionForPR_should_ReturnUIURL_When_PublicURLConfigured(t *testing.T) {
	t.Parallel()
	const want = "https://workspace.example.com/?session=created-id"
	tests := []struct {
		name  string
		setup func(f *recordingSessionRPC)
	}{
		{"still creating", func(f *recordingSessionRPC) { f.awaitErr = domain.ErrCreationAwaitTimeout }},
		{"active", func(f *recordingSessionRPC) {
			f.outcome = domain.CreationOutcome{Status: domain.Active}
			f.live = liveCreated(t)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newRecordingSessionRPC()
			tc.setup(f)
			gh := &githubHandlers{cache: &fakeUserPRSource{}, store: &stubStore{}, svc: f, publicURL: "https://workspace.example.com"}

			res, err := gh.createSessionForPR(context.Background(), makeToolReq(createSessionForPRArgs(t, "pr-ui-url", 14, nil)))
			require.NoError(t, err)

			assert.Equal(t, want, parseResult(t, res)["ui_url"])
		})
	}
}

func TestCreateSessionForPR_should_OmitUIURL_When_PublicURLUnset(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.awaitErr = domain.ErrCreationAwaitTimeout
	gh := &githubHandlers{cache: &fakeUserPRSource{}, store: &stubStore{}, svc: f}

	res, err := gh.createSessionForPR(context.Background(), makeToolReq(createSessionForPRArgs(t, "pr-no-ui-url", 15, nil)))
	require.NoError(t, err)

	assert.NotContains(t, parseResult(t, res), "ui_url")
}

func TestCreateSessionForPR_should_MapOutcomeErrors_When_AwaitDoesNotSucceed(t *testing.T) {
	t.Parallel()
	f := newRecordingSessionRPC()
	f.awaitErr = domain.ErrCreationVanished
	gh := &githubHandlers{cache: &fakeUserPRSource{}, store: &stubStore{}, svc: f}

	res, err := gh.createSessionForPR(context.Background(), makeToolReq(createSessionForPRArgs(t, "pr-vanished", 13, nil)))
	require.NoError(t, err)

	assert.Equal(t, ErrSessionCreationCancelled, resultErrCode(t, res))
}

func TestWorkflowTools_should_MapRPCErrorsAndPassArguments_When_ServiceReplies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newRecordingSessionRPC()
	h := &workflowHandlers{svc: f}

	res, err := h.createWorkflow(ctx, makeToolReq(map[string]interface{}{"slug": "s", "name": "n", "command": "c"}))
	require.NoError(t, err)
	assert.True(t, parseResult(t, res)["success"].(bool))
	created := f.reqs["CreateWorkflow"].(*sessionv1.CreateWorkflowRequest)
	assert.Equal(t, []string{"s", "n", "c", "directory"}, []string{created.Slug, created.Name, created.Command, created.SessionType})

	res, err = h.runWorkflow(ctx, makeToolReq(map[string]interface{}{"id": "wf-1", "arg": "x"}))
	require.NoError(t, err)
	assert.Equal(t, "run-session", parseResult(t, res)["session_id"])
	assert.Equal(t, "x", f.reqs["RunWorkflow"].(*sessionv1.RunWorkflowRequest).Arg)

	calls := map[string]func() (*mcpgo.CallToolResult, error){
		"CreateWorkflow": func() (*mcpgo.CallToolResult, error) {
			return h.createWorkflow(ctx, makeToolReq(map[string]interface{}{"slug": "s", "name": "n", "command": "c"}))
		},
		"UpdateWorkflow": func() (*mcpgo.CallToolResult, error) {
			return h.updateWorkflow(ctx, makeToolReq(map[string]interface{}{"id": "wf-1"}))
		},
		"DeleteWorkflow": func() (*mcpgo.CallToolResult, error) {
			return h.deleteWorkflow(ctx, makeToolReq(map[string]interface{}{"id": "wf-1"}))
		},
		"ListWorkflows": func() (*mcpgo.CallToolResult, error) { return h.listWorkflows(ctx, makeToolReq(nil)) },
		"RunWorkflow": func() (*mcpgo.CallToolResult, error) {
			return h.runWorkflow(ctx, makeToolReq(map[string]interface{}{"id": "wf-1"}))
		},
	}
	assertRPCErrorMapping(t, f, calls)
}

func TestApprovalRuleTools_should_MapRPCErrors_When_ServiceFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newRecordingSessionRPC()
	h := &rulesHandlers{svc: f}

	res, err := h.listApprovalRules(ctx, makeToolReq(map[string]interface{}{"source_filter": "user"}))
	require.NoError(t, err)
	assert.True(t, parseResult(t, res)["success"].(bool))
	assert.Equal(t, "user", *f.reqs["ListApprovalRules"].(*sessionv1.ListApprovalRulesRequest).SourceFilter)

	res, err = h.reloadClaudeSettingsRules(ctx, makeToolReq(nil))
	require.NoError(t, err)
	m := parseResult(t, res)
	assert.Equal(t, float64(7), m["rule_count"])
	assert.Equal(t, "reloaded", m["message"])

	calls := map[string]func() (*mcpgo.CallToolResult, error){
		"ListApprovalRules": func() (*mcpgo.CallToolResult, error) { return h.listApprovalRules(ctx, makeToolReq(nil)) },
		"UpsertApprovalRule": func() (*mcpgo.CallToolResult, error) {
			return h.upsertApprovalRule(ctx, makeToolReq(map[string]interface{}{"name": "r"}))
		},
		"DeleteApprovalRule": func() (*mcpgo.CallToolResult, error) {
			return h.deleteApprovalRule(ctx, makeToolReq(map[string]interface{}{"id": "r1"}))
		},
		"ReloadClaudeSettingsRules": func() (*mcpgo.CallToolResult, error) { return h.reloadClaudeSettingsRules(ctx, makeToolReq(nil)) },
	}
	assertRPCErrorMapping(t, f, calls)
}

func TestTaggingRuleTools_should_MapRPCErrors_When_ServiceFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newRecordingSessionRPC()
	h := &taggingRulesHandlers{svc: f}

	calls := map[string]func() (*mcpgo.CallToolResult, error){
		"ListTaggingRules": func() (*mcpgo.CallToolResult, error) { return h.listTaggingRules(ctx, makeToolReq(nil)) },
		"UpsertTaggingRule": func() (*mcpgo.CallToolResult, error) {
			return h.upsertTaggingRule(ctx, makeToolReq(map[string]interface{}{"name": "r", "output_tag": "t", "program_pattern": "claude"}))
		},
		"DeleteTaggingRule": func() (*mcpgo.CallToolResult, error) {
			return h.deleteTaggingRule(ctx, makeToolReq(map[string]interface{}{"id": "r1"}))
		},
	}
	assertRPCErrorMapping(t, f, calls)
}

// assertRPCErrorMapping runs every call once with the service failing with a connect NotFound
// and once with a plain error, pinning that each handler reached its RPC and maps both.
func assertRPCErrorMapping(t *testing.T, f *recordingSessionRPC, calls map[string]func() (*mcpgo.CallToolResult, error)) {
	t.Helper()
	for rpc, call := range calls {
		t.Run(rpc, func(t *testing.T) {
			f.errs[rpc] = connect.NewError(connect.CodeNotFound, errors.New("nope"))
			res, err := call()
			require.NoError(t, err)
			assert.Equal(t, ErrItemNotFound, resultErrCode(t, res), "connect NotFound maps to ITEM_NOT_FOUND")

			f.errs[rpc] = errors.New("kaboom")
			res, err = call()
			require.NoError(t, err)
			assert.Equal(t, ErrInternalError, resultErrCode(t, res), "a plain error maps to INTERNAL_ERROR")
			delete(f.errs, rpc)
		})
	}
}

func TestNotificationHistoryTool_should_PassFiltersAndMapErrors_When_ServiceReplies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newRecordingSessionRPC()
	h := &notificationHandlers{svc: f}

	res, err := h.getNotificationHistory(ctx, makeToolReq(map[string]interface{}{"limit": float64(500), "offset": float64(3), "session_id": "s1", "unread_only": true}))
	require.NoError(t, err)
	assert.True(t, parseResult(t, res)["success"].(bool))
	req := f.reqs["GetNotificationHistory"].(*sessionv1.GetNotificationHistoryRequest)
	assert.Equal(t, int32(50), *req.Limit, "limit is capped at 50")
	assert.Equal(t, int32(3), *req.Offset)
	assert.Equal(t, "s1", *req.SessionId)
	assert.True(t, *req.UnreadOnly)

	f.errs["GetNotificationHistory"] = errors.New("store closed")
	res, err = h.getNotificationHistory(ctx, makeToolReq(nil))
	require.NoError(t, err)
	assert.Equal(t, ErrInternalError, resultErrCode(t, res))
}

func TestClaudeHistoryTool_should_PassQueryAndMapErrors_When_ServiceReplies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newRecordingSessionRPC()
	h := &historyHandlers{svc: f}

	res, err := h.searchClaudeHistory(ctx, makeToolReq(map[string]interface{}{"query": "needle", "limit": float64(1000), "offset": float64(4)}))
	require.NoError(t, err)
	assert.True(t, parseResult(t, res)["success"].(bool))
	req := f.reqs["SearchClaudeHistory"].(*sessionv1.SearchClaudeHistoryRequest)
	assert.Equal(t, []any{"needle", int32(100), int32(4)}, []any{req.Query, req.Limit, req.Offset}, "limit is capped at 100")

	f.errs["SearchClaudeHistory"] = errors.New("index corrupt")
	res, err = h.searchClaudeHistory(ctx, makeToolReq(map[string]interface{}{"query": "needle"}))
	require.NoError(t, err)
	assert.Equal(t, ErrInternalError, resultErrCode(t, res))
}

func TestToolModules_should_BeAbsent_When_NoSessionServiceIsWired(t *testing.T) {
	t.Parallel()
	var none SessionRPC
	assert.Nil(t, newWorkflowModule(none))
	assert.Nil(t, newRulesModule(none))
	assert.Nil(t, newTaggingRulesModule(none))
	assert.Nil(t, newNotificationsModule(none))
	assert.Nil(t, newHistoryModule(none))
	assert.NotNil(t, newWorkflowModule(newRecordingSessionRPC()))
}
