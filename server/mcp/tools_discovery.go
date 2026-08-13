package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/tstapler/stapler-squad/session"
)

type discoveryHandlers struct {
	store session.InstanceStore
}

// paginationCursor encodes the last-seen session title for list_sessions pagination.
type paginationCursor struct {
	LastTitle string    `json:"last_title"`
	CreatedAt time.Time `json:"created_at"`
}

func encodeCursor(title string, createdAt time.Time) string {
	c := paginationCursor{LastTitle: title, CreatedAt: createdAt}
	b, _ := json.Marshal(c)
	return base64.StdEncoding.EncodeToString(b)
}

func decodeCursor(s string) (*paginationCursor, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var c paginationCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func instanceToSummary(inst *session.Instance) SessionSummary {
	tags := inst.Tags
	if tags == nil {
		tags = []string{}
	}
	lastActivity := inst.UpdatedAt
	if inst.CreatedAt.After(lastActivity) {
		lastActivity = inst.CreatedAt
	}
	return SessionSummary{
		ID:             inst.Title,
		Title:          inst.Title,
		Status:         inst.Status.String(),
		Tags:           tags,
		Branch:         inst.Branch,
		Path:           inst.Path,
		CreatedAt:      inst.CreatedAt,
		LastActivityAt: lastActivity,
	}
}

func instanceToDetail(inst *session.Instance) SessionDetail {
	return SessionDetail{
		SessionSummary: instanceToSummary(inst),
		Program:        inst.Program,
		SessionType:    string(inst.SessionType),
		WorkingDir:     inst.WorkingDir,
	}
}

func errResult(code, message, remediation string) *mcpgo.CallToolResult {
	result := MCPResult{Success: false, Error: &MCPError{Code: code, Message: message, Remediation: remediation}}
	b, _ := json.Marshal(result)
	return mcpgo.NewToolResultText(string(b))
}

func okResult(v any) *mcpgo.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("marshal response: %v", err), "")
	}
	return mcpgo.NewToolResultText(string(b))
}

func (d *discoveryHandlers) listSessions(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	statusFilter, _ := args["status_filter"].(string)
	limitF, _ := args["limit"].(float64)
	cursorStr, _ := args["cursor"].(string)

	limit := 10
	if limitF > 0 {
		limit = int(limitF)
	}
	if limit > 100 {
		limit = 100
	}

	instances, err := d.store.LoadInstances()
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), ""), nil
	}

	// Apply status filter.
	if statusFilter != "" {
		filtered := instances[:0]
		for _, inst := range instances {
			if strings.EqualFold(inst.Status.String(), statusFilter) {
				filtered = append(filtered, inst)
			}
		}
		instances = filtered
	}

	totalCount := len(instances)

	// Apply cursor (skip sessions up to and including the cursor position).
	if cursorStr != "" {
		cursor, err := decodeCursor(cursorStr)
		if err != nil {
			return errResult(ErrInvalidArgument, "invalid cursor", "Use the next_cursor value from a previous list_sessions response."), nil
		}
		start := 0
		for i, inst := range instances {
			if inst.Title == cursor.LastTitle {
				start = i + 1
				break
			}
		}
		instances = instances[start:]
	}

	// Apply limit.
	var nextCursor *string
	if len(instances) > limit {
		last := instances[limit-1]
		c := encodeCursor(last.Title, last.CreatedAt)
		nextCursor = &c
		instances = instances[:limit]
	}

	summaries := make([]SessionSummary, len(instances))
	for i, inst := range instances {
		summaries[i] = instanceToSummary(inst)
	}

	return okResult(ListSessionsResult{
		MCPResult:  MCPResult{Success: true},
		Sessions:   summaries,
		TotalCount: totalCount,
		NextCursor: nextCursor,
	}), nil
}

func (d *discoveryHandlers) getSession(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return errResult(ErrInvalidArgument, "session_id is required", ""), nil
	}

	instances, err := d.store.LoadInstances()
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), ""), nil
	}

	for _, inst := range instances {
		if inst.MatchesID(sessionID) {
			detail := instanceToDetail(inst)
			return okResult(GetSessionResult{
				MCPResult: MCPResult{Success: true},
				Session:   &detail,
			}), nil
		}
	}

	return errResult(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID),
		"Use list_sessions or search_sessions to find valid session IDs."), nil
}

func (d *discoveryHandlers) searchSessions(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errResult(ErrInvalidArgument, "query is required", ""), nil
	}
	limitF, _ := args["limit"].(float64)
	limit := 10
	if limitF > 0 {
		limit = int(limitF)
	}
	if limit > 50 {
		limit = 50
	}

	// Extract tag_filter as []string.
	var tagFilter []string
	if raw, ok := args["tag_filter"]; ok {
		if arr, ok := raw.([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					tagFilter = append(tagFilter, s)
				}
			}
		}
	}

	instances, err := d.store.LoadInstances()
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("load sessions: %v", err), ""), nil
	}

	queryLower := strings.ToLower(query)
	var matched []*session.Instance
	for _, inst := range instances {
		if matchesSearch(inst, queryLower, tagFilter) {
			matched = append(matched, inst)
		}
	}

	totalCount := len(matched)
	if len(matched) > limit {
		matched = matched[:limit]
	}

	summaries := make([]SessionSummary, len(matched))
	for i, inst := range matched {
		summaries[i] = instanceToSummary(inst)
	}

	return okResult(SearchSessionsResult{
		MCPResult:  MCPResult{Success: true},
		Sessions:   summaries,
		TotalCount: totalCount,
	}), nil
}

func matchesSearch(inst *session.Instance, queryLower string, tagFilter []string) bool {
	// Must match all required tags.
	for _, required := range tagFilter {
		found := false
		for _, t := range inst.Tags {
			if strings.EqualFold(t, required) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Must match query in at least one field.
	if queryLower == "" {
		return true
	}
	searchable := strings.ToLower(inst.Title + " " + inst.Path + " " + inst.Branch + " " + strings.Join(inst.Tags, " "))
	return strings.Contains(searchable, queryLower)
}
