package workflows

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
)

// StartRetentionEnforcer starts a background goroutine that periodically archives
// completed workflow sessions according to per-workflow retention settings:
//
//   - archive_after_hours > 0: archive completed sessions that stopped more than
//     N hours ago (requires maybeAutoArchive to be suppressed for these workflows)
//   - keep_sessions > 0: keep only the N most recent completed sessions, archiving
//     older ones
//
// Guards:
//   - Never archives sessions with status Active (1), Creating (0), or Paused (2)
//   - archive_after_hours == 0 means disabled (skip time-based archival for that workflow)
//   - keep_sessions == 0 means disabled (keep all sessions)
//
// The goroutine exits when ctx is cancelled.
func StartRetentionEnforcer(
	ctx context.Context,
	entClient *ent.Client,
	workflowRepo session.WorkflowRepository,
	interval time.Duration,
) {
	if entClient == nil || workflowRepo == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run once immediately after startup.
		runRetentionSweep(ctx, entClient, workflowRepo)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runRetentionSweep(ctx, entClient, workflowRepo)
			}
		}
	}()
	log.Info("[workflows/retention] retention enforcer started", "interval", interval)
}

// RunRetentionSweep performs a single retention sweep. Exported for use in tests.
func RunRetentionSweep(ctx context.Context, entClient *ent.Client, workflowRepo session.WorkflowRepository) {
	runRetentionSweep(ctx, entClient, workflowRepo)
}

func runRetentionSweep(ctx context.Context, entClient *ent.Client, workflowRepo session.WorkflowRepository) {
	workflows, err := workflowRepo.ListAll(ctx)
	if err != nil {
		log.Warn("[workflows/retention] failed to list workflows", "err", err)
		return
	}

	now := time.Now()
	totalArchived := 0

	for _, wf := range workflows {
		archived := 0

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
						int(session.Active),
						int(session.Creating),
						int(session.Paused),
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
						int(session.Active),
						int(session.Creating),
						int(session.Paused),
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
				n, err := entClient.Session.Update().
					Where(entsession.IDIn(excess...)).
					SetArchivedAt(now).
					Save(ctx)
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
