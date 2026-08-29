package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// prFixEventTypes is the single source of truth for the 4 GitHub webhook event types
// this feature reacts to — used to build both Handle's dispatch switch guard and
// GitHubWebhookHandler's firstPRFixDelivery map, so adding a 5th event type later only
// means updating this slice (and extractPRFixEvent's dispatch) instead of silently
// missing one of several previously hand-duplicated lists.
var prFixEventTypes = []string{"check_run", "workflow_run", "pull_request_review", "issue_comment"}

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

	// ciBudgetExceededMarker is the shared prefix a CI job's soft-timeout wrapper step
	// emits (e.g. "::error::CI-BUDGET-EXCEEDED: test exceeded its 44m soft budget",
	// Epic 4.2) when it hits its own duration budget rather than failing on broken
	// code. Kept in sync with each wrapped workflow step's own comment pointing back
	// here (see plan.md Task 4.2.1b-i).
	ciBudgetExceededMarker = "CI-BUDGET-EXCEEDED:"
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
	case "check_run", "workflow_run":
		return extractCheckOrWorkflowRunEvent(payload, eventType)
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

// payloadRunID reads a check_run/workflow_run object's own numeric "id" field — needed
// by the budget-marker checks below (Task 4.2.1d/h) to look up the run's check-run
// annotations. extractCheckOrWorkflowRunEvent deliberately doesn't return this (it
// stays a pure, payload-only extractor unchanged by this story — see
// TestExtractCheckRunEvent_should_HandleAllActionabilityCases), so this is a second,
// narrower reader over the same payload["check_run"/"workflow_run"] object.
func payloadRunID(payload map[string]interface{}, key string) (int64, bool) {
	run, _ := payload[key].(map[string]interface{})
	id, isFloat := run["id"].(float64)
	if !isFloat {
		return 0, false
	}
	return int64(id), true
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

// --- CI-budget-exceeded marker detection (Epic 4.2 Story 4.2.1) ------------
//
// Distinguishes "this job exceeded its soft CI-duration budget" (research/ux.md §3)
// from "this job's code is broken" so the auto-fix webhook doesn't spuriously trigger
// on a slow-but-not-broken PR. A wrapped CI step emits a normal `::error::` workflow
// command carrying ciBudgetExceededMarker as a prefix — GitHub attaches that to the
// step's own check run as an annotation automatically, no extra API call needed on
// the workflow-YAML side (see plan.md's Epic 4.2 "Why check-run annotations" note).

// checkRunAnnotation is the subset of a GitHub check run annotation's fields (GET
// /repos/{owner}/{repo}/check-runs/{id}/annotations) that budget-marker detection needs.
type checkRunAnnotation struct {
	Message string `json:"message"`
}

// checkRunAnnotationsFetcher fetches a check run's annotations. fetchCheckRunAnnotations
// (below) is the production implementation; tests substitute a fake — this file has no
// existing fake HTTP client to reuse for this endpoint (Task 4.2.1i-a).
type checkRunAnnotationsFetcher func(ctx context.Context, fullName string, checkRunID int64) ([]checkRunAnnotation, error)

// checkRunHasBudgetMarker reports whether any of checkRunID's annotations carries the
// ciBudgetExceededMarker prefix (Task 4.2.1c). On a fetch error, it returns (false,
// err) — the CALLER decides how to fail open (Task 4.2.1d: log.Warn and proceed as if
// no marker were found), matching this file's existing fail-open convention (e.g.
// selfLoginCache.Get's doc comment) rather than swallowing the error here.
func checkRunHasBudgetMarker(ctx context.Context, fullName string, checkRunID int64, fetch checkRunAnnotationsFetcher) (bool, error) {
	annotations, err := fetch(ctx, fullName, checkRunID)
	if err != nil {
		return false, err
	}
	for _, a := range annotations {
		if strings.HasPrefix(a.Message, ciBudgetExceededMarker) {
			return true, nil
		}
	}
	return false, nil
}

// workflowRunJob is the subset of a workflow run job's fields (GET
// /repos/{owner}/{repo}/actions/runs/{id}/jobs) that budget-marker detection needs.
type workflowRunJob struct {
	conclusion string
	checkRunID int64
}

// workflowRunJobsFetcher lists a workflow run's jobs. fetchWorkflowRunJobs (below) is
// the production implementation; tests substitute a fake (Task 4.2.1i-b).
type workflowRunJobsFetcher func(ctx context.Context, fullName string, workflowRunID int64) ([]workflowRunJob, error)

// workflowRunHasOnlyBudgetFailures reports whether EVERY job in workflowRunID whose
// conclusion is failure-shaped (failureShapedConclusions) carries the
// ciBudgetExceededMarker on its own check run's annotations (Task 4.2.1g). A mixed run
// — some jobs budget-exceeded, others genuinely broken — returns false, so it stays
// actionable (a partial budget failure is never silently swallowed). On any fetch
// error (the jobs list, or a per-job annotations lookup), returns (false, err); the
// caller fails open the same way checkRunHasBudgetMarker's callers do.
func workflowRunHasOnlyBudgetFailures(ctx context.Context, fullName string, workflowRunID int64, listJobs workflowRunJobsFetcher, fetchAnnotations checkRunAnnotationsFetcher) (bool, error) {
	jobs, err := listJobs(ctx, fullName, workflowRunID)
	if err != nil {
		return false, err
	}
	sawFailure := false
	for _, job := range jobs {
		if !failureShapedConclusions[job.conclusion] {
			continue
		}
		sawFailure = true
		hasMarker, markerErr := checkRunHasBudgetMarker(ctx, fullName, job.checkRunID, fetchAnnotations)
		if markerErr != nil {
			return false, markerErr
		}
		if !hasMarker {
			return false, nil
		}
	}
	return sawFailure, nil
}

// fetchCheckRunAnnotations is the production checkRunAnnotationsFetcher: calls the
// GitHub Checks API's GET /repos/{owner}/{repo}/check-runs/{id}/annotations directly
// via net/http and this file's own token resolution (ciBudgetGHGet/ciBudgetGHToken
// below). This story's scope is restricted to this file rather than adding a new
// exported function to the github package (see plan.md's Files list for Story 4.2.1).
func fetchCheckRunAnnotations(ctx context.Context, fullName string, checkRunID int64) ([]checkRunAnnotation, error) {
	repo, ok := splitRepoFullName(fullName)
	if !ok {
		return nil, fmt.Errorf("malformed repository full_name %q", fullName)
	}
	apiPath := fmt.Sprintf("repos/%s/%s/check-runs/%d/annotations", url.PathEscape(repo.Owner()), url.PathEscape(repo.Repo()), checkRunID)
	var annotations []checkRunAnnotation
	if err := ciBudgetGHGet(ctx, apiPath, &annotations); err != nil {
		return nil, err
	}
	return annotations, nil
}

// fetchWorkflowRunJobs is the production workflowRunJobsFetcher: calls GET
// /repos/{owner}/{repo}/actions/runs/{id}/jobs and derives each job's check-run id from
// its own check_run_url field rather than trusting the job's own "id". GitHub's own
// documented example response (docs.github.com/en/rest/actions/workflow-jobs) shows a
// job id of 21 whose check_run_url ends in ".../check-runs/4" — the two ids are NOT
// interchangeable (Task 4.2.1g's fallback), so job.id is never used for the
// per-job annotations lookup below.
func fetchWorkflowRunJobs(ctx context.Context, fullName string, workflowRunID int64) ([]workflowRunJob, error) {
	repo, ok := splitRepoFullName(fullName)
	if !ok {
		return nil, fmt.Errorf("malformed repository full_name %q", fullName)
	}
	apiPath := fmt.Sprintf("repos/%s/%s/actions/runs/%d/jobs", url.PathEscape(repo.Owner()), url.PathEscape(repo.Repo()), workflowRunID)
	var envelope struct {
		Jobs []struct {
			Conclusion  string `json:"conclusion"`
			CheckRunURL string `json:"check_run_url"`
		} `json:"jobs"`
	}
	if err := ciBudgetGHGet(ctx, apiPath, &envelope); err != nil {
		return nil, err
	}
	jobs := make([]workflowRunJob, 0, len(envelope.Jobs))
	for _, j := range envelope.Jobs {
		checkRunID, idOK := parseTrailingID(j.CheckRunURL)
		if !idOK {
			continue // malformed/missing check_run_url — nothing to correlate against
		}
		jobs = append(jobs, workflowRunJob{conclusion: j.Conclusion, checkRunID: checkRunID})
	}
	return jobs, nil
}

// ciBudgetGHGet issues an authenticated GET to path against the GitHub REST API
// (github.RestBaseURLForHost/github.HTTPClient — this file's existing GitHub API
// client, same test seam other github package callers use) and decodes a 200 response
// into out.
func ciBudgetGHGet(ctx context.Context, path string, out interface{}) error {
	token := ciBudgetGHToken()
	if token == "" {
		return errors.New("github token not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, github.RestBaseURLForHost("")+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := github.HTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github API %s: unexpected status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// ciBudgetGHToken resolves a GitHub token using the same precedence as the github
// package's unexported getGHToken (env GITHUB_TOKEN, then GH_TOKEN, then the OS
// keychain) — duplicated here rather than exported from that package, per this
// story's file-scope restriction.
func ciBudgetGHToken() string {
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		return tok
	}
	if tok := os.Getenv("GH_TOKEN"); tok != "" {
		return tok
	}
	return github.GetKeychainToken()
}

// splitRepoFullName splits a repository "owner/repo" full_name into a github.RepoRef —
// the codebase's existing newtype for an owner/repo pair (github/repo_ref.go), per
// .claude/rules/primitive-obsession-checklist.md, rather than two bare strings.
func splitRepoFullName(fullName string) (github.RepoRef, bool) {
	owner, repo, found := strings.Cut(fullName, "/")
	if !found || owner == "" || repo == "" {
		return github.RepoRef{}, false
	}
	ref, err := github.NewRepoRef(owner, repo)
	if err != nil {
		return github.RepoRef{}, false
	}
	return ref, true
}

// parseTrailingID extracts the trailing numeric path segment from a URL such as
// "https://api.github.com/repos/o/r/check-runs/4" -> 4.
func parseTrailingID(rawURL string) (int64, bool) {
	idx := strings.LastIndex(rawURL, "/")
	if idx == -1 || idx == len(rawURL)-1 {
		return 0, false
	}
	id, err := strconv.ParseInt(rawURL[idx+1:], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// handlePRFixEvent is the dispatch target for check_run/workflow_run/
// pull_request_review/issue_comment deliveries. Order is deliberate: actionability
// (cheap) before signature, and the self-actor filter's GitHub API call only runs
// once verified, never reachable from an unauthenticated request.
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

	if !actionable || len(prNumbers) == 0 {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "no_match", DeliveryID: deliveryID})
		w.WriteHeader(http.StatusOK)
		return
	}

	verified, sigErr := h.verifySignatureForRepo(ctx, fullName, body, r.Header.Get("X-Hub-Signature-256"))
	if sigErr != nil {
		log.Error("[GitHubWebhookHandler] failed to list github_push workflows", "err", sigErr)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !verified {
		persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{
			Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "invalid signature",
		})
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// CI-budget-exceeded marker filter (Task 4.2.1d/h): only reachable once the
	// delivery is verified, mirroring the self-actor filter's placement below — an
	// unauthenticated request can never trigger a GitHub API lookup. Skipped entirely
	// (never even resolves runID) for event types other than check_run/workflow_run —
	// a missing "id" in the payload behaves like "no marker found" (proceeds to fire).
	if eventType == "check_run" || eventType == "workflow_run" {
		var hasOnlyBudgetFailures bool
		var budgetErr error
		if runID, idOK := payloadRunID(payload, eventType); idOK {
			if eventType == "check_run" {
				hasOnlyBudgetFailures, budgetErr = checkRunHasBudgetMarker(ctx, fullName, runID, fetchCheckRunAnnotations)
			} else {
				hasOnlyBudgetFailures, budgetErr = workflowRunHasOnlyBudgetFailures(ctx, fullName, runID, fetchWorkflowRunJobs, fetchCheckRunAnnotations)
			}
		}
		if budgetErr != nil {
			log.Warn("[GitHubWebhookHandler] failed to check CI-budget-exceeded marker; failing open (treating as a genuine failure)", "event_type", eventType, "err", budgetErr)
		} else if hasOnlyBudgetFailures {
			persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "no_match", DeliveryID: deliveryID})
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Self-actor filter (ADR-001): only reachable once the delivery is verified, so an
	// unauthenticated request can never force selfLogin's GitHub API call.
	if eventType == "issue_comment" || eventType == "pull_request_review" {
		if actorLogin := extractActorLogin(eventType, payload); actorLogin != "" && strings.EqualFold(actorLogin, h.selfLogin.Get(ctx)) {
			persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "no_match", DeliveryID: deliveryID})
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	if once, ok := h.firstPRFixDelivery[eventType]; ok {
		once.Do(func() {
			log.Info(fmt.Sprintf("[GitHubWebhookHandler] first verified %s delivery received — /webhooks/github reachability confirmed", eventType))
		})
	}

	// Delivery-level dedup (AC0) — see ExistsByDeliveryID's doc comment for why this
	// can't reuse claimTriggerFireEvent's unique-index claim. A lookup failure fails
	// open rather than blocking a legitimate delivery.
	if deliveryID != "" && h.fireEvents != nil {
		duplicate, dupErr := h.fireEvents.ExistsByDeliveryID(ctx, deliveryID)
		if dupErr != nil {
			log.Warn("[GitHubWebhookHandler] delivery-dedup check failed, proceeding without dedup", "delivery_id", deliveryID, "err", dupErr)
		} else if duplicate {
			log.Info("[GitHubWebhookHandler] duplicate delivery, skipping reprocessing", "delivery_id", deliveryID, "event_type", eventType)
			w.WriteHeader(http.StatusOK)
			return
		}
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
