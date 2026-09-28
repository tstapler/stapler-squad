package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/diagnose"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// stubStore implements session.InstanceStore for tests.
type stubStore struct {
	instances []*session.Instance
	saveErr   error
	loadErr   error
}

func (s *stubStore) LoadInstances() ([]*session.Instance, error) {
	return s.instances, s.loadErr
}

func (s *stubStore) SaveInstances(insts []*session.Instance) error {
	s.instances = insts
	return s.saveErr
}

func (s *stubStore) AddInstance(inst *session.Instance) error {
	s.instances = append(s.instances, inst)
	return s.saveErr
}

func (s *stubStore) DeleteInstance(title string) error {
	for i, inst := range s.instances {
		if inst.Title == title {
			s.instances = append(s.instances[:i], s.instances[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *stubStore) UpdateInstanceLastUserResponse(title string, t time.Time) error {
	return nil
}

func (s *stubStore) UpdateInstanceMetadata(currentTitle string, newTitle, category, note, workingDir *string) error {
	return nil
}

func (s *stubStore) ListInstanceData() ([]session.InstanceData, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	data := make([]session.InstanceData, 0, len(s.instances))
	for _, inst := range s.instances {
		data = append(data, inst.ToInstanceData())
	}
	return data, nil
}

// makeScrollbackMgr creates a scrollback manager backed by a temp directory.
func makeScrollbackMgr(t *testing.T) *scrollback.ScrollbackManager {
	t.Helper()
	cfg := scrollback.DefaultScrollbackConfig()
	cfg.StoragePath = t.TempDir()
	return scrollback.NewScrollbackManager(cfg)
}

// makeToolReq constructs a CallToolRequest with the given args.
func makeToolReq(args map[string]interface{}) mcpgo.CallToolRequest {
	return mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Arguments: args,
		},
	}
}

// parseResult extracts and unmarshals the JSON text from a CallToolResult into
// a generic map. Use this when the precise result type is not known statically.
func parseResult(t *testing.T, res *mcpgo.CallToolResult) map[string]interface{} {
	t.Helper()
	if res == nil {
		t.Fatal("parseResult: result is nil")
	}
	if len(res.Content) == 0 {
		t.Fatal("parseResult: result has no content")
	}
	tc, ok := res.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("parseResult: content[0] is not TextContent, got %T", res.Content[0])
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Fatalf("parseResult: unmarshal JSON: %v\nJSON: %s", err, tc.Text)
	}
	return m
}

// assertDuplicateWriteGuardRejected asserts result is a failure carrying the
// SafetyGateReasonDuplicateWriteAttemptForDispatch error code. Shared by the
// steer_session/write_to_session/resume_session
// "guard runs before the final identity re-check" ordering regression tests
// (tools_terminal_test.go, tools_lifecycle_test.go).
func assertDuplicateWriteGuardRejected(t *testing.T, result *mcpgo.CallToolResult) {
	t.Helper()
	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, _ := m["error"].(map[string]interface{})
	require.NotNil(t, errObj)
	require.Equal(t, string(diagnose.SafetyGateReasonDuplicateWriteAttemptForDispatch), errObj["code"],
		"the duplicate-write guard must be reached (and reject) before the identity re-check ever runs")
}

// fakeNudgeGateEvaluator is a scripted nudgeGateEvaluator for handler-level
// Epic 4.1 tests that want to exercise the steer/write/resume handlers'
// plumbing (does it call Evaluate, map a failure to the right MCP error, and
// proceed to the final identity re-check + write on success) without
// standing up real flag/idle/cap infrastructure. The real pipeline's own
// flag/idle/identity/cap logic is covered directly in
// diagnose_gate_wiring_test.go against a real *diagnose.NudgeGate.
type fakeNudgeGateEvaluator struct {
	ok     bool
	reason diagnose.SafetyGateReason
}

func (f fakeNudgeGateEvaluator) Evaluate(context.Context, diagnose.GateInput) (bool, *diagnose.SafetyGateReason) {
	if f.ok {
		return true, nil
	}
	reason := f.reason
	return false, &reason
}

// newFakeTmuxShowEnvironment writes a fake tmux script answering any
// `show-environment` call with outputLine/exitCode, for driving
// tmux.VerifyIdentityImmediatelyBeforeWrite's real facade under TMUX_BIN —
// mirrors session/tmux/write_gate_ownership_test.go's helper of the same
// name (duplicated rather than imported: that helper is unexported to its
// own package's test binary).
func newFakeTmuxShowEnvironment(t *testing.T, outputLine string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	fakeTmux := filepath.Join(dir, "tmux")
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
  *show-environment*) echo '%s'; exit %d ;;
esac
exit 1
`, outputLine, exitCode)
	require.NoError(t, os.WriteFile(fakeTmux, []byte(script), 0o755))
	return fakeTmux
}

// newGatedTestInstance builds a *session.Instance suitable for exercising the
// Epic 4.1 gate insertion points: a real tmux session name (via
// SetTmuxSession, so GetTmuxSessionName() is deterministic and non-empty,
// with no real tmux server involved -- mirrors
// tools_lifecycle_worktree_guard_test.go's newLiveOccupant precedent) and a
// generated UUID readable via Snapshot().UUID.
func newGatedTestInstance(t *testing.T, title, program string) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{Title: title, Path: t.TempDir(), Program: program})
	require.NoError(t, err)
	inst.SetTmuxSession(tmux.NewTmuxSessionWithDeps(title, "true", nil, nil))
	inst.Status = session.Active
	return inst
}

// requireToolTextResult asserts res is a plain-text success result (the shape
// every mutating backlog handler returns via mcpgo.NewToolResultText on
// success — errResult's JSON-shaped errors, parsed by parseResult, are the
// only other content[0] shape these handlers produce) and returns its text.
func requireToolTextResult(t *testing.T, res *mcpgo.CallToolResult) string {
	t.Helper()
	if res == nil {
		t.Fatal("requireToolTextResult: result is nil")
	}
	if len(res.Content) == 0 {
		t.Fatal("requireToolTextResult: result has no content")
	}
	tc, ok := res.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("requireToolTextResult: content[0] is not TextContent, got %T", res.Content[0])
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Text), &m); err == nil {
		if success, ok := m["success"].(bool); ok && !success {
			t.Fatalf("requireToolTextResult: expected success, got error result: %s", tc.Text)
		}
	}
	return tc.Text
}
