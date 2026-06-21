package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/headless"
)

// ReviewGateSpawner can create a short-lived review session for a backlog item.
// Deprecated: use headless.Pool via NewBacklogLifecycleListenerWithSpawner instead.
// Retained for backward compatibility with existing tests and callers.
type ReviewGateSpawner interface {
	// SpawnReviewSession creates a one-shot review session for item using prompt.
	// itemSessionID is the UUID of the work ItemSession being reviewed.
	SpawnReviewSession(ctx context.Context, item *ent.BacklogItem, itemSessionID string, prompt string) (*Instance, error)
}

// maxConcurrentReviewGates is the maximum number of review gates that can run
// concurrently. This caps goroutine fan-out when many sessions exit simultaneously.
const maxConcurrentReviewGates = 8

// BacklogLifecycleListener drives backlog item state transitions in response to
// session lifecycle events. It must be registered via Instance.RegisterLifecycleListener.
//
// OnLifecycleEvent is non-blocking; all DB work is dispatched to a goroutine.
// Call SetEnabled(false) to make all callbacks no-ops without unwiring.
type BacklogLifecycleListener struct {
	storage        *Storage
	sessionCreator ReviewGateSpawner

	// poolMu guards headlessPool for concurrent Set/get access.
	poolMu       sync.RWMutex
	headlessPool *headless.Pool

	// reviewSem limits concurrent review gate goroutines.
	reviewSem chan struct{}

	// shutdownCtx is cancelled by Shutdown(); used by long-running review gate calls.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	enabled atomic.Bool
}

// SetEnabled toggles whether this listener processes lifecycle events.
// Safe to call concurrently.
func (l *BacklogLifecycleListener) SetEnabled(v bool) { l.enabled.Store(v) }

// SetHeadlessPool wires in the headless LLM pool after construction.
// Calling this enables the headless review gate path even when the listener was
// created via NewBacklogLifecycleListenerWithSpawner.
func (l *BacklogLifecycleListener) SetHeadlessPool(p *headless.Pool) {
	l.poolMu.Lock()
	defer l.poolMu.Unlock()
	l.headlessPool = p
}

// getHeadlessPool returns the current headless pool under a read lock.
func (l *BacklogLifecycleListener) getHeadlessPool() *headless.Pool {
	l.poolMu.RLock()
	defer l.poolMu.RUnlock()
	return l.headlessPool
}

// Shutdown cancels in-flight review gate calls. Safe to call concurrently.
func (l *BacklogLifecycleListener) Shutdown() {
	if l.shutdownCancel != nil {
		l.shutdownCancel()
	}
}

// newListenerBase initialises fields common to all BacklogLifecycleListener constructors.
func newListenerBase(storage *Storage) *BacklogLifecycleListener {
	ctx, cancel := context.WithCancel(context.Background())
	return &BacklogLifecycleListener{
		storage:        storage,
		reviewSem:      make(chan struct{}, maxConcurrentReviewGates),
		shutdownCtx:    ctx,
		shutdownCancel: cancel,
	}
}

// NewBacklogLifecycleListener creates a listener backed by the given storage.
// The review gate is disabled (sessionCreator=nil, headlessPool=nil).
func NewBacklogLifecycleListener(storage *Storage) *BacklogLifecycleListener {
	return newListenerBase(storage)
}

// NewBacklogLifecycleListenerWithSpawner creates a listener that will spawn a
// review gate session when a work session exits and SkipReviewGate is false.
func NewBacklogLifecycleListenerWithSpawner(storage *Storage, spawner ReviewGateSpawner) *BacklogLifecycleListener {
	l := newListenerBase(storage)
	l.sessionCreator = spawner
	return l
}

// NewBacklogLifecycleListenerWithPool creates a listener that uses a headless.Pool
// for review gate calls instead of spawning a tmux session.
func NewBacklogLifecycleListenerWithPool(storage *Storage, pool *headless.Pool) *BacklogLifecycleListener {
	l := newListenerBase(storage)
	l.headlessPool = pool
	return l
}

// instanceBacklogListener is a per-instance shim that binds the instance UUID into
// every lifecycle callback. Created and registered via WireToInstance.
type instanceBacklogListener struct {
	parent       *BacklogLifecycleListener
	instanceUUID string
}

func (il *instanceBacklogListener) OnLifecycleEvent(event LifecycleEvent, _ string) {
	if !il.parent.enabled.Load() {
		return
	}
	switch event {
	case EventStarted:
		go il.parent.onSessionStarted(il.instanceUUID)
	case EventExited:
		go il.parent.onSessionExited(il.instanceUUID)
	}
}

// WireToInstance creates a per-instance listener shim and registers it on inst.
// Call this for every Instance that should participate in backlog lifecycle tracking.
func (l *BacklogLifecycleListener) WireToInstance(inst *Instance) {
	inst.RegisterLifecycleListener(&instanceBacklogListener{
		parent:       l,
		instanceUUID: inst.UUID,
	})
}

// onSessionStarted records the start time for the ItemSession linked to sessionUUID.
func (l *BacklogLifecycleListener) onSessionStarted(sessionUUID string) {
	ctx := context.Background()
	is, err := l.storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return
		}
		log.ErrorLog.Printf("[BacklogLifecycle] GetItemSessionBySessionUUID(%s) error: %v", sessionUUID, err)
		return
	}
	if err := l.storage.UpdateItemSessionStarted(ctx, is.ID.String(), time.Now()); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] UpdateItemSessionStarted(%s) error: %v", is.ID, err)
	}
}

// onSessionExited drives the in_progress→review (or in_progress→done for skip_review_gate) transition.
func (l *BacklogLifecycleListener) onSessionExited(sessionUUID string) {
	ctx := context.Background()

	is, err := l.storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return
		}
		log.ErrorLog.Printf("[BacklogLifecycle] GetItemSessionBySessionUUID(%s) error: %v", sessionUUID, err)
		return
	}

	// Record end time for all session roles (triage, review, work).
	now := time.Now()
	if err := l.storage.UpdateItemSessionEnded(ctx, is.ID.String(), now); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] UpdateItemSessionEnded(%s) error: %v", is.ID, err)
	}

	// Only drive in_progress→review/done transitions for work sessions.
	if is.SessionRole != SessionRoleWork {
		return
	}

	// BacklogItem edge is eager-loaded by GetItemSessionBySessionUUID.
	item, err := is.Edges.BacklogItemOrErr()
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] BacklogItemOrErr for session %s: %v", sessionUUID, err)
		return
	}

	if BacklogStatus(item.Status) != BacklogStatusInProgress {
		log.DebugLog.Printf("[BacklogLifecycle] item %s is %s (not in_progress); skipping", item.ID, item.Status)
		return
	}

	toStatus := BacklogStatusReview
	if item.SkipReviewGate {
		toStatus = BacklogStatusDone
	}

	updatedAt := item.UpdatedAt
	precondition := &BacklogItemPrecondition{
		ExpectedStatus:    string(BacklogStatusInProgress),
		ExpectedUpdatedAt: &updatedAt,
	}
	if _, err := l.storage.TransitionBacklogItemStatus(ctx, item.ID.String(), toStatus, precondition); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] TransitionBacklogItemStatus item=%s to=%s: %v", item.ID, toStatus, err)
		return
	}

	log.InfoLog.Printf("[BacklogLifecycle] item %s transitioned to %s (session %s exited)", item.ID, toStatus, sessionUUID)

	// Spawn review gate if the item moved to review and a review mechanism is configured.
	if toStatus == BacklogStatusReview && !item.SkipReviewGate && (l.getHeadlessPool() != nil || l.sessionCreator != nil) {
		go func() {
			// Acquire the bounded semaphore to prevent unbounded goroutine fan-out
			// when many sessions exit simultaneously.
			select {
			case l.reviewSem <- struct{}{}:
			case <-l.shutdownCtx.Done():
				return
			}
			defer func() { <-l.reviewSem }()
			l.spawnReviewGate(item, is)
		}()
	}
}

// spawnReviewGate creates a one-shot review session for item, using the diff
// from the work session's worktree.
func (l *BacklogLifecycleListener) spawnReviewGate(item *ent.BacklogItem, is *ent.ItemSession) {
	ctx := l.shutdownCtx

	// Precondition: repo_path must be set or we have nothing to review.
	if item.RepoPath == "" {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate item=%s has no repo path set; skipping review gate", item.ID)
		return
	}

	// Get the git diff.
	worktreePath := item.RepoPath
	diff, truncated, diffErr := GetGitDiff(ctx, worktreePath, is.LastCommitSha)
	if diffErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate GetGitDiff item=%s: %v", item.ID, diffErr)
		diff = ""
	}

	// Security check — block if secrets detected.
	if secErr := RunPreGateSecurityCheck(diff); secErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate security check blocked item=%s: %v", item.ID, secErr)
		// Record a failed review ItemSession with a FAIL verdict so the gate verdict
		// is visible in the UI and operators can act (override or re-review).
		summary := fmt.Sprintf("Review blocked by security check: %v. Override required to proceed.", secErr)
		if _, _, createErr := l.storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
			ItemID:      item.ID.String(),
			SessionUUID: "review-blocked-" + item.ID.String(),
			SessionRole: SessionRoleReview,
		}, ReviewVerdictData{
			OverallOutcome: ReviewVerdictFail,
			Summary:        summary,
		}); createErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (security block) item=%s: %v", item.ID, createErr)
			return
		}
		log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate security check blocked for item %s — FAIL verdict recorded", item.ID)
		return
	}

	// Deserialize AC snapshot.
	acSnapshot, _ := ParseAcCriteria(is.AcSnapshot)
	if len(acSnapshot) == 0 {
		acSnapshot, _ = ParseAcCriteria(item.AcceptanceCriteria)
	}

	prompt := BuildReviewPrompt(item, acSnapshot, diff, truncated, is.ID.String())

	pool := l.getHeadlessPool()
	if pool != nil {
		// Headless path: call LLM directly without spawning a tmux session.
		// Use JSON-output prompts because headless claude -p has no tool access.
		reviewCtx, reviewCancel := context.WithTimeout(ctx, headless.DefaultCallTimeout)
		defer reviewCancel()

		headlessPrompt := BuildHeadlessReviewPrompt(item, acSnapshot, diff, truncated)
		reviewResult, callErr := pool.CallBlocking(reviewCtx, headless.FeatureKeyReview, headless.HeadlessReviewSystemPrompt(), headlessPrompt)
		if callErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate headless.CallBlocking item=%s: %v", item.ID, callErr)
			return
		}

		overall, perCriterion, summary := ParseHeadlessVerdictResult(reviewResult)
		perCriterionJSON, _ := json.Marshal(perCriterion)

		// Create a synthetic ItemSession and its ReviewVerdict atomically so there
		// is never a dangling session with no verdict if the verdict write fails.
		reviewSessionUUID := "headless-review-" + uuid.New().String()
		reviewIS, _, createErr := l.storage.CreateItemSessionWithVerdict(ctx, ItemSessionData{
			ItemID:      item.ID.String(),
			SessionUUID: reviewSessionUUID,
			SessionRole: SessionRoleReview,
			AcSnapshot:  is.AcSnapshot,
		}, ReviewVerdictData{
			OverallOutcome: overall,
			PerCriterion:   string(perCriterionJSON),
			Summary:        summary,
		})
		if createErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSessionWithVerdict (headless) item=%s: %v", item.ID, createErr)
			return
		}

		log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate headless review complete for item %s (session %s, outcome %s)", item.ID, reviewIS.ID, overall)
		return
	}

	// Legacy path: spawn a tmux review session via sessionCreator.
	if l.sessionCreator == nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate item=%s: no review mechanism configured", item.ID)
		return
	}

	reviewInst, spawnErr := l.sessionCreator.SpawnReviewSession(ctx, item, is.ID.String(), prompt)
	if spawnErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate SpawnReviewSession item=%s: %v", item.ID, spawnErr)
		return
	}

	// Create ItemSession linking the new review session to the backlog item.
	if _, createErr := l.storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID.String(),
		SessionUUID: reviewInst.UUID,
		SessionRole: SessionRoleReview,
		AcSnapshot:  is.AcSnapshot,
	}); createErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSession item=%s review=%s: %v", item.ID, reviewInst.UUID, createErr)
		return
	}

	log.InfoLog.Printf("[BacklogLifecycle] spawnReviewGate spawned review session %s for item %s", reviewInst.UUID, item.ID)
}

// ReconcileStuck calls ReconcileStuckItems and logs the result.
// Intended to be called on a periodic ticker as a safety net for abnormal session exits.
// No-op when the listener is disabled.
func (l *BacklogLifecycleListener) ReconcileStuck(ctx context.Context) {
	if !l.enabled.Load() {
		return
	}
	er, ok := l.storage.repo.(*EntRepository)
	if !ok {
		return
	}
	n, err := er.ReconcileStuckItems(ctx)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] ReconcileStuckItems error: %v", err)
		return
	}
	if n > 0 {
		log.InfoLog.Printf("[BacklogLifecycle] ReconcileStuckItems: transitioned %d stuck items to review", n)
	} else {
		log.DebugLog.Printf("[BacklogLifecycle] ReconcileStuckItems: no stuck items found")
	}
}
