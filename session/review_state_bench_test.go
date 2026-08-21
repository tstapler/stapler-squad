package session

import (
	"testing"
	"time"
)

// ── Allocation enforcement ────────────────────────────────────────────────────

// TestGetTimeSinceLastMeaningfulOutput_ZeroAllocsHotPath asserts that the atomic
// fast path (ns != 0) allocates nothing. This is the hot path hit on every
// review-queue poll tick for every active session.
func TestGetTimeSinceLastMeaningfulOutput_ZeroAllocsHotPath(t *testing.T) {
	inst := &Instance{
		Title:     "bench-session",
		UUID:      "bench-uuid",
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}
	// Seed the atomic shadow so the hot path (ns != 0) is taken.
	inst.lastMeaningfulOutputNs = time.Now().Add(-1 * time.Second).UnixNano()

	allocs := testing.AllocsPerRun(100, func() {
		_ = inst.GetTimeSinceLastMeaningfulOutput()
	})
	if allocs != 0 {
		t.Errorf("GetTimeSinceLastMeaningfulOutput hot path: got %.0f allocs, want 0 (atomic fast path broken)", allocs)
	}
}

// ── Benchmarks ────────────────────────────────────────────────────────────────

func BenchmarkGetTimeSinceLastMeaningfulOutput_HotPath(b *testing.B) {
	b.ReportAllocs()
	inst := &Instance{
		Title:     "bench-session",
		UUID:      "bench-uuid",
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}
	inst.lastMeaningfulOutputNs = time.Now().Add(-1 * time.Second).UnixNano()

	b.ResetTimer()
	for b.Loop() {
		_ = inst.GetTimeSinceLastMeaningfulOutput()
	}
}

// BenchmarkGetTimeSinceLastMeaningfulOutput_ColdPath benchmarks the fallback
// (locked) path for completeness, so regressions in the lock path are visible too.
func BenchmarkGetTimeSinceLastMeaningfulOutput_ColdPath(b *testing.B) {
	b.ReportAllocs()
	inst := &Instance{
		Title:     "bench-session",
		UUID:      "bench-uuid",
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}
	// Keep atomic at zero to always take the locked fallback path.

	b.ResetTimer()
	for b.Loop() {
		_ = inst.GetTimeSinceLastMeaningfulOutput()
	}
}
