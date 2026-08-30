package services

import (
	"context"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

// fakeFeatureController is a test double for FeatureController.
type fakeFeatureController struct {
	enableCalled  bool
	disableCalled bool
	enabled       bool
	failEnable    error
	failDisable   error
}

func (f *fakeFeatureController) Enable(_ context.Context) error {
	f.enableCalled = true
	if f.failEnable != nil {
		return f.failEnable
	}
	f.enabled = true
	return nil
}

func (f *fakeFeatureController) Disable() error {
	f.disableCalled = true
	if f.failDisable != nil {
		return f.failDisable
	}
	f.enabled = false
	return nil
}

func (f *fakeFeatureController) IsEnabled() bool { return f.enabled }

// newFeatureFlagService creates a minimal SessionService wired for feature-flag tests.
// Config I/O is redirected to a temporary directory via STAPLER_SQUAD_TEST_DIR so
// tests are isolated from the developer's real ~/.stapler-squad config.
func newFeatureFlagService(t *testing.T) *SessionService {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	storage := createTestStorage(t)
	eventBus := events.NewEventBus(100)
	svc := NewSessionService(storage, eventBus)
	t.Cleanup(func() { svc.Shutdown() })
	return svc
}

// --------------------------------------------------------------------------
// GetFeatureFlags
// --------------------------------------------------------------------------

// TestGetFeatureFlags_ReturnsKnownFlags verifies that GetFeatureFlags includes
// the "backlog" flag with a non-empty description in its response.
func TestGetFeatureFlags_ReturnsKnownFlags(t *testing.T) {
	svc := newFeatureFlagService(t)

	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg)

	var backlogFlag *sessionv1.FeatureFlag
	for _, f := range resp.Msg.Flags {
		if f.Name == "backlog" {
			backlogFlag = f
			break
		}
	}
	require.NotNil(t, backlogFlag, "expected 'backlog' flag in GetFeatureFlags response")
	assert.NotEmpty(t, backlogFlag.Description, "backlog flag should have a non-empty description")
}

// TestGetFeatureFlags_should_ReturnAllSevenResyncFlagsWithCorrectDefaults_When_RegistryQueried
// covers project_plans/terminal-resync-reliability/implementation/validation.md's AC7 row:
// all 7 terminal-resync feature flags must be present in the GetFeatureFlags response with
// no controller wired and no config.json override. Originally all 7 defaulted to disabled;
// terminal:resync-exec-gate-fast-lane graduated to default-on 2026-08-25 (see
// featureFlagDefault's doc comment), so this now checks each flag against its own expected
// default instead of asserting a single shared value.
func TestGetFeatureFlags_should_ReturnAllSevenResyncFlagsWithCorrectDefaults_When_RegistryQueried(t *testing.T) {
	svc := newFeatureFlagService(t)

	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg)

	// terminal:resync-exec-gate-fast-lane graduated to default-on 2026-08-25 (see
	// featureFlagDefault's doc comment) — every other resync flag still defaults off.
	wantDefault := map[string]bool{
		"terminal:resync-visibility-scope":              false,
		"terminal:resync-correlation-id":                false,
		"terminal:resync-skip-stale-dimension-slowpath": false,
		"terminal:resync-exec-gate-fast-lane":           true,
		"terminal:resync-stagger":                       false,
		"terminal:resync-compression":                   false,
		"terminal:resync-batching":                      false,
	}

	byName := make(map[string]*sessionv1.FeatureFlag, len(resp.Msg.Flags))
	for _, f := range resp.Msg.Flags {
		byName[f.Name] = f
	}

	for name, want := range wantDefault {
		flag, ok := byName[name]
		require.Truef(t, ok, "expected %q flag in GetFeatureFlags response", name)
		assert.Equalf(t, want, flag.Enabled, "%q default mismatch", name)
		assert.NotEmptyf(t, flag.Description, "%q should have a non-empty description", name)
	}
}

// TestGetFeatureFlags_ReflectsControllerState verifies that when a FeatureController
// is wired and reports IsEnabled=true, GetFeatureFlags returns enabled=true for that flag.
func TestGetFeatureFlags_ReflectsControllerState(t *testing.T) {
	svc := newFeatureFlagService(t)

	ctrl := &fakeFeatureController{enabled: true}
	svc.SetFeatureController("backlog", ctrl)

	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg)

	var backlogFlag *sessionv1.FeatureFlag
	for _, f := range resp.Msg.Flags {
		if f.Name == "backlog" {
			backlogFlag = f
			break
		}
	}
	require.NotNil(t, backlogFlag, "expected 'backlog' flag in GetFeatureFlags response")
	assert.True(t, backlogFlag.Enabled, "backlog flag should be enabled when controller reports IsEnabled=true")
}

// TestGetFeatureFlags_should_PopulateStatusDetailForBacklogOnly_When_QuotaGatePausedByQuota
// verifies the "backlog" flag's StatusDetail reflects a wired status-detail
// provider, and every other flag's StatusDetail stays empty.
func TestGetFeatureFlags_should_PopulateStatusDetailForBacklogOnly_When_QuotaGatePausedByQuota(t *testing.T) {
	svc := newFeatureFlagService(t)
	svc.SetStatusDetailProvider("backlog", func() string { return "Paused: session-quota headroom below threshold." })

	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg)

	for _, f := range resp.Msg.Flags {
		if f.Name == "backlog" {
			assert.Equal(t, "Paused: session-quota headroom below threshold.", f.StatusDetail)
		} else {
			assert.Empty(t, f.StatusDetail, "flag %q should have empty StatusDetail (no provider wired)", f.Name)
		}
	}
}

// TestGetFeatureFlags_should_ReturnEmptyStatusDetail_When_ProviderReturnsEmptyString
// verifies a wired-but-healthy provider produces no noise on the response.
func TestGetFeatureFlags_should_ReturnEmptyStatusDetail_When_ProviderReturnsEmptyString(t *testing.T) {
	svc := newFeatureFlagService(t)
	svc.SetStatusDetailProvider("backlog", func() string { return "" })

	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)

	var backlogFlag *sessionv1.FeatureFlag
	for _, f := range resp.Msg.Flags {
		if f.Name == "backlog" {
			backlogFlag = f
			break
		}
	}
	require.NotNil(t, backlogFlag)
	assert.Empty(t, backlogFlag.StatusDetail)
}

// TestGetFeatureFlags_should_ReflectHandoffSummaryConfigState_When_QueriedEnabledOrDisabled
// verifies the "handoff-summary" flag added for Finding 2 (backlog review of
// the restart-with-summary feature) surfaces config.HandoffSummaryConfig's
// real, config.json-backed EnabledOrDefault() state through the generic
// GetFeatureFlags registry -- not the separate feature_flags map -- covering
// both the enabled and explicitly-disabled cases.
func TestGetFeatureFlags_should_ReflectHandoffSummaryConfigState_When_QueriedEnabledOrDisabled(t *testing.T) {
	svc := newFeatureFlagService(t)
	svc.SetFeatureController("handoff-summary", HandoffSummaryFeatureController{})

	findFlag := func(t *testing.T) *sessionv1.FeatureFlag {
		t.Helper()
		resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
		require.NoError(t, err)
		for _, f := range resp.Msg.Flags {
			if f.Name == "handoff-summary" {
				return f
			}
		}
		t.Fatal("expected 'handoff-summary' flag in GetFeatureFlags response")
		return nil
	}

	// Default (no config.json override yet): enabled.
	flag := findFlag(t)
	assert.True(t, flag.Enabled, "handoff-summary should default to enabled")
	assert.NotEmpty(t, flag.Description)

	// Explicitly disabled via config.HandoffSummary, not the generic flags map.
	disabled := false
	cfg := config.LoadConfig()
	cfg.HandoffSummary.Enabled = &disabled
	require.NoError(t, config.SaveConfig(cfg))

	flag = findFlag(t)
	assert.False(t, flag.Enabled, "handoff-summary should reflect config.HandoffSummary.Enabled=false")

	// Re-enabled.
	enabled := true
	cfg = config.LoadConfig()
	cfg.HandoffSummary.Enabled = &enabled
	require.NoError(t, config.SaveConfig(cfg))

	flag = findFlag(t)
	assert.True(t, flag.Enabled, "handoff-summary should reflect config.HandoffSummary.Enabled=true")
}

// TestHandoffSummaryFeatureController_EnableAndDisable_should_ReturnClearError
// verifies UpdateFeatureFlag-style toggling of "handoff-summary" is refused
// with a clear, actionable error rather than silently no-op'ing -- the real
// toggle lives at config.json's handoff_summary.enabled key, not the generic
// feature-flags map UpdateFeatureFlag persists to.
func TestHandoffSummaryFeatureController_EnableAndDisable_should_ReturnClearError(t *testing.T) {
	ctrl := HandoffSummaryFeatureController{}

	err := ctrl.Enable(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handoff_summary.enabled")

	err = ctrl.Disable()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handoff_summary.enabled")
}

// --------------------------------------------------------------------------
// UpdateFeatureFlag
// --------------------------------------------------------------------------

// TestUpdateFeatureFlag_UnknownFlag verifies that calling UpdateFeatureFlag with an
// unrecognised flag name returns a CodeInvalidArgument error.
func TestUpdateFeatureFlag_UnknownFlag(t *testing.T) {
	svc := newFeatureFlagService(t)

	_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name:    "unknown",
		Enabled: true,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestUpdateFeatureFlag_EnablesController verifies that UpdateFeatureFlag{name:"backlog",
// enabled:true} calls Enable on the wired controller.
func TestUpdateFeatureFlag_EnablesController(t *testing.T) {
	svc := newFeatureFlagService(t)

	ctrl := &fakeFeatureController{}
	svc.SetFeatureController("backlog", ctrl)

	resp, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name:    "backlog",
		Enabled: true,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.True(t, resp.Msg.Flag.Enabled)

	assert.True(t, ctrl.enableCalled, "expected Enable to be called on the controller")
	assert.False(t, ctrl.disableCalled, "expected Disable NOT to be called")
}

// TestUpdateFeatureFlag_DisablesController verifies that UpdateFeatureFlag{name:"backlog",
// enabled:false} calls Disable on the wired controller.
func TestUpdateFeatureFlag_DisablesController(t *testing.T) {
	svc := newFeatureFlagService(t)

	ctrl := &fakeFeatureController{enabled: true}
	svc.SetFeatureController("backlog", ctrl)

	resp, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name:    "backlog",
		Enabled: false,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg)
	assert.False(t, resp.Msg.Flag.Enabled)

	assert.True(t, ctrl.disableCalled, "expected Disable to be called on the controller")
	assert.False(t, ctrl.enableCalled, "expected Enable NOT to be called")
}

// TestUpdateFeatureFlag_RollsBackDiskFlagWhenControllerFails verifies that when the
// wired controller's Enable fails, UpdateFeatureFlag returns an error AND rolls back
// the just-persisted disk flag, so disk config and in-memory controller state can
// never diverge (previously a controller failure was only logged and the RPC
// reported success, leaving TriggerSync's disk-gated interceptor permanently out of
// sync with the in-memory feature check).
func TestUpdateFeatureFlag_RollsBackDiskFlagWhenControllerFails(t *testing.T) {
	svc := newFeatureFlagService(t)

	ctrl := &fakeFeatureController{failEnable: assert.AnError}
	svc.SetFeatureController("backlog", ctrl)

	_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
		Name:    "backlog",
		Enabled: true,
	}))
	require.Error(t, err)

	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInternal, connectErr.Code())
	assert.True(t, ctrl.enableCalled, "expected Enable to have been attempted")

	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	var backlogFlag *sessionv1.FeatureFlag
	for _, f := range resp.Msg.Flags {
		if f.Name == "backlog" {
			backlogFlag = f
			break
		}
	}
	require.NotNil(t, backlogFlag)
	assert.False(t, backlogFlag.Enabled, "disk flag must be rolled back to its previous (disabled) value after the controller failed")
}

// blockingFeatureController lets a test control exactly when Enable's critical
// section starts and ends, to deterministically prove mutual exclusion without
// relying on timing/sleep-based flakiness.
type blockingFeatureController struct {
	entered chan struct{}
	unblock chan struct{}
	enabled bool
}

func (b *blockingFeatureController) Enable(_ context.Context) error {
	close(b.entered)
	<-b.unblock
	b.enabled = true
	return nil
}

func (b *blockingFeatureController) Disable() error {
	b.enabled = false
	return nil
}

func (b *blockingFeatureController) IsEnabled() bool { return b.enabled }

// TestUpdateFeatureFlag_SerializesConcurrentTogglesOfSameFlag proves the new
// updateMu lock actually excludes concurrent UpdateFeatureFlag calls for the same
// flag — without it, a second call's read-modify-write of the disk config could
// interleave with the first call's still-in-flight controller toggle and rollback,
// reintroducing the disk/controller divergence the rollback fix is meant to close.
func TestUpdateFeatureFlag_SerializesConcurrentTogglesOfSameFlag(t *testing.T) {
	svc := newFeatureFlagService(t)

	ctrl := &blockingFeatureController{
		entered: make(chan struct{}),
		unblock: make(chan struct{}),
	}
	svc.SetFeatureController("backlog", ctrl)

	firstDone := make(chan struct{})
	go func() {
		_, _ = svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
			Name: "backlog", Enabled: true,
		}))
		close(firstDone)
	}()

	<-ctrl.entered // first call's Enable() is now blocked mid-critical-section

	secondDone := make(chan struct{})
	go func() {
		_, _ = svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{
			Name: "backlog", Enabled: false,
		}))
		close(secondDone)
	}()

	// The second call must be blocked on updateMu — it cannot complete while the
	// first is still holding the lock inside its Enable() call.
	select {
	case <-secondDone:
		t.Fatal("second UpdateFeatureFlag call completed while the first was still in its critical section — updateMu did not serialize")
	case <-time.After(50 * time.Millisecond):
	}

	close(ctrl.unblock) // release the first call
	<-firstDone
	<-secondDone
}
