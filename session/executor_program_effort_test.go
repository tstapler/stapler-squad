package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// agy/gemini/proxy-claude programs must never receive --effort.
func TestResolveExecutorProgramWithEffort_should_OnlyAppendEffort_When_ClaudeProgram(t *testing.T) {
	t.Parallel()
	families := map[string]string{"sonnet": "claude-sonnet-4-6"}
	cases := []struct{ program, model, effort, want string }{
		{"", "family:sonnet", "high", "claude --model claude-sonnet-4-6 --effort high"},
		{"claude", "family:sonnet", "", "claude --model claude-sonnet-4-6"},
		{"agy", "family:sonnet", "high", "agy"},
		{"gemini", "family:sonnet", "high", "gemini"},
		{"proxy-claude", "family:sonnet", "high", "proxy-claude"},
		{"", "", "high", ""},
	}
	for _, tc := range cases {
		got, err := ResolveExecutorProgramWithEffort(tc.program, tc.model, tc.effort, families)
		assert.NoError(t, err)
		assert.Equal(t, tc.want, got, "program=%q effort=%q", tc.program, tc.effort)
	}
}
