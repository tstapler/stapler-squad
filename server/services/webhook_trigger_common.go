package services

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// renderAndFireTrigger renders wf.PromptTemplate against payload (via
// workflows.RenderTriggerPrompt — real inert-data-block framing + sanitize/truncate,
// per Task 3.1.1a) and fires it (via scheduler.FireTrigger, Task 3.2.1a), updating the
// already-claimed (workflow_id, delivery_id) TriggerFireEvent row (see
// claimTriggerFireEvent) to its final outcome. Callers must have already claimed the
// row as "pending" before calling this.
func renderAndFireTrigger(ctx context.Context, fireEvents session.TriggerFireEventRepository, scheduler *workflows.Scheduler, wf *ent.Workflow, deliveryID string, payload map[string]interface{}) {
	wfID := wf.ID

	renderedPrompt, err := workflows.RenderTriggerPrompt(wf.PromptTemplate, payload)
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

	sessionID, fireErr := scheduler.FireTrigger(ctx, wf, renderedPrompt, deliveryID)
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

// claimAndFireTrigger composes claimTriggerFireEvent (synchronous — must resolve
// before the handler responds, per AC12's dedup contract) + dispatcher.Dispatch
// (asynchronous — the actual render + CreateSession work). Used by
// GitHubWebhookHandler, where matching is fully resolved before dedup claiming (repo +
// branch + signature all checked per candidate first).
func claimAndFireTrigger(ctx context.Context, dispatcher *TriggerFireDispatcher, fireEvents session.TriggerFireEventRepository, scheduler *workflows.Scheduler, wf *ent.Workflow, deliveryID string, payload map[string]interface{}) {
	ok, _ := claimTriggerFireEvent(ctx, fireEvents, wf, deliveryID)
	if !ok {
		return
	}
	dispatcher.Dispatch(fireEvents, scheduler, wf, deliveryID, payload)
}

// maxInFlightTriggerFires bounds concurrent async webhook-trigger-fire goroutines
// (each driving scheduler.FireTrigger's CreateSession call) — same shape/rationale
// as session/chain_firer.go's maxInFlightChainFires and this package's
// maxInFlightCallbacks: an inbound webhook handler must respond promptly (GitHub's
// ~10s delivery timeout) rather than block on CreateSession's tmux+git-worktree
// provisioning cost, but the resulting goroutine fan-out still needs a cap.
const maxInFlightTriggerFires = 20

// triggerFireDispatchTimeout bounds a single async trigger-fire goroutine (template
// render + CreateSession) — matches session/chain_firer.go's chainFireTimeout, since
// both ultimately drive the same underlying CreateSession call.
const triggerFireDispatchTimeout = 5 * time.Minute

// TriggerFireDispatcher moves the actual trigger-fire step (template render +
// scheduler.FireTrigger's CreateSession call) off the inbound-webhook request
// goroutine, so GitHubWebhookHandler.Handle / GenericWebhookHandler.Handle can
// respond to the webhook sender before CreateSession's tmux+git-worktree
// provisioning completes. Shared by both handlers — same non-blocking,
// semaphore-bounded shape as session/chain_firer.go's ChainFirer and this
// package's CallbackDispatcher. Concrete type, not an interface — one
// implementation, per .claude/rules/interface-pollution-checklist.md.
type TriggerFireDispatcher struct {
	inFlight chan struct{}
}

// NewTriggerFireDispatcher constructs a TriggerFireDispatcher.
func NewTriggerFireDispatcher() *TriggerFireDispatcher {
	return &TriggerFireDispatcher{inFlight: make(chan struct{}, maxInFlightTriggerFires)}
}

// Dispatch reserves a semaphore slot and renders+fires wf's trigger in a new
// goroutine — same non-blocking shape as CallbackDispatcher.Dispatch /
// ChainFirer.Dispatch (a non-blocking select on a semaphore-sized channel either
// reserves a slot immediately or drops the dispatch). Never blocks the caller.
//
// Callers must already have claimed (workflow_id, delivery_id) as "pending" via
// claimTriggerFireEvent before calling Dispatch. A dropped dispatch (at capacity)
// still flips that already-claimed row to "fired_failed" rather than leaving it
// "pending" forever, so the drop is visible in the TriggerFireEvent audit trail
// instead of silently vanishing.
func (d *TriggerFireDispatcher) Dispatch(fireEvents session.TriggerFireEventRepository, scheduler *workflows.Scheduler, wf *ent.Workflow, deliveryID string, payload map[string]interface{}) {
	if d == nil {
		return
	}

	select {
	case d.inFlight <- struct{}{}:
	default:
		log.Warn("[TriggerFireDispatcher] dispatch dropped, at capacity", "slug", wf.Slug, "delivery_id", deliveryID)
		if updErr := fireEvents.UpdateOutcome(context.Background(), wf.ID, deliveryID, "fired_failed", "", "trigger-fire dispatcher at capacity"); updErr != nil {
			log.Warn("[TriggerFireDispatcher] failed to update trigger fire event outcome after drop", "slug", wf.Slug, "err", updErr)
		}
		return
	}

	go func() {
		defer func() { <-d.inFlight }()
		defer func() {
			if rec := recover(); rec != nil {
				log.Warn("[TriggerFireDispatcher] fire panicked (recovered)", "slug", wf.Slug, "delivery_id", deliveryID, "recovered", rec)
			}
		}()
		// A fresh context.Background(), not ctx from the request goroutine — by the
		// time this goroutine's scheduler.FireTrigger (CreateSession) call actually
		// runs, the inbound HTTP request's context may already be cancelled (response
		// already sent) or long gone, mirroring session/chain_firer.go's
		// ChainFirer.Dispatch and its dispatchChainFire doc-comment rationale one
		// layer up.
		fireCtx, cancel := context.WithTimeout(context.Background(), triggerFireDispatchTimeout)
		defer cancel()
		renderAndFireTrigger(fireCtx, fireEvents, scheduler, wf, deliveryID, payload)
	}()
}

// uuidPtr returns a pointer to v — used at call sites persisting a TriggerFireEvent
// with a known Workflow ID.
func uuidPtr(v uuid.UUID) *uuid.UUID {
	return &v
}
