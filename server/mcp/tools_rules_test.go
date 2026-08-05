package mcp

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/server/services"
)

func newTestRulesHandlers(t *testing.T) *rulesHandlers {
	t.Helper()
	svc := services.NewSessionService(newTestBacklogStorage(t), nil)
	return &rulesHandlers{svc: svc}
}

func TestUpsertApprovalRule_should_CreateRule_When_IdProvided(t *testing.T) {
	h := newTestRulesHandlers(t)

	res, err := h.upsertApprovalRule(context.Background(), makeToolReq(map[string]interface{}{
		"id":       "my-rule-id",
		"name":     "Allow git status",
		"decision": "auto_allow",
	}))
	if err != nil {
		t.Fatalf("upsertApprovalRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
	rule, ok := out["rule"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected rule field in result, got: %+v", out)
	}
	if rule["id"] != "my-rule-id" {
		t.Errorf("expected id=my-rule-id, got %v", rule["id"])
	}
	if rule["name"] != "Allow git status" {
		t.Errorf("expected name=Allow git status, got %v", rule["name"])
	}
	if rule["decision"] != "auto_allow" {
		t.Errorf("expected decision=auto_allow, got %v", rule["decision"])
	}
}

func TestUpsertApprovalRule_should_ReturnInvalidArgument_When_IdOmitted(t *testing.T) {
	// The tool description advertises "omit id to create a new rule", but the
	// underlying RulesService currently requires a non-empty Rule.Id
	// unconditionally and nothing auto-generates one along this path. This
	// test documents the actual current behavior rather than the advertised
	// one; see the discrepancy noted for a follow-up fix.
	h := newTestRulesHandlers(t)

	res, err := h.upsertApprovalRule(context.Background(), makeToolReq(map[string]interface{}{
		"name": "New rule without id",
	}))
	if err != nil {
		t.Fatalf("upsertApprovalRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false when id is omitted, got: %+v", out)
	}
}

func TestUpsertApprovalRule_should_ReturnInvalidArgument_When_NameMissing(t *testing.T) {
	h := newTestRulesHandlers(t)

	res, err := h.upsertApprovalRule(context.Background(), makeToolReq(map[string]interface{}{
		"id": "some-id",
	}))
	if err != nil {
		t.Fatalf("upsertApprovalRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false when name is missing, got: %+v", out)
	}
}

func TestListApprovalRules_should_ReturnCreatedRule_When_RuleExists(t *testing.T) {
	h := newTestRulesHandlers(t)

	if _, err := h.upsertApprovalRule(context.Background(), makeToolReq(map[string]interface{}{
		"id":   "listed-rule",
		"name": "Listed rule",
	})); err != nil {
		t.Fatalf("setup upsertApprovalRule failed: %v", err)
	}

	res, err := h.listApprovalRules(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("listApprovalRules returned error: %v", err)
	}
	out := parseResult(t, res)
	rules, ok := out["rules"].([]interface{})
	if !ok {
		t.Fatalf("expected rules field in result, got: %+v", out)
	}
	found := false
	for _, r := range rules {
		rule, ok := r.(map[string]interface{})
		if ok && rule["id"] == "listed-rule" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected listed-rule to be present in list, got: %+v", rules)
	}
}

func TestDeleteApprovalRule_should_RemoveRule_When_RuleExists(t *testing.T) {
	h := newTestRulesHandlers(t)

	if _, err := h.upsertApprovalRule(context.Background(), makeToolReq(map[string]interface{}{
		"id":   "to-delete",
		"name": "To delete",
	})); err != nil {
		t.Fatalf("setup upsertApprovalRule failed: %v", err)
	}

	res, err := h.deleteApprovalRule(context.Background(), makeToolReq(map[string]interface{}{
		"id": "to-delete",
	}))
	if err != nil {
		t.Fatalf("deleteApprovalRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
}

func TestDeleteApprovalRule_should_ReturnNotFound_When_RuleDoesNotExist(t *testing.T) {
	h := newTestRulesHandlers(t)

	res, err := h.deleteApprovalRule(context.Background(), makeToolReq(map[string]interface{}{
		"id": "does-not-exist",
	}))
	if err != nil {
		t.Fatalf("deleteApprovalRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false for missing rule, got: %+v", out)
	}
}

func TestDeleteApprovalRule_should_ReturnInvalidArgument_When_IdMissing(t *testing.T) {
	h := newTestRulesHandlers(t)

	res, err := h.deleteApprovalRule(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("deleteApprovalRule returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false when id is missing, got: %+v", out)
	}
}
