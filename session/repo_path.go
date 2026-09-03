package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	gitutil "github.com/tstapler/stapler-squad/session/git"
)

const detectWorktreeCacheTTL = 5 * time.Minute

type detectWorktreeCacheEntry struct {
	info   *WorktreeInfo
	err    error
	expiry time.Time
}

var detectWorktreeCache = struct {
	sync.RWMutex
	m map[string]detectWorktreeCacheEntry
}{m: make(map[string]detectWorktreeCacheEntry)}

// RepoPathManager handles GOPATH-style repository path management.
// Repositories are stored in a consistent location based on their URL:
//   - ~/.stapler-squad/repos/github.com/owner/repo (main clone)
//   - Worktrees are created relative to the main repo as needed
type RepoPathManager struct {
	baseDir string // Base directory for repos (default: ~/.stapler-squad/repos)
}

// NewRepoPathManager creates a new RepoPathManager with the default base directory.
func NewRepoPathManager() *RepoPathManager {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = os.TempDir()
	}
	return &RepoPathManager{
		baseDir: filepath.Join(homeDir, ".stapler-squad", "repos"),
	}
}

// NewRepoPathManagerWithBase creates a RepoPathManager with a custom base directory.
func NewRepoPathManagerWithBase(baseDir string) *RepoPathManager {
	return &RepoPathManager{
		baseDir: baseDir,
	}
}

// GitHubRef represents a parsed GitHub reference.
type GitHubRef struct {
	Host     string // GitHub host, e.g. "github.com" or a GHES hostname; "" means github.com
	Owner    string
	Repo     string
	Branch   string
	PRNumber int
	Type     GitHubRefType
}

// PRURL returns the canonical URL of the PR this ref points at, or "" if
// PRNumber is not set (e.g. a plain repo or branch ref). This is the single
// source of truth for GitHub PR URL construction — session_service.go's
// CreateSession handler and the github_pr_url_backfill.go migration both call
// this instead of formatting the URL themselves, so a host-normalization or
// URL-shape fix only has to be made once.
func (r *GitHubRef) PRURL() string {
	if r == nil || r.PRNumber <= 0 {
		return ""
	}
	return fmt.Sprintf("https://%s/%s/%s/pull/%d",
		github.NormalizeHost(r.Host), r.Owner, r.Repo, r.PRNumber)
}

// GitHubRefType indicates what kind of GitHub reference this is.
type GitHubRefType int

const (
	GitHubRefTypeRepo GitHubRefType = iota
	GitHubRefTypeBranch
	GitHubRefTypePR
)

// isTraversalSegment reports whether s is "." or ".." — never valid as a
// GitHub owner or repo name, but syntactically accepted by the "no slash"
// regex character classes below. GetRepoPath joins Owner/Repo directly into a
// local filesystem path (filepath.Join(baseDir, "github.com", owner, repo)),
// so letting either through would allow a crafted repo_path (e.g.
// "https://github.com/../..") to resolve outside the intended clone
// directory.
func isTraversalSegment(s string) bool {
	return s == "." || s == ".."
}

// ParseGitHubURL parses a github.com URL and returns the components.
// Supported formats:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo/tree/branch
//   - https://github.com/owner/repo/pull/123
//   - owner/repo (shorthand)
//   - owner/repo:branch (shorthand with branch)
func ParseGitHubURL(input string) (*GitHubRef, error) {
	return ParseGitHubURLWithHosts(input, nil)
}

// ParseGitHubURLWithHosts parses a GitHub URL or shorthand, additionally
// recognizing URLs against any of the given GitHub Enterprise hostnames (in
// addition to github.com). Delegates the regex/host matching to the github
// package's ParseGitHubRefWithHosts, then re-validates Owner/Repo against
// isTraversalSegment: the github package's own parsing provides no
// path-traversal protection (its only validation, isValidGitHubName, is used
// solely for the shorthand path and explicitly permits "." and ".."), so this
// guard must be re-applied here before Owner/Repo are ever used to build a
// local filesystem path in GetRepoPath.
func ParseGitHubURLWithHosts(input string, enterpriseHosts []string) (*GitHubRef, error) {
	input = strings.TrimSpace(input)

	// Reject local-looking paths before shorthand parsing gets a chance at
	// them: "owner/repo" shorthand's regex can't distinguish a real shorthand
	// from a relative/absolute/home-relative local path (e.g. ".git/repo",
	// "/abs/path", "~/path") — both are just "segment/segment". Full URLs are
	// unaffected since they always start with "http://" or "https://".
	if strings.HasPrefix(input, "/") || strings.HasPrefix(input, "~") || strings.HasPrefix(input, ".") {
		return nil, fmt.Errorf("not a recognized GitHub URL format: %s", input)
	}

	parsed, err := github.ParseGitHubRefWithHosts(input, enterpriseHosts)
	if err != nil {
		return nil, fmt.Errorf("not a recognized GitHub URL format: %s", input)
	}

	if isTraversalSegment(parsed.Owner) || isTraversalSegment(parsed.Repo) {
		return nil, fmt.Errorf("invalid GitHub owner/repo: %s", input)
	}

	var refType GitHubRefType
	switch parsed.Type {
	case github.RefTypePR:
		refType = GitHubRefTypePR
	case github.RefTypeBranch:
		refType = GitHubRefTypeBranch
	case github.RefTypeRepo:
		refType = GitHubRefTypeRepo
	default:
		// File/Commit/Issue/Compare/Release refs aren't valid session-creation
		// inputs; treat them as unrecognized rather than silently misrouting.
		return nil, fmt.Errorf("not a recognized GitHub URL format: %s", input)
	}

	return &GitHubRef{
		Host:     parsed.Host,
		Owner:    parsed.Owner,
		Repo:     parsed.Repo,
		Branch:   parsed.Branch,
		PRNumber: parsed.PRNumber,
		Type:     refType,
	}, nil
}

// IsGitHubURL returns true if the input looks like a github.com URL or shorthand.
func IsGitHubURL(input string) bool {
	_, err := ParseGitHubURL(input)
	return err == nil
}

// IsGitHubURLWithHosts returns true if the input looks like a GitHub URL or
// shorthand, recognizing URLs against the given GitHub Enterprise hostnames
// in addition to github.com.
func IsGitHubURLWithHosts(input string, enterpriseHosts []string) bool {
	_, err := ParseGitHubURLWithHosts(input, enterpriseHosts)
	return err == nil
}

// repoHost returns the normalized host for ref, defaulting to "github.com"
// when Host is unset (github.com refs leave Host as the zero value).
func repoHost(ref *GitHubRef) string {
	if ref.Host == "" {
		return "github.com"
	}
	return ref.Host
}

// hostFromClonedPath attempts to recover a GitHub host from a session's
// persisted local path, for rows whose github_pr_url was never populated
// (see runGitHubPRURLBackfill). Paths for GitHub-backed sessions follow the
// GOPATH-style convention documented on RepoPathManager:
// <baseDir>/<host>/<owner>/<repo>[/...]. Returns "" if path doesn't contain
// an <owner>/<repo> segment pair immediately preceded by a host-shaped
// segment (contains a "." or is "localhost") — guessing "github.com" for a
// segment that isn't actually a hostname would produce an incorrect URL for
// GitHub Enterprise rows.
func hostFromClonedPath(path string, ref github.RepoRef) string {
	if path == "" || !ref.IsValid() {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i+1] != ref.Owner() || parts[i+2] != ref.Repo() {
			continue
		}
		host := parts[i]
		if host != "localhost" && !strings.Contains(host, ".") {
			continue
		}
		return host
	}
	return ""
}

// GetRepoPath returns the local path where a GitHub repo should be stored.
// Format: ~/.stapler-squad/repos/<host>/owner/repo
func (m *RepoPathManager) GetRepoPath(ref *GitHubRef) string {
	return filepath.Join(m.baseDir, repoHost(ref), ref.Owner, ref.Repo)
}

// GetCloneURL returns the git clone URL for a GitHub ref, injecting a stored
// keychain token for the ref's host when one is available (required for
// private repos on GitHub Enterprise hosts, where an unauthenticated clone
// would otherwise fail).
func (m *RepoPathManager) GetCloneURL(ref *GitHubRef) string {
	host := repoHost(ref)
	if token := github.GetKeychainTokenForHost(host); token != "" {
		return fmt.Sprintf("https://x-access-token:%s@%s/%s/%s.git", token, host, ref.Owner, ref.Repo)
	}
	return fmt.Sprintf("https://%s/%s/%s.git", host, ref.Owner, ref.Repo)
}

// sanitizeCloneOutput strips an embedded credential from git's own error output
// before it's wrapped into an error (which callers may log or surface to the
// UI). git commonly echoes the clone URL verbatim in messages like
// "fatal: repository 'https://x-access-token:<token>@host/owner/repo.git' not
// found" — cloneURL is only ever credential-bearing here, so a literal
// substring replace is sufficient without needing to parse URLs.
func sanitizeCloneOutput(output []byte, cloneURL string) string {
	if !strings.Contains(cloneURL, "@") {
		return string(output)
	}
	redacted := strings.Replace(cloneURL, cloneURL[len("https://"):strings.Index(cloneURL, "@")+1], "", 1)
	return strings.ReplaceAll(string(output), cloneURL, redacted)
}

// isCorruptedClone reports whether repoPath's .git directory exists but HEAD
// still points at the literal placeholder ref git's internal bootstrap phase
// writes before a `git clone` completes ("ref: refs/heads/.invalid", see
// builtin/clone.c's chicken-and-egg comment and refs.c's write_file call) —
// the signature left behind when the clone subprocess is killed (timeout,
// process kill, truncated network connection) in between. A directory in
// this state passes a naive ".git exists" check forever, so EnsureRepoCloned
// must detect it explicitly rather than trusting os.Stat.
//
// This deliberately checks the *raw* symbolic HEAD target rather than
// whether HEAD resolves: a brand-new, zero-commit repo (`git init`, no
// commits yet) also has an unresolvable HEAD — it symbolically points at
// "refs/heads/main" (or whatever the default branch is), which simply
// doesn't exist as a ref yet ("unborn branch"). That's a legitimate, healthy
// repo state, not corruption, and must not be misdiagnosed as an interrupted
// clone (which would then fail trying to read a nonexistent origin remote
// for re-clone instead of letting normal "create an initial commit" handling
// take over).
// Uses go-git per the `prefer-go-git-over-subshells` skill.
func isCorruptedClone(repoPath string) bool {
	repo, err := gitutil.OpenRepo(repoPath)
	if err != nil {
		return true
	}
	headRef, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return true
	}
	if headRef.Type() != plumbing.SymbolicReference {
		return false
	}
	return headRef.Target() == plumbing.ReferenceName("refs/heads/.invalid")
}

// RepairCorruptedGitRepo detects and repairs the .invalid-HEAD clone corruption
// (see isCorruptedClone) for repos reached via a plain on-disk path rather than
// through RepoPathManager.EnsureRepoCloned — e.g. a backlog item's stored
// RepoPath, which CreateBacklogWorktree resolves and hands straight to
// ResolveSessionPath (pure path expansion, no corruption check) and then to
// git.NewGitWorktreeFromCommitSHA/NewGitWorktreeWithBranch. Those never call
// EnsureRepoCloned, so a repo corrupted the same way EnsureRepoCloned guards
// against (an interrupted `git clone` leaving "ref: refs/heads/.invalid" as
// HEAD forever) would otherwise fail worktree creation indefinitely with no
// self-heal. If repoPath isn't a git repo at all, or its HEAD resolves fine,
// this is a no-op. Repair re-clones from the corrupted repo's own origin remote.
func RepairCorruptedGitRepo(repoPath string) error {
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err != nil {
		return nil
	}
	if !isCorruptedClone(repoPath) {
		return nil
	}
	originURL, err := readOriginRemoteURL(repoPath)
	if err != nil {
		return fmt.Errorf("repo at %s has corrupted HEAD (interrupted clone) and its origin remote could not be read for re-clone: %w", repoPath, err)
	}
	log.Warn("repository is corrupted (unresolvable HEAD, likely from an interrupted clone); removing and re-cloning", "path", repoPath)
	if err := os.RemoveAll(repoPath); err != nil {
		return fmt.Errorf("failed to remove corrupted repository at %s: %w", repoPath, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "git", "clone", originURL, repoPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to re-clone repository at %s: %w\nOutput: %s", repoPath, err, sanitizeCloneOutput(output, originURL))
	}
	if strings.Contains(originURL, "@") {
		publicURL := strings.Replace(originURL, originURL[strings.Index(originURL, "://")+3:strings.Index(originURL, "@")+1], "", 1)
		setURLCmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "remote", "set-url", "origin", publicURL)
		if output, err := setURLCmd.CombinedOutput(); err != nil {
			log.Warn("failed to strip credentials from remote url", "err", err, "output", string(output))
		}
	}
	log.Info("successfully re-cloned previously corrupted repository", "path", repoPath)
	return nil
}

// readOriginRemoteURL reads the origin remote's URL from an on-disk repo whose
// HEAD may be unresolvable — repo.Remote reads only .git/config, so this works
// even when isCorruptedClone(repoPath) is true.
func readOriginRemoteURL(repoPath string) (string, error) {
	repo, err := gitutil.OpenRepo(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repo: %w", err)
	}
	remote, err := repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to read origin remote: %w", err)
	}
	urls := remote.Config().URLs
	if len(urls) == 0 {
		return "", fmt.Errorf("origin remote has no URL")
	}
	return urls[0], nil
}

// EnsureRepoCloned ensures the repository is cloned to the local path.
// If already cloned, it fetches the latest changes.
// Returns the path to the cloned repository.
// ctx bounds the underlying git fetch/clone subprocess (via safeexec.CommandContext)
// with a hard per-operation timeout, so callers that cancel ctx actually kill the
// subprocess rather than merely abandoning the RPC while it keeps running.
func (m *RepoPathManager) EnsureRepoCloned(ctx context.Context, ref *GitHubRef) (string, error) {
	repoPath := m.GetRepoPath(ref)
	cloneURL := m.GetCloneURL(ref)

	// Check if repo already exists
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		if isCorruptedClone(repoPath) {
			// Left behind by a previously interrupted clone (see
			// isCorruptedClone) — HEAD will never resolve on its own, and
			// every future fetch against this directory would keep
			// "succeeding" without ever fixing it. Remove and re-clone below
			// rather than handing back a repo no worktree can be built from.
			log.Warn("repository is corrupted (unresolvable HEAD, likely from an interrupted clone); removing and re-cloning", "path", repoPath)
			if rmErr := os.RemoveAll(repoPath); rmErr != nil {
				return "", fmt.Errorf("failed to remove corrupted repository at %s: %w", repoPath, rmErr)
			}
		} else {
			// Repo exists, fetch latest
			log.Info("repository exists, fetching latest", "path", repoPath)
			fetchCtx, fetchCancel := context.WithTimeout(ctx, 60*time.Second)
			defer fetchCancel()
			cmd := safeexec.CommandContext(fetchCtx, "git", "-C", repoPath, "fetch", "--all", "--prune")
			if output, err := cmd.CombinedOutput(); err != nil {
				// Only propagate when the caller's own ctx (not the local 60s
				// fetchCtx timeout) is what killed the fetch — that means the
				// caller's deadline was actually hit and callers like
				// CreateSession need to see that as an error rather than
				// silently proceeding with a possibly-stale repo. A fetch that
				// merely fails on its own (network blip, no connectivity) still
				// falls through to "existing repo is still usable" below.
				if ctx.Err() != nil {
					return "", fmt.Errorf("failed to fetch repository: %w", ctx.Err())
				}
				log.Warn("failed to fetch repository", "err", err, "output", string(output))
				// Don't fail - the existing repo is still usable
			}
			return repoPath, nil
		}
	}

	// Create parent directory
	parentDir := filepath.Dir(repoPath)
	if err := os.MkdirAll(parentDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", parentDir, err)
	}

	// Clone the repository. cloneURL may carry an embedded keychain token
	// (see GetCloneURL) — never log it verbatim, and reset the remote's URL
	// back to a credential-free form after cloning so the token isn't left
	// sitting in the resulting .git/config indefinitely.
	host := repoHost(ref)
	log.Info("cloning repository", "host", host, "owner", ref.Owner, "repo", ref.Repo, "path", repoPath)
	cloneCtx, cloneCancel := context.WithTimeout(ctx, 120*time.Second)
	defer cloneCancel()
	cmd := safeexec.CommandContext(cloneCtx, "git", "clone", cloneURL, repoPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		// A killed/timed-out clone leaves a partially-initialized .git
		// directory behind (see isCorruptedClone) — remove it so the next
		// call re-clones from scratch instead of finding and reusing a
		// broken repo forever.
		if rmErr := os.RemoveAll(repoPath); rmErr != nil {
			log.Warn("failed to remove partial clone directory after failed clone", "path", repoPath, "err", rmErr)
		}
		return "", fmt.Errorf("failed to clone repository: %w\nOutput: %s", err, sanitizeCloneOutput(output, cloneURL))
	}

	if strings.Contains(cloneURL, "@") {
		publicURL := fmt.Sprintf("https://%s/%s/%s.git", host, ref.Owner, ref.Repo)
		setURLCmd := safeexec.CommandContext(ctx, "git", "-C", repoPath, "remote", "set-url", "origin", publicURL)
		if output, err := setURLCmd.CombinedOutput(); err != nil {
			log.Warn("failed to strip credentials from remote url", "err", err, "output", string(output))
		}
	}

	log.Info("successfully cloned repository", "owner", ref.Owner, "repo", ref.Repo)
	return repoPath, nil
}

// ResolveGitHubInput takes a GitHub URL/shorthand and returns a resolved path.
// It clones the repo if necessary and returns the local path.
// Also returns the parsed GitHubRef for storing metadata.
func (m *RepoPathManager) ResolveGitHubInput(input string) (localPath string, ref *GitHubRef, err error) {
	ref, err = ParseGitHubURL(input)
	if err != nil {
		return "", nil, err
	}

	localPath, err = m.EnsureRepoCloned(context.Background(), ref)
	if err != nil {
		return "", nil, err
	}

	return localPath, ref, nil
}

// ResolveGitHubInputCtx takes a GitHub URL/shorthand and returns a resolved path,
// threading ctx down to EnsureRepoCloned so the underlying git clone/fetch
// subprocess is actually cancelled if ctx is cancelled or times out.
func (m *RepoPathManager) ResolveGitHubInputCtx(ctx context.Context, input string) (localPath string, ref *GitHubRef, err error) {
	ref, err = ParseGitHubURL(input)
	if err != nil {
		return "", nil, err
	}

	localPath, err = m.EnsureRepoCloned(ctx, ref)
	if err != nil {
		return "", nil, err
	}

	return localPath, ref, nil
}

// ResolveGitHubInputCtxWithHosts takes a GitHub URL/shorthand and returns a
// resolved path, recognizing URLs against the given GitHub Enterprise
// hostnames in addition to github.com, and threading ctx down to
// EnsureRepoCloned so the underlying git clone/fetch subprocess is actually
// cancelled if ctx is cancelled or times out.
func (m *RepoPathManager) ResolveGitHubInputCtxWithHosts(ctx context.Context, input string, enterpriseHosts []string) (localPath string, ref *GitHubRef, err error) {
	ref, err = ParseGitHubURLWithHosts(input, enterpriseHosts)
	if err != nil {
		return "", nil, err
	}

	localPath, err = m.EnsureRepoCloned(ctx, ref)
	if err != nil {
		return "", nil, err
	}

	return localPath, ref, nil
}

// DefaultRepoPathManager is the default instance used for GitHub URL resolution.
var DefaultRepoPathManager = NewRepoPathManager()

// ResolveGitHubInput is a convenience function using the default manager.
func ResolveGitHubInput(input string) (localPath string, ref *GitHubRef, err error) {
	return DefaultRepoPathManager.ResolveGitHubInput(input)
}

// ResolveGitHubInputCtx is a convenience function using the default manager,
// threading ctx down to the underlying git clone/fetch subprocess.
func ResolveGitHubInputCtx(ctx context.Context, input string) (localPath string, ref *GitHubRef, err error) {
	return DefaultRepoPathManager.ResolveGitHubInputCtx(ctx, input)
}

// ResolveGitHubInputCtxWithHosts is a convenience function using the default
// manager, recognizing URLs against the given GitHub Enterprise hostnames in
// addition to github.com.
func ResolveGitHubInputCtxWithHosts(ctx context.Context, input string, enterpriseHosts []string) (localPath string, ref *GitHubRef, err error) {
	return DefaultRepoPathManager.ResolveGitHubInputCtxWithHosts(ctx, input, enterpriseHosts)
}

// WorktreeInfo contains information about a git worktree
type WorktreeInfo struct {
	// IsWorktree is true if the path is a git worktree (not the main repo)
	IsWorktree bool
	// MainRepoPath is the path to the main repository's .git directory
	// For a worktree at ~/.stapler-squad/worktrees/foo, this might be /path/to/main/repo/.git
	MainRepoPath string
	// MainRepoRoot is the working directory root of the main repository
	MainRepoRoot string
	// RemoteURL is the git remote origin URL (e.g., https://github.com/owner/repo.git)
	RemoteURL string
	// GitHubOwner is the owner extracted from a GitHub remote URL
	GitHubOwner string
	// GitHubRepo is the repo name extracted from a GitHub remote URL
	GitHubRepo string
}

// DetectWorktree checks if the given path is a git worktree and extracts relevant info.
// Results are cached per-path for 5 minutes to avoid repeated git subprocess calls on
// every LoadInstances invocation for sessions whose GitHubOwner was never resolved.
// Returns WorktreeInfo with IsWorktree=false if it's not a worktree or not a git repo.
func DetectWorktree(path string) (*WorktreeInfo, error) {
	now := time.Now()
	detectWorktreeCache.RLock()
	if entry, ok := detectWorktreeCache.m[path]; ok && now.Before(entry.expiry) {
		detectWorktreeCache.RUnlock()
		return entry.info, entry.err
	}
	detectWorktreeCache.RUnlock()

	info, err := detectWorktreeUncached(path)

	detectWorktreeCache.Lock()
	detectWorktreeCache.m[path] = detectWorktreeCacheEntry{info: info, err: err, expiry: now.Add(detectWorktreeCacheTTL)}
	detectWorktreeCache.Unlock()

	return info, err
}

func detectWorktreeUncached(path string) (*WorktreeInfo, error) {
	info := &WorktreeInfo{}

	// Check if .git exists
	gitPath := filepath.Join(path, ".git")
	stat, err := os.Stat(gitPath)
	if err != nil {
		// Not a git repository
		return info, nil
	}

	// If .git is a file (not a directory), it's a worktree
	// The file contains: gitdir: /path/to/main/repo/.git/worktrees/name
	if !stat.IsDir() {
		info.IsWorktree = true

		// Read the .git file to get the gitdir path
		content, err := os.ReadFile(gitPath)
		if err != nil {
			return info, fmt.Errorf("failed to read .git file: %w", err)
		}

		// Parse: gitdir: /path/to/main/repo/.git/worktrees/name
		gitdirLine := strings.TrimSpace(string(content))
		if strings.HasPrefix(gitdirLine, "gitdir: ") {
			gitdir := strings.TrimPrefix(gitdirLine, "gitdir: ")
			// Extract the main repo .git path (remove /worktrees/name suffix)
			if idx := strings.Index(gitdir, "/.git/worktrees/"); idx != -1 {
				info.MainRepoPath = gitdir[:idx+5] // Include "/.git"
				info.MainRepoRoot = gitdir[:idx]
			}
		}
	}

	// Get the remote URL using git command (works for both worktrees and main repos)
	remoteCtx, remoteCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer remoteCancel()
	cmd := safeexec.CommandContext(remoteCtx, "git", "-C", path, "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err == nil {
		info.RemoteURL = strings.TrimSpace(string(output))
		// Try to parse GitHub owner/repo from the URL
		info.GitHubOwner, info.GitHubRepo = parseGitHubRemoteURL(info.RemoteURL)
	}

	return info, nil
}

// parseGitHubRemoteURL extracts owner and repo from various GitHub URL formats.
// Supports:
//   - https://github.com/owner/repo.git
//   - https://github.com/owner/repo
//   - git@github.com:owner/repo.git
//   - git@github.com:owner/repo
func parseGitHubRemoteURL(url string) (owner, repo string) {
	url = strings.TrimSpace(url)

	// HTTPS format: https://github.com/owner/repo.git
	httpsPattern := regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)
	if match := httpsPattern.FindStringSubmatch(url); match != nil {
		return match[1], match[2]
	}

	// SSH format: git@github.com:owner/repo.git
	sshPattern := regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
	if match := sshPattern.FindStringSubmatch(url); match != nil {
		return match[1], match[2]
	}

	return "", ""
}

// GetMainRepoPath uses git rev-parse --git-common-dir to get the main repo path.
// This is more reliable than parsing the .git file.
func GetMainRepoPath(path string) (string, error) {
	mainCtx, mainCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer mainCancel()
	cmd := safeexec.CommandContext(mainCtx, "git", "-C", path, "rev-parse", "--git-common-dir")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git common dir: %w", err)
	}

	gitCommonDir := strings.TrimSpace(string(output))

	// If it's an absolute path, use it directly
	if filepath.IsAbs(gitCommonDir) {
		// Remove the .git suffix to get the repo root
		if strings.HasSuffix(gitCommonDir, "/.git") || strings.HasSuffix(gitCommonDir, "\\.git") {
			return filepath.Dir(gitCommonDir), nil
		}
		return gitCommonDir, nil
	}

	// If relative, resolve it from the worktree path
	absPath := filepath.Join(path, gitCommonDir)
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Remove the .git suffix
	if strings.HasSuffix(absPath, "/.git") || strings.HasSuffix(absPath, "\\.git") {
		return filepath.Dir(absPath), nil
	}
	return absPath, nil
}

// WorkspaceKey returns a canonical identity for the repo/workspace a session belongs to,
// used to group sibling worktrees/branches of the same repo as peers. Prefers the GitHub
// owner/repo (stable across worktree paths); falls back to MainRepoPath, then Path.
// Returns "" when none are set (e.g. a bare one-off session with no git remote).
func WorkspaceKey(githubOwner, githubRepo, mainRepoPath, path string) string {
	if githubOwner != "" && githubRepo != "" {
		return "gh:" + strings.ToLower(githubOwner) + "/" + strings.ToLower(githubRepo)
	}
	if mainRepoPath != "" {
		return "path:" + mainRepoPath
	}
	if path != "" {
		return "path:" + path
	}
	return ""
}

// WorkspaceKey returns this instance's workspace identity. See the package-level
// WorkspaceKey function for the derivation rules.
func (i *Instance) WorkspaceKey() string {
	return WorkspaceKey(i.GitHubOwner, i.GitHubRepo, i.MainRepoPath, i.Path)
}

// WorkspaceKey returns this instance data's workspace identity. See the package-level
// WorkspaceKey function for the derivation rules.
func (d InstanceData) WorkspaceKey() string {
	return WorkspaceKey(d.GitHubOwner, d.GitHubRepo, d.MainRepoPath, d.Path)
}
