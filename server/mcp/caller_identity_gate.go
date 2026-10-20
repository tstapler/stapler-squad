package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
)

// Typed result codes of the wake and peer-help tools (plan Story 4.5). They
// are lowercase so an agent can switch on them; the older tools' uppercase
// codes are unchanged.
const (
	CodeIdentityUnverified   = "identity_unverified"
	CodeFlagOff              = "flag_off"
	CodeCapExceeded          = "cap_exceeded"
	CodeUnavailable          = "unavailable"
	CodeNoReconcilePredicate = "no_reconcile_predicate"
	CodeRejected             = "rejected"
	CodeForbidden            = "FORBIDDEN"
)

// callerRole is what a verified session is for, as far as tool access goes.
type callerRole string

const (
	roleWork     callerRole = "work"
	roleTriage   callerRole = "triage"
	roleReview   callerRole = "review"
	roleDiagnose callerRole = "diagnose"
	// roleUnknown is a failed role lookup; it is on no allowlist, so the gate fails closed.
	roleUnknown callerRole = "unknown"
)

// Tool names of the identity-gated surface.
const (
	toolScheduleWake = "schedule_wake"
	toolListWakes    = "list_wakes"
	toolCancelWake   = "cancel_wake"
	toolRequestHelp  = "request_help"
	toolAnswerHelp   = "answer_help"
)

// identityGatedTools is the role allowlist of ADR-007 rule 2: work sessions
// use all five tools; the hidden roles, which already have bespoke reporting
// tools, may only ask for help and list their wakes. A change here needs an ADR
// amendment, and every tool that grants authority by identity must have a row.
var identityGatedTools = map[string][]callerRole{
	toolScheduleWake: {roleWork},
	toolListWakes:    {roleWork, roleTriage, roleReview, roleDiagnose},
	toolCancelWake:   {roleWork},
	toolRequestHelp:  {roleWork, roleTriage, roleReview, roleDiagnose},
	toolAnswerHelp:   {roleWork},
}

// roleOfItemSession maps a backlog ItemSession role to a callerRole; a session
// with no link, or any role not listed, is a work session.
func roleOfItemSession(role string) callerRole {
	switch role {
	case session.SessionRoleTriage:
		return roleTriage
	case session.SessionRoleReview:
		return roleReview
	case session.SessionRoleDiagnose:
		return roleDiagnose
	default:
		return roleWork
	}
}

// callerRoleLookup resolves a verified session's role.
type callerRoleLookup func(ctx context.Context, sessionUUID string) callerRole

// callerIdentityGate wraps tools that grant authority by identity.
type callerIdentityGate struct {
	roleOf  callerRoleLookup
	metrics *wakes.Metrics
}

// identityHandler is a tool body that runs only for a verified, allowed caller.
type identityHandler func(ctx context.Context, caller string, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)

// withCallerIdentityGate wraps handler so it runs only when the request carries
// a verified session identity (ADR-007 rule 0) whose role is on tool's
// allowlist. An unverified claim, however plausible, gets `identity_unverified`
// and the handler never runs, so nothing is written. The caller is passed in;
// handlers must never read a session id from their arguments.
func withCallerIdentityGate(g *callerIdentityGate, tool string, handler identityHandler) mcpserver.ToolHandlerFunc {
	allowed, ok := identityGatedTools[tool]
	if !ok {
		panic(fmt.Sprintf("withCallerIdentityGate: %q has no row in identityGatedTools", tool))
	}
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		caller, verified := verifiedCallerSessionUUID(ctx)
		if !verified {
			g.metrics.Inc(wakes.MetricIdentityUnverified, tool)
			return typedErrorResult(CodeIdentityUnverified, "", IdentityUnverifiedMessage, "Relaunch this session; the credential is issued at launch."), nil
		}
		role := g.roleOf(ctx, caller)
		if !slices.Contains(allowed, role) {
			g.metrics.Inc(wakes.MetricCallerGateDenied, tool, string(role))
			return errResult(ErrPermissionDenied, fmt.Sprintf("%s is not available to a %s session", tool, role), ""), nil
		}
		return handler(ctx, caller, req)
	}
}

// typedError is an MCPError with the machine-readable detail the typed codes carry.
type typedError struct {
	MCPError
	Reason string `json:"reason,omitempty"`
	Limit  string `json:"limit,omitempty"`
}

type typedErrorEnvelope struct {
	Success bool       `json:"success"`
	Error   typedError `json:"error"`
}

// typedErrorResult renders a typed failure; reason names the sub-case
// (rejected: peer_archived) and is empty when the code says it all.
func typedErrorResult(code, reason, message, remediation string) *mcpgo.CallToolResult {
	return rawTypedError(typedError{MCPError: MCPError{Code: code, Message: message, Remediation: remediation}, Reason: reason})
}

func rawTypedError(e typedError) *mcpgo.CallToolResult {
	b, err := json.Marshal(typedErrorEnvelope{Error: e})
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("marshal error: %v", err), "")
	}
	return mcpgo.NewToolResultText(string(b))
}

// roleLookupFrom resolves a role from a backlog-link read. Only "no link" means
// a work session; any other error yields roleUnknown, which no tool allows.
func roleLookupFrom(get func(ctx context.Context, sessionUUID string) (session.ItemSessionSummary, error)) callerRoleLookup {
	return func(ctx context.Context, sessionUUID string) callerRole {
		link, err := get(ctx, sessionUUID)
		switch {
		case errors.Is(err, session.ErrNotFound):
			return roleWork
		case err != nil:
			return roleUnknown
		}
		return roleOfItemSession(link.Role)
	}
}

// registerIdentityGatedTools registers the five wake and peer-help tools behind
// one identity gate whose role lookup reads the caller's backlog link.
func registerIdentityGatedTools(s *mcpserver.MCPServer, storage *session.Storage) {
	gate := &callerIdentityGate{
		metrics: wakes.NewMetrics(),
		roleOf:  roleLookupFrom(storage.GetItemSessionBySessionUUID),
	}
	registerWakeTools(s, newWakeHandlers(gate))
	registerPeerHelpTools(s, newPeerHelpHandlers(gate, storage))
}
