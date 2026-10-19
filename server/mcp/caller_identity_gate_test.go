package mcp

import (
	"context"
	"maps"
	"slices"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

var newToolNames = []string{toolScheduleWake, toolListWakes, toolCancelWake, toolRequestHelp, toolAnswerHelp}

// probeArgs satisfies every tool's argument validation, so a call that reaches
// the handler would act.
var probeArgs = map[string]any{
	"trigger_type": "at", "message": "m", "delay_minutes": float64(5), "wake_id": "w1",
	"peer": "human", "question": "q", "request_id": "r1", "answer": "a",
}

// refusesUnverified reports whether name's registered handler refuses a caller
// that only claims an identity (the forged-header case).
func refusesUnverified(t *testing.T, s *mcpserver.MCPServer, name string) bool {
	t.Helper()
	tool := s.GetTool(name)
	require.NotNil(t, tool, "tool %q must be registered", name)
	res, err := tool.Handler(WithSessionUUID(context.Background(), "claimed-b"), makeToolReq(probeArgs))
	require.NoError(t, err)
	return resultCode(parseResult(t, res)) == CodeIdentityUnverified
}

func TestIdentityGatedTools_should_BeExactlyTheFiveToolsWithTheAdr007Allowlist(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t, newToolNames, slices.Collect(maps.Keys(identityGatedTools)))
	hidden := []callerRole{roleTriage, roleReview, roleDiagnose}
	for tool, allowed := range identityGatedTools {
		assert.Contains(t, allowed, roleWork, "%s: work sessions may use all five tools", tool)
		for _, role := range hidden {
			want := tool == toolRequestHelp || tool == toolListWakes
			assert.Equal(t, want, slices.Contains(allowed, role), "%s for %s", tool, role)
		}
	}
}

func TestRegistration_should_WrapEveryNewToolInTheIdentityGate(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	for _, name := range newToolNames {
		assert.True(t, refusesUnverified(t, r.server, name), "%s must be registered through withCallerIdentityGate", name)
	}
	assert.Zero(t, len(r.backend.scheduleCalls()), "no write")
	for _, name := range newToolNames {
		assert.EqualValues(t, 1, r.metrics.Value(wakes.MetricIdentityUnverified, name), name)
	}
}

func TestRegistration_should_DetectAToolRegisteredWithoutTheGate(t *testing.T) {
	t.Parallel()
	s := mcpserver.NewMCPServer("test", "0.0.0")
	s.AddTool(mcpgo.NewTool(toolScheduleWake), func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return okResult(map[string]any{"success": true}), nil
	})
	assert.False(t, refusesUnverified(t, s, toolScheduleWake), "the probe must flag an unwrapped tool")
}

func TestRegisterIdentityGatedTools_should_RegisterAllFiveOnTheCoreServer(t *testing.T) {
	t.Parallel()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	core := NewCore(&stubStore{}, nil, nil, storage, nil, nil, nil, nil, nil, nil)
	for _, name := range newToolNames {
		assert.True(t, refusesUnverified(t, core, name), name)
	}
}

func TestIdentityGate_should_DenyRolesOutsideTheAllowlistAndCount(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	r.roles["rev"] = roleReview
	calls := map[string]map[string]any{
		toolScheduleWake: probeArgs, toolCancelWake: probeArgs, toolAnswerHelp: probeArgs,
		toolListWakes: probeArgs, toolRequestHelp: probeArgs,
	}
	for tool, args := range calls {
		m := r.call(as("rev"), tool, args)
		denied := tool == toolScheduleWake || tool == toolCancelWake || tool == toolAnswerHelp
		assert.Equal(t, denied, resultCode(m) == ErrPermissionDenied, tool)
	}
	assert.EqualValues(t, 1, r.metrics.Value(wakes.MetricCallerGateDenied, toolScheduleWake, "review"))
	for _, c := range r.backend.scheduleCalls() {
		_, answerWake := c.Spec.(domain.GuidanceAnsweredSpec)
		assert.True(t, answerWake, "only the allowed request_help scheduled anything; a denied schedule_wake wrote nothing")
	}
}

func TestRoleOfItemSession_should_MapHiddenRolesAndDefaultToWork(t *testing.T) {
	t.Parallel()
	assert.Equal(t, roleTriage, roleOfItemSession(session.SessionRoleTriage))
	assert.Equal(t, roleReview, roleOfItemSession(session.SessionRoleReview))
	assert.Equal(t, roleDiagnose, roleOfItemSession(session.SessionRoleDiagnose))
	assert.Equal(t, roleWork, roleOfItemSession(session.SessionRoleWork))
	assert.Equal(t, roleWork, roleOfItemSession(""))
}

func TestWithCallerIdentityGate_should_PanicAtRegistration_When_ToolHasNoAllowlistRow(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() {
		withCallerIdentityGate(&callerIdentityGate{}, "some_new_tool", nil)
	})
}
