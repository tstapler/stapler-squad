package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// testWriter returns a read-write TerminalWriter on the real subprocess seam.
func testWriter() TerminalWriter {
	w, err := TerminalReadWrite.Writer(nil)
	if err != nil {
		panic(err)
	}
	return w
}

// fakeTmuxSender counts calls on the tmuxInputSender seam.
type fakeTmuxSender struct {
	mu      sync.Mutex
	inputs  [][]byte
	resizes [][2]int
}

func (f *fakeTmuxSender) SendInput(_, _ string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, append([]byte(nil), data...))
	return nil
}

func (f *fakeTmuxSender) Resize(_ capturePaneStreamParams, cols, rows int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, [2]int{cols, rows})
}

func (f *fakeTmuxSender) counts() (inputs, resizes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inputs), len(f.resizes)
}

func hiddenInstance(t *testing.T, hidden bool) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   "access-" + t.Name(),
		Path:    t.TempDir(),
		Program: "claude",
		Hidden:  hidden,
	})
	require.NoError(t, err)
	return inst
}

// T-RO-06
func TestAccessFor_ShouldReturnReadOnlyForHiddenAndReadWriteForVisibleViaSnapshotOnly(t *testing.T) {
	assert.Equal(t, TerminalReadOnly, AccessFor(hiddenInstance(t, true)))
	assert.Equal(t, TerminalReadWrite, AccessFor(hiddenInstance(t, false)))
	assert.Equal(t, TerminalReadOnly, AccessFor(nil), "a missing instance fails closed")
	var zero TerminalAccess
	assert.Equal(t, TerminalReadOnly, zero, "the zero value fails closed")
}

// T-RO-07
func TestWriter_ShouldReturnErrReadOnly_WhenAccessReadOnly(t *testing.T) {
	w, err := TerminalReadOnly.Writer(nil)
	assert.Nil(t, w)
	assert.True(t, errors.Is(err, ErrReadOnly))

	w, err = TerminalReadWrite.Writer(nil)
	require.NoError(t, err)
	assert.NotNil(t, w)
}

type recordingControlMode struct {
	calls int
	err   error
}

func (r *recordingControlMode) SendInputViaControlMode(context.Context, []byte) error {
	r.calls++
	return r.err
}

func TestPaneWriter_ShouldFallBackToTheSenderSeam_WhenControlModeFails(t *testing.T) {
	sender := &fakeTmuxSender{}
	w, err := TerminalReadWrite.Writer(sender)
	require.NoError(t, err)

	cm := &recordingControlMode{}
	require.NoError(t, w.SendInput(context.Background(), PaneInput{ControlMode: cm, Data: []byte("a")}))
	inputs, _ := sender.counts()
	assert.Equal(t, 0, inputs, "control mode succeeded; no subprocess")

	cm.err = errors.New("queue full")
	require.NoError(t, w.SendInput(context.Background(), PaneInput{ControlMode: cm, Data: []byte("b")}))
	inputs, _ = sender.counts()
	assert.Equal(t, 1, inputs)
}
