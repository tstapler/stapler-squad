package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/streamhub"
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
	first := w.Writer()
	require.NotNil(t, first, "retry made the session Active; the attach must become read-write")
	require.Same(t, first, w.Writer(), "the upgraded writer is kept, not rebuilt")
	require.Equal(t, 1, upgrades)
}

func TestUpgradableWriter_should_UpgradeFromAnyNonActiveStatus(t *testing.T) {
	t.Parallel()
	for _, from := range []session.Status{session.Paused, session.Stopped, session.Crashed, session.Failed} {
		t.Run(from.String(), func(t *testing.T) {
			t.Parallel()
			inst := newTymuxInstanceWithStatus(t, from)
			w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)
			require.Nil(t, w.Writer())
			session.SetStatusForTest(t, inst, session.Active, nil)
			require.NotNil(t, w.Writer())
		})
	}
}

func TestUpgradableWriter_should_StayReadOnly_When_InstanceIsArchived(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)

	archivedAt := time.Now()
	session.SetStatusForTest(t, inst, session.Active, &archivedAt)
	require.Nil(t, w.Writer(), "an archived session never upgrades")
}

func TestUpgradableWriter_should_NeverUpgrade_When_ArchivedAtAttach(t *testing.T) {
	t.Parallel()
	archivedAt := time.Now()
	inst := newTymuxInstanceWithStatus(t, session.Active)
	session.SetStatusForTest(t, inst, session.Active, &archivedAt)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)

	session.SetStatusForTest(t, inst, session.Active, nil)
	require.Nil(t, w.Writer(), "archived at attach is permanent even if later unarchived")
}

func TestUpgradableWriter_should_StayReadOnly_When_SessionBecomesHiddenBeforeUpgrade(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)

	session.SetHiddenForTest(t, inst, true)
	session.SetStatusForTest(t, inst, session.Active, nil)
	require.Nil(t, w.Writer(), "a hidden session must never gain write access")
}

func TestUpgradableWriter_should_UpgradeExactlyOnce_When_WriterIsCalledConcurrently(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)
	var mu sync.Mutex
	upgrades := 0
	w.SetOnUpgrade(func() { mu.Lock(); upgrades++; mu.Unlock() })
	session.SetStatusForTest(t, inst, session.Active, nil)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = w.Writer() }()
	}
	wg.Wait()
	<-w.Upgraded()
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, upgrades)
}

func TestInputReadLoop_should_ForwardInput_When_WriterUpgradesMidConnection(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)
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

func TestInputReadLoop_should_RecordResize_When_FrameIsDroppedWhileReadOnly(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)
	p := inputReadLoopParams{currentWriter: w.Writer, onDroppedResize: w.NoteDroppedResize, sessionID: "upgrade"}
	frame := &sessionv1.TerminalData{Data: &sessionv1.TerminalData_Resize{Resize: &sessionv1.TerminalResize{Cols: 132, Rows: 43}}}

	_, _, ok := w.LastDroppedResize()
	require.False(t, ok)
	require.NoError(t, dispatchInputReadLoopFrame(p, frame))
	cols, rows, ok := w.LastDroppedResize()
	require.True(t, ok)
	require.Equal(t, [2]int{132, 43}, [2]int{cols, rows})
}

// attachReadOnly builds a hub with one read-only subscriber for the wiring tests.
func attachReadOnly(t *testing.T) (*streamhub.StreamHub, streamhub.SubscriberID) {
	t.Helper()
	hub := streamhub.NewStreamHub("wire", nil, streamhub.WithTeardownGrace(time.Hour))
	t.Cleanup(func() { _ = hub.ForceTeardown() })
	return hub, hub.AttachSubscriber(noopHubTransport{}, streamhub.ReadOnlyCapability())
}

type noopHubTransport struct{}

func (noopHubTransport) Send([]byte) error { return nil }
func (noopHubTransport) Close() error      { return nil }

func handshakeReq(cols, rows int32) *sessionv1.CurrentPaneRequest {
	return &sessionv1.CurrentPaneRequest{TargetCols: &cols, TargetRows: &rows}
}

func waitUpgraded(t *testing.T, w *upgradableWriter) {
	t.Helper()
	select {
	case <-w.Upgraded():
	case <-time.After(10 * time.Second):
		t.Fatal("writer was never upgraded")
	}
}

func TestWireWriterUpgrade_should_UpgradeSubscriberAndVote_When_StatusFlipsWithNoClientFrame(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)
	hub, id := attachReadOnly(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h.wireWriterUpgrade(ctx, writerUpgrade{hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(100, 30), bus: bus})

	session.SetStatusForTest(t, inst, session.Active, nil)
	bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))

	waitUpgraded(t, w)
	got := hub.NegotiatedSize()
	require.Equal(t, [2]int{100, 30}, [2]int{got.Cols(), got.Rows()}, "upgrade must cast the handshake vote")
}

func TestWireWriterUpgrade_should_VoteWithLatestDroppedResize_When_OneWasRecorded(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)
	hub, id := attachReadOnly(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h.wireWriterUpgrade(ctx, writerUpgrade{hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(100, 30), bus: bus})
	w.NoteDroppedResize(160, 50)

	session.SetStatusForTest(t, inst, session.Active, nil)
	bus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status"}))

	waitUpgraded(t, w)
	got := hub.NegotiatedSize()
	require.Equal(t, [2]int{160, 50}, [2]int{got.Cols(), got.Rows()}, "the stale connect-time size must not win")
}

func TestWireWriterUpgrade_should_UpgradeImmediately_When_StatusFlippedBeforeWiring(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)
	hub, id := attachReadOnly(t)
	session.SetStatusForTest(t, inst, session.Active, nil)

	h.wireWriterUpgrade(context.Background(), writerUpgrade{hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(90, 20)})

	waitUpgraded(t, w)
	got := hub.NegotiatedSize()
	require.Equal(t, [2]int{90, 20}, [2]int{got.Cols(), got.Rows()})
}
