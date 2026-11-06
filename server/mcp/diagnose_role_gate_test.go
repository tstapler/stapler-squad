package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alwaysDiagnose/neverDiagnose are diagnoseCallerCheck stubs for the tests
// below — real behavior (session.Storage.IsDiagnoseCaller) is exercised by
// TestStorage_IsDiagnoseCaller in session/storage_diagnose_caller_test.go.
func alwaysDiagnose(context.Context, string) bool { return true }
func neverDiagnose(context.Context, string) bool  { return false }

// TestDenyIfDiagnoseCaller_TableDriven is the direct unit test for gap #1's
// core decision function: a diagnose-role caller is denied, every other
// caller shape (no checker wired, no session UUID in context, checker says
// no) is allowed through unchanged — the check must only ever narrow, never
// broaden, what an unidentified caller can do.
func TestDenyIfDiagnoseCaller_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		check     diagnoseCallerCheck
		withUUID  bool
		wantDenom bool // true: expect a denial result
	}{
		{"no checker wired (stdio fallback)", nil, true, false},
		{"no session UUID in context", alwaysDiagnose, false, false},
		{"checker says not diagnose", neverDiagnose, true, false},
		{"checker says diagnose", alwaysDiagnose, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.withUUID {
				ctx = WithSessionUUID(ctx, "caller-uuid")
			}
			res := denyIfDiagnoseCaller(ctx, tt.check, "write_to_session")
			if tt.wantDenom {
				require.NotNil(t, res)
				m := parseResult(t, res)
				assert.False(t, m["success"].(bool))
				errObj := m["error"].(map[string]interface{})
				assert.Equal(t, ErrPermissionDenied, errObj["code"])
				assert.Contains(t, errObj["message"], "write_to_session")
			} else {
				assert.Nil(t, res, "must allow through — this caller was not positively identified as a diagnose session")
			}
		})
	}
}

// TestWithDiagnoseGate_SkipsHandler_When_CallerIsDiagnoseSession proves the
// registration-time wrapper actually short-circuits the wrapped handler,
// rather than just computing a result nobody uses. write_to_session,
// send_control, run_command, steer_session, and resume_session previously had
// zero caller-identity checks at all, so a dispatched Diagnose & Nudge
// session could call any of them on any live session.
func TestWithDiagnoseGate_SkipsHandler_When_CallerIsDiagnoseSession(t *testing.T) {
	t.Parallel()
	handlerCalled := false
	stubHandler := func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		handlerCalled = true
		return okResult(map[string]string{"ok": "yes"}), nil
	}

	gated := withDiagnoseGate(alwaysDiagnose, "run_command", stubHandler)
	ctx := WithSessionUUID(context.Background(), "caller-uuid")
	res, err := gated(ctx, makeToolReq(nil))
	require.NoError(t, err)
	assert.False(t, handlerCalled, "the wrapped handler must never run for a positively-identified diagnose caller")
	m := parseResult(t, res)
	assert.False(t, m["success"].(bool))

	// Sanity: the same wrapper lets a non-diagnose caller straight through.
	handlerCalled = false
	gated2 := withDiagnoseGate(neverDiagnose, "run_command", stubHandler)
	_, err = gated2(ctx, makeToolReq(nil))
	require.NoError(t, err)
	assert.True(t, handlerCalled, "a caller not identified as diagnose must reach the wrapped handler")
}

// TestRegisterTerminalTools_GatesRestrictedToolsBehindDiagnoseCheck is the
// registration-level regression guard: it proves write_to_session,
// send_control, run_command, and steer_session are actually wired through
// withDiagnoseGate in registerTerminalTools — not just that the gate function
// itself works in isolation. Calls each registered tool's real Handler (via
// MCPServer.GetTool, not HandleMessage's JSON-RPC plumbing) with a caller
// positively identified as a diagnose session and asserts every one refuses.
// wait_for_output and read_session_output are read-only and deliberately
// left ungated — included here (expected to succeed, or at least not be
// denied for this reason) so a future over-broad gate is caught too.
func TestRegisterTerminalTools_GatesRestrictedToolsBehindDiagnoseCheck(t *testing.T) {
	t.Parallel()
	th := &terminalHandlers{
		store:         &stubStore{},
		scrollback:    makeScrollbackMgr(t),
		writeLim:      newTokenBucket(10, 10),
		diagnoseCheck: alwaysDiagnose,
	}
	s := mcpserver.NewMCPServer("test", "0.0.0")
	registerTerminalTools(s, th)

	ctx := WithSessionUUID(context.Background(), "diagnose-caller-uuid")
	restricted := []string{"write_to_session", "send_control", "run_command", "steer_session"}
	for _, name := range restricted {
		t.Run(name, func(t *testing.T) {
			tool := s.GetTool(name)
			require.NotNil(t, tool, "tool %q must be registered", name)
			res, err := tool.Handler(ctx, makeToolReq(map[string]interface{}{
				"session_id": "some-session",
				"input":      "x",
				"command":    "x",
				"message":    "x",
				"key":        "C",
			}))
			require.NoError(t, err)
			m := parseResult(t, res)
			assert.False(t, m["success"].(bool), "%s must refuse a caller identified as a Diagnose & Nudge session", name)
			errObj := m["error"].(map[string]interface{})
			assert.Equal(t, ErrPermissionDenied, errObj["code"], "%s", name)
		})
	}
}

// TestRegisterLifecycleTools_GatesResumeSessionBehindDiagnoseCheck mirrors
// TestRegisterTerminalTools_GatesRestrictedToolsBehindDiagnoseCheck for
// resume_session, which lives in the separate lifecycleHandlers struct.
func TestRegisterLifecycleTools_GatesResumeSessionBehindDiagnoseCheck(t *testing.T) {
	t.Parallel()
	lh := &lifecycleHandlers{store: &stubStore{}, diagnoseCheck: alwaysDiagnose}
	s := mcpserver.NewMCPServer("test", "0.0.0")
	registerLifecycleTools(s, lh)

	tool := s.GetTool("resume_session")
	require.NotNil(t, tool)
	ctx := WithSessionUUID(context.Background(), "diagnose-caller-uuid")
	res, err := tool.Handler(ctx, makeToolReq(map[string]interface{}{"session_id": "some-session"}))
	require.NoError(t, err)
	m := parseResult(t, res)
	assert.False(t, m["success"].(bool), "resume_session must refuse a caller identified as a Diagnose & Nudge session")
	errObj := m["error"].(map[string]interface{})
	assert.Equal(t, ErrPermissionDenied, errObj["code"])
}
