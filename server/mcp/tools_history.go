package mcp

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

type historyHandlers struct {
	svc *services.SessionService
}

// maxSnippetsPerResult and maxSnippetLen bound how much of a single
// search_claude_history result an LLM caller pays for in context — a heavily
// matched session can otherwise dominate the response with duplicate
// context.
const (
	maxSnippetsPerResult = 3
	maxSnippetLen        = 300
)

// truncateSearchSnippets caps a result's snippets to maxCount and truncates
// each snippet's text via session.SanitizeForAgentContext.
func truncateSearchSnippets(snippets []*sessionv1.SearchSnippet, maxCount, maxLen int) []SnippetResult {
	if len(snippets) > maxCount {
		snippets = snippets[:maxCount]
	}
	out := make([]SnippetResult, len(snippets))
	for i, sn := range snippets {
		out[i] = SnippetResult{
			Text:        session.SanitizeForAgentContext(sn.Text, maxLen),
			MessageRole: sn.MessageRole,
		}
	}
	return out
}

// --- search_claude_history ---

func (h *historyHandlers) searchClaudeHistory(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return errResult(ErrInvalidArgument, "query is required", ""), nil
	}

	limit := 10
	if lf, ok := args["limit"].(float64); ok && lf > 0 {
		limit = int(lf)
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if of, ok := args["offset"].(float64); ok && of > 0 {
		offset = int(of)
	}

	if h.svc == nil {
		return errResult(ErrInternalError, "session service unavailable on this server configuration", ""), nil
	}

	// SearchClaudeHistory runs a full IncrementalSync on every call (see
	// project_plans/mcp-search-list-tools's Risk Control) — log the call
	// duration so a cost spike is visible if this bound proves wrong in
	// practice.
	start := time.Now()
	resp, err := h.svc.SearchClaudeHistory(ctx, connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{
		Query:  query,
		Limit:  int32(limit),
		Offset: int32(offset),
	}))
	log.DebugLog().Printf("[mcp:search_claude_history] query=%q duration=%s", query, time.Since(start))
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("failed to search claude history: %v", err), ""), nil
	}

	results := make([]SearchResultSummary, len(resp.Msg.Results))
	for i, r := range resp.Msg.Results {
		results[i] = SearchResultSummary{
			SessionID:   r.SessionId,
			SessionName: r.SessionName,
			Project:     r.Project,
			Score:       r.Score,
			Snippets:    truncateSearchSnippets(r.Snippets, maxSnippetsPerResult, maxSnippetLen),
		}
	}

	return okResult(SearchClaudeHistoryResult{
		MCPResult:   MCPResult{Success: true},
		Results:     results,
		TotalCount:  int(resp.Msg.TotalMatches),
		HasMore:     resp.Msg.HasMore,
		QueryTimeMs: resp.Msg.QueryTimeMs,
	}), nil
}

// --- Registration ---

// registerHistoryTools registers the Claude/session-history search tool.
// Not feature-flag-gated (see registerNotificationTools). project/model/
// start_time/end_time are omitted from the schema — see ADR-001.
func registerHistoryTools(s *mcpserver.MCPServer, h *historyHandlers) {
	s.AddTool(
		mcpgo.NewTool("search_claude_history",
			mcpgo.WithDescription("Full-text search across Claude conversation history. Default limit is 10 to avoid filling LLM context (the underlying search defaults to 20); each result's snippets are truncated to keep one heavily-matched conversation from dominating the response."),
			mcpgo.WithString("query",
				mcpgo.Description("Search query, matched via full-text (BM25) search"),
				mcpgo.Required(),
			),
			mcpgo.WithNumber("limit",
				mcpgo.Description("Max results (default 10, max 100)"),
				mcpgo.DefaultNumber(10),
				mcpgo.Min(1),
				mcpgo.Max(100),
			),
			mcpgo.WithNumber("offset",
				mcpgo.Description("Number of results to skip, for pagination"),
				mcpgo.DefaultNumber(0),
				mcpgo.Min(0),
			),
		),
		h.searchClaudeHistory,
	)
}
