package streamhub

import (
	"sync"
	"time"
)

// interactiveWindow is how long after its last input a subscriber still counts as the
// interactive viewer for resize negotiation.
const interactiveWindow = 15 * time.Second

// interactivity tracks which subscribers recently sent input. The zero value is ready to use.
type interactivity struct {
	mu     sync.Mutex
	lastAt map[SubscriberID]time.Time
}

// NoteInteractive records that subscriber id just sent input, making it the interactive viewer
// for the next interactiveWindow. Call it from the input path; it is cheap and safe for
// concurrent use. Unknown or detached ids are harmless: their entries expire on the next read.
func (h *StreamHub) NoteInteractive(id SubscriberID) {
	now := h.clock.Now()
	h.interactive.mu.Lock()
	defer h.interactive.mu.Unlock()
	if h.interactive.lastAt == nil {
		h.interactive.lastAt = make(map[SubscriberID]time.Time)
	}
	h.interactive.lastAt[id] = now
}

// preferInteractive narrows votes to the subscribers that recently sent input, so a hidden or
// backgrounded viewer re-voting a smaller size cannot repeatedly shrink a pane someone is typing
// into. With no recently-active voter it returns votes unchanged (ADR-002's smallest common size).
func (h *StreamHub) preferInteractive(votes []ResizeVote) []ResizeVote {
	now := h.clock.Now()
	h.interactive.mu.Lock()
	for id, at := range h.interactive.lastAt {
		if now.Sub(at) > interactiveWindow {
			delete(h.interactive.lastAt, id)
		}
	}
	active := make([]ResizeVote, 0, len(votes))
	for _, v := range votes {
		if _, ok := h.interactive.lastAt[v.SubscriberID]; ok {
			active = append(active, v)
		}
	}
	h.interactive.mu.Unlock()
	if len(active) == 0 {
		return votes
	}
	return active
}
