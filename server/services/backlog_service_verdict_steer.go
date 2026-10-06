package services

// backlog_service_verdict_steer.go — delivers a recorded review verdict to the
// item's live work session as a steer, so the session can end its turn after
// request_review instead of polling (wait_for_backlog_event / ScheduleWakeup),
// where every wake re-reads the whole context and can pay a prompt-cache
// rewrite. Reuses SessionSteerer, the same injection path PR-fix steering uses.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
)

const (
	// The session may still be finishing the turn that called request_review
	// when the verdict lands, so wait for it to go idle rather than give up.
	defaultVerdictSteerPollInterval = 5 * time.Second
	defaultVerdictSteerReadyTimeout = 5 * time.Minute

	maxVerdictSummaryBytes = 2000
)

// StartVerdictSteering subscribes to backlog events and steers the item's live
// work session whenever a review verdict is recorded. No-op without an event
// bus or steerer; stops with the service's shutdown context.
func (s *BacklogService) StartVerdictSteering() {
	if s.eventBus == nil || s.sessionSteerer == nil {
		return
	}
	ch, subID := s.eventBus.Subscribe(s.shutdownCtx)
	go func() {
		defer s.eventBus.Unsubscribe(subID)
		for {
			select {
			case <-s.shutdownCtx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				if p := verdictRecordedPayload(evt); p != nil {
					go s.steerWorkSessionWithVerdict(s.shutdownCtx, p)
				}
			}
		}
	}()
}

func verdictRecordedPayload(evt *events.Event) *events.BacklogItemEventPayload {
	if evt == nil || evt.Type != events.EventBacklogItemChanged || evt.BacklogItemPayload == nil {
		return nil
	}
	p := evt.BacklogItemPayload
	if p.Kind != events.BacklogChangeVerdictRecorded || p.IsSnapshot || p.Item == nil || p.Verdict == nil {
		return nil
	}
	return p
}

// steerWorkSessionWithVerdict is a no-op when the item has no live work
// session (e.g. it already exited and the lifecycle listener handles it).
func (s *BacklogService) steerWorkSessionWithVerdict(ctx context.Context, p *events.BacklogItemEventPayload) {
	itemID, itemTitle := p.Item.ID, p.Item.Title
	sessions, err := s.storage.ListItemSessions(ctx, itemID)
	if err != nil {
		log.WarningLog().Printf("[VerdictSteer] ListItemSessions item=%s: %v", itemID, err)
		return
	}
	var workUUID string
	for i := range sessions {
		if sessions[i].Role == session.SessionRoleWork && sessions[i].EndedAt == nil {
			workUUID = sessions[i].SessionUUID
		}
	}
	if workUUID == "" {
		return
	}
	if _, live := s.sessionSteerer.SessionProgram(workUUID); !live {
		return
	}

	if !s.waitForSteerReady(ctx, workUUID) {
		s.notifyVerdictSteerFailed(itemID, itemTitle, workUUID, "session never went idle")
		return
	}
	msg := buildVerdictSteerMessage(itemID, p.Verdict)
	if err := s.sessionSteerer.SteerActiveSession(ctx, workUUID, msg); err != nil {
		log.WarningLog().Printf("[VerdictSteer] steer session=%s item=%s: %v", workUUID, itemID, err)
		s.notifyVerdictSteerFailed(itemID, itemTitle, workUUID, err.Error())
		return
	}
	log.InfoLog().Printf("[VerdictSteer] delivered %s verdict to session=%s item=%s", p.Verdict.OverallOutcome, workUUID, itemID)
}

func (s *BacklogService) waitForSteerReady(ctx context.Context, sessionUUID string) bool {
	poll, timeout := s.verdictSteerPollInterval, s.verdictSteerReadyTimeout
	if poll <= 0 {
		poll = defaultVerdictSteerPollInterval
	}
	if timeout <= 0 {
		timeout = defaultVerdictSteerReadyTimeout
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		if s.sessionSteerer.IsReadyForSteer(sessionUUID) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-tick.C:
		}
	}
}

func buildVerdictSteerMessage(itemID string, v *session.ReviewVerdictData) string {
	summary := truncateUTF8Bytes(strings.TrimSpace(v.Summary), maxVerdictSummaryBytes)
	var next string
	if v.OverallOutcome == session.ReviewOutcomePass {
		next = "Run /backlog/ship now to open the pull request yourself."
	} else {
		next = fmt.Sprintf("Fix the noted gaps in this same session and call request_review again (at most %d review attempts per session; after that run /backlog/ship to hand off to a human).", session.MaxSameSessionReviewAttempts)
	}
	return fmt.Sprintf("Review verdict for backlog item %s: %s.\n%s\n\n%s\nThis message is the verdict delivery; do not poll with wait_for_backlog_event or ScheduleWakeup.",
		itemID, v.OverallOutcome, summary, next)
}

func (s *BacklogService) notifyVerdictSteerFailed(itemID, itemTitle, sessionUUID, reason string) {
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		derivePriority(false, true),
		fmt.Sprintf("Could not deliver review verdict — %s", itemTitle),
		fmt.Sprintf("%s — session %s did not receive its review verdict (%s). Tell it to run /backlog/status.", itemTitle, sessionUUID, reason),
		map[string]string{"item_id": itemID},
	))
}
