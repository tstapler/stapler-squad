package commitwalk

import (
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
)

// ParentSource supplies commit parents and committer times to the merge-base
// and counting walks. ParentMemo is the per-call implementation.
type ParentSource interface {
	Info(h plumbing.Hash) (CommitInfo, error)
	Get(h plumbing.Hash) ([]plumbing.Hash, error)
}

var _ ParentSource = (*ParentMemo)(nil)

// AheadBehindFrom is phase 2: a pure function of the commit graph behind src.
// It reads nothing from any cachedRepo.
func AheadBehindFrom(src ParentSource, head, base plumbing.Hash) (ahead, behind int, err error) {
	mb, mbErr := FindMergeBase(src, head, base)
	if mbErr != nil {
		return 0, 0, fmt.Errorf("merge base: %w", mbErr)
	}
	if ahead, err = CountCommitsTo(src, head, mb); err != nil {
		return 0, 0, err
	}
	if behind, err = CountCommitsTo(src, base, mb); err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}
