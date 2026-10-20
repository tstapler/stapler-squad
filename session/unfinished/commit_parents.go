package unfinished

import (
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// CommitParentsReader reads a commit's parents and committer time without
// decoding the message or signatures. The merge-base walks and the shared
// commit graph need only those two fields (go-git's full commit decode was
// 4.9% of scanner allocations in the S1 profile).
//
// Implementations append the parents to buf[:0] and return it, so the result
// never aliases pooled memory. when is the committer time in unix seconds,
// exactly what object.Commit.Committer.When.Unix() reports for the same
// object (the zero time's value when the header is absent or malformed).
type CommitParentsReader interface {
	Parents(s storer.EncodedObjectStorer, h plumbing.Hash, buf []plumbing.Hash) (parents []plumbing.Hash, when int64, err error)
}

// stockCommitParents is the in-package header-only reader; the fork's
// DecodeCommitHeader (Story 1.2.2/1.2.3) is deferred, see ADR-004.
var stockCommitParents CommitParentsReader = stockParentsReader{}
