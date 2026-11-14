package tymux

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// The consistency harness: a fake tymuxd produces seq-numbered Claude-Code-style redraw output
// (cursor-relative, so a duplicated or missing chunk visibly corrupts the screen), while tests
// inject resize-free faults: output_gap, subscriber queue overflow, reconnect, slow or failing
// captures. The real session, fanout and RenderSnapshotRedraw sit between the daemon and a
// client model; after every scenario the client's screen must equal the daemon's exactly.

const (
	harnessRows = 12
	harnessCols = 40
)

type fakeDaemon struct {
	sequenced bool

	emitMu sync.Mutex // serializes emit so stream order equals seq order
	mu     sync.Mutex // guards model, seq, stream
	model  *refTerm
	seq    uint64
	stream *fakeAttachStream

	attachCalls     atomic.Int64
	captureCalls    atomic.Int64
	captureFailures atomic.Int32
	captureHold     atomic.Pointer[chan struct{}]
}

func newFakeDaemon(sequenced bool) *fakeDaemon {
	return &fakeDaemon{sequenced: sequenced, model: newRefTerm(harnessRows, harnessCols)}
}

func (d *fakeDaemon) snapshotLocked() *v1.PaneSnapshot {
	snap := &v1.PaneSnapshot{
		Rows: harnessRows, Cols: harnessCols,
		CursorRow: uint32(d.model.cr), CursorCol: uint32(min(d.model.cc, harnessCols-1)),
		Liveness: v1.Liveness_LIVENESS_LIVE,
		Modes:    &v1.TerminalModes{},
	}
	for _, r := range d.model.grid {
		row := &v1.Row{}
		for _, ch := range r {
			row.Cells = append(row.Cells, &v1.Cell{Text: string(ch)})
		}
		snap.Grid = append(snap.Grid, row)
	}
	if d.sequenced {
		seq := d.seq
		snap.Seq = &seq
	}
	return snap
}

func (d *fakeDaemon) chunkEvent(seq uint64, b []byte) *v1.AttachEvent {
	if !d.sequenced {
		return &v1.AttachEvent{Payload: &v1.AttachEvent_Output{Output: b}}
	}
	return &v1.AttachEvent{Payload: &v1.AttachEvent_OutputChunk{OutputChunk: &v1.OutputChunk{Seq: seq, Data: b}}}
}

// attach is the transport's Attach: a new stream whose first event is a priming snapshot taken
// atomically with registering the stream, as tymuxd does.
func (d *fakeDaemon) attach(ctx context.Context) attachStream {
	d.attachCalls.Add(1)
	d.emitMu.Lock()
	defer d.emitMu.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	st := newFakeAttachStream(ctx)
	st.push(&v1.AttachEvent{Payload: &v1.AttachEvent_Snapshot{Snapshot: d.snapshotLocked()}})
	d.stream = st
	return st
}

func (d *fakeDaemon) capture(ctx context.Context, _ *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
	d.captureCalls.Add(1)
	if d.captureFailures.Add(-1) >= 0 {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("injected capture failure"))
	}
	d.captureFailures.Store(0)
	d.mu.Lock()
	snap := d.snapshotLocked()
	d.mu.Unlock()
	if hold := d.captureHold.Load(); hold != nil {
		select {
		case <-*hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return connect.NewResponse(snap), nil
}

// emit produces output the client stream receives.
func (d *fakeDaemon) emit(b []byte) {
	d.emitMu.Lock()
	defer d.emitMu.Unlock()
	d.mu.Lock()
	d.seq++
	seq := d.seq
	d.model.Write(b)
	st := d.stream
	d.mu.Unlock()
	if st != nil {
		st.push(d.chunkEvent(seq, b))
	}
}

// emitLost produces output the stream never delivers (a lagged broadcast receiver, or a
// disconnect), then, when signal is set, the OutputGap event; later output is already in the
// pane when the gap event is processed, as when a slow consumer finally reads.
func (d *fakeDaemon) emitLost(lost, later [][]byte) {
	d.emitMu.Lock()
	defer d.emitMu.Unlock()
	d.mu.Lock()
	for _, b := range lost {
		d.seq++
		d.model.Write(b)
	}
	type queued struct {
		seq uint64
		b   []byte
	}
	q := make([]queued, 0, len(later))
	for _, b := range later {
		d.seq++
		d.model.Write(b)
		q = append(q, queued{d.seq, b})
	}
	st := d.stream
	d.mu.Unlock()
	if st == nil {
		return
	}
	st.push(&v1.AttachEvent{Payload: &v1.AttachEvent_OutputGap{OutputGap: true}})
	for _, e := range q {
		st.push(d.chunkEvent(e.seq, e.b))
	}
}

// disconnect ends the current stream the way a dropped transport does; output until the next
// attach is lost to the client.
func (d *fakeDaemon) disconnect() {
	d.emitMu.Lock()
	defer d.emitMu.Unlock()
	d.mu.Lock()
	st := d.stream
	d.stream = nil
	d.mu.Unlock()
	if st != nil {
		close(st.events)
	}
}

func (d *fakeDaemon) screen() string { return d.model.Screen() }

// harnessClient is a subscriber: a goroutine parsing every delivered frame into its own model,
// which a test can stall to overflow its queue.
type harnessClient struct {
	term  *refTerm
	gate  sync.RWMutex
	ch    chan []byte
	id    string
	stopC chan struct{}
}

func newHarnessClient(s *tymuxGRPCSession) *harnessClient {
	c := &harnessClient{term: newRefTerm(harnessRows, harnessCols), stopC: make(chan struct{})}
	c.id, c.ch = s.fanout.Subscribe()
	go func() {
		for data := range c.ch {
			c.gate.RLock()
			c.term.Write(data)
			c.gate.RUnlock()
		}
		close(c.stopC)
	}()
	return c
}

func (c *harnessClient) pause()  { c.gate.Lock() }
func (c *harnessClient) resume() { c.gate.Unlock() }

type harness struct {
	t *testing.T
	d *fakeDaemon
	s *tymuxGRPCSession
	c *harnessClient
}

func newHarness(t *testing.T, sequenced bool) *harness {
	t.Helper()
	t.Setenv(restartModeEnv, "never")
	resetClaims(t)
	dir := t.TempDir()
	d := newFakeDaemon(sequenced)
	transport := &fakeTransport{
		createSessionFn: func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
		capturePaneFn: d.capture,
		attachFn:      d.attach,
	}
	m := NewTymuxGRPCSession(transport)
	s := m.(*tymuxGRPCSession)
	s.reconnectBaseDelay, s.reconnectMaxDelay = time.Millisecond, 5*time.Millisecond
	s.resync.retryBase, s.resync.retryCap, s.resync.freshFor = time.Millisecond, 10*time.Millisecond, 5*time.Millisecond
	require.NoError(t, m.Start(dir))
	t.Cleanup(func() { _ = m.Close() })
	h := &harness{t: t, d: d, s: s, c: newHarnessClient(s)}
	require.Eventually(t, func() bool { return d.attachCalls.Load() >= 1 }, 2*time.Second, time.Millisecond)
	return h
}

// converge waits for the client to equal the daemon, then holds for a settle window to catch a
// late duplicate or stale frame undoing it. Zero tolerance: any difference fails.
func (h *harness) converge() {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if h.c.term.Screen() == h.d.screen() {
			time.Sleep(60 * time.Millisecond) //nolint:notimesleeptest quiet-period check against real reader goroutines; no injectable clock
			if h.c.term.Screen() == h.d.screen() {
				return
			}
		}
		time.Sleep(2 * time.Millisecond) //nolint:notimesleeptest poll interval for convergence against real reader goroutines
	}
	assert.Equal(h.t, h.d.screen(), h.c.term.Screen(), "client screen diverged from the authoritative pane")
	h.t.FailNow()
}

// frames generates Claude Code style output: appended lines, cursor-up block redraws, spinner
// rewrites and absolute cursor writes. All are position-dependent.
func frames(rng *rand.Rand, n, base int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		id := base + i
		switch rng.Intn(4) {
		case 0:
			out[i] = []byte(fmt.Sprintf("line %04d %s\r\n", id, words(rng)))
		case 1:
			k := 1 + rng.Intn(4)
			b := fmt.Sprintf("\x1b[%dA", k)
			for j := 0; j < k; j++ {
				b += fmt.Sprintf("\r\x1b[2Kblock %04d.%d %s\r\n", id, j, words(rng))
			}
			out[i] = []byte(b)
		case 2:
			out[i] = []byte(fmt.Sprintf("\r\x1b[2K* thinking %04d %s", id, words(rng)))
		default:
			out[i] = []byte(fmt.Sprintf("\x1b[%d;%dHX%04d", 1+rng.Intn(harnessRows), 1+rng.Intn(20), id))
		}
	}
	return out
}

func words(rng *rand.Rand) string {
	w := []string{"alpha", "beta", "gamma", "delta", "eps"}
	return w[rng.Intn(len(w))] + " " + w[rng.Intn(len(w))]
}

func (h *harness) emitAll(fs [][]byte) {
	for _, f := range fs {
		h.d.emit(f)
	}
}

func TestStreamConsistency_Baseline(t *testing.T) {
	h := newHarness(t, true)
	h.emitAll(frames(rand.New(rand.NewSource(1)), 300, 0))
	h.converge()
}

// The race the sequence numbers close: output already in the pane (so inside the capture) is
// also queued in the stream behind the gap marker. Applying both double-renders it.
func TestStreamConsistency_OutputGapWithOutputQueuedBehindIt(t *testing.T) {
	for seed := int64(1); seed <= 10; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			h := newHarness(t, true)
			rng := rand.New(rand.NewSource(seed))
			h.emitAll(frames(rng, 20, 0))
			h.d.emitLost(frames(rng, 10, 100), frames(rng, 10, 200))
			h.converge()
			assert.EqualValues(t, 1, h.s.outputGapCount.Load())
		})
	}
}

// Negative control: the same fault against an unsequenced daemon must diverge, proving the
// harness can see the bug the sequenced path fixes.
func TestStreamConsistency_UnsequencedStreamStillDoubleAppliesQueuedOutput(t *testing.T) {
	h := newHarness(t, false)
	rng := rand.New(rand.NewSource(3))
	h.emitAll(frames(rng, 20, 0))
	h.d.emitLost(frames(rng, 5, 100), [][]byte{[]byte("dup-a\r\ndup-b\r\ndup-c\r\n")})
	time.Sleep(300 * time.Millisecond) //nolint:notimesleeptest absence check: lets the real reader goroutines drain before asserting divergence
	assert.NotEqual(t, h.d.screen(), h.c.term.Screen(),
		"harness failed to expose the legacy double-apply race; it cannot prove the sequenced fix")
}

func TestStreamConsistency_OutputKeepsFlowingWhileCaptureIsSlow(t *testing.T) {
	h := newHarness(t, true)
	rng := rand.New(rand.NewSource(7))
	h.emitAll(frames(rng, 20, 0))
	hold := make(chan struct{})
	h.d.captureHold.Store(&hold)
	h.d.emitLost(frames(rng, 5, 100), nil)
	require.Eventually(t, func() bool { return h.d.captureCalls.Load() >= 1 }, 2*time.Second, time.Millisecond)
	h.emitAll(frames(rng, 60, 300)) // arrives after the capture instant, before it returns
	close(hold)
	h.converge()
}

func TestStreamConsistency_SubscriberQueueOverflowRepaintsExactly(t *testing.T) {
	for seed := int64(1); seed <= 5; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			h := newHarness(t, true)
			rng := rand.New(rand.NewSource(seed))
			h.emitAll(frames(rng, 10, 0))
			h.c.pause()
			h.emitAll(frames(rng, 3*subscriberBufferSize, 100))
			h.c.resume()
			h.emitAll(frames(rng, 40, 1000))
			h.converge()
		})
	}
}

func TestStreamConsistency_ReconnectMidStream(t *testing.T) {
	h := newHarness(t, true)
	rng := rand.New(rand.NewSource(11))
	h.emitAll(frames(rng, 30, 0))
	h.d.disconnect()
	h.emitAll(frames(rng, 10, 100)) // produced while no stream exists
	require.Eventually(t, func() bool { return h.d.attachCalls.Load() >= 2 }, 5*time.Second, time.Millisecond)
	h.emitAll(frames(rng, 30, 200))
	h.converge()
}

func TestStreamConsistency_FailedCaptureRetriesWithBackoff(t *testing.T) {
	h := newHarness(t, true)
	rng := rand.New(rand.NewSource(13))
	h.emitAll(frames(rng, 20, 0))
	h.d.captureFailures.Store(3)
	h.d.emitLost(frames(rng, 5, 100), frames(rng, 5, 200))
	h.converge()
	assert.GreaterOrEqual(t, h.d.captureCalls.Load(), int64(4), "three failures then one success")
}

func TestStreamConsistency_GivesUpAfterMaxAttemptsAndRecoversOnNextGap(t *testing.T) {
	h := newHarness(t, true)
	h.s.resync.maxAttempts = 2
	rng := rand.New(rand.NewSource(17))
	h.emitAll(frames(rng, 20, 0))
	h.d.captureFailures.Store(100)
	h.d.emitLost(frames(rng, 5, 100), nil)
	require.Eventually(t, func() bool {
		h.s.resync.mu.Lock()
		defer h.s.resync.mu.Unlock()
		return !h.s.resync.running
	}, 5*time.Second, time.Millisecond)
	assert.NotEqual(t, h.d.screen(), h.c.term.Screen(), "the hole is still there after giving up")

	h.d.captureFailures.Store(0)
	h.d.emitLost(frames(rng, 3, 300), nil)
	h.converge()
}

// Concurrent consumers asking for a redraw share one CapturePane, and each reply reproduces the
// pane exactly as it was at the capture instant (it travels outside the stream, so it must not
// include output the stream will still deliver).
func TestStreamConsistency_ConcurrentRedrawRequestsShareOneCapture(t *testing.T) {
	h := newHarness(t, true)
	rng := rand.New(rand.NewSource(19))
	h.emitAll(frames(rng, 40, 0))
	h.converge()

	before := h.d.captureCalls.Load()
	hold := make(chan struct{})
	h.d.captureHold.Store(&hold)
	const callers = 8
	replies := make([]string, callers)
	var wg sync.WaitGroup
	for i := range replies {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, _, ok, err := h.s.CapturePaneRedraw(context.Background())
			assert.NoError(t, err)
			assert.True(t, ok)
			replies[i] = data
		}()
	}
	require.Eventually(t, func() bool { return h.d.captureCalls.Load() > before }, 2*time.Second, time.Millisecond)
	atCapture := h.d.screen()
	h.emitAll(frames(rng, 10, 500)) // output after the capture instant, before it returns
	close(hold)
	wg.Wait()

	assert.EqualValues(t, 1, h.d.captureCalls.Load()-before, "N concurrent requests must share one capture")
	for i, data := range replies {
		term := newRefTerm(harnessRows, harnessCols)
		term.Write([]byte(data))
		assert.Equal(t, atCapture, term.Screen(), "reply %d must be the pane at the capture instant", i)
	}
	h.converge()
}

// A burst of subscriber overflows is repaired from one capture plus the ring while no output has
// been lost upstream; a gap invalidates that capture.
func TestStreamConsistency_RecentCaptureIsReusedForDropRepairUntilAGap(t *testing.T) {
	h := newHarness(t, true)
	h.s.resync.freshFor = time.Minute
	rng := rand.New(rand.NewSource(23))
	h.emitAll(frames(rng, 10, 0))

	overflow := func(base int) {
		h.c.pause()
		h.emitAll(frames(rng, 2*subscriberBufferSize, base))
		h.c.resume()
		h.converge()
	}
	overflow(100)
	first := h.d.captureCalls.Load()
	require.GreaterOrEqual(t, first, int64(1))

	overflow(1000)
	assert.Equal(t, first, h.d.captureCalls.Load(), "the second overflow reuses the capture and replays the ring")

	h.d.emitLost(frames(rng, 3, 2000), nil)
	h.converge()
	assert.Greater(t, h.d.captureCalls.Load(), first, "a gap makes the cached capture unusable")
}

// Randomized fault schedules, reproducible by seed. Every schedule must end identical.
// The resync retry budget is lifted: injected capture failures accumulate across gaps while the
// resync goroutine lags, and flooding evicts the replay tail, so a schedule can legitimately burn
// the default 8 attempts; giving up is covered by the GivesUpAfterMaxAttempts test.
func TestStreamConsistency_RandomFaultSchedules(t *testing.T) {
	for seed := int64(100); seed < 130; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			h := newHarness(t, true)
			h.s.resync.maxAttempts = math.MaxInt
			rng := rand.New(rand.NewSource(seed))
			n := 0
			next := func(k int) [][]byte { n += k; return frames(rng, k, n) }
			paused := false
			for step := 0; step < 25; step++ {
				switch rng.Intn(8) {
				case 0:
					h.d.emitLost(next(1+rng.Intn(5)), next(rng.Intn(6)))
				case 1:
					if !paused {
						h.c.pause()
						paused = true
					}
					h.emitAll(next(subscriberBufferSize + rng.Intn(100)))
				case 2:
					if paused {
						h.c.resume()
						paused = false
					}
				case 3:
					hold := make(chan struct{})
					h.d.captureHold.Store(&hold)
					h.d.emitLost(next(2), next(rng.Intn(4)))
					h.emitAll(next(rng.Intn(30)))
					h.d.captureHold.Store(nil)
					close(hold)
				case 4:
					h.d.captureFailures.Store(int32(rng.Intn(3)))
					h.d.emitLost(next(1), nil)
				default:
					h.emitAll(next(1 + rng.Intn(40)))
				}
			}
			if paused {
				h.c.resume()
			}
			h.converge()
		})
	}
}

// A background resync (need 0) must not splice a capture that started before a resize: it
// shows the old size, and ResyncAt would replay post-resize output on top of it.
func TestStreamConsistency_ResizeRejectsPreResizeCaptures(t *testing.T) {
	h := newHarness(t, true)
	h.s.resync.freshFor = time.Minute
	rng := rand.New(rand.NewSource(31))
	h.emitAll(frames(rng, 10, 0))

	hold := make(chan struct{})
	h.d.captureHold.Store(&hold)
	h.d.emitLost(frames(rng, 3, 100), nil) // starts a gap flight that stays in flight
	require.Eventually(t, func() bool { return h.d.captureCalls.Load() >= 1 }, 2*time.Second, time.Millisecond)

	resizeDone := make(chan error, 1)
	go func() { resizeDone <- h.s.ResyncInStream(context.Background()) }()
	require.Eventually(t, func() bool {
		h.s.resync.mu.Lock()
		defer h.s.resync.mu.Unlock()
		return h.s.resync.sizeEpoch != 0
	}, 2*time.Second, time.Millisecond)

	type result struct {
		snap *resyncSnap
		err  error
	}
	bg := make(chan result, 1)
	go func() {
		snap, _, err := h.s.snapshotForSplice(context.Background(), 0, resyncReasonDrop)
		bg <- result{snap, err}
	}()
	close(hold)

	require.NoError(t, <-resizeDone)
	r := <-bg
	require.NoError(t, r.err)
	h.s.resync.mu.Lock()
	sizeEpoch := h.s.resync.sizeEpoch
	h.s.resync.mu.Unlock()
	assert.GreaterOrEqual(t, r.snap.epoch, sizeEpoch, "a background resync reused a pre-resize capture")
	h.converge()
}

func TestFanout_DisableSeqIsIdempotentAndGapAnnouncementExpires(t *testing.T) {
	f := NewClientFanout()
	f.DisableSeq()
	assert.Zero(t, f.SeqGeneration(), "DisableSeq on an unsequenced stream must not churn the generation")

	f.ResetSeq(10)
	f.NoteGapSignaled()
	assert.False(t, f.BroadcastSeq(11, []byte("a")), "contiguous chunk")
	assert.True(t, f.BroadcastSeq(15, []byte("b")), "a later jump is a real gap, not the old announcement's")
}
