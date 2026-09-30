package services

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// staleCreationSweeperCheckInterval is how often the sweeper re-evaluates every live
// Creating instance for staleness. Mirrors StaleSessionNotifier's cadence (60s is
// fine-grained enough relative to the (minutes-scale) configured threshold, without
// meaningful I/O overhead beyond the occasional flip's terminal write).
const staleCreationSweeperCheckInterval = 60 * time.Second

// staleCreationWriteTimeout bounds each individual commitTerminalStatus call this
// sweeper makes, mirroring the pipeline's own terminal-write timeout
// (runBackgroundResolutionPipeline's writeCtx) so one slow/unavailable DB doesn't
// block the sweep loop indefinitely.
const staleCreationWriteTimeout = 30 * time.Second

// StaleCreationSweeper is the Stale-Creation Sweeper (Epic 4.1): a ticker-driven
// sweeper that scans every Creating-status instance and flips any whose persisted
// creation_progress hasn't been updated within the configured threshold
// (config.CreationStaleConfig) to Failed/FailureReason=Stale.
//
// It reads the *persisted* CreationProgressUpdatedAt (falling back to CreatedAt when
// never updated) rather than any in-process/monotonic clock, so a Creating row left
// over from a process that was killed before this deploy is correctly flipped on this
// process's first sweep -- no in-memory bookkeeping about whether a pipeline goroutine
// for it ever ran here is required (see Task 4.1.2's acceptance criteria).
//
// The flip itself always goes through commitTerminalStatus (never
// instance.TryForceStatusIfEpoch directly), presenting back the instance's
// CreationEpoch() captured at scan time -- so a late-arriving pipeline success that
// already won the durable write can never be clobbered by a stale-flip that started
// evaluating moments earlier (ADR-002's epoch fence).
type StaleCreationSweeper struct {
	poller   *session.ReviewQueuePoller
	storage  terminalStatusStore
	eventBus *events.EventBus
}

// NewStaleCreationSweeper constructs a sweeper. poller supplies the live set of
// instances to scan (via GetInstances()); storage is where the terminal write lands
// (via commitTerminalStatus); eventBus publishes the resulting SessionUpdatedEvent. A
// nil eventBus is tolerated (publish becomes a no-op), matching
// StaleSessionNotifier's construction-time tolerance.
func NewStaleCreationSweeper(poller *session.ReviewQueuePoller, storage terminalStatusStore, eventBus *events.EventBus) *StaleCreationSweeper {
	return &StaleCreationSweeper{
		poller:   poller,
		storage:  storage,
		eventBus: eventBus,
	}
}

// Start runs the periodic sweep loop. Blocks until ctx is cancelled. Mirrors
// StaleSessionNotifier.Start's shape: run once immediately, then on every tick.
func (s *StaleCreationSweeper) Start(ctx context.Context) {
	ticker := time.NewTicker(staleCreationSweeperCheckInterval)
	defer ticker.Stop()

	log.Info("stale creation sweeper started", "check_interval", staleCreationSweeperCheckInterval)

	s.sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("stale creation sweeper stopped")
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep evaluates every live Creating instance against the current configured
// threshold. config.LoadConfig() is called fresh on every invocation, mirroring
// StaleSessionNotifier.checkAll, so a threshold change made via the Settings UI takes
// effect on the very next tick with no server restart required.
func (s *StaleCreationSweeper) sweep(ctx context.Context) {
	cfg := config.LoadConfig()
	threshold := time.Duration(cfg.CreationStale.ThresholdMinutesOrDefault()) * time.Minute

	for _, inst := range s.poller.GetInstances() {
		if session.Status(inst.GetStatus()) != session.Creating {
			continue
		}

		lastProgress := inst.CreationProgressUpdatedAt()
		if lastProgress.IsZero() {
			// Never updated -- e.g. the goroutine that would have called
			// SetCreationProgress never even started. Fall back to the
			// Creating-onset time (Task 4.1.2f).
			lastProgress = inst.GetCreatedAt()
		}

		if time.Since(lastProgress) <= threshold {
			continue
		}

		s.flipStale(ctx, inst)
	}
}

// flipStale commits the terminal Failed/Stale write for one instance found stale
// during sweep. Captures CreationEpoch() immediately before the write so the fence
// presented to commitTerminalStatus reflects this scan, per the sweeper's own
// race-safety guarantee (Task 4.1.2e).
func (s *StaleCreationSweeper) flipStale(ctx context.Context, inst *session.Instance) {
	epoch := inst.CreationEpoch()

	writeCtx, cancel := context.WithTimeout(ctx, staleCreationWriteTimeout)
	defer cancel()

	applied := commitTerminalStatus(writeCtx, s.storage, inst, epoch, session.Failed, "Stale")
	if !applied {
		log.Info("stale creation sweeper: terminal write skipped, epoch already advanced",
			"session", inst.GetStableID(), "epoch", epoch)
		return
	}

	RecordSessionCreationMetrics(ctx, SessionCreationOutcomeStale, time.Since(inst.GetCreatedAt()))

	if s.eventBus != nil {
		s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"status", "creation_progress"}))
	}

	log.Info("stale creation sweeper: flipped Creating instance to Failed/Stale",
		"session", inst.GetStableID())
}
