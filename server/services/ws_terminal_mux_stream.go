package services

import (
	"errors"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// muxStreamQueueBytes caps one stream's unsent output. WriteMessage blocks (up to the
	// caller's write deadline) rather than buffering past it; a message larger than the
	// cap is admitted only into an empty queue.
	muxStreamQueueBytes = 1 << 20
	// muxInboundMessages / muxInboundBytes bound client -> server messages a handler has
	// not read yet; exceeding either closes the stream with 1013.
	muxInboundMessages = 256
	muxInboundBytes    = 4 << 20
	// muxCloseFlushWait bounds how long a closing stream waits for credit to flush its tail.
	muxCloseFlushWait = 5 * time.Second
)

var errMuxStreamClosed = errors.New("terminal mux stream closed")

type muxOutMessage struct {
	kind     int
	data     []byte
	sent     int
	enqueued time.Time
}

type muxInMessage struct {
	kind int
	data []byte
}

// muxStreamConn is one virtual WebSocket carried by a muxSocket. It implements
// streamConn so the StreamTerminal handlers run over it unchanged.
type muxStreamConn struct {
	id   uint32
	sock *muxSocket

	mu      sync.Mutex
	changed chan struct{} // closed and replaced on any state change (broadcast)

	in        []muxInMessage
	inBytes   int
	partial   []byte
	partKind  int
	readErr   error // set once the stream ends; queued messages drain first
	readDL    time.Time
	writeDL   time.Time
	out       []*muxOutMessage
	outBytes  int
	credit    int
	queued    bool // on the socket's runnable list
	closing   bool // handler returned: flush out, then send CLOSE
	closeCode uint16
	closeSent bool
	ended     bool // no more frames either way
	flushTmr  *time.Timer
}

func newMuxStreamConn(id uint32, sock *muxSocket) *muxStreamConn {
	return &muxStreamConn{id: id, sock: sock, changed: make(chan struct{}), credit: muxStreamWindow}
}

func (s *muxStreamConn) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// waitChanged blocks until the state changes or deadline passes (zero = never).
func waitChanged(ch <-chan struct{}, deadline time.Time) error {
	if deadline.IsZero() {
		<-ch
		return nil
	}
	d := time.Until(deadline)
	if d <= 0 {
		return os.ErrDeadlineExceeded
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ch:
		return nil
	case <-t.C:
		return os.ErrDeadlineExceeded
	}
}

// ReadMessage returns the next complete client message, or the stream's end error once drained.
func (s *muxStreamConn) ReadMessage() (int, []byte, error) {
	for {
		s.mu.Lock()
		if len(s.in) > 0 {
			m := s.in[0]
			s.in[0] = muxInMessage{}
			s.in = s.in[1:]
			s.inBytes -= len(m.data)
			s.mu.Unlock()
			return m.kind, m.data, nil
		}
		if s.readErr != nil {
			err := s.readErr
			s.mu.Unlock()
			return 0, nil, err
		}
		ch, dl := s.changed, s.readDL
		s.mu.Unlock()
		if err := waitChanged(ch, dl); err != nil {
			return 0, nil, err
		}
	}
}

// WriteMessage queues one message on this stream only and returns once it is queued, so a
// slow consumer of another stream cannot block the caller. data is copied.
func (s *muxStreamConn) WriteMessage(kind int, data []byte) error {
	cp := append([]byte(nil), data...)
	for {
		s.mu.Lock()
		if s.ended || s.closing {
			s.mu.Unlock()
			return errMuxStreamClosed
		}
		if s.outBytes == 0 || s.outBytes+len(cp) <= muxStreamQueueBytes {
			s.out = append(s.out, &muxOutMessage{kind: kind, data: cp, enqueued: time.Now()})
			s.outBytes += len(cp)
			depth := s.outBytes
			s.mu.Unlock()
			muxMetrics.queueDepth(depth)
			s.sock.markRunnable(s)
			return nil
		}
		ch, dl := s.changed, s.writeDL
		s.mu.Unlock()
		if err := waitChanged(ch, dl); err != nil {
			return err
		}
	}
}

func (s *muxStreamConn) SetReadDeadline(t time.Time) error {
	s.mu.Lock()
	s.readDL = t
	s.notifyLocked()
	s.mu.Unlock()
	return nil
}

func (s *muxStreamConn) SetWriteDeadline(t time.Time) error {
	s.mu.Lock()
	s.writeDL = t
	s.notifyLocked()
	s.mu.Unlock()
	return nil
}

// Close ends the stream from the handler side: queued output is flushed (bounded by
// muxCloseFlushWait and the client's credit), then CLOSE is sent.
func (s *muxStreamConn) Close() error {
	s.mu.Lock()
	if s.ended || s.closing {
		s.mu.Unlock()
		return nil
	}
	s.closing, s.closeCode = true, websocket.CloseNormalClosure
	if s.readErr == nil {
		s.readErr = errMuxStreamClosed
	}
	s.flushTmr = time.AfterFunc(muxCloseFlushWait, func() { s.abort(websocket.CloseGoingAway) })
	s.notifyLocked()
	s.mu.Unlock()
	s.sock.markRunnable(s)
	return nil
}

// abort drops unsent output and ends the stream, sending CLOSE(code) at once.
func (s *muxStreamConn) abort(code uint16) {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.out, s.outBytes = nil, 0
	s.closing, s.closeCode = true, code
	if s.readErr == nil {
		s.readErr = &websocket.CloseError{Code: int(code)}
	}
	s.notifyLocked()
	s.mu.Unlock()
	s.sock.markRunnable(s)
}

// remoteClose handles a CLOSE from the client: no frame is sent back.
func (s *muxStreamConn) remoteClose(code uint16, reason string) {
	s.endWith(&websocket.CloseError{Code: int(code), Text: reason})
}

// endWith ends the stream without sending anything (socket died or peer closed).
func (s *muxStreamConn) endWith(err error) {
	s.mu.Lock()
	s.endLocked(err)
	s.mu.Unlock()
}

func (s *muxStreamConn) endLocked(err error) {
	if s.ended {
		return
	}
	s.ended, s.closeSent = true, true
	if s.readErr == nil || errors.Is(s.readErr, errMuxStreamClosed) {
		s.readErr = err
	}
	s.out, s.outBytes = nil, 0
	if s.flushTmr != nil {
		s.flushTmr.Stop()
	}
	s.notifyLocked()
}

// deliver appends one client DATA frame, reassembling chunks into messages.
func (s *muxStreamConn) deliver(kind int, fin bool, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended || s.closing {
		return
	}
	if len(s.partial) == 0 {
		s.partKind = kind
	}
	s.partial = append(s.partial, payload...)
	if len(s.partial) > muxMaxMessageBytes {
		s.partial = nil
		go s.abort(websocket.CloseMessageTooBig)
		return
	}
	if !fin {
		return
	}
	if len(s.in) >= muxInboundMessages || s.inBytes+len(s.partial) > muxInboundBytes {
		s.partial = nil
		go s.abort(websocket.CloseTryAgainLater)
		return
	}
	s.in = append(s.in, muxInMessage{kind: s.partKind, data: s.partial})
	s.inBytes += len(s.partial)
	s.partial = nil
	s.notifyLocked()
}

// addCredit grants more send window and reports whether output is now sendable.
func (s *muxStreamConn) addCredit(n int) {
	s.mu.Lock()
	s.credit += n
	pending := len(s.out) > 0
	s.mu.Unlock()
	if pending {
		s.sock.markRunnable(s)
	}
}

// nextFrame builds the next frame for this stream under credit. more reports whether the
// stream should stay runnable; a stalled stream (no credit) is re-queued by addCredit.
func (s *muxStreamConn) nextFrame() (frame []byte, more bool, stalled bool, holWait time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queued = false
	if s.ended {
		return nil, false, false, 0
	}
	if len(s.out) == 0 {
		if s.closing && !s.closeSent {
			s.closeSent = true
			s.endLocked(s.readErr)
			return encodeMuxClose(s.id, s.closeCode, ""), false, false, 0
		}
		return nil, false, false, 0
	}
	head := s.out[0]
	remaining := len(head.data) - head.sent
	n := min(muxChunkBytes, remaining)
	if n > 0 && s.credit <= 0 {
		return nil, false, true, 0
	}
	n = min(n, max(s.credit, 0))
	if remaining == 0 {
		n = 0
	}
	if head.sent == 0 {
		holWait = time.Since(head.enqueued)
	}
	fin := head.sent+n == len(head.data)
	frame = encodeMuxData(s.id, head.kind, fin, head.data[head.sent:head.sent+n])
	head.sent += n
	s.credit -= n
	s.outBytes -= n
	if fin {
		s.out[0] = nil
		s.out = s.out[1:]
	}
	s.notifyLocked() // writers blocked on queue room
	more = len(s.out) > 0 || (s.closing && !s.closeSent)
	return frame, more, false, holWait
}
