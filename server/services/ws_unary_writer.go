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

var errUnaryWriterOverflow = errors.New("unary writer queue full")

// chunkStream is the unsent tail of a large response, sent one CHUNK at a time.
type chunkStream struct {
	id      uint32
	payload []byte
}

// unaryWriter is the socket's single writer. Whole frames (RESPONSE heads,
// ERRORs, PONGs) have priority over CHUNK frames, and CHUNKs of different ids
// are interleaved round-robin, so a small response never waits for a large one.
type unaryWriter struct {
	conn *websocket.Conn

	mu      sync.Mutex
	ready   [][]byte
	streams []*chunkStream
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
func (w *unaryWriter) enqueueResponse(id uint32, meta unaryResponseMeta, payload []byte) error {
	total := uint32(len(payload))
	probe, err := encodeUnaryResponse(id, total, false, meta, nil)
	if err != nil {
		return err
	}
	room := unaryMaxFrameBytes - len(probe)
	if room <= 0 {
		return errors.New("response meta exceeds frame size")
	}
	if len(payload) <= room {
		head, err := encodeUnaryResponse(id, total, true, meta, payload)
		if err != nil {
			return err
		}
		return w.enqueue(head)
	}
	head, err := encodeUnaryResponse(id, total, false, meta, payload[:room])
	if err != nil {
		return err
	}
	w.mu.Lock()
	if len(w.ready) >= unaryReadyLimit {
		w.mu.Unlock()
		return errUnaryWriterOverflow
	}
	w.ready = append(w.ready, head)
	w.streams = append(w.streams, &chunkStream{id: id, payload: payload[room:]})
	w.mu.Unlock()
	w.wake()
	return nil
}

// drop discards the unsent CHUNKs of id (CANCEL after the RESPONSE head).
func (w *unaryWriter) drop(id uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	kept := w.streams[:0]
	for _, s := range w.streams {
		if s.id != id {
			kept = append(kept, s)
		}
	}
	w.streams = kept
}

// next pops the next frame to write, or nil when idle.
func (w *unaryWriter) next() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.ready) > 0 {
		f := w.ready[0]
		w.ready[0] = nil
		w.ready = w.ready[1:]
		return f
	}
	if len(w.streams) == 0 {
		return nil
	}
	s := w.streams[0]
	n := unaryMaxFrameBytes - unaryFrameHeaderLen - 1
	fin := len(s.payload) <= n
	if fin {
		n = len(s.payload)
	}
	frame := encodeUnaryChunk(s.id, fin, s.payload[:n])
	s.payload = s.payload[n:]
	w.streams = w.streams[1:]
	if !fin {
		w.streams = append(w.streams, s)
	}
	return frame
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
