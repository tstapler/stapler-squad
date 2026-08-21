package mcp

import (
	"context"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
)

// rulesHandlers implements auto-approval-rule-management MCP tools. Rules
// govern whether a tool call is auto-allowed, auto-denied, or escalated for
// human review during a session.
type rulesHandlers struct {
	svc *services.SessionService
}

// registerRulesTools registers the approval-rule-related MCP tools.
func registerRulesTools(s *mcpserver.MCPServer, h *rulesHandlers) {
	s.AddTool(
		mcpgo.NewTool("list_approval_rules",
			mcpgo.WithDescription("List auto-approval rules that govern whether tool calls are auto-allowed, auto-denied, or escalated for review."),
			mcpgo.WithString("source_filter", mcpgo.Description("Optional: only return rules from this source (user, seed, claude-settings)")),
		),
		h.listApprovalRules,
	)

	s.AddTool(
		mcpgo.NewTool("upsert_approval_rule",
			mcpgo.WithDescription("Create or update a user-defined auto-approval rule. Pass an existing rule's id to update it, or omit id to create a new one."),
			mcpgo.WithString("id", mcpgo.Description("Rule ID to update; omit to create a new rule")),
			mcpgo.WithString("name", mcpgo.Description("Human-readable rule name"), mcpgo.Required()),
			mcpgo.WithString("tool_name", mcpgo.Description("Exact tool name this rule applies to (e.g. Bash, Edit)")),
			mcpgo.WithString("tool_pattern", mcpgo.Description("Regex pattern matching tool names this rule applies to")),
			mcpgo.WithString("tool_category", mcpgo.Description("Tool category this rule applies to"), mcpgo.Enum("builtin", "builtin-agent", "mcp", "mcp-read", "mcp-write")),
			mcpgo.WithString("command_pattern", mcpgo.Description("Regex pattern matching Bash command text")),
			mcpgo.WithString("file_pattern", mcpgo.Description("Glob/regex pattern matching file paths for Edit/Write/Read tools")),
			mcpgo.WithString("decision", mcpgo.Description("What to do when this rule matches"), mcpgo.DefaultString("escalate"), mcpgo.Enum("auto_allow", "auto_deny", "escalate")),
			mcpgo.WithString("risk_level", mcpgo.Description("Risk level this rule represents"), mcpgo.DefaultString("medium"), mcpgo.Enum("low", "medium", "high", "critical")),
			mcpgo.WithString("reason", mcpgo.Description("Explanation shown to the user for why this rule fires")),
			mcpgo.WithString("alternative", mcpgo.Description("Suggested alternative action when this rule denies a call")),
			mcpgo.WithNumber("priority", mcpgo.Description("Rule priority; higher runs first (default: 0)")),
			mcpgo.WithBoolean("enabled", mcpgo.Description("Whether the rule is active (default: true)"), mcpgo.DefaultBool(true)),
			mcpgo.WithArray("programs", mcpgo.Description("Bash programs this rule matches (e.g. git, npm)"), mcpgo.Items(map[string]any{"type": "string"})),
			mcpgo.WithArray("subcommands", mcpgo.Description("Bash subcommands this rule matches (e.g. status, log)"), mcpgo.Items(map[string]any{"type": "string"})),
			mcpgo.WithArray("blocked_subcommands", mcpgo.Description("Bash subcommands this rule blocks"), mcpgo.Items(map[string]any{"type": "string"})),
			mcpgo.WithArray("required_flags", mcpgo.Description("Flags that must be present for this rule to match"), mcpgo.Items(map[string]any{"type": "string"})),
			mcpgo.WithArray("forbidden_flags", mcpgo.Description("Flags that must NOT be present for this rule to match"), mcpgo.Items(map[string]any{"type": "string"})),
		),
		h.upsertApprovalRule,
	)

	s.AddTool(
		mcpgo.NewTool("delete_approval_rule",
			mcpgo.WithDescription("Delete a user-defined auto-approval rule by ID."),
			mcpgo.WithString("id", mcpgo.Description("Rule ID to delete"), mcpgo.Required()),
		),
		h.deleteApprovalRule,
	)

	s.AddTool(
		mcpgo.NewTool("reload_claude_settings_rules",
			mcpgo.WithDescription("Re-parse ~/.claude/settings.json (and project-level equivalents) and hot-swap the resulting rules into the live classifier, without a server restart."),
		),
		h.reloadClaudeSettingsRules,
	)
}

// ApprovalRuleResult is the wire representation of an approval rule returned by MCP tools.
type ApprovalRuleResult struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	ToolName           string   `json:"tool_name,omitempty"`
	ToolPattern        string   `json:"tool_pattern,omitempty"`
	ToolCategory       string   `json:"tool_category,omitempty"`
	CommandPattern     string   `json:"command_pattern,omitempty"`
	FilePattern        string   `json:"file_pattern,omitempty"`
	Decision           string   `json:"decision"`
	RiskLevel          string   `json:"risk_level"`
	Reason             string   `json:"reason,omitempty"`
	Alternative        string   `json:"alternative,omitempty"`
	Priority           int32    `json:"priority"`
	Enabled            bool     `json:"enabled"`
	Source             string   `json:"source,omitempty"`
	Programs           []string `json:"programs,omitempty"`
	Subcommands        []string `json:"subcommands,omitempty"`
	BlockedSubcommands []string `json:"blocked_subcommands,omitempty"`
	RequiredFlags      []string `json:"required_flags,omitempty"`
	ForbiddenFlags     []string `json:"forbidden_flags,omitempty"`
}

func ruleToResult(r *sessionv1.ApprovalRuleProto) ApprovalRuleResult {
	return ApprovalRuleResult{
		ID: r.GetId(), Name: r.GetName(), ToolName: r.GetToolName(), ToolPattern: r.GetToolPattern(),
		ToolCategory: r.GetToolCategory(), CommandPattern: r.GetCommandPattern(), FilePattern: r.GetFilePattern(),
		Decision: mcpAutoDecisionToString(r.GetDecision()), RiskLevel: r.GetRiskLevel(), Reason: r.GetReason(),
		Alternative: r.GetAlternative(), Priority: r.GetPriority(), Enabled: r.GetEnabled(), Source: r.GetSource(),
		Programs: r.GetPrograms(), Subcommands: r.GetSubcommands(), BlockedSubcommands: r.GetBlockedSubcommands(),
		RequiredFlags: r.GetRequiredFlags(), ForbiddenFlags: r.GetForbiddenFlags(),
	}
}

func mcpAutoDecisionToString(d sessionv1.AutoDecision) string {
	switch d {
	case sessionv1.AutoDecision_AUTO_DECISION_ALLOW:
		return "auto_allow"
	case sessionv1.AutoDecision_AUTO_DECISION_DENY:
		return "auto_deny"
	default:
		return "escalate"
	}
}

func mcpStringToAutoDecision(s string) sessionv1.AutoDecision {
	switch s {
	case "auto_allow":
		return sessionv1.AutoDecision_AUTO_DECISION_ALLOW
	case "auto_deny":
		return sessionv1.AutoDecision_AUTO_DECISION_DENY
	default:
		return sessionv1.AutoDecision_AUTO_DECISION_ESCALATE
	}
}

func stringArrayArg(args map[string]any, key string) []string {
	raw, ok := args[key].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

type ListApprovalRulesResult struct {
	MCPResult
	Rules []ApprovalRuleResult `json:"rules"`
}

func (h *rulesHandlers) listApprovalRules(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	protoReq := &sessionv1.ListApprovalRulesRequest{SourceFilter: stringPtrArg(args, "source_filter")}
	resp, err := h.svc.ListApprovalRules(ctx, connect.NewRequest(protoReq))
	if err != nil {
		return workflowServiceErrResult(err)
	}
	out := make([]ApprovalRuleResult, 0, len(resp.Msg.GetRules()))
	for _, r := range resp.Msg.GetRules() {
		out = append(out, ruleToResult(r))
	}
	return okResult(ListApprovalRulesResult{MCPResult: MCPResult{Success: true}, Rules: out}), nil
}

type UpsertApprovalRuleResult struct {
	MCPResult
	Rule    ApprovalRuleResult `json:"rule"`
	Created bool               `json:"created"`
}

func (h *rulesHandlers) upsertApprovalRule(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	name, _ := args["name"].(string)
	if name == "" {
		return errResult(ErrInvalidArgument, "name is required", ""), nil
	}
	decision, _ := args["decision"].(string)
	if decision == "" {
		decision = "escalate"
	}
	riskLevel, _ := args["risk_level"].(string)
	if riskLevel == "" {
		riskLevel = "medium"
	}
	enabled := true
	if v, ok := args["enabled"].(bool); ok {
		enabled = v
	}
	var priority int32
	if v, ok := args["priority"].(float64); ok {
		priority = int32(v)
	}
	rule := &sessionv1.ApprovalRuleProto{
		Id:                 stringArg(args, "id"),
		Name:               name,
		ToolName:           stringArg(args, "tool_name"),
		ToolPattern:        stringArg(args, "tool_pattern"),
		ToolCategory:       stringArg(args, "tool_category"),
		CommandPattern:     stringArg(args, "command_pattern"),
		FilePattern:        stringArg(args, "file_pattern"),
		Decision:           mcpStringToAutoDecision(decision),
		RiskLevel:          riskLevel,
		Reason:             stringArg(args, "reason"),
		Alternative:        stringArg(args, "alternative"),
		Priority:           priority,
		Enabled:            enabled,
		Programs:           stringArrayArg(args, "programs"),
		Subcommands:        stringArrayArg(args, "subcommands"),
		BlockedSubcommands: stringArrayArg(args, "blocked_subcommands"),
		RequiredFlags:      stringArrayArg(args, "required_flags"),
		ForbiddenFlags:     stringArrayArg(args, "forbidden_flags"),
	}
	resp, err := h.svc.UpsertApprovalRule(ctx, connect.NewRequest(&sessionv1.UpsertApprovalRuleRequest{Rule: rule}))
	if err != nil {
		return workflowServiceErrResult(err)
	}
	return okResult(UpsertApprovalRuleResult{
		MCPResult: MCPResult{Success: true},
		Rule:      ruleToResult(resp.Msg.GetRule()),
		Created:   resp.Msg.GetCreated(),
	}), nil
}

func (h *rulesHandlers) deleteApprovalRule(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	id, _ := args["id"].(string)
	if id == "" {
		return errResult(ErrInvalidArgument, "id is required", ""), nil
	}
	resp, err := h.svc.DeleteApprovalRule(ctx, connect.NewRequest(&sessionv1.DeleteApprovalRuleRequest{Id: id}))
	if err != nil {
		return workflowServiceErrResult(err)
	}
	return okResult(MCPResult{Success: resp.Msg.GetSuccess()}), nil
}

type ReloadClaudeSettingsRulesResult struct {
	MCPResult
	RuleCount int32  `json:"rule_count"`
	Message   string `json:"message"`
}

func (h *rulesHandlers) reloadClaudeSettingsRules(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	resp, err := h.svc.ReloadClaudeSettingsRules(ctx, connect.NewRequest(&sessionv1.ReloadClaudeSettingsRulesRequest{}))
	if err != nil {
		return workflowServiceErrResult(err)
	}
	return okResult(ReloadClaudeSettingsRulesResult{
		MCPResult: MCPResult{Success: resp.Msg.GetSuccess()},
		RuleCount: resp.Msg.GetRuleCount(),
		Message:   resp.Msg.GetMessage(),
	}), nil
}
