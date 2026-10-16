package mcp

import (
	"context"
	"errors"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/tstapler/stapler-squad/server/services"
)

// claimCheckService is the slice of *services.BacklogService the
// check_cross_host_claim tool needs.
type claimCheckService interface {
	LookupCrossHostClaim(ctx context.Context, externalURL string) (services.ClaimVerdict, error)
}

type claimHandlers struct {
	svc          claimCheckService
	enabledCheck func() bool // optional; nil means always enabled
}

// CheckCrossHostClaimResult is the check_cross_host_claim tool's result. Checked
// distinguishes "confirmed clear" (claimed=false, checked=true) from "could not
// confirm" (claimed=false, checked=false).
type CheckCrossHostClaimResult struct {
	MCPResult
	Claimed        bool   `json:"claimed"`
	Checked        bool   `json:"checked"`
	ClaimingHostID string `json:"claiming_host_id,omitempty"`
	ItemDeepLink   string `json:"item_deep_link,omitempty"`
	Disputed       bool   `json:"disputed,omitempty"`
	Note           string `json:"note,omitempty"`
}

func registerClaimTools(s *mcpserver.MCPServer, h *claimHandlers) {
	s.AddTool(
		mcpgo.NewTool("check_cross_host_claim",
			mcpgo.WithDescription("Check whether another Stapler Squad host has already claimed an external URL (e.g. a GitHub issue), before starting work on it. claimed=true names the claiming host and its item deep link. claimed=false with checked=false means the check could not be completed (an unreachable peer, or the feature is disabled), not that the URL is clear."),
			mcpgo.WithString("external_url",
				mcpgo.Description("The external URL to check, e.g. https://github.com/owner/repo/issues/42"),
				mcpgo.Required(),
			),
		),
		h.checkCrossHostClaim,
	)
}

func (h *claimHandlers) checkCrossHostClaim(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	externalURL, _ := req.GetArguments()["external_url"].(string)
	externalURL = strings.TrimSpace(externalURL)
	if externalURL == "" {
		return errResult(ErrInvalidArgument, "external_url is required", ""), nil
	}

	verdict, err := h.svc.LookupCrossHostClaim(ctx, externalURL)
	if errors.Is(err, services.ErrCrossHostClaimDedupDisabled) {
		return okResult(CheckCrossHostClaimResult{MCPResult: MCPResult{Success: true}, Note: "cross_host_claim_dedup is disabled on this host"}), nil
	}
	if err != nil {
		return errResult(ErrInternalError, "claim check failed: "+err.Error(), ""), nil
	}

	out := CheckCrossHostClaimResult{MCPResult: MCPResult{Success: true}}
	switch verdict.Kind {
	case services.ClaimHeldByOther:
		out.Claimed, out.Checked = true, true
		out.ClaimingHostID = verdict.Record.ClaimingHostID.String()
		out.ItemDeepLink = verdict.Record.ItemDeepLink
		out.Disputed = verdict.Record.Disputed
	case services.ClaimCheckIndeterminate:
		out.Note = "at least one known peer was unreachable; the URL could not be confirmed clear"
	default:
		out.Checked = true
	}
	return okResult(out), nil
}
