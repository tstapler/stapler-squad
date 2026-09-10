package session

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// PR Tracking Methods for Instance
// These methods provide GitHub PR lifecycle management for sessions created from PR URLs

// RefreshPRInfo fetches the latest PR information from GitHub
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) RefreshPRInfo(ctx context.Context) (*github.PRInfo, error) {
	if !i.IsPRSession() {
		return nil, fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.Info("refreshing PR info", "session", i.Title, "pr", i.GitHubPRNumber, "owner", i.GitHubOwner, "repo", i.GitHubRepo)

	prInfo, err := github.GetPRInfoCtx(ctx, i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR info for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully refreshed PR info", "session", i.Title, "title", prInfo.Title, "state", prInfo.State)

	return prInfo, nil
}

// GetPRComments fetches all comments on the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRComments(ctx context.Context) ([]github.PRComment, error) {
	if !i.IsPRSession() {
		return nil, fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.Info("fetching PR comments", "session", i.Title, "pr", i.GitHubPRNumber, "owner", i.GitHubOwner, "repo", i.GitHubRepo)

	comments, err := github.GetPRComments(ctx, i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR comments for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully fetched PR comments", "session", i.Title, "count", len(comments))

	return comments, nil
}

// GetPRDiff fetches the diff for the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRDiff(ctx context.Context) (string, error) {
	if !i.IsPRSession() {
		return "", fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.Info("fetching PR diff", "session", i.Title, "pr", i.GitHubPRNumber, "owner", i.GitHubOwner, "repo", i.GitHubRepo)

	diff, err := github.GetPRDiff(ctx, i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber)
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR diff for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully fetched PR diff", "session", i.Title, "bytes", len(diff))

	return diff, nil
}

// PostComment posts a comment to the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) PostComment(ctx context.Context, body string) error {
	if !i.IsPRSession() {
		return fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	if body == "" {
		return fmt.Errorf("comment body cannot be empty")
	}

	log.Info("posting comment to PR", "pr", i.GitHubPRNumber, "session", i.Title)

	if err := github.PostPRComment(ctx, i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber, body); err != nil {
		return fmt.Errorf("failed to post comment to PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully posted comment to PR", "pr", i.GitHubPRNumber, "session", i.Title)

	return nil
}

// SetCommitStatus posts a commit status to the PR's current head commit via the GitHub
// Statuses API. It re-fetches PR info first so the status always lands on the current
// HEAD SHA rather than a stale one cached before a force-push or rebase.
// Returns an error if this is not a PR session or if the GitHub API call fails.
//
// Currently has no non-test callers; an untagged ctx inherits GitHubCallOriginFrom's
// background default until a caller exists.
func (i *Instance) SetCommitStatus(ctx context.Context, state github.CommitStatusState, statusContext, description string) error {
	if !i.IsPRSession() {
		return fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	prInfo, err := i.RefreshPRInfo(ctx)
	if err != nil {
		return err
	}

	req, err := github.NewCommitStatusRequest(prInfo.HeadSHA, state, statusContext, description)
	if err != nil {
		return fmt.Errorf("invalid commit status request for instance '%s': %w", i.Title, err)
	}

	repo, err := github.NewRepoRef(i.GitHubOwner, i.GitHubRepo)
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}

	log.Info("setting commit status on PR", "pr", i.GitHubPRNumber, "session", i.Title, "sha", prInfo.HeadSHA, "state", state, "context", statusContext)

	if err := github.SetCommitStatus(repo, req); err != nil {
		return fmt.Errorf("failed to set commit status for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully set commit status on PR", "pr", i.GitHubPRNumber, "session", i.Title, "sha", prInfo.HeadSHA, "state", state)

	return nil
}

// MergePR merges the PR using the specified merge method
// method can be: "merge", "squash", or "rebase"
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) MergePR(ctx context.Context, method string) error {
	if !i.IsPRSession() {
		return fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	// Validate merge method
	switch method {
	case "merge", "squash", "rebase":
		// Valid method
	case "":
		method = "merge" // Default to merge if not specified
	default:
		return fmt.Errorf("invalid merge method '%s': must be 'merge', 'squash', or 'rebase'", method)
	}

	log.Info("merging PR", "pr", i.GitHubPRNumber, "session", i.Title, "method", method)

	if err := github.MergePR(ctx, i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber, method); err != nil {
		return fmt.Errorf("failed to merge PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully merged PR", "pr", i.GitHubPRNumber, "session", i.Title)

	return nil
}

// ClosePR closes the PR without merging
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) ClosePR(ctx context.Context) error {
	if !i.IsPRSession() {
		return fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.Info("closing PR without merging", "pr", i.GitHubPRNumber, "session", i.Title)

	if err := github.ClosePR(ctx, i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber); err != nil {
		return fmt.Errorf("failed to close PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully closed PR", "pr", i.GitHubPRNumber, "session", i.Title)

	return nil
}

// GeneratePRContextPrompt generates a context prompt for Claude based on PR information
// This can be used to initialize a Claude Code session with comprehensive PR context
// Returns an error if this is not a PR session or if the GitHub API call fails
//
// Currently has no non-test callers; an untagged ctx inherits GitHubCallOriginFrom's
// background default until a caller exists.
func (i *Instance) GeneratePRContextPrompt(ctx context.Context) (string, error) {
	if !i.IsPRSession() {
		return "", fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.Info("generating PR context prompt", "session", i.Title, "pr", i.GitHubPRNumber, "owner", i.GitHubOwner, "repo", i.GitHubRepo)

	// Fetch PR information
	prInfo, err := i.RefreshPRInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR info for context prompt: %w", err)
	}

	// Generate prompt with PR description
	prompt := github.GeneratePRPrompt(prInfo, true)

	log.Info("successfully generated PR context prompt", "session", i.Title, "bytes", len(prompt))

	return prompt, nil
}
