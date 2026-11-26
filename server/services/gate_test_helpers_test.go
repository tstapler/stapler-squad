package services

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// gateTestClock is a mutex-guarded manual clock for the stats tests.
type gateTestClock struct {
	mu sync.Mutex
	t  time.Time
}

func newGateTestClock() *gateTestClock {
	return &gateTestClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
}

func (c *gateTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *gateTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// gateMsgCapture is an injected slog handler (never the global default, BUG-087).
type gateMsgCapture struct {
	mu   *sync.Mutex
	msgs *[]string
}

func newLogCapture() (*slog.Logger, func(msg string) int) {
	h := gateMsgCapture{mu: &sync.Mutex{}, msgs: &[]string{}}
	return slog.New(h), func(msg string) int {
		h.mu.Lock()
		defer h.mu.Unlock()
		n := 0
		for _, m := range *h.msgs {
			if m == msg {
				n++
			}
		}
		return n
	}
}

func (gateMsgCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h gateMsgCapture) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	*h.msgs = append(*h.msgs, r.Message)
	h.mu.Unlock()
	return nil
}
func (h gateMsgCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h gateMsgCapture) WithGroup(string) slog.Handler      { return h }
