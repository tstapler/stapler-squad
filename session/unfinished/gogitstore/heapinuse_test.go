package gogitstore

import (
	"runtime"
	"testing"
)

// The default readHeapInUse must track MemStats.HeapInuse (the quantity the
// pressure thresholds were written against) without stopping the world.
func TestReadHeapInUse_ShouldTrackMemStatsHeapInuse_WhenReadWithoutStopTheWorld(t *testing.T) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	got := readHeapInUse()
	if got == 0 {
		t.Fatal("readHeapInUse returned 0")
	}
	lo, hi := m.HeapInuse/2, m.HeapInuse*2
	if got < lo || got > hi {
		t.Errorf("readHeapInUse=%d outside [%d, %d] around MemStats.HeapInuse=%d", got, lo, hi, m.HeapInuse)
	}
}
