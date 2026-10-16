package session

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Polling for a review verdict re-reads the whole context on every wake; the
// app steers the session instead (BacklogService.StartVerdictSteering).
func TestPostReviewPrompts_DoNotRecommendPolling(t *testing.T) {
	t.Parallel()
	item := &BacklogItemData{ID: uuid.New().String(), Title: "T"}
	files, err := buildDefaultSlashCommandSet(item)
	require.NoError(t, err)

	prompts := map[string]string{
		"review.md":                files["review.md"],
		"sddInitialPromptTemplate": sddInitialPromptTemplate,
	}
	for name, text := range prompts {
		assert.NotContains(t, text, "wait_for_backlog_event", name)
		assert.Contains(t, text, "do NOT use ScheduleWakeup", name)
		assert.Contains(t, text, "end your turn and stay idle", name)
	}
}
