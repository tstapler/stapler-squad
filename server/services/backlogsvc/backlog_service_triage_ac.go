package backlogsvc

// backlog_service_triage_ac.go — the acceptance-criteria gate at the end of headless
// triage: an item must leave triage with at least one verifiable criterion, or the owner
// is told why it cannot.

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services/notify"
	"github.com/tstapler/stapler-squad/session/backlog/pipeline"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/headless"
)

// acDerivationBudget bounds the secondary derivation call; it is a short JSON-only prompt.
const acDerivationBudget = 3 * time.Minute

// reasonNoAcceptanceCriteria is the structured-log reason tag for an item that could not
// be given acceptance criteria. It must never appear on a review verdict.
const reasonNoAcceptanceCriteria = "no_acceptance_criteria"

// itemHasAcceptanceCriteria reports whether the item already carries at least one criterion.
func itemHasAcceptanceCriteria(item *backlogdto.BacklogItemData) bool {
	if item.AcceptanceCriteria == "" {
		return false
	}
	criteria, err := domain.ParseAcCriteria(item.AcceptanceCriteria)
	return err == nil && len(criteria) > 0
}

// deriveAcFromDescription runs one focused LLM call that returns acceptance criteria for
// the item. An empty slice with a nil error means the model judged the item underivable.
func deriveAcFromDescription(ctx context.Context, caller headless.PoolClient, model, workDir string,
	item *backlogdto.BacklogItemData, triageSummary string) ([]domain.AcCriterion, error) {
	raw, err := caller.CallBlocking(ctx, headless.FeatureKeyTriage, headless.HeadlessTriageSystemPrompt(),
		pipeline.BuildACDerivationPrompt(item, triageSummary),
		headless.CallOptions{WorkDir: workDir, Model: model}, func(float64, bool) {})
	if err != nil {
		return nil, fmt.Errorf("acceptance-criteria derivation call: %w", err)
	}
	parsed, err := pipeline.ParseHeadlessTriageResult(raw)
	if err != nil {
		return nil, fmt.Errorf("acceptance-criteria derivation parse: %w", err)
	}
	return parsed.AcceptanceCriteria, nil
}

// ensureTriageAcceptanceCriteria guarantees result carries criteria when the item has none,
// deriving them with a second call. It returns false when none could be derived.
func (s *BacklogService) ensureTriageAcceptanceCriteria(caller headless.PoolClient, model, workDir string,
	item *backlogdto.BacklogItemData, result *pipeline.HeadlessTriageResult) bool {
	if len(result.AcceptanceCriteria) > 0 || itemHasAcceptanceCriteria(item) {
		return true
	}
	ctx, cancel := context.WithTimeout(s.shutdownCtx, acDerivationBudget)
	defer cancel()
	generated, err := deriveAcFromDescription(ctx, caller, model, workDir, item, result.Summary)
	if err != nil {
		log.Warn("[TriggerTriage] acceptance-criteria derivation failed", "item", item.ID, "error", err)
		return false
	}
	if len(generated) == 0 {
		return false
	}
	result.AcceptanceCriteria = generated
	log.Info("[TriggerTriage] derived acceptance criteria from description", "item", item.ID, "count", len(generated))
	return true
}

// notifyMissingAcceptanceCriteria tells the owner triage could not derive criteria; the item
// stays at idea. No-op without an event bus.
func (s *BacklogService) notifyMissingAcceptanceCriteria(itemID, itemTitle string) {
	log.Warn("[TriggerTriage] triage could not derive acceptance criteria; item left at idea", "item", itemID, "reason", reasonNoAcceptanceCriteria)
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(events.NewNotificationEvent(
		itemID, "", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		notify.DerivePriority(true, true),
		"Triage needs acceptance criteria",
		fmt.Sprintf("%s — triage could not derive a verifiable acceptance criterion from the description. Add at least one criterion, then re-trigger triage.", itemTitle),
		map[string]string{"item_id": itemID, "reason": reasonNoAcceptanceCriteria},
	))
}
