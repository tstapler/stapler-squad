package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// PRInfo contains metadata about a GitHub pull request
type PRInfo struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	HeadRef     string    `json:"headRefName"`
	BaseRef     string    `json:"baseRefName"`
	State       string    `json:"state"`
	Author      string    `json:"author"`
	Labels      []string  `json:"labels"`
	HTMLURL     string    `json:"url"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	IsDraft     bool      `json:"isDraft"`
	Mergeable   string    `json:"mergeable"`
	Additions   int       `json:"additions"`
	Deletions   int       `json:"deletions"`
	ChangedFiles int      `json:"changedFiles"`
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

// CheckGHAuth checks if GitHub CLI is installed and authenticated
func CheckGHAuth() error {
	// Check if gh is installed
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is not installed. Please install it: https://cli.github.com/")
	}

	// Check if gh is authenticated
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("GitHub CLI is not authenticated. Please run 'gh auth login' first")
	}

	return nil
}

// GetPRInfo fetches metadata for a pull request
func GetPRInfo(owner, repo string, prNumber int) (*PRInfo, error) {
	if err := CheckGHAuth(); err != nil {
		return nil, err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	prRef := strconv.Itoa(prNumber)

	// Get PR info with all relevant fields
	fields := "number,title,body,headRefName,baseRefName,state,url,createdAt,updatedAt,isDraft,mergeable,additions,deletions,changedFiles,author,labels"
	cmd := exec.Command("gh", "pr", "view", prRef, "--repo", repoRef, "--json", fields)
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

	// Parse timestamps
	createdAt, _ := time.Parse(time.RFC3339, resp.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, resp.UpdatedAt)

	// Extract label names
	labels := make([]string, len(resp.Labels))
	for i, label := range resp.Labels {
		labels[i] = label.Name
	}

	return &PRInfo{
		Number:       resp.Number,
		Title:        resp.Title,
		Body:         resp.Body,
		HeadRef:      resp.HeadRefName,
		BaseRef:      resp.BaseRefName,
		State:        resp.State,
		Author:       resp.Author.Login,
		Labels:       labels,
		HTMLURL:      resp.URL,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		IsDraft:      resp.IsDraft,
		Mergeable:    resp.Mergeable,
		Additions:    resp.Additions,
		Deletions:    resp.Deletions,
		ChangedFiles: resp.ChangedFiles,
	}, nil
}

// GetPRComments fetches all comments on a pull request
func GetPRComments(owner, repo string, prNumber int) ([]PRComment, error) {
	if err := CheckGHAuth(); err != nil {
		return nil, err
	}

	repoRef := fmt.Sprintf("%s/%s", owner, repo)
	prRef := strconv.Itoa(prNumber)

	// Get comments
	cmd := exec.Command("gh", "pr", "view", prRef, "--repo", repoRef, "--json", "comments")
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

	cmd := exec.Command("gh", "pr", "diff", prRef, "--repo", repoRef)
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

	cmd := exec.Command("gh", "pr", "comment", prRef, "--repo", repoRef, "--body", body)
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

	cmd := exec.Command("gh", args...)
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

	cmd := exec.Command("gh", "pr", "close", prRef, "--repo", repoRef)
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
	cmd := exec.Command("gh", "repo", "clone", repoRef, targetPath)
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
	cmd := exec.Command("git", "-C", repoPath, "fetch", "origin", branchName)
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
	cmd := exec.Command("git", "-C", repoPath, "checkout", branchName)
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
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
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

	sb.WriteString(fmt.Sprintf("Working on PR #%d: %s\n", pr.Number, pr.Title))
	sb.WriteString(fmt.Sprintf("Branch: %s → %s\n", pr.HeadRef, pr.BaseRef))
	sb.WriteString(fmt.Sprintf("Author: %s | State: %s\n", pr.Author, pr.State))

	if pr.ChangedFiles > 0 {
		sb.WriteString(fmt.Sprintf("Changes: +%d/-%d across %d files\n", pr.Additions, pr.Deletions, pr.ChangedFiles))
	}

	if len(pr.Labels) > 0 {
		sb.WriteString(fmt.Sprintf("Labels: %s\n", strings.Join(pr.Labels, ", ")))
	}

	if includeDescription && pr.Body != "" {
		sb.WriteString("\n## PR Description\n")
		sb.WriteString(pr.Body)
		sb.WriteString("\n")
	}

	return sb.String()
}
