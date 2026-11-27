package approvals

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/piextension"
)

func TestHandlePiExtensionLoaded_RecordsPing_ViaSessionIDHeader(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(approvalkit.NewApprovalStore(""), nil, bus)
	tracker := piextension.NewPiExtensionHealthTracker()
	h.SetPiExtensionHealthTracker(tracker)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/pi-extension-loaded", strings.NewReader(`{}`))
	req.Header.Set("X-CS-Session-ID", "sess-1")
	rec := httptest.NewRecorder()

	h.HandlePiExtensionLoaded(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, piextension.PiExtensionHealthLoaded, tracker.HealthFor("sess-1"))
}

func TestHandlePiExtensionLoaded_NoTracker_StillReturns200(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(approvalkit.NewApprovalStore(""), nil, bus)
	// Deliberately never call SetPiExtensionHealthTracker.

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/pi-extension-loaded", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	require.NotPanics(t, func() { h.HandlePiExtensionLoaded(rec, req) })
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlePiExtensionLoaded_RejectsNonPost(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(approvalkit.NewApprovalStore(""), nil, bus)
	h.SetPiExtensionHealthTracker(piextension.NewPiExtensionHealthTracker())

	req := httptest.NewRequest(http.MethodGet, "/api/hooks/pi-extension-loaded", nil)
	rec := httptest.NewRecorder()

	h.HandlePiExtensionLoaded(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandlePiExtensionLoaded_MalformedBody_StillReturns200(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(config.FeaturePiSupport, true))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(approvalkit.NewApprovalStore(""), nil, bus)
	h.SetPiExtensionHealthTracker(piextension.NewPiExtensionHealthTracker())

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
	svctest.WithIsolatedStateDir(t)
	require.False(t, config.LoadConfig().GetFeatureFlag(config.FeaturePiSupport))

	bus := events.NewEventBus(1)
	defer bus.Close()
	h := NewApprovalHandler(approvalkit.NewApprovalStore(""), nil, bus)
	tracker := piextension.NewPiExtensionHealthTracker()
	h.SetPiExtensionHealthTracker(tracker)

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/pi-extension-loaded", strings.NewReader(`{}`))
	req.Header.Set("X-CS-Session-ID", "sess-1")
	rec := httptest.NewRecorder()

	h.HandlePiExtensionLoaded(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, piextension.PiExtensionHealthUnknown, tracker.HealthFor("sess-1"))
}
