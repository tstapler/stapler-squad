package tymux

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

func TestSharedDaemonBlockedInTest(t *testing.T) {
	tests := []struct {
		name  string
		addr  string
		allow string
		want  bool
	}{
		{"shared default is blocked", defaultTymuxdAddr, "", true},
		{"explicit opt-in allows it", defaultTymuxdAddr, "1", false},
		{"test-owned address is allowed", "http://127.0.0.1:45123", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(allowSharedDaemonInTestEnv, tc.allow)
			assert.Equal(t, tc.want, sharedDaemonBlockedInTest(tc.addr))
		})
	}
}

func TestEnsureDaemonRunning_RefusesSharedDaemonInTest(t *testing.T) {
	t.Setenv(allowSharedDaemonInTestEnv, "")
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")

	_, err := EnsureDaemonRunning(context.Background(), ResolveDaemonConfig())

	require.ErrorIs(t, err, ErrSharedDaemonInTest)
}

func TestNewRealTransport_SharedDaemonRedirectedToDeadAddr(t *testing.T) {
	t.Setenv(allowSharedDaemonInTestEnv, "")
	t.Setenv("TYMUXD_ADDR", "")

	// Port 1 refuses the connection; a call reaching the real daemon would succeed.
	_, err := NewRealTransport("").ListSessions(context.Background(), connect.NewRequest(&v1.ListSessionsRequest{}))

	require.Error(t, err)
}
