package services

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// GitHubWebhookHandler handles POST /webhooks/github: GitHub push-event deliveries
// matched against enabled github_push-type Workflow rows. Concrete type, not an
// interface — one implementation, per .claude/rules/interface-pollution-checklist.md.
type GitHubWebhookHandler struct {
	repo       session.WorkflowRepository
	scheduler  *workflows.Scheduler
	dispatcher *TriggerFireDispatcher
	fireEvents session.TriggerFireEventRepository
	cfg        *config.Config
}

// NewGitHubWebhookHandler constructs a GitHubWebhookHandler. dispatcher is typically
// shared with GenericWebhookHandler so one semaphore bounds trigger-fire fan-out
// across every inbound webhook route (see TriggerFireDispatcher's doc comment).
func NewGitHubWebhookHandler(repo session.WorkflowRepository, scheduler *workflows.Scheduler, dispatcher *TriggerFireDispatcher, fireEvents session.TriggerFireEventRepository, cfg *config.Config) *GitHubWebhookHandler {
	return &GitHubWebhookHandler{repo: repo, scheduler: scheduler, dispatcher: dispatcher, fireEvents: fireEvents, cfg: cfg}
}

// RegisterRoutes registers the GitHub webhook endpoint on mux.
func (h *GitHubWebhookHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/github", h.Handle)
}

// Handle processes an inbound GitHub webhook delivery: verifies HMAC signature per
// matching-repo candidate, matches repo/branch across all enabled github_push-type
// Workflow rows, dedups and fires each fresh match.
func (h *GitHubWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Feature-flag gate as the handler's first line (defense in depth alongside the
	// route-registration-time gate — see server.go).
	if h.cfg == nil || !h.cfg.GetFeatureFlag("webhook_triggers") {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	sigHeader := r.Header.Get("X-Hub-Signature-256")

	r.Body = http.MaxBytesReader(w, r.Body, MaxWebhookBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "failed to read request body",
		})
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "malformed JSON",
		})
		http.Error(w, "malformed JSON", http.StatusBadRequest)
		return
	}

	fullName, branch, ok := extractGitHubRepoAndBranch(payload)
	if !ok {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "missing repository.full_name or ref",
		})
		http.Error(w, "missing repository.full_name or ref", http.StatusBadRequest)
		return
	}

	candidates, err := h.repo.ListByTriggerType(ctx, "github_push")
	if err != nil {
		log.Error("[GitHubWebhookHandler] failed to list github_push workflows", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Narrow to enabled workflows watching this repo BEFORE decrypting any secret
	// (Task 2.2.1b — avoid a linear decrypt-and-check over every workflow in the
	// system).
	var repoCandidates []*ent.Workflow
	for _, wf := range candidates {
		if wf.CronEnabled && wf.GithubRepo == fullName {
			repoCandidates = append(repoCandidates, wf)
		}
	}

	if len(repoCandidates) == 0 {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "no_match", DeliveryID: deliveryID})
		w.WriteHeader(http.StatusOK)
		return
	}

	// Verify signature per candidate (each Workflow has its own secret) against the RAW
	// body — never re-marshaled JSON (pitfalls §1.1). A candidate whose signature
	// verifies AND whose branch matches is eligible to fire. If NO candidate's secret
	// verifies the signature, the request is unauthenticated for every repo-matching
	// workflow we know about and is rejected outright (AC1's "invalid signature"
	// branch) — firing is never gated on a DIFFERENT workflow's secret than the one
	// being fired.
	var matched []*ent.Workflow
	signatureVerifiedAny := false
	for _, wf := range repoCandidates {
		secret, err := decryptWorkflowSecret(h.cfg, wf)
		if err != nil {
			log.Warn("[GitHubWebhookHandler] failed to decrypt webhook secret", "slug", wf.Slug, "err", err)
			continue
		}
		if !VerifyGitHubSignature(secret, body, sigHeader) {
			continue
		}
		signatureVerifiedAny = true
		if wf.GithubBranch != branch {
			continue
		}
		matched = append(matched, wf)
	}

	if !signatureVerifiedAny {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "invalid signature",
		})
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	if len(matched) == 0 {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "no_match", DeliveryID: deliveryID})
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, wf := range matched {
		claimAndFireTrigger(ctx, h.dispatcher, h.fireEvents, h.scheduler, wf, deliveryID, payload)
	}

	w.WriteHeader(http.StatusOK)
}

// extractGitHubRepoAndBranch pulls repository.full_name and the branch name (ref with
// "refs/heads/" stripped) out of a GitHub push-event payload. ok is false when either
// field is missing or the wrong type.
func extractGitHubRepoAndBranch(payload map[string]interface{}) (fullName, branch string, ok bool) {
	repoObj, _ := payload["repository"].(map[string]interface{})
	if repoObj == nil {
		return "", "", false
	}
	fullName, _ = repoObj["full_name"].(string)
	if fullName == "" {
		return "", "", false
	}
	ref, _ := payload["ref"].(string)
	if ref == "" {
		return "", "", false
	}
	branch = strings.TrimPrefix(ref, "refs/heads/")
	return fullName, branch, true
}
