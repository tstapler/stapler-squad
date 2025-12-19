// Package vcs provides an abstraction layer over version control systems.
// It supports Jujutsu (JJ) as the primary VCS with automatic fallback to Git.
package vcs

import (
	"fmt"
	"time"
)

// VCSType represents the version control system in use
type VCSType string

const (
	VCSTypeJJ  VCSType = "jj"
	VCSTypeGit VCSType = "git"
)

// ChangeStrategy defines how to handle uncommitted changes during workspace switches
type ChangeStrategy int

const (
	// KeepAsWIP keeps changes as a separate WIP revision (JJ) or stash (Git)
	KeepAsWIP ChangeStrategy = iota
	// BringAlong keeps changes as parent of new location (JJ) or stash pop (Git)
	BringAlong
	// Abandon discards uncommitted changes
	Abandon
)

func (s ChangeStrategy) String() string {
	switch s {
	case KeepAsWIP:
		return "keep_as_wip"
	case BringAlong:
		return "bring_along"
	case Abandon:
		return "abandon"
	default:
		return "unknown"
	}
}

// ParseChangeStrategy converts a string to ChangeStrategy
func ParseChangeStrategy(s string) (ChangeStrategy, error) {
	switch s {
	case "keep_as_wip", "keep":
		return KeepAsWIP, nil
	case "bring_along", "bring":
		return BringAlong, nil
	case "abandon", "discard":
		return Abandon, nil
	default:
		return KeepAsWIP, fmt.Errorf("unknown change strategy: %s", s)
	}
}

// SwitchOptions configures how to handle a workspace switch
type SwitchOptions struct {
	// ChangeStrategy determines how to handle uncommitted changes
	ChangeStrategy ChangeStrategy
	// CreateIfMissing creates the bookmark/branch if it doesn't exist
	CreateIfMissing bool
	// BaseRevision is the base for new bookmark creation (empty = current)
	BaseRevision string
}

// Revision represents a VCS revision (commit in Git, change in JJ)
type Revision struct {
	// ID is the unique identifier (commit SHA in Git, change ID in JJ)
	ID string
	// ShortID is a shortened version of the ID for display
	ShortID string
	// Description is the commit/change message (first line)
	Description string
	// Author is the revision author
	Author string
	// Timestamp is when the revision was created
	Timestamp time.Time
	// Bookmarks are the bookmarks/branches pointing to this revision
	Bookmarks []string
	// IsCurrent indicates if this is the current working copy revision
	IsCurrent bool
}

// Bookmark represents a named reference (branch in Git, bookmark in JJ)
type Bookmark struct {
	// Name is the bookmark/branch name
	Name string
	// RevisionID is the revision this bookmark points to
	RevisionID string
	// IsRemote indicates if this is a remote tracking bookmark
	IsRemote bool
	// Upstream is the remote tracking bookmark (if any)
	Upstream string
}

// Worktree represents a working tree/workspace
type Worktree struct {
	// Path is the filesystem path to the worktree
	Path string
	// Name is the worktree name (JJ workspace name)
	Name string
	// Bookmark is the associated bookmark/branch
	Bookmark string
	// RevisionID is the current revision in this worktree
	RevisionID string
	// IsCurrent indicates if this is the current worktree
	IsCurrent bool
}

// WorkingCopyStatus represents the status of the working copy
type WorkingCopyStatus struct {
	// HasChanges indicates if there are uncommitted changes
	HasChanges bool
	// ModifiedFiles is the count of modified files
	ModifiedFiles int
	// AddedFiles is the count of newly added files
	AddedFiles int
	// DeletedFiles is the count of deleted files
	DeletedFiles int
	// ConflictedFiles is the count of files with conflicts
	ConflictedFiles int
}

// VCS is the abstraction interface over JJ and Git operations
type VCS interface {
	// Type returns the VCS type (jj or git)
	Type() VCSType

	// RepoPath returns the root path of the repository
	RepoPath() string

	// --- Status Operations ---

	// GetStatus returns the working copy status
	GetStatus() (*WorkingCopyStatus, error)

	// HasUncommittedChanges returns true if there are uncommitted changes
	HasUncommittedChanges() (bool, error)

	// GetCurrentRevision returns the current revision
	GetCurrentRevision() (*Revision, error)

	// GetCurrentBookmark returns the current bookmark/branch name (may be empty)
	GetCurrentBookmark() (string, error)

	// --- Navigation Operations ---

	// SwitchTo switches to the target revision or bookmark
	SwitchTo(target string, opts SwitchOptions) error

	// CreateBookmark creates a new bookmark/branch
	CreateBookmark(name string, base string) error

	// --- Working Copy Management ---

	// DescribeWIP describes the current change as WIP (JJ: describe, Git: no-op)
	DescribeWIP(message string) error

	// AbandonChanges abandons uncommitted changes
	AbandonChanges() error

	// --- Listing Operations ---

	// ListBookmarks returns all bookmarks/branches
	ListBookmarks() ([]Bookmark, error)

	// ListRecentRevisions returns recent revisions for display
	ListRecentRevisions(limit int) ([]Revision, error)

	// --- Worktree Operations ---

	// CreateWorktree creates a new worktree at the specified path
	CreateWorktree(path string, bookmark string) error

	// ListWorktrees returns all worktrees
	ListWorktrees() ([]Worktree, error)
}
