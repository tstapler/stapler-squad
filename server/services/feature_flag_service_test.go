package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
)

// TestFeatureFlagService_should_ToggleForwardingBehaviorForNextRequestOnly_When_UpdateFeatureFlagCalledMidSession
// is REQ-13's integration test (validation.md): terminalAppScrollForwardingClaudeFlagName
// is re-read fresh from config on every scrollbackResultForRequest call, not
// cached from an earlier request in the same session -- flipping it mid-session
// (the same live-settable path UpdateFeatureFlag drives) changes only the next
// request's behavior. Reuses the nil-*session.Instance-panic proof
// TestScrollbackResultForRequest_should_SkipAppScrollGate_When_FlagIsOff and
// TestScrollbackResultForRequest_should_AttemptAppScrollGate_When_FlagIsOn
// (connectrpc_websocket_test.go) already established for "was AppScrollGate
// reached" -- decisive because AppScrollGate(nil, ...) panics immediately.
func TestFeatureFlagService_should_ToggleForwardingBehaviorForNextRequestOnly_When_UpdateFeatureFlagCalledMidSession(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalAppScrollForwardingClaudeFlagName, true))
	assertScrollbackRequestReachesAppScrollGate(t, "native")

	// Flip the flag mid-session, the same live-settable path UpdateFeatureFlag
	// drives (never an env var/restart gate, per this repo's rollout-flag
	// convention).
	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalAppScrollForwardingClaudeFlagName, false))
	assertScrollbackRequestSkipsAppScrollGate(t)
}

// assertScrollbackRequestReachesAppScrollGate asserts a nil-*session.Instance
// request reaches AppScrollGate -- decisive because AppScrollGate(nil, ...)
// panics immediately (inst.GetProgram() dereferences a nil receiver's
// field), mirroring TestScrollbackResultForRequest_should_AttemptAppScrollGate_When_FlagIsOn's
// (connectrpc_websocket_test.go) proof technique.
func assertScrollbackRequestReachesAppScrollGate(t *testing.T, fallbackContent string) {
	t.Helper()
	reachedGate := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				reachedGate = true
			}
		}()
		_, _ = scrollbackResultForRequest(scrollbackRequestParams{
			startLine: "-100",
			endLine:   "-1",
			logPrefix: "[test]",
			fallback:  func(string, string) (string, error) { return fallbackContent, nil },
		})
	}()
	require.True(t, reachedGate, "expected AppScrollGate to be reached")
}

// assertScrollbackRequestSkipsAppScrollGate asserts the fallback runs and
// AppScrollGate is never reached, mirroring
// TestScrollbackResultForRequest_should_SkipAppScrollGate_When_FlagIsOff's
// (connectrpc_websocket_test.go) proof technique.
func assertScrollbackRequestSkipsAppScrollGate(t *testing.T) {
	t.Helper()
	fallbackCalled := false
	result, err := scrollbackResultForRequest(scrollbackRequestParams{
		startLine: "-100",
		endLine:   "-1",
		logPrefix: "[test]",
		fallback: func(string, string) (string, error) {
			fallbackCalled = true
			return "tmux-native content", nil
		},
	})

	require.NoError(t, err)
	require.True(t, fallbackCalled, "expected the fallback to be used instead of AppScrollGate")
	require.Nil(t, result.AppScroll)
}
