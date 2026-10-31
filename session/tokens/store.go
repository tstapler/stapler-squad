package tokens

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/scanqueue"
	"github.com/tstapler/stapler-squad/session/scanstate"
)

const (
	subChanSize = 64

	// stateConsumer names this store's rows in the scan-state table.
	stateConsumer = "tokens"

	// flushInterval and flushBatch bound how fast parse checkpoints are written
	// back: live files that change every second are written once per interval,
	// and a cold backfill's thousands of checkpoints trickle out instead of
	// bursting gzip+SQLite work on top of the parsing.
	flushInterval = 5 * time.Second
	flushBatch    = 100
)

// StateStore persists parse checkpoints across restarts. *scanstate.Scoped implements it.
type StateStore interface {
	Load(ctx context.Context) (map[string]scanstate.Record, error)
	Save(ctx context.Context, recs []scanstate.Record) error
	Delete(ctx context.Context, paths []string) error
}

// cachedEntry is one entry in the TokenStore cache. result is immutable once
// cached; cp is the resumable parser position that produced it.
type cachedEntry struct {
	result *ParseResult
	cp     *Checkpoint
}

// TokenStore caches parsed JSONL results keyed by file path.
// It parses all JSONL files in a directory on startup and keeps the cache
// fresh via fsnotify callbacks. With a StateStore, parse checkpoints survive
// restarts and growing transcripts are parsed incrementally from the last
// checkpoint instead of from byte 0.
type TokenStore struct {
	mu     sync.RWMutex
	cache  map[string]*cachedEntry
	byUUID map[string]*ParseResult // secondary index: SessionUUID → result
	// restored holds paths loaded from the StateStore that the startup walk has
	// not yet confirmed exist on disk.
	restored map[string]struct{}

	parser       *Parser
	historyDir   string
	isLoadingVal int32 // 1 while background restore+walk is running

	// queue coalesces by path and paces startup backfill; nothing is dropped.
	queue *scanqueue.Queue

	states     StateStore
	writer     *scanstate.Writer[*cachedEntry] // checkpoints awaiting write-back
	flushEvery time.Duration

	// subscribers receive notifications when the store is updated.
	subsMu sync.RWMutex
	subs   []chan *ParseResult

	cancelFunc context.CancelFunc
}

// Option configures a TokenStore.
type Option func(*TokenStore, *scanqueue.Config)

// WithStateStore persists parse checkpoints in s.
func WithStateStore(s StateStore) Option {
	return func(ts *TokenStore, _ *scanqueue.Config) { ts.states = s }
}

// WithQueueConfig sets worker count, startup delay and backfill throttle.
func WithQueueConfig(c scanqueue.Config) Option {
	return func(_ *TokenStore, qc *scanqueue.Config) { *qc = c }
}

// NewTokenStore creates a TokenStore that will parse all JSONL files in
// historyDir on startup.
func NewTokenStore(historyDir string, opts ...Option) *TokenStore {
	ts := &TokenStore{
		cache:      make(map[string]*cachedEntry),
		byUUID:     make(map[string]*ParseResult),
		restored:   make(map[string]struct{}),
		parser:     NewParser(),
		historyDir: historyDir,
		flushEvery: flushInterval,
	}
	var qc scanqueue.Config
	for _, o := range opts {
		o(ts, &qc)
	}
	ts.queue = scanqueue.New(qc, ts.parseAndCache)
	if ts.states != nil {
		ts.writer = scanstate.NewWriter(ts.states, encodeEntry)
	}
	return ts
}

// Start launches background workers and the initial restore+walk.
// It stops when ctx is cancelled. Call this once after creating the store.
func (ts *TokenStore) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	ts.cancelFunc = cancel

	ts.queue.Start(ctx)
	if ts.writer != nil {
		go ts.writer.Run(ctx, ts.flushEvery, flushBatch, func(err error) {
			log.Warn("[TokenStore] checkpoint flush failed", "err", err)
		})
	}
	go ts.walkAndEnqueue(ctx)
}

// Stop cancels the background context, stopping all goroutines.
func (ts *TokenStore) Stop() {
	if ts.cancelFunc != nil {
		ts.cancelFunc()
	}
}

// OnHistoryFileChanged is called by the HistoryFileWatcher callback when a file
// is created or modified. It enqueues the file for re-parsing.
func (ts *TokenStore) OnHistoryFileChanged(filePath string) {
	if !strings.HasSuffix(filePath, ".jsonl") {
		return
	}
	base := filepath.Base(filePath)
	if strings.HasPrefix(base, "agent-") {
		return
	}
	ts.enqueue(filePath)
}

// GetAll returns a snapshot of all cached ParseResult values under read lock.
func (ts *TokenStore) GetAll() []*ParseResult {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	results := make([]*ParseResult, 0, len(ts.cache))
	for _, entry := range ts.cache {
		if entry != nil && entry.result != nil {
			results = append(results, entry.result)
		}
	}
	return results
}

// GetByUUID returns the ParseResult for a given conversation UUID, or nil.
func (ts *TokenStore) GetByUUID(uuid string) *ParseResult {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.byUUID[uuid]
}

// IsLoading returns true while the background walk is still in progress.
func (ts *TokenStore) IsLoading() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.isLoadingVal > 0
}

// Subscribe returns a channel that receives the changed file's *ParseResult
// whenever the store is updated by a single-file reparse, or nil when the
// initial directory walk completes.
// The caller should drain the channel promptly to avoid blocking notifications.
func (ts *TokenStore) Subscribe() <-chan *ParseResult {
	ch := make(chan *ParseResult, subChanSize)
	ts.subsMu.Lock()
	ts.subs = append(ts.subs, ch)
	ts.subsMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (ts *TokenStore) Unsubscribe(ch <-chan *ParseResult) {
	ts.subsMu.Lock()
	defer ts.subsMu.Unlock()
	newSubs := ts.subs[:0]
	for _, s := range ts.subs {
		if s != ch {
			newSubs = append(newSubs, s)
		}
	}
	ts.subs = newSubs
}

// notify sends a non-blocking notification of result to all subscribers.
// result is the freshly parsed file for a single-file reparse, or nil when
// the initial directory walk has completed.
func (ts *TokenStore) notify(result *ParseResult) {
	ts.subsMu.RLock()
	defer ts.subsMu.RUnlock()
	for _, ch := range ts.subs {
		select {
		case ch <- result:
		default:
		}
	}
}

// enqueue schedules a live re-parse of filePath, coalescing with any pending one.
func (ts *TokenStore) enqueue(filePath string) { ts.queue.Enqueue(filePath) }

// QueueStats reports parse-queue activity (enqueued, coalesced, processed).
func (ts *TokenStore) QueueStats() scanqueue.Stats { return ts.queue.Stats() }

// parseAndCache brings the cache entry for filePath up to date, parsing only
// what was appended since the entry's checkpoint.
func (ts *TokenStore) parseAndCache(filePath string) {
	stat, err := os.Stat(filePath)
	if err != nil {
		return
	}

	ts.mu.RLock()
	existing := ts.cache[filePath]
	ts.mu.RUnlock()

	var prev *Checkpoint
	if existing != nil && existing.cp != nil {
		if existing.cp.Pos.Unchanged(stat) {
			return // cache is still valid
		}
		prev = existing.cp
	}

	result, cp, err := ts.parser.ParseFileFrom(filePath, prev)
	if err != nil {
		log.Warn("[TokenStore] parse failed", "path", filePath, "err", err)
		return
	}

	entry := &cachedEntry{result: result, cp: cp}
	ts.mu.Lock()
	ts.cache[filePath] = entry
	delete(ts.restored, filePath)
	if result.SessionUUID != "" {
		ts.byUUID[result.SessionUUID] = result
	}
	ts.mu.Unlock()

	ts.markDirty(filePath, entry)
	ts.notify(result)
}

func (ts *TokenStore) markDirty(filePath string, e *cachedEntry) {
	if ts.writer != nil {
		ts.writer.Mark(filePath, e)
	}
}

func encodeEntry(_ string, e *cachedEntry) (scanstate.Record, error) {
	blob, err := e.cp.Encode()
	if err != nil {
		return scanstate.Record{}, err
	}
	return scanstate.Record{Position: e.cp.Pos, State: blob}, nil
}

// FlushState writes every pending checkpoint to the StateStore.
func (ts *TokenStore) FlushState(ctx context.Context) error {
	if ts.writer == nil {
		return nil
	}
	return ts.writer.Flush(ctx, 0)
}

// restoreState loads persisted checkpoints into the cache so unchanged files
// need no parsing. A record that no longer decodes is ignored (its file is
// simply parsed from byte 0).
func (ts *TokenStore) restoreState(ctx context.Context) {
	if ts.states == nil {
		return
	}
	recs, err := ts.states.Load(ctx)
	if err != nil {
		log.Warn("[TokenStore] load checkpoints failed; parsing from scratch", "err", err)
		return
	}
	for path, rec := range recs {
		if ctx.Err() != nil {
			return
		}
		cp, err := DecodeCheckpoint(rec.State, rec.Position)
		if err != nil {
			continue
		}
		result := cp.Result()
		ts.mu.Lock()
		if _, live := ts.cache[path]; live {
			ts.mu.Unlock() // a live change already parsed this file; it is newer than the checkpoint
			continue
		}
		ts.cache[path] = &cachedEntry{result: result, cp: cp}
		ts.restored[path] = struct{}{}
		if result.SessionUUID != "" {
			ts.byUUID[result.SessionUUID] = result
		}
		ts.mu.Unlock()
	}
}

// dropUnseenRestored removes restored entries whose files the walk did not find.
func (ts *TokenStore) dropUnseenRestored(ctx context.Context) {
	ts.mu.Lock()
	var gone []string
	for path := range ts.restored {
		gone = append(gone, path)
		if e := ts.cache[path]; e != nil && ts.byUUID[e.result.SessionUUID] == e.result {
			delete(ts.byUUID, e.result.SessionUUID)
		}
		delete(ts.cache, path)
	}
	ts.restored = make(map[string]struct{})
	ts.mu.Unlock()
	if len(gone) > 0 && ts.states != nil {
		if err := ts.states.Delete(ctx, gone); err != nil {
			log.Warn("[TokenStore] delete stale checkpoints failed", "count", len(gone), "err", err)
		}
	}
}

// walkAndEnqueue restores persisted checkpoints, then walks historyDir
// recursively and queues all .jsonl files as paced backfill.
func (ts *TokenStore) walkAndEnqueue(ctx context.Context) {
	ts.mu.Lock()
	ts.isLoadingVal = 1
	ts.mu.Unlock()

	defer func() {
		ts.mu.Lock()
		ts.isLoadingVal = 0
		ts.mu.Unlock()
		ts.notify(nil)
	}()

	ts.restoreState(ctx)

	if ts.historyDir == "" {
		return
	}

	_ = filepath.WalkDir(ts.historyDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || ctx.Err() != nil {
			return nil
		}
		if !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, "agent-") {
			return nil
		}
		ts.mu.Lock()
		// Confirmed on disk: keep the restored entry, but leave it in the
		// cache until the queue decides whether the file changed.
		delete(ts.restored, path)
		ts.mu.Unlock()
		ts.queue.EnqueueBackfill(path)
		return nil
	})
	if ctx.Err() == nil {
		ts.dropUnseenRestored(ctx)
	}
}
