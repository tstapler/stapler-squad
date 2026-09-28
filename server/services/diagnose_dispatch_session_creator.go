package services

// diagnose_dispatch_session_creator.go — Phase 7's concrete
// HeadlessDiagnosticSessionCreator implementation (DiagnoseDispatcherDeps.SessionCreator),
// closing the gap Phase 5 deliberately left open (see
// HeadlessDiagnosticSessionCreator's doc comment in diagnose_dispatcher.go).
//
// Narrowed scope, documented here: TriggerTriage
// (backlog_service_trigger_triage.go) creates an isolated git worktree before
// running its headless call, because triage produces planning docs meant to
// travel with the eventual PR. A diagnose dispatch does neither -- the
// diagnostic agent's job is read-mostly investigation plus MCP tool calls
// back into this same server (create_backlog_item/post_backlog_update/nudge,
// wired in server/mcp/diagnose_gate_wiring.go), not committing code -- so
// this implementation runs the dispatched call directly against
// item.RepoPath, the same no-worktree-isolation shape
// headlessTriageUUIDPrefix's own doc comment describes for "bounded one-shot
// headless subprocess calls" (see session.IsTmuxBackedSessionRole). If a
// future diagnose outcome needs its own git changes, add worktree isolation
// then -- this is a deliberate narrowing, not an oversight.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// diagnoseHeadlessCaller is the narrow headless-pool surface
// DiagnoseDispatchSessionCreator needs -- satisfied by *headless.Pool.
// Declared locally so tests can inject a fake without a real Pool/claude
// binary, mirroring session/gate_custom_check.go's identical convention.
type diagnoseHeadlessCaller interface {
	CallBlocking(ctx context.Context, key headless.FeatureKey, systemPrompt, userPrompt string, opts headless.CallOptions, sink headless.CostSink) (string, error)
}

// diagnoseSessionPersistence is the narrow storage surface
// DiagnoseDispatchSessionCreator needs to record/close the synthetic
// ItemSession row for a dispatched diagnostic session -- satisfied by
// *session.Storage.
type diagnoseSessionPersistence interface {
	CreateItemSession(ctx context.Context, data session.ItemSessionData) (session.ItemSessionSummary, error)
	UpdateItemSessionEndedWithReason(ctx context.Context, id string, endedAt time.Time, reason string) error
}

// diagnoseDispatchCallBudget bounds how long the dispatched diagnostic
// agent's own headless call may run before it's cut off -- mirrors
// triageCallBudget's role for TriggerTriage's headless call
// (backlog_service_trigger_triage.go), sized generously since diagnosis
// includes reading a whole bundle plus MCP tool calls.
const diagnoseDispatchCallBudget = 15 * time.Minute

// diagnoseDispatchMCPServerName is this project's own MCP server name, as
// registered by session.InstanceOptions.MCPServerURL's --mcp-config key
// (session/instance.go: `--mcp-config '{"stapler-squad":...}'`) -- the
// diagnostic agent's MCP tools are exposed to the model as
// "mcp__<name>__<tool>" (session/tokens/doc.go documents this naming).
const diagnoseDispatchMCPServerName = "stapler-squad"

// diagnoseDispatchAllowedTools restricts the dispatched diagnostic agent
// (runDiagnosticCall) to exactly the tool surface session/diagnose/prompt.go's
// instructionBlock() documents to the model: file a bug, post a note, or
// nudge the target session. Nothing else -- notably NOT run_command
// (server/mcp/tools_terminal.go), which writes directly into a session's tmux
// pane via session.SubmitContentWithEnter with NO NudgeGate check, NO
// identity reverification, and NO cap/cooldown, completely bypassing
// ADR-002/ADR-003. Before this allowlist, an unrestricted headless call
// granted the dispatched agent every registered MCP tool, including that one.
//
// TODO(backlog): this is a client-side (--allowedTools) mitigation only.
// CLI-level tool restriction is not a hard security boundary under
// bypassPermissions-style permission modes -- see
// headless.CodebaseReadAllowedTools's doc comment and the ADR-001 addendum it
// cites, which empirically proved an unlisted Bash command still executes
// under that mode. The stronger, still-needed follow-up is SERVER-side
// enforcement: route run_command (server/mcp/tools_terminal.go) through the
// same evaluateNudgeGate/checkDuplicateWriteGuard/verifyNudgeIdentity
// pipeline steer_session/write_to_session/resume_session already use, or
// reject it outright when the calling session is a headless-diagnose
// dispatch.
var diagnoseDispatchAllowedTools = strings.Join([]string{
	mcpToolName("create_backlog_item"),
	mcpToolName("post_backlog_update"),
	mcpToolName("resume_session"),
	mcpToolName("steer_session"),
	mcpToolName("write_to_session"),
}, ",")

// mcpToolName renders name as this server's own MCP tool identifier, in the
// "mcp__<server>__<tool>" form the claude CLI's --allowedTools/--disallowedTools
// flags expect for MCP-exposed tools (as opposed to a built-in tool like
// "Read").
func mcpToolName(name string) string {
	return "mcp__" + diagnoseDispatchMCPServerName + "__" + name
}

// DiagnoseDispatchSessionCreator is the production
// HeadlessDiagnosticSessionCreator implementation: records a synthetic,
// non-tmux-backed ItemSession row (SessionRoleDiagnose), then runs the
// diagnostic prompt through the shared headless pool -- reusing the same
// headlessPool + storage.CreateItemSession primitives TriggerTriage already
// established for headless dispatch, per this file's package doc comment for
// what's intentionally NOT reused (worktree isolation).
type DiagnoseDispatchSessionCreator struct {
	pool    diagnoseHeadlessCaller
	storage diagnoseSessionPersistence
}

// NewDiagnoseDispatchSessionCreator constructs a DiagnoseDispatchSessionCreator.
func NewDiagnoseDispatchSessionCreator(pool diagnoseHeadlessCaller, storage diagnoseSessionPersistence) *DiagnoseDispatchSessionCreator {
	return &DiagnoseDispatchSessionCreator{pool: pool, storage: storage}
}

// CreateHeadlessDiagnosticSession implements HeadlessDiagnosticSessionCreator.
// It synchronously records the ItemSession row (so it's durably visible
// before this call returns) and, on success, dispatches the actual headless
// call in a detached goroutine, returning nil immediately: the caller
// (DiagnoseDispatcher.dispatchAndRecord) only wants to know whether dispatch
// itself failed synchronously, not wait for the (potentially many-minute)
// diagnostic investigation to finish.
//
// The goroutine's own outcome (Nudged/BugFiled/InconclusiveNoteFiled/
// SkippedSafetyGate) is recorded elsewhere, by the diagnostic agent's own MCP
// tool calls (see diagnose_dispatcher.go's notifyDiagnoseEvent doc comment);
// this method only ends the ItemSession row once the call returns, and -- if
// the call itself errored or never made a terminal tool call at all --
// leaves the DiagnoseDispatch row for Story 6.1.5's stale-dispatch sweeper
// (SupersededSessionSweeper) to eventually mark Stalled.
func (c *DiagnoseDispatchSessionCreator) CreateHeadlessDiagnosticSession(ctx context.Context, req HeadlessDiagnosticSessionRequest) error {
	if c.pool == nil {
		return fmt.Errorf("diagnose dispatch session creator: headless pool not available")
	}

	is, err := c.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      req.ItemID,
		SessionUUID: req.DiagnosticSessionUUID,
		SessionRole: session.SessionRoleDiagnose,
	})
	if err != nil {
		return fmt.Errorf("diagnose dispatch session creator: create item session for item %s: %w", req.ItemID, err)
	}

	// Detached: ConnectRPC cancels the request-scoped ctx the instant
	// RequestDiagnosis returns, but the diagnostic call must keep running
	// after that -- mirrors TriggerHandoffSummary's identical dispatch shape
	// (handoff_summary_service.go).
	go c.runDiagnosticCall(context.Background(), req, is.ID)

	return nil
}

// runDiagnosticCall runs the dispatched diagnostic prompt and ends the
// ItemSession row when it returns, regardless of outcome.
func (c *DiagnoseDispatchSessionCreator) runDiagnosticCall(ctx context.Context, req HeadlessDiagnosticSessionRequest, itemSessionID string) {
	callCtx, cancel := context.WithTimeout(ctx, diagnoseDispatchCallBudget)
	defer cancel()

	endReason := "completed"
	opts := headless.CallOptions{WorkDir: req.RepoPath, AllowedTools: diagnoseDispatchAllowedTools}
	if _, err := c.pool.CallBlocking(callCtx, headless.FeatureKeyCustom, "", req.Prompt, opts, headless.DiscardCost); err != nil {
		log.Warn("[DiagnoseDispatchSessionCreator] diagnostic call failed", "item", req.ItemID, "session", req.DiagnosticSessionUUID, "error", err)
		endReason = "diagnostic_call_failed"
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	if endErr := c.storage.UpdateItemSessionEndedWithReason(cleanupCtx, itemSessionID, time.Now(), endReason); endErr != nil {
		log.Warn("[DiagnoseDispatchSessionCreator] failed to mark diagnostic item session ended", "item", req.ItemID, "item_session", itemSessionID, "error", endErr)
	}
}
