package deliverygate

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
)

// HiddenKind is the kind of a hidden session, derived from its tags.
type HiddenKind string

const (
	KindReview     HiddenKind = "review"
	KindTriage     HiddenKind = "triage"
	KindDiagnose   HiddenKind = "diagnose"
	KindOther      HiddenKind = "other"
	KindUnresolved HiddenKind = "unresolved" // metrics label only
)

// FlagSettings is one immutable reading of the gate flag.
type FlagSettings struct {
	Global        bool
	KindOverrides map[HiddenKind]bool
}

// FlagLoader reads the persisted flag. A returned error keeps the last good snapshot.
type FlagLoader func() (FlagSettings, error)

// FlagObserver is what a flag service notifies after persisting a change, so a
// stale ticker read can never outlive an operator rollback.
type FlagObserver interface {
	OnFlagChanged(name string)
}

// ConfigFlagLoader reads hidden_session_gate from config.json. A missing file
// is the registry default (off); an unreadable or unparsable file is an error
// (so the last good snapshot is kept, never a silent flip).
func ConfigFlagLoader() (FlagSettings, error) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return FlagSettings{}, err
	}
	cfg, err := config.LoadConfigFromPath(filepath.Join(dir, config.ConfigFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FlagSettings{}, nil
		}
		return FlagSettings{}, err
	}
	return FlagSettings{Global: cfg.GetFeatureFlagWithDefault(config.HiddenSessionGateFeatureFlag, false)}, nil
}

// FlagCache serves the flag with no I/O on read. Reload is the only swapper:
// it loads inside reloadMu, so a ticker read parked before its swap cannot
// publish a value older than an UpdateFeatureFlag reload that ran meanwhile.
type FlagCache struct {
	loader   FlagLoader
	snapshot atomic.Pointer[FlagSettings]
	reloadMu sync.Mutex
	logger   *slog.Logger

	// betweenLoadAndSwap is a test seam: nil in production.
	betweenLoadAndSwap func()

	// swapHook runs inside reloadMu after each successful swap, so the stats
	// accumulator sees every change in the order it was published. Set once by
	// NewGate before the cache is shared.
	swapHook func(FlagSettings)

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// NewFlagCache starts at the registry default (off) without touching the
// loader; BuildRuntimeDeps calls Reload once and the ticker keeps it fresh.
func NewFlagCache(loader FlagLoader, logger *slog.Logger) *FlagCache {
	c := &FlagCache{loader: loader, logger: logger}
	c.snapshot.Store(&FlagSettings{})
	return c
}

// Reload re-reads the flag and swaps the snapshot. A read error keeps the last good one.
func (c *FlagCache) Reload() {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()
	s, err := c.loader()
	if c.betweenLoadAndSwap != nil {
		c.betweenLoadAndSwap()
	}
	if err != nil {
		c.logger.Warn("hidden_session_gate flag reload failed; keeping last value", "err", err)
		return
	}
	c.snapshot.Store(&s)
	if c.swapHook != nil {
		c.swapHook(s)
	}
}

// OnFlagChanged implements FlagObserver.
func (c *FlagCache) OnFlagChanged(name string) {
	if name == config.HiddenSessionGateFeatureFlag {
		c.Reload()
	}
}

// Enabled is the global value (no I/O).
func (c *FlagCache) Enabled() bool { return c.snapshot.Load().Global }

// EnabledFor applies kind override > global (Story 2.11 fills the overrides).
func (c *FlagCache) EnabledFor(kind HiddenKind) bool {
	s := c.snapshot.Load()
	if v, ok := s.KindOverrides[kind]; ok {
		return v
	}
	return s.Global
}

// Start runs the reload ticker. Stop cancels and joins it (BUG-089).
func (c *FlagCache) Start(ctx context.Context, tick <-chan time.Time) {
	c.stop = make(chan struct{})
	c.done = make(chan struct{})
	go c.tickLoop(ctx, tick)
}

func (c *FlagCache) tickLoop(ctx context.Context, tick <-chan time.Time) {
	defer close(c.done)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stop:
			return
		case _, ok := <-tick:
			if !ok {
				return
			}
			c.Reload()
		}
	}
}

// Stop ends the ticker goroutine and waits for it. Safe without Start and idempotent.
func (c *FlagCache) Stop() {
	c.stopOnce.Do(func() {
		if c.stop != nil {
			close(c.stop)
			<-c.done
		}
	})
}
