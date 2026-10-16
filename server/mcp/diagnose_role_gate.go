package mcp

import (
	"context"
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// diagnoseCallerCheck resolves whether a caller session UUID is a dispatched
// Diagnose & Nudge session (session.SessionRoleDiagnose). A nil value means
// storage wasn't wired at server construction (e.g. the stdio fallback path
// with no backlog support) — treated the same as "not identified", since this
// check only ever narrows what a positively-identified diagnose session may
// do and must never affect any other caller.
type diagnoseCallerCheck func(ctx context.Context, sessionUUID string) bool

// diagnoseRestrictedRemediation is the remediation text returned alongside a
// denial — reuses declineToDiagnoseRemediation (tools_diagnose.go) so both
// gates point the dispatched agent at the same narrow action space.
const diagnoseRestrictedRemediation = declineToDiagnoseRemediation

// denyIfDiagnoseCaller is the real, server-side enforcement behind AllowedTools'
// best-effort client-side restriction on a dispatched Diagnose & Nudge
// session (see server/services/session_service.go's diagnosticSessionAllowedTools
// doc comment for why AllowedTools alone isn't sufficient). It rejects
// toolName for any caller positively identified as a diagnose-role session —
// diagnose_nudge_session is that role's one legitimate write path and is
// gated separately, by its own role check in tools_diagnose.go's
// resolveDiagnoseItemLink, so it never reaches this function at all.
//
// Returns nil (allow) whenever the caller can't be positively identified as a
// diagnose session: no session UUID in context, no checker wired, or the
// checker itself says no. Mirrors the fail-closed-on-match,
// PERMISSION_DENIED convention resolveDiagnoseItemLink/resolveItemLink
// already use for role checks elsewhere in this package.
func denyIfDiagnoseCaller(ctx context.Context, check diagnoseCallerCheck, toolName string) *mcpgo.CallToolResult {
	if check == nil {
		return nil
	}
	callerUUID, ok := sessionUUIDFromContext(ctx)
	if !ok {
		return nil
	}
	if !check(ctx, callerUUID) {
		return nil
	}
	return errResult(ErrPermissionDenied,
		fmt.Sprintf("%s is not available to a Diagnose & Nudge session", toolName),
		diagnoseRestrictedRemediation)
}

// withDiagnoseGate wraps handler so a caller positively identified as a
// dispatched Diagnose & Nudge session (check) is rejected before handler ever
// runs — see denyIfDiagnoseCaller above. A package-level function (not a
// method on any one handler struct) so every gated tool's registration
// (terminalHandlers' write_to_session/send_control/run_command/steer_session,
// lifecycleHandlers' resume_session) shares one implementation instead of an
// identical guard clause duplicated inline in each handler body, which trips
// this repo's dupl gate.
func withDiagnoseGate(check diagnoseCallerCheck, toolName string, handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		if denied := denyIfDiagnoseCaller(ctx, check, toolName); denied != nil {
			return denied, nil
		}
		return handler(ctx, req)
	}
}
