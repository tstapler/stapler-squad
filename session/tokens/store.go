package tokens

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

const (
	workerPoolSize = 4
	parseQueueSize = 256
	subChanSize    = 64
)

// cachedEntry is one entry in the TokenStore cache.
type cachedEntry struct {
	result  *ParseResult
	modTime time.Time
}

// TokenStore caches parsed JSONL results keyed by file path.
// It pre-parses all JSONL files in a directory on startup and keeps the cache
// fresh via fsnotify callbacks.
type TokenStore struct {
	mu     sync.RWMutex
	cache  map[string]*cachedEntry
	byUUID map[string]*ParseResult // secondary index: SessionUUID → result

	parser       *Parser
	historyDir   string
	isLoadingVal int32 // atomic: 1 while background walk is running

	// parseQueue is the work queue for the worker pool.
	parseQueue chan string

	// inflight tracks files currently being parsed to prevent duplicate work.
	inflight sync.Map // key: filePath, value: struct{}

	// subscribers receive notifications when the store is updated.
	subsMu sync.RWMutex
	subs   []chan struct{}

	cancelFunc context.CancelFunc
}

// NewTokenStore creates a TokenStore that will pre-parse all JSONL files in
// historyDir on startup.
func NewTokenStore(historyDir string) *TokenStore {
	return &TokenStore{
		cache:      make(map[string]*cachedEntry),
		byUUID:     make(map[string]*ParseResult),
		parser:     NewParser(),
		historyDir: historyDir,
		parseQueue: make(chan string, parseQueueSize),
	}
}

// Start launches background workers and the initial directory walker.
// It stops when ctx is cancelled. Call this once after creating the store.
func (ts *TokenStore) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	ts.cancelFunc = cancel

	// Start worker pool.
	for i := 0; i < workerPoolSize; i++ {
		go ts.worker(ctx)
	}

	// Start the initial walk in the background.
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

// Subscribe returns a channel that receives a struct{} whenever the store is updated.
// The caller should drain the channel promptly to avoid blocking notifications.
func (ts *TokenStore) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, subChanSize)
	ts.subsMu.Lock()
	ts.subs = append(ts.subs, ch)
	ts.subsMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (ts *TokenStore) Unsubscribe(ch <-chan struct{}) {
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

// notify sends a non-blocking notification to all subscribers.
func (ts *TokenStore) notify() {
	ts.subsMu.RLock()
	defer ts.subsMu.RUnlock()
	for _, ch := range ts.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// enqueue adds a file to the parse queue if it's not already in-flight.
func (ts *TokenStore) enqueue(filePath string) {
	if _, loaded := ts.inflight.LoadOrStore(filePath, struct{}{}); loaded {
		return // already in flight
	}
	select {
	case ts.parseQueue <- filePath:
	default:
		// Queue full — remove from inflight so it can be retried later.
		ts.inflight.Delete(filePath)
		log.Warn("[TokenStore] parse queue full, dropping", "path", filePath)
	}
}

// worker is a pool worker that reads from parseQueue and parses files.
func (ts *TokenStore) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case filePath := <-ts.parseQueue:
			ts.parseAndCache(filePath)
		}
	}
}

// parseAndCache parses a JSONL file and updates the cache.
func (ts *TokenStore) parseAndCache(filePath string) {
	defer ts.inflight.Delete(filePath)

	// Check if file has changed since last parse.
	stat, err := os.Stat(filePath)
	if err != nil {
		return
	}

	ts.mu.RLock()
	existing := ts.cache[filePath]
	ts.mu.RUnlock()

	if existing != nil && !stat.ModTime().After(existing.modTime) {
		return // cache is still valid
	}

	result, err := ts.parser.ParseFile(filePath)
	if err != nil {
		log.Warn("[TokenStore] parse failed", "path", filePath, "err", err)
		return
	}

	ts.mu.Lock()
	ts.cache[filePath] = &cachedEntry{
		result:  result,
		modTime: stat.ModTime(),
	}
	if result.SessionUUID != "" {
		ts.byUUID[result.SessionUUID] = result
	}
	ts.mu.Unlock()

	ts.notify()
}

// walkAndEnqueue walks historyDir recursively and enqueues all .jsonl files.
func (ts *TokenStore) walkAndEnqueue(ctx context.Context) {
	ts.mu.Lock()
	ts.isLoadingVal = 1
	ts.mu.Unlock()

	defer func() {
		ts.mu.Lock()
		ts.isLoadingVal = 0
		ts.mu.Unlock()
		ts.notify()
	}()

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
		ts.enqueue(path)
		return nil
	})
}
