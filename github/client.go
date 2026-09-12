package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	sessiongit "github.com/tstapler/stapler-squad/session/git"
	"golang.org/x/sync/singleflight"
)

// ErrNoPR is returned by GetPRForBranch when no pull request exists for the branch.
var ErrNoPR = errors.New("no pull request found for branch")

// authResult caches the outcome of a gh auth status check with an expiry time.
type authResult struct {
	err    error
	expiry time.Time
}

var (
	// ghAuthCache is keyed by host ("" means github.com) so a github.com auth
	// failure/success is never conflated with a GHE host's — see CheckGHAuthForHost.
	ghAuthCache sync.Map           // map[string]authResult
	ghAuthGroup singleflight.Group //nolint:exhaustruct
)

const ghAuthTTL = 5 * time.Minute

// githubGraphQLMigrationFlagName gates GetPRInfoCtx's dispatch to
// GetPRInfoGraphQL below. server/services/feature_flag_service.go's
// knownFeatureFlags registers this same literal under its own
// githubGraphQLMigrationFlagName constant — github cannot import
// server/services (that would be a cycle; server/services already imports
// github), so the name is duplicated here rather than shared, mirroring
// githubPriorityAdmissionFlagName's precedent (http_client.go). Keep both
// constants' string values in sync if this flag is ever renamed. Default
// off — see plan.md's Risk Control section for the dated flip trigger.
const githubGraphQLMigrationFlagName = "github:graphql-pr-info"

// PR state strings. These are the single source of truth for the three PR
// lifecycle states surfaced by PRInfo.State — GetPRByNumber and any other
// consumer of PRInfo.State should compare against these constants rather
// than hardcoding "open"/"closed"/"merged" string literals.
const (
	PRStateOpen   = "open"
	PRStateClosed = "closed"
	PRStateMerged = "merged"
)

// PRInfo contains metadata about a GitHub pull request
type PRInfo struct {
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	HeadRef      string    `json:"headRefName"`
	HeadSHA      string    `json:"headRefOid"`
	BaseRef      string    `json:"baseRefName"`
	State        string    `json:"state"`
	Author       string    `json:"author"`
	Labels       []string  `json:"labels"`
	HTMLURL      string    `json:"url"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	IsDraft      bool      `json:"isDraft"`
	Mergeable    string    `json:"mergeable"`
	Additions    int       `json:"additions"`
	Deletions    int       `json:"deletions"`
	ChangedFiles int       `json:"changedFiles"`

	// Review and CI status fields (populated by GetPRInfo with extended fields)
	ReviewDecision        string // "approved" / "changes_requested" / "review_required" / ""
	ApprovedCount         int    // Count of current non-dismissed APPROVED reviews
	ChangesRequestedCount int    // Count of current non-dismissed CHANGES_REQUESTED reviews
	CheckConclusion       string // "success" / "failure" / "pending" / "action_required" / "neutral" / ""
	CheckStatus           string // "completed" / "in_progress" / ""
	Checks                []CheckItem
	Reviews               []ReviewItem
}

// PRComment represents a comment on a PR (either issue comment or review comment)
type PRComment struct {
	ID        int       `json:"id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	Path      string    `json:"path,omitempty"`     // For review comments
	Line      int       `json:"line,omitempty"`     // For review comments
	IsReview  bool      `json:"isReview,omitempty"` // True if this is a review comment
}

// ghPRResponse represents the JSON response from gh pr view --json
type ghPRResponse struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	HeadRefName  string `json:"headRefName"`
	HeadRefOid   string `json:"headRefOid"`
	BaseRefName  string `json:"baseRefName"`
	State        string `json:"state"`
	URL          string `json:"url"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	IsDraft      bool   `json:"isDraft"`
	Mergeable    string `json:"mergeable"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	ChangedFiles int    `json:"changedFiles"`
	Author       struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	ReviewDecision    string              `json:"reviewDecision"` // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Reviews           []ghReviewItem      `json:"reviews"`
	StatusCheckRollup []ghStatusCheckItem `json:"statusCheckRollup"`
}

// ghReviewItem represents a single review from gh pr view --json reviews
type ghReviewItem struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	State string `json:"state"` // APPROVED, CHANGES_REQUESTED, DISMISSED, COMMENTED, PENDING
	Body  string `json:"body"`
}

// ghStatusCheckItem represents a single status check from gh pr view --json statusCheckRollup
type ghStatusCheckItem struct {
	Name       string `json:"name"`
	Context    string `json:"context"`
	State      string `json:"state"`      // SUCCESS, FAILURE, PENDING, ERROR, NEUTRAL
	Status     string `json:"status"`     // completed, in_progress, queued
	Conclusion string `json:"conclusion"` // success, failure, cancelled, action_required, neutral, skipped, timed_out
}

// CheckItem is one itemized CI check from a PR's statusCheckRollup — the data
// getCheckConclusion collapses into a single string; exported so callers that want
// the itemized view (e.g. the VCS tab's "why blocked" rollup) don't need to.
type CheckItem struct {
	Name       string
	Context    string
	State      string
	Status     string
	Conclusion string
}

// ReviewItem is one PR review — the data parseReviewCounts collapses into
// approved/changes-requested counts, exported so callers can also read the
// reviewer's Body text (e.g. a CHANGES_REQUESTED review's stated reason).
type ReviewItem struct {
	Author string
	State  string
	Body   string
}

// ghCommentResponse represents a comment from gh pr view --json comments
type ghCommentResponse struct {
	ID        int    `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

// CheckGHAuth verifies github.com authentication. Equivalent to
// CheckGHAuthForHost(ctx, "") — kept for callers with no host to give it yet.
//
// ctx is threaded through to the underlying request so GitHubCallOriginFrom
// sees the caller's origin tag (e.g. an interactive PR action) instead of
// always falling back to the background-tier default — a singleflight-joined
// call still only uses the first caller's ctx for the shared in-flight
// request, the same limitation singleflight coalescing always has.
func CheckGHAuth(ctx context.Context) error {
	return CheckGHAuthForHost(ctx, "")
}

// CheckGHAuthForHost verifies GitHub authentication for host ("" means
// github.com) via GET /user using the native HTTP client. No subprocess is
// invoked — avoids forkExec lock contention. Results are cached for 5
// minutes, keyed per host so a github.com auth failure/success is never
// conflated with a GHE host's. Concurrent callers for the same host share a
// single inflight call via singleflight.
func CheckGHAuthForHost(ctx context.Context, host string) error {
	// Fast path: return cached result if still fresh.
	if v, ok := ghAuthCache.Load(host); ok {
		if r := v.(authResult); time.Now().Before(r.expiry) {
			return r.err
		}
	}

	// Slow path: at most one goroutine calls the API; others wait and reuse the result.
	res, err, _ := ghAuthGroup.Do("auth:"+host, func() (interface{}, error) {
		// context.WithoutCancel: ghAuthGroup coalesces every concurrent caller
		// process-wide onto whichever one's Do() call happened to start the
		// in-flight request — if that leader's own ctx got canceled first
		// (e.g. its RPC/test returned), a plain context.WithTimeout(ctx, ...)
		// here would cancel the shared request out from under every other
		// still-waiting caller too, surfacing as their result instead of a
		// real auth failure. Detach from ctx's cancellation, keep its
		// call-origin/call-site values, and rely solely on the fixed 10s
		// timeout to bound the request.
		authCheckCtx, authCheckCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer authCheckCancel()
		authCheckCtx = WithGitHubCallSite(authCheckCtx, "auth.check")

		req, buildErr := newGHRequestForHost(authCheckCtx, host, "user")
		if buildErr != nil {
			authErr := fmt.Errorf("GitHub auth check: failed to build request: %w", buildErr)
			ghAuthCache.Store(host, authResult{err: authErr, expiry: time.Now().Add(ghAuthTTL)})
			return authErr, nil
		}

		resp, doErr := ghHTTPClient.Do(req)
		if doErr != nil {
			authErr := fmt.Errorf("GitHub auth check: request failed: %w", doErr)
			ghAuthCache.Store(host, authResult{err: authErr, expiry: time.Now().Add(ghAuthTTL)})
			return authErr, nil
		}
		defer resp.Body.Close()

		var authErr error
		if resp.StatusCode == http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
		} else {
			// classifyGHResponse distinguishes real auth failures (401, or a
			// 403 with no rate-limit signal) from rate limiting (403 with
			// Retry-After or X-RateLimit-Remaining: 0, or 429) — a plain
			// "not authenticated, run gh auth login" message on a rate-limited
			// response is misleading and sends users to fix the wrong thing.
			authErr = classifyGHResponse(resp, "", false)
		}

		ghAuthCache.Store(host, authResult{err: authErr, expiry: time.Now().Add(ghAuthTTL)})
		return authErr, nil
	})

	// singleflight itself never returns a non-nil error here (we always return
	// nil from the Do closure and carry the real error in res), but handle it
	// defensively.
	if err != nil {
		return err
	}
	if res != nil {
		return res.(error)
	}
	return nil
}

// GetCurrentUserLogin returns the GitHub login of the authenticated user on
// host ("" means github.com) via GET /user. Returns an empty string (not an
// error) when unauthenticated so callers can degrade gracefully. Returns a
// non-nil error when the request is rate limited instead — that isn't the
// same as being unauthenticated.
func GetCurrentUserLogin(ctx context.Context, host string) (string, error) {
	ctx = WithGitHubCallSite(ctx, "user.login")
	req, err := newGHRequestForHost(ctx, host, "user")
	if err != nil {
		return "", fmt.Errorf("build /user request: %w", err)
	}
	return fetchLoginFromRequest(req)
}

// GetCurrentUserLoginWithToken fetches the GitHub login for an explicit token
// on host ("" means github.com).
// Returns ("", nil) when the token is invalid or unauthenticated, and a
// non-nil error when the request is rate limited instead.
func GetCurrentUserLoginWithToken(ctx context.Context, host, token string) (string, error) {
	ctx = WithGitHubCallSite(ctx, "user.login")
	req, err := newGHRequestForHostWithToken(ctx, host, "user", token)
	if err != nil {
		return "", fmt.Errorf("build /user request: %w", err)
	}
	return fetchLoginFromRequest(req)
}

func fetchLoginFromRequest(req *http.Request) (string, error) {
	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("/user request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// A rate-limited 403 isn't "unauthenticated" — surface it as an error
		// so callers don't silently treat rate limiting as a missing/invalid
		// token and degrade as if no one is logged in.
		if isGHRateLimited(resp) {
			return "", classifyGHResponse(resp, "", false)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("/user: unexpected status %d", resp.StatusCode)
	}

	var u struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", fmt.Errorf("decode /user response: %w", err)
	}
	return u.Login, nil
}

// GetPRInfo fetches metadata for a pull request including review and CI status.
func GetPRInfo(owner, repo string, prNumber int) (*PRInfo, error) {
	ref, _ := NewRepoRef(owner, repo)
	return GetPRInfoCtx(context.Background(), ref, prNumber)
}

// GetPRInfoCtx fetches metadata for a pull request with context support.
// Includes review decisions and CI/check status. ref.Host() "" means github.com.
func GetPRInfoCtx(ctx context.Context, ref RepoRef, prNumber int) (*PRInfo, error) {
	if config.LoadConfig().GetFeatureFlagWithDefault(githubGraphQLMigrationFlagName, false) {
		// GetPRInfoGraphQL is github.com-only today; this flag defaults off,
		// so a GHE ref silently falling back to github.com's GraphQL API here
		// is a pre-existing gap, not something introduced by host-awareness.
		return GetPRInfoGraphQL(ctx, ref.Owner(), ref.Repo(), prNumber)
	}

	if err := CheckGHAuthForHost(ctx, ref.Host()); err != nil {
		return nil, err
	}

	repoRef := fmt.Sprintf("%s/%s", ref.Owner(), ref.Repo())
	if ref.Host() != "" {
		repoRef = fmt.Sprintf("%s/%s", ref.Host(), repoRef)
	}
	prRef := strconv.Itoa(prNumber)

	fields := "number,title,body,headRefName,headRefOid,baseRefName,state,url,createdAt,updatedAt,isDraft,mergeable,additions,deletions,changedFiles,author,labels,reviews,reviewDecision,statusCheckRollup"
	cmd := safeexec.CommandContext(ctx, "gh", "pr", "view", prRef, "--repo", repoRef, "--json", fields)
	output, err := runGHCLICommand(ctx, "pr.view", func() ([]byte, error) { return cmd.Output() })
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to get PR info: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to get PR info: %w", err)
	}

	var resp ghPRResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse PR info: %w", err)
	}

	createdAt, _ := time.Parse(time.RFC3339, resp.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, resp.UpdatedAt)

	labels := make([]string, len(resp.Labels))
	for i, label := range resp.Labels {
		labels[i] = label.Name
	}

	approvedCount, changesReqCount := parseReviewCounts(resp.Reviews)
	checkConclusion, checkStatus := getCheckConclusion(resp.StatusCheckRollup)

	checks := make([]CheckItem, len(resp.StatusCheckRollup))
	for i, c := range resp.StatusCheckRollup {
		checks[i] = CheckItem(c)
	}
	reviews := make([]ReviewItem, len(resp.Reviews))
	for i, r := range resp.Reviews {
		reviews[i] = ReviewItem{Author: r.Author.Login, State: r.State, Body: r.Body}
	}

	return &PRInfo{
		Number:                resp.Number,
		Title:                 resp.Title,
		Body:                  resp.Body,
		HeadRef:               resp.HeadRefName,
		HeadSHA:               resp.HeadRefOid,
		BaseRef:               resp.BaseRefName,
		State:                 strings.ToLower(resp.State),
		Author:                resp.Author.Login,
		Labels:                labels,
		HTMLURL:               resp.URL,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		IsDraft:               resp.IsDraft,
		Mergeable:             resp.Mergeable,
		Additions:             resp.Additions,
		Deletions:             resp.Deletions,
		ChangedFiles:          resp.ChangedFiles,
		ReviewDecision:        strings.ToLower(resp.ReviewDecision),
		ApprovedCount:         approvedCount,
		ChangesRequestedCount: changesReqCount,
		CheckConclusion:       checkConclusion,
		CheckStatus:           checkStatus,
		Checks:                checks,
		Reviews:               reviews,
	}, nil
}

// parseReviewCounts derives approved/changes-requested counts from review items.
// Uses latest non-dismissed, non-comment state per reviewer.
func parseReviewCounts(reviews []ghReviewItem) (approved, changesRequested int) {
	latestState := make(map[string]string)
	for _, r := range reviews {
		login := r.Author.Login
		state := strings.ToUpper(r.State)
		if state == "DISMISSED" {
			delete(latestState, login)
			continue
		}
		// COMMENTED does not override a blocking or approving review
		if state == "COMMENTED" {
			continue
		}
		latestState[login] = state
	}
	for _, state := range latestState {
		switch state {
		case "APPROVED":
			approved++
		case "CHANGES_REQUESTED":
			changesRequested++
		}
	}
	return
}

// getCheckConclusion derives a single conclusion from statusCheckRollup items.
func getCheckConclusion(checks []ghStatusCheckItem) (conclusion, status string) {
	if len(checks) == 0 {
		return "", ""
	}
	hasInProgress := false
	hasFailure := false
	allSuccess := true

	for _, check := range checks {
		c := strings.ToLower(check.Conclusion)
		s := strings.ToLower(check.Status)
		st := strings.ToLower(check.State)
		if c == "" {
			c = st
		}
		switch {
		case c == "failure" || c == "error" || c == "action_required" || c == "timed_out":
			hasFailure = true
			allSuccess = false
		case c == "success":
			// success
		case s == "in_progress" || s == "queued" || c == "pending":
			hasInProgress = true
			allSuccess = false
		default:
			allSuccess = false
		}
	}
	if hasFailure {
		return "failure", "completed"
	}
	if hasInProgress {
		return "pending", "in_progress"
	}
	if allSuccess {
		return "success", "completed"
	}
	return "neutral", "completed"
}

// GetPRForBranch finds the GitHub PR associated with a branch.
// Uses the GitHub REST API directly (no gh subprocess) to avoid forkExec lock contention.
// Returns ErrNoPR when no pull request exists for the branch.
func GetPRForBranch(ctx context.Context, ref RepoRef, branch string) (*PRInfo, error) {
	ctx = WithGitHubCallSite(ctx, "pr.lookup.by_branch")
	owner, repo, host := ref.Owner(), ref.Repo(), ref.Host()
	apiPath := fmt.Sprintf("repos/%s/%s/pulls?head=%s&state=all&per_page=10",
		url.PathEscape(owner), url.PathEscape(repo),
		url.QueryEscape(owner+":"+branch))

	req, err := newGHRequestForHost(ctx, host, apiPath)
	if err != nil {
		return nil, fmt.Errorf("build PR list request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PR list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyGHResponse(resp, "", false)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read PR list response: %w", err)
	}

	var prs []struct {
		Number    int    `json:"number"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(body, &prs); err != nil {
		return nil, fmt.Errorf("parse PR list: %w", err)
	}
	if len(prs) == 0 {
		return nil, ErrNoPR
	}

	// Use most recently updated PR when multiple PRs target the same branch.
	sort.Slice(prs, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, prs[i].UpdatedAt)
		tj, _ := time.Parse(time.RFC3339, prs[j].UpdatedAt)
		return ti.After(tj)
	})

	return GetPRInfoCtx(ctx, ref, prs[0].Number)
}

// GetPRByNumber fetches a single pull request by its number using the GitHub
// REST API directly (no gh subprocess). Unlike GetPRForBranch, which looks a
// PR up by head branch name and can therefore match the wrong PR when a
// branch is reused or renamed, GetPRByNumber looks a PR up by its immutable
// number — the root-cause fix for branch-name-keyed lookups matching stale
// or unrelated PRs.
//
// Returns ErrNoPR when no pull request exists for the given number (HTTP
// 404). Before returning success, the response's base.repo.full_name is
// compared against the requested owner/repo; a mismatch returns a non-nil,
// non-ErrNoPR error rather than trusting the response body blindly.
func GetPRByNumber(ctx context.Context, ref RepoRef, prNumber int) (*PRInfo, error) {
	ctx = WithGitHubCallSite(ctx, "pr.view.by_number")
	owner, repo := ref.Owner(), ref.Repo()
	apiPath := fmt.Sprintf("repos/%s/%s/pulls/%d",
		url.PathEscape(owner), url.PathEscape(repo), prNumber)

	req, err := newGHRequestForHost(ctx, ref.Host(), apiPath)
	if err != nil {
		return nil, fmt.Errorf("build PR request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PR request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, ErrNoPR
	}
	if resp.StatusCode != http.StatusOK {
		return nil, classifyGHResponse(resp, "", false)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read PR response: %w", err)
	}

	// Distinct local response struct for this REST endpoint's shape — note
	// the author field here is user.login, NOT author.login (the shape used
	// by ghPRResponse for the gh pr view --json subprocess path above).
	var prResp struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
		Merged  bool   `json:"merged"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref  string `json:"ref"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &prResp); err != nil {
		return nil, fmt.Errorf("parse PR response: %w", err)
	}

	wantFullName := owner + "/" + repo
	if prResp.Base.Repo.FullName != wantFullName {
		return nil, fmt.Errorf("PR #%d base.repo.full_name %q does not match requested repo %q",
			prNumber, prResp.Base.Repo.FullName, wantFullName)
	}

	state := PRStateOpen
	switch {
	case prResp.Merged:
		state = PRStateMerged
	case prResp.State == PRStateClosed:
		state = PRStateClosed
	}

	return &PRInfo{
		Number:  prResp.Number,
		HeadRef: prResp.Head.Ref,
		BaseRef: prResp.Base.Ref,
		State:   state,
		Author:  prResp.User.Login,
		HTMLURL: prResp.HTMLURL,
	}, nil
}

// ghRepoArg formats ref as gh's --repo flag value: "OWNER/REPO" for
// github.com, or "HOST/OWNER/REPO" for a GitHub Enterprise host — gh's --repo
// flag accepts both forms ("[HOST/]OWNER/REPO").
func ghRepoArg(ref RepoRef) string {
	if ref.Host() != "" {
		return fmt.Sprintf("%s/%s/%s", ref.Host(), ref.Owner(), ref.Repo())
	}
	return fmt.Sprintf("%s/%s", ref.Owner(), ref.Repo())
}

// setGHHostEnv sets GH_HOST=host on cmd's environment when host is a GitHub
// Enterprise host. gh's --repo flag alone is not always sufficient to pick
// the right auth context; GH_HOST is the documented way to point the whole
// gh invocation at an enterprise instance.
func setGHHostEnv(cmd *exec.Cmd, host string) {
	if host == "" {
		return
	}
	cmd.Env = append(os.Environ(), "GH_HOST="+NormalizeHost(host))
}

// GetPRComments fetches all comments on a pull request.
func GetPRComments(ctx context.Context, ref RepoRef, prNumber int) ([]PRComment, error) {
	if err := CheckGHAuthForHost(ctx, ref.Host()); err != nil {
		return nil, err
	}

	repoRef := ghRepoArg(ref)
	prRef := strconv.Itoa(prNumber)

	// Get comments
	commentsCtx, commentsCancel := context.WithTimeout(ctx, 30*time.Second)
	defer commentsCancel()
	cmd := safeexec.CommandContext(commentsCtx, "gh", "pr", "view", prRef, "--repo", repoRef, "--json", "comments")
	setGHHostEnv(cmd, ref.Host())
	output, err := runGHCLICommand(commentsCtx, "pr.comments", func() ([]byte, error) { return cmd.Output() })
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to get PR comments: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to get PR comments: %w", err)
	}

	var resp struct {
		Comments []ghCommentResponse `json:"comments"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse PR comments: %w", err)
	}

	comments := make([]PRComment, len(resp.Comments))
	for i, c := range resp.Comments {
		createdAt, _ := time.Parse(time.RFC3339, c.CreatedAt)
		comments[i] = PRComment{
			ID:        c.ID,
			Author:    c.Author.Login,
			Body:      c.Body,
			CreatedAt: createdAt,
			Path:      c.Path,
			Line:      c.Line,
			IsReview:  c.Path != "",
		}
	}

	return comments, nil
}

// GetPRDiff fetches the diff for a pull request.
func GetPRDiff(ctx context.Context, ref RepoRef, prNumber int) (string, error) {
	if err := CheckGHAuthForHost(ctx, ref.Host()); err != nil {
		return "", err
	}

	repoRef := ghRepoArg(ref)
	prRef := strconv.Itoa(prNumber)

	diffCtx, diffCancel := context.WithTimeout(ctx, 30*time.Second)
	defer diffCancel()
	cmd := safeexec.CommandContext(diffCtx, "gh", "pr", "diff", prRef, "--repo", repoRef)
	setGHHostEnv(cmd, ref.Host())
	output, err := runGHCLICommand(diffCtx, "pr.diff", func() ([]byte, error) { return cmd.Output() })
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to get PR diff: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to get PR diff: %w", err)
	}

	return string(output), nil
}

// PostPRComment posts a comment on a pull request.
func PostPRComment(ctx context.Context, ref RepoRef, prNumber int, body string) error {
	if err := CheckGHAuthForHost(ctx, ref.Host()); err != nil {
		return err
	}

	repoRef := ghRepoArg(ref)
	prRef := strconv.Itoa(prNumber)

	commentCtx, commentCancel := context.WithTimeout(ctx, 30*time.Second)
	defer commentCancel()
	cmd := safeexec.CommandContext(commentCtx, "gh", "pr", "comment", prRef, "--repo", repoRef, "--body", body)
	setGHHostEnv(cmd, ref.Host())
	if _, err := runGHCLICommand(commentCtx, "pr.comment", func() ([]byte, error) { return nil, cmd.Run() }); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to post comment: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to post comment: %w", err)
	}

	return nil
}

// MergePR merges a pull request.
// method can be: "merge", "squash", or "rebase"
func MergePR(ctx context.Context, ref RepoRef, prNumber int, method string) error {
	if err := CheckGHAuthForHost(ctx, ref.Host()); err != nil {
		return err
	}

	repoRef := ghRepoArg(ref)
	prRef := strconv.Itoa(prNumber)

	args := []string{"pr", "merge", prRef, "--repo", repoRef}
	switch method {
	case "squash":
		args = append(args, "--squash")
	case "rebase":
		args = append(args, "--rebase")
	default:
		args = append(args, "--merge")
	}

	mergeCtx, mergeCancel := context.WithTimeout(ctx, 60*time.Second)
	defer mergeCancel()
	cmd := safeexec.CommandContext(mergeCtx, "gh", args...)
	setGHHostEnv(cmd, ref.Host())
	if _, err := runGHCLICommand(mergeCtx, "pr.merge", func() ([]byte, error) { return nil, cmd.Run() }); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to merge PR: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to merge PR: %w", err)
	}

	return nil
}

// ClosePR closes a pull request without merging.
func ClosePR(ctx context.Context, ref RepoRef, prNumber int) error {
	if err := CheckGHAuthForHost(ctx, ref.Host()); err != nil {
		return err
	}

	repoRef := ghRepoArg(ref)
	prRef := strconv.Itoa(prNumber)

	closeCtx, closeCancel := context.WithTimeout(ctx, 30*time.Second)
	defer closeCancel()
	cmd := safeexec.CommandContext(closeCtx, "gh", "pr", "close", prRef, "--repo", repoRef)
	setGHHostEnv(cmd, ref.Host())
	if _, err := runGHCLICommand(closeCtx, "pr.close", func() ([]byte, error) { return nil, cmd.Run() }); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to close PR: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to close PR: %w", err)
	}

	return nil
}

// remoteURLCache memoises GetRemoteURL results per canonical git common-dir
// (see remoteCacheKey), not per repoPath. Remote URLs are stable for a repo's
// lifetime; no TTL needed.
var remoteURLCache sync.Map // map[string]string

// remoteURLGroup coalesces concurrent GetRemoteURL cache misses for the same
// canonical key (e.g. many session worktrees of one repo starting at once)
// into a single subprocess call.
var remoteURLGroup singleflight.Group //nolint:exhaustruct

// remoteCacheKey resolves repoPath to the shared git common-dir (the main
// .git directory) so every worktree of the same repository maps to one cache
// entry instead of one per worktree path. Filesystem-only, no subprocess —
// same parsing approach as session/unfinished/gogit_vcs_reader.go's
// gitCommonDir, though its fallback differs slightly (this always falls back
// to repoPath itself on any parse failure, since here that's just the old
// per-worktree cache key — a degraded cache hit rate, never a correctness
// issue — whereas gitCommonDir's callers need a real git dir path).
func remoteCacheKey(repoPath string) string {
	gitPath := filepath.Join(repoPath, ".git")
	data, err := os.ReadFile(gitPath) // #nosec G304 -- repoPath is a session's own worktree directory, not user-supplied network input
	if err != nil {
		// .git is a directory (main repo, not a linked worktree) or missing.
		return repoPath
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return repoPath
	}
	wtGitDir := strings.TrimPrefix(line, prefix)
	if !filepath.IsAbs(wtGitDir) {
		wtGitDir = filepath.Join(repoPath, wtGitDir)
	}
	cdData, err := os.ReadFile(filepath.Join(wtGitDir, "commondir")) // #nosec G304 -- wtGitDir derives from repoPath's own .git file, not user input
	if err != nil {
		return repoPath
	}
	commondir := strings.TrimSpace(string(cdData))
	if !filepath.IsAbs(commondir) {
		commondir = filepath.Join(wtGitDir, commondir)
	}
	return filepath.Clean(commondir)
}

// GetRemoteURL returns the remote URL of a repository (used to determine owner/repo).
// Reads the URL out of the repo's own git config via go-git (session/git.OpenRepo,
// which sets EnableDotGitCommonDir so linked worktrees resolve correctly) instead of
// shelling out to `git remote get-url` — no subprocess, so no ForkLock contention.
// Cached and singleflight-coalesced per canonical repo (shared across all its
// worktrees), not per repoPath — a repo's remote is identical for every worktree,
// so without the shared key, every session worktree caused its own cache miss.
func GetRemoteURL(repoPath string) (string, error) {
	key := remoteCacheKey(repoPath)
	if v, ok := remoteURLCache.Load(key); ok {
		return v.(string), nil
	}
	v, err, _ := remoteURLGroup.Do(key, func() (any, error) {
		if v, ok := remoteURLCache.Load(key); ok {
			return v.(string), nil
		}
		repo, err := sessiongit.OpenRepo(repoPath)
		if err != nil {
			return "", fmt.Errorf("failed to get remote URL: failed to open repository: %w", err)
		}
		remote, err := repo.Remote("origin")
		if err != nil {
			return "", fmt.Errorf("failed to get remote URL: %w", err)
		}
		urls := remote.Config().URLs
		if len(urls) == 0 {
			return "", fmt.Errorf("failed to get remote URL: origin has no configured URL")
		}
		url := urls[0]
		remoteURLCache.Store(key, url)
		return url, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// GetOwnerRepoFromRemote returns a RepoRef for a local git repository by
// reading the origin remote URL and parsing it. Returns an invalid zero-value
// RepoRef (not an error) when the remote is not a GitHub URL.
func GetOwnerRepoFromRemote(repoPath string, enterpriseHosts []string) (RepoRef, error) {
	remoteURL, err := GetRemoteURL(repoPath)
	if err != nil {
		return RepoRef{}, err
	}
	ref, parseErr := ParseGitHubRefWithHosts(remoteURL, enterpriseHosts)
	if parseErr != nil {
		return RepoRef{}, nil // not a GitHub URL — callers check IsValid()
	}
	r, _ := NewRepoRefWithHost(ref.Owner, ref.Repo, ref.Host)
	return r, nil
}

// GeneratePRPrompt generates a context prompt from PR information
// This can be used to initialize a Claude Code session with PR context
func GeneratePRPrompt(pr *PRInfo, includeDescription bool) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Working on PR #%d: %s\n", pr.Number, pr.Title)
	fmt.Fprintf(&sb, "Branch: %s → %s\n", pr.HeadRef, pr.BaseRef)
	fmt.Fprintf(&sb, "Author: %s | State: %s\n", pr.Author, pr.State)

	if pr.ChangedFiles > 0 {
		fmt.Fprintf(&sb, "Changes: +%d/-%d across %d files\n", pr.Additions, pr.Deletions, pr.ChangedFiles)
	}

	if len(pr.Labels) > 0 {
		fmt.Fprintf(&sb, "Labels: %s\n", strings.Join(pr.Labels, ", "))
	}

	if includeDescription && pr.Body != "" {
		sb.WriteString("\n## PR Description\n")
		sb.WriteString(pr.Body)
		sb.WriteString("\n")
	}

	return sb.String()
}

// GetPRForBranchConditional is GetPRForBranch with ETag conditional request support.
// Pass the previously returned newEtag (empty string for first call).
// Returns (nil, etag, false, nil) on 304 Not Modified — caller should treat as unchanged.
// Every error path also returns changed=false, including ErrNotAuthenticated
// when no token is configured — callers must check err before treating
// changed=false as "unchanged, no error."
func GetPRForBranchConditional(ctx context.Context, ref RepoRef, branch, etag string) (info *PRInfo, newEtag string, changed bool, err error) {
	ctx = WithGitHubCallSite(ctx, "pr.lookup.by_branch.conditional")
	owner, repo, host := ref.Owner(), ref.Repo(), ref.Host()
	if getGHTokenForAccount(ctx, AccountRef{Host: host}) == "" {
		return nil, etag, false, ErrNotAuthenticated
	}

	apiPath := fmt.Sprintf("repos/%s/%s/pulls?head=%s&state=all&per_page=10",
		url.PathEscape(owner), url.PathEscape(repo),
		url.QueryEscape(owner+":"+branch))

	req, err := newGHRequestForHost(ctx, host, apiPath)
	if err != nil {
		return nil, etag, false, fmt.Errorf("build PR list request: %w", err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, etag, false, fmt.Errorf("PR list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, false, nil
	}

	respEtag := resp.Header.Get("ETag")

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, etag, false, fmt.Errorf("GitHub API: unauthorized (401)")
	}
	if resp.StatusCode == http.StatusForbidden {
		if resp.Header.Get("Retry-After") != "" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil, etag, false, fmt.Errorf("GitHub API: secondary rate limit (403)")
		}
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil, etag, false, fmt.Errorf("GitHub API: primary rate limit exhausted (403)")
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, etag, false, fmt.Errorf("GitHub API: forbidden (403)")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, etag, false, fmt.Errorf("GitHub API: rate limited (429)")
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, etag, false, fmt.Errorf("GitHub API returned status %d for PR list", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, etag, false, fmt.Errorf("read PR list response: %w", err)
	}

	var prs []struct {
		Number    int    `json:"number"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(body, &prs); err != nil {
		return nil, etag, false, fmt.Errorf("parse PR list: %w", err)
	}
	if len(prs) == 0 {
		return nil, respEtag, true, ErrNoPR
	}

	sort.Slice(prs, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, prs[i].UpdatedAt)
		tj, _ := time.Parse(time.RFC3339, prs[j].UpdatedAt)
		return ti.After(tj)
	})

	prInfo, err := GetPRInfoCtx(ctx, ref, prs[0].Number)
	if err != nil {
		return nil, respEtag, false, err
	}
	return prInfo, respEtag, true, nil
}
