package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestReloadClaudeSettingsRules_should_ReturnRuleCountAndMessage_When_SettingsFileValid(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatalf("failed to create temp .claude dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"),
		[]byte(`{"permissions":{"allow":["Bash(git status)"]}}`), 0644); err != nil {
		t.Fatalf("failed to write temp settings.json: %v", err)
	}

	h := newTestRulesHandlers(t)

	res, err := h.reloadClaudeSettingsRules(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("reloadClaudeSettingsRules returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); !success {
		t.Fatalf("expected success=true, got: %+v", out)
	}
	if out["rule_count"] != float64(1) { // JSON numbers decode as float64
		t.Errorf("expected rule_count=1, got %v", out["rule_count"])
	}
	msg, _ := out["message"].(string)
	if !strings.Contains(msg, "Reloaded 1 claude-settings rule(s)") {
		t.Errorf("expected message to report the reload, got: %q", msg)
	}
}

// TestReloadClaudeSettingsRules_should_ReturnFailureMessage_When_SettingsFileMalformed covers
// the handler's failure path (previously untested — only the success path was covered; see
// the sibling DeleteApprovalRule_should_ReturnNotFound/InvalidArgument tests above for the
// established pattern). The service-level CodeUnimplemented branch (tools_rules.go:238-241's
// `if err != nil` case, when no ClaudeSettingsWatcher is wired) is exercised directly at
// TestReloadClaudeSettingsRules_WatcherNotConfigured_ReturnsUnimplemented in
// server/services/rules_service_test.go; it can't be reproduced through this package's
// rulesHandlers, because newTestRulesHandlers builds its SessionService via
// services.NewSessionService, which unconditionally wires a non-nil watcher (see
// session_service.go's NewSessionServiceWithSearchEngine) — there's no way to construct a
// SessionService with claudeSettingsWatcher left nil from outside the services package. The
// malformed-settings-file case below is the failure path this package's handler can actually
// reach: it exercises the same response-mapping code (tools_rules.go's success/message
// fields on the returned ReloadClaudeSettingsRulesResult), just via a non-error, success=false
// response rather than a connect error.
func TestReloadClaudeSettingsRules_should_ReturnFailureMessage_When_SettingsFileMalformed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatalf("failed to create temp .claude dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"),
		[]byte(`{"permissions": {"allow": [`), 0644); err != nil { // truncated/invalid JSON
		t.Fatalf("failed to write temp settings.json: %v", err)
	}

	h := newTestRulesHandlers(t)

	res, err := h.reloadClaudeSettingsRules(context.Background(), makeToolReq(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("reloadClaudeSettingsRules returned error: %v", err)
	}
	out := parseResult(t, res)
	if success, _ := out["success"].(bool); success {
		t.Fatalf("expected success=false for malformed settings file, got: %+v", out)
	}
	msg, _ := out["message"].(string)
	if !strings.Contains(msg, "Failed to reload") {
		t.Errorf("expected message to report the failure, got: %q", msg)
	}
	if out["rule_count"] != float64(0) {
		t.Errorf("expected rule_count=0 when the only settings file failed to parse, got %v", out["rule_count"])
	}
}
