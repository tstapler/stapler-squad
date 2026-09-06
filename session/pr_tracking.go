package session

import (
	"fmt"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// PR Tracking Methods for Instance
// These methods provide GitHub PR lifecycle management for sessions created from PR URLs

// RefreshPRInfo fetches the latest PR information from GitHub
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) RefreshPRInfo() (*github.PRInfo, error) {
	if !i.IsPRSession() {
		return nil, fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	gh := i.GitHub()
	log.Info("refreshing PR info", "session", i.Title, "pr", gh.PRNumber, "owner", gh.Owner, "repo", gh.Repo)

	prInfo, err := github.GetPRInfo(gh.Owner, gh.Repo, gh.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR info for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully refreshed PR info", "session", i.Title, "title", prInfo.Title, "state", prInfo.State)

	return prInfo, nil
}

// GetPRComments fetches all comments on the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRComments() ([]github.PRComment, error) {
	if !i.IsPRSession() {
		return nil, fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	gh := i.GitHub()
	log.Info("fetching PR comments", "session", i.Title, "pr", gh.PRNumber, "owner", gh.Owner, "repo", gh.Repo)

	comments, err := github.GetPRComments(gh.Owner, gh.Repo, gh.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR comments for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully fetched PR comments", "session", i.Title, "count", len(comments))

	return comments, nil
}

// GetPRDiff fetches the diff for the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRDiff() (string, error) {
	if !i.IsPRSession() {
		return "", fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	gh := i.GitHub()
	log.Info("fetching PR diff", "session", i.Title, "pr", gh.PRNumber, "owner", gh.Owner, "repo", gh.Repo)

	diff, err := github.GetPRDiff(gh.Owner, gh.Repo, gh.PRNumber)
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR diff for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully fetched PR diff", "session", i.Title, "bytes", len(diff))

	return diff, nil
}

// PostComment posts a comment to the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) PostComment(body string) error {
	if !i.IsPRSession() {
		return fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	if body == "" {
		return fmt.Errorf("comment body cannot be empty")
	}

	gh := i.GitHub()
	log.Info("posting comment to PR", "pr", gh.PRNumber, "session", i.Title)

	if err := github.PostPRComment(gh.Owner, gh.Repo, gh.PRNumber, body); err != nil {
		return fmt.Errorf("failed to post comment to PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully posted comment to PR", "pr", gh.PRNumber, "session", i.Title)

	return nil
}

// SetCommitStatus posts a commit status to the PR's current head commit via the GitHub
// Statuses API. It re-fetches PR info first so the status always lands on the current
// HEAD SHA rather than a stale one cached before a force-push or rebase.
// Returns an error if this is not a PR session or if the GitHub API call fails.
func (i *Instance) SetCommitStatus(state github.CommitStatusState, statusContext, description string) error {
	if !i.IsPRSession() {
		return fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	prInfo, err := i.RefreshPRInfo()
	if err != nil {
		return err
	}

	req, err := github.NewCommitStatusRequest(prInfo.HeadSHA, state, statusContext, description)
	if err != nil {
		return fmt.Errorf("invalid commit status request for instance '%s': %w", i.Title, err)
	}

	gh := i.GitHub()
	repo, err := github.NewRepoRef(gh.Owner, gh.Repo)
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}

	log.Info("setting commit status on PR", "pr", gh.PRNumber, "session", i.Title, "sha", prInfo.HeadSHA, "state", state, "context", statusContext)

	if err := github.SetCommitStatus(repo, req); err != nil {
		return fmt.Errorf("failed to set commit status for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully set commit status on PR", "pr", gh.PRNumber, "session", i.Title, "sha", prInfo.HeadSHA, "state", state)

	return nil
}

// MergePR merges the PR using the specified merge method
// method can be: "merge", "squash", or "rebase"
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) MergePR(method string) error {
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

	gh := i.GitHub()
	log.Info("merging PR", "pr", gh.PRNumber, "session", i.Title, "method", method)

	if err := github.MergePR(gh.Owner, gh.Repo, gh.PRNumber, method); err != nil {
		return fmt.Errorf("failed to merge PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully merged PR", "pr", gh.PRNumber, "session", i.Title)

	return nil
}

// ClosePR closes the PR without merging
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) ClosePR() error {
	if !i.IsPRSession() {
		return fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	gh := i.GitHub()
	log.Info("closing PR without merging", "pr", gh.PRNumber, "session", i.Title)

	if err := github.ClosePR(gh.Owner, gh.Repo, gh.PRNumber); err != nil {
		return fmt.Errorf("failed to close PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully closed PR", "pr", gh.PRNumber, "session", i.Title)

	return nil
}

// GeneratePRContextPrompt generates a context prompt for Claude based on PR information
// This can be used to initialize a Claude Code session with comprehensive PR context
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GeneratePRContextPrompt() (string, error) {
	if !i.IsPRSession() {
		return "", fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	gh := i.GitHub()
	log.Info("generating PR context prompt", "session", i.Title, "pr", gh.PRNumber, "owner", gh.Owner, "repo", gh.Repo)

	// Fetch PR information
	prInfo, err := i.RefreshPRInfo()
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR info for context prompt: %w", err)
	}

	// Generate prompt with PR description
	prompt := github.GeneratePRPrompt(prInfo, true)

	log.Info("successfully generated PR context prompt", "session", i.Title, "bytes", len(prompt))

	return prompt, nil
}
