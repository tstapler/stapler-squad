package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGestureForwardStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy *GestureForwardStrategy
		wantName string
	}{
		{
			name:     "claude gesture forward",
			strategy: NewClaudeGestureForwardStrategy(),
			wantName: "claude-gesture-forward",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.strategy.Name())
			assert.False(t, tt.strategy.CanHandle("claude"), "CanHandle always returns false: strategies are selected by their owning adapter")

			assert.Equal(t, [][]byte{pageUpBytes}, tt.strategy.KeySequences(ScrollUp))
			assert.Equal(t, [][]byte{pageUpBytes}, tt.strategy.KeySequences(ScrollDown))
			assert.Equal(t, RedrawQuiescenceCapture, tt.strategy.CaptureVia())

			cap := tt.strategy.Capability()
			assert.Equal(t, [][]byte{pageUpBytes}, cap.KeySequencesUp)
			assert.Equal(t, "2.1.270", cap.VerifiedAgainstVersion)
			assert.NotEmpty(t, cap.KnownFailureModes)
		})
	}
}
