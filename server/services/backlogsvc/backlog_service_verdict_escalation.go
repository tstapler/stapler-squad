package backlogsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ownerChoiceFixed means the owner has resolved the gap outside the app (environment repaired,
// criterion text corrected); the item then reopens for rework like any failed review.
const (
	ownerChoiceFixed = "I fixed it, retry"
	ownerChoiceLeave = "Leave it"
)

// flaggedForOwner returns the verdicts of the latest review that more rework cannot resolve
// and whose criterion is still unmet (a waiver or completion since then clears the flag).
func flaggedForOwner(perCriterionJSON string, criteria []session.AcCriterion) []session.CriterionVerdict {
	var verdicts []session.CriterionVerdict
	if err := json.Unmarshal([]byte(perCriterionJSON), &verdicts); err != nil {
		return nil
	}
	var out []session.CriterionVerdict
	for _, v := range verdicts {
		if !v.NeedsOwner() || v.CriterionIndex < 0 || v.CriterionIndex >= len(criteria) {
			continue
		}
		if st := criteria[v.CriterionIndex].Status; st == session.AcStatusDone || st == session.AcStatusWaived {
			continue
		}
		out = append(out, v)
	}
	return out
}

// escalateVerdictToOwner raises one owner-action request for the first flagged criterion
// instead of reopening the worker blindly. It returns true when the item should stay in
// review (a request is pending or was just raised).
func (s *BacklogService) escalateVerdictToOwner(ctx context.Context, item *session.BacklogItemData, flagged []session.CriterionVerdict) bool {
	v := flagged[0]
	id, err := uuid.Parse(item.ID)
	if err != nil {
		return false
	}
	responses := []string{ownerChoiceFixed, ownerChoiceLeave}
	question := fmt.Sprintf("Reviewer says criterion %d cannot be fixed by more rework (%s). %s", v.CriterionIndex, classLabel(v), truncateForNotification2(v.Evidence))
	if v.ProposedWaiver || v.FailureClass == domain.FailureClassUnattainable || v.FailureClass == domain.FailureClassUnrelatedFailure {
		responses = append([]string{domain.OwnerActionWaiveResponse}, responses...)
		question = domain.FormatOwnerActionWaiver(v.CriterionIndex) + question
	}
	questionText := domain.OwnerActionQuestionPrefix + question
	if prior, found := s.priorEscalation(ctx, item.ID, questionText); found {
		return prior.Answer != ownerChoiceFixed
	}
	row, err := s.storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeBacklogItem,
		ItemID:       &id,
		QuestionText: questionText,
		QuestionType: domain.QuestionTypeMultipleChoice,
		Options:      mustJSON(responses),
	})
	if err != nil {
		// A pending request already covers this item; keep it parked either way.
		log.Info("[AutoReopenAfterFailedReview] owner escalation not created", "item", item.ID, "error", err)
		return true
	}
	if s.eventBus != nil {
		s.eventBus.Publish(events.NewNotificationEvent(
			item.ID, "", uuid.New().String(),
			int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED),
			int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
			"Review needs your decision", question,
			map[string]string{"item_id": item.ID, "guidance_request_id": row.ID},
		))
	}
	return true
}

// priorEscalation finds an earlier request with the same text, so a sweep that re-runs the
// router neither re-asks a pending question nor re-asks one the owner already answered.
func (s *BacklogService) priorEscalation(ctx context.Context, itemID, questionText string) (*session.GuidanceRequestData, bool) {
	rows, _, _, err := s.storage.ListGuidanceRequestsForScope(ctx, domain.RequestScopeBacklogItem, itemID, 0)
	if err != nil {
		return nil, false
	}
	for _, r := range rows {
		if r.QuestionText == questionText {
			return r, true
		}
	}
	return nil, false
}

func classLabel(v session.CriterionVerdict) string {
	if v.FailureClass == "" {
		return "proposed waiver"
	}
	return strings.ToLower(strings.ReplaceAll(string(v.FailureClass), "_", " "))
}

func truncateForNotification2(s string) string {
	const maxEvidence = 300
	r := []rune(s)
	if len(r) <= maxEvidence {
		return s
	}
	return string(r[:maxEvidence]) + "..."
}

func mustJSON(v []string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ResumeAfterOwnerWaiver continues an item parked in review once the owner waived the
// criterion that blocked it. The waived criterion no longer counts as unmet, so the normal
// failed-review reopen runs; if the item moved on already this is a no-op.
func (s *BacklogService) ResumeAfterOwnerWaiver(ctx context.Context, itemID string) {
	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil || session.BacklogStatus(item.Status) != session.BacklogStatusReview {
		return
	}
	if err := s.AutoReopenAfterFailedReview(ctx, itemID); err != nil {
		log.Warn("[ResumeAfterOwnerWaiver] reopen failed", "item", itemID, "error", err)
	}
}
