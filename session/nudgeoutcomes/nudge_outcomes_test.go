package nudgeoutcomes

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
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

func TestNudgeOutcomeRecorder_SnapshotSortedByKindThenOutcome(t *testing.T) {
	t.Parallel()
	r := NewNudgeOutcomeRecorder("", nil)
	r.Record(NudgeKindStall, NudgeOutcomeNotIdle, "S1")
	r.Record(NudgeKindStall, NudgeOutcomeDelivered, "S1")
	r.Record(NudgeKindDiagnose, NudgeOutcomeUncertain, "S1")

	totals := r.Snapshot().Totals
	got := make([]string, 0, len(totals))
	for _, c := range totals {
		got = append(got, string(c.Kind)+"/"+string(c.Outcome))
	}
	assert.Equal(t, []string{"diagnose/uncertain", "stall/delivered", "stall/not_idle"}, got)
}

func TestNudgeOutcomeRecorder_StartsEmptyWhenLedgerCorrupt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, nudgeOutcomesFile), []byte("{not json"), 0o600))

	r := NewNudgeOutcomeRecorder(dir, nil)
	assert.Empty(t, r.Snapshot().Totals)
	r.Record(NudgeKindDiagnose, NudgeOutcomeFailed, "S1")
	assert.Equal(t, int64(1), nudTotals(r.Snapshot())["diagnose/failed"])
}

func TestNudgeOutcomeRecorder_StartsEmptyWhenLedgerUnreadable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, nudgeOutcomesFile), 0o700)) // ReadFile fails, not ErrNotExist

	assert.Empty(t, NewNudgeOutcomeRecorder(dir, nil).Snapshot().Totals)
}

func TestNudgeOutcomeRecorder_KeepsCountInMemoryWhenWriteFails(t *testing.T) {
	t.Parallel()
	r := NewNudgeOutcomeRecorder(filepath.Join(t.TempDir(), "missing"), nil)
	r.Record(NudgeKindStall, NudgeOutcomeUncertain, "S1")
	assert.Equal(t, int64(1), nudTotals(r.Snapshot())["stall/uncertain"])
}

func TestDefaultNudgeOutcomes_SwapRecordAndRestore(t *testing.T) {
	orig := DefaultNudgeOutcomes()
	rec := NewNudgeOutcomeRecorder("", nil)
	restore := SwapNudgeOutcomeRecorder(rec)

	assert.Same(t, rec, DefaultNudgeOutcomes())
	RecordNudgeOutcome(NudgeKindStall, NudgeOutcomeLeaseBusy, "S1")
	assert.Equal(t, int64(1), nudTotals(rec.Snapshot())["stall/lease_busy"])

	restore()
	assert.Same(t, orig, DefaultNudgeOutcomes())
}

func TestInitNudgeOutcomes_InstallsDurableRecorderInConfigDir(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	defer SwapNudgeOutcomeRecorder(DefaultNudgeOutcomes())()

	InitNudgeOutcomes()
	RecordNudgeOutcome(NudgeKindStall, NudgeOutcomeDelivered, "S1")

	dir, err := config.GetConfigDir()
	require.NoError(t, err)
	reloaded := NewNudgeOutcomeRecorder(dir, nil)
	assert.Equal(t, int64(1), nudTotals(reloaded.Snapshot())["stall/delivered"])
}
