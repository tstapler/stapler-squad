package mcp

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// diagnoseHandlers implements the Diagnose & Nudge MCP tools (backlog item
// 68964304): submit_diagnosis_result and diagnose_nudge_session. live may be
// nil (e.g. the stdio fallback path with no *services.SessionService) —
// diagnose_nudge_session then always refuses with SESSION_NOT_FOUND rather
// than panicking. eventBus is optional — nil means submit_diagnosis_result's
// outcome notification (see notifyDiagnosisResult) is skipped, matching every
// other optional-eventBus handler's nil-degrades-to-off convention.
type diagnoseHandlers struct {
	storage  *session.Storage
	live     liveInstanceFinder
	eventBus *events.EventBus
}

// diagnoseOutcomeValues are the valid submit_diagnosis_result outcomes,
// matching buildDiagnosePrompt's three-way action space (server/services/
// diagnostic_service.go): a genuine defect, an inconclusive/informational
// note, a successful nudge, or (added for completeness) no action needed at
// all.
var diagnoseOutcomeValues = []string{"BUG_FILED", "NOTE_ONLY", "NUDGED", "NO_ACTION"}

func registerDiagnoseTools(s *mcpserver.MCPServer, dh *diagnoseHandlers) {
	registerSubmitDiagnosisResultTool(s, dh)
	registerDiagnoseNudgeSessionTool(s, dh)
}

func registerSubmitDiagnosisResultTool(s *mcpserver.MCPServer, dh *diagnoseHandlers) {
	s.AddTool(
		mcpgo.NewTool("submit_diagnosis_result",
			mcpgo.WithDescription("Submit the outcome of a Diagnose & Nudge investigation. Role: diagnose only. Call exactly once, after concluding your investigation — after filing a bug (create_backlog_item), posting a note (post_backlog_update), or nudging (diagnose_nudge_session)."),
			mcpgo.WithString("item_id",
				mcpgo.Description("UUID of the backlog item this diagnostic session was dispatched for"),
				mcpgo.Required(),
			),
			mcpgo.WithString("outcome",
				mcpgo.Description("What action you took"),
				mcpgo.Enum(diagnoseOutcomeValues...),
				mcpgo.Required(),
			),
			mcpgo.WithString("summary",
				mcpgo.Description("What you found and what you did about it (max 2000 chars)"),
				mcpgo.Required(),
			),
		),
		dh.submitDiagnosisResult,
	)
}

func registerDiagnoseNudgeSessionTool(s *mcpserver.MCPServer, dh *diagnoseHandlers) {
	s.AddTool(
		mcpgo.NewTool("diagnose_nudge_session",
			mcpgo.WithDescription("Send a redirect message to a linked session that Diagnose & Nudge dispatch identified as a nudge candidate. Role: diagnose only. Refuses (does not error loudly, just declines) if the target is no longer idle or its tmux pane identity can't be re-verified immediately before the write — a refusal means file a bug or post a note instead via submit_diagnosis_result, do not retry this call."),
			mcpgo.WithString("item_id",
				mcpgo.Description("UUID of the backlog item"),
				mcpgo.Required(),
			),
			mcpgo.WithString("session_id",
				mcpgo.Description("UUID of the target session to nudge"),
				mcpgo.Required(),
			),
			mcpgo.WithString("stuck_reason",
				mcpgo.Description("The domain.StuckReason value this nudge is being made for — used to charge the item's nudge cap/cooldown"),
				mcpgo.Required(),
			),
			mcpgo.WithString("message",
				mcpgo.Description("The redirect/nudge message to send to the target session"),
				mcpgo.Required(),
			),
		),
		dh.nudgeSession,
	)
}

// resolveDiagnoseItemLink is resolveItemLink's diagnose-role counterpart:
// backlogHandlers.resolveItemLink is a method on a different struct, so this
// is a small free-function duplicate calling the same
// session.ResolveItemLink — mirrors submitReviewVerdict's own
// resolveItemLink-then-role-check shape for SessionRoleReview.
func resolveDiagnoseItemLink(ctx context.Context, storage *session.Storage, callerUUID, itemID string) (session.ItemSessionSummary, *mcpgo.CallToolResult) {
	itemSession, linkErr := session.ResolveItemLink(ctx, storage, callerUUID, itemID)
	if linkErr == nil {
		if itemSession.Role != session.SessionRoleDiagnose {
			return session.ItemSessionSummary{}, errResult(ErrPermissionDenied, fmt.Sprintf("session role is %q — only %q may call this tool", itemSession.Role, session.SessionRoleDiagnose), "")
		}
		return itemSession, nil
	}
	switch linkErr.Code {
	case session.ItemLinkNotFound:
		return session.ItemSessionSummary{}, errResult(ErrItemNotFound, linkErr.Message, linkErr.Remediation)
	case session.ItemLinkPermissionDenied:
		return session.ItemSessionSummary{}, errResult(ErrPermissionDenied, linkErr.Message, linkErr.Remediation)
	default:
		return session.ItemSessionSummary{}, errResult(ErrInternalError, linkErr.Message, "")
	}
}

func (dh *diagnoseHandlers) submitDiagnosisResult(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	callerUUID, err := callerSessionUUID(ctx)
	if err != nil {
		return errResult(ErrPermissionDenied, err.Error(), "Set STAPLER_SESSION_UUID in your environment."), nil
	}
	args := req.GetArguments()

	itemID, ok := args["item_id"].(string)
	if !ok || itemID == "" {
		return errResult(ErrInvalidArgument, "item_id is required", ""), nil
	}
	if err := validateUUID(itemID); err != nil {
		return errResult(ErrInvalidArgument, err.Error(), ""), nil
	}
	outcome, ok := args["outcome"].(string)
	if !ok || outcome == "" {
		return errResult(ErrInvalidArgument, "outcome is required", ""), nil
	}
	summary, ok := args["summary"].(string)
	if !ok || summary == "" {
		return errResult(ErrInvalidArgument, "summary is required", ""), nil
	}

	if _, errRes := resolveDiagnoseItemLink(ctx, dh.storage, callerUUID, itemID); errRes != nil {
		return errRes, nil
	}

	note := fmt.Sprintf("Diagnosis result: %s\n\n%s", outcome, summary)
	if noteErr := dh.storage.AppendActivityNote(ctx, itemID, callerUUID, "diagnose", note); noteErr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("record diagnosis result: %v", noteErr), ""), nil
	}
	dh.notifyDiagnosisResult(itemID, outcome, summary)

	return mcpgo.NewToolResultText(fmt.Sprintf("Diagnosis result recorded for item %s: %s", itemID, outcome)), nil
}

// diagnosisOutcomeTitles gives each diagnoseOutcomeValues entry an
// operator-facing title, keyed the same as the note AppendActivityNote
// records, so the notification and the activity log agree on what happened.
var diagnosisOutcomeTitles = map[string]string{
	"BUG_FILED": "Diagnosis: bug filed",
	"NOTE_ONLY": "Diagnosis: note added",
	"NUDGED":    "Diagnosis: session nudged",
	"NO_ACTION": "Diagnosis: no action needed",
}

// notifyDiagnosisResult publishes an operator-facing notification for a
// completed Diagnose & Nudge dispatch, carrying the actual outcome/summary
// and linking to the backlog item (via metadata's item_id) rather than the
// hidden, one-shot diagnostic session itself — that session is excluded from
// the default session list, so a notification that only links to it is a
// dead end. Without this, the only signal a completed diagnose dispatch ever
// produced was the generic "Session Completed" push built from the session's
// raw internal title (e.g. "diagnose:e32264b0:..."), which carries no
// information about what was found. Best-effort: nil eventBus is a no-op.
func (dh *diagnoseHandlers) notifyDiagnosisResult(itemID, outcome, summary string) {
	if dh.eventBus == nil {
		return
	}
	title := diagnosisOutcomeTitles[outcome]
	if title == "" {
		title = "Diagnosis complete"
	}
	// BUG_FILED added new backlog work the operator didn't ask for and may
	// want to triage soon; the other outcomes are routine background noise.
	priority := sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW
	if outcome == "BUG_FILED" {
		priority = sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
	}
	dh.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO),
		int32(priority),
		title,
		summary,
		map[string]string{"item_id": itemID},
	))
}

func (dh *diagnoseHandlers) nudgeSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	callerUUID, err := callerSessionUUID(ctx)
	if err != nil {
		return errResult(ErrPermissionDenied, err.Error(), "Set STAPLER_SESSION_UUID in your environment."), nil
	}

	nudgeArgs, errRes := parseNudgeSessionArgs(req)
	if errRes != nil {
		return errRes, nil
	}
	if _, errRes := resolveDiagnoseItemLink(ctx, dh.storage, callerUUID, nudgeArgs.itemID); errRes != nil {
		return errRes, nil
	}
	return dh.performNudge(ctx, callerUUID, nudgeArgs)
}

// nudgeSessionArgs is submit_diagnosis_result's sibling tool's parsed input —
// bundled into a struct (primitive-obsession-checklist) since all four
// fields are threaded together through parsing, the gate checks, and the
// write.
type nudgeSessionArgs struct {
	itemID          string
	targetSessionID string
	stuckReason     string
	message         string
}

func parseNudgeSessionArgs(req mcpgo.CallToolRequest) (nudgeSessionArgs, *mcpgo.CallToolResult) {
	args := req.GetArguments()

	itemID, ok := args["item_id"].(string)
	if !ok || itemID == "" {
		return nudgeSessionArgs{}, errResult(ErrInvalidArgument, "item_id is required", "")
	}
	if err := validateUUID(itemID); err != nil {
		return nudgeSessionArgs{}, errResult(ErrInvalidArgument, err.Error(), "")
	}
	targetSessionID, ok := args["session_id"].(string)
	if !ok || targetSessionID == "" {
		return nudgeSessionArgs{}, errResult(ErrInvalidArgument, "session_id is required", "")
	}
	stuckReason, ok := args["stuck_reason"].(string)
	if !ok || stuckReason == "" {
		return nudgeSessionArgs{}, errResult(ErrInvalidArgument, "stuck_reason is required", "")
	}
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return nudgeSessionArgs{}, errResult(ErrInvalidArgument, "message is required", "")
	}
	return nudgeSessionArgs{itemID: itemID, targetSessionID: targetSessionID, stuckReason: stuckReason, message: message}, nil
}

// performNudge applies AC4's cap/cooldown gate, AC2's idle+identity
// write-time gate, and the write itself, then records the attempt. Split out
// of nudgeSession purely to stay under the funlen gate.
const declineToDiagnoseRemediation = "File a bug via create_backlog_item or post a note via submit_diagnosis_result instead."

// checkNudgeCapForWrite is AC4's cap/cooldown re-check, extracted from
// performNudge purely to stay under the funlen gate. Re-checked here, not
// just trusted from the dispatch prompt — a long-running diagnostic session
// could otherwise call this tool after its own cap-eligibility snapshot went
// stale (e.g. another dispatch nudged the same item in the meantime).
func (dh *diagnoseHandlers) checkNudgeCapForWrite(ctx context.Context, itemID, stuckReason string) *mcpgo.CallToolResult {
	allowed, disallowReason, gateErr := dh.storage.DiagnoseNudgeAllowed(ctx, itemID, domain.StuckReason(stuckReason))
	if gateErr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("check nudge eligibility: %v", gateErr), "")
	}
	if !allowed {
		return errResult(ErrPermissionDenied, disallowReason, declineToDiagnoseRemediation)
	}
	return nil
}

// diagnoseNudgeDisabledMessage is returned when the diagnose_nudge_enabled
// feature flag is off — see config.EffectiveDiagnoseNudgeEnabled's doc
// comment for why this is checked here, at the write call site, rather than
// only at dispatch time.
const diagnoseNudgeDisabledMessage = "autonomous nudging is currently disabled (diagnose_nudge_enabled feature flag is off)"

// diagnoseNudgeAlreadyAttemptedMessage is returned when this diagnostic
// dispatch has already claimed its one nudge-write attempt — see
// session.Storage.ClaimDiagnoseNudgeAttempt's doc comment. Distinct from the
// item-level cap/cooldown (checkNudgeCapForWrite): that bounds total nudges
// per item across many dispatches, this stops a SINGLE dispatch from writing
// twice if its own LLM retries after an ambiguous MCP tool response.
const diagnoseNudgeAlreadyAttemptedMessage = "this diagnostic session has already attempted a nudge write"

// claimNudgeAttemptForWrite is the duplicate-write guard, extracted from
// performNudge purely to stay under the funlen gate: claims callerUUID's one
// nudge attempt atomically, immediately before the write itself. A failed
// claim means this exact dispatched session already attempted a nudge —
// refuse regardless of remaining cap headroom, rather than let an ambiguous
// MCP response make its LLM retry the write.
func (dh *diagnoseHandlers) claimNudgeAttemptForWrite(ctx context.Context, callerUUID string) *mcpgo.CallToolResult {
	claimed, claimErr := dh.storage.ClaimDiagnoseNudgeAttempt(ctx, callerUUID)
	if claimErr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("check duplicate-write guard: %v", claimErr), "")
	}
	if !claimed {
		return errResult(ErrPermissionDenied, diagnoseNudgeAlreadyAttemptedMessage, declineToDiagnoseRemediation)
	}
	return nil
}

// resolveNudgeTarget finds and idle/pane-ownership-verifies a's target
// instance — extracted from performNudge purely to stay under the funlen
// gate. See session.VerifyNudgeSafeToWrite's doc comment for why the
// write-time pane-ownership re-verification can't be skipped even though
// DiagnoseNudgeAllowed/dispatch-time eligibility already looked idle (AC2's
// two-part write-time gate).
func (dh *diagnoseHandlers) resolveNudgeTarget(ctx context.Context, a nudgeSessionArgs) (*session.Instance, *mcpgo.CallToolResult) {
	if dh.live == nil {
		return nil, errResult(ErrSessionNotFound, "no live session lookup available", "")
	}
	target := dh.live.FindLiveInstance(a.targetSessionID)
	if target == nil {
		return nil, errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found or not live", a.targetSessionID), "")
	}
	if verifyErr := session.VerifyNudgeSafeToWrite(ctx, target); verifyErr != nil {
		return nil, errResult(ErrPermissionDenied, verifyErr.Error(), declineToDiagnoseRemediation)
	}
	return target, nil
}

// writeNudge takes the target's write lease BEFORE claiming the dispatch's one
// nudge attempt, so a busy lease (a driver key, a steer) never consumes the
// attempt and the diagnostic agent may retry (Story 5.0).
func (dh *diagnoseHandlers) writeNudge(ctx context.Context, callerUUID string, target *session.Instance, message string) *mcpgo.CallToolResult {
	lease, busy := acquireMCPWriteLease(target, session.LeaseWriterNudge)
	if busy != nil {
		return busy
	}
	if errRes := dh.claimNudgeAttemptForWrite(ctx, callerUUID); errRes != nil {
		lease.Release()
		return errRes
	}
	if submitErr := session.SubmitContentWithEnter(ctx, target, lease, message); submitErr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("nudge write failed: %v", submitErr), "")
	}
	return nil
}

func (dh *diagnoseHandlers) performNudge(ctx context.Context, callerUUID string, a nudgeSessionArgs) (*mcpgo.CallToolResult, error) {
	// Kill switch (AC3): read fresh at the write instant, not cached from
	// dispatch time, so flipping the flag off mid-flight still blocks an
	// already-dispatched diagnostic agent's write.
	if !config.EffectiveDiagnoseNudgeEnabled(config.LoadConfig()) {
		return errResult(ErrPermissionDenied, diagnoseNudgeDisabledMessage, declineToDiagnoseRemediation), nil
	}

	if errRes := dh.checkNudgeCapForWrite(ctx, a.itemID, a.stuckReason); errRes != nil {
		return errRes, nil
	}

	target, errRes := dh.resolveNudgeTarget(ctx, a)
	if errRes != nil {
		return errRes, nil
	}

	if errRes := dh.writeNudge(ctx, callerUUID, target, a.message); errRes != nil {
		return errRes, nil
	}

	if _, recErr := dh.storage.RecordDiagnoseNudgeAttempt(ctx, a.itemID, domain.StuckReason(a.stuckReason)); recErr != nil {
		log.Warn("[diagnose_nudge_session] failed to record nudge attempt", "item", a.itemID, "session", a.targetSessionID, "err", recErr)
	}

	return mcpgo.NewToolResultText(fmt.Sprintf("Nudged session %s for item %s", a.targetSessionID, a.itemID)), nil
}
