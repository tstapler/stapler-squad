package gatestats

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/fsops"
	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
)

// removeFailFS refuses every Remove (an undeletable stale temp file).
type removeFailFS struct{ fsops.FSOps }

func (removeFailFS) Remove(string) error { return errDiskFull }

func TestNewStatsWriter_ShouldDefaultClockTimerAndLogger_WhenNilArgumentsGiven(t *testing.T) {
	t.Parallel()
	clk := newGateTestClock()
	store, _ := newTestStore(t, memfs.New(), clk, 1)
	w := NewStatsWriter(deliverygate.NewStats(clk.Now), store, nil, nil, nil)
	require.NotNil(t, w.now)
	require.NotNil(t, w.after)
	require.NotNil(t, w.logger)
	assert.WithinDuration(t, time.Now(), w.now(), time.Minute)
}

func TestStatsWriter_ShouldStopAndClearRunning_WhenTickChannelCloses(t *testing.T) {
	t.Parallel()
	mfs, clk := memfs.New(), newGateTestClock()
	w, stats, _ := newWriterEnv(t, 1, mfs, clk)
	tick := make(chan time.Time)
	var wg sync.WaitGroup
	w.Start(context.Background(), tick, &wg)
	require.True(t, stats.WriterRunning())
	close(tick)
	wg.Wait()
	assert.False(t, stats.WriterRunning())
}

func TestStatsStore_ShouldKeepGoingAndWarn_WhenStaleTempCannotBeRemoved(t *testing.T) {
	t.Parallel()
	mfs, clk := memfs.New(), newGateTestClock()
	mfs.Put(testStatsDir+"/"+statsFileName+".9.tmp", []byte("stale"))
	store, warns := newTestStore(t, mfs, clk, 1)
	store.fs = removeFailFS{FSOps: store.fs}
	_, ok := store.Load()
	assert.False(t, ok)
	assert.Equal(t, 1, warns("delivery_gate_stats_stale_temp_undeletable"))
}

func TestStatsStore_ShouldReportNoForeignWriter_WhenFileIsUnreadableOrNotJSON(t *testing.T) {
	t.Parallel()
	mfs, clk := memfs.New(), newGateTestClock()
	store, _ := newTestStore(t, mfs, clk, 1)
	assert.False(t, store.ForeignWriterActive(), "no file")
	mfs.Put(testStatsDir+"/"+statsFileName, []byte("{nope"))
	assert.False(t, store.ForeignWriterActive(), "invalid JSON")
	mfs.Put(testStatsDir+"/"+statsFileName, []byte(`{"writer_pid":0}`))
	assert.False(t, store.ForeignWriterActive(), "pid 0")
}
