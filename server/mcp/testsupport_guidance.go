package mcp

import (
	"context"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/tstapler/stapler-squad/session"
)

// ResolveItemLinkViaMCPAdapter exercises the exact resolveItemLink code path
// every mutating backlog MCP tool uses. Exported (mirrors
// session.NewTestEntRepository's precedent for a cross-package test-support
// function in a regular, non-_test.go file) solely so
// server/services/guidance_request_ownership_parity_test.go can verify the
// ConnectRPC GuidanceRequestService's own backlog-item ownership check
// produces an identical (error code, remediation text) pair to this one for
// the same fixture — guarding against the MCP and RPC adapters drifting
// apart from the session.ResolveItemLink helper they both wrap.
func ResolveItemLinkViaMCPAdapter(ctx context.Context, storage *session.Storage, callerUUID, itemID string) (session.ItemSessionSummary, *mcpgo.CallToolResult) {
	h := &backlogHandlers{storage: storage}
	return h.resolveItemLink(ctx, callerUUID, itemID)
}
