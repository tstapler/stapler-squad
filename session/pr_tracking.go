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

	log.InfoLog.Printf("Refreshing PR info for instance '%s' (PR #%d on %s/%s)",
		i.Title, i.GitHubPRNumber, i.GitHubOwner, i.GitHubRepo)

	prInfo, err := github.GetPRInfo(i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR info for instance '%s': %w", i.Title, err)
	}

	log.InfoLog.Printf("Successfully refreshed PR info for instance '%s': %s (state: %s)",
		i.Title, prInfo.Title, prInfo.State)

	return prInfo, nil
}

// GetPRComments fetches all comments on the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRComments() ([]github.PRComment, error) {
	if !i.IsPRSession() {
		return nil, fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.InfoLog.Printf("Fetching PR comments for instance '%s' (PR #%d on %s/%s)",
		i.Title, i.GitHubPRNumber, i.GitHubOwner, i.GitHubRepo)

	comments, err := github.GetPRComments(i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR comments for instance '%s': %w", i.Title, err)
	}

	log.InfoLog.Printf("Successfully fetched %d comments for instance '%s'", len(comments), i.Title)

	return comments, nil
}

// GetPRDiff fetches the diff for the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRDiff() (string, error) {
	if !i.IsPRSession() {
		return "", fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.InfoLog.Printf("Fetching PR diff for instance '%s' (PR #%d on %s/%s)",
		i.Title, i.GitHubPRNumber, i.GitHubOwner, i.GitHubRepo)

	diff, err := github.GetPRDiff(i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber)
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR diff for instance '%s': %w", i.Title, err)
	}

	log.InfoLog.Printf("Successfully fetched PR diff for instance '%s' (%d bytes)", i.Title, len(diff))

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

	log.InfoLog.Printf("Posting comment to PR #%d for instance '%s'", i.GitHubPRNumber, i.Title)

	if err := github.PostPRComment(i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber, body); err != nil {
		return fmt.Errorf("failed to post comment to PR for instance '%s': %w", i.Title, err)
	}

	log.InfoLog.Printf("Successfully posted comment to PR #%d for instance '%s'", i.GitHubPRNumber, i.Title)

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

	log.InfoLog.Printf("Merging PR #%d for instance '%s' using method '%s'",
		i.GitHubPRNumber, i.Title, method)

	if err := github.MergePR(i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber, method); err != nil {
		return fmt.Errorf("failed to merge PR for instance '%s': %w", i.Title, err)
	}

	log.InfoLog.Printf("Successfully merged PR #%d for instance '%s'", i.GitHubPRNumber, i.Title)

	return nil
}

// ClosePR closes the PR without merging
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) ClosePR() error {
	if !i.IsPRSession() {
		return fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.InfoLog.Printf("Closing PR #%d for instance '%s' without merging",
		i.GitHubPRNumber, i.Title)

	if err := github.ClosePR(i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber); err != nil {
		return fmt.Errorf("failed to close PR for instance '%s': %w", i.Title, err)
	}

	log.InfoLog.Printf("Successfully closed PR #%d for instance '%s'", i.GitHubPRNumber, i.Title)

	return nil
}

// GeneratePRContextPrompt generates a context prompt for Claude based on PR information
// This can be used to initialize a Claude Code session with comprehensive PR context
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GeneratePRContextPrompt() (string, error) {
	if !i.IsPRSession() {
		return "", fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}

	log.InfoLog.Printf("Generating PR context prompt for instance '%s' (PR #%d on %s/%s)",
		i.Title, i.GitHubPRNumber, i.GitHubOwner, i.GitHubRepo)

	// Fetch PR information
	prInfo, err := i.RefreshPRInfo()
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR info for context prompt: %w", err)
	}

	// Generate prompt with PR description
	prompt := github.GeneratePRPrompt(prInfo, true)

	log.InfoLog.Printf("Successfully generated PR context prompt for instance '%s' (%d bytes)",
		i.Title, len(prompt))

	return prompt, nil
}
