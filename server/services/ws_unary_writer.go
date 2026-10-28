package services

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// unaryWriteWait bounds one frame write so a client that stops reading cannot
// pin the writer (and its in-flight slots) forever.
const unaryWriteWait = 10 * time.Second

// unaryReadyLimit bounds queued small frames; exceeding it means the peer is not
// reading, and the socket is closed rather than buffering without limit.
const unaryReadyLimit = 1024

// unaryQueuedBytesLimit bounds unsent CHUNK bytes per socket; each tail can be
// up to wsUnaryMaxResponseBytes, so the frame-count bound alone does not cap memory.
const unaryQueuedBytesLimit = 64 << 20

var errUnaryWriterOverflow = errors.New("unary writer queue full")

// chunkStream is the unsent tail of a large response, sent one CHUNK at a time.
type chunkStream struct {
	id      uint32
	payload []byte
	onDone  func() // called once, outside w.mu, when the last CHUNK is popped or the stream is dropped
}

// unaryWriter is the socket's single writer. Whole frames (RESPONSE heads,
// ERRORs, PONGs) have priority over CHUNK frames, and CHUNKs of different ids
// are interleaved round-robin, so a small response never waits for a large one.
type unaryWriter struct {
	conn *websocket.Conn

	mu      sync.Mutex
	ready   [][]byte
	streams []*chunkStream
	queued  int // unsent bytes across streams
	notify  chan struct{}
}

func newUnaryWriter(conn *websocket.Conn) *unaryWriter {
	return &unaryWriter{conn: conn, notify: make(chan struct{}, 1)}
}

func (w *unaryWriter) wake() {
	select {
	case w.notify <- struct{}{}:
	default:
	}
}

// enqueue queues a complete frame; it fails when the bound is exceeded.
func (w *unaryWriter) enqueue(frame []byte) error {
	w.mu.Lock()
	if len(w.ready) >= unaryReadyLimit {
		w.mu.Unlock()
		return errUnaryWriterOverflow
	}
	w.ready = append(w.ready, frame)
	w.mu.Unlock()
	w.wake()
	return nil
}

// enqueueResponse frames a fully buffered payload: one RESPONSE when it fits in
// maxFrameBytes, otherwise a RESPONSE head (fin=0) plus round-robin CHUNKs.
// When streamed is true, onDone runs later (last CHUNK popped, or drop); when
// false (single frame or error) the caller owns cleanup and onDone never runs.
func (w *unaryWriter) enqueueResponse(id uint32, meta unaryResponseMeta, payload []byte, onDone func()) (streamed bool, err error) {
	total := uint32(len(payload))
	probe, err := encodeUnaryResponse(id, total, false, meta, nil)
	if err != nil {
		return false, err
	}
	room := unaryMaxFrameBytes - len(probe)
	if room <= 0 {
		return false, errors.New("response meta exceeds frame size")
	}
	if len(payload) <= room {
		head, err := encodeUnaryResponse(id, total, true, meta, payload)
		if err != nil {
			return false, err
		}
		return false, w.enqueue(head)
	}
	head, err := encodeUnaryResponse(id, total, false, meta, payload[:room])
	if err != nil {
		return false, err
	}
	w.mu.Lock()
	if len(w.ready) >= unaryReadyLimit || w.queued+len(payload)-room > unaryQueuedBytesLimit {
		w.mu.Unlock()
		return false, errUnaryWriterOverflow
	}
	w.ready = append(w.ready, head)
	w.queued += len(payload) - room
	w.streams = append(w.streams, &chunkStream{id: id, payload: payload[room:], onDone: onDone})
	w.mu.Unlock()
	w.wake()
	return true, nil
}

// drop discards the unsent CHUNKs of id (CANCEL after the RESPONSE head).
func (w *unaryWriter) drop(id uint32) {
	var done []func()
	w.mu.Lock()
	kept := w.streams[:0]
	for _, s := range w.streams {
		if s.id != id {
			kept = append(kept, s)
		} else {
			w.queued -= len(s.payload)
			if s.onDone != nil {
				done = append(done, s.onDone)
			}
		}
	}
	w.streams = kept
	w.mu.Unlock()
	for _, f := range done {
		f()
	}
}

// next pops the next frame to write, or nil when idle.
func (w *unaryWriter) next() []byte {
	frame, onDone := w.pop()
	if onDone != nil {
		onDone()
	}
	return frame
}

func (w *unaryWriter) pop() (frame []byte, onDone func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.ready) > 0 {
		f := w.ready[0]
		w.ready[0] = nil
		w.ready = w.ready[1:]
		return f, nil
	}
	if len(w.streams) == 0 {
		return nil, nil
	}
	s := w.streams[0]
	n := unaryMaxFrameBytes - unaryFrameHeaderLen - 1
	fin := len(s.payload) <= n
	if fin {
		n = len(s.payload)
	}
	frame = encodeUnaryChunk(s.id, fin, s.payload[:n])
	s.payload = s.payload[n:]
	w.queued -= n
	w.streams = w.streams[1:]
	if !fin {
		w.streams = append(w.streams, s)
	} else {
		onDone = s.onDone
	}
	return frame, onDone
}

// run writes frames until ctx ends or a write fails.
func (w *unaryWriter) run(ctx context.Context) error {
	for {
		frame := w.next()
		if frame == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-w.notify:
			}
			continue
		}
		if err := w.conn.SetWriteDeadline(time.Now().Add(unaryWriteWait)); err != nil {
			return err
		}
		if err := w.conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return err
		}
	}
}
