package github

import (
	"fmt"
	"strconv"
)

// RepoRef is a value object that bundles a GitHub owner and repository name.
// Both fields are non-empty by construction — holding a RepoRef proves the
// invariant holds without any further nil/empty checks at use sites.
type RepoRef struct {
	owner string
	repo  string
}

// NewRepoRef constructs a RepoRef, returning an error if either field is empty.
func NewRepoRef(owner, repo string) (RepoRef, error) {
	if owner == "" {
		return RepoRef{}, fmt.Errorf("github owner must not be empty")
	}
	if repo == "" {
		return RepoRef{}, fmt.Errorf("github repo must not be empty")
	}
	return RepoRef{owner: owner, repo: repo}, nil
}

func (r RepoRef) Owner() string { return r.owner }
func (r RepoRef) Repo() string  { return r.repo }

// IsValid reports whether the ref was constructed with non-empty owner and repo.
// The zero value (RepoRef{}) is not valid.
func (r RepoRef) IsValid() bool { return r.owner != "" && r.repo != "" }

// String returns "owner/repo".
func (r RepoRef) String() string { return r.owner + "/" + r.repo }

// BranchKey returns the map key used to match sessions by branch:
// "owner/branch".
func (r RepoRef) BranchKey(branch string) string { return r.owner + "/" + branch }

// PRKey returns the map key used to match sessions by PR number:
// "owner/#number".
func (r RepoRef) PRKey(number int) string { return r.owner + "/#" + strconv.Itoa(number) }
