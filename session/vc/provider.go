package vc

import "errors"

// ErrNoVCSFound is returned when no version control system is detected
var ErrNoVCSFound = errors.New("no version control system found")

// ErrNotImplemented is returned when a feature is not implemented for a VCS
var ErrNotImplemented = errors.New("feature not implemented for this VCS")

// VCSProvider defines the interface for version control operations.
// Implementations exist for Git and Jujutsu.
type VCSProvider interface {
	// Type returns the type of version control system
	Type() VCSType

	// Name returns the human-readable name of the VCS (e.g., "Git", "Jujutsu")
	Name() string

	// WorkDir returns the working directory path
	WorkDir() string

	// --- Status Operations ---

	// GetStatus returns the current status of the working directory
	GetStatus() (*VCSStatus, error)

	// GetBranch returns the current branch name (Git) or bookmark/change ID (Jujutsu)
	GetBranch() (string, error)

	// GetChangedFiles returns a list of all changed files
	GetChangedFiles() ([]FileChange, error)

	// --- Staging Operations ---

	// Stage adds a file to the staging area (Git) or marks it for inclusion (Jujutsu)
	Stage(path string) error

	// StageAll stages all changed files
	StageAll() error

	// Unstage removes a file from the staging area
	Unstage(path string) error

	// UnstageAll unstages all files
	UnstageAll() error

	// --- Commit Operations ---

	// Commit creates a new commit with the given message
	Commit(message string) error

	// AmendCommit amends the last commit (Git) or updates the current change description (Jujutsu)
	AmendCommit(message string) error

	// --- Remote Operations ---

	// Push pushes commits to the remote repository
	Push() error

	// PushWithOptions pushes with additional options (force, set-upstream, etc.)
	PushWithOptions(opts PushOptions) error

	// Pull pulls changes from the remote repository
	Pull() error

	// Fetch fetches changes from the remote without merging
	Fetch() error

	// --- Diff Operations ---

	// GetFileDiff returns the diff for a specific file
	GetFileDiff(path string) (string, error)

	// GetDiff returns the full diff of uncommitted changes
	GetDiff() (string, error)

	// --- Interactive Terminal ---

	// GetInteractiveCommand returns the command to open an interactive VCS tool
	// For Git: "lazygit" or "tig" if available, otherwise "git status"
	// For Jujutsu: "lazyjj" if available, otherwise "jj log"
	GetInteractiveCommand() string

	// GetLogCommand returns the command to view the commit/change log
	GetLogCommand() string
}

// PushOptions contains options for the Push operation
type PushOptions struct {
	Force       bool   // Force push (use with caution)
	SetUpstream bool   // Set upstream tracking reference
	Remote      string // Remote name (defaults to "origin")
	Branch      string // Branch name (defaults to current branch)
}

// CommitOptions contains options for the Commit operation
type CommitOptions struct {
	Message    string   // Commit message
	Amend      bool     // Amend the last commit
	AllowEmpty bool     // Allow empty commits
	Author     string   // Override author
	Files      []string // Specific files to commit (empty = all staged)
}
