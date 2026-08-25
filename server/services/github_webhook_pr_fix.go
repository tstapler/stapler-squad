package services

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// GitHub's documented action/conclusion/state enum values relevant to deciding
// whether a check_run/workflow_run/pull_request_review/issue_comment delivery is
// worth reacting to. An unrecognized value simply falls through to "not
// actionable" rather than erroring (see plan.md's Unresolved Questions).
const (
	ghActionCompleted = "completed"
	ghActionSubmitted = "submitted"
	ghActionCreated   = "created"

	ghConclusionFailure        = "failure"
	ghConclusionTimedOut       = "timed_out"
	ghConclusionCancelled      = "cancelled"
	ghConclusionActionRequired = "action_required"

	ghReviewStateChangesRequested = "changes_requested"
	ghReviewStateCommented        = "commented"
)

// failureShapedConclusions mirrors CIFailing's own terminal-failure semantics
// (session/git/worktree_git.go) — these are the check_run/workflow_run conclusions
// worth immediately reconciling for, as opposed to "success"/"neutral"/"skipped"/
// "stale" which never warrant a fix-loop trigger.
var failureShapedConclusions = map[string]bool{
	ghConclusionFailure:        true,
	ghConclusionTimedOut:       true,
	ghConclusionCancelled:      true,
	ghConclusionActionRequired: true,
}

// extractPRFixEvent dispatches to the extractor matching eventType. ok is false only
// when repository.full_name is missing/malformed — every other "not worth reacting
// to" case is reported via actionable=false, ok=true.
func extractPRFixEvent(eventType string, payload map[string]interface{}) (repoFullName string, prNumbers []int, actionable bool, ok bool) {
	switch eventType {
	case "check_run":
		return extractCheckOrWorkflowRunEvent(payload, "check_run")
	case "workflow_run":
		return extractCheckOrWorkflowRunEvent(payload, "workflow_run")
	case "pull_request_review":
		return extractPullRequestReviewEvent(payload)
	case "issue_comment":
		return extractIssueCommentEvent(payload)
	default:
		return "", nil, false, false
	}
}

// extractCheckOrWorkflowRunEvent handles both check_run and workflow_run — identical
// shape, differing only in the top-level payload key.
func extractCheckOrWorkflowRunEvent(payload map[string]interface{}, key string) (repoFullName string, prNumbers []int, actionable bool, ok bool) {
	repoFullName, ok = payloadRepoFullName(payload)
	if !ok {
		return "", nil, false, false
	}
	action, _ := payload["action"].(string)
	run, _ := payload[key].(map[string]interface{})
	if run == nil {
		return repoFullName, nil, false, true
	}
	conclusion, _ := run["conclusion"].(string)
	prNumbers = payloadPullRequestNumbers(run)
	actionable = action == ghActionCompleted && failureShapedConclusions[conclusion]
	return repoFullName, prNumbers, actionable, true
}

// extractPullRequestReviewEvent extracts a pull_request_review delivery's PR number
// and actionability. actorLogin is read separately via extractActorLogin for the
// self-actor filter (Story 2.2.1).
func extractPullRequestReviewEvent(payload map[string]interface{}) (repoFullName string, prNumbers []int, actionable bool, ok bool) {
	repoFullName, ok = payloadRepoFullName(payload)
	if !ok {
		return "", nil, false, false
	}
	action, _ := payload["action"].(string)
	review, _ := payload["review"].(map[string]interface{})
	pr, _ := payload["pull_request"].(map[string]interface{})
	if pr == nil {
		return repoFullName, nil, false, true
	}
	if number, isFloat := pr["number"].(float64); isFloat {
		prNumbers = []int{int(number)}
	}
	state, _ := review["state"].(string)
	actionable = action == ghActionSubmitted && (state == ghReviewStateChangesRequested || state == ghReviewStateCommented)
	return repoFullName, prNumbers, actionable, true
}

// extractIssueCommentEvent extracts an issue_comment delivery's PR number and
// actionability. issue_comment fires for both plain-issue and PR conversation-tab
// comments — only the latter (issue.pull_request present) is actionable.
func extractIssueCommentEvent(payload map[string]interface{}) (repoFullName string, prNumbers []int, actionable bool, ok bool) {
	repoFullName, ok = payloadRepoFullName(payload)
	if !ok {
		return "", nil, false, false
	}
	action, _ := payload["action"].(string)
	issue, _ := payload["issue"].(map[string]interface{})
	if issue == nil {
		return repoFullName, nil, false, true
	}
	_, isPR := issue["pull_request"]
	if number, isFloat := issue["number"].(float64); isFloat && isPR {
		prNumbers = []int{int(number)}
	}
	actionable = action == ghActionCreated && isPR && len(prNumbers) > 0
	return repoFullName, prNumbers, actionable, true
}

// extractActorLogin reads the login of the account that authored an issue_comment or
// pull_request_review event, for the self-actor filter (ADR-001). Returns "" for
// event types with no relevant actor field.
func extractActorLogin(eventType string, payload map[string]interface{}) string {
	switch eventType {
	case "issue_comment":
		comment, _ := payload["comment"].(map[string]interface{})
		user, _ := comment["user"].(map[string]interface{})
		login, _ := user["login"].(string)
		return login
	case "pull_request_review":
		review, _ := payload["review"].(map[string]interface{})
		user, _ := review["user"].(map[string]interface{})
		login, _ := user["login"].(string)
		return login
	default:
		return ""
	}
}

// payloadRepoFullName pulls repository.full_name out of any GitHub webhook payload.
// Mirrors extractGitHubRepoAndBranch's degrade-gracefully ok-bool contract.
func payloadRepoFullName(payload map[string]interface{}) (string, bool) {
	repoObj, _ := payload["repository"].(map[string]interface{})
	fullName, _ := repoObj["full_name"].(string)
	if fullName == "" {
		return "", false
	}
	return fullName, true
}

// payloadPullRequestNumbers reads a check_run/workflow_run object's pull_requests
// array of PR numbers. GitHub documents this as empty for a fork PR (the check run
// can't be associated with a PR from a different repo) — an empty result is not an
// error, just nothing to match against.
func payloadPullRequestNumbers(run map[string]interface{}) []int {
	prs, _ := run["pull_requests"].([]interface{})
	numbers := make([]int, 0, len(prs))
	for _, pr := range prs {
		prObj, _ := pr.(map[string]interface{})
		if number, isFloat := prObj["number"].(float64); isFloat {
			numbers = append(numbers, int(number))
		}
	}
	return numbers
}

// selfLoginCacheTTL bounds how long a resolved (or failed) self-login lookup is
// reused before GetCurrentUserLogin is called again.
const selfLoginCacheTTL = 5 * time.Minute

// selfLoginCache TTL-caches this instance's own GitHub login (via
// github.GetCurrentUserLogin) for ADR-001's self-actor filter — avoids one GitHub API
// call per issue_comment/pull_request_review delivery.
type selfLoginCache struct {
	mu        sync.RWMutex
	login     string
	fetchedAt time.Time
}

func newSelfLoginCache() *selfLoginCache {
	return &selfLoginCache{}
}

// Get returns the cached login, refreshing it if stale. On a lookup error or an
// unauthenticated ("", nil) result, it caches "" and logs a Warn once per refresh
// (not once per event) — callers must treat "" as "cannot determine, don't suppress"
// per ADR-001's fail-open contract.
func (c *selfLoginCache) Get(ctx context.Context) string {
	c.mu.RLock()
	fresh := time.Since(c.fetchedAt) < selfLoginCacheTTL
	login := c.login
	c.mu.RUnlock()
	if fresh {
		return login
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Re-check under the write lock in case another goroutine refreshed first.
	if time.Since(c.fetchedAt) < selfLoginCacheTTL {
		return c.login
	}
	login, err := github.GetCurrentUserLogin(ctx)
	if err != nil || login == "" {
		log.Warn("[GitHubWebhookHandler] could not resolve this instance's own GitHub login for the self-actor filter; PR-fix webhook events will not be self-filtered until this succeeds", "err", err)
		login = ""
	}
	c.login = login
	c.fetchedAt = time.Now()
	return c.login
}

// handlePRFixEvent is the dispatch target (Task 2.1.1a) for check_run/workflow_run/
// pull_request_review/issue_comment deliveries. It extracts the event, applies the
// pr_event_webhooks flag gate and the actionability/self-actor pre-filters, verifies
// the signature (only once actionable — an unsigned non-actionable request costs
// nothing beyond a map lookup), and routes a match to h.prFixRouter, persisting a
// TriggerFireEvent outcome for every branch.
func (h *GitHubWebhookHandler) handlePRFixEvent(w http.ResponseWriter, r *http.Request, payload map[string]interface{}, body []byte, deliveryID, eventType string) {
	ctx := r.Context()
	if h.cfg == nil || !h.cfg.GetFeatureFlag("pr_event_webhooks") {
		// True no-op — not even a "no_match" row, per Story 2.1.3.
		w.WriteHeader(http.StatusOK)
		return
	}

	fullName, prNumbers, actionable, ok := extractPRFixEvent(eventType, payload)
	if !ok {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "missing repository.full_name",
		})
		http.Error(w, "missing repository.full_name", http.StatusBadRequest)
		return
	}

	if actionable && (eventType == "issue_comment" || eventType == "pull_request_review") {
		if actorLogin := extractActorLogin(eventType, payload); actorLogin != "" && strings.EqualFold(actorLogin, h.selfLogin.Get(ctx)) {
			actionable = false
		}
	}

	if !actionable || len(prNumbers) == 0 {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "no_match", DeliveryID: deliveryID})
		w.WriteHeader(http.StatusOK)
		return
	}

	if !h.verifySignatureForRepo(ctx, fullName, body, r.Header.Get("X-Hub-Signature-256")) {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "invalid signature",
		})
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	if once, ok := h.firstPRFixDelivery[eventType]; ok {
		once.Do(func() {
			log.Info("[GitHubWebhookHandler] first verified delivery received — /webhooks/github reachability confirmed", "event_type", eventType)
		})
	}

	if h.prFixRouter == nil {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "fired_failed", DeliveryID: deliveryID, ErrorMessage: "no PRFixEventRouter configured",
		})
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, prNumber := range prNumbers {
		matched, err := h.prFixRouter.TriggerPRFixForEvent(ctx, fullName, prNumber)
		outcome := "no_match"
		errMsg := ""
		switch {
		case err != nil:
			outcome, errMsg = "fired_failed", err.Error()
		case matched:
			outcome = "fired_success"
		}
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: outcome, DeliveryID: deliveryID, ErrorMessage: errMsg})
	}

	w.WriteHeader(http.StatusOK)
}
