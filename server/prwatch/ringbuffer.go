package prwatch

import "sync"

// DispositionBufferSize is the number of recent dispositions kept (D14).
const DispositionBufferSize = 200

// RingBuffer keeps the newest DispositionBufferSize dispositions. Disposition
// has no actor login or event text field, so none can be stored.
type RingBuffer struct {
	mu    sync.Mutex
	buf   [DispositionBufferSize]Disposition
	next  int
	count int
}

var _ Recorder = (*RingBuffer)(nil)

// Record stores d, evicting the oldest entry when full.
func (r *RingBuffer) Record(d Disposition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.next] = d
	r.next = (r.next + 1) % DispositionBufferSize
	if r.count < DispositionBufferSize {
		r.count++
	}
}

// Len returns how many dispositions are held.
func (r *RingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// Entries returns the held dispositions, oldest first.
func (r *RingBuffer) Entries() []Disposition {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Disposition, 0, r.count)
	start := (r.next - r.count + DispositionBufferSize) % DispositionBufferSize
	for i := 0; i < r.count; i++ {
		out = append(out, r.buf[(start+i)%DispositionBufferSize])
	}
	return out
}
