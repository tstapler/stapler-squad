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
	"google.golang.org/protobuf/proto"
)

var _ FlowService = (*backlogflow.BacklogFlowService)(nil)

// recordingFlowService is the second FlowService implementation: it counts each
// call by method so a handler routed to the wrong RPC fails.
type recordingFlowService struct {
	calls map[string]int
	got   map[string]proto.Message // last request per method
	err   error
}

func newRecordingFlowService() *recordingFlowService {
	return &recordingFlowService{calls: map[string]int{}, got: map[string]proto.Message{}}
}

func (f *recordingFlowService) note(m string, req proto.Message) {
	f.calls[m]++
	f.got[m] = req
}

func (f *recordingFlowService) GetFlowSummary(_ context.Context, req *connect.Request[sessionv1.GetFlowSummaryRequest]) (*connect.Response[sessionv1.GetFlowSummaryResponse], error) {
	f.note("GetFlowSummary", req.Msg)
	return connect.NewResponse(&sessionv1.GetFlowSummaryResponse{}), f.err
}

func (f *recordingFlowService) ListFlowDrilldown(_ context.Context, req *connect.Request[sessionv1.ListFlowDrilldownRequest]) (*connect.Response[sessionv1.ListFlowDrilldownResponse], error) {
	f.note("ListFlowDrilldown", req.Msg)
	return connect.NewResponse(&sessionv1.ListFlowDrilldownResponse{}), f.err
}

func (f *recordingFlowService) CompareFlowWindows(_ context.Context, req *connect.Request[sessionv1.CompareFlowWindowsRequest]) (*connect.Response[sessionv1.CompareFlowWindowsResponse], error) {
	f.note("CompareFlowWindows", req.Msg)
	return connect.NewResponse(&sessionv1.CompareFlowWindowsResponse{}), f.err
}

func (f *recordingFlowService) ListDeadlockCandidates(_ context.Context, req *connect.Request[sessionv1.ListDeadlockCandidatesRequest]) (*connect.Response[sessionv1.ListDeadlockCandidatesResponse], error) {
	f.note("ListDeadlockCandidates", req.Msg)
	return connect.NewResponse(&sessionv1.ListDeadlockCandidatesResponse{}), f.err
}

func (f *recordingFlowService) ListThrashEpisodes(_ context.Context, req *connect.Request[sessionv1.ListThrashEpisodesRequest]) (*connect.Response[sessionv1.ListThrashEpisodesResponse], error) {
	f.note("ListThrashEpisodes", req.Msg)
	return connect.NewResponse(&sessionv1.ListThrashEpisodesResponse{}), f.err
}

func (f *recordingFlowService) GetItemFlowTimeline(_ context.Context, req *connect.Request[sessionv1.GetItemFlowTimelineRequest]) (*connect.Response[sessionv1.GetItemFlowTimelineResponse], error) {
	f.note("GetItemFlowTimeline", req.Msg)
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
			fake := newRecordingFlowService()
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
	fake := &recordingFlowService{calls: map[string]int{}, got: map[string]proto.Message{}, err: connect.NewError(connect.CodeFailedPrecondition, errors.New("flow disabled"))}
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
	fake := newRecordingFlowService()
	assert.Same(t, fake, FlowServiceFromOptions(WithFlowService(fake)))
	assert.True(t, FlowServiceFromOptions() == nil, "no option leaves a nil interface")
}

// The MCP copy must not drift from the RPC form; the parity test compares output.
func TestFlowJSONOptions_should_EqualTheRpcOptions(t *testing.T) {
	t.Parallel()
	assert.Equal(t, backlogflow.FlowJSONOptions(), flowJSONOptions())
}

func TestFlowHandlers_should_PassToolArgumentsToThePort(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		method string
		args   map[string]any
		call   func(*flowHandlers, context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
		want   proto.Message
	}{
		{"summary", "GetFlowSummary", map[string]any{"window_days": 7.0, "compact": true, "list_limit": 5.0, "sections": []any{"tiles"}},
			(*flowHandlers).getFlowSummary,
			&sessionv1.GetFlowSummaryRequest{WindowDays: 7, Compact: true, ListLimit: 5, Sections: []string{"tiles"}}},
		{"drilldown", "ListFlowDrilldown", map[string]any{"key": "k", "window_days": 9.0, "limit": 3.0, "offset": 4.0},
			(*flowHandlers).listFlowDrilldown,
			&sessionv1.ListFlowDrilldownRequest{Key: "k", WindowDays: 9, Limit: 3, Offset: 4}},
		{"compare offsets", "CompareFlowWindows", map[string]any{"a_days": 7.0, "b_days": 7.0, "b_offset_days": 7.0},
			(*flowHandlers).compareFlowWindows,
			&sessionv1.CompareFlowWindowsRequest{ADays: 7, BDays: 7, BOffsetDays: 7}},
		{"deadlock", "ListDeadlockCandidates", map[string]any{"window_days": 2.0, "limit": 6.0},
			(*flowHandlers).listFlowDeadlockCandidates,
			&sessionv1.ListDeadlockCandidatesRequest{WindowDays: 2, Limit: 6}},
		{"thrash all time", "ListThrashEpisodes", map[string]any{"all_time": true, "limit": 8.0},
			(*flowHandlers).listFlowThrashEpisodes,
			&sessionv1.ListThrashEpisodesRequest{AllTime: true, Limit: 8}},
		{"timeline", "GetItemFlowTimeline", map[string]any{"item_id": "it-1", "window_days": 11.0},
			(*flowHandlers).getItemFlowTimeline,
			&sessionv1.GetItemFlowTimelineRequest{ItemId: "it-1", WindowDays: 11}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newRecordingFlowService()

			_, err := tc.call(&flowHandlers{svc: fake}, context.Background(), makeToolReq(tc.args))

			require.NoError(t, err)
			require.NotNil(t, fake.got[tc.method], "the port was not called")
			assert.True(t, proto.Equal(tc.want, fake.got[tc.method]), "want %v, got %v", tc.want, fake.got[tc.method])
		})
	}
}

func TestCompareFlowWindows_should_RejectBothForms_WithoutCallingThePort(t *testing.T) {
	t.Parallel()
	fake := newRecordingFlowService()
	args := map[string]any{
		"a_days": 7.0, "a_start": "2026-01-01", "a_end": "2026-01-08", "b_start": "2026-01-08", "b_end": "2026-01-15",
	}

	res, err := (&flowHandlers{svc: fake}).compareFlowWindows(context.Background(), makeToolReq(args))

	require.NoError(t, err)
	assert.Equal(t, ErrInvalidArgument, parseResult(t, res)["error"].(map[string]any)["code"])
	assert.Empty(t, fake.calls, "mixing offsets and dates must not reach the service")
}

func TestSanitizeFlowKeyEcho_should_FlattenNewlines(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a b", sanitizeFlowKeyEcho("a\nb"))
}

// A typed-nil pointer boxed into the port defeats the absent-service guard: the flow tools would
// reach a nil service instead of answering ErrUnavailable. This pins that hazard so the wiring
// (flowMCPOptions, which never produces it) stays the only supplier.
func TestWithFlowService_should_BoxATypedNil_When_GivenANilPointer(t *testing.T) {
	t.Parallel()
	got := FlowServiceFromOptions(WithFlowService((*backlogflow.BacklogFlowService)(nil)))

	assert.False(t, got == nil, "documented hazard: a typed-nil pointer is a non-nil interface")
}
