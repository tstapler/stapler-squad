package services

import (
	"context"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func keyOf(path string, size int64) analysisKey {
	return analysisKey{Path: path, Size: size, MtimeNs: 1}
}

// gatedRunner blocks until release is closed or ctx is cancelled.
func gatedRunner(started chan<- struct{}, release <-chan struct{}) analysisRunner {
	return func(ctx context.Context) (*Analysis, error) {
		started <- struct{}{}
		select {
		case <-release:
			return &Analysis{LineCount: 3}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func analysisEventually(t *testing.T, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 30*time.Second, time.Millisecond) // ceiling only; returns as soon as cond holds
}

func TestAnalysisCache_ShouldReportPendingThenReady_And_RebuildInFullOnValidatorChange(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	c := newAnalysisCache(analysisCacheEntries, 1)
	defer c.Close()
	started, release := make(chan struct{}, 2), make(chan struct{})

	k1 := keyOf("/w/a.log", 100)
	state, a := c.Start(k1, gatedRunner(started, release))
	assert.Equal(t, analysisStatePending, state)
	assert.Nil(t, a)
	<-started
	close(release)
	analysisEventually(t, func() bool { s, _ := c.State(k1); return s == analysisStateReady })
	_, a = c.State(k1)
	assert.Equal(t, int64(3), a.LineCount)

	// File appended: the old entry is dropped and the new key starts from scratch.
	k2 := keyOf("/w/a.log", 150)
	var runs atomic.Int32
	state, _ = c.Start(k2, func(context.Context) (*Analysis, error) { runs.Add(1); return &Analysis{Size: 150}, nil })
	assert.Equal(t, analysisStatePending, state)
	analysisEventually(t, func() bool { s, _ := c.State(k2); return s == analysisStateReady })
	s, _ := c.State(k1)
	assert.Equal(t, analysisStateUnspecified, s, "old validator entry invalidated")
	assert.Equal(t, int32(1), runs.Load())
}

func TestAnalysisCache_ShouldCancelPass_When_LastWaiterLeavesAndNotCacheOwned(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	c := newAnalysisCache(analysisCacheEntries, 1)
	defer c.Close()
	started := make(chan struct{}, 1)
	k := keyOf("/w/b.log", 10)

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	errs := make(chan error, 2)
	run := gatedRunner(started, nil)
	for _, ctx := range []context.Context{ctx1, ctx2} {
		go func() { _, err := c.Await(ctx, k, run); errs <- err }()
	}
	<-started
	// started only proves waiter 1 registered; cancelling before waiter 2 joins would drop the entry.
	analysisEventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		e, ok := c.entries[k]
		return ok && e.waiters == 2
	})
	cancel1()
	require.ErrorIs(t, <-errs, context.Canceled)
	s, _ := c.State(k)
	assert.Equal(t, analysisStatePending, s, "one waiter remains, pass keeps running")
	cancel2()
	require.ErrorIs(t, <-errs, context.Canceled)
	analysisEventually(t, func() bool { s, _ := c.State(k); return s == analysisStateUnspecified })
}

func TestAnalysisCache_ShouldDetachEntryImmediately_When_LastWaiterCancelsAPassThatIsSlowToStop(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	c := newAnalysisCache(analysisCacheEntries, 1)
	defer c.Close()
	started, proceed := make(chan struct{}, 1), make(chan struct{})
	k := keyOf("/w/slow-stop.log", 10)
	slowStop := func(ctx context.Context) (*Analysis, error) {
		started <- struct{}{}
		<-ctx.Done()
		<-proceed // the pass takes a while to notice the cancel
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Await(ctx, k, slowStop); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)

	c.mu.Lock()
	_, stillCached := c.entries[k]
	c.mu.Unlock()
	assert.False(t, stillCached, "a joiner must not be handed the doomed entry")
	close(proceed)
}

func TestAnalysisCache_ShouldNotCacheFailure_When_PassFailsWithTransientError(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	c := newAnalysisCache(analysisCacheEntries, 1)
	defer c.Close()
	k := keyOf("/w/emfile.log", 10)
	var runs atomic.Int32
	run := func(context.Context) (*Analysis, error) {
		if runs.Add(1) == 1 {
			return nil, &fs.PathError{Op: "open", Path: "emfile.log", Err: syscall.EMFILE}
		}
		return &Analysis{LineCount: 7}, nil
	}

	_, err := c.Await(context.Background(), k, run)
	require.ErrorIs(t, err, syscall.EMFILE)
	analysisEventually(t, func() bool { s, _ := c.State(k); return s == analysisStateUnspecified })

	got, err := c.Await(context.Background(), k, run)
	require.NoError(t, err)
	assert.Equal(t, int64(7), got.LineCount)
	assert.Equal(t, int32(2), runs.Load())
}

func TestAnalysisCache_ShouldCacheFailedState_When_PassFailsDeterministically(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	c := newAnalysisCache(analysisCacheEntries, 1)
	defer c.Close()
	k := keyOf("/w/denied.log", 10)
	var runs atomic.Int32
	run := func(context.Context) (*Analysis, error) {
		runs.Add(1)
		return nil, &fs.PathError{Op: "open", Path: "denied.log", Err: syscall.EACCES}
	}

	state, _ := c.Start(k, run)
	assert.Equal(t, analysisStatePending, state)
	analysisEventually(t, func() bool { s, _ := c.State(k); return s == analysisStateFailed })
	state, _ = c.Start(k, run)
	assert.Equal(t, analysisStateFailed, state, "terminal until the validator changes")
	assert.Equal(t, int32(1), runs.Load(), "a persistent failure is not re-analysed per request")

	c.Start(keyOf("/w/denied.log", 11), run) // the file changed: a fresh attempt
	analysisEventually(t, func() bool { return runs.Load() == 2 })
}

func TestAnalysisCache_ShouldRetryFailedEntry_When_FailedTTLExpires(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	c := newAnalysisCache(analysisCacheEntries, 1)
	defer c.Close()
	clock := time.Now()
	var mu sync.Mutex
	c.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	k := keyOf("/w/chmod.log", 10)
	var runs atomic.Int32
	run := func(context.Context) (*Analysis, error) {
		if runs.Add(1) == 1 {
			return nil, &fs.PathError{Op: "open", Path: "chmod.log", Err: syscall.EACCES}
		}
		return &Analysis{LineCount: 9}, nil
	}

	c.Start(k, run)
	analysisEventually(t, func() bool { s, _ := c.State(k); return s == analysisStateFailed })
	c.Start(k, run)
	assert.Equal(t, int32(1), runs.Load(), "within the TTL the failure is not retried")

	mu.Lock()
	clock = clock.Add(analysisFailedTTL)
	mu.Unlock()
	c.Start(k, run) // permissions fixed meanwhile; the validator never changed
	analysisEventually(t, func() bool { s, _ := c.State(k); return s == analysisStateReady })
	assert.Equal(t, int32(2), runs.Load())
}

func TestAnalysisCache_ShouldKeepPassRunning_When_StartedCacheOwnedAndWaiterLeaves(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	c := newAnalysisCache(analysisCacheEntries, 1)
	defer c.Close()
	started, release := make(chan struct{}, 1), make(chan struct{})
	k := keyOf("/w/c.log", 10)
	run := gatedRunner(started, release)
	c.Start(k, run)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Await(ctx, k, run); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	s, _ := c.State(k)
	assert.Equal(t, analysisStatePending, s)
	close(release)
	analysisEventually(t, func() bool { s, _ := c.State(k); return s == analysisStateReady })
}

func TestAnalysisCache_ShouldRunOnePassAtATime_And_EvictLeastRecentlyUsed(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	c := newAnalysisCache(2, 1)
	defer c.Close()
	var active, peak atomic.Int32
	run := func(context.Context) (*Analysis, error) {
		n := active.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(5 * time.Millisecond) //nolint:notimesleeptest holds the slot so overlapping runs are observable
		active.Add(-1)
		return &Analysis{}, nil
	}
	keys := []analysisKey{keyOf("/w/1", 1), keyOf("/w/2", 1), keyOf("/w/3", 1)}
	for _, k := range keys {
		c.Start(k, run)
	}
	analysisEventually(t, func() bool { s, _ := c.State(keys[2]); return s == analysisStateReady })
	assert.Equal(t, int32(1), peak.Load(), "process-wide semaphore of 1")
	s, _ := c.State(keys[0])
	assert.Equal(t, analysisStateUnspecified, s, "oldest entry evicted past capacity")
}

func TestAnalysisCache_ShouldDiscardResultAndMarkStale_When_FileChangesDuringPass(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	dir := "/w"
	mem := newMemBackend()
	mem.AddFile(dir, "log.txt", []byte(strings.Repeat("line\n", 100)), readCorpusMTime)
	ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}, Flags: &stubFilesFlags{rangeRead: true}}, mem)
	defer ops.analysis.Close()

	var once atomic.Bool
	mem.SetOnRead(func(int) {
		if once.CompareAndSwap(false, true) { // an agent appends mid-pass
			mem.AddFile(dir, "log.txt", []byte(strings.Repeat("line\n", 200)), readCorpusMTime.Add(time.Second))
		}
	})
	fr, err := ops.readFileFacts("test-session", "log.txt")
	require.NoError(t, err)
	_, err = ops.analysis.Await(context.Background(), fr.Src.Key, ops.analysisRunnerFor(fr.Src))
	require.ErrorIs(t, err, errAnalysisStale)
	s, a := ops.analysis.State(fr.Src.Key)
	assert.Equal(t, analysisStateUnspecified, s, "stale result never cached under the old key")
	assert.Nil(t, a)

	// The next stat sees the new validator and analyzes the new bytes.
	mem.SetOnRead(nil)
	resp, err := ops.StatWith(context.Background(), &sessionv1.StatFileRequest{SessionId: "test-session", Path: "log.txt", WantAnalysis: true})
	require.NoError(t, err)
	assert.Equal(t, analysisStatePending, resp.Analysis)
	fr2, _ := ops.readFileFacts("test-session", "log.txt")
	a2, err := ops.analysis.Await(context.Background(), fr2.Src.Key, ops.analysisRunnerFor(fr2.Src))
	require.NoError(t, err)
	assert.Equal(t, int64(200), a2.LineCount)
}

func TestStatFile_ShouldReturnAnalysisStateAndSha256Token_When_WantFlagsAndRangeReadOn(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	dir := "/w"
	body := []byte(strings.Repeat("0123456789\n", 200_000)) // 2.2 MB: above the cheap 1 MiB hash path
	mem := newMemBackend()
	mem.AddFile(dir, "big.log", body, readCorpusMTime)
	flags := &stubFilesFlags{}
	ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}, Flags: flags}, mem)
	defer ops.analysis.Close()
	ctx := context.Background()
	stat := func(analysis, sha bool) *sessionv1.StatFileResponse {
		resp, err := ops.StatWith(ctx, &sessionv1.StatFileRequest{SessionId: "test-session", Path: "big.log", WantAnalysis: analysis, WantSha256: sha})
		require.NoError(t, err)
		return resp
	}

	assert.Equal(t, analysisStateUnspecified, stat(true, true).Analysis, "dark while files:range-read is off")

	flags.rangeRead = true
	assert.Equal(t, analysisStateUnspecified, stat(false, false).Analysis, "cheap path unchanged")
	analysisEventually(t, func() bool { return stat(true, false).Analysis == analysisStateReady })
	resp := stat(true, true)
	want := NewFileVersionToken(int64(len(body)), readCorpusMTime.UnixNano(), body)
	assert.Equal(t, want.String(), resp.VersionToken, "ready sha256 token equals NewFileVersionToken")
	assert.NotEqual(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE, resp.ReadOnlyReason)
	assert.True(t, strings.HasSuffix(stat(true, false).VersionToken, ":-"), "sha stays out unless want_sha256")
}

func TestStatFile_ShouldKeepDashToken_When_AnalysisPending(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	dir := "/w"
	mem := newMemBackend()
	mem.AddFile(dir, "big.log", []byte(strings.Repeat("x\n", 600_000)), readCorpusMTime)
	ops := NewFileOps(FilePolicy{Workspace: &fakeWorkspaceProvider{existingDir: dir}, Flags: &stubFilesFlags{rangeRead: true}}, mem)
	defer ops.analysis.Close()
	gate := make(chan struct{})
	mem.SetOnRead(func(n int) {
		if n == analyzeChunkBytes { // only the background pass, not the stat's sniff
			<-gate
		}
	})
	resp, err := ops.StatWith(context.Background(), &sessionv1.StatFileRequest{SessionId: "test-session", Path: "big.log", WantSha256: true})
	require.NoError(t, err)
	assert.Equal(t, analysisStatePending, resp.Analysis)
	assert.True(t, strings.HasSuffix(resp.VersionToken, ":-"))
	close(gate)
}
