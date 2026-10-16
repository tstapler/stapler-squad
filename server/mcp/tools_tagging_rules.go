package mcp

import (
	"context"
	"fmt"
	"regexp"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
)

// taggingRulesHandlers implements session-tagging-rule-management MCP tools, sibling to
// rulesHandlers' approval-rule tools (per requirements.md: "add sibling MCP tools... not a
// new MCP namespace"). Tagging rules govern which tags auto-apply to a session based on its
// name/branch/path/program — a different decision from approval rules' tool-call gating.
type taggingRulesHandlers struct {
	svc *services.SessionService
}

// registerTaggingRulesTools registers the session-tagging-rule-related MCP tools.
func registerTaggingRulesTools(s *mcpserver.MCPServer, h *taggingRulesHandlers) {
	s.AddTool(
		mcpgo.NewTool("list_tagging_rules",
			mcpgo.WithDescription("List session-tagging rules that auto-apply tags to sessions based on name/branch/path/program."),
		),
		h.listTaggingRules,
	)

	s.AddTool(
		mcpgo.NewTool("upsert_tagging_rule",
			mcpgo.WithDescription("Create or update a user-defined session-tagging rule. Pass an existing rule's id to update it, or omit id to create a new one."),
			mcpgo.WithString("id", mcpgo.Description("Rule ID to update; omit to create a new rule")),
			mcpgo.WithString("name", mcpgo.Description("Human-readable rule name"), mcpgo.Required()),
			mcpgo.WithString("name_pattern", mcpgo.Description("Regex pattern matching the session title/name")),
			mcpgo.WithString("branch_pattern", mcpgo.Description("Regex pattern matching the session's git branch")),
			mcpgo.WithString("path_pattern", mcpgo.Description("Regex pattern matching the session's working directory path")),
			mcpgo.WithString("program_pattern", mcpgo.Description("Regex pattern matching the session's program (e.g. claude, aider)")),
			mcpgo.WithArray("required_tags", mcpgo.Description("Tags the session must already have for this rule to match"), mcpgo.Items(map[string]any{"type": "string"})),
			mcpgo.WithString("output_tag", mcpgo.Description("Tag applied when this rule matches"), mcpgo.Required()),
			mcpgo.WithNumber("priority", mcpgo.Description("Rule priority; higher runs first (default: 0)")),
			mcpgo.WithBoolean("enabled", mcpgo.Description("Whether the rule is active (default: true)"), mcpgo.DefaultBool(true)),
		),
		h.upsertTaggingRule,
	)

	s.AddTool(
		mcpgo.NewTool("delete_tagging_rule",
			mcpgo.WithDescription("Delete a user-defined session-tagging rule by ID."),
			mcpgo.WithString("id", mcpgo.Description("Rule ID to delete"), mcpgo.Required()),
		),
		h.deleteTaggingRule,
	)
}

// TaggingRuleResult is the wire representation of a tagging rule returned by MCP tools.
type TaggingRuleResult struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	NamePattern    string   `json:"name_pattern,omitempty"`
	BranchPattern  string   `json:"branch_pattern,omitempty"`
	PathPattern    string   `json:"path_pattern,omitempty"`
	ProgramPattern string   `json:"program_pattern,omitempty"`
	RequiredTags   []string `json:"required_tags,omitempty"`
	OutputTag      string   `json:"output_tag"`
	Priority       int32    `json:"priority"`
	Enabled        bool     `json:"enabled"`
	Source         string   `json:"source,omitempty"`
	FireCount7d    int32    `json:"fire_count_7d"`
}

func taggingRuleToResult(r *sessionv1.TaggingRuleProto) TaggingRuleResult {
	return TaggingRuleResult{
		ID: r.GetId(), Name: r.GetName(), NamePattern: r.GetNamePattern(), BranchPattern: r.GetBranchPattern(),
		PathPattern: r.GetPathPattern(), ProgramPattern: r.GetProgramPattern(), RequiredTags: r.GetRequiredTags(),
		OutputTag: r.GetOutputTag(), Priority: r.GetPriority(), Enabled: r.GetEnabled(), Source: r.GetSource(),
		FireCount7d: r.GetFireCount_7D(),
	}
}

// validateTaggingRulePatternArgs compiles every provided regex pattern argument, returning
// the first compile error found (or nil if all patterns are valid/absent). Mirrors
// validateTaggingRuleSpec's server-side check, but runs before the RPC round-trip so an
// invalid pattern comes back as an MCP tool error, never a panic.
func validateTaggingRulePatternArgs(args map[string]any) error {
	for _, key := range []string{"name_pattern", "branch_pattern", "path_pattern", "program_pattern"} {
		pat := stringArg(args, key)
		if pat == "" {
			continue
		}
		if _, err := regexp.Compile(pat); err != nil {
			return fmt.Errorf("invalid regex for %s %q: %w", key, pat, err)
		}
	}
	return nil
}

type ListTaggingRulesResult struct {
	MCPResult
	Rules []TaggingRuleResult `json:"rules"`
}

func (h *taggingRulesHandlers) listTaggingRules(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	resp, err := h.svc.ListTaggingRules(ctx, connect.NewRequest(&sessionv1.ListTaggingRulesRequest{}))
	if err != nil {
		return workflowServiceErrResult(err)
	}
	out := make([]TaggingRuleResult, 0, len(resp.Msg.GetRules()))
	for _, r := range resp.Msg.GetRules() {
		out = append(out, taggingRuleToResult(r))
	}
	return okResult(ListTaggingRulesResult{MCPResult: MCPResult{Success: true}, Rules: out}), nil
}

type UpsertTaggingRuleResult struct {
	MCPResult
	Rule    TaggingRuleResult `json:"rule"`
	Created bool              `json:"created"`
}

// taggingRuleProtoFromArgs builds the wire-shaped rule from validated MCP tool arguments.
func taggingRuleProtoFromArgs(args map[string]any, name, outputTag string) *sessionv1.TaggingRuleProto {
	enabled := true
	if v, ok := args["enabled"].(bool); ok {
		enabled = v
	}
	var priority int32
	if v, ok := args["priority"].(float64); ok {
		priority = int32(v)
	}
	return &sessionv1.TaggingRuleProto{
		Id:             stringArg(args, "id"),
		Name:           name,
		NamePattern:    stringArg(args, "name_pattern"),
		BranchPattern:  stringArg(args, "branch_pattern"),
		PathPattern:    stringArg(args, "path_pattern"),
		ProgramPattern: stringArg(args, "program_pattern"),
		RequiredTags:   stringArrayArg(args, "required_tags"),
		OutputTag:      outputTag,
		Priority:       priority,
		Enabled:        enabled,
	}
}

func (h *taggingRulesHandlers) upsertTaggingRule(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	name, _ := args["name"].(string)
	if name == "" {
		return errResult(ErrInvalidArgument, "name is required", ""), nil
	}
	outputTag, _ := args["output_tag"].(string)
	if outputTag == "" {
		return errResult(ErrInvalidArgument, "output_tag is required", ""), nil
	}
	if err := validateTaggingRulePatternArgs(args); err != nil {
		return errResult(ErrInvalidArgument, err.Error(), ""), nil
	}

	rule := taggingRuleProtoFromArgs(args, name, outputTag)
	resp, err := h.svc.UpsertTaggingRule(ctx, connect.NewRequest(&sessionv1.UpsertTaggingRuleRequest{Rule: rule}))
	if err != nil {
		return workflowServiceErrResult(err)
	}
	return okResult(UpsertTaggingRuleResult{
		MCPResult: MCPResult{Success: true},
		Rule:      taggingRuleToResult(resp.Msg.GetRule()),
		Created:   resp.Msg.GetCreated(),
	}), nil
}

func (h *taggingRulesHandlers) deleteTaggingRule(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	id, _ := args["id"].(string)
	if id == "" {
		return errResult(ErrInvalidArgument, "id is required", ""), nil
	}
	resp, err := h.svc.DeleteTaggingRule(ctx, connect.NewRequest(&sessionv1.DeleteTaggingRuleRequest{Id: id}))
	if err != nil {
		return workflowServiceErrResult(err)
	}
	return okResult(MCPResult{Success: resp.Msg.GetSuccess()}), nil
}
