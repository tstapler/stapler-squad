package vc

import (
	"fmt"
	"os/exec"
	"strings"
)

// JujutsuProvider implements VCSProvider for Jujutsu repositories
type JujutsuProvider struct {
	workDir  string
	repoRoot string
}

// NewJujutsuProvider creates a new Jujutsu provider for the given directory
func NewJujutsuProvider(path string) (*JujutsuProvider, error) {
	root, err := FindVCSRoot(path, VCSJujutsu)
	if err != nil {
		return nil, fmt.Errorf("not a jujutsu repository: %w", err)
	}
	return &JujutsuProvider{
		workDir:  path,
		repoRoot: root,
	}, nil
}

func (j *JujutsuProvider) Type() VCSType {
	return VCSJujutsu
}

func (j *JujutsuProvider) Name() string {
	return "Jujutsu"
}

func (j *JujutsuProvider) WorkDir() string {
	return j.workDir
}

// runJJ executes a jj command and returns the output
func (j *JujutsuProvider) runJJ(args ...string) (string, error) {
	// Use --no-pager to prevent interactive output
	cmd := exec.Command("jj", append([]string{"--no-pager", "-R", j.repoRoot}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("jj %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (j *JujutsuProvider) GetStatus() (*VCSStatus, error) {
	status := &VCSStatus{
		Type: VCSJujutsu,
	}

	// Get current change ID and description
	// Using template for machine-readable output
	if output, err := j.runJJ("log", "-r", "@", "--no-graph", "-T", `change_id.short() ++ "\n" ++ description.first_line()`); err == nil {
		lines := strings.SplitN(output, "\n", 2)
		if len(lines) >= 1 {
			status.HeadCommit = lines[0]
		}
		if len(lines) >= 2 {
			status.Description = lines[1]
		}
	}

	// Get bookmarks (jj's equivalent of branches)
	if output, err := j.runJJ("log", "-r", "@", "--no-graph", "-T", `bookmarks`); err == nil {
		status.Branch = strings.TrimSpace(output)
		if status.Branch == "" {
			// No bookmark, show change ID as "branch"
			status.Branch = "@"
		}
	}

	// Get changed files
	files, err := j.GetChangedFiles()
	if err != nil {
		return status, err
	}

	// Categorize files - in jj, working copy changes are always "staged"
	// since jj auto-commits to the working copy commit
	for _, f := range files {
		switch {
		case f.Status == FileConflict:
			status.ConflictFiles = append(status.ConflictFiles, f)
			status.HasConflicts = true
		case f.Status == FileUntracked:
			status.UntrackedFiles = append(status.UntrackedFiles, f)
			status.HasUntracked = true
		default:
			// In jj, changes are in the working copy commit
			// We'll show them as "staged" since they're part of @
			status.StagedFiles = append(status.StagedFiles, f)
			status.HasStaged = true
		}
	}

	status.IsClean = !status.HasStaged && !status.HasUnstaged && !status.HasUntracked && !status.HasConflicts

	return status, nil
}

func (j *JujutsuProvider) GetBranch() (string, error) {
	// Get bookmarks on current change
	output, err := j.runJJ("log", "-r", "@", "--no-graph", "-T", `bookmarks`)
	if err != nil {
		return "@", nil // Default to @ if we can't get bookmarks
	}

	bookmarks := strings.TrimSpace(output)
	if bookmarks == "" {
		// Return change ID if no bookmarks
		if changeId, err := j.runJJ("log", "-r", "@", "--no-graph", "-T", `change_id.short()`); err == nil {
			return changeId, nil
		}
		return "@", nil
	}
	return bookmarks, nil
}

func (j *JujutsuProvider) GetChangedFiles() ([]FileChange, error) {
	// Get status output
	output, err := j.runJJ("status")
	if err != nil {
		return nil, err
	}

	if output == "" {
		return nil, nil
	}

	var files []FileChange
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip header lines
		if strings.HasPrefix(line, "Working copy changes:") ||
			strings.HasPrefix(line, "The working copy is clean") ||
			strings.HasPrefix(line, "Working copy :") ||
			strings.HasPrefix(line, "Parent commit:") {
			continue
		}

		// Parse status line: "M path" or "A path" etc.
		if len(line) >= 2 && line[1] == ' ' {
			statusChar := line[0]
			path := strings.TrimSpace(line[2:])

			files = append(files, FileChange{
				Path:     path,
				Status:   parseJJStatusChar(statusChar),
				IsStaged: true, // In jj, working copy changes are always in @
			})
		}
	}

	return files, nil
}

// parseJJStatusChar converts a jj status character to FileStatus
func parseJJStatusChar(c byte) FileStatus {
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
	default:
		return FileModified
	}
}

// Stage in jj is a no-op since changes are automatically included in @
func (j *JujutsuProvider) Stage(path string) error {
	// In jj, files are automatically tracked when modified
	// We can use 'jj file track' to explicitly track untracked files
	_, err := j.runJJ("file", "track", path)
	return err
}

func (j *JujutsuProvider) StageAll() error {
	// Track all untracked files
	_, err := j.runJJ("file", "track", ".")
	return err
}

// Unstage in jj means restoring a file to its parent's version
func (j *JujutsuProvider) Unstage(path string) error {
	// Restore file from parent
	_, err := j.runJJ("restore", "--from", "@-", path)
	return err
}

func (j *JujutsuProvider) UnstageAll() error {
	// Restore all files from parent
	_, err := j.runJJ("restore", "--from", "@-")
	return err
}

// Commit in jj creates a new empty change after the current one
func (j *JujutsuProvider) Commit(message string) error {
	// First describe the current change
	if message != "" {
		if _, err := j.runJJ("describe", "-m", message); err != nil {
			return err
		}
	}
	// Then create a new change
	_, err := j.runJJ("new")
	return err
}

// AmendCommit updates the current change's description
func (j *JujutsuProvider) AmendCommit(message string) error {
	if message == "" {
		// Open editor for description
		return ErrNotImplemented // Would need interactive terminal
	}
	_, err := j.runJJ("describe", "-m", message)
	return err
}

func (j *JujutsuProvider) Push() error {
	// Push current bookmark to remote
	_, err := j.runJJ("git", "push")
	return err
}

func (j *JujutsuProvider) PushWithOptions(opts PushOptions) error {
	args := []string{"git", "push"}

	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Branch != "" {
		args = append(args, "-b", opts.Branch)
	}
	if opts.Remote != "" {
		args = append(args, "--remote", opts.Remote)
	}

	_, err := j.runJJ(args...)
	return err
}

func (j *JujutsuProvider) Pull() error {
	// Fetch and rebase
	if _, err := j.runJJ("git", "fetch"); err != nil {
		return err
	}
	// Optionally rebase onto the fetched changes
	return nil
}

func (j *JujutsuProvider) Fetch() error {
	_, err := j.runJJ("git", "fetch")
	return err
}

func (j *JujutsuProvider) GetFileDiff(path string) (string, error) {
	// Use root: prefix to specify repo-relative paths, which works regardless of
	// the current working directory within the repo
	return j.runJJ("diff", "--", fmt.Sprintf("root:%s", path))
}

func (j *JujutsuProvider) GetDiff() (string, error) {
	return j.runJJ("diff")
}

func (j *JujutsuProvider) GetInteractiveCommand() string {
	// Check for available interactive tools
	if IsToolAvailable("lazyjj") {
		return "lazyjj"
	}
	// Fallback to jj log
	return "jj log"
}

func (j *JujutsuProvider) GetLogCommand() string {
	return "jj log"
}

// Jujutsu-specific methods

// Describe updates the description of a change
func (j *JujutsuProvider) Describe(message string) error {
	_, err := j.runJJ("describe", "-m", message)
	return err
}

// New creates a new change after the current one
func (j *JujutsuProvider) New() error {
	_, err := j.runJJ("new")
	return err
}

// Squash squashes the current change into its parent
func (j *JujutsuProvider) Squash() error {
	_, err := j.runJJ("squash")
	return err
}

// Edit starts editing a change
func (j *JujutsuProvider) Edit(changeID string) error {
	_, err := j.runJJ("edit", changeID)
	return err
}

// Abandon abandons the current change
func (j *JujutsuProvider) Abandon() error {
	_, err := j.runJJ("abandon")
	return err
}
