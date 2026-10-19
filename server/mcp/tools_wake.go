package mcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// wakeHandlers backs schedule_wake, list_wakes and cancel_wake. Every handler
// takes the verified caller from the identity gate; none reads a session id
// from its arguments.
type wakeHandlers struct {
	gate  *callerIdentityGate
	flags wakeFlags
	deps  func() *WakeToolDeps
	now   func() time.Time
}

func newWakeHandlers(gate *callerIdentityGate) *wakeHandlers {
	return &wakeHandlers{gate: gate, deps: currentWakeToolDeps, now: time.Now}
}

func registerWakeTools(s *mcpserver.MCPServer, h *wakeHandlers) {
	s.AddTool(mcpgo.NewTool(toolScheduleWake,
		mcpgo.WithDescription("Ask to be woken later instead of polling: at a time, on a cron schedule, when a backlog item reaches a state, when a PR merges or its checks finish, when a peer session reaches a state, or when a help/guidance request is answered. The wake is a new prompt delivered into THIS session; it survives restarts. The session is always the verified caller; any session_id argument is ignored. Limits apply (pending per session, cron interval, fires per hour) and a refusal names the limit."),
		mcpgo.WithString("trigger_type", mcpgo.Required(), mcpgo.Enum(triggerTypeNames()...)),
		mcpgo.WithString("message", mcpgo.Required(), mcpgo.Description("The prompt delivered to you when the wake fires.")),
		mcpgo.WithString("at", mcpgo.Description("RFC 3339 time, for trigger_type=at.")),
		mcpgo.WithNumber("delay_minutes", mcpgo.Description("Minutes from now, for trigger_type=at (alternative to at).")),
		mcpgo.WithString("cron", mcpgo.Description("5-field cron expression, optionally prefixed CRON_TZ=<zone> (default UTC); for trigger_type=cron.")),
		mcpgo.WithString("item_id", mcpgo.Description("Backlog item id, for backlog_event.")),
		mcpgo.WithString("kind", mcpgo.Description("backlog_event: status_reached:<status>, verdict_recorded, item_archived, item_removed or session_attached:<role>. pr_event: merged or closed. ci_event: success, failure or completed.")),
		mcpgo.WithString("repo", mcpgo.Description("owner/repo, for pr_event and ci_event (a pull request of a connected account).")),
		mcpgo.WithNumber("number", mcpgo.Description("Pull request number, for pr_event and ci_event.")),
		mcpgo.WithString("target_session", mcpgo.Description("UUID of the peer session, for session_state.")),
		mcpgo.WithString("state", mcpgo.Description("session_state target: idle, active, paused, stopped, hibernated, crashed, failed or permanently_failed.")),
		mcpgo.WithString("guidance_request_id", mcpgo.Description("Request id, for guidance_answered.")),
		mcpgo.WithNumber("max_fires", mcpgo.Description("How many times a recurring wake may fire (default 12 for cron, 1 otherwise).")),
		mcpgo.WithNumber("expires_in_minutes", mcpgo.Description("Give up after this long (default 24 hours, at most 7 days).")),
		mcpgo.WithBoolean("allow_revive", mcpgo.Description("Permit the normal resume path when this session is stopped (never when archived). Default false.")),
	), withCallerIdentityGate(h.gate, toolScheduleWake, h.scheduleWake))

	s.AddTool(mcpgo.NewTool(toolListWakes,
		mcpgo.WithDescription("List your own scheduled wakes. Live ones by default."),
		mcpgo.WithBoolean("include_ended", mcpgo.Description("Also list delivered, missed, skipped and cancelled wakes still retained.")),
	), withCallerIdentityGate(h.gate, toolListWakes, h.listWakes))

	s.AddTool(mcpgo.NewTool(toolCancelWake,
		mcpgo.WithDescription("Cancel one of your own scheduled wakes."),
		mcpgo.WithString("wake_id", mcpgo.Required()),
	), withCallerIdentityGate(h.gate, toolCancelWake, h.cancelWake))
}

// backend returns the dispatcher, or the typed result explaining why not.
func (h *wakeHandlers) backend(tool string) (WakeBackend, *mcpgo.CallToolResult) {
	if scope := h.flags.toolOff(tool); scope != "" {
		return nil, flagOff(scope)
	}
	deps := h.deps()
	if deps == nil || deps.Backend == nil {
		return nil, typedErrorResult(CodeUnavailable, "", "the wake dispatcher is not running in this process", "Try again later; it may be starting or led by another process.")
	}
	return deps.Backend, nil
}

func flagOff(scope string) *mcpgo.CallToolResult {
	return typedErrorResult(CodeFlagOff, scope, fmt.Sprintf("this feature is turned off (agent_wake_triggers, scope %s)", scope), "Ask the operator to enable it.")
}

// WakeInfo is the JSON shape of one wake.
type WakeInfo struct {
	ID          string  `json:"id"`
	TriggerType string  `json:"trigger_type"`
	Condition   string  `json:"condition"`
	Status      string  `json:"status"`
	EndReason   string  `json:"end_reason,omitempty"`
	Message     string  `json:"message"`
	NextFireAt  *string `json:"next_fire_at,omitempty"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
	FireCount   int     `json:"fire_count"`
	MaxFires    int     `json:"max_fires"`
	Generation  int     `json:"generation"`
	AllowRevive bool    `json:"allow_revive"`
}

func wakeInfoOf(r *session.WakeTriggerData) WakeInfo {
	return WakeInfo{
		ID: r.ID, TriggerType: string(r.Type), Condition: r.Spec.Canonical(), Status: string(r.Status),
		EndReason: string(r.EndReason), Message: r.Action.Payload, NextFireAt: rfc3339Ptr(r.NextFireAt),
		ExpiresAt: rfc3339Ptr(r.ExpiresAt), FireCount: r.FireCount, MaxFires: r.MaxFires,
		Generation: r.Generation, AllowRevive: r.AllowRevive,
	}
}

func rfc3339Ptr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// ScheduleWakeResult is schedule_wake's success shape.
type ScheduleWakeResult struct {
	MCPResult
	Wake WakeInfo `json:"wake"`
	// Created is false when an identical live wake already existed.
	Created bool `json:"created"`
}

func (h *wakeHandlers) scheduleWake(ctx context.Context, caller string, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	be, refusal := h.backend(toolScheduleWake)
	if refusal != nil {
		return refusal, nil
	}
	args := req.GetArguments()
	typ := domain.WakeTriggerType(stringArg(args, "trigger_type"))
	if !typ.IsValid() {
		return errResult(ErrInvalidArgument, fmt.Sprintf("trigger_type %q is not one of %v", typ, triggerTypeNames()), ""), nil
	}
	if scope := h.flags.triggerOff(typ); scope != "" {
		return flagOff(scope), nil
	}
	spec, err := wakeSpecFromArgs(typ, args, h.now())
	if err != nil {
		return errResult(ErrInvalidArgument, err.Error(), ""), nil
	}
	action, err := domain.NewWakeAction(domain.WakeActionSteerText, stringArg(args, "message"))
	if err != nil {
		return errResult(ErrInvalidArgument, err.Error(), ""), nil
	}
	gen, err := be.NextGeneration(ctx, caller)
	if err != nil {
		return errResult(ErrInternalError, err.Error(), ""), nil
	}
	sr := wakes.ScheduleRequest{
		Owner: caller, Spec: spec, Action: action, MaxFires: intArg(args, "max_fires"),
		AllowRevive: boolArg(args, "allow_revive"), Generation: gen,
	}
	if mins := intArg(args, "expires_in_minutes"); mins > 0 {
		exp := h.now().Add(time.Duration(mins) * time.Minute)
		sr.ExpiresAt = &exp
	}
	row, created, err := be.Schedule(ctx, sr)
	if err != nil {
		return scheduleErrorResult(err), nil
	}
	return okResult(ScheduleWakeResult{MCPResult: MCPResult{Success: true}, Wake: wakeInfoOf(row), Created: created}), nil
}

// scheduleErrorResult maps a dispatcher refusal to its typed code. None is a
// silent drop (Wake limits).
func scheduleErrorResult(err error) *mcpgo.CallToolResult {
	var capErr *wakes.CapExceededError
	switch {
	case errors.As(err, &capErr):
		return rawTypedError(typedError{
			MCPError: MCPError{Code: CodeCapExceeded, Message: capErr.Error(), Remediation: "Cancel a wake with cancel_wake or wait for the limit to clear."},
			Limit:    capErr.Limit,
		})
	case errors.Is(err, wakes.ErrNoReconcilePredicate):
		return typedErrorResult(CodeNoReconcilePredicate, "", err.Error(), "Use a condition that can be re-checked from stored state, such as status_reached:<status>.")
	case errors.Is(err, wakes.ErrEventStreamUnavailable):
		return typedErrorResult(CodeUnavailable, "event_stream", "event stream unavailable", "Time-based wakes (at, cron) still work.")
	case errors.Is(err, wakes.ErrUnavailable):
		return typedErrorResult(CodeUnavailable, "source", err.Error(), "")
	case errors.Is(err, wakes.ErrForbidden):
		return errResult(ErrPermissionDenied, err.Error(), "")
	case errors.Is(err, wakes.ErrInvalid):
		return errResult(ErrInvalidArgument, err.Error(), "")
	default:
		return errResult(ErrInternalError, err.Error(), "")
	}
}

func wakeSpecFromArgs(typ domain.WakeTriggerType, args map[string]any, now time.Time) (domain.WakeSpec, error) {
	switch typ {
	case domain.WakeTriggerAt:
		return atSpecFromArgs(args, now)
	case domain.WakeTriggerCron:
		return domain.CronSpec{Expr: stringArg(args, "cron")}, nil
	case domain.WakeTriggerBacklogEvent:
		return domain.BacklogEventSpec{ItemID: stringArg(args, "item_id"), Kind: stringArg(args, "kind")}, nil
	case domain.WakeTriggerPREvent:
		return domain.PREventSpec{Repo: stringArg(args, "repo"), Number: intArg(args, "number"), Kind: stringArg(args, "kind")}, nil
	case domain.WakeTriggerCIEvent:
		return domain.CIEventSpec{Repo: stringArg(args, "repo"), Number: intArg(args, "number"), Kind: stringArg(args, "kind")}, nil
	case domain.WakeTriggerSessionState:
		return domain.SessionStateSpec{SessionUUID: stringArg(args, "target_session"), State: stringArg(args, "state")}, nil
	case domain.WakeTriggerGuidanceAnswered:
		return domain.GuidanceAnsweredSpec{GuidanceRequestID: stringArg(args, "guidance_request_id")}, nil
	}
	return nil, fmt.Errorf("unsupported trigger_type %q", typ)
}

func atSpecFromArgs(args map[string]any, now time.Time) (domain.WakeSpec, error) {
	if raw := stringArg(args, "at"); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, fmt.Errorf("at must be an RFC 3339 time: %w", err)
		}
		if !at.After(now) {
			return nil, errors.New("at is in the past")
		}
		return domain.AtSpec{At: at}, nil
	}
	if mins := intArg(args, "delay_minutes"); mins > 0 {
		return domain.AtSpec{At: now.Add(time.Duration(mins) * time.Minute)}, nil
	}
	return nil, errors.New("trigger_type=at needs at or delay_minutes")
}

// liveWakeStatuses are the statuses list_wakes shows by default.
var liveWakeStatuses = map[domain.WakeStatus]bool{
	domain.WakeStatusPending: true, domain.WakeStatusClaimed: true, domain.WakeStatusNeedsRevive: true,
}

// ListWakesResult is list_wakes' success shape.
type ListWakesResult struct {
	MCPResult
	Wakes []WakeInfo `json:"wakes"`
}

func (h *wakeHandlers) listWakes(ctx context.Context, caller string, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	be, refusal := h.backend(toolListWakes)
	if refusal != nil {
		return refusal, nil
	}
	rows, err := be.ListByOwner(ctx, caller)
	if err != nil {
		return errResult(ErrInternalError, err.Error(), ""), nil
	}
	includeEnded := boolArg(req.GetArguments(), "include_ended")
	out := ListWakesResult{MCPResult: MCPResult{Success: true}, Wakes: []WakeInfo{}}
	for _, r := range rows {
		if includeEnded || liveWakeStatuses[r.Status] {
			out.Wakes = append(out.Wakes, wakeInfoOf(r))
		}
	}
	return okResult(out), nil
}

// CancelWakeResult is cancel_wake's success shape.
type CancelWakeResult struct {
	MCPResult
	Wake WakeInfo `json:"wake"`
	// Cancelled is false when the wake had already ended (for example fired first).
	Cancelled bool `json:"cancelled"`
}

func (h *wakeHandlers) cancelWake(ctx context.Context, caller string, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	be, refusal := h.backend(toolCancelWake)
	if refusal != nil {
		return refusal, nil
	}
	row, cancelled, err := be.Cancel(ctx, caller, stringArg(req.GetArguments(), "wake_id"))
	switch {
	case errors.Is(err, wakes.ErrWakeNotFound):
		return errResult(ErrItemNotFound, "no such wake", ""), nil
	case errors.Is(err, wakes.ErrWakeNotOwner):
		return typedErrorResult(CodeForbidden, "", "that wake belongs to another session", ""), nil
	case err != nil:
		return errResult(ErrInternalError, err.Error(), ""), nil
	}
	return okResult(CancelWakeResult{MCPResult: MCPResult{Success: true}, Wake: wakeInfoOf(row), Cancelled: cancelled}), nil
}

func intArg(args map[string]any, key string) int {
	f, _ := args[key].(float64)
	return int(f)
}
