package services

// completion_reviewer.go — EventBus subscriber that, when a backlog item reaches done,
// asks a tool-less one-shot headless call what future sessions should know and appends the
// answer to the repo's memory file. Best-effort: every failure is logged and dropped.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

const (
	completionReviewerFlagName      = "backlog:completion-reviewer"
	maxConcurrentCompletionReviews  = 8
	completionReviewDedupWindow     = 10 * time.Minute
	completionReviewCallTimeout     = 3 * time.Minute
	completionReviewStatusLogPeriod = time.Hour
)

// MemoryEntry is one learning to persist, tagged with the item it came from.
type MemoryEntry struct {
	SourceItemID string
	RepoPath     string
	Content      string
}

// MemoryStore is the reviewer's only write path. Append-only on purpose: no delete or
// update method exists, so the reviewer cannot remove an existing entry. Pin handling, if the
// store ever grows it, is internal to the store.
type MemoryStore interface {
	Append(ctx context.Context, entry MemoryEntry) error
}

// CompletionReviewCall runs the model call. The production implementation is text-only.
type CompletionReviewCall func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

// NewHeadlessCompletionReviewCall returns the production call: a one-shot headless.Pool
// subprocess with CallOptions.NoTools (no built-in tools, no MCP servers, no skills) run in a
// fresh empty directory, so the model can only emit text. Go code is the only path to Append.
func NewHeadlessCompletionReviewCall() CompletionReviewCall {
	return func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		pool := headless.DefaultPool() // resolved per call: it is installed after server wiring
		if pool == nil {
			return "", errors.New("headless pool not initialized")
		}
		dir, err := os.MkdirTemp("", "ssq-completion-review-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir)
		ctx, cancel := context.WithTimeout(ctx, completionReviewCallTimeout)
		defer cancel()
		return pool.CallBlocking(ctx, headless.FeatureKeyCompletionReview, systemPrompt, userPrompt,
			headless.CallOptions{WorkDir: dir, NoTools: true}, headless.DiscardCost)
	}
}

// completionReviewer holds the reviewer's collaborators. Nil call/memory disables it.
type completionReviewer struct {
	storage  *session.Storage
	call     CompletionReviewCall
	memory   MemoryStore
	notifier session.Notifier
	enabled  func() bool // live-read rollout flag

	attempts, failures atomic.Int64
	lastStatusLog      atomic.Int64 // unix nanos
}

// StartCompletionReviewerSubscriber subscribes to the EventBus (fed by the repository's
// ItemChangePublisher fan-out) and reviews each item that transitions to done. Dispatch runs in
// bounded background goroutines with their own panic recovery.
func StartCompletionReviewerSubscriber(ctx context.Context, bus *events.EventBus, storage *session.Storage, call CompletionReviewCall, memory MemoryStore, notifier session.Notifier) <-chan struct{} {
	done := make(chan struct{})
	if bus == nil || storage == nil {
		log.Warn("completion_reviewer: missing dependency, subscriber not started")
		close(done)
		return done
	}
	r := &completionReviewer{
		storage: storage, call: call, memory: memory, notifier: notifier,
		enabled: func() bool { return config.LoadConfig().GetFeatureFlag(completionReviewerFlagName) },
	}
	return r.start(ctx, bus)
}

func (r *completionReviewer) start(ctx context.Context, bus *events.EventBus) <-chan struct{} {
	done := make(chan struct{})
	ch, _ := bus.Subscribe(ctx)
	go func() {
		defer close(done)
		defer func() {
			if rec := recover(); rec != nil {
				log.Warn("completion_reviewer: subscriber loop panic recovered, reviewer stopped", "panic", fmt.Sprint(rec))
			}
		}()
		r.loop(ctx, ch)
	}()
	return done
}

func (r *completionReviewer) loop(ctx context.Context, ch <-chan *events.Event) {
	sem := make(chan struct{}, maxConcurrentCompletionReviews)
	recent := map[string]time.Time{}
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			if event == nil || event.Type != events.EventBacklogItemChanged {
				continue
			}
			p := event.BacklogItemPayload
			if p == nil || p.Item == nil || p.Kind != events.BacklogChangeStatusTransition || p.NewStatus != string(session.BacklogStatusDone) {
				continue
			}
			if !r.enabled() || r.call == nil || r.memory == nil {
				continue
			}
			if skipRecentlyReviewed(recent, p.Item.ID, time.Now()) {
				log.Debug("completion_reviewer: skipped (dedup window)", "item", p.Item.ID)
				continue
			}
			// Acquire before spawning so a burst cannot pile up blocked goroutines.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			wg.Add(1)
			go func(item *session.BacklogItemData) {
				defer wg.Done()
				defer func() { <-sem }()
				r.safeReview(ctx, item)
			}(p.Item)
		case <-ctx.Done():
			return
		}
	}
}

// skipRecentlyReviewed reports whether itemID was dispatched within the dedup window,
// recording now otherwise. Only the single loop goroutine touches recent, so no lock.
func skipRecentlyReviewed(recent map[string]time.Time, itemID string, now time.Time) bool {
	for id, at := range recent {
		if now.Sub(at) >= completionReviewDedupWindow {
			delete(recent, id)
		}
	}
	if _, seen := recent[itemID]; seen {
		return true
	}
	recent[itemID] = now
	return false
}

// safeReview isolates panics: the publisher's own recovery only covers its synchronous call.
func (r *completionReviewer) safeReview(ctx context.Context, item *session.BacklogItemData) {
	defer func() {
		if rec := recover(); rec != nil {
			r.failures.Add(1)
			log.Warn("completion_reviewer: panic recovered", "item", item.ID, "panic", fmt.Sprint(rec))
		}
	}()
	r.review(ctx, item)
	r.maybeLogStatus()
}

func hasNonTrivialCompletionReviewContent(item *session.BacklogItemData) bool {
	if item == nil || strings.TrimSpace(item.Description) == "" {
		return false
	}
	criteria, _ := session.ParseAcCriteria(item.AcceptanceCriteria)
	return len(criteria) > 0
}

func (r *completionReviewer) review(ctx context.Context, item *session.BacklogItemData) {
	if !hasNonTrivialCompletionReviewContent(item) {
		log.Debug("completion_reviewer: skipped (thin content)", "item", item.ID)
		return
	}
	r.attempts.Add(1)
	sessions, err := r.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		log.Debug("completion_reviewer: session lookup failed, reviewing without sessions", "item", item.ID, "err", err)
	}
	raw, err := r.call(ctx, completionReviewSystemPrompt, buildCompletionReviewUserPrompt(session.BuildCompletionReviewContext(item, sessions)))
	if err != nil {
		r.fail("ai call", item.ID, err)
		return
	}
	learnings, ok := ParseReviewOutcome(raw)
	if !ok {
		log.Debug("completion_reviewer: nothing to record or unparseable output", "item", item.ID)
		return
	}
	if err := r.memory.Append(ctx, MemoryEntry{SourceItemID: item.ID, RepoPath: item.RepoPath, Content: learnings}); err != nil {
		r.fail("memory write", item.ID, err)
		return
	}
	log.Info("completion_reviewer: reviewed item", "item", item.ID, "repo", item.RepoPath)
	if r.notifier != nil {
		r.notifier.Notify(item.ID, "Completion review recorded", "Recorded learnings for future sessions.", int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), false, false)
	}
}

func (r *completionReviewer) fail(stage, itemID string, err error) {
	r.failures.Add(1)
	log.Warn("completion_reviewer: "+stage+" failed", "item", itemID, "err", err,
		"failures", r.failures.Load(), "attempts", r.attempts.Load())
}

// maybeLogStatus emits a periodic line so "disabled/no credential" is distinguishable from
// "enabled, 0 successes" long after startup.
func (r *completionReviewer) maybeLogStatus() {
	now := time.Now().UnixNano()
	last := r.lastStatusLog.Load()
	n := r.attempts.Load()
	due := now-last >= int64(completionReviewStatusLogPeriod) || (n > 0 && n%100 == 0)
	if !due || !r.lastStatusLog.CompareAndSwap(last, now) {
		return
	}
	log.Info("completion_reviewer: status", "enabled", r.call != nil && r.memory != nil,
		"attempts", r.attempts.Load(), "failures", r.failures.Load())
}
