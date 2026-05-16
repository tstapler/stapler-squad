package services

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
)

// fakeFeatureController is a test double for FeatureController.
type fakeFeatureController struct {
	enableCalled  bool
	disableCalled bool
	enabled       bool
}

func (f *fakeFeatureController) Enable(_ context.Context) error {
	f.enableCalled = true
	f.enabled = true
	return nil
}

func (f *fakeFeatureController) Disable() error {
	f.disableCalled = true
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
	return NewSessionService(storage, eventBus)
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
