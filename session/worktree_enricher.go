package session

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/singleflight"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/scanqueue"
	"github.com/tstapler/stapler-squad/telemetry"
)

// WorktreeEnricher runs the filesystem probes that used to block LoadInstances
// (worktree detection, GitHub owner/repo migration, missing-worktree pause check)
// after the instances are live. See docs/adr/ADR-038-post-start-worktree-enrichment.md.
//
// A bounded scanqueue pool drains one task per path; singleflight keeps a
// blocked path to a single goroutine. A probe that outlives Deadline releases its
// worker, is retried with backoff (joining the still-blocked call), and its
// eventual result is applied then, through the instance actor.
type WorktreeEnricher struct {
	cfg   WorktreeEnricherConfig
	queue *scanqueue.Queue
	group singleflight.Group

	mu    sync.Mutex
	tasks map[string][]*enrichTask // path -> instances awaiting a result
	stuck map[string]*stuckPath

	timedOut, completed atomic.Uint64
}

// WorktreeEnricherConfig tunes a WorktreeEnricher; zero values pick defaults.
type WorktreeEnricherConfig struct {
	Workers    int           // concurrent probes; default 4
	Deadline   time.Duration // per-attempt wait before a path counts as stuck; default 10s
	BackoffMin time.Duration // first retry delay for a stuck path; default 5s
	BackoffMax time.Duration // retry delay ceiling; default 5m

	// Persist saves an instance after enrichment changed it. Optional.
	Persist func(*Instance) error

	// Ready gates a path before probing (extension point for a TCC consent gate).
	// Returning false parks the path: its tasks stay registered, the Await barrier
	// is released, and nothing runs until Unpark(path). Nil means always ready.
	Ready func(path string) bool

	// Probe, AfterFunc and Now are injectable for deterministic tests.
	Probe     func(path string) WorktreeProbeResult
	AfterFunc func(d time.Duration, f func())
	Now       func() time.Time
}

// WorktreeProbeResult is everything one blocking filesystem visit learns.
type WorktreeProbeResult struct {
	Info    *WorktreeInfo
	InfoErr error
	StatErr error
}

type enrichTask struct {
	inst *Instance
	done chan struct{} // closed after the first attempt for this task settles
	once sync.Once
}

func (t *enrichTask) release() { t.once.Do(func() { close(t.done) }) }

type stuckPath struct {
	since   time.Time
	nextTry time.Time
	backoff time.Duration
}

// NewWorktreeEnricher builds an enricher; call Start, then Enqueue per instance.
func NewWorktreeEnricher(cfg WorktreeEnricherConfig) *WorktreeEnricher {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.Deadline <= 0 {
		cfg.Deadline = 10 * time.Second
	}
	if cfg.BackoffMin <= 0 {
		cfg.BackoffMin = 5 * time.Second
	}
	if cfg.BackoffMax <= 0 {
		cfg.BackoffMax = 5 * time.Minute
	}
	if cfg.Probe == nil {
		cfg.Probe = probeWorktreePath
	}
	if cfg.AfterFunc == nil {
		cfg.AfterFunc = func(d time.Duration, f func()) { time.AfterFunc(d, f) }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	e := &WorktreeEnricher{cfg: cfg, tasks: map[string][]*enrichTask{}, stuck: map[string]*stuckPath{}}
	e.queue = scanqueue.New(scanqueue.Config{Workers: cfg.Workers}, e.handle)
	activeWorktreeEnricher.Store(e)
	return e
}

func probeWorktreePath(path string) WorktreeProbeResult {
	var r WorktreeProbeResult
	r.Info, r.InfoErr = DetectWorktree(path)
	_, r.StatErr = os.Stat(path)
	return r
}

// Start launches the worker pool; it stops when ctx is done.
func (e *WorktreeEnricher) Start(ctx context.Context) { e.queue.Start(ctx) }

// Enqueue schedules inst's enrichment. Archived and terminal sessions are skipped.
func (e *WorktreeEnricher) Enqueue(inst *Instance) {
	if inst == nil || enrichSkipsStatus(inst) {
		return
	}
	path := inst.worktreeDetectPath()
	if path == "" {
		return
	}
	e.mu.Lock()
	e.tasks[path] = append(e.tasks[path], &enrichTask{inst: inst, done: make(chan struct{})})
	e.mu.Unlock()
	e.queue.Enqueue(path)
}

// Await blocks until inst's first enrichment attempt settled (applied, failed or
// timed out) or maxWait elapsed. Server start calls it before Start(false) so a
// missing-worktree pause lands before the instance is started.
func (e *WorktreeEnricher) Await(inst *Instance, maxWait time.Duration) {
	e.mu.Lock()
	var t *enrichTask
	for _, ts := range e.tasks {
		for _, c := range ts {
			if c.inst == inst {
				t = c
			}
		}
	}
	e.mu.Unlock()
	if t == nil {
		return
	}
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case <-t.done:
	case <-timer.C:
	}
}

func enrichSkipsStatus(inst *Instance) bool {
	if inst.IsArchived() {
		return true
	}
	switch inst.GetLifecycleStatus() {
	case Stopped, Failed, PermanentlyFailed:
		return true
	}
	return false
}

// Unpark requeues a path parked by Config.Ready.
func (e *WorktreeEnricher) Unpark(path string) { e.queue.Enqueue(path) }

func (e *WorktreeEnricher) handle(path string) {
	if e.cfg.Ready != nil && !e.cfg.Ready(path) {
		e.mu.Lock()
		e.releaseLocked(path)
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	if sp := e.stuck[path]; sp != nil && e.cfg.Now().Before(sp.nextTry) {
		e.releaseLocked(path) // first attempt is over for these tasks; a retry is already scheduled
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()

	runningProbes.Add(1)
	defer runningProbes.Add(-1)
	start := e.cfg.Now()
	ch := e.group.DoChan(path, func() (any, error) { return e.cfg.Probe(path), nil })
	timer := time.NewTimer(e.cfg.Deadline)
	defer timer.Stop()
	select {
	case r := <-ch:
		e.finish(path, r.Val.(WorktreeProbeResult), e.cfg.Now().Sub(start))
	case <-timer.C:
		e.markStuck(path)
	}
}

func (e *WorktreeEnricher) releaseLocked(path string) {
	for _, t := range e.tasks[path] {
		t.release()
	}
}

func (e *WorktreeEnricher) markStuck(path string) {
	e.mu.Lock()
	sp := e.stuck[path]
	now := e.cfg.Now()
	if sp == nil {
		sp = &stuckPath{since: now, backoff: e.cfg.BackoffMin}
		e.stuck[path] = sp
		stuckPaths.Add(1)
	} else {
		sp.backoff = min(sp.backoff*2, e.cfg.BackoffMax)
	}
	sp.nextTry = now.Add(sp.backoff)
	backoff, since := sp.backoff, sp.since
	e.mu.Unlock()

	e.timedOut.Add(1)
	enrichmentResults.Add(context.Background(), 1, metric.WithAttributes(attribute.String("result", "timeout")))
	log.Warn("worktree enrichment: path stuck, will retry", "path", path, "stuck_for", now.Sub(since), "retry_in", backoff)
	e.cfg.AfterFunc(backoff, func() { e.queue.Enqueue(path) })
	// Release the Await barrier last, so a waiter observes the scheduled retry.
	e.mu.Lock()
	e.releaseLocked(path)
	e.mu.Unlock()
}

func (e *WorktreeEnricher) finish(path string, res WorktreeProbeResult, took time.Duration) {
	e.mu.Lock()
	late := false
	if sp := e.stuck[path]; sp != nil {
		late = true
		delete(e.stuck, path)
		stuckPaths.Add(-1)
	}
	tasks := e.tasks[path]
	delete(e.tasks, path)
	e.mu.Unlock()

	result := "ok"
	if late {
		result = "late"
		log.Info("worktree enrichment: stuck path finished (slow, not stuck)", "path", path, "took", took)
	}
	e.completed.Add(1)
	enrichmentResults.Add(context.Background(), 1, metric.WithAttributes(attribute.String("result", result)))
	for i, t := range tasks {
		// Only the first attempt of a task may touch a not-yet-live instance: the
		// server's Await barrier holds Start(false) until release below.
		e.apply(t.inst, res, !late || t.inst.liveInstance.Load() != nil)
		tasks[i].release()
	}
}

// apply publishes res to inst through the actor/setters; never revives or starts.
func (e *WorktreeEnricher) apply(inst *Instance, res WorktreeProbeResult, allowed bool) {
	if !allowed || enrichSkipsStatus(inst) {
		return
	}
	changed := false
	snap := inst.Snapshot()
	if (snap.GitHub.GitHubOwner == "" || snap.GitHub.GitHubRepo == "") && res.InfoErr == nil && res.Info != nil {
		if err := inst.applyWorktreeInfo(res.Info); err != nil {
			log.Warn("worktree enrichment: apply failed", "session", inst.Title, "err", err)
		} else {
			changed = true
			if res.Info.GitHubOwner != "" {
				log.Info("auto-detected github info", "session", inst.Title, "owner", res.Info.GitHubOwner, "repo", res.Info.GitHubRepo)
			}
		}
	} else if res.InfoErr != nil {
		log.Warn("failed to detect worktree info", "session", inst.Title, "err", res.InfoErr)
	}
	if e.pauseIfWorktreeMissing(inst, res.StatErr) {
		changed = true
	}
	if changed && e.cfg.Persist != nil {
		if err := e.cfg.Persist(inst); err != nil {
			log.Warn("worktree enrichment: persist failed", "session", inst.Title, "err", err)
		}
	}
}

// pauseIfWorktreeMissing marks a not-yet-started worktree session Paused when its
// directory is gone, mirroring what fromInstanceData used to do inline.
func (e *WorktreeEnricher) pauseIfWorktreeMissing(inst *Instance, statErr error) bool {
	if !errors.Is(statErr, fs.ErrNotExist) || inst.Started() || inst.Paused() || inst.Hibernated() || !inst.gitManager.HasWorktree() {
		return false
	}
	worktreePath := inst.gitManager.GetWorktreePath()
	_, alreadyLogged := loggedMissingWorktree.LoadOrStore(inst.Title, struct{}{})
	logger := log.ForSession(inst.Title)
	logger.Log(context.Background(), worktreeMissingLevel(alreadyLogged), "worktree directory missing, marking as paused", "path", worktreePath)
	err := inst.sendSyncErr(func(s *instanceState) error { return transitionToLocked(s, context.Background(), Paused) })
	if err != nil {
		logger.Warn("could not pause session with missing worktree", "err", err)
		return false
	}
	// Same wiring fromInstanceData does for a session restored as Paused.
	inst.started.Store(true)
	inst.wireTmuxSession(shellLaunch(inst.Program))
	return true
}

// Stats reports completed and timed-out probe counts.
type WorktreeEnricherStats struct{ Completed, TimedOut uint64 }

func (e *WorktreeEnricher) Stats() WorktreeEnricherStats {
	return WorktreeEnricherStats{Completed: e.completed.Load(), TimedOut: e.timedOut.Load()}
}

// Package-level gauges: queued is derived from the active enricher's queue.
var (
	runningProbes atomic.Int64
	stuckPaths    atomic.Int64

	activeWorktreeEnricher atomic.Pointer[WorktreeEnricher]

	enrichmentResults = mustEnrichmentCounter()
)

func mustEnrichmentCounter() metric.Int64Counter {
	m := telemetry.GetMeter()
	c, err := m.Int64Counter("worktree_enrichment_total",
		metric.WithDescription("Post-start worktree enrichment probes by result (ok, late, timeout)"))
	if err != nil {
		panic(err)
	}
	for name, read := range map[string]func() int64{
		"worktree_enrichment_running":     runningProbes.Load,
		"worktree_enrichment_stuck_paths": stuckPaths.Load,
		"worktree_enrichment_queued": func() int64 {
			if e := activeWorktreeEnricher.Load(); e != nil {
				return int64(e.queue.Pending())
			}
			return 0
		},
	} {
		read := read
		if _, err := m.Int64ObservableGauge(name, metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(read())
			return nil
		})); err != nil {
			panic(err)
		}
	}
	return c
}
