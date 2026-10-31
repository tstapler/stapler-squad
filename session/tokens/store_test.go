package tokens

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// syncBuffer wraps bytes.Buffer with a mutex, matching the pattern already used in
// executor/safeexec/safeexec_pg_test.go and server/services/autonomous_orchestration_service_test.go.
// A plain bytes.Buffer here would be a real -race hazard the moment a future test drives
// concurrent writes through captureLogs (e.g. by calling Start()), even though today's
// sole caller is synchronous.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// captureLogs redirects slog's default logger to a JSON-lines buffer for the
// duration of the test, restoring the previous default on cleanup. Not safe
// to use from a t.Parallel() test — it mutates the process-wide slog default.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	return buf
}

func TestTokenStore_WhenFileNotCached_ExpectParseOnGetAll(t *testing.T) {
	t.Parallel()
	store := NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	// Manually enqueue a valid fixture file.
	store.enqueue("testdata/valid_session.jsonl")

	// Wait for the worker to process it.
	var results []*ParseResult
	wait.RequireEventually(t, func() bool {
		results = store.GetAll()
		return len(results) > 0
	}, 5*time.Second, 10*time.Millisecond)

	require.Len(t, results, 1)
	assert.Greater(t, results[0].TotalInput, int64(0))
}

func TestTokenStore_WhenFileCached_ExpectCacheHitSkipsReparse(t *testing.T) {
	t.Parallel()
	store := NewTokenStore("")

	// parseAndCache is the queue handler; calling it directly keeps this synchronous.
	store.parseAndCache("testdata/valid_session.jsonl")
	results1 := store.GetAll()
	require.Len(t, results1, 1)

	// Unchanged size and mtime: the cache entry must be kept as-is.
	store.parseAndCache("testdata/valid_session.jsonl")
	results2 := store.GetAll()
	require.Len(t, results2, 1)

	assert.Same(t, results1[0], results2[0], "expected same ParseResult pointer on cache hit")
}

func TestTokenStore_WhenGetByUUID_ExpectDirectLookup(t *testing.T) {
	t.Parallel()
	store := NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	// Parse a file with known session UUID (from filename).
	store.enqueue("testdata/valid_session.jsonl")

	wait.RequireEventually(t, func() bool { return len(store.GetAll()) > 0 }, 5*time.Second, 10*time.Millisecond)

	// The UUID comes from the filename.
	result := store.GetByUUID("valid_session")
	assert.NotNil(t, result)

	// Unknown UUID should return nil.
	unknown := store.GetByUUID("unknown-uuid-that-does-not-exist")
	assert.Nil(t, unknown)
}

func TestTokenStore_WhenConcurrentRequests_ExpectNoDataRace(t *testing.T) {
	t.Parallel()
	store := NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	store.enqueue("testdata/valid_session.jsonl")

	// Wait for initial parse.
	wait.RequireEventually(t, func() bool { return len(store.GetAll()) > 0 }, 5*time.Second, 10*time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results := store.GetAll()
			// Each goroutine should get a valid, non-nil result.
			for _, r := range results {
				assert.NotNil(t, r)
			}
		}()
	}
	wg.Wait()
}

func TestTokenStore_Subscribe_WhenStoreUpdated_ExpectNotification(t *testing.T) {
	t.Parallel()
	store := NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	ch := store.Subscribe()
	defer store.Unsubscribe(ch)

	store.enqueue("testdata/valid_session.jsonl")

	select {
	case <-ch:
		// Notification received.
	case <-time.After(5 * time.Second):
		t.Fatal("expected subscription notification within 5 seconds")
	}
}

// TestTokenStoreSubscribe_WhenSingleFileReparsed_ExpectSubscriberReceivesThatFilesParseResult
// asserts that a single-file reparse notification (triggered via enqueue,
// same path production code uses on an fsnotify callback) carries that
// file's freshly parsed *ParseResult, not a bare signal.
func TestTokenStoreSubscribe_WhenSingleFileReparsed_ExpectSubscriberReceivesThatFilesParseResult(t *testing.T) {
	t.Parallel()
	const path = "testdata/valid_session.jsonl"

	// Deliberately not calling Start(): the background walk's own
	// walk-complete notify(nil) would race against Subscribe() below and
	// could be mistaken for this test's single-file notification. Driving
	// parseAndCache directly exercises the same notify(result) call site
	// production's worker pool uses on an fsnotify callback, without that
	// race.
	store := NewTokenStore("")

	want, err := store.parser.ParseFile(path)
	require.NoError(t, err)

	ch := store.Subscribe()
	defer store.Unsubscribe(ch)

	store.parseAndCache(path)

	select {
	case got := <-ch:
		require.NotNil(t, got, "expected the reparsed file's *ParseResult, not nil")
		assert.Equal(t, want.SessionUUID, got.SessionUUID)
		assert.Equal(t, want.TotalInput, got.TotalInput)
		assert.Equal(t, want.TotalOutput, got.TotalOutput)
		assert.Equal(t, want.CacheCreation, got.CacheCreation)
		assert.Equal(t, want.CacheRead, got.CacheRead)
		assert.Equal(t, want.PrimaryModel, got.PrimaryModel)
	case <-time.After(5 * time.Second):
		t.Fatal("expected subscription notification within 5 seconds")
	}
}

// TestTokenStoreSubscribe_WhenInitialWalkCompletes_ExpectSubscriberReceivesNil
// asserts that the deferred notify at the end of walkAndEnqueue sends nil,
// distinguishing "walk finished" from "this file changed". The subscriber is
// created before Start() so it can't miss the walk-complete notification.
func TestTokenStoreSubscribe_WhenInitialWalkCompletes_ExpectSubscriberReceivesNil(t *testing.T) {
	t.Parallel()
	store := NewTokenStore("") // empty historyDir: walkAndEnqueue returns (and notifies) immediately.

	ch := store.Subscribe()
	defer store.Unsubscribe(ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	select {
	case got := <-ch:
		assert.Nil(t, got, "expected nil for the initial-walk-complete notification")
	case <-time.After(5 * time.Second):
		t.Fatal("expected subscription notification within 5 seconds")
	}
}

// A burst far beyond the old 256-slot queue must neither drop nor duplicate work.
func TestEnqueue_BurstCoalescesByPathAndNeverDrops(t *testing.T) {
	t.Parallel()
	store := NewTokenStore("") // Start() deliberately not called — nothing drains the queue.

	const distinct = 1000 // well past the old parseQueueSize of 256
	for round := 0; round < 3; round++ {
		for i := 0; i < distinct; i++ {
			store.enqueue(fmt.Sprintf("testdata/burst-%d.jsonl", i))
		}
	}

	assert.Equal(t, distinct, store.queue.Pending(), "every distinct path queued exactly once")
	st := store.QueueStats()
	assert.Equal(t, uint64(3*distinct), st.Enqueued)
	assert.Equal(t, uint64(2*distinct), st.Coalesced, "repeat enqueues of a pending path coalesce")
}
