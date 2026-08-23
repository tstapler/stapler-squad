package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/tymux"
)

// fakeTymuxManagerForBackendRestarted is a minimal tymux.TymuxManager fake
// used only to prove BackendRestarted() is reachable through the interface
// (Phase 5 spec-compliance sweep, Story 2.5.3 gap): it embeds the interface
// (so any unused method panics if called — none should be for this test)
// and overrides only BackendRestarted with a canned value.
type fakeTymuxManagerForBackendRestarted struct {
	tymux.TymuxManager // deliberately nil; unused methods must not be called
	restarted          bool
	since              time.Time
}

func (f *fakeTymuxManagerForBackendRestarted) BackendRestarted() (bool, time.Time) {
	return f.restarted, f.since
}

// TestTymuxBackend_BackendRestarted_ObservableThroughTymuxManagerInterface
// closes the Phase 5 sweep's Gap 1: Story 2.5.3's acceptance criterion says
// BackendTymux "surfaces a distinct...condition to its caller," but
// BackendRestarted() previously existed only on the unexported concrete
// tymuxGRPCSession type, so no real caller holding a
// ProcessManager/TymuxManager reference could ever observe it — only tests
// doing an unexported-type assertion from inside the package could. This
// test drives BackendRestarted() through a fake TymuxManager (typed as the
// interface, never the concrete type) wrapped by *TymuxBackend, proving a
// caller with only the interface/TymuxBackend surface can observe the
// state.
func TestTymuxBackend_BackendRestarted_ObservableThroughTymuxManagerInterface(t *testing.T) {
	since := time.Now()
	fake := &fakeTymuxManagerForBackendRestarted{restarted: true, since: since}

	// mgr is typed as the interface, not *tymuxGRPCSession — the whole
	// point of the fix.
	var mgr tymux.TymuxManager = fake
	backend := NewTymuxBackend(mgr)

	restarted, gotSince := backend.BackendRestarted()

	assert.True(t, restarted, "BackendRestarted() must be reachable through *TymuxBackend, not just the unexported concrete type")
	assert.Equal(t, since, gotSince)
}
