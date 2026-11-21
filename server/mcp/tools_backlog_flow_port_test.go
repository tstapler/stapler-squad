package mcp

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
)

var _ FlowService = (*backlogflow.BacklogFlowService)(nil)

// recordingFlowService is the second FlowService implementation: it counts each
// call by method so a handler routed to the wrong RPC fails.
type recordingFlowService struct {
	calls map[string]int
	err   error
}

func (f *recordingFlowService) note(m string) { f.calls[m]++ }

func (f *recordingFlowService) GetFlowSummary(context.Context, *connect.Request[sessionv1.GetFlowSummaryRequest]) (*connect.Response[sessionv1.GetFlowSummaryResponse], error) {
	f.note("GetFlowSummary")
	return connect.NewResponse(&sessionv1.GetFlowSummaryResponse{}), f.err
}

func (f *recordingFlowService) ListFlowDrilldown(context.Context, *connect.Request[sessionv1.ListFlowDrilldownRequest]) (*connect.Response[sessionv1.ListFlowDrilldownResponse], error) {
	f.note("ListFlowDrilldown")
	return connect.NewResponse(&sessionv1.ListFlowDrilldownResponse{}), f.err
}

func (f *recordingFlowService) CompareFlowWindows(context.Context, *connect.Request[sessionv1.CompareFlowWindowsRequest]) (*connect.Response[sessionv1.CompareFlowWindowsResponse], error) {
	f.note("CompareFlowWindows")
	return connect.NewResponse(&sessionv1.CompareFlowWindowsResponse{}), f.err
}

func (f *recordingFlowService) ListDeadlockCandidates(context.Context, *connect.Request[sessionv1.ListDeadlockCandidatesRequest]) (*connect.Response[sessionv1.ListDeadlockCandidatesResponse], error) {
	f.note("ListDeadlockCandidates")
	return connect.NewResponse(&sessionv1.ListDeadlockCandidatesResponse{}), f.err
}

func (f *recordingFlowService) ListThrashEpisodes(context.Context, *connect.Request[sessionv1.ListThrashEpisodesRequest]) (*connect.Response[sessionv1.ListThrashEpisodesResponse], error) {
	f.note("ListThrashEpisodes")
	return connect.NewResponse(&sessionv1.ListThrashEpisodesResponse{}), f.err
}

func (f *recordingFlowService) GetItemFlowTimeline(context.Context, *connect.Request[sessionv1.GetItemFlowTimelineRequest]) (*connect.Response[sessionv1.GetItemFlowTimelineResponse], error) {
	f.note("GetItemFlowTimeline")
	return connect.NewResponse(&sessionv1.GetItemFlowTimelineResponse{}), f.err
}

func TestFlowHandlers_should_CallTheMatchingPortMethod_When_FlowServiceIsAFake(t *testing.T) {
	t.Parallel()
	type handler func(*flowHandlers, context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	cases := []struct {
		method string
		args   map[string]any
		call   handler
	}{
		{"GetFlowSummary", nil, (*flowHandlers).getFlowSummary},
		{"ListFlowDrilldown", map[string]any{"key": "k"}, (*flowHandlers).listFlowDrilldown},
		{"CompareFlowWindows", nil, (*flowHandlers).compareFlowWindows},
		{"ListDeadlockCandidates", nil, (*flowHandlers).listFlowDeadlockCandidates},
		{"ListThrashEpisodes", nil, (*flowHandlers).listFlowThrashEpisodes},
		{"GetItemFlowTimeline", map[string]any{"item_id": "i"}, (*flowHandlers).getItemFlowTimeline},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			fake := &recordingFlowService{calls: map[string]int{}}
			h := &flowHandlers{svc: fake}

			res, err := tc.call(h, context.Background(), makeToolReq(tc.args))

			require.NoError(t, err)
			assert.False(t, res.IsError, "a successful port reply must not become an error result")
			assert.Equal(t, map[string]int{tc.method: 1}, fake.calls)
		})
	}
}

func TestFlowHandlers_should_ReportPortError_When_FlowServiceFails(t *testing.T) {
	t.Parallel()
	fake := &recordingFlowService{calls: map[string]int{}, err: connect.NewError(connect.CodeFailedPrecondition, errors.New("flow disabled"))}
	h := &flowHandlers{svc: fake}

	res, err := h.getFlowSummary(context.Background(), makeToolReq(nil))

	require.NoError(t, err)
	assert.Equal(t, ErrUnavailable, parseResult(t, res)["error"].(map[string]any)["code"], "the port's error must reach flowErrorResult")
}

func TestFlowHandlers_should_AnswerUnavailable_When_NoFlowServiceInjected(t *testing.T) {
	t.Parallel()
	res, err := (&flowHandlers{}).getFlowSummary(context.Background(), makeToolReq(nil))

	require.NoError(t, err)
	assert.Equal(t, ErrUnavailable, parseResult(t, res)["error"].(map[string]any)["code"])
}

func TestWithFlowService_should_CarryTheInjectedPort(t *testing.T) {
	t.Parallel()
	fake := &recordingFlowService{calls: map[string]int{}}
	assert.Same(t, fake, FlowServiceFromOptions(WithFlowService(fake)))
	assert.True(t, FlowServiceFromOptions() == nil, "no option leaves a nil interface")
}

// The MCP copy must not drift from the RPC form; the parity test compares output.
func TestFlowJSONOptions_should_EqualTheRpcOptions(t *testing.T) {
	t.Parallel()
	assert.Equal(t, backlogflow.FlowJSONOptions(), flowJSONOptions())
}
