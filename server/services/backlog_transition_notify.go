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

// Denied-transition callers and reasons recorded on a transition_denied row.
const (
	transitionCallerDequeueClaim = "dequeue_claim"
	deniedReasonConfigGraph      = "config_graph"
	deniedReasonOperatorGate     = "operator_gate"
)

// classifyTransitionDenial maps an engine denial to its denied_reason. ok is
// false for errors that are not engine denials (other gate errors, CAS races).
func classifyTransitionDenial(err error) (reason string, ok bool) {
	switch {
	case errors.Is(err, session.ErrTransitionNotAllowed):
		return deniedReasonConfigGraph, true
	case errors.Is(err, session.ErrGateNotSatisfied):
		return deniedReasonOperatorGate, true
	}
	return "", false
}

// notifyClaimDenied records an engine denial of the dequeue claim; a no-op for
// errors classifyTransitionDenial does not recognise.
func (s *BacklogService) notifyClaimDenied(ctx context.Context, item *session.BacklogItemData, from session.BacklogStatus, denyErr error) {
	reason, ok := classifyTransitionDenial(denyErr)
	if !ok {
		return
	}
	s.notifyTransitionDenied(ctx, item, domain.TransitionDenial{
		Caller: transitionCallerDequeueClaim,
		From:   string(from),
		To:     string(session.BacklogStatusInProgress),
		Reason: reason,
	}, denyErr)
}

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
