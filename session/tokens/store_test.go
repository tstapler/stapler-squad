package tokens

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenStore_WhenFileNotCached_ExpectParseOnGetAll(t *testing.T) {
	store := NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	// Manually enqueue a valid fixture file.
	store.enqueue("testdata/valid_session.jsonl")

	// Wait for the worker to process it.
	deadline := time.Now().Add(5 * time.Second)
	var results []*ParseResult
	for time.Now().Before(deadline) {
		results = store.GetAll()
		if len(results) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	require.Len(t, results, 1)
	assert.Greater(t, results[0].TotalInput, int64(0))
}

func TestTokenStore_WhenFileCached_ExpectCacheHitSkipsReparse(t *testing.T) {
	store := NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	// Parse and cache a file.
	store.enqueue("testdata/valid_session.jsonl")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(store.GetAll()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Get the cached result pointer.
	results1 := store.GetAll()
	require.NotEmpty(t, results1)
	ptr1 := results1[0]

	// Enqueue again — modtime hasn't changed, so cache should not be reparsed.
	store.enqueue("testdata/valid_session.jsonl")
	time.Sleep(200 * time.Millisecond)

	results2 := store.GetAll()
	require.NotEmpty(t, results2)
	ptr2 := results2[0]

	// Same pointer (no reparse).
	assert.Equal(t, ptr1, ptr2, "expected same ParseResult pointer on cache hit")
}

func TestTokenStore_WhenGetByUUID_ExpectDirectLookup(t *testing.T) {
	store := NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	// Parse a file with known session UUID (from filename).
	store.enqueue("testdata/valid_session.jsonl")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(store.GetAll()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The UUID comes from the filename.
	result := store.GetByUUID("valid_session")
	assert.NotNil(t, result)

	// Unknown UUID should return nil.
	unknown := store.GetByUUID("unknown-uuid-that-does-not-exist")
	assert.Nil(t, unknown)
}

func TestTokenStore_WhenConcurrentRequests_ExpectNoDataRace(t *testing.T) {
	store := NewTokenStore("")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store.Start(ctx)

	store.enqueue("testdata/valid_session.jsonl")

	// Wait for initial parse.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(store.GetAll()) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

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
