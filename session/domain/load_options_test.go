package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadOptionsPresets_should_PinEveryFlag_When_Declared(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		got  LoadOptions
		want LoadOptions
	}{
		{"minimal", LoadMinimal, LoadOptions{}},
		{"summary", LoadSummary, LoadOptions{LoadWorktree: true, LoadDiffStats: true, LoadTags: true, LoadClaudeSession: true}},
		{"full", LoadFull, LoadOptions{LoadWorktree: true, LoadDiffStats: true, LoadDiffContent: true, LoadTags: true, LoadClaudeSession: true}},
		{"diff only", LoadDiffOnly, LoadOptions{LoadWorktree: true, LoadDiffStats: true, LoadDiffContent: true}},
		{"review queue", LoadForReviewQueue, LoadOptions{LoadWorktree: true, LoadDiffStats: true, LoadTags: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

func TestLoadOptions_should_ReturnModifiedCopy_When_WithMethodsCalled(t *testing.T) {
	t.Parallel()
	base := LoadOptions{}

	assert.Equal(t, LoadOptions{LoadDiffContent: true, LoadDiffStats: true}, base.WithDiffContent())
	assert.Equal(t, LoadOptions{LoadTags: true}, base.WithTags())
	assert.False(t, LoadFull.WithoutTags().LoadTags)
	assert.False(t, LoadFull.WithoutDiffContent().LoadDiffContent)
	assert.True(t, LoadFull.WithoutDiffContent().LoadDiffStats, "dropping content keeps stats")
	assert.Equal(t, LoadOptions{}, base, "receiver is a value; the original is untouched")
}
