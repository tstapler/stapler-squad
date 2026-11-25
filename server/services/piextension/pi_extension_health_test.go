package piextension

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPiExtensionHealthTracker returns a tracker with an injectable clock
// and a caller-chosen grace window, so grace-window transitions can be tested
// without sleeping for real minutes.
func newTestPiExtensionHealthTracker(graceWindow time.Duration) (*PiExtensionHealthTracker, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)}
	tracker := NewPiExtensionHealthTracker()
	tracker.graceWindow = graceWindow
	tracker.now = clock.Now
	return tracker, clock
}

type fakeClock struct {
	t time.Time
}

func (c *fakeClock) Now() time.Time { return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.t = c.t.Add(d)
}

func TestPiExtensionHealthTracker_ShouldReturnUnknown_WhenNoPingHasArrived(t *testing.T) {
	tracker, _ := newTestPiExtensionHealthTracker(time.Minute)
	assert.Equal(t, PiExtensionHealthUnknown, tracker.HealthFor("sess-1"))
}

func TestPiExtensionHealthTracker_ShouldReturnLoaded_AfterPingArrives(t *testing.T) {
	tracker, _ := newTestPiExtensionHealthTracker(time.Minute)
	tracker.RecordPing("sess-1")
	assert.Equal(t, PiExtensionHealthLoaded, tracker.HealthFor("sess-1"))
}

func TestPiExtensionHealthTracker_ShouldReturnFailed_WhenGraceWindowElapsesWithNoPing(t *testing.T) {
	tracker, clock := newTestPiExtensionHealthTracker(time.Minute)
	// First observation starts the grace-window clock.
	assert.Equal(t, PiExtensionHealthUnknown, tracker.HealthFor("sess-1"))
	clock.Advance(2 * time.Minute)
	assert.Equal(t, PiExtensionHealthFailed, tracker.HealthFor("sess-1"))
}

func TestPiExtensionHealthTracker_ShouldReturnFailed_WhenLoadedThenGraceWindowElapsesWithNoRePing(t *testing.T) {
	tracker, clock := newTestPiExtensionHealthTracker(time.Minute)
	tracker.RecordPing("sess-1")
	require.Equal(t, PiExtensionHealthLoaded, tracker.HealthFor("sess-1"))
	clock.Advance(2 * time.Minute)
	assert.Equal(t, PiExtensionHealthFailed, tracker.HealthFor("sess-1"))
}

// TestPiExtensionHealthTracker_ShouldTransitionBackToLoaded_WhenLatePingArrivesAfterFailed
// covers plan.md Task 4.2.1e's decision: a late-but-successful load is still
// real enforcement, so the badge must reflect current truth, not the worst
// historical state.
func TestPiExtensionHealthTracker_ShouldTransitionBackToLoaded_WhenLatePingArrivesAfterFailed(t *testing.T) {
	tracker, clock := newTestPiExtensionHealthTracker(time.Minute)
	assert.Equal(t, PiExtensionHealthUnknown, tracker.HealthFor("sess-1"))
	clock.Advance(2 * time.Minute)
	require.Equal(t, PiExtensionHealthFailed, tracker.HealthFor("sess-1"))

	tracker.RecordPing("sess-1")
	assert.Equal(t, PiExtensionHealthLoaded, tracker.HealthFor("sess-1"))
}

// TestPiExtensionHealthTracker_ShouldRecoverToLoaded_AfterSimulatedServerRestart
// covers Story 4.2.3's AC: a fresh tracker (simulating a server restart wiping
// the in-memory map) that receives one re-ping reports Loaded again, with no
// dependency on the old tracker's state.
func TestPiExtensionHealthTracker_ShouldRecoverToLoaded_AfterSimulatedServerRestart(t *testing.T) {
	before, _ := newTestPiExtensionHealthTracker(piExtensionHealthGraceWindow)
	before.RecordPing("sess-1")
	require.Equal(t, PiExtensionHealthLoaded, before.HealthFor("sess-1"))

	// Discard `before` entirely (simulating a process restart) and construct a
	// brand new tracker with no knowledge of the prior state.
	after, _ := newTestPiExtensionHealthTracker(piExtensionHealthGraceWindow)
	assert.Equal(t, PiExtensionHealthUnknown, after.HealthFor("sess-1"))

	after.RecordPing("sess-1")
	assert.Equal(t, PiExtensionHealthLoaded, after.HealthFor("sess-1"))
}

func TestPiExtensionHealthTracker_ShouldNotAffectUnrelatedSessions(t *testing.T) {
	tracker, clock := newTestPiExtensionHealthTracker(time.Minute)
	tracker.RecordPing("sess-1")
	clock.Advance(2 * time.Minute)
	assert.Equal(t, PiExtensionHealthFailed, tracker.HealthFor("sess-1"))
	// sess-2 was never observed before the clock advanced 2 minutes, so its own
	// grace window starts now and it must read Unknown, not inherit sess-1's Failed.
	assert.Equal(t, PiExtensionHealthUnknown, tracker.HealthFor("sess-2"))
}

// TestPiExtensionHealthTracker_Forget_EvictsTheEntry covers MAJOR 1: without
// Forget, nothing ever removes a session's entry and the tracker's map grows
// unboundedly for the life of the process. After Forget, HealthFor treats the
// session as never-before-observed (Unknown, with a freshly-started grace
// window) rather than remembering its prior Loaded/Failed state.
func TestPiExtensionHealthTracker_Forget_EvictsTheEntry(t *testing.T) {
	tracker, clock := newTestPiExtensionHealthTracker(time.Minute)
	tracker.RecordPing("sess-1")
	require.Equal(t, PiExtensionHealthLoaded, tracker.HealthFor("sess-1"))

	tracker.Forget("sess-1")

	// Re-observing after Forget starts a brand new grace window: advancing by
	// exactly the grace window (relative to the old lastPingAt) would have
	// reported Failed if the old entry had survived, but a re-observed session
	// starts fresh and must read Unknown immediately after Forget.
	assert.Equal(t, PiExtensionHealthUnknown, tracker.HealthFor("sess-1"))
	clock.Advance(2 * time.Minute)
	assert.Equal(t, PiExtensionHealthFailed, tracker.HealthFor("sess-1"))
}

// TestPiExtensionHealthTracker_Forget_EmptySessionID_NoPanic guards the same
// empty-string no-op convention RecordPing/HealthFor already follow.
func TestPiExtensionHealthTracker_Forget_EmptySessionID_NoPanic(t *testing.T) {
	tracker, _ := newTestPiExtensionHealthTracker(time.Minute)
	require.NotPanics(t, func() { tracker.Forget("") })
}

// TestPiExtensionHealthTracker_Forget_UnknownSessionID_NoPanic covers
// forgetting a session that was never observed (e.g. a race between an
// instance being destroyed and its first health ping).
func TestPiExtensionHealthTracker_Forget_UnknownSessionID_NoPanic(t *testing.T) {
	tracker, _ := newTestPiExtensionHealthTracker(time.Minute)
	require.NotPanics(t, func() { tracker.Forget("never-seen") })
}

func TestPiExtensionHealth_String(t *testing.T) {
	assert.Equal(t, "unknown", PiExtensionHealthUnknown.String())
	assert.Equal(t, "loaded", PiExtensionHealthLoaded.String())
	assert.Equal(t, "failed", PiExtensionHealthFailed.String())
}
