package tymux

// Sequenced delivery: tymuxd numbers every output chunk (OutputChunk.seq) and every snapshot
// (PaneSnapshot.seq) in one space, so a snapshot taken at N can be spliced into the stream
// exactly: everything <= N is inside the snapshot, everything > N must still follow it.

// seqRingBudgetBytes bounds the recent-output ring ResyncAt replays after a snapshot. A capture
// that takes longer than this much output to return cannot be spliced and is retried.
const seqRingBudgetBytes = 1 << 20

type seqFrame struct {
	seq  uint64
	data []byte
}

// seqState is the fanout's view of the sequenced stream. valid is false until a priming
// snapshot reports a seq, and again after any unsequenced (legacy) output.
type seqState struct {
	valid     bool
	last      uint64     // highest seq delivered
	ring      []seqFrame // contiguous seqs ending at last, oldest first
	ringBytes int
	// gapSignaled records an OutputGap event whose seq jump has not arrived yet, so that jump is
	// not counted as a second, unannounced gap.
	gapSignaled bool
	gaps        uint64 // monotonic count of gaps (signaled or not)
	// gen changes whenever the sequence space may have restarted (reseed, daemon restart), so a
	// snapshot captured under an older gen is never spliced against the new numbering.
	gen uint64
}

func (st *seqState) push(seq uint64, data []byte) {
	st.ring = append(st.ring, seqFrame{seq, data})
	st.ringBytes += len(data)
	for st.ringBytes > seqRingBudgetBytes && len(st.ring) > 1 {
		st.ringBytes -= len(st.ring[0].data)
		st.ring[0] = seqFrame{} // release the evicted payload
		st.ring = st.ring[1:]
	}
}

func (st *seqState) clearRing() {
	st.ring, st.ringBytes = nil, 0
}

// tailSince returns the concatenated output after seq, or ok=false when the ring no longer
// reaches back to seq+1.
func (st *seqState) tailSince(seq uint64) (tail []byte, ok bool) {
	if !st.valid {
		return nil, false
	}
	if seq >= st.last {
		return nil, true
	}
	if len(st.ring) == 0 || st.ring[0].seq > seq+1 {
		return nil, false
	}
	for _, fr := range st.ring {
		if fr.seq > seq {
			tail = append(tail, fr.data...)
		}
	}
	return tail, true
}

// ResetSeq starts a fresh sequenced stream whose output after base is still to come (a priming
// snapshot at base).
func (f *ClientFanout) ResetSeq(base uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq.valid, f.seq.last, f.seq.gapSignaled = true, base, false
	f.seq.gen++
	f.seq.clearRing()
	f.clearSkipsLocked()
}

// clearSkipsLocked forgets every subscriber's skipThrough: it belongs to the old numbering.
func (f *ClientFanout) clearSkipsLocked() {
	for _, sub := range f.subscribers {
		sub.skipThrough = 0
	}
}

// DisableSeq marks the stream as unsequenced (an older daemon, or legacy output), so resyncs use
// the unsequenced path.
func (f *ClientFanout) DisableSeq() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq.valid = false
	f.seq.gen++
	f.seq.clearRing()
	f.clearSkipsLocked()
}

// SeqGeneration identifies the current sequence space; read it before a capture and pass it to
// ResyncAt.
func (f *ClientFanout) SeqGeneration() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seq.gen
}

// SeqValid reports whether resyncs can be spliced by sequence number.
func (f *ClientFanout) SeqValid() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seq.valid
}

// NoteGapSignaled records that tymuxd announced lost output; the seq jump that follows belongs
// to this gap.
func (f *ClientFanout) NoteGapSignaled() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq.gapSignaled = true
	f.seq.gaps++
}

// GapGeneration changes whenever output was lost; a cached snapshot older than it is stale.
func (f *ClientFanout) GapGeneration() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seq.gaps
}

// BroadcastSeq is Broadcast for a sequenced chunk. It drops duplicates (seq already delivered),
// skips subscribers whose resync already contained this seq, and reports gap=true when the seq
// jumped without a preceding OutputGap (the caller must resync).
func (f *ClientFanout) BroadcastSeq(seq uint64, data []byte) (gap bool) {
	f.mu.Lock()
	if f.seq.valid {
		st := &f.seq
		if seq <= st.last {
			f.mu.Unlock()
			return false
		}
		if seq != st.last+1 {
			st.clearRing()
			if st.gapSignaled {
				st.gapSignaled = false
			} else {
				st.gaps++
				gap = true
			}
		}
		st.last = seq
		st.push(seq, data)
	}
	dropped := f.deliverLocked(data, seq)
	onDrop := f.onDrop
	f.mu.Unlock()
	if dropped && onDrop != nil {
		onDrop()
	}
	return gap
}

// ResyncScope selects which subscribers a resync replaces.
type ResyncScope int

const (
	// ScopeAll resyncs every subscriber (the upstream stream lost output).
	ScopeAll ResyncScope = iota
	// ScopeDropped resyncs only subscribers whose own queue overflowed.
	ScopeDropped
)

// ResyncAt splices a snapshot taken at snapSeq into the stream: each selected subscriber's
// queue is replaced with redraw followed by every output chunk after snapSeq, and chunks <=
// snapSeq arriving later are discarded for it. dirtyOnly limits it to subscribers that lost a
// frame. gen is SeqGeneration as read before the capture. ok is false (nothing changed) when the
// stream is unsequenced, the sequence space was reseeded since gen, or the ring no longer
// reaches back to snapSeq+1; the caller retries with a newer snapshot.
func (f *ClientFanout) ResyncAt(snapSeq, gen uint64, redraw []byte, scope ResyncScope) (n int, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if gen != f.seq.gen {
		return 0, false
	}
	tail, ok := f.seq.tailSince(snapSeq)
	if !ok {
		return 0, false
	}
	payload := redraw
	if len(tail) > 0 {
		payload = append(append(make([]byte, 0, len(redraw)+len(tail)), redraw...), tail...)
	}
	through := max(snapSeq, f.seq.last)
	for _, sub := range f.subscribers {
		if scope == ScopeDropped && !sub.dropped {
			continue
		}
		for drained := false; !drained; {
			select {
			case <-sub.ch:
			default:
				drained = true
			}
		}
		sub.ch <- payload // emptied under f.mu above, so this cannot block
		sub.dropped, sub.skipThrough = false, through
		n++
	}
	return n, true
}

// deliverLocked is the non-blocking send shared by Broadcast and BroadcastSeq. seq 0 means
// unsequenced. It reports whether any subscriber dropped the frame. f.mu must be held.
func (f *ClientFanout) deliverLocked(data []byte, seq uint64) (dropped bool) {
	for _, sub := range f.subscribers {
		if seq != 0 && seq <= sub.skipThrough {
			continue
		}
		if sub.resyncing {
			if len(sub.held) < cap(sub.ch)-1 {
				sub.held = append(sub.held, data)
			} else {
				sub.heldLost = true
			}
			continue
		}
		select {
		case sub.ch <- data:
		default:
			sub.dropped = true
			dropped = true
		}
	}
	return dropped
}
