package crew

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/queue"
)

// lookoutRunRecoverMsg is logged when the run() goroutine panics.
const lookoutRunRecoverMsg = "[Lookout] panic in run() goroutine, session may be stuck"

// LookoutState represents the supervisor state of a session's Lookout goroutine.
type LookoutState int

const (
	// LookoutIdle: Waiting for a TaskComplete event.
	LookoutIdle LookoutState = iota
	// LookoutActive: Session is working; seen at least one tool use.
	LookoutActive
	// LookoutSweeping: TaskComplete received; quality gate pipeline executing.
	LookoutSweeping
	// LookoutAwaitingRetry: Sweep failed; Earpiece injected; waiting for session to resume.
	LookoutAwaitingRetry
	// LookoutFallen: maxRetries exhausted or oscillation detected; escalated to Mastermind.
	LookoutFallen
	// LookoutStopped: Context cancelled; goroutine exited cleanly.
	LookoutStopped
)

// String returns a human-readable name for the LookoutState.
func (s LookoutState) String() string {
	switch s {
	case LookoutIdle:
		return "Idle"
	case LookoutActive:
		return "Active"
	case LookoutSweeping:
		return "Sweeping"
	case LookoutAwaitingRetry:
		return "AwaitingRetry"
	case LookoutFallen:
		return "Fallen"
	case LookoutStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}

// LookoutResult is sent from Lookout to Fixer via doneCh when the Lookout completes.
type LookoutResult struct {
	SessionID  string
	FinalState LookoutState
	Score      *queue.Score // Non-nil when FinalState was Idle after a passing Sweep
	Error      error
}

// LookoutConfig holds the configuration needed to create a Lookout.
type LookoutConfig struct {
	SessionID  string
	GoingDark  bool   // true = Earpiece enabled; false = Supervised mode
	MaxRetries int    // Default 3
	WorkingDir string // Session working directory for Sweep
	Program    string // Program name (for logging)
}

// Lookout is the per-session supervisor goroutine that watches for TaskComplete,
// triggers the Sweep quality gate, and manages the correction retry loop.
//
// State machine: Idle → Active → Sweeping → AwaitingRetry → (back to Sweeping) or Fallen
// All state reads are safe for concurrent callers via RLock.
type Lookout struct {
	cfg LookoutConfig

	mu            sync.RWMutex
	state         LookoutState
	retryCount    int
	failureHashes map[string]bool // for oscillation detection

	// Channels for event/sweep delivery
	taskCompleteCh chan string       // receives session ID on TaskComplete event
	sweepResultCh  chan *SweepResult // receives async sweep results
	earpieceCh     chan *SweepResult // signals Fixer to inject earpiece correction prompt

	// Result delivery to Fixer
	doneCh chan<- LookoutResult

	// Retry history accumulation
	retryHistory []queue.RetryAttempt

	wg     sync.WaitGroup // tracks the run() goroutine for clean shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// NewLookout creates a new Lookout. parentCtx is inherited from the Fixer so that
// Fixer cancellation propagates to all Lookouts. doneCh is a send-only channel the
// Lookout uses to report its final result to the Fixer.
func NewLookout(parentCtx context.Context, cfg LookoutConfig, doneCh chan<- LookoutResult) *Lookout {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	ctx, cancel := context.WithCancel(parentCtx)
	return &Lookout{
		cfg:            cfg,
		state:          LookoutIdle,
		failureHashes:  make(map[string]bool),
		taskCompleteCh: make(chan string, 1),
		sweepResultCh:  make(chan *SweepResult, 1),
		earpieceCh:     make(chan *SweepResult, 1),
		doneCh:         doneCh,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// State returns the current LookoutState. Safe for concurrent reads.
func (l *Lookout) State() LookoutState {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.state
}

// setState transitions the Lookout to a new state. Must be called within run() goroutine.
func (l *Lookout) setState(s LookoutState) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state = s
}

// OnTaskComplete signals the Lookout that a TaskComplete event occurred for its session.
// Called by the Fixer's ReviewQueueObserver. Non-blocking (buffered channel).
func (l *Lookout) OnTaskComplete() {
	select {
	case l.taskCompleteCh <- l.cfg.SessionID:
	default:
		// Already queued, ignore duplicate
	}
}

// Start launches the Lookout's run() goroutine.
func (l *Lookout) Start() {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.run()
	}()
}

// Stop cancels the Lookout's context and waits for run() to exit.
func (l *Lookout) Stop() {
	l.cancel()
	l.wg.Wait()
}

// EarpieceCh returns the read-only channel the Fixer uses to receive SweepResults
// that require earpiece correction injection.
func (l *Lookout) EarpieceCh() <-chan *SweepResult {
	return l.earpieceCh
}

// run is the Lookout's main goroutine implementing the state machine:
//
//	Idle -> Sweeping          (on taskCompleteCh signal)
//	Sweeping -> Idle          (sweep passed; sends LookoutResult to doneCh)
//	Sweeping -> AwaitingRetry (sweep failed, retryCount < maxRetries)
//	Sweeping -> Fallen        (sweep failed, retryCount >= maxRetries OR oscillation)
//	AwaitingRetry -> Idle     (after backoff; increments retryCount)
//	Fallen -> (stays until ctx.Done)
func (l *Lookout) run() {
	defer func() {
		if r := recover(); r != nil {
			log.ErrorLog.Printf("%s: %s: %v", lookoutRunRecoverMsg, l.cfg.SessionID, r)
		}
		l.setState(LookoutStopped)
	}()

	for {
		switch l.State() {
		case LookoutIdle:
			select {
			case <-l.ctx.Done():
				return
			case sessionID := <-l.taskCompleteCh:
				_ = sessionID
				l.handleTaskComplete()
			}

		case LookoutSweeping:
			// Launch sweep as a sub-goroutine; result delivered to sweepResultCh.
			go l.runSweepAsync()
			select {
			case <-l.ctx.Done():
				return
			case result := <-l.sweepResultCh:
				l.handleSweepResult(result)
			}

		case LookoutAwaitingRetry:
			l.mu.RLock()
			attempt := l.retryCount
			l.mu.RUnlock()
			select {
			case <-l.ctx.Done():
				return
			case <-time.After(backoffDuration(attempt)):
				l.mu.Lock()
				l.retryCount++
				l.mu.Unlock()
				// Transition to Sweeping (not Idle) so the retry actually re-runs the sweep
				// without waiting for another TaskComplete signal that will never arrive.
				l.setState(LookoutSweeping)
			}

		case LookoutFallen:
			select {
			case <-l.ctx.Done():
				return
			}

		case LookoutStopped:
			return
		}
	}
}

// handleTaskComplete transitions from Idle to Sweeping and resets state for a
// fresh sweep cycle.
func (l *Lookout) handleTaskComplete() {
	l.mu.Lock()
	l.retryCount = 0
	l.retryHistory = nil
	l.failureHashes = make(map[string]bool)
	l.mu.Unlock()

	log.InfoLog.Printf("[Lookout:%s] TaskComplete received, starting sweep", l.cfg.SessionID)
	l.setState(LookoutSweeping)
}

// runSweepAsync executes the sweep pipeline in a goroutine and sends the result
// to sweepResultCh. It is always launched from the Sweeping state.
func (l *Lookout) runSweepAsync() {
	defer func() {
		if r := recover(); r != nil {
			log.ErrorLog.Printf("[Lookout:%s] panic in runSweepAsync: %v", l.cfg.SessionID, r)
			select {
			case l.sweepResultCh <- &SweepResult{Status: SweepStatusError}:
			default:
			}
		}
	}()

	runner, err := DetectTestRunner(l.cfg.WorkingDir)
	if err != nil || runner == nil {
		log.InfoLog.Printf("[Lookout:%s] No test runner detected (err=%v)", l.cfg.SessionID, err)
		select {
		case l.sweepResultCh <- &SweepResult{Status: SweepStatusNoTestsFound}:
		default:
		}
		return
	}

	log.InfoLog.Printf("[Lookout:%s] Running sweep with %s (%s)", l.cfg.SessionID, runner.Name, runner.Command)
	result, err := RunSweep(l.ctx, l.cfg.WorkingDir, runner)
	if err != nil {
		log.ErrorLog.Printf("[Lookout:%s] Sweep error: %v", l.cfg.SessionID, err)
		select {
		case l.sweepResultCh <- &SweepResult{Status: SweepStatusError}:
		default:
		}
		return
	}

	diffSummary, _ := CollectDiffSummary(l.cfg.WorkingDir)
	result.DiffSummary = diffSummary
	select {
	case l.sweepResultCh <- result:
	default:
		// Context was cancelled; discard result to avoid goroutine block.
		log.DebugLog.Printf("[Lookout:%s] sweepResultCh full on send, result discarded", l.cfg.SessionID)
	}
}

// handleSweepResult processes the outcome of a sweep and transitions the state
// machine accordingly.
func (l *Lookout) handleSweepResult(result *SweepResult) {
	if result == nil {
		log.ErrorLog.Printf("[Lookout:%s] Received nil sweep result", l.cfg.SessionID)
		l.setState(LookoutIdle)
		return
	}

	switch {
	case result.Status == SweepStatusPass || result.Status == SweepStatusNoTestsFound:
		// Sweep passed (or no tests to run -- treat as success).
		score := l.assembleScore(result)

		l.mu.Lock()
		l.retryCount = 0
		l.mu.Unlock()

		log.InfoLog.Printf("[Lookout:%s] Sweep passed (status=%d), delivering score to Fixer",
			l.cfg.SessionID, result.Status)

		// Non-blocking send to doneCh so the run loop does not deadlock if nobody
		// is reading yet.
		select {
		case l.doneCh <- LookoutResult{
			SessionID:  l.cfg.SessionID,
			FinalState: LookoutIdle,
			Score:      score,
		}:
		default:
			log.ErrorLog.Printf("[Lookout:%s] doneCh full, dropping result", l.cfg.SessionID)
		}

		l.setState(LookoutIdle)

	default:
		// Failure path: SweepStatusFail, SweepStatusTimeout, SweepStatusError.
		l.mu.RLock()
		retries := l.retryCount
		maxRetries := l.cfg.MaxRetries
		l.mu.RUnlock()

		log.InfoLog.Printf("[Lookout:%s] Sweep failed (status=%d, attempt=%d/%d)",
			l.cfg.SessionID, result.Status, retries, maxRetries)

		if retries >= maxRetries {
			// Exhausted retries -- fall.
			log.InfoLog.Printf("[Lookout:%s] Max retries exhausted, falling", l.cfg.SessionID)
			select {
			case l.doneCh <- LookoutResult{
				SessionID:  l.cfg.SessionID,
				FinalState: LookoutFallen,
				Score:      l.assembleScore(result),
			}:
			default:
				log.ErrorLog.Printf("[Lookout:%s] doneCh full, dropping fallen result", l.cfg.SessionID)
			}
			l.setState(LookoutFallen)
			return
		}

		// Record retry attempt.
		reason := result.TestOutput
		const maxReasonLen = 500
		if len(reason) > maxReasonLen {
			reason = reason[len(reason)-maxReasonLen:]
		}
		attempt := queue.RetryAttempt{
			Number:        int32(retries + 1),
			FailureReason: reason,
			TimestampMs:   time.Now().UnixMilli(),
		}

		l.mu.Lock()
		l.retryHistory = append(l.retryHistory, attempt)

		// Oscillation detection: if we have seen the same failure hash before,
		// the session is flip-flopping and should fall immediately.
		if result.FailureHash != "" && l.failureHashes[result.FailureHash] {
			l.mu.Unlock()
			log.InfoLog.Printf("[Lookout:%s] Oscillation detected (hash=%s), falling",
				l.cfg.SessionID, result.FailureHash[:min(16, len(result.FailureHash))])
			select {
			case l.doneCh <- LookoutResult{
				SessionID:  l.cfg.SessionID,
				FinalState: LookoutFallen,
				Score:      l.assembleScore(result),
			}:
			default:
			}
			l.setState(LookoutFallen)
			return
		}
		if result.FailureHash != "" {
			l.failureHashes[result.FailureHash] = true
		}
		l.mu.Unlock()

		// Signal the Fixer to inject an earpiece correction prompt only when
		// autonomous mode (GoingDark) is enabled. In Supervised mode the score
		// is assembled but no injection occurs — the human reviews the failure.
		if l.cfg.GoingDark {
			select {
			case l.earpieceCh <- result:
			default:
				// Channel full -- Fixer will pick it up on next read.
			}
		} else {
			log.DebugLog.Printf("[Lookout:%s] Supervised mode: skip earpiece injection (GoingDark=false)", l.cfg.SessionID)
		}

		l.setState(LookoutAwaitingRetry)
	}
}

// backoffDuration returns the wait time before the next retry attempt.
func backoffDuration(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return 5 * time.Second
	case attempt == 2:
		return 10 * time.Second
	default:
		return 20 * time.Second
	}
}

// assembleScore builds the Score from a passing SweepResult and accumulated retry history.
func (l *Lookout) assembleScore(result *SweepResult) *queue.Score {
	const maxExcerptLen = 2000
	excerpt := result.TestOutput
	if len(excerpt) > maxExcerptLen {
		excerpt = excerpt[len(excerpt)-maxExcerptLen:]
	}

	tr := &queue.TestResults{
		Passed:           result.Status == SweepStatusPass,
		OutputExcerpt:    excerpt,
		DurationMs:       result.Duration.Milliseconds(),
		FailingTestNames: result.FailingTests,
	}

	rh := &queue.RetryHistory{
		AttemptCount: int32(l.retryCount),
		MaxRetries:   int32(l.cfg.MaxRetries),
		Attempts:     l.retryHistory,
	}

	score := &queue.Score{
		TestResults:  tr,
		RetryHistory: rh,
	}

	if result.DiffSummary != nil {
		score.DiffSummary = result.DiffSummary
	}

	return score
}
