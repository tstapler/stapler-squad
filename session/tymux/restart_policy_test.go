package tymux

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

func TestRestartPolicyFor(t *testing.T) {
	for _, tc := range []struct {
		name         string
		env          string
		capabilities []string
		want         v1.RestartMode // UNSPECIFIED means no policy is sent
	}{
		{"default", "", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_ON_FAILURE},
		{"unrecognized falls back", "bogus", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_ON_FAILURE},
		{"always", "always", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_ALWAYS},
		{"never", " Never ", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_UNSPECIFIED},
		{"off", "off", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_UNSPECIFIED},
		{"false", "FALSE", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_UNSPECIFIED},
		{"none", "none", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_UNSPECIFIED},
		{"disabled", "disabled", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_UNSPECIFIED},
		{"explicit on_failure", "on_failure", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_ON_FAILURE},
		{"daemon without pane.lifecycle", "", []string{"create_session.stdin_stream"}, v1.RestartMode_RESTART_MODE_UNSPECIFIED},
		{"pre-capability daemon", "", nil, v1.RestartMode_RESTART_MODE_UNSPECIFIED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(restartModeEnv, tc.env)
			sess := NewTymuxGRPCSession(&fakeTransport{capabilities: tc.capabilities}).(*tymuxGRPCSession)
			got := sess.restartPolicyFor(context.Background())
			if tc.want == v1.RestartMode_RESTART_MODE_UNSPECIFIED {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, tc.want, got.GetMode())
		})
	}
}

func TestStart_SendsRestartPolicyOnlyToCapableDaemons(t *testing.T) {
	t.Setenv(restartModeEnv, "")
	for _, tc := range []struct {
		name         string
		capabilities []string
		wantMode     v1.RestartMode
	}{
		{"capable", []string{capPaneLifecycle}, v1.RestartMode_RESTART_MODE_ON_FAILURE},
		{"older daemon", nil, v1.RestartMode_RESTART_MODE_UNSPECIFIED},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			var got *v1.RestartPolicy
			transport := &fakeTransport{
				capabilities: tc.capabilities,
				createSessionFn: func(_ context.Context, req *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
					got = req.Msg.GetRestartPolicy()
					return connect.NewResponse(fakeSession(testSessionID, testPaneID, dir, v1.Liveness_LIVENESS_LIVE)), nil
				},
			}
			require.NoError(t, NewTymuxGRPCSession(transport).Start(dir))
			assert.Equal(t, tc.wantMode, got.GetMode())
		})
	}
}
