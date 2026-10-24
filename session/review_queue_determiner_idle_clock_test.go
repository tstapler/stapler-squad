package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session/detection"
)

// idleClockDeterminer disables staleness so only the idle/stuck clocks decide the outcome.
func idleClockDeterminer() *DefaultStatusDeterminer {
	cfg := DefaultReviewQueuePollerConfig()
	cfg.StalenessThreshold = 24 * time.Hour
	return NewDefaultStatusDeterminer(cfg)
}

func TestDetermine_Should_ReturnIdleReason_When_IdleClockOutputOnAndOutputOlderThanThreshold(t *testing.T) {
	t.Parallel()
	determiner := idleClockDeterminer()
	detector := detection.NewStatusDetector()

	cases := []struct {
		name       string
		opts       DeterminerOptions
		wantAction DetectionAction
		wantReason AttentionReason
	}{
		{"flag on idles on old output", DeterminerOptions{IdleClockOutput: true}, DetectionActionAdd, ReasonIdle},
		{"flag off keeps UpdatedAt clock", DeterminerOptions{}, DetectionActionSkip, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inst := startedInstance("idle-clock", withCreatedAgo(time.Hour), withAges(2*time.Second, 10*time.Minute))
			got := determiner.Determine(inst, "", InstanceStatusInfo{}, detector, tc.opts)
			assert.Equal(t, tc.wantAction, got.Action)
			assert.Equal(t, tc.wantReason, got.Reason)
		})
	}
}

func TestDetermine_Should_ApplyStuckThresholdToOutputAge_When_IdleClockOutputOnAndWaitingForAgent(t *testing.T) {
	t.Parallel()
	determiner := idleClockDeterminer()
	detector := detection.NewStatusDetector()
	info := InstanceStatusInfo{
		IsControllerActive: true,
		ClaudeStatus:       detection.StatusWaitingForAgent,
		IdleState:          detection.IdleStateInfo{State: detection.IdleStateTimeout},
	}

	cases := []struct {
		name       string
		opts       DeterminerOptions
		wantAction DetectionAction
	}{
		{"flag on: 31 min old output is stuck, item added", DeterminerOptions{IdleClockOutput: true}, DetectionActionAdd},
		{"flag off: 1 min old UpdatedAt still counts as active", DeterminerOptions{}, DetectionActionRemove},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			inst := startedInstance("idle-clock", withCreatedAgo(time.Hour), withAges(time.Minute, 31*time.Minute))
			got := determiner.Determine(inst, "", info, detector, tc.opts)
			assert.Equal(t, tc.wantAction, got.Action)
		})
	}
}
