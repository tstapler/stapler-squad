package crew

import (
	"context"
	"sync"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/queue"
)

// Fixer is the Crew Autonomy supervisor. It subscribes to the ReviewQueue and,
// when a session completes a task (ReasonTaskComplete), spawns a Lookout to run
// The Sweep quality gate. If the Sweep passes, it enriches the ReviewItem with
// the assembled Score. If the Sweep fails after maxRetries, it adds a
// ReasonTestsFailing item so the human Mastermind can review it.
//
// Fixer implements session.ReviewQueueObserver and is safe for concurrent use.
type Fixer struct {
	mu       sync.RWMutex
	lookouts map[string]*Lookout // keyed by sessionID

	doneCh  chan LookoutResult // all Lookouts report here
	queue   *session.ReviewQueue
	poller  InstanceFinder // ReviewQueuePoller.FindInstance
	checker TmuxPaneChecker

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewFixer creates a new Fixer.
// queue is the shared ReviewQueue; poller is used to look up instances by sessionID.
// checker is the TmuxPaneChecker used by InjectEarpiece (pass nil for the default).
func NewFixer(
	reviewQueue *session.ReviewQueue,
	poller InstanceFinder,
	checker TmuxPaneChecker,
) *Fixer {
	if checker == nil {
		checker = &DefaultTmuxPaneChecker{}
	}
	return &Fixer{
		lookouts: make(map[string]*Lookout),
		doneCh:   make(chan LookoutResult, 32),
		queue:    reviewQueue,
		poller:   poller,
		checker:  checker,
	}
}

// Start subscribes the Fixer to the ReviewQueue and launches its background goroutine.
// It must be called once after creation.
func (f *Fixer) Start(ctx context.Context) {
	f.ctx, f.cancel = context.WithCancel(ctx)
	f.queue.Subscribe(f)
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		f.reapLookouts()
	}()
	log.InfoLog.Printf("[Fixer] started")
}

// Stop unsubscribes, cancels all Lookouts, and waits for all goroutines to exit.
func (f *Fixer) Stop() {
	f.queue.Unsubscribe(f)

	// Cancel the root context so all child goroutines exit.
	if f.cancel != nil {
		f.cancel()
	}

	// Stop every active Lookout.
	f.mu.Lock()
	for _, l := range f.lookouts {
		l.Stop()
	}
	f.mu.Unlock()

	f.wg.Wait()
	log.InfoLog.Printf("[Fixer] stopped")
}

// --- ReviewQueueObserver implementation ---

// OnItemAdded is called when a new item is added to the ReviewQueue.
// If the item's reason is ReasonTaskComplete, a Lookout is spawned for the session.
func (f *Fixer) OnItemAdded(item *session.ReviewItem) {
	if item.Reason != session.ReasonTaskComplete {
		return
	}
	f.spawnLookout(item)
}

// OnItemRemoved is called when an item is removed from the ReviewQueue.
// If there is an active Lookout for the session, it is stopped and removed.
func (f *Fixer) OnItemRemoved(sessionID string) {
	f.mu.Lock()
	l, ok := f.lookouts[sessionID]
	if ok {
		delete(f.lookouts, sessionID)
	}
	f.mu.Unlock()

	if ok {
		l.Stop()
		log.InfoLog.Printf("[Fixer] stopped lookout for removed session %s", sessionID)
	}
}

// OnQueueUpdated is a no-op for the Fixer; it reacts to individual adds/removes.
func (f *Fixer) OnQueueUpdated(_ []*session.ReviewItem) {}

// --- Internal helpers ---

// spawnLookout creates and starts a Lookout for the given review item.
// If a Lookout already exists for the session, the call is ignored.
func (f *Fixer) spawnLookout(item *session.ReviewItem) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.lookouts[item.SessionID]; exists {
		log.InfoLog.Printf("[Fixer] lookout already active for session %s, ignoring duplicate TaskComplete", item.SessionID)
		return
	}

	// Determine the working directory and trust level from the live instance.
	workingDir := item.WorkingDir
	autonomousMode := false // Supervised mode is the safe default
	if f.poller != nil {
		if inst := f.poller.FindInstance(item.SessionID); inst != nil {
			if workingDir == "" {
				workingDir = inst.WorkingDir
			}
			autonomousMode = inst.AutonomousMode
		}
	}

	cfg := LookoutConfig{
		SessionID:  item.SessionID,
		GoingDark:  autonomousMode,
		MaxRetries: 3,
		WorkingDir: workingDir,
		Program:    item.Program,
	}

	lookout := NewLookout(cfg, f.doneCh)
	f.lookouts[item.SessionID] = lookout

	// Start the Lookout state machine.
	lookout.Start()

	// Start an earpiece watcher goroutine for this Lookout.
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		f.watchEarpiece(item.SessionID, item.SessionName, workingDir, cfg.MaxRetries, lookout)
	}()

	// Immediately signal TaskComplete so the Lookout begins sweeping.
	lookout.OnTaskComplete()

	log.InfoLog.Printf("[Fixer] spawned lookout for session %s (workingDir=%s)", item.SessionID, workingDir)
}

// watchEarpiece reads from a Lookout's EarpieceCh and injects correction prompts
// until the Lookout stops or the Fixer's context is cancelled.
func (f *Fixer) watchEarpiece(sessionID, sessionName, workingDir string, maxRetries int, l *Lookout) {
	earpieceCh := l.EarpieceCh()
	retryAttempt := 1

	for {
		select {
		case <-f.ctx.Done():
			return
		case result, ok := <-earpieceCh:
			if !ok {
				return
			}
			if result == nil {
				continue
			}
			log.InfoLog.Printf("[Fixer] injecting earpiece for session %s (attempt %d/%d)", sessionID, retryAttempt, maxRetries)
			if err := InjectEarpiece(
				sessionID,
				sessionName,
				workingDir,
				retryAttempt,
				maxRetries,
				result,
				f.poller,
				f.checker,
			); err != nil {
				log.ErrorLog.Printf("[Fixer] earpiece injection failed for %s: %v", sessionID, err)
			}
			retryAttempt++
		}
	}
}

// reapLookouts processes results delivered to doneCh by completed Lookouts.
func (f *Fixer) reapLookouts() {
	for {
		select {
		case <-f.ctx.Done():
			return
		case result := <-f.doneCh:
			f.handleLookoutResult(result)
		}
	}
}

// handleLookoutResult acts on a completed Lookout's result:
//   - Pass (FinalState=LookoutIdle, Score != nil): enriches the ReviewItem with the Score.
//   - Fall (FinalState=LookoutFallen): adds a ReasonTestsFailing item for human review.
func (f *Fixer) handleLookoutResult(result LookoutResult) {
	// Remove the Lookout from the map.
	f.mu.Lock()
	delete(f.lookouts, result.SessionID)
	f.mu.Unlock()

	switch result.FinalState {
	case LookoutIdle:
		// Sweep passed — enrich the existing ReviewItem with the Score.
		if result.Score != nil {
			f.enrichReviewItemScore(result.SessionID, result.Score)
		}
		log.InfoLog.Printf("[Fixer] session %s: sweep passed, score attached", result.SessionID)

	case LookoutFallen:
		// Max retries exhausted — escalate to the Mastermind review queue.
		log.InfoLog.Printf("[Fixer] session %s: lookout fallen, escalating to Mastermind", result.SessionID)
		f.escalateToMastermind(result)

	default:
		log.InfoLog.Printf("[Fixer] session %s: lookout finished with state %s", result.SessionID, result.FinalState)
	}
}

// enrichReviewItemScore updates an existing ReviewItem in the queue with the Score.
// If the item is no longer in the queue, this is a no-op.
func (f *Fixer) enrichReviewItemScore(sessionID string, score *queue.Score) {
	item, ok := f.queue.Get(sessionID)
	if !ok {
		log.InfoLog.Printf("[Fixer] enrichReviewItemScore: session %s not in queue, skipping", sessionID)
		return
	}

	item.Score = score

	// Re-add to queue (Add performs an upsert, preserving DetectedAt).
	f.queue.Add(item)
	log.InfoLog.Printf("[Fixer] attached Score to ReviewItem for session %s", sessionID)
}

// escalateToMastermind adds a ReasonTestsFailing ReviewItem to the queue so a human
// reviewer can inspect the failed correction loop.
func (f *Fixer) escalateToMastermind(result LookoutResult) {
	// Preserve existing ReviewItem fields if the session is still in the queue.
	item, ok := f.queue.Get(result.SessionID)
	if !ok {
		// Session already removed from queue — nothing to escalate.
		log.InfoLog.Printf("[Fixer] escalateToMastermind: session %s not in queue, skipping", result.SessionID)
		return
	}

	// Upgrade the reason to reflect the failed correction loop.
	item.Reason = session.ReasonTestsFailing
	item.Priority = session.PriorityHigh

	if result.Score != nil {
		item.Score = result.Score
	}

	f.queue.Add(item)
	log.InfoLog.Printf("[Fixer] escalated session %s to Mastermind (ReasonTestsFailing)", result.SessionID)
}
