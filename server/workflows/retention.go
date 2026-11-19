package workflows

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/stagecfg"
)

// LiveSessionStopper kills the process behind a live session (by title) and marks its
// in-memory Instance archived, so archiving the DB row does not leak a tmux pane or let the
// instance write Active back. A nil stopper archives rows only (tests, no live instances).
type LiveSessionStopper func(title string) error

// Option configures the retention enforcer.
type Option func(*sweepConfig)

type sweepConfig struct{ stopper LiveSessionStopper }

// WithLiveSessionStopper makes Phase 0 stop live instances before archiving their rows.
func WithLiveSessionStopper(s LiveSessionStopper) Option {
	return func(c *sweepConfig) { c.stopper = s }
}

func newSweepConfig(opts []Option) sweepConfig {
	var c sweepConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// StartRetentionEnforcer starts a background goroutine that periodically archives
// completed workflow sessions according to per-workflow retention settings:
//
//   - archive_after_hours > 0: archive completed sessions that stopped more than
//     N hours ago (requires maybeAutoArchive to be suppressed for these workflows),
//     and reap Active/Creating sessions silent for that long (Phase 0)
//   - keep_sessions > 0: keep only the N most recent completed sessions, archiving
//     older ones
//
// Guards:
//   - Phases 1-2 never archive Active (1), Creating (0), or Paused (2); only Phase 0
//     reaps Active/Creating sessions, and only after archive_after_hours of silence
//   - archive_after_hours == 0 means disabled (skip time-based archival for that workflow)
//   - keep_sessions == 0 means disabled (keep all sessions)
//
// The goroutine exits when ctx is cancelled and is tracked on wg so the caller
// can join it.
func StartRetentionEnforcer(
	ctx context.Context,
	entClient *ent.Client,
	workflowRepo stagecfg.WorkflowRepository,
	interval time.Duration,
	wg *sync.WaitGroup,
	opts ...Option,
) {
	cfg := newSweepConfig(opts)
	if entClient == nil || workflowRepo == nil {
		return
	}
	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run once immediately after startup.
		runRetentionSweep(ctx, entClient, workflowRepo, cfg)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runRetentionSweep(ctx, entClient, workflowRepo, cfg)
			}
		}
	})
	log.Info("[workflows/retention] retention enforcer started", "interval", interval)
}

// RunRetentionSweep performs a single retention sweep. Exported for use in tests.
func RunRetentionSweep(ctx context.Context, entClient *ent.Client, workflowRepo stagecfg.WorkflowRepository, opts ...Option) {
	runRetentionSweep(ctx, entClient, workflowRepo, newSweepConfig(opts))
}

// archiveExcessSessions archives the keep_sessions overflow, re-applying the
// same predicates the ID query already used. They are not redundant: the query
// and this update are two separate round trips, so a session revived in between
// (poller revival, boot hot-restore, a manual RetryNow) would otherwise be
// archived while still Active — one of the two paths that produce the
// Active + archived state ADR-001's load-time self-heal has to clean up.
func archiveExcessSessions(ctx context.Context, entClient *ent.Client, excess []int, now time.Time) (int, error) {
	return entClient.Session.Update().
		Where(
			entsession.IDIn(excess...),
			entsession.ArchivedAtIsNil(),
			entsession.StatusNotIn(
				int(domain.Active),
				int(domain.Creating),
				int(domain.Paused),
			),
		).
		SetArchivedAt(now).
		Save(ctx)
}

// archiveStuckLiveSessions reaps Active/Creating sessions with no activity since cutoff.
// Activity is last_meaningful_output, falling back to updated_at when it was never set:
// updated_at alone is unusable because a service restart rewrites it for every row.
// Paused is excluded — it is a deliberate user state. Each candidate's process is stopped
// via cfg.stopper first; a session whose stop fails is left for the next sweep rather than
// archived with a live pane. The final UPDATE re-applies the predicate and is atomic.
func archiveStuckLiveSessions(ctx context.Context, entClient *ent.Client, cfg sweepConfig, workflowID string, now, cutoff time.Time) (int, error) {
	stuck := entsession.And(
		entsession.WorkflowID(workflowID),
		entsession.ArchivedAtIsNil(),
		entsession.StatusIn(int(domain.Active), int(domain.Creating)),
		entsession.Or(
			entsession.LastMeaningfulOutputLT(cutoff),
			entsession.And(
				entsession.LastMeaningfulOutputIsNil(),
				entsession.UpdatedAtLT(cutoff),
			),
		),
	)
	rows, err := entClient.Session.Query().Where(stuck).All(ctx)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		if cfg.stopper != nil {
			if err := cfg.stopper(r.Title); err != nil {
				log.Warn("[workflows/retention] failed to stop stuck session; will retry next sweep",
					"session", r.Title, "err", err)
				continue
			}
		}
		ids = append(ids, r.ID)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return entClient.Session.Update().
		Where(entsession.IDIn(ids...), stuck).
		SetStatus(int(domain.Stopped)).
		SetArchivedAt(now).
		Save(ctx)
}

func runRetentionSweep(ctx context.Context, entClient *ent.Client, workflowRepo stagecfg.WorkflowRepository, cfg sweepConfig) {
	workflows, err := workflowRepo.ListAll(ctx)
	if err != nil {
		log.Warn("[workflows/retention] failed to list workflows", "err", err)
		return
	}

	now := time.Now()
	totalArchived := 0

	for _, wf := range workflows {
		archived := 0

		// Phase 0: reap workflow sessions stuck Active/Creating past archive_after_hours
		// (e.g. blocked at an unanswerable confirmation prompt).
		if wf.ArchiveAfterHours > 0 {
			n, err := archiveStuckLiveSessions(ctx, entClient, cfg, wf.ID.String(), now,
				now.Add(-time.Duration(wf.ArchiveAfterHours)*time.Hour))
			if err != nil {
				log.Warn("[workflows/retention] stuck-active archive failed",
					"workflow", wf.Slug, "err", err)
			} else if n > 0 {
				log.Warn("[workflows/retention] archived stuck Active/Creating sessions",
					"workflow", wf.Slug, "count", n, "archive_after_hours", wf.ArchiveAfterHours)
				archived += n
			}
		}

		// Phase 1: time-based archival (archive_after_hours > 0)
		if wf.ArchiveAfterHours > 0 {
			cutoff := now.Add(-time.Duration(wf.ArchiveAfterHours) * time.Hour)
			n, err := entClient.Session.Update().
				Where(
					entsession.WorkflowID(wf.ID.String()),
					entsession.ArchivedAtIsNil(),
					// Only archive terminal sessions — never active/creating/paused.
					// DB status values: Creating=0, Active=1, Paused=2, Stopped=3, Hibernated=4
					entsession.StatusNotIn(
						int(domain.Active),
						int(domain.Creating),
						int(domain.Paused),
					),
					entsession.UpdatedAtLT(cutoff),
				).
				SetArchivedAt(now).
				Save(ctx)
			if err != nil {
				log.Warn("[workflows/retention] time-based archive failed",
					"workflow", wf.Slug, "err", err)
			} else {
				archived += n
			}
		}

		// Phase 2: count-based keep_sessions enforcement
		if wf.KeepSessions > 0 {
			// Find all non-archived, non-live sessions for this workflow ordered by creation time DESC.
			ids, err := entClient.Session.Query().
				Where(
					entsession.WorkflowID(wf.ID.String()),
					entsession.ArchivedAtIsNil(),
					entsession.StatusNotIn(
						int(domain.Active),
						int(domain.Creating),
						int(domain.Paused),
					),
				).
				Order(entsession.ByCreatedAt()).
				IDs(ctx)
			if err != nil {
				log.Warn("[workflows/retention] keep_sessions query failed",
					"workflow", wf.Slug, "err", err)
				continue
			}

			// IDs are returned oldest-first. We want to keep the N newest,
			// so archive everything except the last KeepSessions entries.
			if len(ids) > wf.KeepSessions {
				excess := ids[:len(ids)-wf.KeepSessions] // oldest entries
				n, err := archiveExcessSessions(ctx, entClient, excess, now)
				if err != nil {
					log.Warn("[workflows/retention] keep_sessions archive failed",
						"workflow", wf.Slug, "err", err)
				} else {
					archived += n
				}
			}
		}

		if archived > 0 {
			log.Info("[workflows/retention] archived sessions for workflow",
				"workflow", wf.Slug, "archived", archived)
			totalArchived += archived
		}
	}

	if totalArchived > 0 {
		log.Info("[workflows/retention] sweep complete", "total_archived", totalArchived)
	}
}
