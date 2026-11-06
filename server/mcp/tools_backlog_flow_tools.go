package mcp

import (
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/flow"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// setCompareDates reads the absolute-date arguments into msg. hasDates is true
// when any of the four was supplied; a partial or malformed set is an error.
func setCompareDates(args map[string]any, msg *sessionv1.CompareFlowWindowsRequest) (hasDates bool, errRes *mcpgo.CallToolResult) {
	dst := map[string]**timestamppb.Timestamp{
		"a_start": &msg.AStart, "a_end": &msg.AEnd, "b_start": &msg.BStart, "b_end": &msg.BEnd,
	}
	for _, name := range []string{"a_start", "a_end", "b_start", "b_end"} {
		raw := flowStringArg(args, name)
		if raw == "" {
			continue
		}
		t, err := parseFlowDate(raw)
		if err != nil {
			return true, compareFormError(fmt.Sprintf("%s: %v", name, err))
		}
		*dst[name], hasDates = timestamppb.New(t), true
	}
	if hasDates && (msg.AStart == nil || msg.AEnd == nil || msg.BStart == nil || msg.BEnd == nil) {
		return true, compareFormError("absolute dates need all of a_start, a_end, b_start and b_end")
	}
	return hasDates, nil
}

// drillKeyEnum is the registry-backed enum for list_flow_drilldown's key.
func drillKeyEnum() []string {
	keys := flow.DrillKeys()
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = string(k)
	}
	return out
}

func windowOptions() []mcpgo.ToolOption {
	return []mcpgo.ToolOption{
		mcpgo.WithNumber("window_days",
			mcpgo.Description("Look back this many days from now (1..365, default 30). 0 is rejected; pass all_time=true for all-time."),
			mcpgo.DefaultNumber(flowDefaultWindowDays), mcpgo.Min(1), mcpgo.Max(flowMaxWindowDays)),
		mcpgo.WithBoolean("all_time",
			mcpgo.Description("All-time window (slow path; refused above the row ceiling). window_days is ignored when set.")),
	}
}

func getFlowSummaryTool() mcpgo.Tool {
	all := flow.AllSections()
	sections := make([]any, 0, len(all))
	for _, s := range all {
		sections = append(sections, string(s))
	}
	opts := append([]mcpgo.ToolOption{
		mcpgo.WithDescription("Start here for backlog flow health: one sectioned summary (tiles, funnel, causes, review, pr, stuck, anomalies, trends, notes) answering how fast work merges, how often it bounces and why. " +
			"Every number carries a drill_key; pass it to list_flow_drilldown to see the items behind it. Use compare_flow_windows for before/after, and list_flow_deadlock_candidates or list_flow_thrash_episodes for offenders. " +
			"as_of is the cache age (the stdio MCP process keeps its own cache). Top-N lists default to 10, max 50, and carry total_count/returned_count/has_more/truncated so a clamped list is never mistaken for the whole set. " +
			`Example: {"window_days":30,"sections":["tiles","review"],"compact":true}`)},
		windowOptions()...)
	opts = append(opts,
		mcpgo.WithArray("sections", mcpgo.Description("Sections to compute; the rest report status \"skipped\". Empty means all."),
			mcpgo.Items(map[string]any{"type": "string", "enum": sections})),
		mcpgo.WithBoolean("compact", mcpgo.Description("Omit trend buckets and limit every top-N list to 3.")),
		mcpgo.WithNumber("list_limit", mcpgo.Description("Top-N list size (default 10, max 50)."), mcpgo.Min(1), mcpgo.Max(50)),
	)
	return mcpgo.NewTool("get_flow_summary", opts...)
}

func compareFlowWindowsTool() mcpgo.Tool {
	return mcpgo.NewTool("compare_flow_windows",
		mcpgo.WithDescription("Compare two bounded windows metric by metric (A, B, delta, N_A, N_B). Give either offsets (a_days, b_days, b_offset_days) or absolute dates (a_start, a_end, b_start, b_end); not both, and not all-time. "+
			"Dates are YYYY-MM-DD (UTC midnight) or RFC3339 and bounds are half-open [start, end): end is exclusive, so to include Oct 19 as the last day of A pass a_end=2026-10-20. "+
			"Anchor on a tuning commit's ship date this way. Deltas are descriptive: no significance test is computed, and a metric below the minimum sample is reported as incomparable with a reason. "+
			`Example: (A covers Oct 6-19, B covers Oct 20-Nov 2) {"a_start":"2026-10-06","a_end":"2026-10-20","b_start":"2026-10-20","b_end":"2026-11-03"}; or {"a_days":30,"b_days":30,"b_offset_days":30}`),
		mcpgo.WithNumber("a_days", mcpgo.Description("Window A length in days (offset form)."), mcpgo.Min(1), mcpgo.Max(flowMaxWindowDays)),
		mcpgo.WithNumber("b_days", mcpgo.Description("Window B length in days (offset form)."), mcpgo.Min(1), mcpgo.Max(flowMaxWindowDays)),
		mcpgo.WithNumber("b_offset_days", mcpgo.Description("B ends this many days before now; A ends where B starts."), mcpgo.Min(0)),
		mcpgo.WithString("a_start", mcpgo.Description("Window A start, inclusive (date form).")),
		mcpgo.WithString("a_end", mcpgo.Description("Window A end, exclusive (date form).")),
		mcpgo.WithString("b_start", mcpgo.Description("Window B start, inclusive (date form).")),
		mcpgo.WithString("b_end", mcpgo.Description("Window B end, exclusive; clamped to now when in the future (date form).")),
	)
}

func listFlowDrilldownTool() mcpgo.Tool {
	opts := append([]mcpgo.ToolOption{
		mcpgo.WithDescription("List the items behind one number: pass the drill_key beside it in get_flow_summary. Paged: limit defaults to 10 and is clamped to 50; total_count is the full item count and has_more says whether more exist (use offset). " +
			"Besides the keys enumerated here, dynamic keys of the form " + strings.Join(flow.DrillKeyPatterns(), ", ") + " are accepted. " +
			`Example: {"key":"rework.review_fail","window_days":30,"limit":10}`),
		mcpgo.WithString("key", mcpgo.Description("Drill key from get_flow_summary."), mcpgo.Required(), mcpgo.Enum(drillKeyEnum()...)),
	}, windowOptions()...)
	opts = append(opts,
		mcpgo.WithNumber("limit", mcpgo.Description("Page size (default 10, max 50)."), mcpgo.DefaultNumber(10), mcpgo.Min(1), mcpgo.Max(50)),
		mcpgo.WithNumber("offset", mcpgo.Description("Items to skip, for paging."), mcpgo.DefaultNumber(0), mcpgo.Min(0)),
	)
	return mcpgo.NewTool("list_flow_drilldown", opts...)
}

// registerFlowTools registers the flow tools; handlers return ErrUnavailable
// when the flow service was not injected.
func registerFlowTools(s *mcpserver.MCPServer, h *flowHandlers) {
	s.AddTool(getFlowSummaryTool(), h.getFlowSummary)
	s.AddTool(compareFlowWindowsTool(), h.compareFlowWindows)
	s.AddTool(listFlowDrilldownTool(), h.listFlowDrilldown)
}
