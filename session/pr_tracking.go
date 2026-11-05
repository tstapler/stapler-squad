package session

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// PR Tracking Methods for Instance
// These methods provide GitHub PR lifecycle management for sessions created from PR URLs

// resolvePR names the PR an action targets. A nil ref means the session's current
// PR (today the legacy scalars; the active PR once actions are PR-aware). It fails
// for a session that is not a PR session, so every action shares one check.
func (i *Instance) resolvePR(ref *PRRef) (PRRef, error) {
	if ref != nil {
		return *ref, nil
	}
	gh := i.GitHub()
	if !gh.IsPRSession() {
		return PRRef{}, fmt.Errorf("instance '%s' is not a PR session", i.Title)
	}
	resolved, err := NewPRRef(gh.Host, gh.Owner, gh.Repo, gh.PRNumber)
	if err != nil {
		return PRRef{}, fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	return resolved, nil
}

// repoRef converts a PRRef to the github package's repo reference. github.com is
// the empty host there, matching what the legacy scalars carried.
func (r PRRef) repoRef() (github.RepoRef, error) {
	host := r.host
	if github.IsGitHubCom(host) {
		host = ""
	}
	return github.NewRepoRefWithHost(r.owner, r.repo, host)
}

// RefreshPRInfo fetches the latest PR information from GitHub
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) RefreshPRInfo(ctx context.Context) (*github.PRInfo, error) {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return nil, err
	}

	log.Info("refreshing PR info", "session", i.Title, "pr", pr.Number(), "owner", pr.Owner(), "repo", pr.Repo())

	ref, err := pr.repoRef()
	if err != nil {
		return nil, fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	prInfo, err := github.GetPRInfoCtx(ctx, ref, pr.Number())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR info for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully refreshed PR info", "session", i.Title, "title", prInfo.Title, "state", prInfo.State)

	return prInfo, nil
}

// GetPRComments fetches all comments on the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRComments(ctx context.Context) ([]github.PRComment, error) {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return nil, err
	}

	log.Info("fetching PR comments", "session", i.Title, "pr", pr.Number(), "owner", pr.Owner(), "repo", pr.Repo())

	ref, err := pr.repoRef()
	if err != nil {
		return nil, fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	comments, err := github.GetPRComments(ctx, ref, pr.Number())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR comments for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully fetched PR comments", "session", i.Title, "count", len(comments))

	return comments, nil
}

// GetPRDiff fetches the diff for the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) GetPRDiff(ctx context.Context) (string, error) {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return "", err
	}

	log.Info("fetching PR diff", "session", i.Title, "pr", pr.Number(), "owner", pr.Owner(), "repo", pr.Repo())

	ref, err := pr.repoRef()
	if err != nil {
		return "", fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	diff, err := github.GetPRDiff(ctx, ref, pr.Number())
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR diff for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully fetched PR diff", "session", i.Title, "bytes", len(diff))

	return diff, nil
}

// PostComment posts a comment to the PR
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) PostComment(ctx context.Context, body string) error {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return err
	}

	if body == "" {
		return fmt.Errorf("comment body cannot be empty")
	}

	log.Info("posting comment to PR", "pr", pr.Number(), "session", i.Title)

	ref, err := pr.repoRef()
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	if err := github.PostPRComment(ctx, ref, pr.Number(), body); err != nil {
		return fmt.Errorf("failed to post comment to PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully posted comment to PR", "pr", pr.Number(), "session", i.Title)

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
	pr, err := i.resolvePR(nil)
	if err != nil {
		return err
	}

	prInfo, err := i.RefreshPRInfo(ctx)
	if err != nil {
		return err
	}

	req, err := github.NewCommitStatusRequest(prInfo.HeadSHA, state, statusContext, description)
	if err != nil {
		return fmt.Errorf("invalid commit status request for instance '%s': %w", i.Title, err)
	}

	repo, err := pr.repoRef()
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}

	log.Info("setting commit status on PR", "pr", pr.Number(), "session", i.Title, "sha", prInfo.HeadSHA, "state", state, "context", statusContext)

	if err := github.SetCommitStatus(repo, req); err != nil {
		return fmt.Errorf("failed to set commit status for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully set commit status on PR", "pr", pr.Number(), "session", i.Title, "sha", prInfo.HeadSHA, "state", state)

	return nil
}

// MergePR merges the PR using the specified merge method
// method can be: "merge", "squash", or "rebase"
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) MergePR(ctx context.Context, method string) error {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return err
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

	log.Info("merging PR", "pr", pr.Number(), "session", i.Title, "method", method)

	ref, err := pr.repoRef()
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	if err := github.MergePR(ctx, ref, pr.Number(), method); err != nil {
		return fmt.Errorf("failed to merge PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully merged PR", "pr", pr.Number(), "session", i.Title)

	return nil
}

// ClosePR closes the PR without merging
// Returns an error if this is not a PR session or if the GitHub API call fails
func (i *Instance) ClosePR(ctx context.Context) error {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return err
	}

	log.Info("closing PR without merging", "pr", pr.Number(), "session", i.Title)

	ref, err := pr.repoRef()
	if err != nil {
		return fmt.Errorf("invalid repo ref for instance '%s': %w", i.Title, err)
	}
	if err := github.ClosePR(ctx, ref, pr.Number()); err != nil {
		return fmt.Errorf("failed to close PR for instance '%s': %w", i.Title, err)
	}

	log.Info("successfully closed PR", "pr", pr.Number(), "session", i.Title)

	return nil
}

// GeneratePRContextPrompt generates a context prompt for Claude based on PR information
// This can be used to initialize a Claude Code session with comprehensive PR context
// Returns an error if this is not a PR session or if the GitHub API call fails
//
// Currently has no non-test callers; an untagged ctx inherits GitHubCallOriginFrom's
// background default until a caller exists.
func (i *Instance) GeneratePRContextPrompt(ctx context.Context) (string, error) {
	pr, err := i.resolvePR(nil)
	if err != nil {
		return "", err
	}

	log.Info("generating PR context prompt", "session", i.Title, "pr", pr.Number(), "owner", pr.Owner(), "repo", pr.Repo())

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
