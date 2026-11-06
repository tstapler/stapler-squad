package services

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

type recordedCall struct {
	name string
	args []string
}

// fakeRunner returns scripted results per command name and records every call.
func fakeRunner(calls *[]recordedCall, results map[string]struct {
	out string
	err error
}) commandRunner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, recordedCall{name, args})
		r := results[name]
		return []byte(r.out), r.err
	}
}

type result = struct {
	out string
	err error
}

func TestLaunchServicesActivator_RunningAppIsOpenedByBundleID(t *testing.T) {
	t.Parallel()
	var calls []recordedCall
	a := &launchServicesActivator{run: fakeRunner(&calls, map[string]result{
		"/usr/bin/lsappinfo": {out: "ASN:0x0-0x1234: \n"},
	})}

	require.NoError(t, a.Activate(context.Background(), "com.microsoft.VSCode", ""))

	require.Len(t, calls, 2)
	assert.Equal(t, []string{"find", "bundleid=com.microsoft.VSCode"}, calls[0].args)
	assert.Equal(t, "/usr/bin/open", calls[1].name)
	assert.Equal(t, []string{"-b", "com.microsoft.VSCode"}, calls[1].args)
}

func TestLaunchServicesActivator_NeverUsesOsascript(t *testing.T) {
	t.Parallel()
	var calls []recordedCall
	a := &launchServicesActivator{run: fakeRunner(&calls, map[string]result{
		"/usr/bin/lsappinfo": {out: "ASN:0x0-0x1: \n"},
	})}

	require.NoError(t, a.Activate(context.Background(), "", "Terminal"))

	for _, c := range calls {
		assert.NotContains(t, c.name, "osascript")
	}
	assert.Equal(t, []string{"-a", "Terminal"}, calls[1].args)
}

func TestLaunchServicesActivator_NotRunningDoesNotLaunch(t *testing.T) {
	t.Parallel()
	var calls []recordedCall
	a := &launchServicesActivator{run: fakeRunner(&calls, map[string]result{
		"/usr/bin/lsappinfo": {out: "\n"},
	})}

	err := a.Activate(context.Background(), "com.example.Gone", "")

	assert.ErrorIs(t, err, ErrAppNotRunning)
	assert.Len(t, calls, 1, "open must not run for an app that isn't running")
}

func TestLaunchServicesActivator_UnconfirmedRunningCheckDoesNotLaunch(t *testing.T) {
	t.Parallel()
	var calls []recordedCall
	a := &launchServicesActivator{run: fakeRunner(&calls, map[string]result{
		"/usr/bin/lsappinfo": {err: errors.New("exec: not found")},
	})}

	err := a.Activate(context.Background(), "com.example.App", "")

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrAppNotRunning)
	assert.Len(t, calls, 1)
}

func TestLaunchServicesActivator_OpenFailurePropagates(t *testing.T) {
	t.Parallel()
	var calls []recordedCall
	a := &launchServicesActivator{run: fakeRunner(&calls, map[string]result{
		"/usr/bin/lsappinfo": {out: "ASN:0x0-0x1: \n"},
		"/usr/bin/open":      {out: "boom", err: errors.New("exit 1")},
	})}

	err := a.Activate(context.Background(), "com.example.App", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestLaunchServicesActivator_RejectsMalformedBundleID(t *testing.T) {
	t.Parallel()
	var calls []recordedCall
	a := &launchServicesActivator{run: fakeRunner(&calls, nil)}

	err := a.Activate(context.Background(), `x" to quit`, "")

	require.Error(t, err)
	assert.Empty(t, calls)
}

func TestIsBlockedBundleID(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"com.apple.Music", "com.apple.TV", "com.apple.Photos", "com.apple.podcasts"} {
		assert.True(t, isBlockedBundleID(id), id)
	}
	for _, id := range []string{"com.apple.Terminal", "com.apple.dt.Xcode", "com.microsoft.VSCode", ""} {
		assert.False(t, isBlockedBundleID(id), id)
	}
}

func TestIsBlockedTarget_ByAppName(t *testing.T) {
	t.Parallel()
	for _, n := range []string{"Music", "music", " TV ", "Photos"} {
		assert.True(t, isBlockedTarget("", n), n)
	}
	assert.False(t, isBlockedTarget("", "Terminal"))
	assert.False(t, isBlockedTarget("com.microsoft.VSCode", "Music"), "bundle ID takes precedence")
}

func TestLaunchServicesActivator_AcceptsUnderscoreBundleID(t *testing.T) {
	t.Parallel()
	var calls []recordedCall
	a := &launchServicesActivator{run: fakeRunner(&calls, map[string]result{
		"/usr/bin/lsappinfo": {out: "ASN:0x0-0x1: \n"},
	})}
	require.NoError(t, a.Activate(context.Background(), "com.example.My_App", ""))
}

type stubActivator struct {
	called bool
	err    error
}

func (s *stubActivator) Activate(context.Context, string, string) error {
	s.called = true
	return s.err
}

func focusRequest(bundleID string) *connect.Request[sessionv1.FocusWindowRequest] {
	return connect.NewRequest(&sessionv1.FocusWindowRequest{BundleId: &bundleID})
}

func TestFocusWindow_BlockedBundleIsNoOpWithMessage(t *testing.T) {
	t.Parallel()
	if detectPlatform() != "darwin" {
		t.Skip("darwin-only code path")
	}
	svc := setupUtilityService()
	stub := &stubActivator{}
	svc.activator = stub

	resp, err := svc.FocusWindow(context.Background(), focusRequest("com.apple.Music"))

	require.NoError(t, err)
	assert.False(t, resp.Msg.Success)
	assert.Contains(t, resp.Msg.Message, "com.apple.Music")
	assert.False(t, stub.called, "blocked apps must never reach the activator")
}

func TestFocusWindow_NotRunningMessage(t *testing.T) {
	t.Parallel()
	if detectPlatform() != "darwin" {
		t.Skip("darwin-only code path")
	}
	svc := setupUtilityService()
	svc.activator = &stubActivator{err: ErrAppNotRunning}

	resp, err := svc.FocusWindow(context.Background(), focusRequest("com.example.Gone"))

	require.NoError(t, err)
	assert.False(t, resp.Msg.Success)
	assert.Equal(t, "Application 'com.example.Gone' is not running", resp.Msg.Message)
}

func TestFocusWindow_ActivatedSuccessfully(t *testing.T) {
	t.Parallel()
	if detectPlatform() != "darwin" {
		t.Skip("darwin-only code path")
	}
	svc := setupUtilityService()
	stub := &stubActivator{}
	svc.activator = stub

	resp, err := svc.FocusWindow(context.Background(), focusRequest("com.microsoft.VSCode"))

	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
	assert.True(t, stub.called)
}
