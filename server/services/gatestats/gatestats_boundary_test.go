package gatestats

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
)

const statsFilePath = testStatsDir + "/" + statsFileName

// The foreign-writer window is half-open: a write exactly foreignWriterWindow old no longer blocks.
func TestStatsStore_ShouldTreatForeignWriteAsActiveOnlyBeforeTheWindowEnds_WhenCheckedAtTheBoundary(t *testing.T) {
	t.Parallel()
	mfs, clk := memfs.New(), newGateTestClock()
	owner, _ := newTestStore(t, mfs, clk, 100)
	require.NoError(t, owner.Save(sampleStats(clk)))
	other, _ := newTestStore(t, mfs, clk, 200)

	clk.Advance(foreignWriterWindow - time.Nanosecond)
	assert.True(t, other.ForeignWriterActive(), "one nanosecond inside the window")
	clk.Advance(time.Nanosecond)
	assert.False(t, other.ForeignWriterActive(), "exactly at the window")
	clk.Advance(time.Nanosecond)
	assert.False(t, other.ForeignWriterActive(), "past the window")
}

func TestStatsStore_ShouldNotBlockOnWriterPIDZero_WhenFileIsFresh(t *testing.T) {
	t.Parallel()
	mfs, clk := memfs.New(), newGateTestClock()
	store, _ := newTestStore(t, mfs, clk, 200) // liveness fake always reports alive
	mfs.Put(statsFilePath, []byte(`{"writer_pid":0,"written_at":"`+clk.Now().UTC().Format(time.RFC3339)+`"}`))
	assert.False(t, store.ForeignWriterActive())
}

// Behaviour recorded from the pre-move code: the cap is `size > 1 MiB`, so a file of exactly
// 1 MiB still loads and only one byte more is quarantined before it is read.
func TestStatsStore_ShouldQuarantineOnlyFilesLargerThanTheCap_WhenLoadingAtTheSizeBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		size        int
		wantLoaded  bool
		wantCorrupt int
	}{
		{"cap-1", statsFileMaxBytes - 1, true, 0},
		{"cap", statsFileMaxBytes, true, 0},
		{"cap+1", statsFileMaxBytes + 1, false, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			mfs, clk := memfs.New(), newGateTestClock()
			seed, _ := newTestStore(t, mfs, clk, 1)
			require.NoError(t, seed.Save(sampleStats(clk)))
			valid, _ := mfs.Get(statsFilePath)
			padded := []byte(string(valid) + strings.Repeat(" ", c.size-len(valid)))
			require.Len(t, padded, c.size)
			mfs.Put(statsFilePath, padded)

			store, warns := newTestStore(t, mfs, clk, 2)
			_, ok := store.Load()
			assert.Equal(t, c.wantLoaded, ok)
			assert.Equal(t, c.wantCorrupt, warns("delivery_gate_stats_corrupt"))
			assert.Equal(t, !c.wantLoaded, store.Status().Quarantined)
		})
	}
}

// The rate limit is half-open: a repeat exactly statsWarnEvery later logs again.
func TestStatsStore_ShouldRepeatAWarnExactlyAfterTheWarnInterval_WhenFailuresPersist(t *testing.T) {
	t.Parallel()
	mfs, clk := memfs.New(), newGateTestClock()
	mfs.FailRename = errDiskFull
	store, warns := newTestStore(t, mfs, clk, 1)
	const msg = "delivery_gate_stats_write_failed"

	_ = store.Save(sampleStats(clk))
	assert.Equal(t, 1, warns(msg))
	clk.Advance(statsWarnEvery - time.Nanosecond)
	_ = store.Save(sampleStats(clk))
	assert.Equal(t, 1, warns(msg), "suppressed just inside the interval")
	clk.Advance(time.Nanosecond)
	_ = store.Save(sampleStats(clk))
	assert.Equal(t, 2, warns(msg), "logged again exactly at the interval")
}

// partialWriteFS leaves the temp file behind and then fails, like a disk that fills mid-write.
type partialWriteFS struct{ *memfs.MemFS }

func (p partialWriteFS) WriteFileSync(path string, data []byte, _ os.FileMode) error {
	p.Put(path, data[:len(data)/2])
	return errDiskFull
}

func TestStatsStore_ShouldNotLeaveTempFiles_WhenTheWriteOrTheRenameFails(t *testing.T) {
	t.Parallel()
	t.Run("write", func(t *testing.T) {
		t.Parallel()
		mfs, clk := memfs.New(), newGateTestClock()
		store, _ := newTestStore(t, mfs, clk, 1)
		store.fs = partialWriteFS{mfs}
		require.ErrorIs(t, store.Save(sampleStats(clk)), errDiskFull)
		assert.Empty(t, mfs.Names(testStatsDir), "the half-written temp is removed")
		assert.False(t, store.Status().Writable)
	})
	t.Run("rename", func(t *testing.T) {
		t.Parallel()
		mfs, clk := memfs.New(), newGateTestClock()
		mfs.FailRename = errDiskFull
		store, _ := newTestStore(t, mfs, clk, 1)
		require.ErrorIs(t, store.Save(sampleStats(clk)), errDiskFull)
		assert.Empty(t, mfs.Names(testStatsDir), "the renamed-away temp is removed")
	})
}
