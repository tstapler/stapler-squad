package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// --- extractPRFixEvent table-driven tests (Story 2.1.2) ---------------------

func TestExtractCheckRunEvent_should_HandleAllActionabilityCases(t *testing.T) {
	repoOnly := map[string]interface{}{"repository": map[string]interface{}{"full_name": "tstapler/stapler-squad"}}
	withCheckRun := func(action, conclusion string, prs ...int) map[string]interface{} {
		prList := make([]interface{}, 0, len(prs))
		for _, n := range prs {
			prList = append(prList, map[string]interface{}{"number": float64(n)})
		}
		return map[string]interface{}{
			"action": action,
			"check_run": map[string]interface{}{
				"conclusion":    conclusion,
				"pull_requests": prList,
			},
			"repository": repoOnly["repository"],
		}
	}

	tests := []struct {
		name           string
		payload        map[string]interface{}
		wantActionable bool
		wantPRNumbers  []int
		wantOK         bool
	}{
		{"completed+failure is actionable", withCheckRun("completed", "failure", 189), true, []int{189}, true},
		{"completed+success is not actionable", withCheckRun("completed", "success", 189), false, []int{189}, true},
		{"in_progress (no conclusion) is not actionable", withCheckRun("in_progress", "", 189), false, []int{189}, true},
		{"completed+cancelled IS actionable (CIFailing parity)", withCheckRun("completed", "cancelled", 189), true, []int{189}, true},
		{"completed+neutral is not actionable", withCheckRun("completed", "neutral", 189), false, []int{189}, true},
		{"completed+skipped is not actionable", withCheckRun("completed", "skipped", 189), false, []int{189}, true},
		{"completed+stale is not actionable", withCheckRun("completed", "stale", 189), false, []int{189}, true},
		{"no associated PRs (fork PR) — empty, not an error", withCheckRun("completed", "failure"), true, []int{}, true},
		{"missing repository.full_name -> ok=false", map[string]interface{}{"action": "completed", "check_run": map[string]interface{}{"conclusion": "failure"}}, false, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoFullName, prNumbers, actionable, ok := extractPRFixEvent("check_run", tt.payload)
			assert.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}
			assert.Equal(t, "tstapler/stapler-squad", repoFullName)
			assert.Equal(t, tt.wantActionable, actionable)
			assert.Equal(t, tt.wantPRNumbers, prNumbers)
		})
	}
}

func TestExtractWorkflowRunEvent_should_MirrorCheckRunShape(t *testing.T) {
	payload := map[string]interface{}{
		"action": "completed",
		"workflow_run": map[string]interface{}{
			"conclusion":    "failure",
			"pull_requests": []interface{}{map[string]interface{}{"number": float64(42)}},
		},
		"repository": map[string]interface{}{"full_name": "tstapler/stapler-squad"},
	}
	repoFullName, prNumbers, actionable, ok := extractPRFixEvent("workflow_run", payload)
	require.True(t, ok)
	assert.Equal(t, "tstapler/stapler-squad", repoFullName)
	assert.True(t, actionable)
	assert.Equal(t, []int{42}, prNumbers)
}

func TestExtractPullRequestReviewEvent_should_HandleAllStates(t *testing.T) {
	build := func(action, state string) map[string]interface{} {
		return map[string]interface{}{
			"action":       action,
			"review":       map[string]interface{}{"state": state, "user": map[string]interface{}{"login": "some-reviewer"}},
			"pull_request": map[string]interface{}{"number": float64(189)},
			"repository":   map[string]interface{}{"full_name": "tstapler/stapler-squad"},
		}
	}
	tests := []struct {
		name           string
		payload        map[string]interface{}
		wantActionable bool
	}{
		{"submitted+changes_requested is actionable", build("submitted", "changes_requested"), true},
		{"submitted+commented is actionable", build("submitted", "commented"), true},
		{"submitted+approved is not actionable", build("submitted", "approved"), false},
		{"dismissed is not actionable", build("dismissed", "changes_requested"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoFullName, prNumbers, actionable, ok := extractPRFixEvent("pull_request_review", tt.payload)
			require.True(t, ok)
			assert.Equal(t, "tstapler/stapler-squad", repoFullName)
			assert.Equal(t, tt.wantActionable, actionable)
			assert.Equal(t, []int{189}, prNumbers)
		})
	}
	assert.Equal(t, "some-reviewer", extractActorLogin("pull_request_review", build("submitted", "commented")))
}

func TestExtractIssueCommentEvent_should_HandleNonPRIssues(t *testing.T) {
	prComment := map[string]interface{}{
		"action":     "created",
		"issue":      map[string]interface{}{"number": float64(189), "pull_request": map[string]interface{}{"url": "..."}},
		"comment":    map[string]interface{}{"user": map[string]interface{}{"login": "some-human"}},
		"repository": map[string]interface{}{"full_name": "tstapler/stapler-squad"},
	}
	repoFullName, prNumbers, actionable, ok := extractPRFixEvent("issue_comment", prComment)
	require.True(t, ok)
	assert.Equal(t, "tstapler/stapler-squad", repoFullName)
	assert.True(t, actionable)
	assert.Equal(t, []int{189}, prNumbers)
	assert.Equal(t, "some-human", extractActorLogin("issue_comment", prComment))

	plainIssueComment := map[string]interface{}{
		"action":     "created",
		"issue":      map[string]interface{}{"number": float64(5)},
		"comment":    map[string]interface{}{"user": map[string]interface{}{"login": "some-human"}},
		"repository": map[string]interface{}{"full_name": "tstapler/stapler-squad"},
	}
	_, _, actionable, ok = extractPRFixEvent("issue_comment", plainIssueComment)
	require.True(t, ok)
	assert.False(t, actionable, "a plain-issue comment (no issue.pull_request) must not be actionable")
}

// --- selfLoginCache tests (Story 2.2.1, ADR-001) ----------------------------

func TestSelfLoginCache_should_SuppressActionable_When_CommentAuthorMatchesCachedSelfLogin(t *testing.T) {
	cache := newSelfLoginCache()
	cache.mu.Lock()
	cache.login = "stapler-squad-bot"
	cache.fetchedAt = time.Now()
	cache.mu.Unlock()

	assert.Equal(t, "stapler-squad-bot", cache.Get(context.Background()))
}

func TestSelfLoginCache_should_NotSuppressAnything_When_CacheEmpty(t *testing.T) {
	cache := newSelfLoginCache()
	cache.mu.Lock()
	cache.login = ""
	cache.fetchedAt = time.Now() // fresh cached empty result — fails open, no live lookup needed
	cache.mu.Unlock()

	got := cache.Get(context.Background())
	assert.Equal(t, "", got, "empty cached login must never equal a real actor login, so the self-filter never suppresses")
}

// --- handlePRFixEvent tests (Story 2.1.3, 2.3.2, 3.3.1) ---------------------

// fakePRFixEventRouter is a test double for PRFixEventRouter.
type fakePRFixEventRouter struct {
	mu    sync.Mutex
	calls []struct {
		repoFullName string
		prNumber     int
	}
	matched bool
	err     error
}

func (f *fakePRFixEventRouter) TriggerPRFixForEvent(_ context.Context, repoFullName string, prNumber int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		repoFullName string
		prNumber     int
	}{repoFullName, prNumber})
	return f.matched, f.err
}

func (f *fakePRFixEventRouter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func doPRFixEventRequest(t *testing.T, h *GitHubWebhookHandler, eventType string, body []byte, deliveryID, sigHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", eventType)
	if deliveryID != "" {
		req.Header.Set("X-GitHub-Delivery", deliveryID)
	}
	if sigHeader != "" {
		req.Header.Set("X-Hub-Signature-256", sigHeader)
	}
	rec := httptest.NewRecorder()
	h.Handle(rec, req)
	return rec
}

func checkRunFailureBody(t *testing.T) []byte {
	t.Helper()
	return jsonBody(t, map[string]interface{}{
		"action": "completed",
		"check_run": map[string]interface{}{
			"conclusion":    "failure",
			"pull_requests": []interface{}{map[string]interface{}{"number": float64(189)}},
		},
		"repository": map[string]interface{}{"full_name": "tstapler/stapler-squad"},
	})
}

func TestHandlePRFixEvent_should_Return200WithNoAuditRow_When_FeatureFlagDisabled(t *testing.T) {
	infra := newWebhookTestInfra(t)
	// pr_event_webhooks left unset (default false); webhook_triggers is on so Handle
	// reaches the event-type branch.
	newGitHubPushWorkflow(t, infra, "gh-1", "s3cr3t", "tstapler/stapler-squad", "main", "x")
	router := &fakePRFixEventRouter{matched: true}
	h := NewGitHubWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg, router)

	body := checkRunFailureBody(t)
	rec := doPRFixEventRequest(t, h, "check_run", body, "delivery-flag-off", sign("s3cr3t", body))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, router.callCount(), "router must never be called while the flag is off")
}

func TestHandlePRFixEvent_should_CallRouterAndPersistFiredSuccess_When_DeliveryActionableAndMatched(t *testing.T) {
	infra := newWebhookTestInfra(t)
	infra.cfg.FeatureFlags["pr_event_webhooks"] = true
	wf := newGitHubPushWorkflow(t, infra, "gh-2", "s3cr3t", "tstapler/stapler-squad", "main", "x")
	router := &fakePRFixEventRouter{matched: true}
	h := NewGitHubWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg, router)

	body := checkRunFailureBody(t)
	rec := doPRFixEventRequest(t, h, "check_run", body, "delivery-fired", sign("s3cr3t", body))

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, router.callCount())
	assert.Equal(t, "tstapler/stapler-squad", router.calls[0].repoFullName)
	assert.Equal(t, 189, router.calls[0].prNumber)

	events, err := infra.fireEvents.ListByWorkflow(context.Background(), wf.ID, 10)
	require.NoError(t, err)
	// fired_success rows for PR-fix events carry WorkflowID: nil (Migration Plan) — they
	// don't show up under this specific workflow's ListByWorkflow.
	assert.Empty(t, events)
}

func TestHandlePRFixEvent_should_PersistFiredFailed_When_PRFixRouterIsNilConfigured(t *testing.T) {
	infra := newWebhookTestInfra(t)
	infra.cfg.FeatureFlags["pr_event_webhooks"] = true
	newGitHubPushWorkflow(t, infra, "gh-3", "s3cr3t", "tstapler/stapler-squad", "main", "x")
	h := NewGitHubWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg, nil)

	body := checkRunFailureBody(t)
	rec := doPRFixEventRequest(t, h, "check_run", body, "delivery-no-router", sign("s3cr3t", body))

	assert.Equal(t, http.StatusOK, rec.Code, "a router-wiring gap must never surface as a 5xx")
}

func TestHandlePRFixEvent_should_PersistNoMatchWithoutCallingRouter_When_DeliveryNotActionable(t *testing.T) {
	infra := newWebhookTestInfra(t)
	infra.cfg.FeatureFlags["pr_event_webhooks"] = true
	newGitHubPushWorkflow(t, infra, "gh-4", "s3cr3t", "tstapler/stapler-squad", "main", "x")
	router := &fakePRFixEventRouter{matched: true}
	h := NewGitHubWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg, router)

	body := jsonBody(t, map[string]interface{}{
		"action": "completed",
		"check_run": map[string]interface{}{
			"conclusion":    "success",
			"pull_requests": []interface{}{map[string]interface{}{"number": float64(189)}},
		},
		"repository": map[string]interface{}{"full_name": "tstapler/stapler-squad"},
	})
	// Deliberately no/garbage signature — a non-actionable delivery must short-circuit
	// BEFORE signature verification (Story 2.3.2's cost-ordering note).
	rec := doPRFixEventRequest(t, h, "check_run", body, "delivery-healthy", "")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, router.callCount())
}

func TestHandlePRFixEvent_should_Return401AndRecordRejected_When_WorkflowSecretEmpty(t *testing.T) {
	infra := newWebhookTestInfra(t)
	infra.cfg.FeatureFlags["pr_event_webhooks"] = true
	_, err := infra.workflowRepo.Create(context.Background(), session.WorkflowCreateInput{
		Slug:                   "gh-5",
		Name:                   "GH Push Trigger",
		Command:                "base instructions",
		TriggerType:            "github_push",
		GitHubRepo:             "tstapler/stapler-squad",
		GitHubBranch:           "main",
		WebhookSecretEncrypted: "", // empty — decryptWorkflowSecret must early-return
		PromptTemplate:         "x",
		Enabled:                boolPtr(true),
	})
	require.NoError(t, err)
	router := &fakePRFixEventRouter{matched: true}
	h := NewGitHubWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg, router)

	body := checkRunFailureBody(t)
	rec := doPRFixEventRequest(t, h, "check_run", body, "delivery-empty-secret", "sha256=whatever")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Equal(t, 0, router.callCount())
}

func TestHandlePRFixEvent_should_NeverApplySelfFilter_When_EventTypeIsCheckRunOrWorkflowRun(t *testing.T) {
	infra := newWebhookTestInfra(t)
	infra.cfg.FeatureFlags["pr_event_webhooks"] = true
	newGitHubPushWorkflow(t, infra, "gh-6", "s3cr3t", "tstapler/stapler-squad", "main", "x")
	router := &fakePRFixEventRouter{matched: true}
	h := NewGitHubWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg, router)
	// Prime the self-login cache to something that would match if extractActorLogin
	// were (incorrectly) consulted for check_run — it never has an actor field, so this
	// only proves the filter path isn't reached, not that it's bypassed.
	h.selfLogin.mu.Lock()
	h.selfLogin.login = "irrelevant"
	h.selfLogin.fetchedAt = time.Now()
	h.selfLogin.mu.Unlock()

	body := checkRunFailureBody(t)
	rec := doPRFixEventRequest(t, h, "check_run", body, "delivery-checkrun-selffilter", sign("s3cr3t", body))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, router.callCount(), "check_run must never be suppressed by the self-actor filter")
}

func TestHandlePRFixEvent_should_SuppressActionable_When_CommentAuthorIsSelf(t *testing.T) {
	infra := newWebhookTestInfra(t)
	infra.cfg.FeatureFlags["pr_event_webhooks"] = true
	newGitHubPushWorkflow(t, infra, "gh-7", "s3cr3t", "tstapler/stapler-squad", "main", "x")
	router := &fakePRFixEventRouter{matched: true}
	h := NewGitHubWebhookHandler(infra.workflowRepo, infra.scheduler, infra.fireEvents, infra.cfg, router)
	h.selfLogin.mu.Lock()
	h.selfLogin.login = "stapler-squad-bot"
	h.selfLogin.fetchedAt = time.Now()
	h.selfLogin.mu.Unlock()

	body := jsonBody(t, map[string]interface{}{
		"action":     "created",
		"issue":      map[string]interface{}{"number": float64(189), "pull_request": map[string]interface{}{"url": "..."}},
		"comment":    map[string]interface{}{"user": map[string]interface{}{"login": "stapler-squad-bot"}},
		"repository": map[string]interface{}{"full_name": "tstapler/stapler-squad"},
	})
	rec := doPRFixEventRequest(t, h, "issue_comment", body, "delivery-self-comment", sign("s3cr3t", body))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, router.callCount(), "a comment from this instance's own GitHub identity must be suppressed")
}

// --- helpers -----------------------------------------------------------

func jsonBody(t *testing.T, v map[string]interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
