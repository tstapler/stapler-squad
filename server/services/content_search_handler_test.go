package services

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// dirWorkspaces maps session ids to worktree directories that exist on disk.
type dirWorkspaces map[string]session.Workspace

func (d dirWorkspaces) GetWorkspace(id string) (session.Workspace, error) {
	ws, ok := d[id]
	if !ok {
		return session.Workspace{}, connect.NewError(connect.CodeNotFound, errors.New("no such session"))
	}
	return ws, nil
}

func onDisk(dir string) session.Workspace {
	return session.Workspace{RepoRoot: dir, ActiveDir: dir, ExistingDir: dir}
}

type recordingTelemetry struct {
	mu       sync.Mutex
	searches []SearchOutcomeEvent
	firsts   []SearchFirstResultEvent
	writes   []WriteOutcomeEvent
	patches  []PatchOutcomeEvent
}

func (r *recordingTelemetry) RecordWrite(_ context.Context, _ string, ev WriteOutcomeEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, ev)
}

func (r *recordingTelemetry) RecordPatch(_ context.Context, _ string, ev PatchOutcomeEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.patches = append(r.patches, ev)
}

func (r *recordingTelemetry) RecordSearch(_ context.Context, _ string, ev SearchOutcomeEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.searches = append(r.searches, ev)
}
func (r *recordingTelemetry) RecordSearchFirstResult(_ context.Context, _ string, ev SearchFirstResultEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.firsts = append(r.firsts, ev)
}

type handlerRig struct {
	h   *SearchHandler
	tel *recordingTelemetry
}

func newRig(t testing.TB, ws dirWorkspaces) *handlerRig {
	t.Helper()
	tel := &recordingTelemetry{}
	ops := NewFileOps(FilePolicy{Workspace: ws, Telemetry: tel}, osRootBackend{})
	h := newSearchHandler(ops)
	h.viewerV2 = func(context.Context) bool { return true }
	return &handlerRig{h: h, tel: tel}
}

type sink struct {
	mu   sync.Mutex
	msgs []*sessionv1.SearchContentResponse
}

func (s *sink) send(m *sessionv1.SearchContentResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
	return nil
}

func (s *sink) snapshot() []*sessionv1.SearchContentResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*sessionv1.SearchContentResponse(nil), s.msgs...)
}

func (s *sink) kinds() []string {
	var out []string
	for _, m := range s.snapshot() {
		switch m.GetMessage().(type) {
		case *sessionv1.SearchContentResponse_Batch:
			out = append(out, "batch")
		case *sessionv1.SearchContentResponse_Stats:
			out = append(out, "stats")
		case *sessionv1.SearchContentResponse_Queued:
			out = append(out, "queued")
		}
	}
	return out
}

func searchReq(session, client, pattern string) *sessionv1.SearchContentRequest {
	return &sessionv1.SearchContentRequest{
		SessionId: session, ClientId: client,
		Options: &sessionv1.SearchOptions{Pattern: pattern},
	}
}

func fillWorktree(t testing.TB, files map[string]string) session.Workspace {
	dir, _ := writeTree(t, files)
	return onDisk(dir)
}

// gatedSearcher stands in for the walk so admission, queueing and cancellation can be
// observed deterministically. Every call blocks until release is closed or its ctx ends.
type gatedSearcher struct {
	release   chan struct{}
	emitFirst bool // send one match before blocking, so a wire client sees response headers
	started   atomic.Int64
	current   atomic.Int64
	peak      atomic.Int64
}

func newGatedSearcher() *gatedSearcher { return &gatedSearcher{release: make(chan struct{})} }

func (g *gatedSearcher) Search(ctx context.Context, _ fs.FS, _ SearchOptions, emit func([]SearchMatch) error) (SearchStats, error) {
	g.started.Add(1)
	if g.emitFirst {
		if err := emit([]SearchMatch{{Path: "a.txt", Line: 1, Text: "x"}}); err != nil {
			return SearchStats{}, err
		}
	}
	n := g.current.Add(1)
	defer g.current.Add(-1)
	for {
		if p := g.peak.Load(); n <= p || g.peak.CompareAndSwap(p, n) {
			break
		}
	}
	select {
	case <-g.release:
		return SearchStats{}, nil
	case <-ctx.Done():
		return SearchStats{}, ctx.Err()
	}
}

func eventually(t testing.TB, cond func() bool, msg string) {
	t.Helper()
	require.Eventually(t, cond, 5*time.Second, time.Millisecond, msg)
}

func TestSearchContentHandler_ShouldStreamBatchesThenSingleStats_WhenThreeFilesMatch(t *testing.T) {
	rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"a.txt": "needle\n", "b.txt": "needle\n", "c.txt": "needle\n", "d.txt": "none\n"})})
	var out sink

	err := rig.h.Handle(context.Background(), searchReq("s1", "c1", "needle"), "10.0.0.1:1", out.send)

	require.NoError(t, err)
	kinds := out.kinds()
	require.GreaterOrEqual(t, len(kinds), 2)
	assert.Equal(t, "stats", kinds[len(kinds)-1])
	assert.Equal(t, "batch", kinds[0])
	assert.Equal(t, 1, strings.Count(strings.Join(kinds, ","), "stats"))
	total := 0
	for _, m := range out.snapshot() {
		total += len(m.GetBatch().GetMatches())
	}
	assert.Equal(t, 3, total)
	stats := out.snapshot()[len(kinds)-1].GetStats()
	assert.EqualValues(t, 4, stats.GetFilesScanned())
}

func TestSearchContentHandler_ShouldHonorIncludeExcludeGlobs_WhenWorktreeOnDisk(t *testing.T) {
	rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{
		"src/a.go": "Needle\n", "src/a.txt": "Needle\n", "gen/vendor/x.go": "Needle\n", "src/b.go": "needle xneedle\n",
	})})
	req := searchReq("s1", "c1", "needle")
	req.Options.Scope = &sessionv1.SearchScope{IncludeGlobs: []string{"**/*.go"}, ExcludeGlobs: []string{"gen/**"}}
	req.Options.CaseMode = sessionv1.SearchCaseMode_SEARCH_CASE_MODE_INSENSITIVE
	req.Options.WordMode = sessionv1.SearchWordMode_SEARCH_WORD_MODE_WHOLE
	var out sink

	require.NoError(t, rig.h.Handle(context.Background(), req, "10.0.0.1:1", out.send))

	var paths []string
	for _, m := range out.snapshot() {
		for _, sm := range m.GetBatch().GetMatches() {
			paths = append(paths, fmt.Sprintf("%s:%d", sm.GetPath(), sm.GetLine()))
		}
	}
	assert.ElementsMatch(t, []string{"src/a.go:1", "src/b.go:1"}, paths, "whole-word keeps `needle` and drops `xneedle`")
}

func TestSearchRegistry_ShouldCancelPriorSearch_WhenSameSearchKeyStartsNewSearch(t *testing.T) {
	var reg SearchRegistry
	key := SearchKey{SessionID: "s1", ClientID: "a"}
	first, endFirst := reg.Begin(context.Background(), key)
	defer endFirst()
	second, endSecond := reg.Begin(context.Background(), key)
	defer endSecond()
	require.Error(t, first.Err())
	assert.ErrorIs(t, context.Cause(first), errSearchSuperseded)
	assert.NoError(t, second.Err())

	var reg2 SearchRegistry
	ctxs := make([]context.Context, 50)
	var wg sync.WaitGroup
	for i := range ctxs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _ := reg2.Begin(context.Background(), key)
			ctxs[i] = c
		}()
	}
	wg.Wait()
	alive := 0
	for _, c := range ctxs {
		if c.Err() == nil {
			alive++
		}
	}
	assert.Equal(t, 1, alive, "50 racing starts on one key must leave exactly one survivor")
}

func TestSearchContentHandler_ShouldEndFirstStreamCanceled_WhenSecondSearchStartsForSameKey(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"a.txt": "x\n"})})
	gs := newGatedSearcher()
	rig.h.searcher = gs
	var out1, out2 sink
	err1 := make(chan error, 1)
	go func() {
		err1 <- rig.h.Handle(context.Background(), searchReq("s1", "tab", "needle"), "10.0.0.1:1", out1.send)
	}()
	eventually(t, func() bool { return gs.started.Load() == 1 }, "first walk starts")

	err2 := make(chan error, 1)
	go func() {
		err2 <- rig.h.Handle(context.Background(), searchReq("s1", "tab", "needle"), "10.0.0.1:2", out2.send)
	}()

	require.Equal(t, connect.CodeCanceled, connect.CodeOf(<-err1))
	eventually(t, func() bool { return gs.started.Load() == 2 }, "second walk starts")
	close(gs.release)
	require.NoError(t, <-err2)
}

func TestSearchRegistry_ShouldNotCancel_WhenDifferentClientIDsSearchSameSession(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"a.txt": "x\n"})})
	gs := newGatedSearcher()
	rig.h.searcher = gs
	var outA, outB sink
	errs := make(chan error, 2)
	go func() {
		errs <- rig.h.Handle(context.Background(), searchReq("s1", "desktop", "needle"), "10.0.0.1:1", outA.send)
	}()
	go func() {
		errs <- rig.h.Handle(context.Background(), searchReq("s1", "phone", "needle"), "10.0.0.2:1", outB.send)
	}()
	eventually(t, func() bool { return gs.started.Load() == 2 }, "both walks run")
	close(gs.release)
	assert.NoError(t, <-errs)
	assert.NoError(t, <-errs)
	assert.Equal(t, []string{"stats"}, outA.kinds())
	assert.Equal(t, []string{"stats"}, outB.kinds())
}

func TestSearchContentHandler_ShouldReturnFailedPrecondition_WhenViewerV2FlagOff(t *testing.T) {
	rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"a.txt": "needle\n"})})
	rig.h.viewerV2 = func(context.Context) bool { return false }
	var out sink

	err := rig.h.Handle(context.Background(), searchReq("s1", "c", "needle"), "10.0.0.1:1", out.send)

	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "viewer v2 disabled")
	assert.Equal(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_FLAG_OFF, searchReadOnlyReason(t, err))
	assert.Empty(t, out.kinds())
	require.Len(t, rig.tel.searches, 1)
	assert.Equal(t, SearchOutcomeDisabled, rig.tel.searches[0].Outcome)
}

func searchReadOnlyReason(t testing.TB, err error) sessionv1.ReadOnlyReason {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	for _, d := range ce.Details() {
		v, verr := d.Value()
		require.NoError(t, verr)
		if ro, ok := v.(*sessionv1.ReadOnlyReasonDetail); ok {
			return ro.GetReason()
		}
	}
	t.Fatal("no ReadOnlyReasonDetail on error")
	return 0
}

func busyDetail(t testing.TB, err error) string {
	t.Helper()
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	for _, d := range ce.Details() {
		v, verr := d.Value()
		require.NoError(t, verr)
		if b, ok := v.(*sessionv1.SearchBusyDetail); ok {
			return b.GetReason()
		}
	}
	t.Fatal("no SearchBusyDetail on error")
	return ""
}

func TestSearchContentHandler_ShouldRefuseWithWorktreeMissing_WhenExistingDirIsRepoRootFallback(t *testing.T) {
	repo := fillWorktree(t, map[string]string{"a.txt": "needle\n"}).ExistingDir
	gone := session.Workspace{RepoRoot: repo, WorktreeDir: repo + "-gone", ActiveDir: repo + "-gone", ExistingDir: repo}
	rig := newRig(t, dirWorkspaces{"paused": gone, "dirmode": onDisk(repo)})
	gs := newGatedSearcher()
	close(gs.release)
	rig.h.searcher = gs
	var out sink

	err := rig.h.Handle(context.Background(), searchReq("paused", "c", "needle"), "10.0.0.1:1", out.send)

	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Equal(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING, searchReadOnlyReason(t, err))
	assert.Empty(t, out.kinds(), "no batch, no stats")
	assert.Zero(t, gs.started.Load(), "nothing is walked")

	require.NoError(t, rig.h.Handle(context.Background(), searchReq("dirmode", "c", "needle"), "10.0.0.1:1", out.send), "directory-mode sessions search normally")
	assert.EqualValues(t, 1, gs.started.Load())
}

func TestSearchContentHandler_ShouldBoundWalksAndSendQueued_WhenSixSearchesAcrossThreeSessions(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	ws := dirWorkspaces{}
	for _, id := range []string{"s1", "s2", "s3"} {
		ws[id] = fillWorktree(t, map[string]string{"a.txt": "x\n"})
	}
	rig := newRig(t, ws)
	gs := newGatedSearcher()
	rig.h.searcher = gs

	type run struct {
		out  *sink
		err  chan error
		stop context.CancelFunc
	}
	runs := make([]run, 6)
	for i := range runs {
		ctx, cancel := context.WithCancel(context.Background())
		runs[i] = run{out: &sink{}, err: make(chan error, 1), stop: cancel}
		sid := fmt.Sprintf("s%d", i%3+1)
		addr := fmt.Sprintf("10.0.0.%d:1", i+1) // distinct hosts: the per-IP cap is not under test here
		go func() {
			runs[i].err <- rig.h.Handle(ctx, searchReq(sid, fmt.Sprintf("c%d", i), "needle"), addr, runs[i].out.send)
		}()
		if i == 1 {
			eventually(t, func() bool { return gs.started.Load() == 2 }, "two walks running")
		}
	}
	queued := func(r run) bool { return len(r.out.kinds()) > 0 && r.out.kinds()[0] == "queued" }
	eventually(t, func() bool {
		n := 0
		for _, r := range runs[2:] {
			if queued(r) {
				n++
			}
		}
		return n == 4
	}, "the four later searches are told they are queued")
	assert.EqualValues(t, 2, gs.current.Load())
	aheads := make([]int32, 0, len(runs)-2)
	for _, r := range runs[2:] {
		aheads = append(aheads, r.out.snapshot()[0].GetQueued().GetAhead())
	}
	assert.ElementsMatch(t, []int32{2, 3, 4, 5}, aheads, "each queued search counts the two running walks plus those queued before it")

	runs[5].stop() // cancel while queued: never walks
	require.Equal(t, connect.CodeCanceled, connect.CodeOf(<-runs[5].err))
	close(gs.release)
	for i := 0; i < 5; i++ {
		require.NoError(t, <-runs[i].err, "search %d", i)
	}
	assert.LessOrEqual(t, gs.peak.Load(), int64(2), "at most two walks at once")
	assert.EqualValues(t, 5, gs.started.Load(), "the cancelled queued search never walked")
}

func TestSearchContentHandler_ShouldEndQueuedStreamWithSearchBusy_When_QueueWaitExceeds20s(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"a.txt": "x\n"})})
		gs := newGatedSearcher()
		rig.h.searcher = gs
		for i := 0; i < maxConcurrentWalks; i++ {
			go func() {
				_ = rig.h.Handle(context.Background(), searchReq("s1", fmt.Sprintf("hold%d", i), "needle"), fmt.Sprintf("10.0.1.%d:1", i), (&sink{}).send)
			}()
		}
		synctest.Wait()
		require.EqualValues(t, maxConcurrentWalks, gs.started.Load())

		var out sink
		errc := make(chan error, 1)
		go func() {
			errc <- rig.h.Handle(context.Background(), searchReq("s1", "late", "needle"), "10.0.2.1:1", out.send)
		}()
		synctest.Wait()
		assert.Equal(t, []string{"queued"}, out.kinds())

		time.Sleep(maxQueueWait) //nolint:notimesleeptest advances the synctest fake clock
		synctest.Wait()

		err := <-errc
		require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
		assert.Equal(t, "queue_timeout", busyDetail(t, err))
		assert.EqualValues(t, maxConcurrentWalks, gs.started.Load(), "no third walk ran")
		assert.Equal(t, SearchOutcomeBusy, rig.tel.searches[len(rig.tel.searches)-1].Outcome)
		assert.Equal(t, 0, rig.h.gateWaiting(), "the waiter and its slot request are gone")
		close(gs.release)
		synctest.Wait()
		assert.Equal(t, 0, rig.h.limiter.heldTotal(), "stream slots released")
	})
}

func (h *SearchHandler) gateWaiting() int {
	h.gate.mu.Lock()
	defer h.gate.mu.Unlock()
	return len(h.gate.waiters)
}

func (l *ipLimiter) heldTotal() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}

func TestStreamLimiter_ShouldRefuseFourthStreamWithSearchBusy_When_SameRemoteIpHoldsThree(t *testing.T) {
	rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"a.txt": "x\n"})})
	gs := newGatedSearcher()
	rig.h.searcher = gs
	rig.h.gate = newWalkGate(maxSearchStreamsTotal) // the walk gate is not under test here
	errs := make(chan error, 5)
	for i := 0; i < maxSearchStreamsPerOrigin; i++ {
		go func() {
			errs <- rig.h.Handle(context.Background(), searchReq("s1", fmt.Sprintf("tab%d", i), "needle"), fmt.Sprintf("192.168.1.9:%d", 4000+i), (&sink{}).send)
		}()
	}
	eventually(t, func() bool { return gs.started.Load() == 3 }, "three streams open")

	var out sink
	err := rig.h.Handle(context.Background(), searchReq("s1", "tab4", "needle"), "192.168.1.9:4999", out.send)

	require.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.Equal(t, "per_ip", busyDetail(t, err))
	assert.Empty(t, out.kinds(), "no Queued message")
	assert.EqualValues(t, 3, gs.started.Load(), "no walk and no registry entry")
	assert.Equal(t, SearchOutcomeBusy, rig.tel.searches[len(rig.tel.searches)-1].Outcome)

	go func() {
		errs <- rig.h.Handle(context.Background(), searchReq("s1", "phone", "needle"), "192.168.1.20:1", (&sink{}).send)
	}()
	eventually(t, func() bool { return gs.started.Load() == 4 }, "a phone on another IP is unaffected")
	close(gs.release)
	for i := 0; i < 4; i++ {
		require.NoError(t, <-errs)
	}
}

func TestStreamLimiter_ShouldEnforceTotalCap_And_AcceptNewStream_When_OneOfThreeEnds(t *testing.T) {
	l := newIPLimiter(ipCaps{PerHost: maxSearchStreamsPerOrigin, Total: maxSearchStreamsTotal})
	var releases []func()
	for i := 0; i < maxSearchStreamsTotal; i++ {
		rel, why := l.Acquire(fmt.Sprintf("10.0.0.%d:1", i))
		require.Empty(t, why)
		releases = append(releases, rel)
	}
	_, why := l.Acquire("10.0.9.9:1")
	assert.Equal(t, busyTotal, why)

	releases[0]()
	releases[0]() // idempotent
	rel, why := l.Acquire("10.0.9.9:1")
	require.Empty(t, why, "a slot is accepted once one stream ends")
	rel()
	assert.Equal(t, maxSearchStreamsTotal-1, l.heldTotal())

	h := newIPLimiter(ipCaps{PerHost: 3, Total: 8})
	for i := 0; i < 3; i++ {
		_, why := h.Acquire("1.2.3.4:1")
		require.Empty(t, why)
	}
	_, why = h.Acquire("1.2.3.4:2")
	assert.Equal(t, busyPerIP, why)
}

func TestStreamLimiter_ShouldKeepCountersWithinCaps_When_FiftyGoroutinesOpenAndCloseUnderRace(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	l := newIPLimiter(ipCaps{PerHost: 3, Total: 8})
	var wg sync.WaitGroup
	var violations atomic.Int64
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addr := fmt.Sprintf("10.0.0.%d:1", g%5)
			for i := 0; i < 200; i++ {
				rel, why := l.Acquire(addr)
				if why != "" {
					continue
				}
				l.mu.Lock()
				if l.total < 0 || l.total > 8 || l.perHost[remoteHost(addr)] > 3 {
					violations.Add(1)
				}
				l.mu.Unlock()
				rel()
			}
		}()
	}
	wg.Wait()
	assert.Zero(t, violations.Load())
	assert.Zero(t, l.heldTotal())
	assert.Empty(t, l.perHost)
}

func TestStreamLimiter_ShouldKeyOnRemoteAddr_And_IgnoreForwardedHeaders(t *testing.T) {
	assert.Equal(t, remoteHost("1.2.3.4:5000"), remoteHost("1.2.3.4:6000"))
	assert.NotEqual(t, remoteHost("1.2.3.4:5000"), remoteHost("1.2.3.5:5000"))
	assert.Equal(t, "::1", remoteHost("[::1]:80"))

	rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"a.txt": "x\n"})})
	gs := newGatedSearcher()
	rig.h.searcher = gs
	rig.h.gate = newWalkGate(maxSearchStreamsTotal)
	gs.emitFirst = true
	fs := &FileService{ops: rig.h.ops, search: rig.h}
	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewSessionServiceHandler(&searchOnlyService{files: fs}))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)

	open := func(i int) (*connect.ServerStreamForClient[sessionv1.SearchContentResponse], error) {
		req := connect.NewRequest(searchReq("s1", fmt.Sprintf("tab%d", i), "needle"))
		req.Header().Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i)) // would split the key if it were trusted
		st, err := client.SearchContent(context.Background(), req)
		if err != nil {
			return nil, err
		}
		return st, nil
	}
	var streams []*connect.ServerStreamForClient[sessionv1.SearchContentResponse]
	for i := 0; i < maxSearchStreamsPerOrigin; i++ {
		st, err := open(i)
		require.NoError(t, err)
		streams = append(streams, st)
	}
	eventually(t, func() bool { return gs.started.Load() == 3 }, "three streams open over the wire")

	st, err := open(9)
	require.NoError(t, err)
	for st.Receive() {
	}
	require.Error(t, st.Err())
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(st.Err()))
	assert.Equal(t, "per_ip", busyDetail(t, st.Err()), "SearchBusyDetail survives the wire")

	close(gs.release)
	for _, s := range streams {
		for s.Receive() {
		}
		require.NoError(t, s.Err())
	}
}

type searchOnlyService struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	files *FileService
}

func (s *searchOnlyService) SearchContent(ctx context.Context, req *connect.Request[sessionv1.SearchContentRequest], st *connect.ServerStream[sessionv1.SearchContentResponse]) error {
	return s.files.SearchContent(ctx, req, st)
}

func TestSearchContentHandler_ShouldLogQueryLenNotText_When_SearchRuns(t *testing.T) {
	var logs strings.Builder
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { log.SetSlogDefaultForTest(prev) })
	rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"src/AKIA123.go": "key AKIA123\n"})})
	req := searchReq("s1", "c", "AKIA123")
	req.Options.Scope = &sessionv1.SearchScope{IncludeGlobs: []string{"**/*.go"}}
	var out sink

	require.NoError(t, rig.h.Handle(context.Background(), req, "10.0.0.1:1", out.send))

	line := logs.String()
	assert.Contains(t, line, "query_len=7")
	assert.NotContains(t, line, "AKIA123")
	assert.NotContains(t, line, "**/*.go")
	assert.NotContains(t, line, "src/")
	assert.NotContains(t, line, "byte_offset")
	for _, field := range []string{"duration_ms=", "results=1", "files_scanned=1", "truncated=false", "backend=walker"} {
		assert.Contains(t, line, field)
	}
}

func TestSearchContentHandler_ShouldRecordOutcomeAndFirstResult_When_OkTruncatedCancelledInvalidDisabled(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 40; i++ {
		files[fmt.Sprintf("f%02d.txt", i)] = strings.Repeat("needle\n", 40)
	}
	ws := dirWorkspaces{"s1": fillWorktree(t, files)}

	t.Run("ok", func(t *testing.T) {
		rig := newRig(t, dirWorkspaces{"s1": fillWorktree(t, map[string]string{"a.txt": "needle\n"})})
		require.NoError(t, rig.h.Handle(context.Background(), searchReq("s1", "c", "needle"), "10.0.0.1:1", (&sink{}).send))
		require.Len(t, rig.tel.searches, 1)
		ev := rig.tel.searches[0]
		assert.Equal(t, SearchOutcomeOK, ev.Outcome)
		assert.Equal(t, BucketCount(1), ev.Results)
		assert.Equal(t, BucketScale(1), ev.Scale)
		assert.False(t, ev.Queued)
		assert.Equal(t, SearchBackendWalker, ev.Backend)
		assert.Positive(t, ev.Duration)
		require.Len(t, rig.tel.firsts, 1, "exactly one first-result event")
	})
	t.Run("truncated", func(t *testing.T) {
		rig := newRig(t, ws)
		req := searchReq("s1", "c", "needle")
		req.Options.Limits = &sessionv1.SearchLimits{MaxResults: 27}
		require.NoError(t, rig.h.Handle(context.Background(), req, "10.0.0.1:1", (&sink{}).send))
		require.Len(t, rig.tel.searches, 1)
		assert.Equal(t, SearchOutcomeTruncated, rig.tel.searches[0].Outcome)
		assert.Equal(t, BucketCount(27), rig.tel.searches[0].Results)
	})
	t.Run("cancelled", func(t *testing.T) {
		rig := newRig(t, ws)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := rig.h.Handle(ctx, searchReq("s1", "c", "needle"), "10.0.0.1:1", (&sink{}).send)
		require.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
		require.Len(t, rig.tel.searches, 1)
		assert.Equal(t, SearchOutcomeCancelled, rig.tel.searches[0].Outcome)
		assert.Empty(t, rig.tel.firsts)
	})
	t.Run("invalid", func(t *testing.T) {
		rig := newRig(t, ws)
		req := searchReq("s1", "c", "(")
		req.Options.MatchMode = sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_REGEX
		err := rig.h.Handle(context.Background(), req, "10.0.0.1:1", (&sink{}).send)
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		require.Len(t, rig.tel.searches, 1)
		assert.Equal(t, SearchOutcomeInvalid, rig.tel.searches[0].Outcome)
	})
	t.Run("disabled", func(t *testing.T) {
		rig := newRig(t, ws)
		rig.h.viewerV2 = func(context.Context) bool { return false }
		_ = rig.h.Handle(context.Background(), searchReq("s1", "c", "needle"), "10.0.0.1:1", (&sink{}).send)
		require.Len(t, rig.tel.searches, 1)
		assert.Equal(t, SearchOutcomeDisabled, rig.tel.searches[0].Outcome)
	})
}

func TestWalkGate_ShouldReportPositionChange_And_NeverLeakSlot_When_WaiterAbandonsAfterGrant(t *testing.T) {
	g := newWalkGate(1)
	first, ahead := g.Enter()
	require.Nil(t, first)
	require.Zero(t, ahead)
	b, aheadB := g.Enter()
	c, aheadC := g.Enter()
	assert.Equal(t, 1, aheadB)
	assert.Equal(t, 2, aheadC)

	g.Release() // b is granted, c moves up
	<-b.granted
	<-c.moved
	assert.Equal(t, 1, g.Ahead(c))

	g.Abandon(b) // granted but gave up: the slot must go back
	<-c.granted
	assert.Equal(t, 1, g.running)
	g.Release()
	assert.Zero(t, g.running)
}
