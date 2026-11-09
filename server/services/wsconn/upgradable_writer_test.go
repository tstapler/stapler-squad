package wsconn

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
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
	w.SetOnUpgrade(func(services.TerminalWriter) { upgrades++ })

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

func TestUpgradableWriter_should_StayReadOnlyWhileArchived_ThenUpgradeOnUnarchive(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.Active)
	archivedAt := time.Now()
	session.SetStatusForTest(t, inst, session.Active, &archivedAt)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)
	require.Nil(t, w.Writer(), "archived attach is read-only")

	session.SetStatusForTest(t, inst, session.Active, &archivedAt)
	require.Nil(t, w.Writer(), "still archived: stays read-only")

	session.SetStatusForTest(t, inst, session.Active, nil)
	require.NotNil(t, w.Writer(), "the UI shows the terminal writable after unarchive, so the stream must too")
}

func TestUpgradableWriter_should_StayReadOnly_When_UnarchivedButNotActive(t *testing.T) {
	t.Parallel()
	archivedAt := time.Now()
	inst := newTymuxInstanceWithStatus(t, session.Stopped)
	session.SetStatusForTest(t, inst, session.Stopped, &archivedAt)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)

	session.SetStatusForTest(t, inst, session.Stopped, nil)
	require.Nil(t, w.Writer())
	session.SetStatusForTest(t, inst, session.Active, nil)
	require.NotNil(t, w.Writer())
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
	w.SetOnUpgrade(func(services.TerminalWriter) { mu.Lock(); upgrades++; mu.Unlock() })
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
	w.WaitVotes()
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
	_ = h.wireWriterUpgrade(ctx, writerUpgrade{hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(100, 30), bus: bus})

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
	_ = h.wireWriterUpgrade(ctx, writerUpgrade{hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(100, 30), bus: bus})
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

	_ = h.wireWriterUpgrade(context.Background(), writerUpgrade{hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(90, 20)})

	waitUpgraded(t, w)
	got := hub.NegotiatedSize()
	require.Equal(t, [2]int{90, 20}, [2]int{got.Cols(), got.Rows()})
}

func TestUpgradableWriter_should_NotExposeWriterToOtherCallers_UntilUpgradeCallbackFinishes(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	w := NewConnectRPCWebSocketHandler(nil, nil, nil).newUpgradableWriter(inst)
	entered, release := make(chan struct{}), make(chan struct{})
	w.SetOnUpgrade(func(services.TerminalWriter) { close(entered); <-release })
	session.SetStatusForTest(t, inst, session.Active, nil)

	first := make(chan services.TerminalWriter, 1)
	go func() { first <- w.Writer() }()
	<-entered
	require.Nil(t, w.Writer(), "input must not be forwarded before UpgradeSubscriber has run")
	close(release)
	require.NotNil(t, <-first)
	require.NotNil(t, w.Writer())
}

func TestWireWriterUpgrade_should_NotBlockReadLoop_When_ResizeVoteIsSlow(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)
	hub, id := attachReadOnly(t)
	entered, release, voted := make(chan struct{}), make(chan struct{}), make(chan [2]int, 1)
	_ = h.wireWriterUpgrade(context.Background(), writerUpgrade{
		hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(100, 30),
		vote: func(_ context.Context, _ services.TerminalWriter, cols, rows int) {
			close(entered)
			<-release // a hung SetWindowSize / CapturePaneContentRaw
			voted <- [2]int{cols, rows}
		},
	})
	session.SetStatusForTest(t, inst, session.Active, nil)

	got := make(chan services.TerminalWriter, 1)
	go func() { got <- w.Writer() }() // the input read loop triggering the upgrade
	select {
	case writer := <-got:
		require.NotNil(t, writer, "the first input after the upgrade must be forwarded, not dropped")
	case <-time.After(10 * time.Second):
		t.Fatal("read loop blocked on the resize vote")
	}
	<-entered
	require.NotNil(t, w.Writer(), "concurrent callers see the writer while the vote is in flight")
	close(release)
	require.Equal(t, [2]int{100, 30}, <-voted)
}

func TestWireWriterUpgrade_should_UpgradeCapabilityBeforeExposingWriter(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)
	hub, id := attachReadOnly(t)
	size := mustSize(t, 90, 20)
	_ = h.wireWriterUpgrade(context.Background(), writerUpgrade{
		hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(90, 20),
		vote: func(_ context.Context, _ services.TerminalWriter, _, _ int) {},
	})
	session.SetStatusForTest(t, inst, session.Active, nil)
	require.NotNil(t, w.Writer())

	hub.RequestResize(context.Background(), id, size)
	require.Equal(t, size, hub.NegotiatedSize(), "once the writer is exposed the subscriber can already vote")
}

func mustSize(t *testing.T, cols, rows int) streamhub.TerminalSize {
	t.Helper()
	size, err := streamhub.NewTerminalSize(cols, rows)
	require.NoError(t, err)
	return size
}

func TestWireWriterUpgrade_should_VoteResizeRecordedAfterUpgrade(t *testing.T) {
	t.Parallel()
	inst := newTymuxInstanceWithStatus(t, session.PermanentlyFailed)
	h := NewConnectRPCWebSocketHandler(nil, nil, nil)
	w := h.newUpgradableWriter(inst)
	hub, id := attachReadOnly(t)
	_ = h.wireWriterUpgrade(context.Background(), writerUpgrade{hub: hub, id: id, writer: w, instance: inst, handshake: handshakeReq(100, 30)})
	session.SetStatusForTest(t, inst, session.Active, nil)
	require.NotNil(t, w.Writer())
	waitUpgraded(t, w)

	// The read loop saw "read-only" just before the upgrade, then recorded its resize after it.
	w.NoteDroppedResize(160, 50)
	w.WaitVotes()

	got := hub.NegotiatedSize()
	require.Equal(t, [2]int{160, 50}, [2]int{got.Cols(), got.Rows()}, "a resize dropped in the upgrade race must still be voted")
}
