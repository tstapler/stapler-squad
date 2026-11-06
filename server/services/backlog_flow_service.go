package services

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/flow"
)

var _ sessionv1connect.BacklogFlowServiceHandler = (*BacklogFlowService)(nil)

// FlowAggregator is the slice of *flow.Aggregator the service needs, so tests
// can substitute a fake.
type FlowAggregator interface {
	Summary(ctx context.Context, q flow.FlowQuery) (*flow.FlowSummary, error)
	Compare(ctx context.Context, q flow.CompareQuery) (*flow.CompareResult, error)
	Drilldown(ctx context.Context, q flow.DrilldownQuery) (*flow.DrilldownPage, error)
	ItemTimeline(ctx context.Context, q flow.ItemTimelineQuery) (*flow.ItemTimeline, error)
}

const allTimeHint = "use all_time=true for all-time"

// BacklogFlowService is a translate-only ConnectRPC adapter over the flow
// aggregator; no flow math lives here.
type BacklogFlowService struct {
	agg FlowAggregator
	// enabled reports the backlog feature flag; nil means always enabled.
	enabled func() bool
}

// flowMaxWindowDays is the longest bounded window; longer requests are clamped
// (all_time exists for the rest). The MCP tools clamp to the same value.
const flowMaxWindowDays = 365

// SetEnabledCheck gates every handler on the backlog feature flag, as the MCP
// flow tools are. Call it before the service starts serving.
func (s *BacklogFlowService) SetEnabledCheck(enabled func() bool) { s.enabled = enabled }

// NewBacklogFlowService returns a service over agg. A nil agg yields
// FailedPrecondition from every handler.
func NewBacklogFlowService(agg FlowAggregator) *BacklogFlowService {
	return &BacklogFlowService{agg: agg}
}

// NewFlowServiceFromStorage builds the reader and its own aggregator from
// storage (the stdio-fallback path). A nil storage or ent client yields a
// service whose handlers return FailedPrecondition. opts configure the
// aggregator (the server passes flow.WithObserver(NewFlowObserver())).
func NewFlowServiceFromStorage(storage *session.Storage, opts ...flow.AggregatorOption) *BacklogFlowService {
	if storage == nil || storage.GetEntClient() == nil {
		return &BacklogFlowService{}
	}
	reader := session.NewEntFlowReader(storage.GetEntClient())
	src := flow.Sources{Events: reader, Verdicts: reader, Stuck: reader, Items: reader, Markers: reader}
	// Prepended so a caller-supplied WithCaptureSince still wins.
	opts = append([]flow.AggregatorOption{flow.WithCaptureSince(reader.CaptureSince)}, opts...)
	return &BacklogFlowService{agg: flow.NewAggregator(src, flow.SystemClock{}, opts...)}
}

func (s *BacklogFlowService) ready() error {
	if s.enabled != nil && !s.enabled() {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("the backlog feature is disabled"))
	}
	if s.agg == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("flow aggregator is not available"))
	}
	if v := reflect.ValueOf(s.agg); v.Kind() == reflect.Pointer && v.IsNil() {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("flow aggregator is not available"))
	}
	return nil
}

func windowSpec(days int32, allTime bool) (flow.WindowSpec, error) {
	if allTime {
		return flow.WindowSpec{AllTime: true}, nil
	}
	if days < 1 {
		return flow.WindowSpec{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("window_days must be >= 1; %s", allTimeHint))
	}
	return flow.WindowSpec{Days: int(min(days, flowMaxWindowDays))}, nil
}

// flowError maps an aggregator error to a connect code: caller mistakes are
// InvalidArgument, a busy aggregator is ResourceExhausted, the rest Internal.
func flowError(err error) error {
	var ce *connect.Error
	switch {
	case errors.As(err, &ce):
		return err
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case errors.Is(err, flow.ErrCompareAllTime), isFlowInputError(err.Error()):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, flow.ErrWindowTooLarge):
		return connect.NewError(connect.CodeResourceExhausted, windowTooLargeError(err))
	case errors.Is(err, flow.ErrBusy):
		return connect.NewError(connect.CodeResourceExhausted, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

// windowTooLargeError names the refused row count and the ceiling when the
// aggregator reported them, and tells the caller how to narrow the window.
func windowTooLargeError(err error) error {
	var wl *flow.WindowTooLargeError
	if errors.As(err, &wl) {
		return fmt.Errorf("window too large (%d rows, limit %d); use a shorter window", wl.Rows, wl.Limit)
	}
	return fmt.Errorf("window too large; use a shorter window")
}

// isFlowInputError recognises the aggregator's validation messages, which are
// plain errors without sentinels.
func isFlowInputError(msg string) bool {
	for _, p := range []string{"unknown section", "unknown drill key", "compare:", "flow window"} {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}

func sectionNames(in []string) []flow.SectionName {
	return mapSlice(in, func(s string) flow.SectionName { return flow.SectionName(strings.TrimSpace(s)) })
}

// GetFlowSummary returns the sectioned flow summary.
// +api: backlog_flow:get_summary
func (s *BacklogFlowService) GetFlowSummary(
	ctx context.Context, req *connect.Request[sessionv1.GetFlowSummaryRequest],
) (*connect.Response[sessionv1.GetFlowSummaryResponse], error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	m := req.Msg
	w, err := windowSpec(m.WindowDays, m.AllTime)
	if err != nil {
		return nil, err
	}
	sum, err := s.agg.Summary(ctx, flow.FlowQuery{
		Window: w, Sections: sectionNames(m.Sections), Compact: m.Compact, ListLimit: int(m.ListLimit),
	})
	if err != nil {
		return nil, flowError(err)
	}
	return connect.NewResponse(&sessionv1.GetFlowSummaryResponse{Summary: flowSummaryToProto(sum)}), nil
}

// ListDeadlockCandidates returns the review section's deadlock list.
// +api: backlog_flow:list_deadlock_candidates
func (s *BacklogFlowService) ListDeadlockCandidates(
	ctx context.Context, req *connect.Request[sessionv1.ListDeadlockCandidatesRequest],
) (*connect.Response[sessionv1.ListDeadlockCandidatesResponse], error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	w, err := windowSpec(req.Msg.WindowDays, req.Msg.AllTime)
	if err != nil {
		return nil, err
	}
	sum, err := s.agg.Summary(ctx, flow.FlowQuery{
		Window: w, Sections: []flow.SectionName{flow.SectionReview}, ListLimit: int(req.Msg.Limit),
	})
	if err != nil {
		return nil, flowError(err)
	}
	return connect.NewResponse(&sessionv1.ListDeadlockCandidatesResponse{
		Meta: flowMetaToProto(sum.Meta), Status: sectionStatusToProto(sum.Review.Meta),
		Deadlocks: deadlockPage(sum.Review.Deadlocks), WeakFilter: sum.Review.WeakFilter,
	}), nil
}

// ListThrashEpisodes returns the anomalies section's thrash items.
// +api: backlog_flow:list_thrash_episodes
func (s *BacklogFlowService) ListThrashEpisodes(
	ctx context.Context, req *connect.Request[sessionv1.ListThrashEpisodesRequest],
) (*connect.Response[sessionv1.ListThrashEpisodesResponse], error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	w, err := windowSpec(req.Msg.WindowDays, req.Msg.AllTime)
	if err != nil {
		return nil, err
	}
	sum, err := s.agg.Summary(ctx, flow.FlowQuery{
		Window: w, Sections: []flow.SectionName{flow.SectionAnomalies}, ListLimit: int(req.Msg.Limit),
	})
	if err != nil {
		return nil, flowError(err)
	}
	return connect.NewResponse(&sessionv1.ListThrashEpisodesResponse{
		Meta: flowMetaToProto(sum.Meta), Status: sectionStatusToProto(sum.Anomalies.Meta),
		Items: thrashPage(sum.Anomalies.Items),
	}), nil
}

// GetItemFlowTimeline returns one item's transition timeline.
// +api: backlog_flow:get_item_timeline
func (s *BacklogFlowService) GetItemFlowTimeline(
	ctx context.Context, req *connect.Request[sessionv1.GetItemFlowTimelineRequest],
) (*connect.Response[sessionv1.GetItemFlowTimelineResponse], error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Msg.ItemId) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("item_id is required"))
	}
	w, err := windowSpec(req.Msg.WindowDays, req.Msg.AllTime)
	if err != nil {
		return nil, err
	}
	tl, err := s.agg.ItemTimeline(ctx, flow.ItemTimelineQuery{ItemID: flow.ItemID(req.Msg.ItemId), Window: w})
	if err != nil {
		return nil, flowError(err)
	}
	return connect.NewResponse(timelineToProto(tl)), nil
}

// CompareFlowWindows compares two bounded windows.
// +api: backlog_flow:compare_windows
func (s *BacklogFlowService) CompareFlowWindows(
	ctx context.Context, req *connect.Request[sessionv1.CompareFlowWindowsRequest],
) (*connect.Response[sessionv1.CompareFlowWindowsResponse], error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	m := req.Msg
	q := flow.CompareQuery{ADays: int(m.ADays), BDays: int(m.BDays), BOffsetDays: int(m.BOffsetDays)}
	if m.AStart != nil {
		q.AStart = m.AStart.AsTime()
	}
	if m.AEnd != nil {
		q.AEnd = m.AEnd.AsTime()
	}
	if m.BStart != nil {
		q.BStart = m.BStart.AsTime()
	}
	if m.BEnd != nil {
		q.BEnd = m.BEnd.AsTime()
	}
	res, err := s.agg.Compare(ctx, q)
	if err != nil {
		return nil, flowError(err)
	}
	return connect.NewResponse(compareResultToProto(res)), nil
}

// ListFlowDrilldown lists the items behind one drill key.
// +api: backlog_flow:list_drilldown
func (s *BacklogFlowService) ListFlowDrilldown(
	ctx context.Context, req *connect.Request[sessionv1.ListFlowDrilldownRequest],
) (*connect.Response[sessionv1.ListFlowDrilldownResponse], error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	m := req.Msg
	if strings.TrimSpace(m.Key) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("key is required"))
	}
	w, err := windowSpec(m.WindowDays, m.AllTime)
	if err != nil {
		return nil, err
	}
	page, err := s.agg.Drilldown(ctx, flow.DrilldownQuery{
		Key: flow.DrillKey(m.Key), Query: flow.FlowQuery{Window: w}, Limit: int(m.Limit), Offset: int(m.Offset),
	})
	if err != nil {
		return nil, flowError(err)
	}
	return connect.NewResponse(drilldownToProto(page)), nil
}
