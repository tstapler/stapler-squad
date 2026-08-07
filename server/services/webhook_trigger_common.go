package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// MaxWebhookBodyBytes bounds the inbound webhook request body for both
// GitHubWebhookHandler and GenericWebhookHandler — a DoS guard (webhook-triggers
// plan.md pitfalls §1.3).
const MaxWebhookBodyBytes = 5 << 20 // 5 MiB

// decryptWorkflowSecret decrypts wf.WebhookSecretEncrypted using the machine
// encryption key. Shared by both webhook handlers.
func decryptWorkflowSecret(cfg *config.Config, wf *ent.Workflow) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config unavailable")
	}
	if wf.WebhookSecretEncrypted == "" {
		return "", fmt.Errorf("workflow %q has no webhook secret configured", wf.Slug)
	}
	key, err := cfg.GetOrCreateEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("get encryption key: %w", err)
	}
	secret, err := session.DecryptToken(key, wf.WebhookSecretEncrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt webhook secret: %w", err)
	}
	return secret, nil
}

// persistTriggerFireEvent writes a TriggerFireEvent row, logging (not propagating) any
// error — an audit-trail write failure must never block or mask the handler's own HTTP
// response, mirroring Scheduler.recordFireEvent's fire-and-forget shape.
func persistTriggerFireEvent(ctx context.Context, repo session.TriggerFireEventRepository, input session.TriggerFireEventInput) {
	if repo == nil {
		return
	}
	if err := repo.Create(ctx, input); err != nil {
		log.Warn("[WebhookReceiver] failed to record trigger fire event",
			"outcome", input.Outcome, "delivery_id", input.DeliveryID, "err", err)
	}
}

// claimTriggerFireEvent atomically claims (workflow_id, delivery_id) via
// fireEvents.Create("pending"). Returns alreadyClaimed=true when
// session.ErrDuplicateDelivery is returned — this specific workflow already processed
// this delivery, which the caller should treat as "nothing more to do," not an error (a
// sibling matched candidate in the same request may still be new, per Task 2.2.1c/d's
// pre-mortem correction: dedup happens per matched candidate, not once upfront). Any
// other error is logged and also reported via alreadyClaimed=false, ok=false so the
// caller doesn't proceed to fire on an unconfirmed claim.
func claimTriggerFireEvent(ctx context.Context, fireEvents session.TriggerFireEventRepository, wf *ent.Workflow, deliveryID string) (ok, alreadyClaimed bool) {
	wfID := wf.ID
	claimErr := fireEvents.Create(ctx, session.TriggerFireEventInput{
		WorkflowID: &wfID,
		DeliveryID: deliveryID,
		Outcome:    "pending",
	})
	if claimErr == nil {
		return true, false
	}
	if errors.Is(claimErr, session.ErrDuplicateDelivery) {
		return false, true
	}
	log.Warn("[WebhookReceiver] failed to claim trigger fire event", "slug", wf.Slug, "delivery_id", deliveryID, "err", claimErr)
	return false, false
}

// renderAndFireTrigger renders wf.PromptTemplate against payload and fires it, updating
// the already-claimed (workflow_id, delivery_id) TriggerFireEvent row (see
// claimTriggerFireEvent) to its final outcome. Callers must have already claimed the
// row as "pending" before calling this.
//
// TODO(Phase 3): replace the scheduler.FireNow call below with
// scheduler.FireTrigger(ctx, wf, renderedPrompt, deliveryID) once Task 3.2.1b lands.
// FireNow is Phase 1's cron/manual-fire entry point — it prepends wf.Command (and
// appends wf.InputTemplate, if set) ahead of the arg it's given, so today the fired
// session's initial prompt is "wf.Command\n\n<rendered PromptTemplate>", not the
// rendered template alone. That's an acceptable, clearly-marked stub for this phase:
// FireNow is still the only method that wires the admission gate + rate limiter, so
// reusing it (rather than duplicating that logic here) is the safer temporary choice.
func renderAndFireTrigger(ctx context.Context, fireEvents session.TriggerFireEventRepository, scheduler *workflows.Scheduler, wf *ent.Workflow, deliveryID string, payload map[string]interface{}) {
	wfID := wf.ID

	renderedPrompt, err := renderTriggerPromptStub(wf.PromptTemplate, payload)
	if err != nil {
		log.Warn("[WebhookReceiver] failed to render prompt template", "slug", wf.Slug, "err", err)
		if updErr := fireEvents.UpdateOutcome(ctx, wfID, deliveryID, "fired_failed", "", err.Error()); updErr != nil {
			log.Warn("[WebhookReceiver] failed to update trigger fire event outcome", "slug", wf.Slug, "err", updErr)
		}
		return
	}

	if scheduler == nil {
		log.Warn("[WebhookReceiver] no scheduler wired, cannot fire", "slug", wf.Slug)
		if updErr := fireEvents.UpdateOutcome(ctx, wfID, deliveryID, "fired_failed", "", "scheduler unavailable"); updErr != nil {
			log.Warn("[WebhookReceiver] failed to update trigger fire event outcome", "slug", wf.Slug, "err", updErr)
		}
		return
	}

	sessionID, fireErr := scheduler.FireNow(ctx, wf, renderedPrompt)
	if fireErr != nil {
		if updErr := fireEvents.UpdateOutcome(ctx, wfID, deliveryID, "fired_failed", "", fireErr.Error()); updErr != nil {
			log.Warn("[WebhookReceiver] failed to update trigger fire event outcome", "slug", wf.Slug, "err", updErr)
		}
		return
	}

	if updErr := fireEvents.UpdateOutcome(ctx, wfID, deliveryID, "fired_success", sessionID, ""); updErr != nil {
		log.Warn("[WebhookReceiver] failed to update trigger fire event outcome", "slug", wf.Slug, "err", updErr)
	}
}

// claimAndFireTrigger composes claimTriggerFireEvent + renderAndFireTrigger — used by
// GitHubWebhookHandler, where matching is fully resolved before dedup claiming (repo +
// branch + signature all checked per candidate first).
func claimAndFireTrigger(ctx context.Context, fireEvents session.TriggerFireEventRepository, scheduler *workflows.Scheduler, wf *ent.Workflow, deliveryID string, payload map[string]interface{}) {
	ok, _ := claimTriggerFireEvent(ctx, fireEvents, wf, deliveryID)
	if !ok {
		return
	}
	renderAndFireTrigger(ctx, fireEvents, scheduler, wf, deliveryID, payload)
}

// uuidPtr returns a pointer to v — used at call sites persisting a TriggerFireEvent
// with a known Workflow ID.
func uuidPtr(v uuid.UUID) *uuid.UUID {
	return &v
}
