//go:build integration

package tymux

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/launchcmd"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// startRawCatSession launches a pane that puts its tty in raw mode and copies every input byte
// into outFile, then waits until the pane says it is ready. What lands in outFile is exactly
// what the daemon delivered to the pane's stdin.
func startRawCatSession(t *testing.T, outFile string) TymuxManager {
	t.Helper()
	daemon := startLiveTymuxd(t, findTymuxdBinary(t), t.TempDir(), reserveLoopbackAddr(t))
	t.Cleanup(func() { daemon.stop(t) })

	sess := NewTymuxGRPCSession(NewRealTransport("http://" + daemon.addr))
	sess.SetCommand(launchcmd.ExecCommand{Argv: []launchcmd.Arg{
		launchcmd.Word("sh"), launchcmd.Literal("-c"),
		launchcmd.Literal("stty raw -echo; echo READY; exec cat > " + launchcmd.Quote(outFile)),
	}})
	require.NoError(t, sess.Start(t.TempDir()))
	t.Cleanup(func() { go sess.Close() })
	wait.RequireEventually(t, func() bool {
		content, err := sess.CapturePaneContent()
		return err == nil && strings.Contains(content, "READY")
	}, 10*time.Second, 50*time.Millisecond, "pane never became ready")
	return sess
}

func startNumberedHistorySession(t *testing.T) TymuxManager {
	t.Helper()
	// A private daemon and real PTY are required to verify the daemon's history coordinates.
	daemon := startLiveTymuxd(t, findTymuxdBinary(t), t.TempDir(), reserveLoopbackAddr(t))
	t.Cleanup(func() { daemon.stop(t) })
	sess := NewTymuxGRPCSession(NewRealTransport("http://" + daemon.addr))
	sess.SetCommand(launchcmd.ExecCommand{Argv: []launchcmd.Arg{
		launchcmd.Word("sh"), launchcmd.Literal("-c"),
		launchcmd.Literal("i=1; while [ $i -le 120 ]; do printf 'HISTORY-%03d\\n' \"$i\"; i=$((i+1)); done; printf 'READY\\nDONE'; exec cat"),
	}})
	require.NoError(t, sess.Start(t.TempDir()))
	t.Cleanup(func() {
		// Killing the private daemon unblocks Close if a teardown RPC stops responding.
		timer := time.AfterFunc(10*time.Second, func() { _ = daemon.cmd.Process.Kill() })
		defer timer.Stop()
		if err := sess.Close(); err != nil {
			t.Logf("session cleanup: %v", err)
		}
	})
	wait.RequireEventually(t, func() bool {
		content, err := sess.CapturePaneContentRaw()
		return err == nil && strings.Contains(content, "DONE")
	}, 10*time.Second, 50*time.Millisecond, "numbered history never finished printing")

	return sess
}

func TestLiveTymuxd_ScrollbackRange_ReturnsHistoryInsteadOfLiveScreen(t *testing.T) {
	sess := startNumberedHistorySession(t)
	width, height, err := sess.GetPaneDimensions()
	require.NoError(t, err)
	require.Greater(t, width, len("HISTORY-001"))
	require.Greater(t, height, 3)
	require.Less(t, height, 120)

	// Read the same oldest full page through the native offset and tmux range interfaces.
	oldest, err := sess.(*tymuxGRPCSession).capturePane(t.Context(), 1000)
	require.NoError(t, err)
	want := rowsToPlainText(oldest.GetGrid())
	require.Contains(t, want, "HISTORY-001")
	historyLines := 122 - height // 120 numbered lines, READY, and the final DONE cursor row.
	start := "-" + strconv.Itoa(historyLines)
	end := "-" + strconv.Itoa(historyLines-height+1)
	got, err := sess.CapturePaneContentWithOptions(start, end)
	require.NoError(t, err)
	assert.Equal(t, want, got, "tmux-style negative ranges must capture retained history, not offset zero")
	assert.NotContains(t, got, "READY", "live-screen content must not leak into this historical range")
}

func fileSize(path string) int {
	st, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return int(st.Size())
}

// A paste far larger than one chunk must reach the pane complete and in order, interleaved with
// resizes, instead of blocking in a single oversized write with nothing delivered.
func TestLiveTymuxd_LargePaste_ArrivesCompleteAndInOrder(t *testing.T) {
	out := filepath.Join(t.TempDir(), "received")
	sess := startRawCatSession(t, out)

	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = byte('a' + (i/997)%26) // runs differ every 997 bytes so a reorder shows up
	}
	resizerDone := make(chan struct{})
	go func() {
		defer close(resizerDone)
		for i := 0; i < 20; i++ {
			assert.NoError(t, sess.SetWindowSize(80+i%7, 24))
		}
	}()

	n, err := sess.SendKeys(string(payload))
	<-resizerDone

	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	wait.RequireEventually(t, func() bool { return fileSize(out) >= len(payload) }, 30*time.Second, 100*time.Millisecond,
		fmt.Sprintf("daemon delivered %d of %d bytes", fileSize(out), len(payload)))
	got, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	assert.True(t, bytes.Equal(payload, got), "pasted bytes arrived corrupted or reordered")
}

// Reproduces the audit's finding: unserialized Send calls on the standing stream silently
// dropped almost all input (8 concurrent writers delivered 1 of 1600 bytes) with no error.
func TestLiveTymuxd_ConcurrentInputAndResize_DeliversEveryByte(t *testing.T) {
	out := filepath.Join(t.TempDir(), "received")
	sess := startRawCatSession(t, out)

	const writers, perWriter = 8, 200
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				_, err := sess.SendKeys("x")
				assert.NoError(t, err)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			assert.NoError(t, sess.SetWindowSize(80+i%7, 24+i%3))
		}
	}()
	wg.Wait()

	want := writers * perWriter
	wait.RequireEventually(t, func() bool { return fileSize(out) >= want }, 15*time.Second, 50*time.Millisecond,
		fmt.Sprintf("daemon delivered %d of %d bytes", fileSize(out), want))
	assert.Equal(t, want, fileSize(out))

	// The stream must still work after the burst.
	_, err := sess.SendKeys("tail")
	require.NoError(t, err)
	wait.RequireEventually(t, func() bool { return fileSize(out) >= want+4 }, 10*time.Second, 50*time.Millisecond,
		fmt.Sprintf("input died after the burst: %d bytes", fileSize(out)))
}
