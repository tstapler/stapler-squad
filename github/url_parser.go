package github

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// RefType represents the type of GitHub reference parsed from a URL or shorthand
type RefType int

const (
	RefTypePR RefType = iota
	RefTypeBranch
	RefTypeRepo
)

func (t RefType) String() string {
	switch t {
	case RefTypePR:
		return "PR"
	case RefTypeBranch:
		return "Branch"
	case RefTypeRepo:
		return "Repository"
	default:
		return "Unknown"
	}
}

// ParsedGitHubRef represents a parsed GitHub URL or reference
type ParsedGitHubRef struct {
	Type        RefType
	Owner       string
	Repo        string
	PRNumber    int    // Only populated for RefTypePR
	Branch      string // Populated for RefTypeBranch, or PR head branch after fetching
	OriginalURL string // The original input string
}

// CloneURL returns the HTTPS clone URL for the repository
func (p *ParsedGitHubRef) CloneURL() string {
	return fmt.Sprintf("https://github.com/%s/%s.git", p.Owner, p.Repo)
}

// HTMLURL returns the human-readable GitHub URL
func (p *ParsedGitHubRef) HTMLURL() string {
	switch p.Type {
	case RefTypePR:
		return fmt.Sprintf("https://github.com/%s/%s/pull/%d", p.Owner, p.Repo, p.PRNumber)
	case RefTypeBranch:
		return fmt.Sprintf("https://github.com/%s/%s/tree/%s", p.Owner, p.Repo, p.Branch)
	default:
		return fmt.Sprintf("https://github.com/%s/%s", p.Owner, p.Repo)
	}
}

// DisplayName returns a human-readable name for the reference
func (p *ParsedGitHubRef) DisplayName() string {
	switch p.Type {
	case RefTypePR:
		return fmt.Sprintf("%s/%s#%d", p.Owner, p.Repo, p.PRNumber)
	case RefTypeBranch:
		return fmt.Sprintf("%s/%s:%s", p.Owner, p.Repo, p.Branch)
	default:
		return fmt.Sprintf("%s/%s", p.Owner, p.Repo)
	}
}

// SuggestedSessionName returns a suggested session name based on the reference
func (p *ParsedGitHubRef) SuggestedSessionName() string {
	switch p.Type {
	case RefTypePR:
		return fmt.Sprintf("pr-%d-%s", p.PRNumber, p.Repo)
	case RefTypeBranch:
		// Sanitize branch name for session name
		branch := strings.ReplaceAll(p.Branch, "/", "-")
		return fmt.Sprintf("%s-%s", p.Repo, branch)
	default:
		return p.Repo
	}
}

// RepoFullName returns "owner/repo" format
func (p *ParsedGitHubRef) RepoFullName() string {
	return fmt.Sprintf("%s/%s", p.Owner, p.Repo)
}

// Regex patterns for parsing GitHub URLs
var (
	// Full GitHub URL patterns
	// https://github.com/owner/repo/pull/123
	prURLPattern = regexp.MustCompile(`^(?:https?://)?github\.com/([^/]+)/([^/]+)/pull/(\d+)(?:/.*)?$`)

	// https://github.com/owner/repo/tree/branch-name
	branchURLPattern = regexp.MustCompile(`^(?:https?://)?github\.com/([^/]+)/([^/]+)/tree/(.+)$`)

	// https://github.com/owner/repo or github.com/owner/repo
	repoURLPattern = regexp.MustCompile(`^(?:https?://)?github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`)

	// Shorthand patterns
	// owner/repo:branch
	shorthandBranchPattern = regexp.MustCompile(`^([^/:]+)/([^/:]+):(.+)$`)

	// owner/repo (no colon, no https)
	shorthandRepoPattern = regexp.MustCompile(`^([^/:]+)/([^/:]+)$`)
)

// ParseGitHubRef parses a GitHub URL or shorthand reference into a ParsedGitHubRef
// Supported formats:
//   - https://github.com/owner/repo/pull/123
//   - https://github.com/owner/repo/tree/branch-name
//   - https://github.com/owner/repo
//   - github.com/owner/repo
//   - owner/repo:branch-name
//   - owner/repo
func ParseGitHubRef(input string) (*ParsedGitHubRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty input")
	}

	// Try PR URL first
	if matches := prURLPattern.FindStringSubmatch(input); matches != nil {
		prNum, err := strconv.Atoi(matches[3])
		if err != nil {
			return nil, fmt.Errorf("invalid PR number: %w", err)
		}
		return &ParsedGitHubRef{
			Type:        RefTypePR,
			Owner:       matches[1],
			Repo:        matches[2],
			PRNumber:    prNum,
			OriginalURL: input,
		}, nil
	}

	// Try branch URL
	if matches := branchURLPattern.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeBranch,
			Owner:       matches[1],
			Repo:        matches[2],
			Branch:      matches[3],
			OriginalURL: input,
		}, nil
	}

	// Try repo URL (full URL)
	if matches := repoURLPattern.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeRepo,
			Owner:       matches[1],
			Repo:        matches[2],
			OriginalURL: input,
		}, nil
	}

	// Try shorthand with branch (owner/repo:branch)
	if matches := shorthandBranchPattern.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeBranch,
			Owner:       matches[1],
			Repo:        matches[2],
			Branch:      matches[3],
			OriginalURL: input,
		}, nil
	}

	// Try shorthand repo (owner/repo)
	// Only match if it looks like a GitHub reference (contains exactly one slash)
	if matches := shorthandRepoPattern.FindStringSubmatch(input); matches != nil {
		// Additional validation: owner and repo should look valid
		owner, repo := matches[1], matches[2]
		if isValidGitHubName(owner) && isValidGitHubName(repo) {
			return &ParsedGitHubRef{
				Type:        RefTypeRepo,
				Owner:       owner,
				Repo:        repo,
				OriginalURL: input,
			}, nil
		}
	}

	return nil, fmt.Errorf("unrecognized GitHub URL or reference format: %s", input)
}

// IsGitHubRef checks if the input string looks like a GitHub URL or reference
// This is a quick check that doesn't validate the full format
func IsGitHubRef(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}

	// Check for explicit GitHub URLs
	if strings.Contains(input, "github.com/") {
		return true
	}

	// Check for shorthand formats (owner/repo or owner/repo:branch)
	// Must have exactly one "/" and optionally one ":"
	slashCount := strings.Count(input, "/")
	if slashCount == 1 {
		parts := strings.SplitN(input, "/", 2)
		if len(parts) == 2 && isValidGitHubName(parts[0]) {
			// Remove any :branch suffix for repo name validation
			repo := strings.SplitN(parts[1], ":", 2)[0]
			if isValidGitHubName(repo) {
				return true
			}
		}
	}

	return false
}

// isValidGitHubName checks if a string could be a valid GitHub username or repo name
func isValidGitHubName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	// GitHub names can contain alphanumeric, hyphens, and underscores
	// They can't start with a hyphen
	if name[0] == '-' {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
