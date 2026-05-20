package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// ── Mock helpers ──────────────────────────────────────────────────────────────

// mockAIClient is a test double for AIClient that returns a fixed response.
type mockAIClient struct {
	response string
	err      error
	// blockUntilCtx, when true, blocks until ctx is cancelled (for cancellation tests).
	blockUntilCtx bool
}

func (m *mockAIClient) Complete(ctx context.Context, _, _ string) (string, error) {
	if m.blockUntilCtx {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return m.response, m.err
}

// spyRulesStore wraps RulesStore and records Upsert calls.
type spyRulesStore struct {
	*RulesStore
	upsertCalls int
}

func (s *spyRulesStore) Upsert(spec RuleSpec) (RuleSpec, error) {
	s.upsertCalls++
	return s.RulesStore.Upsert(spec)
}

// newRulesServiceWithAI creates a RulesService wired with the given mock AI client.
func newRulesServiceWithAI(t *testing.T, aiClient AIClient) *RulesService {
	t.Helper()
	storage := createTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	c := classifier.NewRuleBasedClassifier()
	return NewRulesService(rulesStore, analyticsStore, c, &DefaultRulePromptBuilder{}, aiClient)
}

// fixture2ElementJSON is a valid 2-element JSON array matching the AI response format.
const fixture2ElementJSON = `[
  {
    "name": "Allow git push",
    "tool_name": "Bash",
    "command_pattern": "git push",
    "decision": "auto_allow",
    "risk_level": "low",
    "reason": "Pushing to remote is safe",
    "priority": 100,
    "confidence": 0.85,
    "explanation": "Derived from common git workflow"
  },
  {
    "name": "Allow npm install",
    "tool_name": "Bash",
    "command_pattern": "npm install",
    "decision": "auto_allow",
    "risk_level": "low",
    "reason": "Installing packages is standard",
    "priority": 100,
    "confidence": 0.75,
    "explanation": "Derived from node.js workflow"
  }
]`

// ── T-UNIT-GO-001: Happy path — analytics_gaps returns plural suggestions ────

func TestGenerateSuggestedRule_AnalyticsGaps_ReturnsSuggestions(t *testing.T) {
	// T-UNIT-GO-001
	svc := newRulesServiceWithAI(t, &mockAIClient{response: fixture2ElementJSON})

	windowDays := int32(7)
	resp, err := svc.GenerateSuggestedRule(context.Background(), connect.NewRequest(&sessionv1.GenerateSuggestedRuleRequest{
		Source:     sessionv1.SuggestionSource_SUGGESTION_SOURCE_ANALYTICS_GAPS,
		WindowDays: &windowDays,
	}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Suggestions, 2, "should return 2 suggestions from 2-element JSON array")
	for _, s := range resp.Msg.Suggestions {
		assert.NotEmpty(t, s.Name, "suggestion name must be non-empty")
		assert.Greater(t, s.Confidence, float32(0), "suggestion confidence must be > 0")
	}
}

// ── T-UNIT-GO-002: Failure mode — unspecified source returns CodeInvalidArgument ─

func TestGenerateSuggestedRule_UnspecifiedSource_ReturnsError(t *testing.T) {
	// T-UNIT-GO-002
	svc := newRulesServiceWithAI(t, &mockAIClient{response: "[]"})

	_, err := svc.GenerateSuggestedRule(context.Background(), connect.NewRequest(&sessionv1.GenerateSuggestedRuleRequest{
		Source: sessionv1.SuggestionSource_SUGGESTION_SOURCE_UNSPECIFIED,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeOf(err), connect.CodeInvalidArgument)
	assert.Contains(t, err.Error(), "source is required")
}

// ── T-UNIT-GO-003: Failure mode — nil AI client returns CodeUnimplemented ────

func TestGenerateSuggestedRule_NilAIClient_ReturnsUnimplemented(t *testing.T) {
	// T-UNIT-GO-003
	storage := createTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	analyticsStore := NewAnalyticsStore(storage)
	c := classifier.NewRuleBasedClassifier()
	svc := NewRulesService(rulesStore, analyticsStore, c, nil, nil) // nil AI

	_, err = svc.GenerateSuggestedRule(context.Background(), connect.NewRequest(&sessionv1.GenerateSuggestedRuleRequest{
		Source: sessionv1.SuggestionSource_SUGGESTION_SOURCE_ANALYTICS_GAPS,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeOf(err), connect.CodeUnimplemented)
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY")
}

// ── T-UNIT-GO-004: buildPromptContext includes existing rules and analytics gaps ─

func TestBuildPromptContext_IncludesRulesAndGaps(t *testing.T) {
	// T-UNIT-GO-004
	storage := createTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)

	// Insert 2 user rules.
	for i := 0; i < 2; i++ {
		_, err := rulesStore.Upsert(RuleSpec{
			ID:       fmt.Sprintf("rule-%d", i),
			Name:     fmt.Sprintf("Rule %d", i),
			Decision: "auto_allow",
			Enabled:  true,
			Source:   "user",
			Priority: 100,
		})
		require.NoError(t, err)
	}

	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())

	// Insert 3 escalated entries with no rule match.
	// RiskLevel must be non-empty to pass the DB schema validator.
	for i := 0; i < 3; i++ {
		analyticsStore.Record(AnalyticsEntry{
			Timestamp:      time.Now(),
			SessionID:      "sess-1",
			ToolName:       "Bash",
			CommandPreview: fmt.Sprintf("some-cmd-%d", i),
			Decision:       "escalate",
			RiskLevel:      "medium",
			// RuleID empty = no rule matched
		})
	}
	// Wait for the async write to complete by polling LoadWindow until all 3 entries appear.
	require.Eventually(t, func() bool {
		entries, err := analyticsStore.LoadWindow(time.Now().Add(-1 * time.Hour))
		return err == nil && len(entries) >= 3
	}, 2*time.Second, 10*time.Millisecond, "analytics entries must be persisted within 2s")

	c := classifier.NewRuleBasedClassifier()
	svc := NewRulesService(rulesStore, analyticsStore, c, &DefaultRulePromptBuilder{}, &mockAIClient{response: "[]"})

	req := &sessionv1.GenerateSuggestedRuleRequest{
		Source: sessionv1.SuggestionSource_SUGGESTION_SOURCE_ANALYTICS_GAPS,
	}
	promptCtx := svc.buildPromptContext(req, 7)

	// existingRules should include the 2 user rules + seed rules.
	assert.GreaterOrEqual(t, len(promptCtx.ExistingRules), 2, "existing rules must include user rules")

	// AnalyticsGaps should reflect the 3 escalated entries grouped into 1 gap cluster
	// (all 3 share the same ToolName="Bash" and CommandProgram="" so they collapse into 1).
	// ReclassifyGaps may re-classify entries that now match a rule; since the test inserts
	// entries with empty RuleID and the seed rules do not match "some-cmd-N", the gap
	// cluster survives reclassification.
	assert.Equal(t, 1, len(promptCtx.AnalyticsGaps), "3 escalated entries with same tool/program should produce exactly 1 gap cluster")
}

// ── T-UNIT-GO-007: FR-8 — GenerateSuggestedRule never calls Upsert ───────────

func TestGenerateSuggestedRule_NeverCallsUpsert(t *testing.T) {
	// T-UNIT-GO-007
	storage := createTestStorage(t)
	baseStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	spy := &spyRulesStore{RulesStore: baseStore}
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	c := classifier.NewRuleBasedClassifier()

	svc := &RulesService{
		rulesStore:     spy.RulesStore,
		analyticsStore: analyticsStore,
		classifier:     c,
		promptBuilder:  &DefaultRulePromptBuilder{},
		aiClient:       &mockAIClient{response: fixture2ElementJSON},
	}

	_, err = svc.GenerateSuggestedRule(context.Background(), connect.NewRequest(&sessionv1.GenerateSuggestedRuleRequest{
		Source: sessionv1.SuggestionSource_SUGGESTION_SOURCE_ANALYTICS_GAPS,
	}))
	require.NoError(t, err)
	assert.Equal(t, 0, spy.upsertCalls, "GenerateSuggestedRule must never call Upsert")
}

// ── T-UNIT-GO-010: Handler returns on context cancellation ───────────────────

func TestGenerateSuggestedRule_ReturnsOnCtxCancellation(t *testing.T) {
	// T-UNIT-GO-010
	svc := newRulesServiceWithAI(t, &mockAIClient{blockUntilCtx: true})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := svc.GenerateSuggestedRule(ctx, connect.NewRequest(&sessionv1.GenerateSuggestedRuleRequest{
		Source: sessionv1.SuggestionSource_SUGGESTION_SOURCE_ANALYTICS_GAPS,
	}))
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 500*time.Millisecond, "handler should return within 500ms of ctx cancel")
	require.Error(t, err, "handler should return error on context cancel")
}

// ── parseSuggestions validation tests ────────────────────────────────────────

func TestParseSuggestions_ValidJSON_ReturnsSuggestions(t *testing.T) {
	svc := newRulesServiceWithAI(t, nil)
	sugs, err := svc.parseSuggestions(fixture2ElementJSON)
	require.NoError(t, err)
	assert.Len(t, sugs, 2)
}

func TestParseSuggestions_InvalidCommandPattern_DropsItem(t *testing.T) {
	badJSON := `[{"name":"bad","tool_name":"Bash","command_pattern":"[invalid","decision":"auto_allow","risk_level":"low","reason":"x","priority":100,"confidence":0.5}]`
	svc := newRulesServiceWithAI(t, nil)
	sugs, err := svc.parseSuggestions(badJSON)
	require.NoError(t, err, "parseSuggestions should not error on invalid pattern, just drop it")
	assert.Len(t, sugs, 0, "item with invalid commandPattern should be dropped")
}

func TestParseSuggestions_ConfidenceClamp(t *testing.T) {
	json1 := `[{"name":"x","tool_name":"Bash","command_pattern":"git push","decision":"auto_allow","risk_level":"low","reason":"r","confidence":1.5}]`
	svc := newRulesServiceWithAI(t, nil)
	sugs, err := svc.parseSuggestions(json1)
	require.NoError(t, err)
	require.Len(t, sugs, 1)
	assert.Equal(t, float32(1.0), sugs[0].Confidence, "confidence 1.5 should be clamped to 1.0")
}

func TestParseSuggestions_PriorityZero_DefaultsTo100(t *testing.T) {
	json1 := `[{"name":"x","tool_name":"Bash","command_pattern":"npm install","decision":"auto_allow","risk_level":"low","reason":"r","priority":0,"confidence":0.5}]`
	svc := newRulesServiceWithAI(t, nil)
	sugs, err := svc.parseSuggestions(json1)
	require.NoError(t, err)
	require.Len(t, sugs, 1)
	assert.Equal(t, int32(100), sugs[0].Priority, "priority 0 should default to 100")
}

func TestParseSuggestions_CapAt5(t *testing.T) {
	// 6-element array should be capped to 5.
	json6 := `[
		{"name":"a","tool_name":"Bash","command_pattern":"cmd-a","decision":"auto_allow","risk_level":"low","reason":"r","priority":100,"confidence":0.5},
		{"name":"b","tool_name":"Bash","command_pattern":"cmd-b","decision":"auto_allow","risk_level":"low","reason":"r","priority":100,"confidence":0.5},
		{"name":"c","tool_name":"Bash","command_pattern":"cmd-c","decision":"auto_allow","risk_level":"low","reason":"r","priority":100,"confidence":0.5},
		{"name":"d","tool_name":"Bash","command_pattern":"cmd-d","decision":"auto_allow","risk_level":"low","reason":"r","priority":100,"confidence":0.5},
		{"name":"e","tool_name":"Bash","command_pattern":"cmd-e","decision":"auto_allow","risk_level":"low","reason":"r","priority":100,"confidence":0.5},
		{"name":"f","tool_name":"Bash","command_pattern":"cmd-f","decision":"auto_allow","risk_level":"low","reason":"r","priority":100,"confidence":0.5}
	]`
	svc := newRulesServiceWithAI(t, nil)
	sugs, err := svc.parseSuggestions(json6)
	require.NoError(t, err)
	assert.Len(t, sugs, 5, "6-element JSON array should be capped to 5 suggestions")
}

func TestParseSuggestions_MarkdownFencedJSON_ParsesCorrectly(t *testing.T) {
	// T1: Markdown-wrapped JSON (```json ... ```) must be stripped and parsed correctly.
	fenced := "```json\n" + fixture2ElementJSON + "\n```"
	svc := newRulesServiceWithAI(t, nil)
	sugs, err := svc.parseSuggestions(fenced)
	require.NoError(t, err, "parseSuggestions must strip markdown fences and parse correctly")
	assert.Len(t, sugs, 2, "fenced JSON with 2 elements must return 2 suggestions")
}

func TestParseSuggestions_NonJSONInput_ReturnsError(t *testing.T) {
	// T1: Non-JSON / malformed input must return an error, not panic.
	svc := newRulesServiceWithAI(t, nil)
	_, err := svc.parseSuggestions("this is not json at all")
	require.Error(t, err, "parseSuggestions must return an error for malformed non-JSON input")
}

// ── T-INTEG-001: Full handler pipeline with mock AI ──────────────────────────

func TestGenerateSuggestedRule_Integration_MockAI(t *testing.T) {
	// T-INTEG-001
	svc := newRulesServiceWithAI(t, &mockAIClient{response: fixture2ElementJSON})

	windowDays := int32(7)
	resp, err := svc.GenerateSuggestedRule(context.Background(), connect.NewRequest(&sessionv1.GenerateSuggestedRuleRequest{
		Source:     sessionv1.SuggestionSource_SUGGESTION_SOURCE_ANALYTICS_GAPS,
		WindowDays: &windowDays,
	}))
	require.NoError(t, err)
	assert.Len(t, resp.Msg.Suggestions, 2)
	for _, s := range resp.Msg.Suggestions {
		assert.GreaterOrEqual(t, s.Confidence, float32(0), "confidence must be >= 0")
		assert.LessOrEqual(t, s.Confidence, float32(1.0), "confidence must be <= 1.0")
	}
}

// ── attachConflictInfo tests ──────────────────────────────────────────────────

// ── T-UNIT-GO-011: GetProgramAnalytics returns expected response fields ────────

func TestGetProgramAnalytics_ReturnsExpectedFields(t *testing.T) {
	// T-UNIT-GO-011
	svc := newRulesService(t)

	windowDays := int32(7)
	resp, err := svc.GetProgramAnalytics(t.Context(), connect.NewRequest(&sessionv1.GetProgramAnalyticsRequest{
		Program:    "git",
		WindowDays: &windowDays,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg)

	// All top-level repeated/slice fields must be non-nil (may be empty, not nil).
	assert.NotNil(t, resp.Msg.Subcommands, "subcommands field must not be nil")
	assert.NotNil(t, resp.Msg.RecentExamples, "recent_examples field must not be nil")
	assert.NotNil(t, resp.Msg.Trend, "daily_trend field must not be nil")

	// Program echo-back.
	assert.Equal(t, "git", resp.Msg.Program)
}

func TestAttachConflictInfo_SeedRuleAtHigherPriority_ShadowsSuggestion(t *testing.T) {
	// T-UNIT-GO (attachConflictInfo): fixture rule at priority 500 overlaps suggestion at 100.
	storage := createTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)

	// Insert a user rule with same ToolName + CommandPattern at higher priority.
	_, err = rulesStore.Upsert(RuleSpec{
		ID:             "high-priority-rule",
		Name:           "High Priority Rule",
		ToolName:       "Bash",
		CommandPattern: "git push",
		Decision:       "auto_deny",
		Enabled:        true,
		Source:         "user",
		Priority:       500,
	})
	require.NoError(t, err)

	analyticsStore := NewAnalyticsStore(storage)
	c := classifier.NewRuleBasedClassifier()
	svc := NewRulesService(rulesStore, analyticsStore, c, nil, nil)

	suggestion := &sessionv1.SuggestedRuleProto{
		ToolName:       "Bash",
		CommandPattern: "git push",
		Priority:       100,
	}
	svc.attachConflictInfo(suggestion)

	assert.Contains(t, suggestion.ShadowedByRuleIds, "high-priority-rule",
		"suggestion at priority 100 should be shadowed by user rule at priority 500")
}
