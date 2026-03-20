package session

import (
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

func TestNewPolicyEngine(t *testing.T) {
	engine := NewPolicyEngine()

	if engine == nil {
		t.Fatal("NewPolicyEngine() returned nil")
	}

	policies := engine.ListPolicies()
	if len(policies) != 0 {
		t.Errorf("New engine should have 0 policies, got %d", len(policies))
	}
}

func TestPolicyEngine_AddPolicy(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "test_policy",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions:    []PolicyCondition{},
	}

	if err := engine.AddPolicy(policy); err != nil {
		t.Fatalf("AddPolicy() failed: %v", err)
	}

	if policy.ID == "" {
		t.Error("Policy ID should be generated")
	}

	if policy.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestPolicyEngine_AddPolicyNoName(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Enabled: true,
		Action:  ActionAutoApprove,
	}

	err := engine.AddPolicy(policy)
	if err == nil {
		t.Error("AddPolicy() should fail with no name")
	}
}

func TestPolicyEngine_AddPolicyInvalidRegex(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:    "invalid_regex",
		Enabled: true,
		Action:  ActionAutoApprove,
		Conditions: []PolicyCondition{
			{
				Field:    "command",
				Operator: "regex",
				Value:    "[invalid(regex",
			},
		},
	}

	err := engine.AddPolicy(policy)
	if err == nil {
		t.Error("AddPolicy() should fail with invalid regex")
	}
}

func TestPolicyEngine_RemovePolicy(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:    "test_remove",
		Enabled: true,
		Action:  ActionAutoApprove,
	}

	engine.AddPolicy(policy)

	if !engine.RemovePolicy(policy.ID) {
		t.Error("RemovePolicy() should return true for existing policy")
	}

	if engine.RemovePolicy("nonexistent") {
		t.Error("RemovePolicy() should return false for nonexistent policy")
	}
}

func TestPolicyEngine_GetPolicy(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:    "test_get",
		Enabled: true,
		Action:  ActionAutoApprove,
	}

	engine.AddPolicy(policy)

	found := engine.GetPolicy(policy.ID)
	if found == nil {
		t.Error("GetPolicy() returned nil for existing policy")
	}

	if found.ID != policy.ID {
		t.Errorf("ID = %q, expected %q", found.ID, policy.ID)
	}
}

func TestPolicyEngine_UpdatePolicy(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:     "test_update",
		Enabled:  true,
		Priority: 100,
		Action:   ActionAutoApprove,
	}

	engine.AddPolicy(policy)

	policy.Priority = 200
	policy.Name = "updated_name"

	if err := engine.UpdatePolicy(policy); err != nil {
		t.Fatalf("UpdatePolicy() failed: %v", err)
	}

	updated := engine.GetPolicy(policy.ID)
	if updated.Priority != 200 {
		t.Errorf("Priority = %d, expected 200", updated.Priority)
	}

	if updated.Name != "updated_name" {
		t.Errorf("Name = %q, expected %q", updated.Name, "updated_name")
	}
}

func TestPolicyEngine_EvaluateSimpleMatch(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "test_eval",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions: []PolicyCondition{
			{
				Field:    "command",
				Operator: "contains",
				Value:    "ls",
			},
		},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
		ExtractedData: map[string]string{
			"command": "ls -la",
		},
	}

	decision, err := engine.Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if !decision.Matched {
		t.Error("Decision should match")
	}

	if decision.Decision != ActionAutoApprove {
		t.Errorf("Decision = %v, expected ActionAutoApprove", decision.Decision)
	}
}

func TestPolicyEngine_EvaluateNoMatch(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "test_no_match",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions: []PolicyCondition{
			{
				Field:    "command",
				Operator: "equals",
				Value:    "specific_command",
			},
		},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
		ExtractedData: map[string]string{
			"command": "different_command",
		},
	}

	decision, err := engine.Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if decision.Matched {
		t.Error("Decision should not match")
	}

	if decision.Decision != ActionPrompt {
		t.Errorf("Decision = %v, expected ActionPrompt", decision.Decision)
	}
}

func TestPolicyEngine_EvaluateDisabledPolicy(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "disabled_policy",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       false, // Disabled
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions:    []PolicyCondition{},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
	}

	decision, err := engine.Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if decision.Matched {
		t.Error("Disabled policy should not match")
	}
}

func TestPolicyEngine_EvaluatePriority(t *testing.T) {
	engine := NewPolicyEngine()

	// Lower priority - auto approve
	lowPriority := &ApprovalPolicy{
		Name:          "low_priority",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      50,
		Action:        ActionAutoApprove,
		Conditions:    []PolicyCondition{},
	}

	// Higher priority - auto reject
	highPriority := &ApprovalPolicy{
		Name:          "high_priority",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoReject,
		Conditions:    []PolicyCondition{},
	}

	engine.AddPolicy(lowPriority)
	engine.AddPolicy(highPriority)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
	}

	decision, err := engine.Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	// Should match high priority policy
	if decision.Decision != ActionAutoReject {
		t.Errorf("Decision = %v, expected ActionAutoReject from higher priority", decision.Decision)
	}
}

func TestPolicyEngine_RegexCondition(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "regex_test",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions: []PolicyCondition{
			{
				Field:    "command",
				Operator: "regex",
				Value:    "^(ls|pwd)\\s.*$",
			},
		},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
		ExtractedData: map[string]string{
			"command": "ls -la",
		},
	}

	decision, err := engine.Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if !decision.Matched {
		t.Error("Regex condition should match")
	}
}

func TestPolicyEngine_MultipleConditions(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "multiple_conditions",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions: []PolicyCondition{
			{
				Field:    "command",
				Operator: "contains",
				Value:    "ls",
			},
			{
				Field:    "command",
				Operator: "not_contains",
				Value:    "rm",
			},
		},
	}

	engine.AddPolicy(policy)

	// Should match (has ls, no rm)
	request1 := &detection.ApprovalRequest{
		ID:   "test-request-1",
		Type: detection.ApprovalCommand,
		ExtractedData: map[string]string{
			"command": "ls -la",
		},
	}

	decision1, _ := engine.Evaluate(request1)
	if !decision1.Matched {
		t.Error("Should match when all conditions are met")
	}

	// Should not match (has both ls and rm)
	request2 := &detection.ApprovalRequest{
		ID:   "test-request-2",
		Type: detection.ApprovalCommand,
		ExtractedData: map[string]string{
			"command": "ls | xargs rm",
		},
	}

	decision2, _ := engine.Evaluate(request2)
	if decision2.Matched {
		t.Error("Should not match when any condition fails")
	}
}

func TestPolicyEngine_TimeRestriction(t *testing.T) {
	engine := NewPolicyEngine()

	now := time.Now()

	policy := &ApprovalPolicy{
		Name:          "time_restricted",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		TimeRestriction: &TimeRestriction{
			DaysOfWeek: []time.Weekday{now.Weekday()}, // Current day
			StartHour:  now.Hour() - 1,                // One hour ago
			EndHour:    now.Hour() + 1,                // One hour from now
		},
		Conditions: []PolicyCondition{},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
	}

	decision, err := engine.Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if !decision.Matched {
		t.Error("Policy should match within time restriction")
	}
}

func TestPolicyEngine_TimeRestrictionOutsideHours(t *testing.T) {
	engine := NewPolicyEngine()

	now := time.Now()

	policy := &ApprovalPolicy{
		Name:          "time_restricted",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		TimeRestriction: &TimeRestriction{
			DaysOfWeek: []time.Weekday{now.Weekday()},
			StartHour:  (now.Hour() + 5) % 24, // 5 hours from now
			EndHour:    (now.Hour() + 6) % 24, // 6 hours from now
		},
		Conditions: []PolicyCondition{},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
	}

	decision, err := engine.Evaluate(request)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if decision.Matched {
		t.Error("Policy should not match outside time restriction")
	}
}

func TestPolicyEngine_UsageLimit(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "usage_limited",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		UsageLimit: &UsageLimit{
			MaxUses:    2,
			TimeWindow: 0, // No time window
		},
		Conditions: []PolicyCondition{},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
	}

	// First use - should match
	decision1, _ := engine.Evaluate(request)
	if !decision1.Matched {
		t.Error("First evaluation should match")
	}

	// Second use - should match
	decision2, _ := engine.Evaluate(request)
	if !decision2.Matched {
		t.Error("Second evaluation should match")
	}

	// Third use - should not match (exceeded limit)
	decision3, _ := engine.Evaluate(request)
	if decision3.Matched {
		t.Error("Third evaluation should not match (exceeded limit)")
	}
}

func TestPolicyEngine_GetAuditLog(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "audit_test",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions:    []PolicyCondition{},
	}

	engine.AddPolicy(policy)

	// Generate some evaluations
	for i := 0; i < 5; i++ {
		request := &detection.ApprovalRequest{
			ID:   "test-request",
			Type: detection.ApprovalCommand,
		}
		engine.Evaluate(request)
	}

	audit := engine.GetAuditLog(3)

	if len(audit) != 3 {
		t.Errorf("GetAuditLog(3) returned %d entries, expected 3", len(audit))
	}
}

func TestPolicyEngine_ClearAuditLog(t *testing.T) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "audit_test",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions:    []PolicyCondition{},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:   "test-request",
		Type: detection.ApprovalCommand,
	}
	engine.Evaluate(request)

	engine.ClearAuditLog()

	audit := engine.GetAuditLog(0)
	if len(audit) != 0 {
		t.Errorf("Audit log length = %d after clear, expected 0", len(audit))
	}
}

func TestPolicyEngine_GetStatistics(t *testing.T) {
	engine := NewPolicyEngine()

	approvePolicy := &ApprovalPolicy{
		Name:          "approve_policy",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions: []PolicyCondition{
			{Field: "command", Operator: "equals", Value: "approve_me"},
		},
	}

	rejectPolicy := &ApprovalPolicy{
		Name:          "reject_policy",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       false, // Disabled
		Priority:      50,
		Action:        ActionAutoReject,
		Conditions:    []PolicyCondition{},
	}

	engine.AddPolicy(approvePolicy)
	engine.AddPolicy(rejectPolicy)

	// Generate evaluations
	request := &detection.ApprovalRequest{
		ID:            "test-request",
		Type:          detection.ApprovalCommand,
		ExtractedData: map[string]string{"command": "approve_me"},
	}
	engine.Evaluate(request)

	stats := engine.GetStatistics()

	if stats.TotalPolicies != 2 {
		t.Errorf("TotalPolicies = %d, expected 2", stats.TotalPolicies)
	}

	if stats.EnabledPolicies != 1 {
		t.Errorf("EnabledPolicies = %d, expected 1", stats.EnabledPolicies)
	}

	if stats.AutoApprovals == 0 {
		t.Error("Expected at least one auto approval")
	}
}

func TestCreateSafeCommandPolicy(t *testing.T) {
	policy := CreateSafeCommandPolicy()

	if policy == nil {
		t.Fatal("CreateSafeCommandPolicy() returned nil")
	}

	if policy.Action != ActionAutoApprove {
		t.Errorf("Action = %v, expected ActionAutoApprove", policy.Action)
	}

	if len(policy.Conditions) == 0 {
		t.Error("Expected at least one condition")
	}
}

func TestCreateNoDestructivePolicy(t *testing.T) {
	policy := CreateNoDestructivePolicy()

	if policy == nil {
		t.Fatal("CreateNoDestructivePolicy() returned nil")
	}

	if policy.Action != ActionAutoReject {
		t.Errorf("Action = %v, expected ActionAutoReject", policy.Action)
	}

	if policy.Priority <= 100 {
		t.Error("Destructive policy should have high priority")
	}
}

func TestCreateBusinessHoursPolicy(t *testing.T) {
	policy := CreateBusinessHoursPolicy()

	if policy == nil {
		t.Fatal("CreateBusinessHoursPolicy() returned nil")
	}

	if policy.TimeRestriction == nil {
		t.Fatal("TimeRestriction should be set")
	}

	if len(policy.TimeRestriction.DaysOfWeek) != 5 {
		t.Errorf("Expected 5 weekdays, got %d", len(policy.TimeRestriction.DaysOfWeek))
	}
}

func Benchmark_PolicyEngine_Evaluate(b *testing.B) {
	engine := NewPolicyEngine()

	policy := &ApprovalPolicy{
		Name:          "benchmark_policy",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions: []PolicyCondition{
			{Field: "command", Operator: "contains", Value: "test"},
		},
	}

	engine.AddPolicy(policy)

	request := &detection.ApprovalRequest{
		ID:            "bench-request",
		Type:          detection.ApprovalCommand,
		ExtractedData: map[string]string{"command": "test command"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Evaluate(request)
	}
}
