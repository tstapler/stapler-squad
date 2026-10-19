package services

import (
	"container/list"
	"context"
	"errors"
	"sync"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

const (
	analysisCacheEntries     = 32
	analysisDefaultParallel  = 1
	rangeHashCapBytes        = 64 << 20 // ADR-036 S-T2: hash_cap_bytes confirmed
	rangeReadCeilingBytes    = 1 << 30  // ADR-036 S-T2: read ceiling confirmed
	analysisStateUnspecified = sessionv1.FileAnalysisState_FILE_ANALYSIS_STATE_UNSPECIFIED
	analysisStatePending     = sessionv1.FileAnalysisState_FILE_ANALYSIS_STATE_PENDING
	analysisStateReady       = sessionv1.FileAnalysisState_FILE_ANALYSIS_STATE_READY
	analysisStateFailed      = sessionv1.FileAnalysisState_FILE_ANALYSIS_STATE_FAILED
)

// errAnalysisStale marks a pass whose file changed underneath it; the result is
// discarded and never cached under the new validator.
var errAnalysisStale = errors.New("file changed during analysis") //nolint:gochecknoglobals

// analysisKey is the validator an Analysis is valid for.
type analysisKey struct {
	Path    string
	Size    int64
	MtimeNs int64
	Inode   uint64
}

// analysisRunner produces one Analysis; it must honor ctx and return
// errAnalysisStale when it detects the file changed.
type analysisRunner func(ctx context.Context) (*Analysis, error)

type analysisEntry struct {
	key      analysisKey
	state    sessionv1.FileAnalysisState
	result   *Analysis
	err      error
	waiters  int  // Await callers currently interested
	detached bool // started by Start: cache-owned, survives zero waiters
	cancel   context.CancelFunc
	done     chan struct{}
	elem     *list.Element
}

// analysisCache is an LRU of background Analyze results keyed by validator. A
// process-wide semaphore bounds concurrent passes; a pass is cancelled when its
// last interested Await caller leaves (unless Start made it cache-owned) or when
// the entry is invalidated, evicted or the cache is closed.
type analysisCache struct {
	mu      sync.Mutex
	entries map[analysisKey]*analysisEntry
	byPath  map[string]analysisKey
	lru     *list.List // front = most recently used
	max     int
	sem     chan struct{}
	ctx     context.Context
	stop    context.CancelFunc
	wg      sync.WaitGroup
}

func newAnalysisCache(maxEntries, parallel int) *analysisCache {
	ctx, stop := context.WithCancel(context.Background())
	return &analysisCache{
		entries: map[analysisKey]*analysisEntry{},
		byPath:  map[string]analysisKey{},
		lru:     list.New(),
		max:     maxEntries,
		sem:     make(chan struct{}, max(1, parallel)),
		ctx:     ctx,
		stop:    stop,
	}
}

// Close cancels every pass and waits for them to exit.
func (c *analysisCache) Close() {
	c.stop()
	c.wg.Wait()
}

// State reports the analysis state for key without starting anything.
func (c *analysisCache) State(key analysisKey) (sessionv1.FileAnalysisState, *Analysis) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return analysisStateUnspecified, nil
	}
	c.lru.MoveToFront(e.elem)
	return e.state, e.result
}

// Start ensures a cache-owned background pass for key and returns its state and
// result (nil until ready).
func (c *analysisCache) Start(key analysisKey, run analysisRunner) (sessionv1.FileAnalysisState, *Analysis) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.ensureLocked(key, run)
	e.detached = true
	return e.state, e.result
}

// Await joins the pass for key (starting one if needed) and waits for it or ctx.
func (c *analysisCache) Await(ctx context.Context, key analysisKey, run analysisRunner) (*Analysis, error) {
	c.mu.Lock()
	e := c.ensureLocked(key, run)
	e.waiters++
	c.mu.Unlock()

	var waitErr error
	select {
	case <-e.done:
	case <-ctx.Done():
		waitErr = ctx.Err()
	}

	c.mu.Lock()
	e.waiters--
	if e.waiters == 0 && !e.detached && e.state == analysisStatePending {
		c.dropLocked(e) // also cancels; keeps a new joiner from receiving this doomed entry
	}
	c.mu.Unlock()
	if waitErr != nil {
		return nil, waitErr
	}
	return e.result, e.err
}

func (c *analysisCache) ensureLocked(key analysisKey, run analysisRunner) *analysisEntry {
	if e, ok := c.entries[key]; ok {
		c.lru.MoveToFront(e.elem)
		return e
	}
	if old, ok := c.byPath[key.Path]; ok {
		c.dropLocked(c.entries[old]) // validator changed: rebuild in full
	}
	ctx, cancel := context.WithCancel(c.ctx)
	e := &analysisEntry{key: key, state: analysisStatePending, cancel: cancel, done: make(chan struct{})}
	e.elem = c.lru.PushFront(e)
	c.entries[key] = e
	c.byPath[key.Path] = key
	for c.lru.Len() > c.max {
		c.dropLocked(c.lru.Back().Value.(*analysisEntry))
	}
	c.wg.Add(1)
	go c.runEntry(ctx, e, run)
	return e
}

// dropLocked removes e and cancels its pass if still running.
func (c *analysisCache) dropLocked(e *analysisEntry) {
	if e == nil || c.entries[e.key] != e {
		return
	}
	e.cancel()
	delete(c.entries, e.key)
	if c.byPath[e.key.Path] == e.key {
		delete(c.byPath, e.key.Path)
	}
	c.lru.Remove(e.elem)
}

func (c *analysisCache) runEntry(ctx context.Context, e *analysisEntry, run analysisRunner) {
	defer c.wg.Done()
	defer close(e.done)
	var res *Analysis
	var err error
	select {
	case c.sem <- struct{}{}:
		res, err = run(ctx)
		<-c.sem
	case <-ctx.Done():
		err = ctx.Err()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case err != nil:
		// Cancel, stale and I/O errors alike are never cached: waiters holding e still read e.err, the next request starts fresh.
		c.dropLocked(e)
		e.state, e.err = analysisStateFailed, err
	default:
		e.state, e.result = analysisStateReady, res
	}
}
