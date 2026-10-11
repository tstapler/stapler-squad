package github

import (
	"fmt"
	"strconv"
	"strings"
)

// RepoRef is a value object that bundles a GitHub owner and repository name,
// plus the host that owns them (empty means github.com). Both owner and repo
// are non-empty by construction — holding a RepoRef proves the invariant
// holds without any further nil/empty checks at use sites.
type RepoRef struct {
	owner string
	repo  string
	host  string
}

// NewRepoRef constructs a github.com RepoRef, returning an error if either
// field is empty. Equivalent to NewRepoRefWithHost(owner, repo, "").
func NewRepoRef(owner, repo string) (RepoRef, error) {
	return NewRepoRefWithHost(owner, repo, "")
}

// NewRepoRefWithHost constructs a RepoRef scoped to host (empty means
// github.com), returning an error if owner or repo is empty.
func NewRepoRefWithHost(owner, repo, host string) (RepoRef, error) {
	if owner == "" {
		return RepoRef{}, fmt.Errorf("github owner must not be empty")
	}
	if repo == "" {
		return RepoRef{}, fmt.Errorf("github repo must not be empty")
	}
	return RepoRef{owner: owner, repo: repo, host: host}, nil
}

func (r RepoRef) Owner() string { return r.owner }
func (r RepoRef) Repo() string  { return r.repo }

// Host returns the GitHub Enterprise host that owns this repo, or "" for github.com.
func (r RepoRef) Host() string { return r.host }

// IsValid reports whether the ref was constructed with non-empty owner and repo.
// The zero value (RepoRef{}) is not valid.
func (r RepoRef) IsValid() bool { return r.owner != "" && r.repo != "" }

// String returns "owner/repo".
func (r RepoRef) String() string { return r.owner + "/" + r.repo }

// LinkKey identifies a session-to-PR match bucket: either a branch or a PR
// number within one host/owner/repo. A distinct type keeps branch keys, PR
// keys and arbitrary strings from being mixed up in the index maps.
type LinkKey string

// repoKey is the one place host, owner and repo are normalized for link keys.
// All three are case-insensitive on GitHub; an empty host means github.com.
func (r RepoRef) repoKey() string {
	return NormalizeHost(r.host) + "/" + strings.ToLower(r.owner) + "/" + strings.ToLower(r.repo)
}

// BranchKey returns "host/owner/repo@branch". The branch is never lowercased:
// git branch names are case-sensitive.
func (r RepoRef) BranchKey(branch string) LinkKey { return LinkKey(r.repoKey() + "@" + branch) }

// PRKey returns "host/owner/repo#number".
func (r RepoRef) PRKey(number int) LinkKey { return LinkKey(r.repoKey() + "#" + strconv.Itoa(number)) }

// LegacyBranchKey is the pre-host/repo key "owner/branch". It exists only for
// the fallback index in UserPRCache.Annotate; do not use it for new matching.
func (r RepoRef) LegacyBranchKey(branch string) LinkKey { return LinkKey(r.owner + "/" + branch) }

// LegacyPRKey is the pre-host/repo key "owner/#number" (fallback index only).
func (r RepoRef) LegacyPRKey(number int) LinkKey {
	return LinkKey(r.owner + "/#" + strconv.Itoa(number))
}
