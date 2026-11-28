//go:build !windows

package services

// Characterization of StreamTerminal through the public SessionService and the
// Connect wire: frames and their order, access and lifecycle errors, input and
// resize gates, flow control, and handler exit. Hermetic: the pane is an os.Pipe
// behind a fake ProcessManager, so no tmux is needed.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/queue"
)

const charWait = 10 * time.Second

// charPM is a pane backend whose output is the read end of a pipe. Methods not
// listed here are unused by the stream paths and panic through the nil embed.
type charPM struct {
	instance.ProcessManager

	mu         sync.Mutex
	pty        *os.File
	ptyErr     error
	sent       []string
	sendErr    error
	sizes      [][2]int
	resizeErr  error
	capture    string
	captureErr error
}

func (p *charPM) GetPTY() (*os.File, error)    { return p.pty, p.ptyErr }
func (p *charPM) GetSessionIdentifier() string { return "char-pane" }
func (p *charPM) HasSession() bool             { return true }
func (p *charPM) FilterBanners(c string) (string, int) {
	return c, 0
}

func (p *charPM) CapturePaneContent() (string, error) { return p.capture, p.captureErr }

func (p *charPM) SendKeys(keys string) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sendErr != nil {
		return 0, p.sendErr
	}
	p.sent = append(p.sent, keys)
	return len(keys), nil
}

func (p *charPM) SetWindowSize(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.resizeErr != nil {
		return p.resizeErr
	}
	p.sizes = append(p.sizes, [2]int{cols, rows})
	return nil
}

func (p *charPM) writes() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.sent)
}

func (p *charPM) resizes() [][2]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.sizes)
}

// streamExit records what each server-side streaming handler returned.
type streamExit struct{ ch chan error }

func (streamExit) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc { return next }
func (streamExit) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (e streamExit) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		err := next(ctx, conn)
		e.ch <- err
		return err
	}
}

func (e streamExit) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-e.ch:
		return err
	case <-time.After(charWait):
		t.Fatal("streaming handler did not return")
		return nil
	}
}

type charEnv struct {
	svc     *SessionService
	storage *session.Storage
	bus     *events.EventBus
	poller  *session.ReviewQueuePoller
	client  sessionv1connect.SessionServiceClient
	exit    streamExit
	// disconnect drops the client connection, as a closed browser tab does.
	// Cancelling a bidi call's context alone does not reset the stream while
	// the Go client still holds the request body open.
	disconnect func()
}

func newCharEnv(t *testing.T, withPoller bool) *charEnv {
	t.Helper()
	e := &charEnv{storage: svctest.CreateTestStorage(t), bus: events.NewEventBus(64), exit: streamExit{ch: make(chan error, 16)}}
	t.Cleanup(e.bus.Close)
	e.svc = NewSessionService(e.storage, e.bus)
	t.Cleanup(func() { e.svc.Shutdown() })
	if withPoller {
		e.poller = session.NewReviewQueuePoller(queue.NewReviewQueue(), instance.NewInstanceStatusManager(), nil)
		e.svc.SetReviewQueuePoller(e.poller)
	}

	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(e.svc, connect.WithInterceptors(e.exit))
	mux.Handle(path, handler)
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	// LIFO: drops any handler still attached before Close waits for it.
	t.Cleanup(srv.CloseClientConnections)
	e.disconnect = srv.CloseClientConnections
	e.client = sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)
	return e
}

// pollablePipe is a pipe whose read end keeps supporting deadlines after the
// handler calls Fd() on it and dups it. os.Pipe's ends revert to blocking mode
// on Fd(), so a read on the dup would never time out.
func pollablePipe(t *testing.T) (r, w *os.File) {
	t.Helper()
	var fds [2]int
	require.NoError(t, syscall.Pipe(fds[:]))
	require.NoError(t, syscall.SetNonblock(fds[0], true))
	return os.NewFile(uintptr(fds[0]), "pane-out"), os.NewFile(uintptr(fds[1]), "pane-in")
}

// charPane is a started, Active instance whose terminal output is whatever the
// test writes into out.
type charPane struct {
	inst *instance.Instance
	pm   *charPM
	out  *os.File
}

func newCharPane(t *testing.T) charPane {
	t.Helper()
	r, w := pollablePipe(t)
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	pm := &charPM{pty: r}
	inst := instance.NewStartedInstanceForTest(t, "char-"+uuid.NewString()[:8], pm)
	inst.UUID = uuid.NewString()
	instance.SetStatusForTest(t, inst, domain.Active, nil)
	return charPane{inst: inst, pm: pm, out: w}
}

func (p charPane) emit(t *testing.T, s string) {
	t.Helper()
	_, err := p.out.WriteString(s)
	require.NoError(t, err)
}

// charStream is a client stream with a background receiver, so a test can wait
// for a frame with a bound and assert that none arrived.
type charStream struct {
	t      *testing.T
	stream *connect.BidiStreamForClient[sessionv1.TerminalData, sessionv1.TerminalData]
	frames chan *sessionv1.TerminalData
	errs   chan error
}

func (e *charEnv) open(t *testing.T, sessionID string) *charStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := &charStream{t: t, frames: make(chan *sessionv1.TerminalData, 64), errs: make(chan error, 1)}
	s.stream = e.client.StreamTerminal(ctx)
	require.NoError(t, s.stream.Send(&sessionv1.TerminalData{SessionId: sessionID}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			msg, err := s.stream.Receive()
			if err != nil {
				s.errs <- err
				return
			}
			s.frames <- msg
		}
	}()
	t.Cleanup(func() {
		cancel()
		e.disconnect()
		select {
		case <-done:
		case <-time.After(charWait):
			t.Error("client receive loop did not exit")
		}
	})
	return s
}

func (s *charStream) send(msg *sessionv1.TerminalData) {
	s.t.Helper()
	require.NoError(s.t, s.stream.Send(msg))
}

func (s *charStream) next() *sessionv1.TerminalData {
	s.t.Helper()
	select {
	case f := <-s.frames:
		return f
	case err := <-s.errs:
		s.t.Fatalf("stream ended while waiting for a frame: %v", err)
	case <-time.After(charWait):
		s.t.Fatal("timed out waiting for a frame")
	}
	return nil
}

func (s *charStream) endErr() error {
	s.t.Helper()
	select {
	case err := <-s.errs:
		return err
	case <-time.After(charWait):
		s.t.Fatal("timed out waiting for the stream to end")
		return nil
	}
}

func (s *charStream) quiet(d time.Duration) {
	s.t.Helper()
	select {
	case f := <-s.frames:
		s.t.Fatalf("unexpected frame: %v", f)
	case <-time.After(d):
	}
}

// nextOutput reads output frames until want bytes have arrived.
func (s *charStream) nextOutput(sessionID string, want int) string {
	s.t.Helper()
	got := ""
	for len(got) < want {
		f := s.next()
		require.IsType(s.t, &sessionv1.TerminalData_Output{}, f.Data)
		assert.Equal(s.t, sessionID, f.SessionId)
		got += string(f.GetOutput().Data)
	}
	return got
}

func inputFrame(id, data string) *sessionv1.TerminalData {
	return &sessionv1.TerminalData{SessionId: id, Data: &sessionv1.TerminalData_Input{Input: &sessionv1.TerminalInput{Data: []byte(data)}}}
}

func resizeFrame(id string, cols, rows int32) *sessionv1.TerminalData {
	return &sessionv1.TerminalData{SessionId: id, Data: &sessionv1.TerminalData_Resize{Resize: &sessionv1.TerminalResize{Cols: cols, Rows: rows}}}
}

func flowFrame(id string, paused bool) *sessionv1.TerminalData {
	return &sessionv1.TerminalData{SessionId: id, Data: &sessionv1.TerminalData_FlowControl{FlowControl: &sessionv1.FlowControl{Paused: paused}}}
}

func interactionEvents(ch <-chan *events.Event) []*events.Event {
	var out []*events.Event
	for {
		select {
		case ev := <-ch:
			if ev.Type == events.EventUserInteraction {
				out = append(out, ev)
			}
		default:
			return out
		}
	}
}

func TestStreamTerminalChar_ShouldSendOutputFramesInPaneOrderUnderTheRequestedID_When_PaneWrites(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	e.poller.AddInstance(p.inst)

	// The frames echo the ID the client asked with, not the stable ID.
	s := e.open(t, p.inst.UUID)
	p.emit(t, "first\n")
	assert.Equal(t, "first\n", s.nextOutput(p.inst.UUID, 6))
	p.emit(t, "second\n")
	assert.Equal(t, "second\n", s.nextOutput(p.inst.UUID, 7))
	assert.False(t, p.inst.Snapshot().LastTerminalUpdate.IsZero(), "output stamps the terminal timestamps")
	assert.True(t, p.inst.HasTerminalViewers(), "an open stream holds a terminal view")

	e.disconnect()
	e.exit.wait(t)
	assert.False(t, p.inst.HasTerminalViewers(), "the view is released when the handler returns")
}

func TestStreamTerminalChar_ShouldForwardInputResizeAndPublishInteraction_When_Visible(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	e.poller.AddInstance(p.inst)
	evCh, subID := e.bus.Subscribe(context.Background())
	defer e.bus.Unsubscribe(subID)

	id := p.inst.Title
	s := e.open(t, id)
	s.send(resizeFrame(id, 132, 43))
	// Later frames carry their own session_id; writes still go to the attached pane.
	s.send(inputFrame("other-id", "ls\r"))
	s.send(resizeFrame(id, 90, 30))
	require.Eventually(t, func() bool { return len(p.pm.resizes()) == 2 }, charWait, 10*time.Millisecond)
	assert.Equal(t, [][2]int{{132, 43}, {90, 30}}, p.pm.resizes())
	assert.Equal(t, []string{"ls\r"}, p.pm.writes())
	assert.False(t, p.inst.Snapshot().LastMeaningfulOutput.IsZero(), "input stamps meaningful output")

	got := interactionEvents(evCh)
	require.Len(t, got, 1)
	assert.Equal(t, "other-id", got[0].SessionID)
	assert.Equal(t, "terminal_input", got[0].InteractionType)
	assert.Empty(t, got[0].Context)
}

func TestStreamTerminalChar_ShouldDropInputAndResizeButStillStreamOutput_When_Hidden(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	instance.SetHiddenForTest(t, p.inst, true)
	e.poller.AddInstance(p.inst)
	evCh, subID := e.bus.Subscribe(context.Background())
	defer e.bus.Unsubscribe(subID)

	id := p.inst.Title
	s := e.open(t, id)
	s.send(inputFrame(id, "rm -rf\r"))
	s.send(resizeFrame(id, 132, 43))
	// Dropped frames leave no pane effect, so a second input proves the resize
	// before it was consumed.
	s.send(inputFrame(id, "again"))
	require.Eventually(t, func() bool { return inputRow(id).ByPath[access.PathStream.String()].Received == 2 }, charWait, 10*time.Millisecond)
	p.emit(t, "still streaming")
	assert.Equal(t, "still streaming", s.nextOutput(id, 15))
	s.quiet(100 * time.Millisecond)
	assert.Empty(t, p.pm.writes())
	assert.Empty(t, p.pm.resizes())
	assert.Empty(t, interactionEvents(evCh))
	assertPathCounts(t, id, access.PathStream, 2, 0)
}

func TestStreamTerminalChar_ShouldReportResizeErrorAndContinue_When_ResizeFails(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	p.pm.resizeErr = errors.New("boom")
	e.poller.AddInstance(p.inst)

	id := p.inst.Title
	s := e.open(t, id)
	s.send(resizeFrame("r1", 10, 10))
	f := s.next()
	assert.Equal(t, "r1", f.SessionId)
	assert.Equal(t, "RESIZE_ERROR", f.GetError().GetCode())
	assert.Equal(t, "Failed to resize terminal: failed to resize terminal: boom", f.GetError().GetMessage())
	s.send(resizeFrame("r2", 0, 0))
	f = s.next()
	assert.Equal(t, "Failed to resize terminal: invalid terminal size 0x0", f.GetError().GetMessage())
	s.send(inputFrame(id, "after"))
	require.Eventually(t, func() bool { return slices.Equal(p.pm.writes(), []string{"after"}) }, charWait, 10*time.Millisecond)
}

func TestStreamTerminalChar_ShouldSendWriteErrorThenCodeInternal_When_PaneWriteFails(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	p.pm.sendErr = errors.New("boom")
	e.poller.AddInstance(p.inst)
	evCh, subID := e.bus.Subscribe(context.Background())
	defer e.bus.Unsubscribe(subID)

	id := p.inst.Title
	s := e.open(t, id)
	s.send(inputFrame("w1", "x"))
	f := s.next()
	assert.Equal(t, "w1", f.SessionId)
	assert.Equal(t, "WRITE_ERROR", f.GetError().GetCode())
	assert.Equal(t, "Failed to write to PTY: boom", f.GetError().GetMessage())
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(e.exit.wait(t)))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(s.endErr()))
	assert.False(t, p.inst.HasTerminalViewers())
	assert.Empty(t, interactionEvents(evCh), "a failed write publishes no interaction")
}

func TestStreamTerminalChar_ShouldHoldOutputUntilResumed_When_ClientPauses(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	e.poller.AddInstance(p.inst)

	id := p.inst.Title
	s := e.open(t, id)
	p.emit(t, "a")
	assert.Equal(t, "a", s.nextOutput(id, 1))
	s.send(flowFrame(id, true))
	// The reader notices a pause within one 250ms read deadline.
	s.quiet(time.Second)
	p.emit(t, "held")
	s.quiet(750 * time.Millisecond)
	s.send(flowFrame(id, false))
	assert.Equal(t, "held", s.nextOutput(id, 4), "paused output is delivered, not dropped")
}

func TestStreamTerminalChar_ShouldIgnoreCurrentPaneAndErrorFrames_When_Received(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	e.poller.AddInstance(p.inst)

	id := p.inst.Title
	s := e.open(t, id)
	s.send(&sessionv1.TerminalData{SessionId: id, Data: &sessionv1.TerminalData_CurrentPaneRequest{CurrentPaneRequest: &sessionv1.CurrentPaneRequest{}}})
	s.send(&sessionv1.TerminalData{SessionId: id, Data: &sessionv1.TerminalData_Error{Error: &sessionv1.TerminalError{Message: "client side", Code: "X"}}})
	s.send(inputFrame(id, "z"))
	require.Eventually(t, func() bool { return len(p.pm.writes()) == 1 }, charWait, 10*time.Millisecond)
	s.quiet(100 * time.Millisecond)
	assert.Empty(t, p.pm.resizes())
}

// A pane EOF does not end the stream. A client half-close does, and today as
// CodeInternal: connect reports it as "unknown: EOF", which the handler's
// EOF strings do not match.
func TestStreamTerminalChar_ShouldOutlivePaneEOFAndEndInternal_When_ClientHalfCloses(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	e.poller.AddInstance(p.inst)

	id := p.inst.Title
	s := e.open(t, id)
	p.emit(t, "bye")
	assert.Equal(t, "bye", s.nextOutput(id, 3))
	require.NoError(t, p.out.Close())
	s.send(inputFrame(id, "still-writable"))
	require.Eventually(t, func() bool { return len(p.pm.writes()) == 1 }, charWait, 10*time.Millisecond)
	select {
	case err := <-e.exit.ch:
		t.Fatalf("handler returned on pane EOF: %v", err)
	case <-time.After(750 * time.Millisecond):
	}
	assert.True(t, p.inst.HasTerminalViewers())

	require.NoError(t, s.stream.CloseRequest())
	err := e.exit.wait(t)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.ErrorContains(t, err, "stream receive error: unknown: EOF")
	assert.False(t, p.inst.HasTerminalViewers())
}

func TestStreamTerminalChar_ShouldReturnTheDocumentedCode_When_AttachIsRefused(t *testing.T) {
	t.Parallel()
	status := func(st domain.Status, archived bool) func(*testing.T, charPane) {
		return func(t *testing.T, p charPane) {
			var at *time.Time
			if archived {
				now := time.Now()
				at = &now
			}
			instance.SetStatusForTest(t, p.inst, st, at)
		}
	}
	cases := []struct {
		name    string
		prepare func(*testing.T, charPane)
		id      string // "" means the pane's title
		code    connect.Code
		message string
	}{
		{name: "unknown session", id: "nope", code: connect.CodeNotFound, message: "session not found: nope"},
		{name: "not started", prepare: func(t *testing.T, p charPane) {
			status(domain.Stopped, false)(t, p)
			p.inst.SetStartedForTest(false)
		}, code: connect.CodeFailedPrecondition, message: "session not started"},
		{name: "paused", prepare: status(domain.Paused, false), code: connect.CodeFailedPrecondition, message: "session is paused"},
		{name: "auto-hibernating", prepare: func(_ *testing.T, p charPane) { p.inst.SetTerminalViewersForTest(-1) },
			code: connect.CodeUnavailable, message: "session is being automatically hibernated; reconnect to resume"},
		{name: "archived hibernated cannot resume", prepare: status(domain.Hibernated, true), code: connect.CodeUnavailable,
			message: `failed to resume hibernated session`},
		{name: "no PTY", prepare: func(_ *testing.T, p charPane) { p.pm.ptyErr = errors.New("gone") }, code: connect.CodeInternal,
			message: "failed to get PTY reader: gone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newCharEnv(t, true)
			p := newCharPane(t)
			if tc.prepare != nil {
				tc.prepare(t, p)
			}
			e.poller.AddInstance(p.inst)
			id := tc.id
			if id == "" {
				id = p.inst.Title
			}
			s := e.open(t, id)
			err := e.exit.wait(t)
			assert.Equal(t, tc.code, connect.CodeOf(err))
			assert.ErrorContains(t, err, tc.message)
			assert.Equal(t, tc.code, connect.CodeOf(s.endErr()))
			if tc.name != "auto-hibernating" {
				assert.False(t, p.inst.HasTerminalViewers(), "a refused attach releases its view")
			}
		})
	}
}

func TestStreamTerminalChar_ShouldRejectTheFirstMessage_When_ItNamesNoSession(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)

	s := e.open(t, "")
	err := e.exit.wait(t)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.ErrorContains(t, err, "session_id is required")
	_ = s.endErr()

	ctx, cancel := context.WithTimeout(context.Background(), charWait)
	defer cancel()
	empty := e.client.StreamTerminal(ctx)
	require.NoError(t, empty.Send(nil))
	require.NoError(t, empty.CloseRequest())
	err = e.exit.wait(t)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.ErrorContains(t, err, "failed to receive initial message")
}

func TestStreamTerminalChar_ShouldWaitForStart_When_ActiveButNotStarted(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	p.inst.SetStartedForTest(false)
	e.poller.AddInstance(p.inst)

	id := p.inst.Title
	s := e.open(t, id)
	p.emit(t, "late")
	s.quiet(500 * time.Millisecond)
	p.inst.MarkStarted()
	assert.Equal(t, "late", s.nextOutput(id, 4))
}

func TestStreamTerminalChar_ShouldEndWithUnavailable_When_CancelledWhileWaitingForStart(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	p.inst.SetStartedForTest(false)
	e.poller.AddInstance(p.inst)

	e.open(t, p.inst.Title)
	require.Eventually(t, p.inst.HasTerminalViewers, charWait, 10*time.Millisecond)
	e.disconnect()
	err := e.exit.wait(t)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	assert.ErrorContains(t, err, "wait for terminal resume")
	assert.False(t, p.inst.HasTerminalViewers())
}

// Without a poller, or when the poller does not know the session, the
// instance comes from storage.
func TestStreamTerminalChar_ShouldFallBackToStorage_When_PollerDoesNotKnowTheSession(t *testing.T) {
	t.Parallel()
	for _, withPoller := range []bool{false, true} {
		e := newCharEnv(t, withPoller)
		stored := instance.NewPausedTestInstance("stored-" + uuid.NewString()[:8])
		require.NoError(t, e.storage.AddInstance(stored))

		e.open(t, stored.Title)
		err := e.exit.wait(t)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err), "withPoller=%v", withPoller)
		assert.ErrorContains(t, err, "session is paused", "withPoller=%v", withPoller)
	}
}

// A poller instance wins over a stored one of the same title: its live state is
// what the review queue sees.
func TestStreamTerminalChar_ShouldPreferThePollerInstance_When_StorageHasTheSameTitle(t *testing.T) {
	t.Parallel()
	e := newCharEnv(t, true)
	p := newCharPane(t)
	stored := instance.NewPausedTestInstance(p.inst.Title)
	require.NoError(t, e.storage.AddInstance(stored))
	e.poller.AddInstance(p.inst)

	id := p.inst.Title
	s := e.open(t, id)
	p.emit(t, "live")
	assert.Equal(t, "live", s.nextOutput(id, 4))
}
