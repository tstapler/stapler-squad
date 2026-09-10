package git

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/tstapler/stapler-squad/log"
)

// MergeBaseResolver returns the merge-base commit for ours and theirs — the common
// ancestor the native three-way merge pipeline (Epic 3.1) diffs both sides against.
//
// (*object.Commit).MergeBase can return more than one candidate for a criss-cross
// merge history (independent, mutually-unreachable merge bases). Per this project's
// explicit scope cut (octopus/criss-cross merge resolution is out of scope — see
// requirements.md), that ambiguity is not treated as an error: the first candidate is
// used deterministically, matching git's own tie-breaking behavior for the common
// (non-octopus) merge case, and every candidate is named in a WARN log line so the
// ambiguity is never silent.
func MergeBaseResolver(ours, theirs *object.Commit) (*object.Commit, error) {
	if ours == nil || theirs == nil {
		return nil, errors.New("MergeBaseResolver: ours and theirs must not be nil")
	}

	bases, err := ours.MergeBase(theirs)
	if err != nil {
		return nil, fmt.Errorf("compute merge base of %s and %s: %w", ours.Hash, theirs.Hash, err)
	}
	if len(bases) == 0 {
		return nil, fmt.Errorf("no merge base found between %s and %s (unrelated histories)", ours.Hash, theirs.Hash)
	}
	if len(bases) > 1 {
		candidates := make([]string, len(bases))
		for i, base := range bases {
			candidates[i] = base.Hash.String()
		}
		log.Warn("MergeBaseResolver: multiple merge-base candidates found (criss-cross merge history), picking the first deterministically",
			"ours", ours.Hash.String(), "theirs", theirs.Hash.String(), "candidates", candidates, "chosen", candidates[0])
	}
	return bases[0], nil
}

// TreeDiffPair computes the two change-sets the diff3 reconciler (Epic 3.2) needs as
// its raw input: base→ours and base→theirs.
func TreeDiffPair(base, ours, theirs *object.Tree) (baseToOurs, baseToTheirs object.Changes, err error) {
	baseToOurs, err = base.Diff(ours)
	if err != nil {
		return nil, nil, fmt.Errorf("diff base tree against ours: %w", err)
	}
	baseToTheirs, err = base.Diff(theirs)
	if err != nil {
		return nil, nil, fmt.Errorf("diff base tree against theirs: %w", err)
	}
	return baseToOurs, baseToTheirs, nil
}
