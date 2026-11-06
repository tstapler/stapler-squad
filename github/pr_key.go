package github

import (
	"fmt"
	"strconv"
)

// PRKey identifies one pull request by host, owner, repo and number. It is
// valid by construction (non-empty owner/repo, positive number, host
// normalized so github.com is never empty), so callers pass one value instead
// of four loose strings.
type PRKey struct {
	repo   RepoRef
	number int
}

// NewPRKey builds a PRKey; an empty host means github.com.
func NewPRKey(host, owner, repo string, number int) (PRKey, error) {
	if number <= 0 {
		return PRKey{}, fmt.Errorf("github PR number must be positive, got %d", number)
	}
	ref, err := NewRepoRefWithHost(owner, repo, NormalizeHost(host))
	if err != nil {
		return PRKey{}, err
	}
	return PRKey{repo: ref, number: number}, nil
}

// Host is the normalized host; never empty.
func (k PRKey) Host() string { return k.repo.Host() }

func (k PRKey) Owner() string    { return k.repo.Owner() }
func (k PRKey) RepoName() string { return k.repo.Repo() }
func (k PRKey) Number() int      { return k.number }

// RepoRef returns the repository part.
func (k PRKey) RepoRef() RepoRef { return k.repo }

// IsValid is false only for the zero value.
func (k PRKey) IsValid() bool { return k.repo.IsValid() && k.number > 0 }

// Key is the case-insensitive "host/owner/repo#number" form used by the
// session-link indexes.
func (k PRKey) Key() LinkKey { return k.repo.PRKey(k.number) }

// String is "host/owner/repo#number" for logs.
func (k PRKey) String() string {
	return k.repo.Host() + "/" + k.repo.String() + "#" + strconv.Itoa(k.number)
}
