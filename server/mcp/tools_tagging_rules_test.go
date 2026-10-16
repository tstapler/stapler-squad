package mcp

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/server/services"
)

func newTestTaggingRulesHandlers(t *testing.T) *taggingRulesHandlers {
	t.Helper()
	svc := services.NewSessionService(newTestBacklogStorage(t), nil)
	return &taggingRulesHandlers{svc: svc}
}

// TestMCP_UpsertTaggingRule_should_AppearInListTaggingRules_When_ValidRuleUpserted covers
// Story 5.1.1's Given-When-Then: upsertTaggingRule followed by listTaggingRules shows the
// new rule with output_tag == "Hotfix", even with no id supplied.
func TestMCP_UpsertTaggingRule_should_AppearInListTaggingRules_When_ValidRuleUpserted(t *testing.T) {
	h := newTestTaggingRulesHandlers(t)

	upsertRes, err := h.upsertTaggingRule(context.Background(), makeToolReq(map[string]interface{}{
		"name":           "Hotfix branch",
		"branch_pattern": "^hotfix/",
		"output_tag":     "Hotfix",
		"priority":       float64(50),
	}))
	if err != nil {
		t.Fatalf("upsertTaggingRule returned error: %v", err)
	}
	upsertOut := parseResult(t, upsertRes)
	if success, _ := upsertOut["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", upsertOut)
	}

	listRes, err := h.listTaggingRules(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("listTaggingRules returned error: %v", err)
	}
	listOut := parseResult(t, listRes)
	rules, ok := listOut["rules"].([]interface{})
	if !ok {
		t.Fatalf("expected rules field in result, got: %+v", listOut)
	}
	found := false
	for _, r := range rules {
		rule, ok := r.(map[string]interface{})
		if ok && rule["output_tag"] == "Hotfix" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a rule with output_tag=Hotfix to be present in list, got: %+v", rules)
	}
}

// TestMCP_UpsertTaggingRule_should_ReturnToolError_When_RegexPatternInvalid covers Story
// 5.1.1's error-handling acceptance criterion: an invalid regex returns an MCP tool error
// result, never a panic — mirrors deleteApprovalRule's error convention.
func TestMCP_UpsertTaggingRule_should_ReturnToolError_When_RegexPatternInvalid(t *testing.T) {
	h := newTestTaggingRulesHandlers(t)

	res, err := h.upsertTaggingRule(context.Background(), makeToolReq(map[string]interface{}{
		"name":           "Broken pattern",
		"branch_pattern": "(unclosed",
		"output_tag":     "Broken",
	}))
	if err != nil {
		t.Fatalf("upsertTaggingRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false for invalid regex, got: %+v", out)
	}
}

func TestMCP_DeleteTaggingRule_should_RemoveRule_When_RuleExists(t *testing.T) {
	h := newTestTaggingRulesHandlers(t)

	upsertRes, err := h.upsertTaggingRule(context.Background(), makeToolReq(map[string]interface{}{
		"id":         "to-delete",
		"name":       "To delete",
		"output_tag": "ToDelete",
	}))
	if err != nil {
		t.Fatalf("setup upsertTaggingRule failed: %v", err)
	}
	if success, _ := parseResult(t, upsertRes)["success"].(bool); !success {
		t.Fatalf("setup upsertTaggingRule did not succeed")
	}

	res, err := h.deleteTaggingRule(context.Background(), makeToolReq(map[string]interface{}{
		"id": "to-delete",
	}))
	if err != nil {
		t.Fatalf("deleteTaggingRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
}

func TestMCP_UpsertTaggingRule_should_ReturnInvalidArgument_When_OutputTagMissing(t *testing.T) {
	h := newTestTaggingRulesHandlers(t)

	res, err := h.upsertTaggingRule(context.Background(), makeToolReq(map[string]interface{}{
		"name": "No output tag",
	}))
	if err != nil {
		t.Fatalf("upsertTaggingRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false when output_tag is missing, got: %+v", out)
	}
}
