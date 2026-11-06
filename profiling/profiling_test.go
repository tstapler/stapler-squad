package profiling

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contendOnMutex makes one goroutine wait on a lock the caller holds, so the
// runtime records a mutex-contention sample whose stack contains this name.
//
//go:noinline
func contendOnMutex() {
	var mu sync.Mutex
	mu.Lock()
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(ready)
		mu.Lock()
		mu.Unlock() //nolint:staticcheck // empty critical section is the point: it only needs to wait
		close(done)
	}()
	<-ready
	for i := 0; i < 8; i++ {
		runtime.Gosched()
	}
	mu.Unlock()
	<-done
}

func fetchProfile(t *testing.T, h http.Handler, path string) []byte {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, rec.Code, path)
	zr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err, "%s must be a gzipped pprof proto", path)
	raw, err := io.ReadAll(zr)
	require.NoError(t, err)
	return raw
}

func TestProfilingMux_DeltaAndCumulativeEndpointsRespond(t *testing.T) {
	mux := newProfilingMux()
	for _, path := range []string{
		"/debug/pprof/mutex", "/debug/pprof/block", "/debug/pprof/heap",
		"/debug/pprof/delta_mutex", "/debug/pprof/delta_block", "/debug/pprof/delta_heap",
	} {
		assert.NotEmpty(t, fetchProfile(t, mux, path), path)
	}
}

// Cumulative /mutex keeps reporting old contention forever; delta_mutex
// reports it once. This is what stops Pyroscope over-counting scraped totals.
func TestProfilingMux_DeltaMutexIsPerInterval(t *testing.T) {
	prev := runtime.SetMutexProfileFraction(1)
	t.Cleanup(func() { runtime.SetMutexProfileFraction(prev) })

	mux := newProfilingMux()
	marker := []byte("contendOnMutex")
	fetchProfile(t, mux, "/debug/pprof/delta_mutex") // baseline: drain earlier contention

	// A waiter is not guaranteed to be parked before Unlock, so retry (bounded, no sleeps).
	seen := false
	for i := 0; i < 200 && !seen; i++ {
		contendOnMutex()
		seen = bytes.Contains(fetchProfile(t, mux, "/debug/pprof/delta_mutex"), marker)
	}
	require.True(t, seen, "delta_mutex never reported the contention we generated")

	assert.False(t, bytes.Contains(fetchProfile(t, mux, "/debug/pprof/delta_mutex"), marker),
		"second delta with no new contention must not repeat the old samples")
	assert.True(t, bytes.Contains(fetchProfile(t, mux, "/debug/pprof/mutex"), marker),
		"cumulative /mutex still reports lifetime contention")
}

func TestOrDefault(t *testing.T) {
	assert.Equal(t, DefaultMutexFraction, orDefault(0, DefaultMutexFraction))
	assert.Equal(t, DefaultMutexFraction, orDefault(-3, DefaultMutexFraction))
	assert.Equal(t, 1, orDefault(1, DefaultMutexFraction))
}
