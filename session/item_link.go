package session

import (
	"context"
	"errors"
	"fmt"

	"github.com/tstapler/stapler-squad/log"
)

// ItemLinkErrorCode classifies why ResolveItemLink failed, so a
// protocol-specific adapter (server/mcp's resolveItemLink,
// server/services' GuidanceRequestService) can translate it into its own
// error shape (mcpgo.CallToolResult vs connect.Error) without duplicating
// the underlying not-found-vs-permission-denied disambiguation.
type ItemLinkErrorCode int

const (
	// ItemLinkNotFound means the backlog item itself doesn't exist.
	ItemLinkNotFound ItemLinkErrorCode = iota
	// ItemLinkPermissionDenied means the item exists but callerUUID has no
	// ItemSession link to it.
	ItemLinkPermissionDenied
	// ItemLinkInternalError means the storage lookup itself failed (neither
	// of the above) — a genuine infrastructure error, not a caller mistake.
	ItemLinkInternalError
)

// ItemLinkError is the result of a failed ResolveItemLink call. Message
// describes what went wrong; Remediation is caller-facing guidance text,
// carried over verbatim from the resolveItemLink implementation this type
// was extracted from (server/mcp/tools_backlog.go) so both the MCP and RPC
// adapters surface identical copy — see item_link_test.go and
// server/services/guidance_request_ownership_parity_test.go.
type ItemLinkError struct {
	Code        ItemLinkErrorCode
	Message     string
	Remediation string
}

func (e *ItemLinkError) Error() string { return e.Message }

// otherBacklogToolsWarning lists every other mutating MCP tool a missing
// item-session link would also reject, so a caller that gives up on
// link_session_to_item knows not to bother retrying any of them either.
const otherBacklogToolsWarning = "If you don't fix this, stop calling ANY backlog MCP tool for this item — " +
	"report_progress, request_review, submit_review_verdict, report_pr_created, submit_triage_result, " +
	"report_blocked, report_duplicate will all fail identically for the same reason."

// ItemNotFoundRemediation is the shared not-found remediation text, exported
// so every caller of ResolveItemLink (and getBacklogItem's own existence
// check) surfaces the identical copy.
const ItemNotFoundRemediation = "This item id does not exist — do not retry any backlog MCP tool call against it. " +
	"If you were given this item id at session start, report it in your final summary; it may have been " +
	"deleted or archived out from under this session."

// ResolveItemLink verifies that callerUUID is linked to itemID, returning the
// ItemSession on success. On failure it disambiguates ItemLinkNotFound (the
// item itself doesn't exist) from ItemLinkPermissionDenied (the item exists
// but this session has no link to it) — GetItemSessionBySessionAndItem's ent
// join predicate returns ErrNotFound for both cases and cannot tell them
// apart on its own, so a second GetBacklogItem lookup is needed to decide
// which.
//
// Extracted from server/mcp/tools_backlog.go's resolveItemLink (which is now
// a thin adapter over this function) so the MCP tool and the
// GuidanceRequestService RPC handler share one implementation instead of
// maintaining two copies that could drift — see
// project_plans/durable-guidance-request/implementation/plan.md's Pattern
// Decisions "Ownership check" row.
func ResolveItemLink(ctx context.Context, storage *Storage, callerUUID, itemID string) (ItemSessionSummary, *ItemLinkError) {
	itemSession, linkErr := storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)
	if linkErr == nil {
		return itemSession, nil
	}
	if !errors.Is(linkErr, ErrNotFound) {
		return ItemSessionSummary{}, &ItemLinkError{
			Code:    ItemLinkInternalError,
			Message: fmt.Sprintf("link check failed: %v", linkErr),
		}
	}

	// Disambiguate: does the item itself exist?
	if _, itemErr := storage.GetBacklogItem(ctx, itemID); itemErr != nil {
		if errors.Is(itemErr, ErrNotFound) {
			return ItemSessionSummary{}, &ItemLinkError{
				Code:        ItemLinkNotFound,
				Message:     fmt.Sprintf("backlog item %q not found", itemID),
				Remediation: ItemNotFoundRemediation,
			}
		}
		return ItemSessionSummary{}, &ItemLinkError{
			Code:    ItemLinkInternalError,
			Message: fmt.Sprintf("get backlog item: %v", itemErr),
		}
	}

	log.InfoLog().Printf("[ResolveItemLink] session=%s not linked to existing item=%s", callerUUID, itemID)
	remediation := fmt.Sprintf("Call link_session_to_item with item_id=%s to link this session before retrying. %s", itemID, otherBacklogToolsWarning)
	if prior, priorErr := storage.GetItemSessionBySessionUUID(ctx, callerUUID); priorErr == nil && prior.BacklogItemID != "" && prior.BacklogItemID != itemID {
		remediation = fmt.Sprintf("This session is currently linked to a different item (%s). Call link_session_to_item with item_id=%s to relink, or use item_id=%s if that's what you meant. %s", prior.BacklogItemID, itemID, prior.BacklogItemID, otherBacklogToolsWarning)
	}
	return ItemSessionSummary{}, &ItemLinkError{
		Code:        ItemLinkPermissionDenied,
		Message:     fmt.Sprintf("session %s is not linked to backlog item %s", callerUUID, itemID),
		Remediation: remediation,
	}
}
