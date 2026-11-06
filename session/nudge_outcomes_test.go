package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nudTotals(r NudgeOutcomeReport) map[string]int64 {
	m := map[string]int64{}
	for _, c := range r.Totals {
		m[string(c.Kind)+"/"+string(c.Outcome)] = c.Value
	}
	return m
}

func TestNudgeOutcomeRecorder_CountsPerKindAndOutcome(t *testing.T) {
	r := NewNudgeOutcomeRecorder("", nil)
	r.Record(NudgeKindStall, NudgeOutcomeDelivered, "S1")
	r.Record(NudgeKindStall, NudgeOutcomeDelivered, "S2")
	r.Record(NudgeKindStall, NudgeOutcomeSkippedCap, "S1")
	r.Record(NudgeKindDiagnose, NudgeOutcomeUncertain, "S3")

	assert.Equal(t, map[string]int64{
		"stall/delivered": 2, "stall/skipped_cap": 1, "diagnose/uncertain": 1,
	}, nudTotals(r.Snapshot()))
}

func TestNudgeOutcomeRecorder_PersistsAcrossRestartAndKeepsFirstSince(t *testing.T) {
	dir := t.TempDir()
	first := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	clock := first
	r := NewNudgeOutcomeRecorder(dir, func() time.Time { return clock })
	r.Record(NudgeKindStall, NudgeOutcomeDelivered, "S1")

	clock = first.Add(48 * time.Hour)
	restarted := NewNudgeOutcomeRecorder(dir, func() time.Time { return clock })
	restarted.Record(NudgeKindStall, NudgeOutcomeDelivered, "S1")

	rep := restarted.Snapshot()
	assert.Equal(t, int64(2), nudTotals(rep)["stall/delivered"])
	require.NotNil(t, rep.Since)
	assert.True(t, rep.Since.Equal(first), "since is the first-ever record, not the restart: %v", rep.Since)
}

func TestNudgeOutcomeRecorder_EmptyBeforeAnyRecord(t *testing.T) {
	rep := NewNudgeOutcomeRecorder(t.TempDir(), nil).Snapshot()
	assert.Nil(t, rep.Since)
	assert.Empty(t, rep.Totals)
}
