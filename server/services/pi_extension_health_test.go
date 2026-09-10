package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/events"
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

// ── HandlePiExtensionLoaded HTTP handler tests ──────────────────────────────

func TestHandlePiExtensionLoaded_RecordsPing_ViaSessionIDHeader(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
	tracker := NewPiExtensionHealthTracker()
	h.SetPiExtensionHealthTracker(tracker)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/pi-extension-loaded", strings.NewReader(`{}`))
	req.Header.Set("X-CS-Session-ID", "sess-1")
	rec := httptest.NewRecorder()

	h.HandlePiExtensionLoaded(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, PiExtensionHealthLoaded, tracker.HealthFor("sess-1"))
}

func TestHandlePiExtensionLoaded_NoTracker_StillReturns200(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
	// Deliberately never call SetPiExtensionHealthTracker.

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/pi-extension-loaded", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() { h.HandlePiExtensionLoaded(rec, req) })
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlePiExtensionLoaded_RejectsNonPost(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
	h.SetPiExtensionHealthTracker(NewPiExtensionHealthTracker())

	req := httptest.NewRequest(http.MethodGet, "/api/hooks/pi-extension-loaded", nil)
	rec := httptest.NewRecorder()

	h.HandlePiExtensionLoaded(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandlePiExtensionLoaded_MalformedBody_StillReturns200(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
	h.SetPiExtensionHealthTracker(NewPiExtensionHealthTracker())

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/pi-extension-loaded", strings.NewReader(`not json`))
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() { h.HandlePiExtensionLoaded(rec, req) })
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestHandlePiExtensionLoaded_ShouldNotRecord_WhenPiSupportFlagIsOff covers the
// spec-compliance gap this fix closes: with pi-support off, the handler must
// not write into the tracker's in-memory map for any POST it receives (every
// other pi surface already checks the flag before acting — see plan.md's Risk
// Control section). The isolated STAPLER_SQUAD_TEST_DIR config starts with all
// flags off by default, so this test deliberately does not call SetFeatureFlag.
func TestHandlePiExtensionLoaded_ShouldNotRecord_WhenPiSupportFlagIsOff(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.False(t, config.LoadConfig().GetFeatureFlag(config.FeaturePiSupport))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
	tracker := NewPiExtensionHealthTracker()
	h.SetPiExtensionHealthTracker(tracker)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/pi-extension-loaded", strings.NewReader(`{}`))
	req.Header.Set("X-CS-Session-ID", "sess-1")
	rec := httptest.NewRecorder()

	h.HandlePiExtensionLoaded(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, PiExtensionHealthUnknown, tracker.HealthFor("sess-1"))
}
