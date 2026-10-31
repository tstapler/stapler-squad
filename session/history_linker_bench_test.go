package session

import (
	"github.com/stretchr/testify/assert"
	"sync/atomic"
	"testing"
	"time"
)

// slowPanePM models the tmux exec behind GetPanePID: it blocks for delay
// (a timer wait, like a subprocess wait) and then reports no live pane so the
// path fallback runs.
type slowPanePM struct {
	ProcessManager
	delay time.Duration
	calls *atomic.Int64
}

func (p slowPanePM) GetPanePID() (int32, error) {
	p.calls.Add(1)
	<-time.After(p.delay)
	return 0, assert.AnError
}

// BenchmarkHistoryLinker_CorrelateParallel measures concurrent correlation of
// distinct linked sessions with a 200us pane-PID lookup, the shape of the
// fsnotify ScanAll plus poll loop contention seen in the live mutex profile.
// With a global correlate lock ns/op is ~the lookup latency regardless of
// GOMAXPROCS; without it ns/op falls as cores are added.
func BenchmarkHistoryLinker_CorrelateParallel(b *testing.B) {
	const n = 64
	var calls atomic.Int64
	linker := NewHistoryLinker(NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, b.TempDir()), nil)
	insts := make([]*Instance, 0, n)
	for i := 0; i < n; i++ {
		title := "bench-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		inst := &Instance{Title: title, Path: b.TempDir(), Status: Running}
		inst.processManager = slowPanePM{delay: 200 * time.Microsecond, calls: &calls}
		inst.SetHistoryInfo("uuid-"+title, "/projects/p/uuid-"+title+".jsonl")
		insts = append(insts, inst)
	}
	linker.SetInstances(insts)

	var next atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			linker.correlateSession(insts[next.Add(1)%n], true)
		}
	})
}
