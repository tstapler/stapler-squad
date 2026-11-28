// Package vcsstatus computes and caches the VCS status of a session's working
// directory: a TTL cache with stale-while-revalidate, coalescing of concurrent
// computations, and memoized per-range git stages.
package vcsstatus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/searchrpc"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/vc"
	"github.com/tstapler/stapler-squad/telemetry"
)

// vcsStatusCacheEntry caches the full VCSStatus result for a working directory.
type vcsStatusCacheEntry struct {
	status   *vc.VCSStatus
	cachedAt time.Time
}

// vcsStatusCacheTTL caps repeated git subprocess overhead for frequent GetVCSStatus
// polls. 15 s matches the GitProvider.branchCacheTTL and keeps the UI feeling fresh.
const vcsStatusCacheTTL = 15 * time.Second

// vcsStatusStaleTTL is how old a cached status may be and still be served instantly
// to a plain poll (while a background refresh runs). Beyond it, or when the request sets
// force_fresh, callers wait for a fresh computation. Matches the UI's 60s active poll.
const vcsStatusStaleTTL = time.Minute

// Cache serves VCS status per working directory. The zero value is ready to use.
type Cache struct {
	// entries caches full VCSStatus results per workdir to avoid spawning 6+
	// git subprocesses on every poll. Keyed by workdir path.
	entries sync.Map // map[string]vcsStatusCacheEntry
	// flights coalesces concurrent cache misses per workdir and bounds parallel computations.
	flights vcsFlights
	// epochs holds a per-workdir counter bumped by Invalidate so a computation that
	// straddles an invalidation does not re-cache a pre-change result. mu makes the epoch
	// check and the cache store (and the bump and the delete) atomic with each other.
	mu     sync.Mutex
	epochs map[string]uint64
	// rangeStages memoizes shipped commits / diff stat per immutable (base, head) SHA range.
	rangeStages vcsRangeStages
}

// Request is one status query.
type Request struct {
	WorkDir      string
	WaitForFresh bool
	// RequestedAt is when the caller's RPC began: with WaitForFresh, a computation
	// started before it is not joined.
	RequestedAt time.Time
	// BaseSHA returns the session's recorded base commit ("" for none). It is called
	// when a computation runs, not when the request arrives.
	BaseSHA func() string
}

// Serve answers GetVCSStatus from the cache or a (shared) computation. Cache
// attributes are set on ctx's current span.
func (c *Cache) Serve(ctx context.Context, req Request) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	span := trace.SpanFromContext(ctx)
	workDir := req.WorkDir
	if cachedStatus, ok := c.fresh(workDir); ok {
		recordVCSOutcome(ctx, vcsOutcomeCacheHit)
		span.SetAttributes(attribute.String("cache", "hit"))
		return connect.NewResponse(&sessionv1.GetVCSStatusResponse{VcsStatus: searchrpc.VCSStatusToProto(cachedStatus)}), nil
	}
	// Stale-while-revalidate: a poller never waits on git for an entry that is merely
	// past its TTL. StatusAsOf tells the UI how old the data is.
	if stale, ok := c.cached(workDir, vcsStatusStaleTTL); ok && !req.WaitForFresh {
		c.refreshInBackground(ctx, req.BaseSHA, workDir)
		recordVCSOutcome(ctx, vcsOutcomeStaleHit)
		span.SetAttributes(attribute.String("cache", "stale"))
		return connect.NewResponse(&sessionv1.GetVCSStatusResponse{VcsStatus: searchrpc.VCSStatusToProto(stale)}), nil
	}
	span.SetAttributes(attribute.String("cache", "miss"))

	// A caller waiting for fresh data must not join a computation that began before its request.
	var notBefore time.Time
	if req.WaitForFresh {
		notBefore = req.RequestedAt
	}
	status, shared, err := c.flights.do(ctx, workDir, notBefore, func(workCtx context.Context) (*vc.VCSStatus, error) {
		return c.compute(workCtx, req.BaseSHA, workDir)
	})
	span.SetAttributes(attribute.Bool("coalesced", shared))
	if err != nil {
		return c.failure(ctx, span, err)
	}
	if shared {
		recordVCSOutcome(ctx, vcsOutcomeCoalesced)
	} else {
		recordVCSOutcome(ctx, vcsOutcomeComputed)
	}
	return connect.NewResponse(&sessionv1.GetVCSStatusResponse{VcsStatus: searchrpc.VCSStatusToProto(status)}), nil
}

// Invalidate drops the cached status of workDir (keyed as Serve keys it),
// so a file save shows in the next status poll instead of after the TTL.
func (c *Cache) Invalidate(workDir string) {
	c.mu.Lock()
	if c.epochs == nil {
		c.epochs = make(map[string]uint64)
	}
	c.epochs[workDir]++
	c.entries.Delete(workDir)
	c.mu.Unlock()
	// A computation already running began before the change: new callers must not join it.
	c.flights.forget(workDir)
}

// cachedAfter reports whether workDir has a cache entry computed after t.
func (c *Cache) cachedAfter(workDir string, t time.Time) bool {
	cached, ok := c.entries.Load(workDir)
	return ok && cached.(vcsStatusCacheEntry).cachedAt.After(t)
}

// refreshInBackground joins (or starts) the workdir's computation without
// blocking the caller. It is itself a waiter, so the work is not cancelled when the
// triggering request ends; vcsComputeTimeout bounds it.
func (c *Cache) refreshInBackground(ctx context.Context, baseSHA func() string, workDir string) {
	// Background refreshes yield to interactive misses: skip when every slot is busy.
	if c.flights.saturated() {
		return
	}
	bgCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), vcsComputeTimeout)
	go func() {
		defer cancel()
		_, _, _ = c.flights.do(bgCtx, workDir, time.Time{}, func(workCtx context.Context) (*vc.VCSStatus, error) {
			return c.compute(workCtx, baseSHA, workDir)
		})
	}()
}

// fresh returns a copy of the cached status for workDir if still within the TTL.
func (c *Cache) fresh(workDir string) (*vc.VCSStatus, bool) {
	return c.cached(workDir, vcsStatusCacheTTL)
}

// cached returns a copy of the cached status for workDir if younger than maxAge.
func (c *Cache) cached(workDir string, maxAge time.Duration) (*vc.VCSStatus, bool) {
	cached, ok := c.entries.Load(workDir)
	if !ok {
		return nil, false
	}
	entry := cached.(vcsStatusCacheEntry)
	if time.Since(entry.cachedAt) >= maxAge {
		return nil, false
	}
	// Shallow-copy before touching StatusAsOf — entry.status is a pointer shared
	// with every other concurrent cache hit (and with the map itself); writing
	// through it directly races with those readers.
	respStatus := *entry.status
	respStatus.StatusAsOf = entry.cachedAt
	return &respStatus, true
}

// failure maps a failed flight to the RPC result: a caller that left
// gets a Connect cancel/deadline error (so it shows in the error-code series);
// a git failure keeps the long-standing in-band Error string.
func (c *Cache) failure(
	ctx context.Context, span trace.Span, err error,
) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	switch {
	case ctx.Err() != nil:
		outcome, code := vcsOutcomeCanceled, connect.CodeCanceled
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = connect.CodeDeadlineExceeded
		}
		recordVCSOutcome(context.WithoutCancel(ctx), outcome)
		span.AddEvent("caller_canceled", trace.WithAttributes(attribute.String("connect.code", code.String())))
		return nil, connect.NewError(code, ctx.Err())
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// The shared computation was cut short (all other waiters left, or the 30s bound hit).
		recordVCSOutcome(ctx, vcsOutcomeError)
		span.SetStatus(codes.Error, err.Error())
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	recordVCSOutcome(ctx, vcsOutcomeError)
	span.SetStatus(codes.Error, err.Error())
	return connect.NewResponse(&sessionv1.GetVCSStatusResponse{Error: err.Error()}), nil
}

// compute does the expensive work for one workDir and fills the cache.
// Errors carry the user-facing message GetVCSStatus has always put in Error.
func (c *Cache) compute(ctx context.Context, baseSHA func() string, workDir string) (*vc.VCSStatus, error) {
	c.mu.Lock()
	epochAtStart := c.epochs[workDir]
	c.mu.Unlock()
	startedAt := time.Now()
	// A flight that finished between the caller's cache check and this start has filled it.
	if st, ok := c.fresh(workDir); ok {
		return st, nil
	}

	// provider.GetStatus below takes no ctx, so don't start it for a call already gone.
	if err := ctx.Err(); err != nil {
		return nil, connect.NewError(connect.CodeCanceled, err)
	}

	var provider vc.VCSProvider
	gitProvider, err := vc.NewGitProvider(workDir)
	if err != nil {
		jjProvider, jjErr := vc.NewJujutsuProvider(workDir)
		if jjErr != nil {
			return nil, fmt.Errorf("not a version-controlled directory: %s", workDir)
		}
		provider = jjProvider
	} else {
		provider = gitProvider
	}

	_, statusSpan := telemetry.GetTracer().Start(ctx, "vcs.status.git_status",
		trace.WithAttributes(attribute.String("vcs.type", provider.Name())))
	var status *vc.VCSStatus
	gitStatusStart := time.Now()
	if gp, ok := provider.(*vc.GitProvider); ok {
		status, err = gp.GetStatusContext(ctx)
	} else {
		status, err = provider.GetStatus()
	}
	recordVCSStage(ctx, vcsStageGitStatus, stageResult(err), time.Since(gitStatusStart))
	if err != nil {
		statusSpan.RecordError(err)
		statusSpan.End()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to get VCS status: %v", err)
	}
	statusSpan.SetAttributes(attribute.Int("files.changed", len(status.StagedFiles)+len(status.UnstagedFiles)+len(status.UntrackedFiles)+len(status.ConflictFiles)))
	statusSpan.End()

	if _, isGit := provider.(*vc.GitProvider); isGit {
		c.addShippedCommits(ctx, baseSHA, workDir, status)
	}
	// A cancelled or timed-out context makes the shipped-commit calls fail, which would mark
	// the status CommitsUnavailable: never cache or return such a partial result.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	now := time.Now()
	status.StatusAsOf = now
	c.storeIfCurrent(workDir, epochAtStart, startedAt, status, now)
	return status, nil
}

// storeIfCurrent caches status unless an invalidation landed since epochAtStart (the
// result may predate it) or a computation that started later already cached a fresher entry.
// Check and store happen under mu, which Invalidate also holds.
func (c *Cache) storeIfCurrent(workDir string, epochAtStart uint64, startedAt time.Time, status *vc.VCSStatus, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epochs[workDir] != epochAtStart || c.cachedAfter(workDir, startedAt) {
		return
	}
	c.entries.Store(workDir, vcsStatusCacheEntry{status: status, cachedAt: now})
}

// addShippedCommits fills Commits/AggregateDiffStat. They are computed here (not inside
// GitProvider.GetStatus) because they need the session's recorded base SHA, an
// Instance-level concept the vc.VCSProvider abstraction doesn't have. Jujutsu has no
// equivalent base SHA concept, so this only runs for git sessions.
func (c *Cache) addShippedCommits(ctx context.Context, base func() string, workDir string, status *vc.VCSStatus) {
	baseSHA := base()
	if baseSHA == "" {
		return
	}
	statusCtx := ctx
	ctx, span := telemetry.GetTracer().Start(ctx, "vcs.status.shipped_commits")
	defer span.End()
	headSHA, headErr := git.GetHeadCommitSHA(workDir)
	if headErr != nil {
		log.Debug("GetVCSStatus: failed to resolve HEAD commit SHA", "workDir", workDir, "err", headErr)
		status.CommitsUnavailable = true
		return
	}
	if headSHA == baseSHA {
		return
	}
	key := vcsRangeKey{workDir: workDir, baseSHA: baseSHA, headSHA: headSHA}
	span.SetAttributes(attribute.String("vcs.head_sha", headSHA))
	if res, listErr := c.rangeStages.shippedCommits(ctx, key); listErr == nil {
		status.Commits = res.commits
		status.CommitsTruncated = res.truncated
	} else {
		log.Debug("GetVCSStatus: failed to list shipped commits", "workDir", workDir, "err", listErr)
		status.CommitsUnavailable = true
	}
	// Sibling of shipped_commits (not a child) so each span times only its own stage.
	diffCtx, diffSpan := telemetry.GetTracer().Start(statusCtx, "vcs.status.diff_stat",
		trace.WithAttributes(attribute.String("vcs.head_sha", headSHA)))
	defer diffSpan.End()
	if diffStat, diffErr := c.rangeStages.aggregateDiffStat(diffCtx, key); diffErr == nil {
		status.AggregateDiffStat = &diffStat
	} else {
		diffSpan.RecordError(diffErr)
		log.Debug("GetVCSStatus: failed to compute aggregate diff stat", "workDir", workDir, "err", diffErr)
	}
}
