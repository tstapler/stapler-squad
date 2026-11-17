package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
	"google.golang.org/protobuf/proto"
)

const (
	flowDefaultWindowDays = 30
	flowMaxWindowDays     = 365
	// flowBoundedWindowDays is the window the "window too large" hint advises.
	flowBoundedWindowDays = 90
	flowAllTimeHint       = "use all_time=true for all-time"
)

// flowHandlers serves the flow-observability tools by calling the same
// in-process BacklogFlowService the RPC handler uses and marshalling its
// response with protojson. MCP-only work here: argument clamping, sanitizing
// untrusted text and error wording; no FlowSummary field is mapped.
type flowHandlers struct {
	svc          *backlogflow.BacklogFlowService
	enabledCheck func() bool
}

// guard returns an error result when the tools cannot run, nil when they can.
func (h *flowHandlers) guard() *mcpgo.CallToolResult {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r
	}
	if h.svc == nil {
		return errResult(ErrUnavailable, "the flow service is not available on this server configuration", "")
	}
	return nil
}

// flowIntArg reads a numeric tool argument; present is false when absent or not a number.
// Values saturate to the int32 range so a later int32 conversion cannot wrap
// a huge argument into a small valid one.
func flowIntArg(args map[string]any, name string) (n int, present bool) {
	switch v := args[name].(type) {
	case float64:
		return saturateInt32(v), true
	case int:
		return saturateInt32(float64(v)), true
	case json.Number:
		f, err := v.Float64()
		return saturateInt32(f), err == nil
	}
	return 0, false
}

func saturateInt32(f float64) int {
	return int(max(min(f, math.MaxInt32), math.MinInt32))
}

func flowStringArg(args map[string]any, name string) string {
	s, _ := args[name].(string)
	return strings.TrimSpace(s)
}

func flowStringListArg(args map[string]any, name string) []string {
	raw, _ := args[name].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// flowWindowArgs resolves window_days/all_time: default 30 days, clamped to 365,
// and 0 or less is refused so no magic number means all-time.
func flowWindowArgs(args map[string]any) (days int32, allTime bool, errRes *mcpgo.CallToolResult) {
	if b, _ := args["all_time"].(bool); b {
		return 0, true, nil
	}
	n, present := flowIntArg(args, "window_days")
	switch {
	case !present:
		return flowDefaultWindowDays, false, nil
	case n < 1:
		return 0, false, errResult(ErrInvalidArgument, fmt.Sprintf("window_days must be 1..%d; %s", flowMaxWindowDays, flowAllTimeHint), "")
	}
	return int32(min(n, flowMaxWindowDays)), false, nil
}

var windowTooLargeCounts = regexp.MustCompile(`\((\d+) rows, limit (\d+)\)`)

// flowErrorResult turns a service error into the tool error envelope.
func flowErrorResult(err error) *mcpgo.CallToolResult {
	msg := err.Error()
	var ce *connect.Error
	if errors.As(err, &ce) {
		msg = ce.Message()
	}
	switch connect.CodeOf(err) {
	case connect.CodeFailedPrecondition:
		return errResult(ErrUnavailable, sanitizeFlowLeaf(msg), "")
	case connect.CodeInvalidArgument:
		return errResult(ErrInvalidArgument, sanitizeFlowError(msg), "")
	case connect.CodeResourceExhausted:
		return resourceExhaustedResult(msg)
	case connect.CodeDeadlineExceeded, connect.CodeCanceled:
		return errResult(ErrInternalError, "flow query timed out or was cancelled; retry with a shorter window", "")
	}
	return errResult(ErrInternalError, sanitizeFlowLeaf(msg), "")
}

func resourceExhaustedResult(msg string) *mcpgo.CallToolResult {
	if !strings.Contains(msg, "window too large") {
		return errResult(ErrUnavailable, "flow aggregator busy: "+sanitizeFlowLeaf(msg)+"; retry shortly", "")
	}
	text := fmt.Sprintf("window too large, use window_days <= %d", flowBoundedWindowDays)
	if m := windowTooLargeCounts.FindStringSubmatch(msg); m != nil {
		text += fmt.Sprintf(" (%s rows, limit %s)", m[1], m[2])
	}
	return errResult(ErrInvalidArgument, text, "")
}

const (
	unknownDrillKeyPrefix = `unknown drill key "`
	validKeysSeparator    = `"; valid keys: `
	// validKeysMaxCount bounds the valid-keys list echoed to the agent.
	validKeysMaxCount = 60
)

// sanitizeFlowError sanitizes an RPC error message. For an unknown-drill-key
// error the echoed (caller-supplied) key is sanitized and truncated and the
// server-generated valid-keys list is bounded. The list follows the key, so the
// separator is found from the right: a key that contains the separator itself
// cannot move the split and smuggle text into the list.
func sanitizeFlowError(msg string) string {
	rest, ok := strings.CutPrefix(msg, unknownDrillKeyPrefix)
	if !ok {
		return sanitizeFlowLeaf(msg)
	}
	i := strings.LastIndex(rest, validKeysSeparator)
	if i < 0 {
		return sanitizeFlowLeaf(msg)
	}
	key, valid := rest[:i], rest[i+len(validKeysSeparator):]
	return unknownDrillKeyPrefix + sanitizeFlowKeyEcho(key) + validKeysSeparator + boundValidKeys(valid)
}

// boundValidKeys keeps the first validKeysMaxCount stable keys of a
// comma-separated list; per-week bucket keys ("<series>@<week>") are dropped
// since their form is in the tool description.
func boundValidKeys(list string) string {
	var kept []string
	dropped := 0
	for _, k := range strings.Split(list, ", ") {
		switch {
		case strings.Contains(k, "@"):
		case len(kept) >= validKeysMaxCount:
			dropped++
		default:
			kept = append(kept, sanitizeFlowKeyEcho(k))
		}
	}
	out := strings.Join(kept, ", ")
	if dropped > 0 {
		out += fmt.Sprintf(", ... (%d more; see the tool description)", dropped)
	}
	return out
}

// flowReply turns a service response or error into the tool result.
func flowReply[M any, PM interface {
	*M
	proto.Message
}](resp *connect.Response[M], err error) *mcpgo.CallToolResult {
	if err != nil {
		return flowErrorResult(err)
	}
	return flowResult(PM(resp.Msg))
}

// flowResult sanitizes and marshals a service response.
func flowResult(msg proto.Message) *mcpgo.CallToolResult {
	sanitizeFlowMessage(msg)
	raw, merr := backlogflow.FlowJSONOptions().Marshal(msg)
	if merr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("marshal flow response: %v", merr), "")
	}
	var compact bytes.Buffer
	if cerr := json.Compact(&compact, raw); cerr != nil {
		return errResult(ErrInternalError, fmt.Sprintf("compact flow response: %v", cerr), "")
	}
	return mcpgo.NewToolResultText(withUntrustedMarker(compact.String(), flowUntrustedFields(msg)))
}

// withUntrustedMarker puts an untrusted_text object first in the JSON object
// body, so the warning is read before any free text; fields empty means none.
func withUntrustedMarker(body string, fields []string) string {
	if len(fields) == 0 || !strings.HasPrefix(body, "{") {
		return body
	}
	marker, _ := json.Marshal(map[string]any{"notice": flowUntrustedNotice, "fields": fields})
	rest := body[1:]
	if rest != "}" {
		rest = "," + rest
	}
	return `{"untrusted_text":` + string(marker) + rest
}

func (h *flowHandlers) getFlowSummary(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := h.guard(); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	days, allTime, bad := flowWindowArgs(args)
	if bad != nil {
		return bad, nil
	}
	compact, _ := args["compact"].(bool)
	limit, _ := flowIntArg(args, "list_limit")
	resp, err := h.svc.GetFlowSummary(ctx, connect.NewRequest(&sessionv1.GetFlowSummaryRequest{
		WindowDays: days, AllTime: allTime, Sections: flowStringListArg(args, "sections"),
		Compact: compact, ListLimit: int32(max(limit, 0)),
	}))
	return flowReply(resp, err), nil
}

func (h *flowHandlers) listFlowDrilldown(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := h.guard(); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	key := flowStringArg(args, "key")
	if key == "" {
		return errResult(ErrInvalidArgument, "key is required", "Take a drill_key from get_flow_summary."), nil
	}
	days, allTime, bad := flowWindowArgs(args)
	if bad != nil {
		return bad, nil
	}
	limit, _ := flowIntArg(args, "limit")
	offset, _ := flowIntArg(args, "offset")
	resp, err := h.svc.ListFlowDrilldown(ctx, connect.NewRequest(&sessionv1.ListFlowDrilldownRequest{
		Key: key, WindowDays: days, AllTime: allTime, Limit: int32(max(limit, 0)), Offset: int32(max(offset, 0)),
	}))
	return flowReply(resp, err), nil
}

const (
	compareOffsetForm = "offsets (a_days, b_days, b_offset_days)"
	compareDateForm   = "absolute dates (a_start, a_end, b_start, b_end)"
)

func compareFormError(detail string) *mcpgo.CallToolResult {
	return errResult(ErrInvalidArgument,
		fmt.Sprintf("compare_flow_windows takes either %s or %s, not both and not all_time: %s", compareOffsetForm, compareDateForm, detail), "")
}

// parseFlowDate reads YYYY-MM-DD as UTC midnight, or RFC3339.
func parseFlowDate(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not YYYY-MM-DD or RFC3339", s)
	}
	return t.UTC(), nil
}

func (h *flowHandlers) compareFlowWindows(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := h.guard(); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	if b, _ := args["all_time"].(bool); b {
		return compareFormError("compare needs bounded windows"), nil
	}
	msg := &sessionv1.CompareFlowWindowsRequest{}
	hasOffsets := false
	for name, dst := range map[string]*int32{"a_days": &msg.ADays, "b_days": &msg.BDays, "b_offset_days": &msg.BOffsetDays} {
		if n, ok := flowIntArg(args, name); ok {
			*dst, hasOffsets = int32(max(n, 0)), true
		}
	}
	hasDates, errRes := setCompareDates(args, msg)
	if errRes != nil {
		return errRes, nil
	}
	if hasOffsets && hasDates {
		return compareFormError("both forms were given"), nil
	}
	resp, err := h.svc.CompareFlowWindows(ctx, connect.NewRequest(msg))
	return flowReply(resp, err), nil
}
