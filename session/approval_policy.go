package session

import (
	"github.com/tstapler/stapler-squad/session/detection"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ApprovalPolicy defines a rule for automatic approval.
type ApprovalPolicy struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	ApprovalTypes   []detection.ApprovalType    `json:"approval_types"` // Types this policy applies to
	Enabled         bool              `json:"enabled"`
	Priority        int               `json:"priority"`   // Higher priority policies checked first
	Conditions      []PolicyCondition `json:"conditions"` // All must match
	Action          PolicyAction      `json:"action"`     // What to do when matched
	TimeRestriction *TimeRestriction  `json:"time_restriction,omitempty"`
	UsageLimit      *UsageLimit       `json:"usage_limit,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	usageCount      int               // Runtime tracking
	lastUsed        time.Time         // Runtime tracking
}

// PolicyCondition represents a single condition that must be met.
type PolicyCondition struct {
	Field    string `json:"field"`    // Field to check (e.g., "command", "file_path")
	Operator string `json:"operator"` // "equals", "contains", "regex", "not_contains"
	Value    string `json:"value"`    // Value to compare against
	compiled *regexp.Regexp
}

// PolicyAction specifies what to do when a policy matches.
type PolicyAction string

const (
	ActionAutoApprove PolicyAction = "auto_approve"
	ActionAutoReject  PolicyAction = "auto_reject"
	ActionPrompt      PolicyAction = "prompt"
	ActionLog         PolicyAction = "log_only"
)

// TimeRestriction limits when a policy is active.
type TimeRestriction struct {
	DaysOfWeek []time.Weekday `json:"days_of_week"` // Empty = all days
	StartHour  int            `json:"start_hour"`   // 0-23
	EndHour    int            `json:"end_hour"`     // 0-23
}

// UsageLimit restricts how many times a policy can be used.
type UsageLimit struct {
	MaxUses     int           `json:"max_uses"`     // 0 = unlimited
	TimeWindow  time.Duration `json:"time_window"`  // 0 = no time window
	PerApproval bool          `json:"per_approval"` // Track per approval type vs globally
}

// PolicyEngine manages approval policies and evaluates approval requests.
type PolicyEngine struct {
	policies    []*ApprovalPolicy
	mu          sync.RWMutex
	auditLog    []PolicyAuditEntry
	maxAuditLog int
}

// PolicyAuditEntry records policy evaluation results.
type PolicyAuditEntry struct {
	Timestamp      time.Time        `json:"timestamp"`
	RequestID      string           `json:"request_id"`
	PolicyID       string           `json:"policy_id"`
	PolicyName     string           `json:"policy_name"`
	Action         PolicyAction     `json:"action"`
	MatchedRequest *detection.ApprovalRequest `json:"matched_request"`
	Reason         string           `json:"reason"`
}

// NewPolicyEngine creates a new approval policy engine.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		policies:    make([]*ApprovalPolicy, 0),
		auditLog:    make([]PolicyAuditEntry, 0),
		maxAuditLog: 10000,
	}
}

// AddPolicy adds a new approval policy.
func (pe *PolicyEngine) AddPolicy(policy *ApprovalPolicy) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	// Validate policy
	if policy.ID == "" {
		policy.ID = generatePolicyID()
	}

	if policy.Name == "" {
		return fmt.Errorf("policy name is required")
	}

	// Compile regex patterns
	for i := range policy.Conditions {
		if policy.Conditions[i].Operator == "regex" {
			compiled, err := regexp.Compile(policy.Conditions[i].Value)
			if err != nil {
				return fmt.Errorf("invalid regex in condition: %w", err)
			}
			policy.Conditions[i].compiled = compiled
		}
	}

	policy.CreatedAt = time.Now()
	policy.UpdatedAt = time.Now()

	pe.policies = append(pe.policies, policy)

	// Sort by priority (higher first)
	pe.sortPolicies()

	return nil
}

// RemovePolicy removes a policy by ID.
func (pe *PolicyEngine) RemovePolicy(id string) bool {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	for i, policy := range pe.policies {
		if policy.ID == id {
			pe.policies = append(pe.policies[:i], pe.policies[i+1:]...)
			return true
		}
	}

	return false
}

// UpdatePolicy updates an existing policy.
func (pe *PolicyEngine) UpdatePolicy(updated *ApprovalPolicy) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	for i, policy := range pe.policies {
		if policy.ID == updated.ID {
			// Preserve creation time
			updated.CreatedAt = policy.CreatedAt
			updated.UpdatedAt = time.Now()

			// Compile regex patterns
			for j := range updated.Conditions {
				if updated.Conditions[j].Operator == "regex" {
					compiled, err := regexp.Compile(updated.Conditions[j].Value)
					if err != nil {
						return fmt.Errorf("invalid regex in condition: %w", err)
					}
					updated.Conditions[j].compiled = compiled
				}
			}

			pe.policies[i] = updated
			pe.sortPolicies()
			return nil
		}
	}

	return fmt.Errorf("policy '%s' not found", updated.ID)
}

// GetPolicy retrieves a policy by ID.
func (pe *PolicyEngine) GetPolicy(id string) *ApprovalPolicy {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	for _, policy := range pe.policies {
		if policy.ID == id {
			return policy
		}
	}

	return nil
}

// ListPolicies returns all policies, sorted by priority.
func (pe *PolicyEngine) ListPolicies() []*ApprovalPolicy {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	result := make([]*ApprovalPolicy, len(pe.policies))
	copy(result, pe.policies)
	return result
}

// Evaluate evaluates an approval request against all policies.
func (pe *PolicyEngine) Evaluate(request *detection.ApprovalRequest) (*PolicyDecision, error) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	decision := &PolicyDecision{
		Request:   request,
		Timestamp: time.Now(),
		Decision:  ActionPrompt, // Default to prompting user
		Matched:   false,
	}

	// Check each policy in priority order
	for _, policy := range pe.policies {
		if !policy.Enabled {
			continue
		}

		// Check if policy applies to this request type
		if !pe.appliesToType(policy, request.Type) {
			continue
		}

		// Check time restrictions
		if policy.TimeRestriction != nil && !pe.checkTimeRestriction(policy.TimeRestriction) {
			continue
		}

		// Check usage limits
		if policy.UsageLimit != nil && !pe.checkUsageLimit(policy) {
			continue
		}

		// Evaluate conditions
		if pe.matchesConditions(policy, request) {
			decision.Matched = true
			decision.MatchedPolicy = policy
			decision.Decision = policy.Action
			decision.Reason = fmt.Sprintf("Matched policy: %s", policy.Name)

			// Update usage tracking
			policy.usageCount++
			policy.lastUsed = time.Now()

			// Audit log
			pe.addAuditEntry(PolicyAuditEntry{
				Timestamp:      time.Now(),
				RequestID:      request.ID,
				PolicyID:       policy.ID,
				PolicyName:     policy.Name,
				Action:         policy.Action,
				MatchedRequest: request,
				Reason:         decision.Reason,
			})

			break // First matching policy wins
		}
	}

	return decision, nil
}

// PolicyDecision represents the result of policy evaluation.
type PolicyDecision struct {
	Request       *detection.ApprovalRequest `json:"request"`
	Timestamp     time.Time        `json:"timestamp"`
	Decision      PolicyAction     `json:"decision"`
	Matched       bool             `json:"matched"`
	MatchedPolicy *ApprovalPolicy  `json:"matched_policy,omitempty"`
	Reason        string           `json:"reason"`
}

// appliesToType checks if a policy applies to a given approval type.
func (pe *PolicyEngine) appliesToType(policy *ApprovalPolicy, requestType detection.ApprovalType) bool {
	if len(policy.ApprovalTypes) == 0 {
		return true // Empty list = applies to all types
	}

	for _, t := range policy.ApprovalTypes {
		if t == requestType {
			return true
		}
	}

	return false
}

// checkTimeRestriction validates if current time falls within policy time restrictions.
func (pe *PolicyEngine) checkTimeRestriction(restriction *TimeRestriction) bool {
	now := time.Now()

	// Check day of week
	if len(restriction.DaysOfWeek) > 0 {
		currentDay := now.Weekday()
		dayAllowed := false
		for _, day := range restriction.DaysOfWeek {
			if day == currentDay {
				dayAllowed = true
				break
			}
		}
		if !dayAllowed {
			return false
		}
	}

	// Check hour range
	currentHour := now.Hour()
	if restriction.StartHour <= restriction.EndHour {
		// Normal range (e.g., 9am-5pm)
		if currentHour < restriction.StartHour || currentHour >= restriction.EndHour {
			return false
		}
	} else {
		// Overnight range (e.g., 10pm-6am)
		if currentHour < restriction.StartHour && currentHour >= restriction.EndHour {
			return false
		}
	}

	return true
}

// checkUsageLimit validates if a policy hasn't exceeded its usage limits.
func (pe *PolicyEngine) checkUsageLimit(policy *ApprovalPolicy) bool {
	if policy.UsageLimit.MaxUses == 0 {
		return true // Unlimited
	}

	// Check time window
	if policy.UsageLimit.TimeWindow > 0 {
		if time.Since(policy.lastUsed) > policy.UsageLimit.TimeWindow {
			// Reset counter if outside time window
			policy.usageCount = 0
		}
	}

	return policy.usageCount < policy.UsageLimit.MaxUses
}

// matchesConditions checks if all policy conditions match the request.
func (pe *PolicyEngine) matchesConditions(policy *ApprovalPolicy, request *detection.ApprovalRequest) bool {
	for _, condition := range policy.Conditions {
		if !pe.evaluateCondition(&condition, request) {
			return false
		}
	}

	return true
}

// evaluateCondition evaluates a single condition against a request.
func (pe *PolicyEngine) evaluateCondition(condition *PolicyCondition, request *detection.ApprovalRequest) bool {
	// Get field value from request
	fieldValue := pe.getFieldValue(condition.Field, request)

	switch condition.Operator {
	case "equals":
		return fieldValue == condition.Value

	case "contains":
		return strings.Contains(fieldValue, condition.Value)

	case "not_contains":
		return !strings.Contains(fieldValue, condition.Value)

	case "regex":
		if condition.compiled != nil {
			return condition.compiled.MatchString(fieldValue)
		}
		return false

	case "starts_with":
		return strings.HasPrefix(fieldValue, condition.Value)

	case "ends_with":
		return strings.HasSuffix(fieldValue, condition.Value)

	default:
		return false
	}
}

// getFieldValue extracts a field value from an approval request.
func (pe *PolicyEngine) getFieldValue(field string, request *detection.ApprovalRequest) string {
	switch field {
	case "type":
		return string(request.Type)
	case "detected_text":
		return request.DetectedText
	case "context":
		return request.Context
	default:
		// Check extracted data
		if val, exists := request.ExtractedData[field]; exists {
			return val
		}
		return ""
	}
}

// sortPolicies sorts policies by priority (highest first).
func (pe *PolicyEngine) sortPolicies() {
	// Simple bubble sort by priority
	n := len(pe.policies)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if pe.policies[j].Priority < pe.policies[j+1].Priority {
				pe.policies[j], pe.policies[j+1] = pe.policies[j+1], pe.policies[j]
			}
		}
	}
}

// addAuditEntry adds an entry to the audit log.
func (pe *PolicyEngine) addAuditEntry(entry PolicyAuditEntry) {
	pe.auditLog = append(pe.auditLog, entry)

	// Enforce max audit log size
	if pe.maxAuditLog > 0 && len(pe.auditLog) > pe.maxAuditLog {
		pe.auditLog = pe.auditLog[len(pe.auditLog)-pe.maxAuditLog:]
	}
}

// GetAuditLog returns recent audit log entries.
func (pe *PolicyEngine) GetAuditLog(limit int) []PolicyAuditEntry {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if limit <= 0 || limit > len(pe.auditLog) {
		limit = len(pe.auditLog)
	}

	// Return most recent first
	result := make([]PolicyAuditEntry, limit)
	for i := 0; i < limit; i++ {
		result[i] = pe.auditLog[len(pe.auditLog)-1-i]
	}

	return result
}

// ClearAuditLog removes all audit log entries.
func (pe *PolicyEngine) ClearAuditLog() {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.auditLog = make([]PolicyAuditEntry, 0)
}

// SetMaxAuditLog sets the maximum number of audit log entries to keep.
func (pe *PolicyEngine) SetMaxAuditLog(max int) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.maxAuditLog = max

	// Trim if necessary
	if max > 0 && len(pe.auditLog) > max {
		pe.auditLog = pe.auditLog[len(pe.auditLog)-max:]
	}
}

// GetStatistics returns statistics about policy usage.
func (pe *PolicyEngine) GetStatistics() PolicyStatistics {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	stats := PolicyStatistics{
		TotalPolicies:    len(pe.policies),
		EnabledPolicies:  0,
		TotalEvaluations: len(pe.auditLog),
	}

	for _, policy := range pe.policies {
		if policy.Enabled {
			stats.EnabledPolicies++
		}
	}

	for _, entry := range pe.auditLog {
		switch entry.Action {
		case ActionAutoApprove:
			stats.AutoApprovals++
		case ActionAutoReject:
			stats.AutoRejections++
		case ActionPrompt:
			stats.PromptedApprovals++
		case ActionLog:
			stats.LoggedOnly++
		}
	}

	return stats
}

// PolicyStatistics provides summary statistics.
type PolicyStatistics struct {
	TotalPolicies     int
	EnabledPolicies   int
	TotalEvaluations  int
	AutoApprovals     int
	AutoRejections    int
	PromptedApprovals int
	LoggedOnly        int
}

// generatePolicyID generates a unique ID for policies.
func generatePolicyID() string {
	return fmt.Sprintf("policy_%d", time.Now().UnixNano())
}

// CreateSafeCommandPolicy creates a policy for automatically approving safe commands.
func CreateSafeCommandPolicy() *ApprovalPolicy {
	return &ApprovalPolicy{
		Name:          "Safe Commands",
		Description:   "Auto-approve common read-only commands",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand},
		Enabled:       true,
		Priority:      100,
		Action:        ActionAutoApprove,
		Conditions: []PolicyCondition{
			{
				Field:    "command",
				Operator: "regex",
				Value:    "^(ls|pwd|cat|echo|grep|find|which|whoami|date|uptime)\\s.*$",
			},
		},
	}
}

// CreateNoDestructivePolicy creates a policy for rejecting destructive commands.
func CreateNoDestructivePolicy() *ApprovalPolicy {
	return &ApprovalPolicy{
		Name:          "Block Destructive Commands",
		Description:   "Auto-reject potentially destructive operations",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand, detection.ApprovalFileWrite},
		Enabled:       true,
		Priority:      200, // Higher priority than safe commands
		Action:        ActionAutoReject,
		Conditions: []PolicyCondition{
			{
				Field:    "command",
				Operator: "regex",
				Value:    "(rm\\s+-rf|sudo\\s+rm|format|mkfs|dd\\s+if=)",
			},
		},
	}
}

// CreateBusinessHoursPolicy creates a policy that only applies during business hours.
func CreateBusinessHoursPolicy() *ApprovalPolicy {
	return &ApprovalPolicy{
		Name:          "Business Hours Auto-Approve",
		Description:   "Auto-approve during business hours only",
		ApprovalTypes: []detection.ApprovalType{detection.ApprovalCommand, detection.ApprovalFileWrite},
		Enabled:       true,
		Priority:      50,
		Action:        ActionAutoApprove,
		TimeRestriction: &TimeRestriction{
			DaysOfWeek: []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
			StartHour:  9,
			EndHour:    17,
		},
		Conditions: []PolicyCondition{},
	}
}
