package services

import (
	"container/list"
	"context"
	"sync"
)

// actionCacheMax bounds the idempotency cache; the oldest key is evicted first.
const actionCacheMax = 1024

// actionFlight is one in-flight or finished attempt of an idempotency key.
type actionFlight struct {
	key  string
	hash string
	done chan struct{}
	res  ActionResult
}

// wait blocks until the leader finishes or ctx ends.
func (f *actionFlight) wait(ctx context.Context) (ActionResult, error) {
	select {
	case <-f.done:
		return f.res, nil
	case <-ctx.Done():
		return ActionResult{}, ctx.Err()
	}
}

// flightRole is what begin made the caller.
type flightRole int

const (
	flightLeader flightRole = iota
	// flightFollower: another call with the same key and payload owns it (or has finished).
	flightFollower
	// flightMismatch: the key was used for a different payload.
	flightMismatch
)

// actionIdempotency is a bounded LRU of idempotency keys with payload hashes.
// A key seen again with the same payload returns the first result; with a
// different payload it is a mismatch (INVALID).
type actionIdempotency struct {
	mu      sync.Mutex
	max     int
	order   *list.List // most recently used first; values are *actionFlight
	byKey   map[string]*list.Element
	removed int // evictions, for tests
}

func newActionIdempotency(max int) *actionIdempotency {
	return &actionIdempotency{max: max, order: list.New(), byKey: map[string]*list.Element{}}
}

// begin registers key for hash or reports who already owns it.
func (c *actionIdempotency) begin(key, hash string) (*actionFlight, flightRole) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byKey[key]; ok {
		f := el.Value.(*actionFlight)
		if f.hash != hash {
			return nil, flightMismatch
		}
		c.order.MoveToFront(el)
		return f, flightFollower
	}
	f := &actionFlight{key: key, hash: hash, done: make(chan struct{})}
	c.byKey[key] = c.order.PushFront(f)
	for c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.byKey, oldest.Value.(*actionFlight).key)
		c.removed++
	}
	return f, flightLeader
}

// settle publishes the leader's result and keeps the key so a retry replays it.
func (c *actionIdempotency) settle(f *actionFlight, res ActionResult) {
	c.mu.Lock()
	f.res = res
	c.mu.Unlock()
	close(f.done)
}

// forget publishes the result to waiting followers and drops the key, so a
// retry runs again: a result that is safe to retry must not be replayed.
func (c *actionIdempotency) forget(f *actionFlight, res ActionResult) {
	c.mu.Lock()
	f.res = res
	if el, ok := c.byKey[f.key]; ok && el.Value.(*actionFlight) == f {
		c.order.Remove(el)
		delete(c.byKey, f.key)
	}
	c.mu.Unlock()
	close(f.done)
}

func (c *actionIdempotency) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
