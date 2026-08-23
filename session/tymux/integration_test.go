//go:build integration

package tymux

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTymuxGRPCSession_LiveTymuxd_StartSendKeysCaptureClose is Task
// 2.2.1d's reserved live-daemon tier, exercised through Epic 2.3's real
// transport (tymux.NewRealTransport) and standing Attach stream — the
// first point in this whole implementation where stapler-squad's Go code
// can prove it actually talks to a live tymuxd, not just a fake
// rpcTransport. Every other test in this package runs against
// fakeTransport/fakeAttachStream and proves nothing about the real wire
// protocol.
//
// Requires a real tymuxd already listening (default 127.0.0.1:7419,
// overridable via TYMUXD_ADDR — see transport.go's tymuxdAddr()). Run
// with: go test -tags integration ./session/tymux/...
func TestTymuxGRPCSession_LiveTymuxd_StartSendKeysCaptureClose(t *testing.T) {
	dir := t.TempDir()
	sess := NewTymuxGRPCSession(NewRealTransport(""))

	require.NoError(t, sess.Start(dir), "Start against a live tymuxd — is tymuxd running (see TYMUXD_ADDR)?")
	defer sess.Close()

	assert.NotEmpty(t, sess.GetSessionIdentifier())
	assert.True(t, sess.IsAlive())

	marker := "tymux-epic-2-3-integration-test"
	n, err := sess.SendKeys("echo " + marker + "\r")
	require.NoError(t, err)
	assert.Greater(t, n, 0)

	var content string
	require.Eventually(t, func() bool {
		content, err = sess.CapturePaneContentRaw()
		return err == nil && strings.Contains(content, marker)
	}, 5*time.Second, 100*time.Millisecond, "expected the echoed marker in captured pane content; last content: %q", content)

	require.NoError(t, sess.Close())
	assert.False(t, sess.IsAlive())
}
