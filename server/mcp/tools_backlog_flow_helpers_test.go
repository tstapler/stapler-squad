package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/flow"
)

// fakeFlowAgg is a services.FlowAggregator returning canned results.
type fakeFlowAgg struct {
	summary  *flow.FlowSummary
	compare  *flow.CompareResult
	drill    *flow.DrilldownPage
	timeline *flow.ItemTimeline
	err      error

	gotSummary  flow.FlowQuery
	gotDrill    flow.DrilldownQuery
	gotTimeline flow.ItemTimelineQuery
}

func (f *fakeFlowAgg) Summary(_ context.Context, q flow.FlowQuery) (*flow.FlowSummary, error) {
	f.gotSummary = q
	return f.summary, f.err
}

func (f *fakeFlowAgg) Compare(context.Context, flow.CompareQuery) (*flow.CompareResult, error) {
	return f.compare, f.err
}

func (f *fakeFlowAgg) Drilldown(_ context.Context, q flow.DrilldownQuery) (*flow.DrilldownPage, error) {
	f.gotDrill = q
	return f.drill, f.err
}

func (f *fakeFlowAgg) ItemTimeline(_ context.Context, q flow.ItemTimelineQuery) (*flow.ItemTimeline, error) {
	f.gotTimeline = q
	return f.timeline, f.err
}

// memFlowSources is an in-memory flow.Sources over a fixed event list.
type memFlowSources struct {
	events []flow.Event
	titles map[flow.ItemID]string
}

func (m *memFlowSources) sources() flow.Sources {
	return flow.Sources{Events: m, Verdicts: m, Stuck: m, Items: m}
}

func (m *memFlowSources) Events(_ context.Context, r flow.EventRange) ([]flow.Event, error) {
	var out []flow.Event
	for _, e := range m.events {
		if !e.At.Before(r.From) && (r.To.IsZero() || e.At.Before(r.To)) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memFlowSources) CountEvents(ctx context.Context, r flow.EventRange) (int, error) {
	evs, err := m.Events(ctx, r)
	return len(evs), err
}

func (m *memFlowSources) StreamEvents(ctx context.Context, r flow.EventRange, fn func([]flow.Event) error) error {
	evs, err := m.Events(ctx, r)
	if err != nil {
		return err
	}
	return fn(evs)
}

func (m *memFlowSources) LastEventsBefore(context.Context, time.Time, []flow.ItemID) ([]flow.Event, error) {
	return nil, nil
}

func (m *memFlowSources) Verdicts(context.Context, flow.FlowWindow) ([]flow.Verdict, error) {
	return nil, nil
}

func (m *memFlowSources) AgentDoneClaims(context.Context, flow.FlowWindow) (map[flow.ItemID]bool, error) {
	return map[flow.ItemID]bool{}, nil
}

func (m *memFlowSources) StuckIntervals(context.Context) ([]flow.StuckInterval, error) {
	return nil, nil
}

func (m *memFlowSources) Items(_ context.Context, ids []flow.ItemID) ([]flow.ItemMeta, error) {
	out := make([]flow.ItemMeta, 0, len(ids))
	for _, id := range ids {
		out = append(out, flow.ItemMeta{ID: id, Title: m.titles[id], Status: domain.BacklogStatusDone})
	}
	return out, nil
}

func (m *memFlowSources) OpenItemIDs(context.Context) ([]flow.ItemID, error) { return nil, nil }

func (m *memFlowSources) Stages(context.Context) ([]domain.BacklogStatus, error) {
	return []domain.BacklogStatus{
		domain.BacklogStatusIdea, domain.BacklogStatusInProgress, domain.BacklogStatusReview,
		domain.BacklogStatusPRPending, domain.BacklogStatusDone,
	}, nil
}

var flowToolNow = time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)

// mergedEvent is a pr_pending to done transition for item i at t.
func mergedEvent(i int, t time.Time) flow.Event {
	return flow.Event{
		ItemID: flow.ItemID(fmt.Sprintf("item-%03d", i)), From: domain.BacklogStatusPRPending,
		To: domain.BacklogStatusDone, At: t, Seq: int64(i),
	}
}

func newRealFlowHandlers(src *memFlowSources) *flowHandlers {
	agg := flow.NewAggregator(src.sources(), flow.FixedClock{T: flowToolNow})
	return &flowHandlers{svc: services.NewBacklogFlowService(agg)}
}

func newFakeFlowHandlers(agg *fakeFlowAgg) *flowHandlers {
	return &flowHandlers{svc: services.NewBacklogFlowService(agg)}
}

// callFlowTool runs a handler and returns the result text and decoded JSON.
func callFlowTool(t *testing.T, h func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error), args map[string]any) (string, map[string]any) {
	t.Helper()
	res, err := h(context.Background(), makeToolReq(args))
	require.NoError(t, err)
	require.NotEmpty(t, res.Content)
	text := res.Content[0].(mcpgo.TextContent).Text
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &m), text)
	return text, m
}

func errorMessage(t *testing.T, m map[string]any) string {
	t.Helper()
	require.Equal(t, false, m["success"], "expected a tool error")
	e, ok := m["error"].(map[string]any)
	require.True(t, ok, "error envelope")
	return e["message"].(string)
}

// fieldPath reads a nested key path ("summary.meta.as_of") from decoded JSON.
func fieldPath(m map[string]any, path string) any {
	var cur any = m
	for _, k := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[k]
	}
	return cur
}
