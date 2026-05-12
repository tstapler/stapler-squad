package git

import (
	"context"
	"fmt"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func getWorktreeDirectory() (string, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "worktrees"), nil
}

// isDirtyCacheTTL is the duration for which a cached IsDirty result is considered fresh.
const isDirtyCacheTTL = 15 * time.Second

// GitWorktree manages git worktree operations for a session
type GitWorktree struct {
	// Path to the repository
	repoPath string
	// Path to the worktree
	worktreePath string
	// Name of the session
	sessionName string
	// Branch name for the worktree
	branchName string
	// Base commit hash for the worktree
	baseCommitSHA string
	// cmdExec is used to execute commands for this worktree.
	cmdExec executor.Executor

	// isDirty cache fields — protected by isDirtyCacheMu.
	isDirtyCacheMu   sync.RWMutex
	isDirtyCache     bool
	isDirtyCacheTime time.Time
}

// NewGitWorktreeFromCommitSHA creates a new GitWorktree that will branch from the given
// commitSHA when Setup() is called, instead of branching from the current HEAD.
// This is used by ForkFromCheckpoint to recreate the exact git state at checkpoint time.
func NewGitWorktreeFromCommitSHA(repoPath, sessionName, branchName, commitSHA string) (*GitWorktree, string, error) {
	if commitSHA == "" {
		return nil, "", fmt.Errorf("commitSHA must not be empty")
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		log.Error("git worktree path abs error, falling back to repoPath", "path", repoPath, "err", err)
		absPath = repoPath
	}

	resolvedRepoPath, err := findGitRepoRoot(absPath)
	if err != nil {
		return nil, "", err
	}

	worktreeDir, err := getWorktreeDirectory()
	if err != nil {
		return nil, "", err
	}

	sanitizedName := sanitizeBranchName(sessionName)
	worktreePath := filepath.Join(worktreeDir, sanitizedName)
	worktreePath = worktreePath + "_" + fmt.Sprintf("%x", time.Now().UnixNano())

	return &GitWorktree{
		repoPath:      resolvedRepoPath,
		sessionName:   sessionName,
		branchName:    branchName,
		worktreePath:  worktreePath,
		baseCommitSHA: commitSHA,
		cmdExec:       executor.MakeExecutor(),
	}, branchName, nil
}

func NewGitWorktreeFromStorage(repoPath string, worktreePath string, sessionName string, branchName string, baseCommitSHA string) *GitWorktree {
	return NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, sessionName, branchName, baseCommitSHA, nil)
}

// NewGitWorktreeFromStorageWithExecutor creates a GitWorktree from stored data with an optional executor.
// If cmdExec is nil, a default executor is used.
func NewGitWorktreeFromStorageWithExecutor(repoPath string, worktreePath string, sessionName string, branchName string, baseCommitSHA string, cmdExec executor.Executor) *GitWorktree {
	// Return nil if the worktree has no actual paths (empty/invalid worktree)
	if repoPath == "" && worktreePath == "" && branchName == "" {
		return nil
	}

	if cmdExec == nil {
		cmdExec = executor.MakeExecutor()
	}

	return &GitWorktree{
		repoPath:      repoPath,
		worktreePath:  worktreePath,
		sessionName:   sessionName,
		branchName:    branchName,
		baseCommitSHA: baseCommitSHA,
		cmdExec:       cmdExec,
	}
}

// NewGitWorktree creates a new GitWorktree instance
func NewGitWorktree(repoPath string, sessionName string) (tree *GitWorktree, branchname string, err error) {
	return NewGitWorktreeWithBranchAndExecutor(repoPath, sessionName, "", nil)
}

// NewGitWorktreeWithBranch creates a new GitWorktree instance with an optional custom branch name
func NewGitWorktreeWithBranch(repoPath string, sessionName string, customBranch string) (tree *GitWorktree, branchname string, err error) {
	return NewGitWorktreeWithBranchAndExecutor(repoPath, sessionName, customBranch, nil)
}

// NewGitWorktreeWithBranchAndExecutor creates a new GitWorktree with optional branch name and executor.
// If cmdExec is nil, a default executor is used.
func NewGitWorktreeWithBranchAndExecutor(repoPath string, sessionName string, customBranch string, cmdExec executor.Executor) (tree *GitWorktree, branchname string, err error) {
	if cmdExec == nil {
		cmdExec = executor.MakeExecutor()
	}

	cfg := config.LoadConfig()

	var branchName string
	if customBranch != "" {
		// Use the custom branch name directly
		branchName = customBranch
	} else {
		// Generate branch name from session name
		sanitizedName := sanitizeBranchName(sessionName)
		branchName = fmt.Sprintf("%s%s", cfg.BranchPrefix, sanitizedName)
	}

	// Convert repoPath to absolute path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		log.Error("git worktree path abs error, falling back to repoPath", "path", repoPath, "err", err)
		// If we can't get absolute path, use original path as fallback
		absPath = repoPath
	}

	repoPath, err = findGitRepoRoot(absPath)
	if err != nil {
		return nil, "", err
	}

	worktreeDir, err := getWorktreeDirectory()
	if err != nil {
		return nil, "", err
	}

	// First check if the branch is already checked out in an existing worktree
	existingWorktreePath, found := findExistingWorktreeForBranch(repoPath, branchName)
	if found {
		log.Info("found existing worktree for branch, reusing it", "branch", branchName, "path", existingWorktreePath)
		return &GitWorktree{
			repoPath:     repoPath,
			sessionName:  sessionName,
			branchName:   branchName,
			worktreePath: existingWorktreePath,
			cmdExec:      cmdExec,
		}, branchName, nil
	}

	// No existing worktree found, create a new one with timestamp suffix
	sanitizedName := sanitizeBranchName(sessionName)
	worktreePath := filepath.Join(worktreeDir, sanitizedName)
	worktreePath = worktreePath + "_" + fmt.Sprintf("%x", time.Now().UnixNano())

	return &GitWorktree{
		repoPath:     repoPath,
		sessionName:  sessionName,
		branchName:   branchName,
		worktreePath: worktreePath,
		cmdExec:      cmdExec,
	}, branchName, nil
}

// GetWorktreePath returns the path to the worktree
func (g *GitWorktree) GetWorktreePath() string {
	return g.worktreePath
}

// GetBranchName returns the name of the branch associated with this worktree
func (g *GitWorktree) GetBranchName() string {
	return g.branchName
}

// GetRepoPath returns the path to the repository
func (g *GitWorktree) GetRepoPath() string {
	return g.repoPath
}

// GetRepoName returns the name of the repository (last part of the repoPath).
func (g *GitWorktree) GetRepoName() string {
	return filepath.Base(g.repoPath)
}

// GetBaseCommitSHA returns the base commit SHA for the worktree
func (g *GitWorktree) GetBaseCommitSHA() string {
	return g.baseCommitSHA
}

// NewGitWorktreeFromExisting creates a GitWorktree from an existing worktree path
// This is used when connecting to worktrees that were created manually or by deleted sessions
func NewGitWorktreeFromExisting(existingWorktreePath string, sessionName string) (*GitWorktree, error) {
	return NewGitWorktreeFromExistingWithExecutor(existingWorktreePath, sessionName, nil)
}

// NewGitWorktreeFromExistingWithExecutor creates a GitWorktree from an existing worktree path with an optional executor.
func NewGitWorktreeFromExistingWithExecutor(existingWorktreePath string, sessionName string, cmdExec executor.Executor) (*GitWorktree, error) {
	if cmdExec == nil {
		cmdExec = executor.MakeExecutor()
	}

	// Ensure the path exists and is a valid git worktree
	if !IsGitRepo(existingWorktreePath) {
		return nil, fmt.Errorf("path '%s' is not a valid git repository or worktree", existingWorktreePath)
	}

	// Find the repository root from the worktree path
	repoPath, err := findGitRepoRoot(existingWorktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to find repository root for worktree '%s': %w", existingWorktreePath, err)
	}

	// Detect the current branch name in the worktree
	branchName, err := getCurrentBranchName(existingWorktreePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect branch name for worktree '%s': %w", existingWorktreePath, err)
	}

	// Get the base commit SHA (HEAD commit)
	baseCommitSHA, err := getHeadCommitSHA(existingWorktreePath)
	if err != nil {
		// This is not critical - we can continue without it
		log.Warn("failed to get base commit SHA for worktree", "path", existingWorktreePath, "err", err)
		baseCommitSHA = ""
	}

	return &GitWorktree{
		repoPath:      repoPath,
		worktreePath:  existingWorktreePath,
		sessionName:   sessionName,
		branchName:    branchName,
		baseCommitSHA: baseCommitSHA,
		cmdExec:       cmdExec,
	}, nil
}

// findExistingWorktreeForBranch checks if the given branch is already checked out in an existing worktree
// Returns the path to the existing worktree and true if found, empty string and false otherwise
func findExistingWorktreeForBranch(repoPath, branchName string) (string, bool) {
	// Run git worktree list --porcelain to get detailed worktree information
	wtCtx, wtCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer wtCancel()
	cmd := safeexec.CommandContext(wtCtx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// If the command fails, assume no existing worktrees
		log.Info("failed to list worktrees for branch check", "err", err)
		return "", false
	}

	// Parse the porcelain output to find matching branch
	return parseWorktreeListForBranch(string(output), branchName)
}

// parseWorktreeListForBranch parses the output of 'git worktree list --porcelain'
// and returns the path of the worktree that has the specified branch checked out
func parseWorktreeListForBranch(porcelainOutput, targetBranch string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(porcelainOutput), "\n")
	var currentWorktreePath string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			// Empty line separates worktree entries
			currentWorktreePath = ""
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			// Extract worktree path
			currentWorktreePath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") && currentWorktreePath != "" {
			// Extract branch name and check if it matches
			branchName := strings.TrimPrefix(line, "branch refs/heads/")
			if branchName == targetBranch {
				return currentWorktreePath, true
			}
		}
	}

	return "", false
}
