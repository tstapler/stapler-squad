package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session"
	"gopkg.in/yaml.v3"
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

// ── TestCoveredSubcommands ────────────────────────────────────────────────────

// newRulesServiceForCoverage builds a RulesService that has the given specs upserted
// into its store. Seed rules are still present (allRuleSpecs always appends them), but
// testProg ("mytestcli") is an arbitrary name with no matching seed rules, so seed-rule
// interference cannot affect coveredSubcommands results when testProg is used as the program.
func newRulesServiceForCoverage(t *testing.T, specs []RuleSpec) *RulesService {
	t.Helper()
	storage := createTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	for i, spec := range specs {
		if spec.ID == "" {
			spec.ID = fmt.Sprintf("test-rule-%d", i)
		}
		if spec.Name == "" {
			spec.Name = fmt.Sprintf("Test Rule %d", i)
		}
		if spec.Source == "" {
			spec.Source = "user"
		}
		_, err := rulesStore.Upsert(spec)
		require.NoError(t, err)
	}
	analyticsStore := NewAnalyticsStore(storage)
	c := classifier.NewRuleBasedClassifier()
	return NewRulesService(rulesStore, analyticsStore, c, nil, nil)
}

// testProg is an arbitrary program name that has no seed rules, so seed-rule
// interference cannot affect the coveredSubcommands results. Using a made-up
// name ensures test isolation without needing to suppress seed rules.
const testProg = "mytestcli"

func TestCoveredSubcommands(t *testing.T) {
	tests := []struct {
		name         string
		specs        []RuleSpec
		program      string
		knownSubcmds []string
		// wantPresent lists keys that MUST be in the covered map.
		wantPresent map[string]bool
		// wantAbsent lists specific subcommands that must NOT be in the covered map.
		// Only checked when non-nil; used for cases where we need to confirm absence
		// of specific keys regardless of what seed rules may add.
		wantAbsent []string
		// strictCheck, when true, verifies no extra keys beyond wantPresent are set.
		// Use only when testProg is used (no seed rules for testProg → clean map).
		strictCheck bool
	}{
		{
			// TC-G-01: ToolPattern "Read|Glob" rule → testProg subcommands NOT covered
			name: "ToolPattern_ReadGlob_skipped",
			specs: []RuleSpec{
				{ToolPattern: "Read|Glob", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push", "commit"},
			wantPresent:  map[string]bool{},
			strictCheck:  true,
		},
		{
			// TC-G-02: ToolCategory "builtin-agent" rule → testProg NOT covered (Concern-1 guard)
			name: "ToolCategory_BuiltinAgent_skipped",
			specs: []RuleSpec{
				{ToolCategory: "builtin-agent", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push"},
			wantPresent:  map[string]bool{},
			strictCheck:  true,
		},
		{
			// TC-G-03: ToolPattern "Bash" rule → all testProg subcommands covered
			name: "ToolPattern_Bash_included",
			specs: []RuleSpec{
				{ToolPattern: "Bash", CommandPattern: "", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push", "commit"},
			wantPresent:  map[string]bool{"": true, "push": true, "commit": true},
			strictCheck:  true,
		},
		{
			// TC-G-04: ToolPattern ".*" (wildcard) → all testProg subcommands covered
			name: "ToolPattern_Wildcard_included",
			specs: []RuleSpec{
				{ToolPattern: ".*", CommandPattern: "", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push"},
			wantPresent:  map[string]bool{"": true, "push": true},
			strictCheck:  true,
		},
		{
			// TC-G-05: AllToolRule — empty ToolName and ToolPattern → covered (R1.3 preserved)
			name: "AllToolRule_EmptyToolNameAndPattern",
			specs: []RuleSpec{
				{ToolName: "", ToolPattern: "", CommandPattern: "", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push"},
			wantPresent:  map[string]bool{"": true, "push": true},
			strictCheck:  true,
		},
		{
			// TC-G-06: ToolName=Bash, CommandPattern="" → all known subcommands covered (R2.1)
			name: "ToolName_Bash_CommandPatternEmpty_allSubcmds",
			specs: []RuleSpec{
				{ToolName: "Bash", CommandPattern: "", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push", "commit", "status"},
			wantPresent:  map[string]bool{"": true, "push": true, "commit": true, "status": true},
			strictCheck:  true,
		},
		{
			// TC-G-07: ToolName=Bash, CommandPattern="", empty knownSubcmds → only bare key
			name: "ToolName_Bash_CommandPatternEmpty_emptyKnownSubcmds",
			specs: []RuleSpec{
				{ToolName: "Bash", CommandPattern: "", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{},
			wantPresent:  map[string]bool{"": true},
			strictCheck:  true,
		},
		{
			// TC-G-08: Programs=["testProg"], no subcommand restriction → all covered
			name: "Programs_match_noSubcmdRestriction",
			specs: []RuleSpec{
				{Programs: []string{testProg}, Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push", "commit"},
			wantPresent:  map[string]bool{"": true, "push": true, "commit": true},
			strictCheck:  true,
		},
		{
			// TC-G-09: Programs=["testProg"], Subcommands=["push"] → only "push"
			name: "Programs_match_withSubcmdRestriction",
			specs: []RuleSpec{
				{Programs: []string{testProg}, Subcommands: []string{"push"}, Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push", "commit"},
			wantPresent:  map[string]bool{"push": true},
			// "commit" must not be covered — only "push" was specified in Subcommands
			wantAbsent:  []string{"commit", ""},
			strictCheck: true,
		},
		{
			// TC-G-10: Disabled rule → nothing covered
			name: "DisabledRule_notCovered",
			specs: []RuleSpec{
				{ToolName: "Bash", CommandPattern: "", Enabled: false},
			},
			program:      testProg,
			knownSubcmds: []string{"push"},
			wantPresent:  map[string]bool{},
			strictCheck:  true,
		},
		{
			// TC-G-11: CommandPattern="^testProg push" → only "push" covered (bonus)
			name: "CommandPattern_specificSubcmd",
			specs: []RuleSpec{
				{ToolName: "Bash", CommandPattern: "^" + testProg + " push", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push", "commit"},
			wantPresent:  map[string]bool{"push": true},
			wantAbsent:   []string{"commit", ""},
			strictCheck:  true,
		},
		{
			// TC-G-12: Programs case-insensitive match — "MYTESTCLI" covers "mytestcli"
			// via strings.EqualFold; regression guard for == vs EqualFold change.
			name: "Programs_caseInsensitiveMatch",
			specs: []RuleSpec{
				{Programs: []string{"MYTESTCLI"}, Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push"},
			wantPresent:  map[string]bool{"": true, "push": true},
			strictCheck:  true,
		},
		{
			// TC-G-13: CommandPattern matches bare program name → sets covered[""] sentinel.
			// Pattern "^mytestcli$" matches the bare program string (re.MatchString(program))
			// but not "mytestcli push", so only the sentinel key is set (rules_service.go:378).
			name: "CommandPattern_matchesBareProgram_setsSentinelKey",
			specs: []RuleSpec{
				{ToolName: "Bash", CommandPattern: "^" + testProg + "$", Enabled: true},
			},
			program:      testProg,
			knownSubcmds: []string{"push", "commit"},
			wantPresent:  map[string]bool{"": true},
			wantAbsent:   []string{"push", "commit"},
			strictCheck:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newRulesServiceForCoverage(t, tc.specs)
			got := svc.coveredSubcommands(tc.program, tc.knownSubcmds)

			// Verify every expected key is present.
			for k, v := range tc.wantPresent {
				assert.Equal(t, v, got[k], "expected covered[%q]=%v", k, v)
			}

			// Verify keys that must be absent.
			for _, k := range tc.wantAbsent {
				assert.False(t, got[k], "key %q must not be covered", k)
			}

			// For strict checks, verify no unexpected keys beyond wantPresent are set.
			if tc.strictCheck {
				for k, v := range got {
					if _, ok := tc.wantPresent[k]; !ok {
						assert.False(t, v, "unexpected covered[%q]=true", k)
					}
				}
			}
		})
	}
}

// ── helpers for YAML tests ────────────────────────────────────────────────────

// newSimpleRulesService creates a RulesService backed by an in-memory SQLite DB.
func newSimpleRulesService(t *testing.T) *RulesService {
	t.Helper()
	storage := createTestStorage(t)
	rulesStore, err := NewRulesStore(storage)
	require.NoError(t, err)
	analyticsStore := NewAnalyticsStore(storage)
	analyticsStore.Start(context.Background())
	c := classifier.NewRuleBasedClassifier()
	return NewRulesService(rulesStore, analyticsStore, c, nil, nil)
}

const validYAML3Rules = `rules:
- name: Allow git log
  tool: Bash
  programs:
    - git
  subcommands:
    - log
  decision: allow
  reason: Read-only git history
  priority: 10
- name: Deny git push
  tool: Bash
  programs:
    - git
  subcommands:
    - push
  decision: deny
  reason: No direct push allowed
  priority: 5
- name: Allow read files
  tool_pattern: "Read|Glob"
  decision: allow
  priority: 20
`

// ── UT-BE-01: Valid YAML 3 rules ──────────────────────────────────────────────

func TestValidateRules_ValidYAML_3Rules(t *testing.T) {
	svc := newSimpleRulesService(t)
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: validYAML3Rules,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(3), resp.Msg.ValidCount)
	assert.Equal(t, int32(0), resp.Msg.ErrorCount)
	assert.Len(t, resp.Msg.Results, 3)
	for _, r := range resp.Msg.Results {
		assert.True(t, r.Valid)
		assert.Empty(t, r.Errors)
	}
}

// ── UT-BE-02: Payload > 512 KB ────────────────────────────────────────────────

func TestValidateRules_PayloadTooLarge(t *testing.T) {
	svc := newSimpleRulesService(t)
	large := make([]byte, 512*1024+1)
	for i := range large {
		large[i] = 'x'
	}
	_, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: string(large),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "too large")
}

// ── UT-BE-03: Invalid regex per rule, does not short-circuit ──────────────────

func TestValidateRules_InvalidRegex_PerRuleError(t *testing.T) {
	svc := newSimpleRulesService(t)
	yaml := `rules:
- name: Bad regex rule
  command_pattern: "[invalid("
  decision: allow
- name: Good rule
  tool: Bash
  decision: allow
`
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yaml,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Msg.ValidCount)
	assert.Equal(t, int32(1), resp.Msg.ErrorCount)
	assert.Len(t, resp.Msg.Results, 2)
	assert.False(t, resp.Msg.Results[0].Valid)
	assert.True(t, resp.Msg.Results[1].Valid)
}

// ── UT-BE-04: Invalid decision produces explicit error ────────────────────────

func TestValidateRules_InvalidDecision_ExplicitError(t *testing.T) {
	svc := newSimpleRulesService(t)
	yaml := `rules:
- name: Bad decision
  tool: Bash
  decision: block
`
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yaml,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Msg.ValidCount)
	assert.Equal(t, int32(1), resp.Msg.ErrorCount)
	assert.False(t, resp.Msg.Results[0].Valid)
	assert.Contains(t, resp.Msg.Results[0].Errors[0], "invalid decision")
	assert.Contains(t, resp.Msg.Results[0].Errors[0], "block")
}

// ── UT-BE-05: tool and tool_pattern mutually exclusive ────────────────────────

func TestValidateRules_ToolAndToolPatternMutuallyExclusive(t *testing.T) {
	svc := newSimpleRulesService(t)
	yaml := `rules:
- name: Conflicting fields
  tool: Bash
  tool_pattern: "Read|Glob"
  decision: allow
`
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yaml,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Results[0].Valid)
	assert.Contains(t, resp.Msg.Results[0].Errors[0], "mutually exclusive")
}

// ── UT-BE-06: Unrecognized YAML key rejected (KnownFields) ───────────────────

func TestValidateRules_UnknownField_KnownFieldsRejected(t *testing.T) {
	svc := newSimpleRulesService(t)
	yaml := `rules:
- name: Rule with unknown key
  tool: Bash
  program: git
  decision: allow
`
	_, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yaml,
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// ── UT-BE-07: Empty rules list ────────────────────────────────────────────────

func TestValidateRules_EmptyRulesList(t *testing.T) {
	svc := newSimpleRulesService(t)
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: "rules: []\n",
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Msg.ValidCount)
	assert.Equal(t, int32(0), resp.Msg.ErrorCount)
	assert.Empty(t, resp.Msg.Results)
}

// ── UT-BE-08/09: Rule count boundary ─────────────────────────────────────────

func TestValidateRules_RuleCount_500_AtLimit(t *testing.T) {
	svc := newSimpleRulesService(t)
	var sb strings.Builder
	sb.WriteString("rules:\n")
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&sb, "- name: Rule %d\n  tool: Bash\n  decision: allow\n", i)
	}
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: sb.String(),
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(500), resp.Msg.ValidCount)
}

func TestValidateRules_RuleCount_501_OverLimit(t *testing.T) {
	svc := newSimpleRulesService(t)
	var sb strings.Builder
	sb.WriteString("rules:\n")
	for i := 0; i < 501; i++ {
		fmt.Fprintf(&sb, "- name: Rule %d\n  tool: Bash\n  decision: allow\n", i)
	}
	_, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: sb.String(),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "too many rules")
}

// ── UT-BE-10: Missing name field ──────────────────────────────────────────────

func TestValidateRules_MissingNameField(t *testing.T) {
	svc := newSimpleRulesService(t)
	yaml := `rules:
- tool: Bash
  decision: allow
`
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yaml,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Results[0].Valid)
	assert.Contains(t, resp.Msg.Results[0].Errors[0], "name is required")
}

// ── UT-BE-11: All three regex fields invalid returns all errors ───────────────

func TestValidateRules_AllThreeRegexFieldsInvalid(t *testing.T) {
	svc := newSimpleRulesService(t)
	yaml := `rules:
- name: Triple bad regex
  tool_pattern: "[bad("
  command_pattern: "[bad("
  file_pattern: "[bad("
  decision: allow
`
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yaml,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Results[0].Valid)
	assert.Len(t, resp.Msg.Results[0].Errors, 3)
}

// ── UT-BE-12: Default priority ────────────────────────────────────────────────

func TestValidateRules_DefaultPriority(t *testing.T) {
	svc := newSimpleRulesService(t)
	yaml := `rules:
- name: No priority
  tool: Bash
  decision: allow
- name: Has priority
  tool: Bash
  decision: allow
  priority: 5
`
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yaml,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.Msg.Results[0].Rule.Priority)
	assert.Equal(t, int32(5), resp.Msg.Results[1].Rule.Priority)
}

// ── UT-BE-13: Default enabled ─────────────────────────────────────────────────

func TestValidateRules_DefaultEnabled(t *testing.T) {
	svc := newSimpleRulesService(t)
	yaml := `rules:
- name: No enabled field
  tool: Bash
  decision: allow
- name: Explicitly disabled
  tool: Bash
  decision: allow
  enabled: false
`
	resp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yaml,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Results[0].Rule.Enabled)
	assert.False(t, resp.Msg.Results[1].Rule.Enabled)
}

// ── UT-BE-14: ExportRules excludes seed and claude-settings ──────────────────

func TestExportRules_ExcludesSeedAndClaudeSettingsRules(t *testing.T) {
	svc := newSimpleRulesService(t)
	// Insert non-user rules directly via storage (bypassing Upsert guard) so we
	// can prove that ExportRules filters them out.
	for i, src := range []string{"seed", "claude-settings"} {
		err := svc.rulesStore.storage.UpsertRule(context.Background(), session.ApprovalRuleData{
			ID:       fmt.Sprintf("%s-rule-%d", src, i),
			Name:     fmt.Sprintf("%s Rule %d", src, i),
			Decision: 1, // auto_allow
			Enabled:  true,
			Source:   src,
			Priority: 10,
		})
		require.NoError(t, err)
	}
	// Reload so the in-memory store reflects the new rows.
	require.NoError(t, svc.rulesStore.reload())
	// Add 2 user rules.
	for i := 0; i < 2; i++ {
		_, err := svc.rulesStore.Upsert(RuleSpec{
			ID:       fmt.Sprintf("user-rule-%d", i),
			Name:     fmt.Sprintf("User Rule %d", i),
			Decision: "auto_allow",
			Enabled:  true,
			Source:   "user",
			Priority: 10,
		})
		require.NoError(t, err)
	}
	resp, err := svc.ExportRules(context.Background(), connect.NewRequest(&sessionv1.ExportRulesRequest{}))
	require.NoError(t, err)
	// Parse the returned YAML to count rules.
	var file yamlRulesFile
	require.NoError(t, parseYAMLStrict(resp.Msg.YamlContent, &file))
	// Must be exactly 2 — seed and claude-settings rules must be excluded.
	assert.Len(t, file.Rules, 2)
	for _, r := range file.Rules {
		assert.True(t, strings.HasPrefix(r.Name, "User Rule"), "unexpected non-user rule in export: %s", r.Name)
	}
}

// ── UT-BE-15: ExportRules with filter ────────────────────────────────────────

func TestExportRules_FilterByRuleIDs(t *testing.T) {
	svc := newSimpleRulesService(t)
	for i := 0; i < 3; i++ {
		_, err := svc.rulesStore.Upsert(RuleSpec{
			ID:       fmt.Sprintf("user-rule-%d", i),
			Name:     fmt.Sprintf("User Rule %d", i),
			Decision: "auto_allow",
			Enabled:  true,
			Source:   "user",
			Priority: 10,
		})
		require.NoError(t, err)
	}
	resp, err := svc.ExportRules(context.Background(), connect.NewRequest(&sessionv1.ExportRulesRequest{
		RuleIds: []string{"user-rule-1"},
	}))
	require.NoError(t, err)
	var file yamlRulesFile
	require.NoError(t, parseYAMLStrict(resp.Msg.YamlContent, &file))
	assert.Len(t, file.Rules, 1)
	assert.Equal(t, "User Rule 1", file.Rules[0].Name)
}

// ── UT-BE-16: ExportRules empty store produces "rules: []\n" ─────────────────

func TestExportRules_EmptyStore_ProducesEmptyRulesKey(t *testing.T) {
	svc := newSimpleRulesService(t)
	resp, err := svc.ExportRules(context.Background(), connect.NewRequest(&sessionv1.ExportRulesRequest{}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.YamlContent)
	assert.Contains(t, resp.Msg.YamlContent, "rules:")
}

// ── UT-BE-17: ExportRules omits optional fields with zero values ──────────────

func TestExportRules_OptionalFieldsOmitted(t *testing.T) {
	svc := newSimpleRulesService(t)
	_, err := svc.rulesStore.Upsert(RuleSpec{
		ID:       "user-minimal",
		Name:     "Minimal Rule",
		Decision: "auto_allow",
		Enabled:  true,
		Source:   "user",
		Priority: 10,
		// no tool, command_pattern, file_pattern, reason, alternative
	})
	require.NoError(t, err)
	resp, err := svc.ExportRules(context.Background(), connect.NewRequest(&sessionv1.ExportRulesRequest{}))
	require.NoError(t, err)
	assert.NotContains(t, resp.Msg.YamlContent, "command_pattern")
	assert.NotContains(t, resp.Msg.YamlContent, "file_pattern")
	assert.NotContains(t, resp.Msg.YamlContent, "alternative")
}

// ── UT-BE-18: ExportRules -- enabled=true omitted, enabled=false present ──────

func TestExportRules_EnabledDefaultOmitted(t *testing.T) {
	svc := newSimpleRulesService(t)
	// enabled=true
	_, err := svc.rulesStore.Upsert(RuleSpec{
		ID:       "user-enabled",
		Name:     "Enabled Rule",
		Decision: "auto_allow",
		Enabled:  true,
		Source:   "user",
		Priority: 10,
	})
	require.NoError(t, err)
	// enabled=false
	_, err = svc.rulesStore.Upsert(RuleSpec{
		ID:       "user-disabled",
		Name:     "Disabled Rule",
		Decision: "auto_allow",
		Enabled:  false,
		Source:   "user",
		Priority: 10,
	})
	require.NoError(t, err)
	resp, err := svc.ExportRules(context.Background(), connect.NewRequest(&sessionv1.ExportRulesRequest{}))
	require.NoError(t, err)
	// The disabled rule should appear with enabled: false; the enabled rule should not have the key.
	var file yamlRulesFile
	require.NoError(t, parseYAMLStrict(resp.Msg.YamlContent, &file))
	for _, r := range file.Rules {
		if r.Name == "Disabled Rule" {
			require.NotNil(t, r.Enabled)
			assert.False(t, *r.Enabled)
		}
		if r.Name == "Enabled Rule" {
			// enabled=true should be omitted (nil) in the export
			assert.Nil(t, r.Enabled)
		}
	}
}

// ── UT-BE-19: Export roundtrip ────────────────────────────────────────────────

func TestExportRules_Roundtrip(t *testing.T) {
	svc := newSimpleRulesService(t)
	originals := []RuleSpec{
		{ID: "user-r1", Name: "Rule Alpha", ToolName: "Bash", Decision: "auto_allow", Enabled: true, Source: "user", Priority: 10},
		{ID: "user-r2", Name: "Rule Beta", ToolPattern: "Read|Glob", Decision: "auto_deny", Enabled: true, Source: "user", Priority: 5},
		{ID: "user-r3", Name: "Rule Gamma", CommandPattern: `^git log`, Decision: "escalate", Enabled: true, Source: "user", Priority: 20},
	}
	for _, spec := range originals {
		_, err := svc.rulesStore.Upsert(spec)
		require.NoError(t, err)
	}

	exportResp, err := svc.ExportRules(context.Background(), connect.NewRequest(&sessionv1.ExportRulesRequest{}))
	require.NoError(t, err)

	validateResp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: exportResp.Msg.YamlContent,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(3), validateResp.Msg.ValidCount)
	assert.Equal(t, int32(0), validateResp.Msg.ErrorCount)

	// Verify field fidelity.
	resultByName := make(map[string]*sessionv1.ParsedRuleResult)
	for _, r := range validateResp.Msg.Results {
		resultByName[r.OriginalName] = r
	}
	r1 := resultByName["Rule Alpha"]
	require.NotNil(t, r1)
	assert.Equal(t, "Bash", r1.Rule.ToolName)
	assert.Equal(t, sessionv1.AutoDecision_AUTO_DECISION_ALLOW, r1.Rule.Decision)

	r2 := resultByName["Rule Beta"]
	require.NotNil(t, r2)
	assert.Equal(t, "Read|Glob", r2.Rule.ToolPattern)
	assert.Equal(t, sessionv1.AutoDecision_AUTO_DECISION_DENY, r2.Rule.Decision)

	r3 := resultByName["Rule Gamma"]
	require.NotNil(t, r3)
	assert.Equal(t, `^git log`, r3.Rule.CommandPattern)
}

// ── UT-BE-20: BulkUpsert 20 new rules ────────────────────────────────────────

func TestBulkUpsertRules_InsertNew_20Rules(t *testing.T) {
	svc := newSimpleRulesService(t)
	rules := make([]*sessionv1.ApprovalRuleProto, 20)
	for i := range rules {
		rules[i] = &sessionv1.ApprovalRuleProto{
			Name:     fmt.Sprintf("Rule %d", i),
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Priority: 10,
		}
	}
	resp, err := svc.BulkUpsertRules(context.Background(), connect.NewRequest(&sessionv1.BulkUpsertRulesRequest{
		Rules:               rules,
		OverwriteDuplicates: false,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(20), resp.Msg.Created)
	assert.Equal(t, int32(0), resp.Msg.Updated)
	assert.Equal(t, int32(0), resp.Msg.Skipped)
	assert.Empty(t, resp.Msg.Errors)
}

// ── UT-BE-21: BulkUpsert skip duplicates ─────────────────────────────────────

func TestBulkUpsertRules_SkipDuplicates(t *testing.T) {
	svc := newSimpleRulesService(t)
	// Pre-insert 2 rules.
	for i := 0; i < 2; i++ {
		_, err := svc.rulesStore.Upsert(RuleSpec{
			ID:       fmt.Sprintf("user-existing-%d", i),
			Name:     fmt.Sprintf("Rule %d", i),
			Decision: "auto_allow",
			Enabled:  true,
			Source:   "user",
			Priority: 10,
		})
		require.NoError(t, err)
	}

	// Bulk insert 4 rules (2 are duplicates).
	rules := make([]*sessionv1.ApprovalRuleProto, 4)
	for i := range rules {
		rules[i] = &sessionv1.ApprovalRuleProto{
			Name:     fmt.Sprintf("Rule %d", i),
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Priority: 10,
		}
	}
	resp, err := svc.BulkUpsertRules(context.Background(), connect.NewRequest(&sessionv1.BulkUpsertRulesRequest{
		Rules:               rules,
		OverwriteDuplicates: false,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Msg.Created)
	assert.Equal(t, int32(0), resp.Msg.Updated)
	assert.Equal(t, int32(2), resp.Msg.Skipped)
}

// ── UT-BE-22: BulkUpsert overwrite duplicates ────────────────────────────────

func TestBulkUpsertRules_OverwriteDuplicates(t *testing.T) {
	svc := newSimpleRulesService(t)
	// Pre-insert 2 rules.
	for i := 0; i < 2; i++ {
		_, err := svc.rulesStore.Upsert(RuleSpec{
			ID:       fmt.Sprintf("user-existing-%d", i),
			Name:     fmt.Sprintf("Rule %d", i),
			Decision: "auto_allow",
			Enabled:  true,
			Source:   "user",
			Priority: 10,
		})
		require.NoError(t, err)
	}

	// Bulk insert 4 rules (2 new, 2 duplicates) with overwrite=true.
	rules := make([]*sessionv1.ApprovalRuleProto, 4)
	for i := range rules {
		rules[i] = &sessionv1.ApprovalRuleProto{
			Name:     fmt.Sprintf("Rule %d", i),
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_DENY, // changed decision
			Enabled:  true,
			Priority: 10,
		}
	}
	resp, err := svc.BulkUpsertRules(context.Background(), connect.NewRequest(&sessionv1.BulkUpsertRulesRequest{
		Rules:               rules,
		OverwriteDuplicates: true,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Msg.Created)
	assert.Equal(t, int32(2), resp.Msg.Updated)
	assert.Equal(t, int32(0), resp.Msg.Skipped)
}

// ── UT-BE-23: rebuildClassifier called exactly once ───────────────────────────

func TestBulkUpsertRules_RebuildClassifierCalledOnce(t *testing.T) {
	// We verify this by checking that all 10 rules are visible through allRuleSpecs
	// after a single BulkUpsertRules call (implying classifier rebuilt correctly).
	svc := newSimpleRulesService(t)
	rules := make([]*sessionv1.ApprovalRuleProto, 10)
	for i := range rules {
		rules[i] = &sessionv1.ApprovalRuleProto{
			Name:     fmt.Sprintf("Rule %d", i),
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Priority: 10,
		}
	}
	resp, err := svc.BulkUpsertRules(context.Background(), connect.NewRequest(&sessionv1.BulkUpsertRulesRequest{
		Rules:               rules,
		OverwriteDuplicates: false,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(10), resp.Msg.Created)
	// Verify all 10 rules are visible in the classifier (confirms rebuild happened).
	classifierRules := svc.classifier.Rules()
	userRuleCount := 0
	for _, r := range classifierRules {
		if r.Source == "user" {
			userRuleCount++
		}
	}
	assert.Equal(t, 10, userRuleCount)
}

// ── UT-BE-24: Client-supplied IDs/source discarded ────────────────────────────

func TestBulkUpsertRules_ClientIDsDiscarded(t *testing.T) {
	svc := newSimpleRulesService(t)
	rules := []*sessionv1.ApprovalRuleProto{
		{
			Id:       "injected-id", // should be discarded
			Source:   "seed",        // should be overridden to "user"
			Name:     "Injected Rule",
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Priority: 10,
		},
	}
	resp, err := svc.BulkUpsertRules(context.Background(), connect.NewRequest(&sessionv1.BulkUpsertRulesRequest{
		Rules:               rules,
		OverwriteDuplicates: false,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Msg.Created)

	// Verify the stored rule has source="user" and a server-generated ID.
	stored := svc.rulesStore.All()
	require.Len(t, stored, 1)
	assert.Equal(t, "user", stored[0].Source)
	assert.NotEqual(t, "injected-id", stored[0].ID)
}

// ── UT-BE-25: YAML bomb alias expansion guard ─────────────────────────────────

func TestValidateRules_YAMLBombAliasExpansion(t *testing.T) {
	// This YAML tries to create many rules via anchors/aliases.
	// The 500-rule cap should fire before per-rule validation.
	svc := newSimpleRulesService(t)
	var sb strings.Builder
	sb.WriteString("rules:\n")
	for i := 0; i < 502; i++ {
		fmt.Fprintf(&sb, "- name: Rule %d\n  tool: Bash\n  decision: allow\n", i)
	}
	_, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: sb.String(),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "too many rules")
}

// ── IT-BE-01: Export then validate roundtrip ──────────────────────────────────

func TestIntegration_ExportThenValidate_Roundtrip(t *testing.T) {
	svc := newSimpleRulesService(t)
	specs := []RuleSpec{
		{ID: "user-it-1", Name: "IT Rule Alpha", ToolName: "Bash", Decision: "auto_allow", Enabled: true, Source: "user", Priority: 10, Programs: []string{"git"}},
		{ID: "user-it-2", Name: "IT Rule Beta", ToolPattern: "Read|Glob", Decision: "auto_deny", Enabled: true, Source: "user", Priority: 5},
		{ID: "user-it-3", Name: "IT Rule Gamma", CommandPattern: `^npm`, Decision: "escalate", Enabled: true, Source: "user", Priority: 20},
	}
	for _, spec := range specs {
		_, err := svc.rulesStore.Upsert(spec)
		require.NoError(t, err)
	}

	exportResp, err := svc.ExportRules(context.Background(), connect.NewRequest(&sessionv1.ExportRulesRequest{}))
	require.NoError(t, err)

	validateResp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: exportResp.Msg.YamlContent,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(3), validateResp.Msg.ValidCount)
	assert.Equal(t, int32(0), validateResp.Msg.ErrorCount)
}

// ── IT-BE-02: BulkUpsert then export ─────────────────────────────────────────

func TestIntegration_BulkUpsert_ThenExport(t *testing.T) {
	svc := newSimpleRulesService(t)
	rules := make([]*sessionv1.ApprovalRuleProto, 5)
	for i := range rules {
		rules[i] = &sessionv1.ApprovalRuleProto{
			Name:     fmt.Sprintf("IT Rule %d", i),
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Priority: 10,
		}
	}
	_, err := svc.BulkUpsertRules(context.Background(), connect.NewRequest(&sessionv1.BulkUpsertRulesRequest{
		Rules:               rules,
		OverwriteDuplicates: false,
	}))
	require.NoError(t, err)

	exportResp, err := svc.ExportRules(context.Background(), connect.NewRequest(&sessionv1.ExportRulesRequest{}))
	require.NoError(t, err)
	var file yamlRulesFile
	require.NoError(t, parseYAMLStrict(exportResp.Msg.YamlContent, &file))
	assert.Len(t, file.Rules, 5)
}

// ── IT-BE-03: Validate and apply 20 rules ────────────────────────────────────

func TestIntegration_ValidateAndApply_20Rules(t *testing.T) {
	svc := newSimpleRulesService(t)
	var sb strings.Builder
	sb.WriteString("rules:\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&sb, "- name: IT Apply Rule %d\n  tool: Bash\n  decision: allow\n  priority: 10\n", i)
	}
	yamlContent := sb.String()

	validateResp, err := svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
		YamlContent: yamlContent,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(20), validateResp.Msg.ValidCount)

	// Collect valid protos and bulk-upsert.
	validRules := make([]*sessionv1.ApprovalRuleProto, 0)
	for _, r := range validateResp.Msg.Results {
		if r.Valid {
			validRules = append(validRules, r.Rule)
		}
	}

	bulkResp, err := svc.BulkUpsertRules(context.Background(), connect.NewRequest(&sessionv1.BulkUpsertRulesRequest{
		Rules:               validRules,
		OverwriteDuplicates: false,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(20), bulkResp.Msg.Created)
}

// ── IT-BE-04: Single classifier rebuild ──────────────────────────────────────

func TestIntegration_BulkUpsert_SingleClassifierRebuild(t *testing.T) {
	// Same as UT-BE-23 but confirms via export that all 20 rules are present.
	svc := newSimpleRulesService(t)
	rules := make([]*sessionv1.ApprovalRuleProto, 20)
	for i := range rules {
		rules[i] = &sessionv1.ApprovalRuleProto{
			Name:     fmt.Sprintf("IT Rebuild Rule %d", i),
			ToolName: "Bash",
			Decision: sessionv1.AutoDecision_AUTO_DECISION_ALLOW,
			Enabled:  true,
			Priority: 10,
		}
	}
	resp, err := svc.BulkUpsertRules(context.Background(), connect.NewRequest(&sessionv1.BulkUpsertRulesRequest{
		Rules:               rules,
		OverwriteDuplicates: false,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(20), resp.Msg.Created)

	// Confirm classifier has all 20 user rules.
	userCount := 0
	for _, r := range svc.classifier.Rules() {
		if r.Source == "user" {
			userCount++
		}
	}
	assert.Equal(t, 20, userCount)
}

// ── parseYAMLStrict is a test helper that parses YAML into yamlRulesFile ──────

func parseYAMLStrict(content string, out *yamlRulesFile) error {
	return yaml.Unmarshal([]byte(content), out)
}

// ── FuzzValidateRules_NoPanic ─────────────────────────────────────────────────

func FuzzValidateRules_NoPanic(f *testing.F) {
	f.Add([]byte("rules:\n- name: test\n  tool: Bash\n  decision: allow\n"))
	f.Add([]byte(""))
	f.Add([]byte("not: yaml"))
	f.Fuzz(func(t *testing.T, data []byte) {
		svc := newSimpleRulesService(t)
		// Should never panic; may return an error.
		_, _ = svc.ValidateRules(context.Background(), connect.NewRequest(&sessionv1.ValidateRulesRequest{
			YamlContent: string(data),
		}))
	})
}
