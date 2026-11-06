package services

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// guidance_session_reaper.go cancels hook and peer guidance rows when their
// session ends. It lives outside session/backlog_lifecycle.go on purpose: a
// session-scope dialog is not backlog-domain.

// guidanceRetentionSweepEvery is how often the reaper purges old questions_json.
const guidanceRetentionSweepEvery = 6 * time.Hour

// SessionEnds answers the reaper's two questions about sessions.
type SessionEnds interface {
	// ResolveUUID maps an event's SessionID (a UUID or a title) to a UUID; "" when unknown.
	ResolveUUID(id string) string
	// Ended reports whether the session is archived, stopped or missing.
	Ended(sessionUUID string) bool
}

type guidanceReaperStore interface {
	CancelGuidanceRequest(ctx context.Context, id uuid.UUID, reason string) (bool, error)
	ListAllPendingGuidanceRequests(ctx context.Context) ([]*session.GuidanceRequestData, error)
}

// GuidanceSessionReaper cancels open rows whose dialog or peer context dies with
// the session: source ask_user_question_hook and source peer only. It never
// touches agent_mcp rows; those follow Story 3.4's delivery states.
type GuidanceSessionReaper struct {
	store    guidanceReaperStore
	sessions SessionEnds
	mirror   *GuidanceHookMirror
	cleared  func(reason string)
	now      func() time.Time
}

// NewGuidanceSessionReaper builds the reaper. mirror (optional) also runs the
// 30-day questions_json purge and drops cache entries; cleared counts
// guidance_mirror_cleared_total.
func NewGuidanceSessionReaper(store guidanceReaperStore, sessions SessionEnds, mirror *GuidanceHookMirror, cleared func(reason string)) *GuidanceSessionReaper {
	return &GuidanceSessionReaper{store: store, sessions: sessions, mirror: mirror, cleared: cleared, now: time.Now}
}

func reapedSource(s domain.QuestionSource) bool {
	return s == domain.QuestionSourceHook || s == domain.QuestionSourcePeer
}

func rowInvolves(row *session.GuidanceRequestData, sessionUUID string) bool {
	return row.SessionUUID == sessionUUID || row.AskerSessionUUID == sessionUUID || row.AnswererSessionUUID == sessionUUID
}

// HandleEvent reaps the rows of a deleted or archived session.
func (r *GuidanceSessionReaper) HandleEvent(ctx context.Context, ev *events.Event) {
	if ev == nil || (ev.Type != events.EventSessionDeleted && ev.Type != events.EventSessionArchived) {
		return
	}
	sessionUUID := r.sessions.ResolveUUID(ev.SessionID)
	if sessionUUID == "" {
		return // the startup pass catches a session that cannot be resolved now
	}
	r.reap(ctx, func(row *session.GuidanceRequestData) bool { return rowInvolves(row, sessionUUID) })
}

// StartupPass reaps open rows whose session ended while the service was down.
func (r *GuidanceSessionReaper) StartupPass(ctx context.Context) {
	r.reap(ctx, func(row *session.GuidanceRequestData) bool {
		for _, id := range []string{row.AskerSessionUUID, row.AnswererSessionUUID} {
			if id != "" && r.sessions.Ended(id) {
				return true
			}
		}
		return false
	})
}

func (r *GuidanceSessionReaper) reap(ctx context.Context, match func(*session.GuidanceRequestData) bool) {
	rows, err := r.store.ListAllPendingGuidanceRequests(ctx)
	if err != nil {
		log.Warn("[GuidanceReaper] list failed", "err", err)
		return
	}
	for _, row := range rows {
		if !reapedSource(row.Source) || !match(row) {
			continue
		}
		id, perr := uuid.Parse(row.ID)
		if perr != nil {
			continue
		}
		applied, cerr := r.store.CancelGuidanceRequest(ctx, id, domain.CancelReasonSessionEnded.String())
		if cerr != nil {
			log.Warn("[GuidanceReaper] cancel failed", "row", row.ID, "err", cerr)
			continue
		}
		if r.mirror != nil {
			r.mirror.pending.Forget(QuestionKey{SessionUUID: row.AskerSessionUUID, QuestionID: row.ID})
		}
		if applied && r.cleared != nil {
			r.cleared(domain.CancelReasonSessionEnded.String())
		}
	}
}

// Run reaps on session events until ctx is done or the bus closes the channel.
// It first rehydrates the cache and runs the startup pass, so a row of a
// session deleted during downtime is cancelled before it is rehydrated.
func (r *GuidanceSessionReaper) Run(ctx context.Context, bus *events.EventBus) {
	eventCh, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)
	r.StartupPass(ctx)
	if r.mirror != nil {
		if n, err := r.mirror.Rehydrate(ctx); err != nil {
			log.Warn("[GuidanceMirror] rehydrate failed", "err", err)
		} else {
			log.Info("[GuidanceMirror] rehydrated pending hook questions", "count", n)
		}
	}
	r.purge(ctx)
	tick := time.NewTicker(guidanceRetentionSweepEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-eventCh:
			if !ok {
				return
			}
			r.HandleEvent(ctx, ev)
		case <-tick.C:
			r.purge(ctx)
		}
	}
}

func (r *GuidanceSessionReaper) purge(ctx context.Context) {
	if r.mirror == nil {
		return
	}
	if n, err := r.mirror.PurgeTerminalQuestions(ctx, r.now()); err != nil {
		log.Warn("[GuidanceMirror] purge failed", "err", err)
	} else if n > 0 {
		log.Info("[GuidanceMirror] purged old questions_json", "rows", n)
	}
}
