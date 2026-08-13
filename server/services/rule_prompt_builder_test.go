package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// T-UNIT-GO-005
func TestDefaultRulePromptBuilder_BuildSystemPrompt_ContainsSchemaAndSeeds(t *testing.T) {
	builder := &DefaultRulePromptBuilder{}
	ctx := RulePromptContext{
		ExistingRules: []RuleSpec{},
		WindowDays:    7,
	}

	prompt := builder.BuildSystemPrompt(ctx)

	assert.Contains(t, prompt, "JSON schema", "system prompt must contain JSON schema block")
	assert.True(t, strings.Contains(strings.ToLower(prompt), "existing rules"),
		"system prompt must reference existing rules")
	assert.True(t, strings.Contains(strings.ToLower(prompt), "priority tiers"),
		"system prompt must contain priority tier legend")
}

// T-UNIT-GO-006
func TestDefaultRulePromptBuilder_BuildUserPrompt_FormatsGap(t *testing.T) {
	builder := &DefaultRulePromptBuilder{}
	ctx := RulePromptContext{
		AnalyticsGaps: []AnalyticsGap{
			{ToolName: "Bash", Program: "git", Count: 42},
		},
		WindowDays: 7,
	}

	prompt := builder.BuildUserPrompt(ctx)

	assert.Contains(t, prompt, "Bash", "user prompt must include tool name from gap")
	assert.Contains(t, prompt, "42", "user prompt must include count from gap")
}

// T-UNIT-GO-009
func TestBuildUserPrompt_RedactsSecretCommandPreviews(t *testing.T) {
	builder := &DefaultRulePromptBuilder{}
	ctx := RulePromptContext{
		AnalyticsGaps: []AnalyticsGap{
			{
				ToolName:           "Bash",
				Program:            "curl",
				Count:              3,
				RepresentativeCmds: []string{"curl -H 'Auth: ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH1234' https://api.example.com"},
			},
		},
		WindowDays: 7,
	}

	prompt := builder.BuildUserPrompt(ctx)

	assert.Contains(t, prompt, redactedPrompt, "user prompt must contain redactedPrompt for secret command")
	assert.NotContains(t, prompt, "ghp_", "user prompt must not contain raw GitHub token")
}

func TestDefaultRulePromptBuilder_BuildUserPrompt_CommandSample(t *testing.T) {
	builder := &DefaultRulePromptBuilder{}
	ctx := RulePromptContext{
		CommandSample: "git push origin main",
		WindowDays:    7,
	}

	prompt := builder.BuildUserPrompt(ctx)

	assert.Contains(t, prompt, "git push origin main", "user prompt must include command sample")
	assert.Contains(t, prompt, "exactly 1 rule", "user prompt must request exactly 1 rule for command sample")
}

func TestDefaultRulePromptBuilder_BuildUserPrompt_RedactsSecretCommandSample(t *testing.T) {
	builder := &DefaultRulePromptBuilder{}
	ctx := RulePromptContext{
		CommandSample: "curl -H 'Authorization: Bearer ghp_AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHH1234'",
		WindowDays:    7,
	}

	prompt := builder.BuildUserPrompt(ctx)

	assert.Contains(t, prompt, redactedPrompt, "user prompt must contain redactedPrompt for secret command sample")
	assert.NotContains(t, prompt, "ghp_", "user prompt must not contain raw GitHub token in command sample")
}

func TestDefaultRulePromptBuilder_BuildUserPrompt_EmptyGaps(t *testing.T) {
	builder := &DefaultRulePromptBuilder{}
	ctx := RulePromptContext{
		AnalyticsGaps: []AnalyticsGap{},
		WindowDays:    7,
	}

	prompt := builder.BuildUserPrompt(ctx)

	assert.True(t, strings.Contains(prompt, "no uncovered") || strings.Contains(prompt, "empty JSON array") || strings.Contains(prompt, "[]"),
		"user prompt for empty gaps should indicate no gaps: %q", prompt)
}
