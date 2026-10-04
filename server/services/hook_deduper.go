package services

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/internal/hookipc"
)

const (
	defaultHookDedupeTTL     = 5 * time.Minute
	defaultHookDedupeEntries = 4096
)

type hookDedupeEntry struct {
	reply     hookipc.ClassificationReply
	err       error
	expiresAt time.Time
	ready     chan struct{}
}

// HookDeduper coalesces concurrent duplicate deliveries and retains successful
// replies briefly. Only requests with a caller-proven stable tool-use identity
// may use it; randomly generated request IDs are deliberately never cached.
type HookDeduper struct {
	mu         sync.Mutex
	entries    map[hookipc.HookRequestID]*hookDedupeEntry
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

func NewHookDeduper() *HookDeduper {
	return newHookDeduper(defaultHookDedupeTTL, defaultHookDedupeEntries, time.Now)
}

func newHookDeduper(ttl time.Duration, maxEntries int, now func() time.Time) *HookDeduper {
	return &HookDeduper{
		entries:    make(map[hookipc.HookRequestID]*hookDedupeEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        now,
	}
}

func (d *HookDeduper) Do(
	ctx context.Context,
	id hookipc.HookRequestID,
	stable bool,
	classify func() (hookipc.ClassificationReply, error),
) (hookipc.ClassificationReply, error) {
	if d == nil || !stable || id == "" {
		return classify()
	}

	now := d.now()
	d.mu.Lock()
	d.removeExpiredLocked(now)
	if existing, ok := d.entries[id]; ok {
		ready := existing.ready
		d.mu.Unlock()
		select {
		case <-ready:
			return existing.reply, existing.err
		case <-ctx.Done():
			return hookipc.ClassificationReply{}, ctx.Err()
		}
	}
	if len(d.entries) >= d.maxEntries && !d.evictOldestReadyLocked() {
		d.mu.Unlock()
		return classify()
	}
	entry := &hookDedupeEntry{ready: make(chan struct{})}
	d.entries[id] = entry
	d.mu.Unlock()

	reply, err := classify()
	d.mu.Lock()
	entry.reply = reply
	entry.err = err
	entry.expiresAt = d.now().Add(d.ttl)
	close(entry.ready)
	if err != nil {
		delete(d.entries, id)
	}
	d.mu.Unlock()
	return reply, err
}

func (d *HookDeduper) removeExpiredLocked(now time.Time) {
	for id, entry := range d.entries {
		if entry.expiresAt.IsZero() {
			continue
		}
		if !now.Before(entry.expiresAt) {
			delete(d.entries, id)
		}
	}
}

func (d *HookDeduper) evictOldestReadyLocked() bool {
	var oldestID hookipc.HookRequestID
	var oldest time.Time
	for id, entry := range d.entries {
		if entry.expiresAt.IsZero() {
			continue
		}
		if oldestID == "" || entry.expiresAt.Before(oldest) {
			oldestID = id
			oldest = entry.expiresAt
		}
	}
	if oldestID == "" {
		return false
	}
	delete(d.entries, oldestID)
	return true
}
