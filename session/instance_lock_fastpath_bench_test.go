package session

import "testing"

// BenchmarkStopController_NoOpFastPath benchmarks the common case where no
// controller is registered. Enforcement for the mutex-profile fix: the
// lock-free HasController() check must stay ahead of i.mu.Lock() — a
// regression back to locking unconditionally shows up here as increased
// ns/op even single-threaded, since Lock/Unlock costs more than an atomic
// load even uncontended.
func BenchmarkStopController_NoOpFastPath(b *testing.B) {
	b.ReportAllocs()
	inst := &Instance{Title: "bench-stop-controller"}
	inst.SetStatusManager(NewInstanceStatusManager())

	b.ResetTimer()
	for b.Loop() {
		inst.StopController()
	}
}

// BenchmarkSetHistoryInfo_NoOpFastPath benchmarks HistoryLinker's dominant
// call pattern — repeated calls with an unchanged UUID/path. Enforcement:
// the claudeSessionMu.RLock() pre-check must stay ahead of the exclusive
// Lock() for this case.
func BenchmarkSetHistoryInfo_NoOpFastPath(b *testing.B) {
	b.ReportAllocs()
	inst := makeTestInstance("bench-history-info")
	inst.SetHistoryInfo("conv-uuid-1", "/path/to/history.jsonl")

	b.ResetTimer()
	for b.Loop() {
		inst.SetHistoryInfo("conv-uuid-1", "/path/to/history.jsonl")
	}
}
