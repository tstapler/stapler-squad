package services

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

func TestClassify_DailyBucketAutoApproveRate(t *testing.T) {
	b := DailyBucket{
		Date:      "2026-04-13",
		AutoAllow: 8,
		AutoDeny:  1,
		Escalate:  1,
		Total:     10,
	}
	got := b.AutoApproveRate()
	if got != 0.8 {
		t.Errorf("AutoApproveRate() = %v, want 0.8", got)
	}

	empty := DailyBucket{}
	if empty.AutoApproveRate() != 0 {
		t.Errorf("AutoApproveRate() on zero bucket = %v, want 0", empty.AutoApproveRate())
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// mustCompileRe compiles a regexp pattern and fails the test if it is invalid.
func mustCompileRe(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("mustCompileRe(%q): %v", pattern, err)
	}
	return re
}

// ── TestReclassifyGaps ───────────────────────────────────────────────────────

func TestReclassifyGaps_should_reclassifyEntry_When_ruleNowCoversCommand(t *testing.T) {
	// TC-G-12: Classifier has rule matching "git push". Entry is escalate with no RuleID.
	c := classifier.NewRuleBasedClassifier()
	rules := []classifier.Rule{
		{
			ID:             "rule-git-push",
			Name:           "Allow git push",
			ToolName:       "Bash",
			CommandPattern: mustCompileRe(t, "^git push"),
			Decision:       classifier.AutoAllow,
			Enabled:        true,
			Priority:       100,
			Source:         "user",
		},
	}
	c.ReplaceRules(rules)

	entries := []AnalyticsEntry{
		{
			ID:             "e1",
			ToolName:       "Bash",
			CommandPreview: "git push origin main",
			Decision:       "escalate",
			RuleID:         "",
		},
	}

	result := ReclassifyGaps(entries, c)
	require.Len(t, result, 1)
	assert.Equal(t, "auto_allow", result[0].Decision, "TC-G-12: escalated entry should be reclassified to auto_allow")
	assert.NotEmpty(t, result[0].RuleID, "TC-G-12: reclassified entry must have a RuleID")
}

func TestReclassifyGaps_should_skipEntry_When_alreadyDecided(t *testing.T) {
	// TC-G-13: Entry with Decision="auto_allow" is unchanged.
	c := classifier.NewRuleBasedClassifier()

	entries := []AnalyticsEntry{
		{
			ID:             "e1",
			ToolName:       "Bash",
			CommandPreview: "git push origin main",
			Decision:       "auto_allow",
			RuleID:         "",
		},
	}

	result := ReclassifyGaps(entries, c)
	require.Len(t, result, 1)
	assert.Equal(t, "auto_allow", result[0].Decision, "TC-G-13: auto_allow entry must remain unchanged")
}

func TestReclassifyGaps_should_skipEntry_When_hasRuleID(t *testing.T) {
	// TC-G-14: Entry with Decision="escalate" and RuleID != "" is unchanged.
	c := classifier.NewRuleBasedClassifier()
	rules := []classifier.Rule{
		{
			ID:             "rule-git-push",
			Name:           "Allow git push",
			ToolName:       "Bash",
			CommandPattern: mustCompileRe(t, "^git push"),
			Decision:       classifier.AutoAllow,
			Enabled:        true,
			Priority:       100,
			Source:         "user",
		},
	}
	c.ReplaceRules(rules)

	entries := []AnalyticsEntry{
		{
			ID:             "e1",
			ToolName:       "Bash",
			CommandPreview: "git push origin main",
			Decision:       "escalate",
			RuleID:         "some-rule-id",
		},
	}

	result := ReclassifyGaps(entries, c)
	require.Len(t, result, 1)
	assert.Equal(t, "escalate", result[0].Decision, "TC-G-14: escalate entry with RuleID must not be reclassified")
	assert.Equal(t, "some-rule-id", result[0].RuleID, "TC-G-14: RuleID must remain unchanged")
}

func TestReclassifyGaps_should_notMutateOriginalSlice(t *testing.T) {
	// TC-G-15: Original slice entries must remain unchanged after reclassification.
	c := classifier.NewRuleBasedClassifier()
	rules := []classifier.Rule{
		{
			ID:             "rule-git-push",
			Name:           "Allow git push",
			ToolName:       "Bash",
			CommandPattern: mustCompileRe(t, "^git push"),
			Decision:       classifier.AutoAllow,
			Enabled:        true,
			Priority:       100,
			Source:         "user",
		},
	}
	c.ReplaceRules(rules)

	original := []AnalyticsEntry{
		{
			ID:             "e1",
			ToolName:       "Bash",
			CommandPreview: "git push origin main",
			Decision:       "escalate",
			RuleID:         "",
		},
	}
	origDecision := original[0].Decision
	origRuleID := original[0].RuleID

	result := ReclassifyGaps(original, c)

	// Original must not have been mutated.
	assert.Equal(t, origDecision, original[0].Decision, "TC-G-15: original slice must not be mutated")
	assert.Equal(t, origRuleID, original[0].RuleID, "TC-G-15: original RuleID must not be mutated")
	// Result should have been reclassified.
	assert.Equal(t, "auto_allow", result[0].Decision, "TC-G-15: returned entry should be reclassified")
}

func TestReclassifyGaps_should_handleCommandUnder200Chars(t *testing.T) {
	// TC-G-16 (R3.3): Short command (well under 200 chars); classifier rule uses
	// CriteriaPrograms matching "git". Verifies truncation is irrelevant for typical commands.
	c := classifier.NewRuleBasedClassifier()
	rules := []classifier.Rule{
		{
			ID:   "rule-git-all",
			Name: "Allow all git",
			Criteria: &classifier.CommandCriteria{
				Programs: []string{"git"},
			},
			Decision: classifier.AutoAllow,
			Enabled:  true,
			Priority: 100,
			Source:   "user",
		},
	}
	c.ReplaceRules(rules)

	cmd := "git status --short --branch" // 28 chars, well under 200
	require.Less(t, len(cmd), 200, "prerequisite: command must be under 200 chars")

	entries := []AnalyticsEntry{
		{
			ID:             "e1",
			ToolName:       "Bash",
			CommandPreview: cmd,
			Decision:       "escalate",
			RuleID:         "",
		},
	}

	result := ReclassifyGaps(entries, c)
	require.Len(t, result, 1)
	assert.Equal(t, "auto_allow", result[0].Decision, "TC-G-16 (R3.3): short command entry should be reclassified")
}

// ── TestComputeSummary ───────────────────────────────────────────────────────

func TestComputeSummary_should_countCoverageGaps_When_escalateNoRuleID(t *testing.T) {
	// TC-G-17: 3 entries: 2 escalate+no-ruleID, 1 auto_allow → CoverageGapCount=2
	entries := []AnalyticsEntry{
		{Decision: "escalate", RuleID: "", ToolName: "Bash"},
		{Decision: "escalate", RuleID: "", ToolName: "Bash"},
		{Decision: "auto_allow", RuleID: "rule-1", ToolName: "Bash"},
	}

	s := ComputeSummary(entries)
	assert.Equal(t, 2, s.CoverageGapCount, "TC-G-17: CoverageGapCount must equal number of escalate+no-ruleID entries")
}

func TestComputeSummary_should_notCountGap_When_escalateWithRuleID(t *testing.T) {
	// TC-G-18: 1 entry: escalate + RuleID="r1" → CoverageGapCount=0
	entries := []AnalyticsEntry{
		{Decision: "escalate", RuleID: "r1", ToolName: "Bash"},
	}

	s := ComputeSummary(entries)
	assert.Equal(t, 0, s.CoverageGapCount, "TC-G-18: escalate with RuleID must not count as a coverage gap")
}

func TestComputeSummary_should_computeCorrectRates(t *testing.T) {
	// TC-G-19: 10 entries: 8 auto_allow, 1 auto_deny, 1 escalate.
	entries := make([]AnalyticsEntry, 0, 10)
	for i := 0; i < 8; i++ {
		entries = append(entries, AnalyticsEntry{Decision: "auto_allow", RuleID: "r1", ToolName: "Bash"})
	}
	entries = append(entries, AnalyticsEntry{Decision: "auto_deny", RuleID: "r2", ToolName: "Bash"})
	entries = append(entries, AnalyticsEntry{Decision: "escalate", RuleID: "", ToolName: "Bash"})

	s := ComputeSummary(entries)
	assert.Equal(t, 10, s.TotalDecisions, "TC-G-19: TotalDecisions must be 10")
	assert.InDelta(t, 0.8, s.AutoApproveRate, 0.001, "TC-G-19: AutoApproveRate must be 0.8")
	assert.InDelta(t, 0.1, s.ManualReviewRate, 0.001, "TC-G-19: ManualReviewRate must be 0.1 (escalate/total)")
}

func TestComputeSummary_should_returnZeroSummary_When_empty(t *testing.T) {
	// TC-G-20: Empty entry slice → all counts zero, rates zero.
	s := ComputeSummary(nil)
	assert.Equal(t, 0, s.TotalDecisions, "TC-G-20: TotalDecisions must be 0")
	assert.Equal(t, 0, s.CoverageGapCount, "TC-G-20: CoverageGapCount must be 0")
	assert.Equal(t, 0.0, s.AutoApproveRate, "TC-G-20: AutoApproveRate must be 0.0")
	assert.Equal(t, 0.0, s.ManualReviewRate, "TC-G-20: ManualReviewRate must be 0.0")
}

func TestComputeSummary_should_showFewerGaps_After_ReclassifyGaps(t *testing.T) {
	// TC-G-21: 3 escalate+no-ruleID entries; classifier covers 2 of them.
	// After ReclassifyGaps, ComputeSummary should show CoverageGapCount=1.
	c := classifier.NewRuleBasedClassifier()
	rules := []classifier.Rule{
		{
			ID:             "rule-git-push",
			Name:           "Allow git push",
			ToolName:       "Bash",
			CommandPattern: mustCompileRe(t, "^git push"),
			Decision:       classifier.AutoAllow,
			Enabled:        true,
			Priority:       100,
			Source:         "user",
		},
	}
	c.ReplaceRules(rules)

	entries := []AnalyticsEntry{
		{Decision: "escalate", RuleID: "", ToolName: "Bash", CommandPreview: "git push origin main"},
		{Decision: "escalate", RuleID: "", ToolName: "Bash", CommandPreview: "git push --force"},
		{Decision: "escalate", RuleID: "", ToolName: "Bash", CommandPreview: "rm -rf /tmp/foo"},
	}

	reclassified := ReclassifyGaps(entries, c)
	s := ComputeSummary(reclassified)
	assert.Equal(t, 1, s.CoverageGapCount, "TC-G-21: 2 of 3 gaps covered by rule; 1 should remain")
}
