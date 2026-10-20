package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// notifyTransitionDenied makes a denied automatic transition visible: a
// transition_denied stuck row, one progress comment and one notification,
// idempotent per (item, caller, from, to, reason) while the row stays open. It
// reports whether it recorded anything new. Service-layer callers only.
func (s *BacklogService) notifyTransitionDenied(ctx context.Context, item *session.BacklogItemData, d domain.TransitionDenial, denyErr error) bool {
	if s.storage == nil {
		return false
	}
	if s.transitionDenialAlreadyOpen(ctx, item.ID, d) {
		return false
	}

	stuckCtx := domain.FormatTransitionDeniedContext(d, denyErr.Error())
	applied, err := s.storage.MarkStuck(ctx, item.ID, domain.StuckReasonTransitionDenied, session.BacklogStatus(d.From), stuckCtx)
	if err != nil {
		log.Warn("[notifyTransitionDenied] MarkStuck failed", "item", item.ID, "error", err)
		return false
	}
	if !applied {
		return false
	}
	note := fmt.Sprintf("automatic %s denied: from=%s to=%s reason=%s", d.Caller, d.From, d.To, d.Reason)
	if noteErr := s.storage.AppendProgressNote(ctx, item.ID, -1, note, d.From); noteErr != nil {
		log.Warn("[notifyTransitionDenied] AppendProgressNote failed", "item", item.ID, "error", noteErr)
	}
	if _, notifyErr := s.storage.MarkStuckNotified(ctx, item.ID, domain.StuckReasonTransitionDenied); notifyErr != nil {
		log.Warn("[notifyTransitionDenied] MarkStuckNotified failed", "item", item.ID, "error", notifyErr)
	}
	if s.eventBus != nil {
		s.eventBus.Publish(events.NewNotificationEvent(
			item.ID, "", uuid.New().String(),
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR),
			derivePriority(true, false),
			"Automatic transition denied",
			fmt.Sprintf("%s — %s", item.Title, note),
			map[string]string{"item_id": item.ID},
		))
	}
	return true
}

// transitionDenialAlreadyOpen reports whether an un-snoozed transition_denied
// row for exactly d is open (a read error counts as open: stay quiet, retry
// next sweep).
func (s *BacklogService) transitionDenialAlreadyOpen(ctx context.Context, itemID string, d domain.TransitionDenial) bool {
	rows, err := s.storage.FindOpenStuckStates(ctx)
	if err != nil {
		log.Warn("[notifyTransitionDenied] FindOpenStuckStates failed", "item", itemID, "error", err)
		return true
	}
	for _, row := range rows {
		if row.ItemID != itemID || row.Reason != domain.StuckReasonTransitionDenied {
			continue
		}
		if prev, ok := domain.ParseTransitionDeniedContext(row.Context); ok && prev == d {
			return true
		}
	}
	return false
}

// transitionDeniedReporter adapts notifyTransitionDenied to the gate's
// session.TransitionDeniedReporter, so session callers reach the service-layer
// stuck row, comment and notification without importing this package.
type transitionDeniedReporter struct{ svc *BacklogService }

// TransitionDeniedReporter returns the reporter to inject into
// session.Storage.SetTransitionGate. Idempotence per (item, caller, from, to,
// reason) is notifyTransitionDenied's.
func (s *BacklogService) TransitionDeniedReporter() session.TransitionDeniedReporter {
	return transitionDeniedReporter{svc: s}
}

// Report implements session.TransitionDeniedReporter.
func (r transitionDeniedReporter) Report(ctx context.Context, item *session.BacklogItemData, caller session.TransitionCaller, from, to session.BacklogStatus, reason string, denyErr error) {
	r.svc.notifyTransitionDenied(ctx, item, domain.TransitionDenial{
		Caller: string(caller),
		From:   string(from),
		To:     string(to),
		Reason: reason,
	}, denyErr)
}

// resolveTransitionDeniedLogged clears an open transition_denied row once the
// same automatic caller succeeds, so a denial is never a permanent park.
func (s *BacklogService) resolveTransitionDeniedLogged(ctx context.Context, itemID string) {
	if s.storage == nil {
		return
	}
	if _, err := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonTransitionDenied); err != nil {
		log.Warn("[resolveTransitionDeniedLogged] ResolveStuck failed", "item", itemID, "error", err)
	}
}

// isEngineDenial reports whether err is the workflow engine refusing a
// transition (graph or gate) rather than a write failure; the gate has already
// reported such a denial for an agent or system caller.
func isEngineDenial(err error) bool {
	return errors.Is(err, session.ErrTransitionNotAllowed) || errors.Is(err, session.ErrGateNotSatisfied)
}

// heldByTransitionDenial is the one guard every spawn point calls (Story
// 1.5.8): true, with a Debug line, while item's status has an open
// transition_denied row the engine still refuses, so the automatic action
// skips instead of churning a session for a transition that cannot happen.
// A lifted denial is resolved here and the action proceeds. Periodic callers
// treat the skip as success; the operator's "Retry now" asks
// transitionDenialHoldError instead.
func (s *BacklogService) heldByTransitionDenial(ctx context.Context, spawnPoint string, item *session.BacklogItemData) bool {
	if s.storage == nil || !s.storage.TransitionDenialHolds(ctx, item) {
		return false
	}
	log.Debug("["+spawnPoint+"] item waits on a denied transition, skipping", "item", item.ID, "status", item.Status)
	return true
}

// transitionDenialHoldError returns session.ErrTransitionDeniedHold, naming the
// denial, while itemID is held by an open denial; nil otherwise (including on
// a read error: the action itself then decides, as before the denial existed).
func (s *BacklogService) transitionDenialHoldError(ctx context.Context, itemID string) error {
	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		return nil
	}
	d, open, err := s.storage.OpenTransitionDenial(ctx, itemID, session.BacklogStatus(item.Status))
	if err != nil || !open || !s.storage.TransitionDenialHolds(ctx, item) {
		return nil
	}
	return fmt.Errorf("%w: %s %s->%s denied (%s); satisfy the gate or re-enable the edge", session.ErrTransitionDeniedHold, d.Caller, d.From, d.To, d.Reason)
}
