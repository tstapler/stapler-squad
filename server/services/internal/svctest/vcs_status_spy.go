package svctest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
	"github.com/tstapler/stapler-squad/session/gitwiring"
)

// StatusSpyBackend counts Status calls. While gate is non-nil each call blocks until the
// gate is closed or its context ends, so tests control timing with channels, not sleeps.
type StatusSpyBackend struct {
	backend.Backend
	Calls     atomic.Int64
	Cancelled atomic.Int64
	IgnoreCtx bool // Status keeps waiting for the gate even if its context ends (a slow git)

	mu       sync.Mutex
	gate     chan struct{}
	released bool
}

func (b *StatusSpyBackend) currentGate() chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gate
}

// Block makes every later Status call wait for Release.
func (b *StatusSpyBackend) Block() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gate, b.released = make(chan struct{}), false
}

// Status implements backend.Backend.
func (b *StatusSpyBackend) Status(ctx context.Context, loc backend.RepoLocation, intent backend.Intent) (backend.StatusResult, error) {
	b.Calls.Add(1)
	if gate := b.currentGate(); gate != nil {
		var ctxDone <-chan struct{}
		if !b.IgnoreCtx {
			ctxDone = ctx.Done()
		}
		select {
		case <-gate:
		case <-ctxDone:
			b.Cancelled.Add(1)
			return backend.StatusResult{}, ctx.Err()
		}
	}
	if b.IgnoreCtx {
		ctx = context.WithoutCancel(ctx)
	}
	return b.Backend.Status(ctx, loc, intent)
}

// InstallStatusSpy swaps the process-wide git backend for a spy; gated makes every Status block.
func InstallStatusSpy(t *testing.T, gated bool) *StatusSpyBackend {
	t.Helper()
	spy := &StatusSpyBackend{Backend: gitwiring.Shared()}
	if gated {
		spy.Block()
	}
	t.Cleanup(gitprovider.Register(func() backend.Backend { return spy }))
	return spy
}

// Release unblocks every waiting and future Status call; safe to call more than once.
func (b *StatusSpyBackend) Release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.gate != nil && !b.released {
		close(b.gate)
		b.released = true
	}
}
