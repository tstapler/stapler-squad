package session

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// promptOfSize returns exactly size bytes of unit repeated (0 yields "").
func promptOfSize(unit string, size int) string {
	return strings.Repeat(unit, size/len(unit)+1)[:size]
}

// writtenBeforeEnter joins every content write the fake saw, up to the Enter.
func writtenBeforeEnter(t *testing.T, fake *fakePaneSubmitter) string {
	t.Helper()
	var got strings.Builder
	for _, c := range fake.sendCalls {
		if c == EnterKeySequence {
			break
		}
		assert.LessOrEqual(t, len(c), maxInlinePromptBytes)
		got.WriteString(c)
	}
	return got.String()
}

func TestParseDriverPrompt_DeliveredIntactAtEverySize(t *testing.T) {
	t.Parallel()
	shapes := map[string]string{
		"printable": "abcdefgh",
		"control":   "a\n\"q\"\t'k'\x1b[31m\x1b[0m\n",
	}
	for name, unit := range shapes {
		for _, size := range []int{0, 4095, 4096, 4097, 64 << 10, 1 << 20} {
			text := promptOfSize(unit, size)
			t.Run(fmt.Sprintf("%s/%d", name, size), func(t *testing.T) {
				t.Parallel()
				p, err := ParseDriverPrompt(text)
				require.NoError(t, err)
				want := strings.TrimSpace(text)
				if hasControlByte(want) {
					want = bracketedPasteStart + want + bracketedPasteEnd
				}
				require.True(t, want == p.Keys(), "keys differ from the expected prompt bytes")
				if want == "" {
					return
				}
				fake := newFakePaneSubmitter()
				require.NoError(t, SubmitDriverContent(context.Background(), fake, newFakeLease(t), p.Keys(), time.Millisecond, 5*time.Second))
				assert.True(t, writtenBeforeEnter(t, fake) == want, "bytes written differ from the prompt")
			})
		}
	}
}

func TestParseDriverPrompt_PasteTerminatorFailsLoudly(t *testing.T) {
	t.Parallel()
	_, err := ParseDriverPrompt("before\x1b[201~echo pwned\n")
	require.ErrorIs(t, err, ErrPromptUndeliverable)
}

func TestSubmitInitialPrompt_UndeliverableWritesNothingAndReleasesLease(t *testing.T) {
	t.Parallel()
	inst := &Instance{UUID: fakeLeaseOwner}
	lease, ok := inst.TryTerminalWriteLease(LeaseWriterDriver)
	require.True(t, ok)
	err := submitInitialPrompt(context.Background(), inst, lease, "x\x1b[201~y")
	require.ErrorIs(t, err, ErrPromptUndeliverable)
	again, ok := inst.TryTerminalWriteLease(LeaseWriterDriver)
	assert.True(t, ok, "lease must be released when the prompt is refused")
	again.Release()
}

func TestSendKeysChunked_PartialFailureReportsProgress(t *testing.T) {
	t.Parallel()
	fake := newFakePaneSubmitter()
	fake.failOnCall = 1
	err := sendKeysChunked(context.Background(), fake, strings.Repeat("a", 3*maxInlinePromptBytes))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 4096 of 12288 bytes")
}

// Real PTY (native backend) with a raw-mode reader: a 100 KiB multi-line prompt
// containing ESC, quotes and tabs must arrive byte for byte.
func TestDriverPrompt_RealPTYReceivesLargePromptIntact(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("requires PTY allocation")
	}
	checkPTYAvailable(t)

	out := filepath.Join(t.TempDir(), "received")
	mgr := NewNativeProcessManager(ProcessManagerOptions{
		SessionName: "driver-prompt-pty",
		Program:     "sh",
		Args:        []string{"-c", "stty raw -echo; exec cat > " + out},
	})
	require.NoError(t, mgr.Start(t.TempDir()))
	t.Cleanup(func() { assert.NoError(t, mgr.Close()) })

	const poll = 20 * time.Millisecond
	require.Eventually(t, func() bool { _, err := os.Stat(out); return err == nil }, 10*time.Second, poll)

	p, err := ParseDriverPrompt(promptOfSize("it's a \"quoted\"\tline \x1b[1mbold\x1b[0m é\n", 100<<10))
	require.NoError(t, err)

	require.NoError(t, sendKeysChunked(context.Background(), ptyKeySender{mgr}, p.Keys()))

	require.Eventually(t, func() bool {
		b, readErr := os.ReadFile(out)
		return readErr == nil && len(b) >= len(p.Keys())
	}, 15*time.Second, poll)
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.True(t, bytes.Equal([]byte(p.Keys()), got), "PTY bytes differ from prompt keys")
}

type ptyKeySender struct{ m *NativeProcessManager }

func (p ptyKeySender) LeaseOwnerUUID() string { return fakeLeaseOwner }
func (p ptyKeySender) SendKeys(k string) error {
	_, err := p.m.SendKeys(k)
	return err
}
