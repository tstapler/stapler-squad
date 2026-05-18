package session

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
)

// ReviewGateSpawner can create a short-lived review session for a backlog item.
type ReviewGateSpawner interface {
	// SpawnReviewSession creates a one-shot review session for item using prompt.
	// itemSessionID is the UUID of the work ItemSession being reviewed.
	SpawnReviewSession(ctx context.Context, item *ent.BacklogItem, itemSessionID string, prompt string) (*Instance, error)
}

// BacklogLifecycleListener drives backlog item state transitions in response to
// session lifecycle events. It must be registered via Instance.RegisterLifecycleListener.
//
// OnLifecycleEvent is non-blocking; all DB work is dispatched to a goroutine.
// Call SetEnabled(false) to make all callbacks no-ops without unwiring.
type BacklogLifecycleListener struct {
	storage        *Storage
	sessionCreator ReviewGateSpawner
	enabled        atomic.Bool
}

// SetEnabled toggles whether this listener processes lifecycle events.
// Safe to call concurrently.
func (l *BacklogLifecycleListener) SetEnabled(v bool) { l.enabled.Store(v) }

// NewBacklogLifecycleListener creates a listener backed by the given storage.
// The review gate is disabled (sessionCreator=nil).
func NewBacklogLifecycleListener(storage *Storage) *BacklogLifecycleListener {
	return &BacklogLifecycleListener{storage: storage}
}

// NewBacklogLifecycleListenerWithSpawner creates a listener that will spawn a
// review gate session when a work session exits and SkipReviewGate is false.
func NewBacklogLifecycleListenerWithSpawner(storage *Storage, spawner ReviewGateSpawner) *BacklogLifecycleListener {
	return &BacklogLifecycleListener{storage: storage, sessionCreator: spawner}
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

	// Recursion guard: only drive transitions for work sessions.
	if is.SessionRole != SessionRoleWork {
		return
	}

	now := time.Now()
	if err := l.storage.UpdateItemSessionEnded(ctx, is.ID.String(), now); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] UpdateItemSessionEnded(%s) error: %v", is.ID, err)
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

	// Spawn review gate if the item moved to review and a spawner is configured.
	if toStatus == BacklogStatusReview && !item.SkipReviewGate && l.sessionCreator != nil {
		go l.spawnReviewGate(item, is)
	}
}

// spawnReviewGate creates a one-shot review session for item, using the diff
// from the work session's worktree.
func (l *BacklogLifecycleListener) spawnReviewGate(item *ent.BacklogItem, is *ent.ItemSession) {
	ctx := context.Background()

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
		blockedSession, createErr := l.storage.CreateItemSession(ctx, ItemSessionData{
			ItemID:      item.ID.String(),
			SessionUUID: "review-blocked-" + item.ID.String(),
			SessionRole: SessionRoleReview,
		})
		if createErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate CreateItemSession (security block) item=%s: %v", item.ID, createErr)
			return
		}
		summary := fmt.Sprintf("Review blocked by security check: %v. Override required to proceed.", secErr)
		if _, verdictErr := l.storage.SaveReviewVerdict(ctx, blockedSession.ID.String(), ReviewVerdictData{
			ItemSessionID:  blockedSession.ID.String(),
			OverallOutcome: ReviewVerdictFail,
			Summary:        summary,
		}); verdictErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] spawnReviewGate SaveReviewVerdict (security block) item=%s: %v", item.ID, verdictErr)
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
