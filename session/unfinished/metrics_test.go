package unfinished

import "testing"

// TestRegisterMetrics_NoError is a smoke check that instrument registration
// against the OTel global (no-op, in tests) meter succeeds — the one thing
// that would break silently if an instrument name were invalid or an
// observable gauge were registered with a description/unit OTel rejects.
func TestRegisterMetrics_NoError(t *testing.T) {
	t.Parallel()
	if err := RegisterMetrics(); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
}
