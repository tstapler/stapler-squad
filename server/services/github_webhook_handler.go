package services

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// PRFixEventRouter looks up and immediately reconciles a tracked pr_pending item for
// a GitHub PR event. Satisfied by *session.BacklogLifecycleListener (its
// TriggerPRFixForEvent method) — defined here, the consumer, per
// .claude/rules/interface-pollution-checklist.md.
type PRFixEventRouter interface {
	TriggerPRFixForEvent(ctx context.Context, repoFullName string, prNumber int) (matched bool, err error)
}

// GitHubWebhookHandler handles POST /webhooks/github: GitHub push-event deliveries
// matched against enabled github_push-type Workflow rows, plus (Story 2.1.1)
// check_run/workflow_run/pull_request_review/issue_comment deliveries routed to
// handlePRFixEvent. Concrete type, not an interface — one implementation, per
// .claude/rules/interface-pollution-checklist.md.
type GitHubWebhookHandler struct {
	repo        session.WorkflowRepository
	scheduler   *workflows.Scheduler
	fireEvents  session.TriggerFireEventRepository
	cfg         *config.Config
	prFixRouter PRFixEventRouter
	selfLogin   *selfLoginCache

	// firstPRFixDelivery logs once per event type on the first verified delivery of
	// that type (see github-webhook-public-reachability.md). Read-only after
	// construction, so safe for concurrent map reads.
	firstPRFixDelivery map[string]*sync.Once
}

// NewGitHubWebhookHandler constructs a GitHubWebhookHandler. prFixRouter may be nil
// (e.g. in tests that only exercise the push path, or if PR-fix wiring is
// unavailable) — handlePRFixEvent treats a nil router as a wiring gap and persists
// "fired_failed" rather than panicking.
func NewGitHubWebhookHandler(repo session.WorkflowRepository, scheduler *workflows.Scheduler, fireEvents session.TriggerFireEventRepository, cfg *config.Config, prFixRouter PRFixEventRouter) *GitHubWebhookHandler {
	firstPRFixDelivery := make(map[string]*sync.Once, len(prFixEventTypes))
	for _, eventType := range prFixEventTypes {
		firstPRFixDelivery[eventType] = &sync.Once{}
	}
	return &GitHubWebhookHandler{
		repo: repo, scheduler: scheduler, fireEvents: fireEvents, cfg: cfg, prFixRouter: prFixRouter,
		selfLogin:          newSelfLoginCache(),
		firstPRFixDelivery: firstPRFixDelivery,
	}
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

	payload, body, ok := readAndDecodeWebhookBody(w, r, h.fireEvents, deliveryID, nil)
	if !ok {
		return
	}

	// X-GitHub-Event branching (Story 2.1.1): "" is kept as a synonym for "push" since
	// the existing push-path tests don't set the header at all. Any of the 4 new
	// PR-fix event types dispatch to handlePRFixEvent (server/services/github_webhook_pr_fix.go);
	// anything else (e.g. GitHub's own "ping" delivery on webhook setup) needs no
	// signature verification or persistence and just succeeds trivially.
	switch eventType := r.Header.Get("X-GitHub-Event"); {
	case eventType == "" || eventType == "push":
		// existing logic below, unchanged.
	case slices.Contains(prFixEventTypes, eventType):
		h.handlePRFixEvent(w, r, payload, body, deliveryID, eventType)
		return
	default:
		w.WriteHeader(http.StatusOK)
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

	repoCandidates, err := h.repoCandidatesFor(ctx, fullName)
	if err != nil {
		log.Error("[GitHubWebhookHandler] failed to list github_push workflows", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
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
	verified := verifiedWorkflowCandidates(h.cfg, repoCandidates, body, sigHeader)
	if len(verified) == 0 {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "invalid signature",
		})
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var matched []*ent.Workflow
	for _, wf := range verified {
		if wf.GithubBranch == branch {
			matched = append(matched, wf)
		}
	}

	if len(matched) == 0 {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "no_match", DeliveryID: deliveryID})
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, wf := range matched {
		claimAndFireTrigger(ctx, h.fireEvents, h.scheduler, wf, deliveryID, payload)
	}

	w.WriteHeader(http.StatusOK)
}

// repoCandidatesFor lists the enabled github_push-type Workflow rows watching
// fullName's repo — this instance's only secret-storage source for a GitHub
// webhook. Shared by the push path (above) and the PR-fix event path
// (verifySignatureForRepo, github_webhook_pr_fix.go).
func (h *GitHubWebhookHandler) repoCandidatesFor(ctx context.Context, fullName string) ([]*ent.Workflow, error) {
	candidates, err := h.repo.ListByTriggerType(ctx, "github_push")
	if err != nil {
		return nil, err
	}
	// Narrow to enabled workflows watching this repo BEFORE decrypting any secret —
	// avoid a linear decrypt-and-check over every workflow in the system.
	var repoCandidates []*ent.Workflow
	for _, wf := range candidates {
		if wf.Enabled && wf.GithubRepo == fullName {
			repoCandidates = append(repoCandidates, wf)
		}
	}
	return repoCandidates, nil
}

// verifiedWorkflowCandidates returns the subset of candidates whose own decrypted
// secret verifies sigHeader against body. Shared by the push path and the PR-fix
// event path so both use the identical per-candidate signature-verification loop.
func verifiedWorkflowCandidates(cfg *config.Config, candidates []*ent.Workflow, body []byte, sigHeader string) []*ent.Workflow {
	var verified []*ent.Workflow
	for _, wf := range candidates {
		secret, err := decryptWorkflowSecret(cfg, wf)
		if err != nil {
			log.Warn("[GitHubWebhookHandler] failed to decrypt webhook secret", "slug", wf.Slug, "err", err)
			continue
		}
		if VerifyGitHubSignature(secret, body, sigHeader) {
			verified = append(verified, wf)
		}
	}
	return verified
}

// verifySignatureForRepo reports whether body/sigHeader verifies against ANY enabled
// github_push-type Workflow row for fullName's repo (used by handlePRFixEvent — GitHub
// signs every event type with the same secret). A non-nil error means the candidate
// lookup itself failed (500), distinct from "no candidate verified" (401).
func (h *GitHubWebhookHandler) verifySignatureForRepo(ctx context.Context, fullName string, body []byte, sigHeader string) (bool, error) {
	candidates, err := h.repoCandidatesFor(ctx, fullName)
	if err != nil {
		return false, err
	}
	return len(verifiedWorkflowCandidates(h.cfg, candidates, body, sigHeader)) > 0, nil
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
