package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// importTrackerIssue creates a backlog item from an already-fetched Linear or
// JIRA issue — the shared tail of import_linear_issue / import_jira_issue,
// mirroring importGitHubIssue's claim check, create, and auto-triage steps.
func (h *backlogHandlers) importTrackerIssue(ctx context.Context, toolName, tracker string, args map[string]any, ext session.ExternalItem) *mcpgo.CallToolResult {
	repoPath, _ := args["repo_path"].(string)
	skipTriage, _ := args["skip_triage"].(bool)

	if h.backlogSvc != nil {
		if verdict, claimErr := h.backlogSvc.LookupCrossHostClaim(ctx, ext.URL); claimErr == nil && verdict.Kind == services.ClaimHeldByOther {
			return errResult(ErrInvalidArgument,
				fmt.Sprintf("issue %s is already claimed by host %s (see %s)", ext.URL, verdict.Record.ClaimingHostID.String(), verdict.Record.ItemDeepLink),
				"Open that link to see the owning item, or import from the web UI with an override reason.")
		}
	}

	title, description := session.BoundImportedText(ext.Title, ext.Description)
	created, err := h.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:       title,
		Description: description,
		Priority:    ext.Priority,
		Status:      string(session.BacklogStatusIdea),
		RepoPath:    repoPath,
		Labels:      ext.Labels,
		Notes:       fmt.Sprintf("Imported from %s issue %s (%s)", tracker, ext.ExternalID, ext.URL),
		ExternalID:  ext.ExternalID,
		ExternalURL: ext.URL,
	})
	if err != nil {
		return errResult(ErrInternalError, fmt.Sprintf("create backlog item: %v", err), "")
	}
	log.InfoLog().Printf("[mcp:%s] session=%s item=%s issue=%s", toolName, callerSessionUUIDForAudit(ctx), created.ID, ext.URL)

	triageNote := " Auto-triage not triggered (no repo_path, skip_triage set, or triage unavailable)."
	if h.backlogSvc != nil && h.backlogSvc.MaybeTriggerTriage(ctx, created.ID, skipTriage, created.RepoPath) {
		triageNote = " Auto-triage started."
	}
	return mcpgo.NewToolResultText(fmt.Sprintf(
		"Imported backlog item %s: %q from %s (status: idea, priority: P%d).%s", created.ID, created.Title, ext.URL, created.Priority, triageNote))
}

func trackerFetchError(tracker string, err error) *mcpgo.CallToolResult {
	return errResult(ErrInternalError, fmt.Sprintf("fetch %s issue: %v", tracker, err),
		"Retry — rate limits and network errors are usually transient. If it names missing credentials or a failed authentication, that is not transient; set LINEAR_API_KEY (Linear) or JIRA_BASE_URL/JIRA_EMAIL/JIRA_API_TOKEN (JIRA), or store them in the keychain.")
}

// importLinearIssue is the one-off Linear import (the sync loop's ItemSource
// polling is the bulk path). The API key comes from the keychain.
func (h *backlogHandlers) importLinearIssue(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	ref, _ := args["issue_url"].(string)
	if strings.TrimSpace(ref) == "" {
		return errResult(ErrInvalidArgument, "issue_url is required", ""), nil
	}
	identifier, err := session.ParseLinearIssueRef(ref)
	if err != nil {
		return errResult(ErrInvalidArgument, err.Error(), "Pass a Linear URL like https://linear.app/org/issue/ENG-123 or an identifier like ENG-123."), nil
	}
	fetch := h.fetchLinearIssue
	if fetch == nil {
		fetch = func(ctx context.Context, id string) (session.ExternalItem, error) {
			return session.NewLinearPlugin().FetchIssue(ctx, session.PluginConfig{}, id)
		}
	}
	ext, err := fetch(ctx, identifier)
	if err != nil {
		return trackerFetchError("Linear", err), nil
	}
	return h.importTrackerIssue(ctx, "import_linear_issue", "Linear", args, ext), nil
}

// importJiraIssue is the one-off JIRA Cloud import. Credentials come from the
// keychain, keyed by the JIRA site's host.
func (h *backlogHandlers) importJiraIssue(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if r := featureDisabledResult(h.enabledCheck); r != nil {
		return r, nil
	}
	args := req.GetArguments()
	ref, _ := args["issue_url"].(string)
	if strings.TrimSpace(ref) == "" {
		return errResult(ErrInvalidArgument, "issue_url is required", ""), nil
	}
	jref, err := session.ParseJiraIssueRef(ref)
	if err != nil {
		return errResult(ErrInvalidArgument, err.Error(), "Pass a JIRA URL like https://org.atlassian.net/browse/ENG-123, or a bare key plus base_url."), nil
	}
	if override, _ := args["base_url"].(string); jref.BaseURL == "" {
		jref.BaseURL = strings.TrimSpace(override)
	}
	if jref.BaseURL == "" {
		return errResult(ErrInvalidArgument, "base_url is required when issue_url is a bare issue key", ""), nil
	}
	fetch := h.fetchJiraIssue
	if fetch == nil {
		fetch = func(ctx context.Context, r session.JiraIssueRef) (session.ExternalItem, error) {
			raw, _ := json.Marshal(map[string]string{"base_url": r.BaseURL})
			return session.NewJiraPlugin().FetchIssue(ctx, session.PluginConfig{Raw: string(raw)}, r.Key)
		}
	}
	ext, err := fetch(ctx, jref)
	if err != nil {
		return trackerFetchError("JIRA", err), nil
	}
	return h.importTrackerIssue(ctx, "import_jira_issue", "JIRA", args, ext), nil
}

func (h *backlogHandlers) registerTrackerImportTools(s *mcpserver.MCPServer) {
	repoPathDesc := "Absolute local filesystem path this item targets, e.g. /home/user/Programming/my-repo — NOT a bare repo name. Omit for a repo-less item."
	skipTriageDesc := "Skip auto-triage on creation (default false: an item with a repo_path is triaged automatically). Set true to leave the item in idea status untouched."

	s.AddTool(
		mcpgo.NewTool("import_linear_issue",
			mcpgo.WithDescription("Create a backlog item pre-populated from a Linear issue (title, description, labels, priority, and a link back as ExternalURL). Requires LINEAR_API_KEY (or the key stored in the keychain)."),
			mcpgo.WithString("issue_url", mcpgo.Description("Linear issue URL, e.g. https://linear.app/org/issue/ENG-123, or an identifier like ENG-123"), mcpgo.Required()),
			mcpgo.WithString("repo_path", mcpgo.Description(repoPathDesc)),
			mcpgo.WithBoolean("skip_triage", mcpgo.Description(skipTriageDesc)),
		),
		h.importLinearIssue,
	)
	s.AddTool(
		mcpgo.NewTool("import_jira_issue",
			mcpgo.WithDescription("Create a backlog item pre-populated from a JIRA Cloud issue (summary, ADF description as plaintext, labels, priority, and a link back as ExternalURL). Requires JIRA_BASE_URL, JIRA_EMAIL and JIRA_API_TOKEN (or the credentials stored in the keychain for that site)."),
			mcpgo.WithString("issue_url", mcpgo.Description("JIRA issue URL, e.g. https://org.atlassian.net/browse/ENG-123, or a bare key like ENG-123 together with base_url"), mcpgo.Required()),
			mcpgo.WithString("base_url", mcpgo.Description("JIRA site base URL, e.g. https://org.atlassian.net. Only needed when issue_url is a bare key.")),
			mcpgo.WithString("repo_path", mcpgo.Description(repoPathDesc)),
			mcpgo.WithBoolean("skip_triage", mcpgo.Description(skipTriageDesc)),
		),
		h.importJiraIssue,
	)
}
