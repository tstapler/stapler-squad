package matview

import (
	"context"
	"sort"
	"sync"
)

type subscriber[K comparable, V any] struct {
	mu      sync.Mutex
	pending map[K]Update[K, V]
	notify  chan struct{}
	keys    []K
}

// put records the latest update for a key, replacing any undelivered one: a
// slow consumer skips intermediate states but never misses the newest.
func (s *subscriber[K, V]) put(u Update[K, V]) {
	s.mu.Lock()
	s.pending[u.Key] = u
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *subscriber[K, V]) take() []Update[K, V] {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Update[K, V], 0, len(s.pending))
	for _, u := range s.pending {
		out = append(out, u)
	}
	clear(s.pending)
	return out
}

// Subscribe pins keys and streams their values: the current value of each key
// as soon as it exists, then conflated updates. Keys beyond MaxKeysPerSub or the
// process-wide MaxPinned are returned as rejected and not watched; callers fall
// back to polling for them. The channel closes when ctx ends or the view closes.
func (v *View[K, V]) Subscribe(ctx context.Context, keys []K) (<-chan Update[K, V], []K) {
	s := &subscriber[K, V]{pending: map[K]Update[K, V]{}, notify: make(chan struct{}, 1)}
	var rejected []K
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		out := make(chan Update[K, V])
		close(out)
		return out, keys
	}
	seen := map[K]bool{}
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		if len(s.keys) >= v.cfg.MaxKeysPerSub || v.pinned >= v.cfg.MaxPinned {
			v.stats.PinRejected++
			rejected = append(rejected, k)
			continue
		}
		e := v.entryLocked(k)
		e.pins++
		v.pinned++
		e.subs[s] = struct{}{}
		s.keys = append(s.keys, k)
		if e.has {
			s.put(Update[K, V]{Key: k, Versioned: e.cur})
		} else {
			v.enqueueLocked(e)
		}
	}
	v.pumpLocked()
	v.mu.Unlock()

	out := make(chan Update[K, V])
	go v.serve(ctx, s, out)
	return out, rejected
}

func (v *View[K, V]) serve(ctx context.Context, s *subscriber[K, V], out chan<- Update[K, V]) {
	defer close(out)
	defer v.unpin(s)
	for {
		select {
		case <-ctx.Done():
			return
		case <-v.done:
			return
		case <-s.notify:
		}
		for _, u := range s.take() {
			select {
			case out <- u:
			case <-ctx.Done():
				return
			case <-v.done:
				return
			}
		}
	}
}

func (v *View[K, V]) unpin(s *subscriber[K, V]) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, k := range s.keys {
		e, ok := v.entries[k]
		if !ok {
			continue
		}
		if _, mine := e.subs[s]; mine {
			delete(e.subs, s)
			e.pins--
			v.pinned--
			e.lastRead = v.cfg.Clock.Now()
		}
	}
}

func (v *View[K, V]) armSweepLocked() {
	v.sweeper = v.cfg.Clock.AfterFunc(v.cfg.IdleEvictAfter/2, func() {
		v.Sweep()
		v.mu.Lock()
		defer v.mu.Unlock()
		if !v.closed {
			v.armSweepLocked()
		}
	})
}

// Sweep evicts unpinned entries idle longer than IdleEvictAfter, then the
// least recently read unpinned entries beyond UnpinnedCap.
func (v *View[K, V]) Sweep() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return
	}
	now := v.cfg.Clock.Now()
	for _, e := range v.entries {
		if v.evictableLocked(e) && v.cfg.IdleEvictAfter > 0 && now.Sub(e.lastRead) > v.cfg.IdleEvictAfter {
			v.evictLocked(e)
		}
	}
	v.evictOverCapLocked()
}

func (v *View[K, V]) evictableLocked(e *entry[K, V]) bool {
	return e.pins == 0 && !e.running && !e.queued && len(e.waiters) == 0
}

func (v *View[K, V]) evictLocked(e *entry[K, V]) {
	v.stats.Evictions++
	v.dropLocked(e, ErrRemoved)
}

func (v *View[K, V]) evictOverCapLocked() {
	var cand []*entry[K, V]
	for _, e := range v.entries {
		if v.evictableLocked(e) {
			cand = append(cand, e)
		}
	}
	over := len(cand) - v.cfg.UnpinnedCap
	if over <= 0 {
		return
	}
	sort.Slice(cand, func(i, j int) bool { return cand[i].lastRead.Before(cand[j].lastRead) })
	for _, e := range cand[:over] {
		v.evictLocked(e)
	}
}

type semaphore chan struct{}

func newSemaphore(n int) semaphore { return make(semaphore, n) }

func (s semaphore) TryAcquire() bool {
	select {
	case s <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s semaphore) Release() { <-s }
