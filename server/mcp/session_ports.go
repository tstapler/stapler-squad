package mcp

import (
	"context"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Consumer-side ports over the session service, one per handler family. Each has
// two implementations: the composition root's adapter over *services.SessionService
// (server/mcp_ports_wiring.go) and a recording fake that reaches branches the real
// service cannot (an await timeout, a vanished creation, an RPC error).

// ProgramLister lists the configured programs, for the create tools' program enum
// and unknown-program warning.
type ProgramLister interface {
	ListProgramsConfig(context.Context, *connect.Request[sessionv1.ListProgramsConfigRequest]) (*connect.Response[sessionv1.ListProgramsConfigResponse], error)
}

// SessionCreator is what create_session and create_session_for_pr call: create,
// wait for the creation pipeline, re-find the live instance and inject hooks.
// InjectHooksConfig takes hook names as strings; the adapter converts them.
type SessionCreator interface {
	ProgramLister
	CreateSession(context.Context, *connect.Request[sessionv1.CreateSessionRequest]) (*connect.Response[sessionv1.CreateSessionResponse], error)
	AwaitCreationTerminal(ctx context.Context, instanceID string, timeout time.Duration) (domain.CreationOutcome, error)
	FindLiveInstance(id string) *instance.Instance
	InjectHooksConfig(rootDir, sessionTitle string, hooks []string) error
}

// SessionLifecycle is SessionCreator plus the pause, resume and stop tools' calls.
type SessionLifecycle interface {
	SessionCreator
	RefuseIfWorktreeSharedWithOtherLiveSession(inst *instance.Instance) error
	RemoveFromAllPollers(sessionTitle string)
	RefreshInstanceHookProof(inst *instance.Instance)
}

// WorkflowRPC is the workflow tools' slice of the session service.
type WorkflowRPC interface {
	CreateWorkflow(context.Context, *connect.Request[sessionv1.CreateWorkflowRequest]) (*connect.Response[sessionv1.CreateWorkflowResponse], error)
	UpdateWorkflow(context.Context, *connect.Request[sessionv1.UpdateWorkflowRequest]) (*connect.Response[sessionv1.UpdateWorkflowResponse], error)
	DeleteWorkflow(context.Context, *connect.Request[sessionv1.DeleteWorkflowRequest]) (*connect.Response[sessionv1.DeleteWorkflowResponse], error)
	ListWorkflows(context.Context, *connect.Request[sessionv1.ListWorkflowsRequest]) (*connect.Response[sessionv1.ListWorkflowsResponse], error)
	RunWorkflow(context.Context, *connect.Request[sessionv1.RunWorkflowRequest]) (*connect.Response[sessionv1.RunWorkflowResponse], error)
}

// ApprovalRulesRPC is the approval-rule tools' slice of the session service.
type ApprovalRulesRPC interface {
	ListApprovalRules(context.Context, *connect.Request[sessionv1.ListApprovalRulesRequest]) (*connect.Response[sessionv1.ListApprovalRulesResponse], error)
	UpsertApprovalRule(context.Context, *connect.Request[sessionv1.UpsertApprovalRuleRequest]) (*connect.Response[sessionv1.UpsertApprovalRuleResponse], error)
	DeleteApprovalRule(context.Context, *connect.Request[sessionv1.DeleteApprovalRuleRequest]) (*connect.Response[sessionv1.DeleteApprovalRuleResponse], error)
	ReloadClaudeSettingsRules(context.Context, *connect.Request[sessionv1.ReloadClaudeSettingsRulesRequest]) (*connect.Response[sessionv1.ReloadClaudeSettingsRulesResponse], error)
}

// TaggingRulesRPC is the tagging-rule tools' slice of the session service.
type TaggingRulesRPC interface {
	ListTaggingRules(context.Context, *connect.Request[sessionv1.ListTaggingRulesRequest]) (*connect.Response[sessionv1.ListTaggingRulesResponse], error)
	UpsertTaggingRule(context.Context, *connect.Request[sessionv1.UpsertTaggingRuleRequest]) (*connect.Response[sessionv1.UpsertTaggingRuleResponse], error)
	DeleteTaggingRule(context.Context, *connect.Request[sessionv1.DeleteTaggingRuleRequest]) (*connect.Response[sessionv1.DeleteTaggingRuleResponse], error)
}

// NotificationHistoryRPC is the get_notification_history tool's slice of the session service.
type NotificationHistoryRPC interface {
	GetNotificationHistory(context.Context, *connect.Request[sessionv1.GetNotificationHistoryRequest]) (*connect.Response[sessionv1.GetNotificationHistoryResponse], error)
}

// ClaudeHistoryRPC is the search_claude_history tool's slice of the session service.
type ClaudeHistoryRPC interface {
	SearchClaudeHistory(context.Context, *connect.Request[sessionv1.SearchClaudeHistoryRequest]) (*connect.Response[sessionv1.SearchClaudeHistoryResponse], error)
}

// SessionRPC is the one value the composition root hands NewCore: every
// per-family port above plus the review signalers. Handlers keep only their own
// family's port, so no handler depends on this whole set.
type SessionRPC interface {
	SessionLifecycle
	WorkflowRPC
	ApprovalRulesRPC
	TaggingRulesRPC
	NotificationHistoryRPC
	ClaudeHistoryRPC
	ReviewCompletionSignaler
	ReviewTrigger
}
