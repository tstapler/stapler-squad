package services

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
)

const (
	searchBatchFiles    = 50
	searchBatchInterval = 100 * time.Millisecond // flush a partial batch this often so slow walks still stream
)

var errSearchSuperseded = errors.New("superseded by a newer search")

// SearchKey scopes newest-wins to one browser tab, so a phone and a desktop viewing
// the same session never cancel each other.
type SearchKey struct {
	SessionID string
	ClientID  string
}

// SearchRegistry keeps at most one live search per SearchKey.
type SearchRegistry struct {
	mu     sync.Mutex
	active map[SearchKey]*searchRun
}

type searchRun struct {
	cancel   context.CancelCauseFunc
	host     string
	finished chan struct{} // closed by end, after the run has released everything it held
}

// Begin registers a search for key, cancelling the previous one inside the same lock
// (a Load-then-Store pair would let two concurrent starts both survive). The returned
// end must be called when the search finishes.
func (r *SearchRegistry) Begin(parent context.Context, key SearchKey) (context.Context, func()) {
	ctx, end, _ := r.begin(parent, key, "")
	return ctx, end
}

// heldBy reports whether the live search for key came from host, so cancelling it frees a slot that host holds.
func (r *SearchRegistry) heldBy(key SearchKey, host string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.active[key]
	return run != nil && run.host == host
}

// begin is Begin that also returns a channel closed once the superseded run (nil if
// none) has finished, so the caller can wait for the slots it held.
func (r *SearchRegistry) begin(parent context.Context, key SearchKey, host string) (context.Context, func(), <-chan struct{}) {
	ctx, cancel := context.WithCancelCause(parent)
	run := &searchRun{cancel: cancel, host: host, finished: make(chan struct{})}
	r.mu.Lock()
	if r.active == nil {
		r.active = map[SearchKey]*searchRun{}
	}
	var prevDone <-chan struct{}
	if prev := r.active[key]; prev != nil {
		prev.cancel(errSearchSuperseded)
		prevDone = prev.finished
	}
	r.active[key] = run
	r.mu.Unlock()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			r.mu.Lock()
			if r.active[key] == run {
				delete(r.active, key)
			}
			r.mu.Unlock()
			cancel(nil)
			close(run.finished)
		})
	}, prevDone
}

// SearchHandler serves SearchContent: validation, stream and walk admission, then the
// ContentSearcher. It holds policy only; matching lives in the searcher.
type SearchHandler struct {
	ops       *FileOps
	searcher  ContentSearcher
	registry  SearchRegistry
	gate      *walkGate
	limiter   *ipLimiter
	viewerV2  func(context.Context) bool
	queueWait time.Duration
}

func newSearchHandler(ops *FileOps) *SearchHandler {
	return &SearchHandler{
		ops:       ops,
		searcher:  NewWalkSearcher(),
		gate:      newWalkGate(maxConcurrentWalks),
		limiter:   newIPLimiter(ipCaps{PerHost: maxSearchStreamsPerOrigin, Total: maxSearchStreamsTotal}),
		viewerV2:  FilesViewerV2Enabled,
		queueWait: maxQueueWait,
	}
}

type sendFunc func(*sessionv1.SearchContentResponse) error

// searchCall is what one call learns, for the log line, metrics and telemetry.
type searchCall struct {
	sessionID   string
	queryLen    int
	large       bool
	start       time.Time
	queued      bool
	firstResult time.Duration
	results     int
	stats       SearchStats
	outcome     SearchOutcome
}

// Handle runs one SearchContent call. remoteAddr is the connection's peer address.
func (h *SearchHandler) Handle(ctx context.Context, msg *sessionv1.SearchContentRequest, remoteAddr string, send sendFunc) (err error) {
	call := &searchCall{
		sessionID: msg.GetSessionId(), queryLen: len(msg.GetOptions().GetPattern()),
		large: msg.GetOptions().GetLargeFiles(), start: time.Now(),
	}
	defer func() { h.finish(ctx, call, err) }()

	if !h.viewerV2(ctx) {
		call.outcome = SearchOutcomeDisabled
		return readOnlyError(connect.CodeFailedPrecondition, "viewer v2 disabled", sessionv1.ReadOnlyReason_READ_ONLY_REASON_FLAG_OFF)
	}
	opts, err := NewSearchOptions(msg.GetOptions())
	if err != nil {
		call.outcome = SearchOutcomeInvalid
		return err
	}
	key := SearchKey{SessionID: msg.GetSessionId(), ClientID: msg.GetClientId()}
	host := remoteHost(remoteAddr)
	// Slot first, then registration; release runs before end so a superseder that waits on end finds the slot free.
	release, end := func() {}, func() {}
	defer func() { release(); end() }()
	var sctx context.Context
	var refused SearchBusyReason
	var freed func()
	if freed, refused = h.limiter.Acquire(remoteAddr); refused == "" {
		release = freed
		sctx, end, _ = h.registry.begin(ctx, key, host)
	} else if h.registry.heldBy(key, host) {
		// Only this host's own earlier search for the key can free a slot for it, so only then is superseding it a fair trade.
		var prevDone <-chan struct{}
		sctx, end, prevDone = h.registry.begin(ctx, key, host)
		select {
		case <-prevDone:
		case <-sctx.Done():
		}
		if freed, refused = h.limiter.Acquire(remoteAddr); refused == "" {
			release = freed
		}
	}
	if refused != "" {
		call.outcome = SearchOutcomeBusy
		return busyError(refused)
	}

	root, closer, err := h.ops.openSearchRoot(msg.GetSessionId())
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	if err := h.awaitWalkSlot(sctx, call, send); err != nil {
		return searchStopError(ctx, sctx, call, err)
	}
	defer h.gate.Release()
	return searchStopError(ctx, sctx, call, h.walk(sctx, root, opts, call, send))
}

// awaitWalkSlot blocks for a walk slot, telling the client its position (once on
// enqueue and again whenever it changes) and giving up after queueWait.
func (h *SearchHandler) awaitWalkSlot(ctx context.Context, call *searchCall, send sendFunc) error {
	w, ahead := h.gate.Enter()
	if w == nil {
		return nil
	}
	call.queued = true
	if err := send(queuedMessage(ahead)); err != nil {
		h.gate.Abandon(w)
		return err
	}
	timer := time.NewTimer(h.queueWait)
	defer timer.Stop()
	for {
		select {
		case <-w.granted:
			return nil
		case <-w.moved:
			if now := h.gate.Ahead(w); now != ahead {
				ahead = now
				if err := send(queuedMessage(ahead)); err != nil {
					h.gate.Abandon(w)
					return err
				}
			}
		case <-ctx.Done():
			h.gate.Abandon(w)
			return context.Cause(ctx)
		case <-timer.C:
			h.gate.Abandon(w)
			return busyError(busyQueueTimeout)
		}
	}
}

func (h *SearchHandler) walk(ctx context.Context, root fs.FS, opts SearchOptions, call *searchCall, send sendFunc) error {
	b := &batcher{send: send, call: call, lastFlush: call.start}
	stats, err := h.searcher.Search(ctx, root, opts, b.add)
	call.stats = stats
	if err != nil {
		return err
	}
	if err := b.flush(); err != nil {
		return err
	}
	return send(&sessionv1.SearchContentResponse{Message: &sessionv1.SearchContentResponse_Stats{Stats: &sessionv1.SearchStats{
		FilesScanned:      int32(stats.FilesScanned),
		BinarySkipped:     int32(stats.BinarySkipped),
		Truncated:         stats.Truncated,
		DurationMs:        stats.Duration.Milliseconds(),
		LargeFilesSkipped: int32(stats.LargeFilesSkipped),
	}}})
}

// batcher groups per-file matches into batches of searchBatchFiles files. The first
// group flushes at once and later partial batches every searchBatchInterval, so the UI
// fills while the walk is still running.
type batcher struct {
	send      sendFunc
	call      *searchCall
	pending   []*sessionv1.SearchMatch
	files     int
	sent      bool
	lastFlush time.Time
}

func (b *batcher) add(ms []SearchMatch) error {
	for _, m := range ms {
		b.pending = append(b.pending, &sessionv1.SearchMatch{
			Path: m.Path, Line: int32(m.Line), Text: m.Text,
			StartUtf16: int32(m.Start), EndUtf16: int32(m.End), ByteOffset: m.ByteOffset,
		})
	}
	b.call.results += len(ms)
	b.files++
	if !b.sent || b.files >= searchBatchFiles || time.Since(b.lastFlush) >= searchBatchInterval {
		return b.flush()
	}
	return nil
}

func (b *batcher) flush() error {
	if len(b.pending) == 0 {
		return nil
	}
	msg := &sessionv1.SearchContentResponse{Message: &sessionv1.SearchContentResponse_Batch{
		Batch: &sessionv1.SearchBatch{Matches: b.pending},
	}}
	b.pending, b.files, b.lastFlush = nil, 0, time.Now()
	if !b.sent {
		b.sent = true
		b.call.firstResult = time.Since(b.call.start)
	}
	return b.send(msg)
}

func queuedMessage(ahead int) *sessionv1.SearchContentResponse {
	return &sessionv1.SearchContentResponse{Message: &sessionv1.SearchContentResponse_Queued{
		Queued: &sessionv1.SearchQueued{Ahead: int32(ahead)},
	}}
}

// openSearchRoot resolves the session's worktree for a search. Unlike reads it never
// accepts the repo-root fallback: a paused or archived session whose worktree is gone
// is refused so the user is not shown another checkout's contents.
func (o *FileOps) openSearchRoot(sessionID string) (fs.FS, io.Closer, error) {
	ws, dir, err := workspaceDirFor(o.policy.Workspace, FileRef{SessionID: sessionID}, AccessRead)
	if err != nil {
		return nil, nil, err
	}
	if !worktreeOnDisk(ws) {
		return nil, nil, readOnlyError(connect.CodeFailedPrecondition, "WorktreeMissing: session worktree is not on disk",
			sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING)
	}
	root, err := o.backend.OpenRoot(dir)
	if err != nil {
		return nil, nil, openRootError(err)
	}
	walkable, ok := root.(interface{ FS() fs.FS })
	if !ok {
		_ = root.Close()
		return nil, nil, connect.NewError(connect.CodeUnimplemented, errors.New("backend cannot be walked"))
	}
	return walkable.FS(), root, nil
}

// searchStopError turns a stopped search into its wire error: superseded and client
// aborts are both Canceled; anything already a Connect error passes through.
func searchStopError(parent, sctx context.Context, call *searchCall, err error) error {
	if err == nil {
		return nil
	}
	var ce *connect.Error
	if errors.As(err, &ce) {
		return err
	}
	if parent.Err() != nil || errors.Is(context.Cause(sctx), errSearchSuperseded) || errors.Is(err, context.Canceled) {
		call.outcome = SearchOutcomeCancelled
		return connect.NewError(connect.CodeCanceled, context.Cause(sctx))
	}
	return connect.NewError(connect.CodeInternal, errors.New("search failed"))
}

func readOnlyError(code connect.Code, msg string, reason sessionv1.ReadOnlyReason) *connect.Error {
	return withDetail(connect.NewError(code, errors.New(msg)), &sessionv1.ReadOnlyReasonDetail{Reason: reason})
}

func busyError(reason SearchBusyReason) *connect.Error {
	return withDetail(connect.NewError(connect.CodeResourceExhausted, errors.New("too many searches are running")),
		&sessionv1.SearchBusyDetail{Reason: string(reason)})
}

func withDetail(ce *connect.Error, d proto.Message) *connect.Error {
	if detail, err := connect.NewErrorDetail(d); err == nil {
		ce.AddDetail(detail)
	}
	return ce
}

// finish records the one slog line, the metrics and the telemetry events. It logs
// the query length, never the query, a glob or a path.
func (h *SearchHandler) finish(ctx context.Context, call *searchCall, err error) {
	total := time.Since(call.start)
	outcome := call.outcome
	if outcome == "" {
		outcome = classifySearchOutcome(call, err)
	}
	log.Info("content search", "session", call.sessionID, "outcome", string(outcome), "query_len", call.queryLen,
		"duration_ms", total.Milliseconds(), "results", call.results, "files_scanned", call.stats.FilesScanned,
		"truncated", call.stats.Truncated, "backend", string(SearchBackendWalker), "queued", call.queued)
	recordSearchMetrics(ctx, outcome, total, call.firstResult)

	scale := BucketScale(call.stats.FilesScanned)
	tel := h.ops.policy.Telemetry
	tel.RecordSearch(ctx, call.sessionID, SearchOutcomeEvent{
		Outcome: outcome, Results: BucketCount(call.results), Scale: scale, Queued: call.queued,
		Backend: SearchBackendWalker, Large: call.large, Duration: total,
	})
	if call.firstResult > 0 {
		tel.RecordSearchFirstResult(ctx, call.sessionID, SearchFirstResultEvent{Scale: scale, Duration: call.firstResult})
	}
}

func classifySearchOutcome(call *searchCall, err error) SearchOutcome {
	switch {
	case err == nil && call.stats.Truncated:
		return SearchOutcomeTruncated
	case err == nil:
		return SearchOutcomeOK
	case connect.CodeOf(err) == connect.CodeCanceled:
		return SearchOutcomeCancelled
	case connect.CodeOf(err) == connect.CodeResourceExhausted:
		return SearchOutcomeBusy
	default:
		return SearchOutcomeError
	}
}
