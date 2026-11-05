package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func newTymuxInstanceWithStatus(t *testing.T, status session.Status) *session.Instance {
	t.Helper()
	inst := session.NewStartedInstanceForTest(t, "upgrade", session.NewTymuxBackend(retainedPaintManager{}))
	inst.Backend = session.BackendTymux
	session.SetStatusForTest(t, inst, status, nil)
	return inst
}

func TestUpgradableWriter_should_UpgradeOnce_When_TymuxInstanceBecomesActive(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)
	upgrades := 0
	w.SetOnUpgrade(func() { upgrades++ })

	require.Nil(t, w.Writer(), "read-only while the session is PermanentlyFailed")
	require.Zero(t, upgrades)

	session.SetStatusForTest(t, inst, session.Active, nil)
	require.NotNil(t, w.Writer(), "retry made the session Active; the attach must become read-write")
	require.NotNil(t, w.Writer())
	require.Equal(t, 1, upgrades)
}

func TestUpgradableWriter_should_StayReadOnly_When_InstanceIsArchived(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)

	archivedAt := time.Now()
	session.SetStatusForTest(t, inst, session.Active, &archivedAt)
	require.Nil(t, w.Writer(), "an archived session never upgrades")
}

func TestUpgradableWriter_should_StayReadOnly_When_ReadOnlyForAReasonOtherThanStatus(t *testing.T) {
	t.Parallel()
	archivedAt := time.Now()
	inst := newTymuxInstanceWithStatus(t, session.Active)
	session.SetStatusForTest(t, inst, session.Active, &archivedAt)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)

	session.SetStatusForTest(t, inst, session.Active, nil)
	require.Nil(t, w.Writer(), "archived at attach is permanent even if later unarchived")
}

func TestInputReadLoop_should_ForwardInput_When_WriterUpgradesMidConnection(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)
	var got [][]byte
	p := inputReadLoopParams{
		currentWriter: w.Writer,
		sessionID:     "upgrade",
		onInput:       func(data []byte) { got = append(got, data) },
	}
	frame := &sessionv1.TerminalData{Data: &sessionv1.TerminalData_Input{Input: &sessionv1.TerminalInput{Data: []byte("x")}}}

	require.NoError(t, dispatchInputReadLoopFrame(p, frame))
	require.Empty(t, got, "dropped while read-only")

	session.SetStatusForTest(t, inst, session.Active, nil)
	require.NoError(t, dispatchInputReadLoopFrame(p, frame))
	require.Len(t, got, 1, "forwarded once the session is Active")
}
