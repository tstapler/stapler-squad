package session

import "fmt"

// GitHubMetadataView is a read-only value object for GitHub session metadata.
// Constructed by Instance.GitHub() from the underlying fields.
// This is intentionally a value type (not a pointer) for safe concurrent reads.
type GitHubMetadataView struct {
	PRNumber       int
	PRURL          string
	Owner          string
	Repo           string
	SourceRef      string
	ClonedRepoPath string
}

// IsPRSession returns true if this metadata represents a PR-based session.
func (gh GitHubMetadataView) IsPRSession() bool {
	return gh.PRNumber > 0
}

// RepoFullName returns "owner/repo" format, or empty string if either is missing.
func (gh GitHubMetadataView) RepoFullName() string {
	if gh.Owner == "" || gh.Repo == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", gh.Owner, gh.Repo)
}

// PRDisplayInfo returns human-readable PR description for UI display.
// Returns empty string if not a PR session.
func (gh GitHubMetadataView) PRDisplayInfo() string {
	if !gh.IsPRSession() {
		return ""
	}
	return fmt.Sprintf("PR #%d on %s", gh.PRNumber, gh.RepoFullName())
}

// IsGitHubSession returns true if owner and repo are both set.
func (gh GitHubMetadataView) IsGitHubSession() bool {
	return gh.Owner != "" && gh.Repo != ""
}

// IsEmpty returns true if no GitHub metadata is set.
func (gh GitHubMetadataView) IsEmpty() bool {
	return gh.PRNumber == 0 && gh.PRURL == "" && gh.Owner == "" && gh.Repo == ""
}
