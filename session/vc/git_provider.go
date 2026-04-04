package vc

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GitProvider implements VCSProvider for Git repositories
type GitProvider struct {
	workDir  string
	repoRoot string
}

// NewGitProvider creates a new Git provider for the given directory
func NewGitProvider(path string) (*GitProvider, error) {
	root, err := FindVCSRoot(path, VCSGit)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	return &GitProvider{
		workDir:  path,
		repoRoot: root,
	}, nil
}

func (g *GitProvider) Type() VCSType {
	return VCSGit
}

func (g *GitProvider) Name() string {
	return "Git"
}

func (g *GitProvider) WorkDir() string {
	return g.workDir
}

// runGit executes a git command and returns the output
func (g *GitProvider) runGit(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", g.repoRoot}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (g *GitProvider) GetStatus() (*VCSStatus, error) {
	status := &VCSStatus{
		Type: VCSGit,
	}

	// Get branch name
	branch, err := g.GetBranch()
	if err == nil {
		status.Branch = branch
	}

	// Get HEAD commit (short)
	if head, err := g.runGit("rev-parse", "--short", "HEAD"); err == nil {
		status.HeadCommit = head
	}

	// Get last commit message
	if desc, err := g.runGit("log", "-1", "--format=%s"); err == nil {
		status.Description = desc
	}

	// Get ahead/behind info
	if branch != "" {
		if output, err := g.runGit("rev-list", "--left-right", "--count", branch+"...@{upstream}"); err == nil {
			parts := strings.Fields(output)
			if len(parts) == 2 {
				status.AheadBy, _ = strconv.Atoi(parts[0])
				status.BehindBy, _ = strconv.Atoi(parts[1])
			}
		}
		// Get upstream name
		if upstream, err := g.runGit("rev-parse", "--abbrev-ref", branch+"@{upstream}"); err == nil {
			status.Upstream = upstream
		}
	}

	// Get changed files using porcelain v2 format
	files, err := g.GetChangedFiles()
	if err != nil {
		return status, err
	}

	// Categorize files
	for _, f := range files {
		switch {
		case f.Status == FileConflict:
			status.ConflictFiles = append(status.ConflictFiles, f)
			status.HasConflicts = true
		case f.Status == FileUntracked:
			status.UntrackedFiles = append(status.UntrackedFiles, f)
			status.HasUntracked = true
		case f.IsStaged:
			status.StagedFiles = append(status.StagedFiles, f)
			status.HasStaged = true
		default:
			status.UnstagedFiles = append(status.UnstagedFiles, f)
			status.HasUnstaged = true
		}
	}

	status.IsClean = !status.HasStaged && !status.HasUnstaged && !status.HasUntracked && !status.HasConflicts

	return status, nil
}

func (g *GitProvider) GetBranch() (string, error) {
	output, err := g.runGit("branch", "--show-current")
	if err != nil {
		// Might be in detached HEAD state
		if output, err := g.runGit("rev-parse", "--short", "HEAD"); err == nil {
			return "(detached: " + output + ")", nil
		}
		return "", err
	}
	if output == "" {
		// Detached HEAD
		if output, err := g.runGit("rev-parse", "--short", "HEAD"); err == nil {
			return "(detached: " + output + ")", nil
		}
	}
	return output, nil
}

func (g *GitProvider) GetChangedFiles() ([]FileChange, error) {
	// Use porcelain v2 for machine-readable output
	output, err := g.runGit("status", "--porcelain=v2", "--untracked-files=all")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	var files []FileChange
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Parse porcelain v2 format
		// Ordinary entries: 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
		// Renamed/copied: 2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <X><score> <path><tab><origPath>
		// Untracked: ? <path>
		// Ignored: ! <path>

		if strings.HasPrefix(line, "? ") {
			files = append(files, FileChange{
				Path:     line[2:],
				Status:   FileUntracked,
				IsStaged: false,
			})
			continue
		}

		if strings.HasPrefix(line, "! ") {
			// Ignored files - skip
			continue
		}

		if strings.HasPrefix(line, "1 ") || strings.HasPrefix(line, "2 ") {
			parts := strings.Fields(line)
			if len(parts) < 9 {
				continue
			}

			xy := parts[1] // XY status codes
			path := parts[8]

			// Handle renamed/copied entries
			if strings.HasPrefix(line, "2 ") {
				if idx := strings.Index(line, "\t"); idx != -1 {
					pathParts := strings.SplitN(line[idx+1:], "\t", 2)
					if len(pathParts) == 2 {
						path = pathParts[0]
					}
				}
			}

			// Parse XY status codes
			// X = status in index (staged), Y = status in worktree (unstaged)
			indexStatus := xy[0]
			worktreeStatus := xy[1]

			// Check for conflicts (unmerged entries)
			if indexStatus == 'U' || worktreeStatus == 'U' ||
				(indexStatus == 'A' && worktreeStatus == 'A') ||
				(indexStatus == 'D' && worktreeStatus == 'D') {
				files = append(files, FileChange{
					Path:     path,
					Status:   FileConflict,
					IsStaged: false,
				})
				continue
			}

			// Add staged change if present
			if indexStatus != '.' && indexStatus != ' ' {
				files = append(files, FileChange{
					Path:     path,
					Status:   parseGitStatusChar(indexStatus),
					IsStaged: true,
				})
			}

			// Add unstaged change if present
			if worktreeStatus != '.' && worktreeStatus != ' ' {
				files = append(files, FileChange{
					Path:     path,
					Status:   parseGitStatusChar(worktreeStatus),
					IsStaged: false,
				})
			}
		}

		// Handle unmerged entries (u <xy> ...)
		if strings.HasPrefix(line, "u ") {
			parts := strings.Fields(line)
			if len(parts) >= 11 {
				path := parts[10]
				files = append(files, FileChange{
					Path:     path,
					Status:   FileConflict,
					IsStaged: false,
				})
			}
		}
	}

	return files, nil
}

// parseGitStatusChar converts a git status character to FileStatus
func parseGitStatusChar(c byte) FileStatus {
	switch c {
	case 'M':
		return FileModified
	case 'A':
		return FileAdded
	case 'D':
		return FileDeleted
	case 'R':
		return FileRenamed
	case 'C':
		return FileCopied
	case '?':
		return FileUntracked
	case '!':
		return FileIgnored
	case 'U':
		return FileConflict
	default:
		return FileModified
	}
}

func (g *GitProvider) Stage(path string) error {
	_, err := g.runGit("add", "--", path)
	return err
}

func (g *GitProvider) StageAll() error {
	_, err := g.runGit("add", "-A")
	return err
}

func (g *GitProvider) Unstage(path string) error {
	_, err := g.runGit("restore", "--staged", "--", path)
	return err
}

func (g *GitProvider) UnstageAll() error {
	_, err := g.runGit("reset", "HEAD")
	return err
}

func (g *GitProvider) Commit(message string) error {
	_, err := g.runGit("commit", "-m", message)
	return err
}

func (g *GitProvider) AmendCommit(message string) error {
	if message == "" {
		_, err := g.runGit("commit", "--amend", "--no-edit")
		return err
	}
	_, err := g.runGit("commit", "--amend", "-m", message)
	return err
}

func (g *GitProvider) Push() error {
	_, err := g.runGit("push")
	return err
}

func (g *GitProvider) PushWithOptions(opts PushOptions) error {
	args := []string{"push"}

	if opts.Force {
		args = append(args, "--force")
	}
	if opts.SetUpstream {
		args = append(args, "--set-upstream")
	}
	if opts.Remote != "" {
		args = append(args, opts.Remote)
	}
	if opts.Branch != "" {
		args = append(args, opts.Branch)
	}

	_, err := g.runGit(args...)
	return err
}

func (g *GitProvider) Pull() error {
	_, err := g.runGit("pull")
	return err
}

func (g *GitProvider) Fetch() error {
	_, err := g.runGit("fetch")
	return err
}

func (g *GitProvider) GetFileDiff(path string) (string, error) {
	// Try staged first, then unstaged
	output, err := g.runGit("diff", "--cached", "--", path)
	if err == nil && output != "" {
		return output, nil
	}
	return g.runGit("diff", "--", path)
}

func (g *GitProvider) GetDiff() (string, error) {
	// Get both staged and unstaged changes
	staged, _ := g.runGit("diff", "--cached")
	unstaged, _ := g.runGit("diff")

	if staged != "" && unstaged != "" {
		return staged + "\n" + unstaged, nil
	}
	if staged != "" {
		return staged, nil
	}
	return unstaged, nil
}

func (g *GitProvider) GetInteractiveCommand() string {
	// Check for available interactive tools in order of preference
	if IsToolAvailable("lazygit") {
		return "lazygit"
	}
	if IsToolAvailable("tig") {
		return "tig"
	}
	if IsToolAvailable("gitui") {
		return "gitui"
	}
	// Fallback to git status in a pager
	return "git status"
}

func (g *GitProvider) GetLogCommand() string {
	if IsToolAvailable("tig") {
		return "tig"
	}
	return "git log --oneline --graph -20"
}
