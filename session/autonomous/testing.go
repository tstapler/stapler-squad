package autonomous

import "testing"

// TestSetAutonomousDriverRunID gives an unstarted driver a run id, so a service
// test can tell two registered drivers apart without a headless pool.
func TestSetAutonomousDriverRunID(t *testing.T, d *AutonomousDriver, id uint64) {
	t.Helper()
	d.runID.Store(id)
}
