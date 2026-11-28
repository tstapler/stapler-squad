package testrepo

import "runtime"

// GateTB is the slice of testing.TB the G5 gate needs, so a negative twin can
// run the gate against a recorder.
type GateTB interface {
	Helper()
	Errorf(format string, args ...any)
}

// FailRecorder is the GateTB a negative twin runs a gate against.
type FailRecorder struct{ Failures int }

func (*FailRecorder) Helper()                 {}
func (f *FailRecorder) Errorf(string, ...any) { f.Failures++ }

// AllocDelta reports per-call TotalAlloc and Mallocs over n calls after one
// warm-up call; single goroutine, so concurrent allocators do not leak in.
func AllocDelta(n int, f func()) (bytesPerOp, allocsPerOp float64) {
	f()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range n {
		f()
	}
	runtime.ReadMemStats(&after)
	return float64(after.TotalAlloc-before.TotalAlloc) / float64(n), float64(after.Mallocs-before.Mallocs) / float64(n)
}
