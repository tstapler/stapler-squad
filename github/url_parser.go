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
	RefTypeFile    // File/blob URL
	RefTypeCommit  // Commit URL
	RefTypeIssue   // Issue URL
	RefTypeCompare // Compare URL (branch comparison)
	RefTypeRelease // Release/tag URL
)

func (t RefType) String() string {
	switch t {
	case RefTypePR:
		return "PR"
	case RefTypeBranch:
		return "Branch"
	case RefTypeRepo:
		return "Repository"
	case RefTypeFile:
		return "File"
	case RefTypeCommit:
		return "Commit"
	case RefTypeIssue:
		return "Issue"
	case RefTypeCompare:
		return "Compare"
	case RefTypeRelease:
		return "Release"
	default:
		return "Unknown"
	}
}

// ParsedGitHubRef represents a parsed GitHub URL or reference
type ParsedGitHubRef struct {
	Type        RefType
	Host        string // GitHub host, e.g. "github.com" or a GHES hostname; "" means github.com
	Owner       string
	Repo        string
	PRNumber    int    // Only populated for RefTypePR
	IssueNumber int    // Only populated for RefTypeIssue
	Branch      string // Populated for RefTypeBranch, or PR head branch after fetching
	CommitSHA   string // Only populated for RefTypeCommit
	FilePath    string // Only populated for RefTypeFile (path within repo)
	LineStart   int    // Only populated for RefTypeFile (optional line number)
	LineEnd     int    // Only populated for RefTypeFile (optional end line for range)
	BaseBranch  string // Only populated for RefTypeCompare (the base of comparison)
	HeadBranch  string // Only populated for RefTypeCompare (the head being compared)
	Tag         string // Only populated for RefTypeRelease
	OriginalURL string // The original input string
}

// CloneURL returns the HTTPS clone URL for the repository
func (p *ParsedGitHubRef) CloneURL() string {
	return fmt.Sprintf("%s/%s/%s.git", webBaseURLForHost(p.Host), p.Owner, p.Repo)
}

// HTMLURL returns the human-readable GitHub URL
func (p *ParsedGitHubRef) HTMLURL() string {
	base := webBaseURLForHost(p.Host)
	switch p.Type {
	case RefTypePR:
		return fmt.Sprintf("%s/%s/%s/pull/%d", base, p.Owner, p.Repo, p.PRNumber)
	case RefTypeBranch:
		return fmt.Sprintf("%s/%s/%s/tree/%s", base, p.Owner, p.Repo, p.Branch)
	case RefTypeFile:
		url := fmt.Sprintf("%s/%s/%s/blob/%s/%s", base, p.Owner, p.Repo, p.Branch, p.FilePath)
		if p.LineStart > 0 {
			if p.LineEnd > 0 && p.LineEnd != p.LineStart {
				url += fmt.Sprintf("#L%d-L%d", p.LineStart, p.LineEnd)
			} else {
				url += fmt.Sprintf("#L%d", p.LineStart)
			}
		}
		return url
	case RefTypeCommit:
		return fmt.Sprintf("%s/%s/%s/commit/%s", base, p.Owner, p.Repo, p.CommitSHA)
	case RefTypeIssue:
		return fmt.Sprintf("%s/%s/%s/issues/%d", base, p.Owner, p.Repo, p.IssueNumber)
	case RefTypeCompare:
		return fmt.Sprintf("%s/%s/%s/compare/%s...%s", base, p.Owner, p.Repo, p.BaseBranch, p.HeadBranch)
	case RefTypeRelease:
		return fmt.Sprintf("%s/%s/%s/releases/tag/%s", base, p.Owner, p.Repo, p.Tag)
	default:
		return fmt.Sprintf("%s/%s/%s", base, p.Owner, p.Repo)
	}
}

// DisplayName returns a human-readable name for the reference
func (p *ParsedGitHubRef) DisplayName() string {
	switch p.Type {
	case RefTypePR:
		return fmt.Sprintf("%s/%s#%d", p.Owner, p.Repo, p.PRNumber)
	case RefTypeBranch:
		return fmt.Sprintf("%s/%s:%s", p.Owner, p.Repo, p.Branch)
	case RefTypeFile:
		name := fmt.Sprintf("%s/%s:%s/%s", p.Owner, p.Repo, p.Branch, p.FilePath)
		if p.LineStart > 0 {
			if p.LineEnd > 0 && p.LineEnd != p.LineStart {
				name += fmt.Sprintf("#L%d-L%d", p.LineStart, p.LineEnd)
			} else {
				name += fmt.Sprintf("#L%d", p.LineStart)
			}
		}
		return name
	case RefTypeCommit:
		shortSHA := p.CommitSHA
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		return fmt.Sprintf("%s/%s@%s", p.Owner, p.Repo, shortSHA)
	case RefTypeIssue:
		return fmt.Sprintf("%s/%s#%d (issue)", p.Owner, p.Repo, p.IssueNumber)
	case RefTypeCompare:
		return fmt.Sprintf("%s/%s: %s...%s", p.Owner, p.Repo, p.BaseBranch, p.HeadBranch)
	case RefTypeRelease:
		return fmt.Sprintf("%s/%s@%s", p.Owner, p.Repo, p.Tag)
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
	case RefTypeFile:
		// Use the file name as part of the session name
		parts := strings.Split(p.FilePath, "/")
		fileName := parts[len(parts)-1]
		// Remove extension for cleaner name
		if idx := strings.LastIndex(fileName, "."); idx > 0 {
			fileName = fileName[:idx]
		}
		branch := strings.ReplaceAll(p.Branch, "/", "-")
		return fmt.Sprintf("%s-%s-%s", p.Repo, branch, fileName)
	case RefTypeCommit:
		shortSHA := p.CommitSHA
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		return fmt.Sprintf("%s-commit-%s", p.Repo, shortSHA)
	case RefTypeIssue:
		return fmt.Sprintf("issue-%d-%s", p.IssueNumber, p.Repo)
	case RefTypeCompare:
		base := strings.ReplaceAll(p.BaseBranch, "/", "-")
		head := strings.ReplaceAll(p.HeadBranch, "/", "-")
		return fmt.Sprintf("%s-%s-vs-%s", p.Repo, base, head)
	case RefTypeRelease:
		tag := strings.ReplaceAll(p.Tag, "/", "-")
		return fmt.Sprintf("%s-%s", p.Repo, tag)
	default:
		return p.Repo
	}
}

// RepoFullName returns "owner/repo" format
func (p *ParsedGitHubRef) RepoFullName() string {
	return fmt.Sprintf("%s/%s", p.Owner, p.Repo)
}

// URL pattern templates for parsing GitHub URLs. Each has a "%s" placeholder
// for a host alternation (e.g. "github\.com|github\.example\.com") and
// captures the matched host as group 1, followed by the original capture
// groups shifted by one.
const (
	// https://github.com/owner/repo/pull/123
	prURLTmpl = `^(?:https?://)?(%s)/([^/]+)/([^/]+)/pull/(\d+)(?:/.*)?$`

	// https://github.com/owner/repo/tree/branch-name
	branchURLTmpl = `^(?:https?://)?(%s)/([^/]+)/([^/]+)/tree/(.+)$`

	// https://github.com/owner/repo/blob/branch/path/to/file.go#L10-L20
	// Captures: host, owner, repo, branch, path, optional line info
	fileURLTmpl = `^(?:https?://)?(%s)/([^/]+)/([^/]+)/blob/([^/]+)/(.+?)(?:#L(\d+)(?:-L(\d+))?)?$`

	// https://github.com/owner/repo/commit/abc123def456
	commitURLTmpl = `^(?:https?://)?(%s)/([^/]+)/([^/]+)/commit/([a-fA-F0-9]+)(?:/.*)?$`

	// https://github.com/owner/repo/issues/42
	issueURLTmpl = `^(?:https?://)?(%s)/([^/]+)/([^/]+)/issues/(\d+)(?:/.*)?$`

	// https://github.com/owner/repo/compare/main...feature
	compareURLTmpl = `^(?:https?://)?(%s)/([^/]+)/([^/]+)/compare/([^.]+)\.\.\.(.+)$`

	// https://github.com/owner/repo/releases/tag/v1.0.0
	releaseURLTmpl = `^(?:https?://)?(%s)/([^/]+)/([^/]+)/releases/tag/(.+)$`

	// https://github.com/owner/repo or github.com/owner/repo
	repoURLTmpl = `^(?:https?://)?(%s)/([^/]+)/([^/]+?)(?:\.git)?/?$`

	// git@github.com:owner/repo.git
	sshURLTmpl = `^git@(%s):([^/]+)/([^/]+?)(?:\.git)?$`

	// ssh://git@github.com/owner/repo.git
	sshProtocolURLTmpl = `^ssh://git@(%s)/([^/]+)/([^/]+?)(?:\.git)?$`
)

// Shorthand patterns carry no host information (they never specify one), so
// they stay static and always resolve to the default host.
var (
	// owner/repo:branch
	shorthandBranchPattern = regexp.MustCompile(`^([^/:]+)/([^/:]+):(.+)$`)

	// owner/repo (no colon, no https)
	shorthandRepoPattern = regexp.MustCompile(`^([^/:]+)/([^/:]+)$`)
)

// hostAlternation builds a regex alternation of github.com plus any
// normalized, deduped extra hosts, each escaped for safe embedding in a
// regex pattern.
func hostAlternation(extraHosts []string) string {
	hosts := []string{regexp.QuoteMeta(defaultHost)}
	seen := map[string]bool{defaultHost: true}
	for _, h := range extraHosts {
		h = NormalizeHost(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		hosts = append(hosts, regexp.QuoteMeta(h))
	}
	return strings.Join(hosts, "|")
}

// ghPatterns is the set of compiled URL patterns for a given host alternation.
type ghPatterns struct {
	pr, branch, file, commit, issue, compare, release, repo, ssh, sshProtocol *regexp.Regexp
}

func compilePatterns(hostAlt string) *ghPatterns {
	return &ghPatterns{
		pr:          regexp.MustCompile(fmt.Sprintf(prURLTmpl, hostAlt)),
		branch:      regexp.MustCompile(fmt.Sprintf(branchURLTmpl, hostAlt)),
		file:        regexp.MustCompile(fmt.Sprintf(fileURLTmpl, hostAlt)),
		commit:      regexp.MustCompile(fmt.Sprintf(commitURLTmpl, hostAlt)),
		issue:       regexp.MustCompile(fmt.Sprintf(issueURLTmpl, hostAlt)),
		compare:     regexp.MustCompile(fmt.Sprintf(compareURLTmpl, hostAlt)),
		release:     regexp.MustCompile(fmt.Sprintf(releaseURLTmpl, hostAlt)),
		repo:        regexp.MustCompile(fmt.Sprintf(repoURLTmpl, hostAlt)),
		ssh:         regexp.MustCompile(fmt.Sprintf(sshURLTmpl, hostAlt)),
		sshProtocol: regexp.MustCompile(fmt.Sprintf(sshProtocolURLTmpl, hostAlt)),
	}
}

// ParseGitHubRef parses a GitHub URL or shorthand reference into a ParsedGitHubRef
// Supported formats:
//   - https://github.com/owner/repo/pull/123
//   - https://github.com/owner/repo/tree/branch-name
//   - https://github.com/owner/repo/blob/branch/path/to/file.go
//   - https://github.com/owner/repo/blob/branch/file.go#L10 (with line number)
//   - https://github.com/owner/repo/blob/branch/file.go#L10-L20 (with line range)
//   - https://github.com/owner/repo/commit/abc123
//   - https://github.com/owner/repo/issues/42
//   - https://github.com/owner/repo/compare/main...feature
//   - https://github.com/owner/repo/releases/tag/v1.0.0
//   - https://github.com/owner/repo
//   - github.com/owner/repo
//   - git@github.com:owner/repo.git (SSH)
//   - ssh://git@github.com/owner/repo (SSH protocol)
//   - owner/repo:branch-name
//   - owner/repo
func ParseGitHubRef(input string) (*ParsedGitHubRef, error) {
	return ParseGitHubRefWithHosts(input, nil)
}

// ParseGitHubRefWithHosts parses a GitHub URL or shorthand reference into a
// ParsedGitHubRef, matching either github.com or any of the supplied
// enterpriseHosts. The Host field of the returned ref is set to the matched
// host (or left empty for shorthand references, which default to github.com).
func ParseGitHubRefWithHosts(input string, enterpriseHosts []string) (*ParsedGitHubRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty input")
	}

	p := compilePatterns(hostAlternation(enterpriseHosts))

	// Try PR URL first
	if matches := p.pr.FindStringSubmatch(input); matches != nil {
		prNum, err := strconv.Atoi(matches[4])
		if err != nil {
			return nil, fmt.Errorf("invalid PR number: %w", err)
		}
		return &ParsedGitHubRef{
			Type:        RefTypePR,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			PRNumber:    prNum,
			OriginalURL: input,
		}, nil
	}

	// Try file/blob URL (must be before branch URL since it's more specific)
	if matches := p.file.FindStringSubmatch(input); matches != nil {
		ref := &ParsedGitHubRef{
			Type:        RefTypeFile,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			Branch:      matches[4],
			FilePath:    matches[5],
			OriginalURL: input,
		}
		// Parse optional line numbers
		if matches[6] != "" {
			lineStart, err := strconv.Atoi(matches[6])
			if err == nil {
				ref.LineStart = lineStart
			}
		}
		if matches[7] != "" {
			lineEnd, err := strconv.Atoi(matches[7])
			if err == nil {
				ref.LineEnd = lineEnd
			}
		}
		return ref, nil
	}

	// Try commit URL
	if matches := p.commit.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeCommit,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			CommitSHA:   matches[4],
			OriginalURL: input,
		}, nil
	}

	// Try issue URL
	if matches := p.issue.FindStringSubmatch(input); matches != nil {
		issueNum, err := strconv.Atoi(matches[4])
		if err != nil {
			return nil, fmt.Errorf("invalid issue number: %w", err)
		}
		return &ParsedGitHubRef{
			Type:        RefTypeIssue,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			IssueNumber: issueNum,
			OriginalURL: input,
		}, nil
	}

	// Try compare URL
	if matches := p.compare.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeCompare,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			BaseBranch:  matches[4],
			HeadBranch:  matches[5],
			OriginalURL: input,
		}, nil
	}

	// Try release URL
	if matches := p.release.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeRelease,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			Tag:         matches[4],
			OriginalURL: input,
		}, nil
	}

	// Try branch URL
	if matches := p.branch.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeBranch,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			Branch:      matches[4],
			OriginalURL: input,
		}, nil
	}

	// Try repo URL (full URL)
	if matches := p.repo.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeRepo,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			OriginalURL: input,
		}, nil
	}

	// Try SSH URL (git@github.com:owner/repo.git)
	if matches := p.ssh.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeRepo,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
			OriginalURL: input,
		}, nil
	}

	// Try SSH protocol URL (ssh://git@github.com/owner/repo)
	if matches := p.sshProtocol.FindStringSubmatch(input); matches != nil {
		return &ParsedGitHubRef{
			Type:        RefTypeRepo,
			Host:        matches[1],
			Owner:       matches[2],
			Repo:        matches[3],
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
	return IsGitHubRefWithHosts(input, nil)
}

// IsGitHubRefWithHosts is the host-aware variant of IsGitHubRef, additionally
// recognizing URLs against any of the supplied enterpriseHosts.
func IsGitHubRefWithHosts(input string, enterpriseHosts []string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}

	hosts := append([]string{defaultHost}, enterpriseHosts...)
	for _, h := range hosts {
		h = NormalizeHost(h)
		if h == "" {
			continue
		}
		if strings.Contains(input, h+"/") {
			return true
		}
		if strings.HasPrefix(input, "git@"+h+":") {
			return true
		}
		if strings.HasPrefix(input, "ssh://git@"+h+"/") {
			return true
		}
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
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}
