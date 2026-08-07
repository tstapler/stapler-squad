package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// newIsolatedCallbackConfigService creates a CallbackConfigService backed by a
// fresh temporary directory, preventing config state from leaking between tests
// (same isolation pattern as newIsolatedDefaultsService, defaults_service_test.go).
func newIsolatedCallbackConfigService(t *testing.T) *CallbackConfigService {
	t.Helper()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	return NewCallbackConfigService()
}

func callbackURLPtr(s string) *string { return &s }

// TestUpdateCallbackConfig_RejectsSSRFTarget covers AC11's config-save half: a URL
// that resolves to the cloud-metadata address is rejected with CodeInvalidArgument
// and never persisted.
func TestUpdateCallbackConfig_RejectsSSRFTarget(t *testing.T) {
	c := newIsolatedCallbackConfigService(t)

	_, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr("http://169.254.169.254/"),
	}))
	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())

	// Not persisted: a subsequent Get must report unconfigured.
	getResp, getErr := c.GetCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.GetCallbackConfigRequest{}))
	require.NoError(t, getErr)
	assert.False(t, getResp.Msg.Config.OnSessionCompleteConfigured, "rejected URL must not be persisted")
}

// TestUpdateCallbackConfig_PersistsValidURL_MaskedOnRead covers AC4 (partial,
// config side): a valid URL is accepted and persisted, but never echoed back in
// plaintext — GetCallbackConfig reports only a boolean.
func TestUpdateCallbackConfig_PersistsValidURL_MaskedOnRead(t *testing.T) {
	c := newIsolatedCallbackConfigService(t)

	updateResp, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr("https://example.com/hook"),
	}))
	require.NoError(t, err)
	assert.True(t, updateResp.Msg.Config.OnSessionCompleteConfigured)
	assert.False(t, updateResp.Msg.Config.OnSessionStaleConfigured)
	assert.False(t, updateResp.Msg.Config.OnQueueItemCreatedConfigured)

	getResp, err := c.GetCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.GetCallbackConfigRequest{}))
	require.NoError(t, err)
	assert.True(t, getResp.Msg.Config.OnSessionCompleteConfigured)
}

// TestUpdateCallbackConfig_UnsetFieldLeavesExistingValueUnchanged covers the
// "unset means leave unchanged" contract UpdateCallbackConfigRequest documents.
func TestUpdateCallbackConfig_UnsetFieldLeavesExistingValueUnchanged(t *testing.T) {
	c := newIsolatedCallbackConfigService(t)

	_, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr("https://example.com/hook"),
	}))
	require.NoError(t, err)

	// Second call sets a different field, leaving on_session_complete_url unset.
	resp, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionStaleUrl: callbackURLPtr("https://example.com/stale-hook"),
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Config.OnSessionCompleteConfigured, "unset field must leave the existing value unchanged")
	assert.True(t, resp.Msg.Config.OnSessionStaleConfigured)
}

// TestUpdateCallbackConfig_EmptyStringClearsURL covers the "empty string clears"
// half of the same contract.
func TestUpdateCallbackConfig_EmptyStringClearsURL(t *testing.T) {
	c := newIsolatedCallbackConfigService(t)

	_, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr("https://example.com/hook"),
	}))
	require.NoError(t, err)

	resp, err := c.UpdateCallbackConfig(context.Background(), connect.NewRequest(&sessionv1.UpdateCallbackConfigRequest{
		OnSessionCompleteUrl: callbackURLPtr(""),
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Config.OnSessionCompleteConfigured)
}
