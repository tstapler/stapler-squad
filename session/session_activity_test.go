package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSessionActivity_Should_AdvanceWithEventLogFlagOff_When_OutputArrives(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "activity", Status: Running}
	at := time.Date(2026, 10, 24, 14, 2, 11, 0, time.UTC)

	inst.Touch(SessionEventOutput, at)
	assert.Equal(t, at, inst.Snapshot().LastMeaningfulChangeAt)

	inst.Touch(SessionEventOutput, at.Add(-time.Minute))
	assert.Equal(t, at, inst.Snapshot().LastMeaningfulChangeAt, "never moves backwards")
}
