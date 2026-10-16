package mcp

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// resetCreateSessionLimiterForTest restores the package-level
// createSessionLimiter (capacity 3, shared by every create_session/
// create_session_for_pr call in this test binary) so tests that call the
// handler directly don't exhaust each other's tokens. Not t.Parallel()-safe.
func resetCreateSessionLimiterForTest(t *testing.T) {
	t.Helper()
	createSessionLimiter = newTokenBucket(3.0/60.0, 3)
}

// TestResetCreateSessionLimiterForTest_should_RestoreCapacity_When_BucketExhausted
// proves the reset helper the other rate-limited tests in this file and
// tools_github_test.go depend on actually restores capacity, rather than
// leaving that trust implicit -- an incorrect reset would otherwise surface
// as a flaky ErrRateLimitExceeded failure in an unrelated test.
func TestResetCreateSessionLimiterForTest_should_RestoreCapacity_When_BucketExhausted(t *testing.T) {
	for createSessionLimiter.allow("global") {
		// Drain the bucket.
	}
	require.False(t, createSessionLimiter.allow("global"), "test setup: bucket must be exhausted before reset")

	resetCreateSessionLimiterForTest(t)

	for i := 0; i < 3; i++ {
		assert.True(t, createSessionLimiter.allow("global"), "expected capacity restored after reset (draw %d)", i)
	}
	assert.False(t, createSessionLimiter.allow("global"), "bucket should be exhausted again after 3 draws")
}

// TestRegisterLifecycleTools_should_IncludeAllBuiltinsInProgramEnum_When_SvcIsWired
// pins AC1/AC4's schema-level claim: create_session's program property must
// no longer hardcode Enum("claude", "aider") -- bash and opencode are the two
// clearest previously-unreachable built-ins (research/features.md).
func TestRegisterLifecycleTools_should_IncludeAllBuiltinsInProgramEnum_When_SvcIsWired(t *testing.T) {
	lh := newWorktreeGuardHandlers(t)
	s := mcpserver.NewMCPServer("test", "0.0.0")
	registerLifecycleTools(s, lh)

	tool := s.GetTool("create_session")
	require.NotNil(t, tool, "create_session must be registered")

	programProp, ok := tool.Tool.InputSchema.Properties["program"].(map[string]any)
	require.True(t, ok, "program property must be a map[string]any, got %#v", tool.Tool.InputSchema.Properties["program"])

	desc, _ := programProp["description"].(string)
	assert.NotEmpty(t, desc, "program property must document valid values")

	enum, ok := programProp["enum"].([]string)
	require.True(t, ok, "program property must have an enum when svc is wired, got %#v", programProp["enum"])
	assert.Contains(t, enum, "bash")
	assert.Contains(t, enum, "opencode")
}

// createSessionArgs builds a minimal create_session request map for title in
// a fresh temp directory (directory session type, no worktree/GitHub I/O).
func createSessionArgs(t *testing.T, title string, extra map[string]interface{}) map[string]interface{} {
	t.Helper()
	args := map[string]interface{}{
		"title": title,
		"path":  t.TempDir(),
	}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

// destroyMCPCreatedSession cleans up a session created directly through
// lh.svc, mirroring server/services' destroyCreatedSession -- that helper's
// svc.waitForPendingCleanup() call is unexported outside the services
// package, so this uses the exported DeleteSession RPC instead.
func destroyMCPCreatedSession(t *testing.T, lh *lifecycleHandlers, id string) {
	t.Helper()
	_, err := lh.svc.DeleteSession(context.Background(), connect.NewRequest(&sessionv1.DeleteSessionRequest{Id: id}))
	if err != nil {
		t.Logf("destroyMCPCreatedSession: cleanup for %q failed (non-fatal): %v", id, err)
	}
}

// TestCreateSession_should_AcceptPreviouslyUnreachableBuiltin_When_ProgramIsBash
// pins AC1: "bash" was a real, always-supported built-in (services.BuiltInPrograms)
// that the old hardcoded Enum("claude", "aider") silently rejected before it ever
// reached the backend. It must also carry no ProgramWarning, since it is a
// recognized program (Story 1.2.1's AC).
func TestCreateSession_should_AcceptPreviouslyUnreachableBuiltin_When_ProgramIsBash(t *testing.T) {
	resetCreateSessionLimiterForTest(t)
	lh := newWorktreeGuardHandlers(t)

	res, err := lh.createSession(context.Background(), makeToolReq(createSessionArgs(t, "program-test-bash", map[string]interface{}{
		"program": "bash",
	})))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "expected success, got: %+v", m)
	t.Cleanup(func() { destroyMCPCreatedSession(t, lh, m["session"].(map[string]interface{})["id"].(string)) })
	assert.Empty(t, m["program_warning"], "a recognized built-in must not set program_warning")
}

// TestCreateSession_should_AcceptCustomProgram_When_RegisteredViaUpsertProgramConfig
// pins AC1's headline scenario: a custom program registered via
// UpsertProgramConfig must be accepted by create_session's MCP schema, not
// just the ConnectRPC API directly.
func TestCreateSession_should_AcceptCustomProgram_When_RegisteredViaUpsertProgramConfig(t *testing.T) {
	resetCreateSessionLimiterForTest(t)
	lh := newWorktreeGuardHandlers(t)
	upsertTestCustomProgram(t, lh.svc)

	res, err := lh.createSession(context.Background(), makeToolReq(createSessionArgs(t, "program-test-custom", map[string]interface{}{
		"program": testCustomProgramID,
	})))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "expected success, got: %+v", m)
	t.Cleanup(func() { destroyMCPCreatedSession(t, lh, m["session"].(map[string]interface{})["id"].(string)) })
	assert.Empty(t, m["program_warning"], "a registered custom program must not set program_warning")
}

// TestCreateSession_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider
// is AC3's regression pin: explicit program="aider" must keep working.
func TestCreateSession_should_AcceptAiderUnchanged_When_ProgramIsExplicitlyAider(t *testing.T) {
	resetCreateSessionLimiterForTest(t)
	lh := newWorktreeGuardHandlers(t)

	res, err := lh.createSession(context.Background(), makeToolReq(createSessionArgs(t, "program-test-aider", map[string]interface{}{
		"program": "aider",
	})))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "expected success, got: %+v", m)
	t.Cleanup(func() { destroyMCPCreatedSession(t, lh, m["session"].(map[string]interface{})["id"].(string)) })
	assert.Empty(t, m["program_warning"])
}

// TestCreateSession_should_DefaultToClaudeProgram_When_ProgramOmitted pins
// AC3's default-unchanged claim: omitting program entirely must still
// default to "claude" (via defaultProgramID) and must never itself warn.
func TestCreateSession_should_DefaultToClaudeProgram_When_ProgramOmitted(t *testing.T) {
	resetCreateSessionLimiterForTest(t)
	lh := newWorktreeGuardHandlers(t)

	res, err := lh.createSession(context.Background(), makeToolReq(createSessionArgs(t, "program-test-default", nil)))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "expected success, got: %+v", m)
	t.Cleanup(func() { destroyMCPCreatedSession(t, lh, m["session"].(map[string]interface{})["id"].(string)) })
	assert.Empty(t, m["program_warning"], "the default program must never itself warn")
}

// TestCreateSession_should_SetProgramWarning_When_ProgramNotInKnownList pins
// AC4's non-fatal-warning mechanism: a typo'd/unrecognized program must still
// succeed (a custom program registered after server start must still work),
// but must warn so the calling agent isn't left silently guessing.
func TestCreateSession_should_SetProgramWarning_When_ProgramNotInKnownList(t *testing.T) {
	resetCreateSessionLimiterForTest(t)
	lh := newWorktreeGuardHandlers(t)

	res, err := lh.createSession(context.Background(), makeToolReq(createSessionArgs(t, "program-test-typo", map[string]interface{}{
		"program": "clade",
	})))
	require.NoError(t, err)

	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "an unrecognized program must warn, not reject, got: %+v", m)
	t.Cleanup(func() { destroyMCPCreatedSession(t, lh, m["session"].(map[string]interface{})["id"].(string)) })
	warning, _ := m["program_warning"].(string)
	assert.Contains(t, warning, "clade")
}

// TestRegisterLifecycleTools_should_OmitEnum_When_SvcIsNil pins that
// registerLifecycleTools actually wires programSchemaOptions' nil-safe path
// through at registration time (its own contract is unit-tested directly in
// tools_program_common_test.go). registerLifecycleTools runs unconditionally
// in NewCore regardless of svc's nilness, so this schema-registration path
// does run in practice — unlike lh.createSession itself, which has no
// nil-svc-safe path and is out of scope here.
func TestRegisterLifecycleTools_should_OmitEnum_When_SvcIsNil(t *testing.T) {
	lh := &lifecycleHandlers{store: &stubStore{}, svc: nil}
	s := mcpserver.NewMCPServer("test", "0.0.0")
	registerLifecycleTools(s, lh)

	tool := s.GetTool("create_session")
	require.NotNil(t, tool)
	programProp, ok := tool.Tool.InputSchema.Properties["program"].(map[string]any)
	require.True(t, ok)

	_, hasEnum := programProp["enum"]
	assert.False(t, hasEnum, "program property must have no enum when svc is nil")
}
