package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
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
	ghAuthState atomic.Value       // stores authResult
	ghAuthGroup singleflight.Group //nolint:exhaustruct
)

const ghAuthTTL = 5 * time.Minute

// PRInfo contains metadata about a GitHub pull request
type PRInfo struct {
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	HeadRef      string    `json:"headRefName"`
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

// CheckGHAuth checks if GitHub CLI is installed and authenticated.
// Results are cached for 5 minutes. Concurrent callers that trigger a refresh
// share a single subprocess invocation via singleflight.
func CheckGHAuth() error {
	// Fast path: return cached result if still fresh.
	if v := ghAuthState.Load(); v != nil {
		if r := v.(authResult); time.Now().Before(r.expiry) {
			return r.err
		}
	}

	// Slow path: at most one goroutine runs the subprocess; others wait and
	// reuse the result.
	res, err, _ := ghAuthGroup.Do("auth", func() (interface{}, error) {
		var authErr error

		// Check if gh is installed.
		if _, lookErr := exec.LookPath("gh"); lookErr != nil {
			authErr = fmt.Errorf("GitHub CLI (gh) is not installed. Please install it: https://cli.github.com/")
		} else {
			// Check if gh is authenticated.
			authCheckCtx, authCheckCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer authCheckCancel()
			cmd := safeexec.CommandContext(authCheckCtx, "gh", "auth", "status")
			if runErr := cmd.Run(); runErr != nil {
				authErr = fmt.Errorf("GitHub CLI is not authenticated. Please run 'gh auth login' first")
			}
		}

		ghAuthState.Store(authResult{err: authErr, expiry: time.Now().Add(ghAuthTTL)})
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

// GetPRInfo fetches metadata for a pull request including review and CI status.
func GetPRInfo(owner, repo string, prNumber int) (*PRInfo, error) {
	return GetPRInfoCtx(context.Background(), owner, repo, prNumber)
}

// GetPRInfoCtx fetches metadata for a pull request with context support.
// Includes review decisions and CI/check status.
func GetPRInfoCtx(ctx context.Context, owner, repo string, prNumber int) (*PRInfo, error) {
	if err := CheckGHAuth(); err != nil {
		return nil, err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	prRef := strconv.Itoa(prNumber)

	fields := "number,title,body,headRefName,baseRefName,state,url,createdAt,updatedAt,isDraft,mergeable,additions,deletions,changedFiles,author,labels,reviews,reviewDecision,statusCheckRollup"
	cmd := safeexec.CommandContext(ctx, "gh", "pr", "view", prRef, "--repo", repoRef, "--json", fields)
	output, err := cmd.Output()
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

	return &PRInfo{
		Number:                resp.Number,
		Title:                 resp.Title,
		Body:                  resp.Body,
		HeadRef:               resp.HeadRefName,
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
// Returns nil (with nil error) if no PR exists for the branch.
func GetPRForBranch(ctx context.Context, owner, repo, branch string) (*PRInfo, error) {
	if err := CheckGHAuth(); err != nil {
		return nil, err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	cmd := safeexec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repoRef,
		"--head", branch,
		"--json", "number,updatedAt",
		"--state", "all",
		"--limit", "10",
	)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to list PRs for branch: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to list PRs for branch: %w", err)
	}

	var prs []struct {
		Number    int    `json:"number"`
		UpdatedAt string `json:"updatedAt"`
	}
	if err := json.Unmarshal(output, &prs); err != nil {
		return nil, fmt.Errorf("failed to parse PR list: %w", err)
	}
	if len(prs) == 0 {
		return nil, ErrNoPR
	}

	// Use most recently updated PR if multiple exist for the branch
	sort.Slice(prs, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, prs[i].UpdatedAt)
		tj, _ := time.Parse(time.RFC3339, prs[j].UpdatedAt)
		return ti.After(tj)
	})

	return GetPRInfoCtx(ctx, owner, repo, prs[0].Number)
}

// IsForkRepo reports whether the given repo is a fork of another repository.
func IsForkRepo(ctx context.Context, owner, repo string) (bool, error) {
	if err := CheckGHAuth(); err != nil {
		return false, err
	}

	cmd := safeexec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/%s", owner, repo),
		"--jq", ".fork",
	)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return false, fmt.Errorf("failed to check fork status: %s", string(exitErr.Stderr))
		}
		return false, fmt.Errorf("failed to check fork status: %w", err)
	}

	return strings.TrimSpace(string(output)) == "true", nil
}

// GetPRComments fetches all comments on a pull request
func GetPRComments(owner, repo string, prNumber int) ([]PRComment, error) {
	if err := CheckGHAuth(); err != nil {
		return nil, err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	prRef := strconv.Itoa(prNumber)

	// Get comments
	commentsCtx, commentsCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer commentsCancel()
	cmd := safeexec.CommandContext(commentsCtx, "gh", "pr", "view", prRef, "--repo", repoRef, "--json", "comments")
	output, err := cmd.Output()
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

// GetPRDiff fetches the diff for a pull request
func GetPRDiff(owner, repo string, prNumber int) (string, error) {
	if err := CheckGHAuth(); err != nil {
		return "", err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	prRef := strconv.Itoa(prNumber)

	diffCtx, diffCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer diffCancel()
	cmd := safeexec.CommandContext(diffCtx, "gh", "pr", "diff", prRef, "--repo", repoRef)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to get PR diff: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to get PR diff: %w", err)
	}

	return string(output), nil
}

// PostPRComment posts a comment on a pull request
func PostPRComment(owner, repo string, prNumber int, body string) error {
	if err := CheckGHAuth(); err != nil {
		return err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	prRef := strconv.Itoa(prNumber)

	commentCtx, commentCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer commentCancel()
	cmd := safeexec.CommandContext(commentCtx, "gh", "pr", "comment", prRef, "--repo", repoRef, "--body", body)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to post comment: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to post comment: %w", err)
	}

	return nil
}

// MergePR merges a pull request
// method can be: "merge", "squash", or "rebase"
func MergePR(owner, repo string, prNumber int, method string) error {
	if err := CheckGHAuth(); err != nil {
		return err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
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

	mergeCtx, mergeCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer mergeCancel()
	cmd := safeexec.CommandContext(mergeCtx, "gh", args...)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to merge PR: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to merge PR: %w", err)
	}

	return nil
}

// ClosePR closes a pull request without merging
func ClosePR(owner, repo string, prNumber int) error {
	if err := CheckGHAuth(); err != nil {
		return err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	prRef := strconv.Itoa(prNumber)

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer closeCancel()
	cmd := safeexec.CommandContext(closeCtx, "gh", "pr", "close", prRef, "--repo", repoRef)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to close PR: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to close PR: %w", err)
	}

	return nil
}

// CloneRepository clones a GitHub repository
func CloneRepository(owner, repo, targetPath string) error {
	if err := CheckGHAuth(); err != nil {
		return err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	cloneCtx, cloneCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cloneCancel()
	cmd := safeexec.CommandContext(cloneCtx, "gh", "repo", "clone", repoRef, targetPath)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to clone repository: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	return nil
}

// FetchBranch fetches a specific branch in an existing repository
func FetchBranch(repoPath, branchName string) error {
	// Fetch the branch from origin
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer fetchCancel()
	cmd := safeexec.CommandContext(fetchCtx, "git", "-C", repoPath, "fetch", "origin", branchName)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to fetch branch: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to fetch branch: %w", err)
	}

	return nil
}

// CheckoutBranch checks out a branch in an existing repository
func CheckoutBranch(repoPath, branchName string) error {
	checkoutCtx, checkoutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer checkoutCancel()
	cmd := safeexec.CommandContext(checkoutCtx, "git", "-C", repoPath, "checkout", branchName)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("failed to checkout branch: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to checkout branch: %w", err)
	}

	return nil
}

// GetRemoteURL returns the remote URL of a repository (used to determine owner/repo)
func GetRemoteURL(repoPath string) (string, error) {
	remoteCtx, remoteCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer remoteCancel()
	cmd := safeexec.CommandContext(remoteCtx, "git", "-C", repoPath, "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to get remote URL: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
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
