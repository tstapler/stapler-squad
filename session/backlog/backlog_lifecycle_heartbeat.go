package backlog

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// heartbeatMinAdvance is how far output must move past last_progress_at before
// the heartbeat writes, so a chatty pane costs at most one write per interval.
const heartbeatMinAdvance = 30 * time.Second

// SetHeartbeatSource wires the live-instance registry that feeds the session
// heartbeat. Nil leaves last_progress_at commit/note-driven only.
func (l *BacklogLifecycleListener) SetHeartbeatSource(s session.LiveSnapshotSource) {
	l.liveWork.mu.Lock()
	defer l.liveWork.mu.Unlock()
	l.liveWork.heartbeatSource = s
}

func (l *BacklogLifecycleListener) heartbeatSource() session.LiveSnapshotSource {
	l.liveWork.mu.Lock()
	defer l.liveWork.mu.Unlock()
	return l.liveWork.heartbeatSource
}

// refreshWorkSessionHeartbeat advances last_progress_at of each in_progress
// item's open work session to the live instance's LastMeaningfulOutputTime, so
// a session that is producing output without committing is not marked
// stale_work. It never moves the clock backwards and skips ended sessions and
// sessions not live in this process.
func (l *BacklogLifecycleListener) refreshWorkSessionHeartbeat(ctx context.Context) {
	src := l.heartbeatSource()
	if src == nil {
		return
	}
	items, err := l.storage.ListBacklogItems(ctx, backlogdto.BacklogItemFilter{
		Statuses: []string{string(domain.BacklogStatusInProgress)},
	})
	if err != nil {
		log.Warn("[BacklogLifecycle] refreshWorkSessionHeartbeat list error", "error", err)
		return
	}
	now := time.Now()
	for _, item := range items {
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.Warn("[BacklogLifecycle] refreshWorkSessionHeartbeat ListItemSessions failed", "item", item.ID, "error", sessErr)
			continue
		}
		for i := range sessions {
			s := &sessions[i]
			if s.Role != instance.SessionRoleWork || s.EndedAt != nil {
				continue
			}
			snap, ok := src.LiveSnapshot(s.SessionUUID)
			if !ok || snap == nil {
				continue
			}
			out := snap.LastMeaningfulOutput
			if out.IsZero() || out.After(now) {
				continue
			}
			clock := s.CreatedAt
			if s.LastProgressAt != nil {
				clock = *s.LastProgressAt
			}
			if out.Sub(clock) < heartbeatMinAdvance {
				continue
			}
			if touchErr := l.storage.TouchItemSessionProgress(ctx, s.ID, out); touchErr != nil {
				log.Warn("[BacklogLifecycle] refreshWorkSessionHeartbeat touch failed", "item", item.ID, "session", s.SessionUUID, "error", touchErr)
			}
		}
	}
}
