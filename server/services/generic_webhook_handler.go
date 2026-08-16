package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
)

// GenericWebhookHandler handles POST /webhooks/{slug}: generic `webhook`-type Workflow
// triggers, matched by event/label_filter, rendered against arbitrary JSON. Concrete
// type, not an interface — one implementation, per
// .claude/rules/interface-pollution-checklist.md.
type GenericWebhookHandler struct {
	repo       session.WorkflowRepository
	scheduler  *workflows.Scheduler
	fireEvents session.TriggerFireEventRepository
	cfg        *config.Config
}

// NewGenericWebhookHandler constructs a GenericWebhookHandler.
func NewGenericWebhookHandler(repo session.WorkflowRepository, scheduler *workflows.Scheduler, fireEvents session.TriggerFireEventRepository, cfg *config.Config) *GenericWebhookHandler {
	return &GenericWebhookHandler{repo: repo, scheduler: scheduler, fireEvents: fireEvents, cfg: cfg}
}

// RegisterRoutes registers the generic webhook endpoint on mux.
func (h *GenericWebhookHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/{slug}", h.Handle)
}

// Handle processes an inbound generic webhook delivery: resolves the Workflow by slug,
// verifies HMAC signature, dedups by a SHA-256 digest of the raw body (generic webhooks
// have no provider-assigned delivery ID), matches event/label_filter, then renders and
// fires.
func (h *GenericWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if h.cfg == nil || !h.cfg.GetFeatureFlag("webhook_triggers") {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	slug := r.PathValue("slug")

	wf, err := h.repo.GetByWebhookSlug(ctx, slug)
	if err != nil {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", ErrorMessage: "unknown slug",
		})
		http.NotFound(w, r)
		return
	}

	// Defense in depth (mirrors Scheduler.Reload's mismatched-trigger guard): a row
	// whose trigger_type isn't "webhook", or that's disabled, must never fire. Reported
	// identically to "unknown slug" (404) rather than a distinguishable status, so an
	// unauthenticated prober can't use the response to enumerate which slugs exist but
	// are merely disabled/misconfigured.
	if wf.TriggerType != "webhook" || !wf.CronEnabled {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			WorkflowID: uuidPtr(wf.ID), Outcome: "no_match",
		})
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxWebhookBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			WorkflowID: uuidPtr(wf.ID), Outcome: "rejected", ErrorMessage: "failed to read request body",
		})
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			WorkflowID: uuidPtr(wf.ID), Outcome: "rejected", ErrorMessage: "malformed JSON",
		})
		http.Error(w, "malformed JSON", http.StatusBadRequest)
		return
	}

	secret, err := decryptWorkflowSecret(h.cfg, wf)
	if err != nil {
		log.Warn("[GenericWebhookHandler] failed to decrypt webhook secret", "slug", wf.Slug, "err", err)
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			WorkflowID: uuidPtr(wf.ID), Outcome: "rejected", ErrorMessage: "invalid signature",
		})
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	sigHeader := r.Header.Get("X-Webhook-Signature")
	if !VerifyWebhookSecret(secret, body, sigHeader) {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			WorkflowID: uuidPtr(wf.ID), Outcome: "rejected", ErrorMessage: "invalid signature",
		})
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Generic webhooks have no provider-assigned delivery ID (unlike GitHub's
	// X-GitHub-Delivery) — use a SHA-256 digest of the raw body instead (Task 2.3.1b).
	// The (workflow_id, delivery_id) composite unique index already scopes this per
	// wf.ID (resolved from {slug} above), so identical payloads delivered to two
	// different slugs don't collide without needing to fold the slug into the digest.
	deliveryID := sha256Hex(body)

	ok, alreadyClaimed := claimTriggerFireEvent(ctx, h.fireEvents, wf, deliveryID)
	if alreadyClaimed {
		// AC12: replayed delivery — already processed, ack without re-firing.
		w.WriteHeader(http.StatusOK)
		return
	}
	if !ok {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !eventFilterMatches(wf.EventFilter, payload) || !labelFilterMatches(wf.LabelFilter, payload) {
		if updErr := h.fireEvents.UpdateOutcome(ctx, wf.ID, deliveryID, "no_match", "", ""); updErr != nil {
			log.Warn("[GenericWebhookHandler] failed to update trigger fire event outcome", "slug", wf.Slug, "err", updErr)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	renderAndFireTrigger(ctx, h.fireEvents, h.scheduler, wf, deliveryID, payload)
	w.WriteHeader(http.StatusOK)
}

// sha256Hex returns the lowercase-hex SHA-256 digest of body.
func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// eventFilterMatches reports whether payload["event"] equals filter. An empty filter
// (unset on the Workflow) matches any event.
func eventFilterMatches(filter string, payload map[string]interface{}) bool {
	if filter == "" {
		return true
	}
	event, _ := payload["event"].(string)
	return event == filter
}

// labelFilterMatches reports whether filter is present in payload["labels"] (a JSON
// array of strings). An empty filter matches regardless of labels. A non-empty filter
// with no "labels" field present in the payload never matches (Task 2.3.1c).
func labelFilterMatches(filter string, payload map[string]interface{}) bool {
	if filter == "" {
		return true
	}
	raw, ok := payload["labels"]
	if !ok {
		return false
	}
	labels, ok := raw.([]interface{})
	if !ok {
		return false
	}
	for _, l := range labels {
		if s, ok := l.(string); ok && s == filter {
			return true
		}
	}
	return false
}
