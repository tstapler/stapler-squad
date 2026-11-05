//go:build integration

package tymux

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

// The web terminal paints its first frame from CapturePaneContentRaw; if that drops SGR the
// pane shows no color until live output arrives. Runs against an isolated tymuxd.
func TestLiveTymuxd_RawCapture_PreservesColor(t *testing.T) {
	bin := findTymuxdBinary(t)
	daemon := startLiveTymuxd(t, bin, t.TempDir(), reserveLoopbackAddr(t))
	defer daemon.stop(t)

	sess := NewTymuxGRPCSession(NewRealTransport("http://" + daemon.addr))
	plainShell(sess)
	require.NoError(t, sess.Start(t.TempDir()))
	defer sess.Close()
	_, err := sess.SendKeys(`printf '\033[31m%s\033[0m\n' COLOR''OUT` + "\r")
	require.NoError(t, err)

	var plain string
	wait.RequireEventually(t, func() bool {
		plain, err = sess.CapturePaneContentRaw()
		return err == nil && strings.Contains(plain, "COLOROUT")
	}, 5*time.Second, 100*time.Millisecond, "printed output never appeared in the pane")

	snap, err := sess.(*tymuxGRPCSession).capturePane(t.Context(), 0)
	require.NoError(t, err)
	var daemonSentColor bool
	for _, row := range snap.GetGrid() {
		cells := row.GetCells()
		if len(cells) > 0 && cells[0].GetText() == "C" && cells[1].GetText() == "O" && cells[0].GetFg() != 0 {
			daemonSentColor = true
		}
	}
	assert.True(t, daemonSentColor, "daemon snapshot cells should carry the foreground color")
	assert.Contains(t, plain, "\x1b[", "CapturePaneContentRaw must keep SGR so the first paint is colored; got %q", plain)
}
