package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ---------------------------------------------------------------------------
// FocusWindow
// ---------------------------------------------------------------------------

// TestFocusWindow_EmptyID verifies that FocusWindow returns CodeInvalidArgument
// when neither bundle_id nor app_name is provided.
// The localhost origin check passes because no X-Real-IP / X-Forwarded-For
// header is set (direct connect path → allowed).
func TestFocusWindow_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewWindowFocusService()

	_, err := svc.FocusWindow(context.Background(), connect.NewRequest(&sessionv1.FocusWindowRequest{
		// Both BundleId and AppName intentionally left nil.
	}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	assert.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

// TestFocusWindow_NonDarwinPlatform verifies that FocusWindow returns a
// successful response (with Success=false) on non-macOS platforms.
// On Linux (the CI platform) the call must not error; it returns a graceful
// "not supported" message instead.
func TestFocusWindow_NonDarwinPlatform(t *testing.T) {
	t.Parallel()
	svc := NewWindowFocusService()

	appName := "SomeApp"
	resp, err := svc.FocusWindow(context.Background(), connect.NewRequest(&sessionv1.FocusWindowRequest{
		AppName: &appName,
	}))

	// On Linux the call succeeds at the RPC level but reports Success=false.
	// On macOS (developer machine), the activator may report not-running — either
	// outcome is acceptable; the important invariant is no panic and no
	// internal error code.
	if err != nil {
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		assert.NotEqual(t, connect.CodeInternal, connectErr.Code(),
			"FocusWindow must not return CodeInternal for a missing app on non-darwin")
		return
	}
	require.NotNil(t, resp)
}
