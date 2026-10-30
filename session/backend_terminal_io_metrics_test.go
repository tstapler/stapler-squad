package session

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/tymux"
)

var errIOFake = errors.New("io fake failure")

// ioTmuxManager/ioTymuxManager stub only the hot-path terminal I/O methods
// (the embedded nil manager panics on anything else, as in this package's
// other fakes).
type ioTmuxManager struct {
	TmuxManager
	err error
}

func (m *ioTmuxManager) SendKeys(string) (int, error) { return 3, m.err }
func (m *ioTmuxManager) SendInputViaControlMode(context.Context, []byte) error {
	return m.err
}
func (m *ioTmuxManager) CapturePaneContent() (string, error)    { return "c", m.err }
func (m *ioTmuxManager) CapturePaneContentRaw() (string, error) { return "r", m.err }
func (m *ioTmuxManager) CapturePaneContentWithOptions(string, string) (string, error) {
	return "o", m.err
}
func (m *ioTmuxManager) CaptureViewport(int) (string, error)    { return "v", m.err }
func (m *ioTmuxManager) SetWindowSize(int, int) error           { return m.err }
func (m *ioTmuxManager) SetDetachedSize(int, int, string) error { return m.err }

type ioTymuxManager struct {
	tymux.TymuxManager
	err error
}

func (m *ioTymuxManager) SendKeys(string) (int, error) { return 3, m.err }
func (m *ioTymuxManager) SendInputViaControlMode(context.Context, []byte) error {
	return m.err
}
func (m *ioTymuxManager) CapturePaneContent() (string, error)    { return "c", m.err }
func (m *ioTymuxManager) CapturePaneContentRaw() (string, error) { return "r", m.err }
func (m *ioTymuxManager) CapturePaneContentWithOptions(string, string) (string, error) {
	return "o", m.err
}
func (m *ioTymuxManager) CaptureViewport(int) (string, error)    { return "v", m.err }
func (m *ioTymuxManager) SetWindowSize(int, int) error           { return m.err }
func (m *ioTymuxManager) SetDetachedSize(int, int, string) error { return m.err }

// terminalIOBackend is the slice of ProcessManager both wrappers share here.
type terminalIOBackend interface {
	SendKeys(string) (int, error)
	SendInputViaControlMode(context.Context, []byte) error
	CapturePaneContent() (string, error)
	CapturePaneContentRaw() (string, error)
	CapturePaneContentWithOptions(string, string) (string, error)
	CaptureViewport(int) (string, error)
	SetWindowSize(int, int) error
	SetDetachedSize(int, int, string) error
}

func TestTerminalIO_RecordsLatencyPerBackendAndOperation(t *testing.T) {
	cases := []struct {
		label   backendLabel
		backend terminalIOBackend
	}{
		{backendLabelTmux, NewTmuxBackend(&ioTmuxManager{})},
		{backendLabelTymux, NewTymuxBackend(&ioTymuxManager{})},
	}
	for _, tc := range cases {
		t.Run(string(tc.label), func(t *testing.T) {
			count := func(op string) uint64 {
				return sumBackendHistogramCount(t, collectBackendDurationMetric(t), tc.label, op)
			}
			before := map[string]uint64{}
			for _, op := range []string{opSendKeys, opWriteStdin, opCapturePane, opResizeWindow} {
				before[op] = count(op)
			}

			n, err := tc.backend.SendKeys("abc")
			require.NoError(t, err)
			assert.Equal(t, 3, n, "wrapper must pass the byte count through")
			require.NoError(t, tc.backend.SendInputViaControlMode(context.Background(), []byte("x")))
			_, err = tc.backend.CapturePaneContent()
			require.NoError(t, err)
			_, err = tc.backend.CapturePaneContentRaw()
			require.NoError(t, err)
			_, err = tc.backend.CapturePaneContentWithOptions("-", "-")
			require.NoError(t, err)
			_, err = tc.backend.CaptureViewport(10)
			require.NoError(t, err)
			require.NoError(t, tc.backend.SetWindowSize(80, 24))
			require.NoError(t, tc.backend.SetDetachedSize(80, 24, "t"))

			assert.Equal(t, before[opSendKeys]+1, count(opSendKeys))
			assert.Equal(t, before[opWriteStdin]+1, count(opWriteStdin))
			assert.Equal(t, before[opCapturePane]+4, count(opCapturePane), "all four capture variants share one series")
			assert.Equal(t, before[opResizeWindow]+2, count(opResizeWindow))
		})
	}
}

func TestTerminalIO_PassesErrorsThrough(t *testing.T) {
	b := NewTymuxBackend(&ioTymuxManager{err: errIOFake})
	_, err := b.SendKeys("x")
	assert.ErrorIs(t, err, errIOFake)
	assert.ErrorIs(t, b.SetWindowSize(1, 1), errIOFake)
}

func BenchmarkTimedBackendOp(b *testing.B) {
	m := NewTmuxBackend(&ioTmuxManager{})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = m.SendKeys("x")
	}
}
