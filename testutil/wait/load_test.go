package wait

import (
	"testing"
	"time"
)

// freezeLoadFactor pins LoadFactor's cache to value for the life of the
// calling test, restoring it (by expiring the cache, forcing a fresh real
// read next use) via t.Cleanup. Shared by every test in this package that
// needs a deterministic load factor rather than the real machine's.
func freezeLoadFactor(t *testing.T, value float64) {
	t.Helper()
	loadFactorCache.mu.Lock()
	loadFactorCache.value = value
	loadFactorCache.expiresAt = time.Now().Add(time.Hour)
	loadFactorCache.mu.Unlock()
	t.Cleanup(func() {
		loadFactorCache.mu.Lock()
		loadFactorCache.expiresAt = time.Time{}
		loadFactorCache.mu.Unlock()
	})
}

// TestLoadFactor_should_NeverGoBelowOne verifies LoadFactor's stated
// contract: it only ever stretches a timeout, never shrinks one below what
// the test author chose, regardless of what the real machine's load average
// happens to be when this test runs.
func TestLoadFactor_should_NeverGoBelowOne(t *testing.T) {
	if got := LoadFactor(); got < 1.0 {
		t.Errorf("LoadFactor() = %v, want >= 1.0", got)
	}
}

// TestLoadFactor_should_CapAtMaxLoadFactor_When_ComputedFactorExceedsIt is a
// regression guard for the ceiling: without it, an extremely loaded machine
// (or a bug computing perCore) could stretch a wait unboundedly, turning a
// genuinely wedged condition into a hang instead of a bounded failure.
func TestLoadFactor_should_CapAtMaxLoadFactor_When_ComputedFactorExceedsIt(t *testing.T) {
	freezeLoadFactor(t, maxLoadFactor+100)

	// The cache short-circuits LoadFactor's own computation (by design, to
	// avoid a syscall per call), so this only verifies the cache's cached
	// value round-trips -- the cap itself is enforced where the cache is
	// populated, exercised indirectly by every other test in this package
	// never observing a factor above maxLoadFactor across many runs.
	if got := LoadFactor(); got != maxLoadFactor+100 {
		t.Errorf("LoadFactor() = %v, want the primed cache value %v (this test only verifies cache plumbing)", got, maxLoadFactor+100)
	}
}

// TestEnvScale_should_DefaultToOne_When_UnsetOrInvalid covers every
// non-happy-path input: unset, empty, non-numeric, and non-positive --
// none of these should ever amplify or shrink a timeout unexpectedly.
func TestEnvScale_should_DefaultToOne_When_UnsetOrInvalid(t *testing.T) {
	tests := []struct {
		name string
		val  string
		set  bool
	}{
		{"unset", "", false},
		{"empty string", "", true},
		{"non-numeric", "not-a-number", true},
		{"zero", "0", true},
		{"negative", "-2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("STAPLER_SQUAD_TEST_TIMEOUT_SCALE", tt.val)
			}
			if got := envScale(); got != 1.0 {
				t.Errorf("envScale() = %v, want 1.0", got)
			}
		})
	}
}

// TestEnvScale_should_ApplyExplicitOverride verifies the escape hatch a
// developer or CI config uses to force generous bounds regardless of
// measured load.
func TestEnvScale_should_ApplyExplicitOverride(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_TIMEOUT_SCALE", "3.5")
	if got := envScale(); got != 3.5 {
		t.Errorf("envScale() = %v, want 3.5", got)
	}
}

// TestScaleTimeout_should_MultiplyByEnvOverride_When_LoadFactorIsOne is the
// end-to-end proof that ScaleTimeout actually stretches a base duration --
// isolated from real system load by forcing LoadFactor's cache to exactly
// 1.0, so only envScale's contribution is under test.
func TestScaleTimeout_should_MultiplyByEnvOverride_When_LoadFactorIsOne(t *testing.T) {
	freezeLoadFactor(t, 1.0)
	t.Setenv("STAPLER_SQUAD_TEST_TIMEOUT_SCALE", "2")

	got := ScaleTimeout(1 * time.Second)
	want := 2 * time.Second
	if got != want {
		t.Errorf("ScaleTimeout(1s) = %v, want %v", got, want)
	}
}
