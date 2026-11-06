package unfinished

// Reporting-only differential over real repositories (read-only, go-git): the
// current repo plus any ~/code/github.com/tstapler/* repo present. Opt-in via
// MERGEBASE_REAL_REPOS=1; never fails on a divergence; counts go to t.Logf for baseline.md.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gitutil "github.com/tstapler/stapler-squad/session/git"
)

const maxRealPairsPerRepo = 6

func realRepoPaths(t *testing.T) []string {
	paths := []string{}
	if wd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Clean(filepath.Join(wd, "..", "..")))
	}
	if home, err := os.UserHomeDir(); err == nil {
		if m, err := filepath.Glob(filepath.Join(home, "code", "github.com", "tstapler", "*")); err == nil {
			for _, p := range m {
				if !strings.HasPrefix(filepath.Base(p), "stapler-squad-") { // sibling clones of this repo add time, not coverage
					paths = append(paths, p)
				}
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range paths {
		if rp, err := filepath.EvalSymlinks(p); err == nil && !seen[rp] {
			seen[rp] = true
			out = append(out, rp)
		}
	}
	return out
}

func TestFindMergeBase_ReportRealWorktreeDifferential(t *testing.T) {
	if os.Getenv("MERGEBASE_REAL_REPOS") == "" {
		t.Skip("reporting-only and ~30s; set MERGEBASE_REAL_REPOS=1 to run")
	}
	var repos, pairs, same, differ, errs, legacyBest, lockstepBest, oracleChecked int
	for _, path := range realRepoPaths(t) {
		repo, err := gitutil.OpenRepo(path)
		if err != nil {
			continue
		}
		head, err := repo.Head()
		if err != nil {
			continue
		}
		var tips []plumbing.Hash
		refs, _ := repo.References()
		_ = refs.ForEach(func(r *plumbing.Reference) error {
			if r.Type() == plumbing.HashReference && (r.Name().IsBranch() || r.Name().IsRemote()) && len(tips) < maxRealPairsPerRepo {
				tips = append(tips, r.Hash())
			}
			return nil
		})
		repos++
		start := time.Now()
		for _, tip := range tips {
			if tip == head.Hash() {
				continue
			}
			pairs++
			l, lerr := legacyStackFindMergeBase(repo, head.Hash(), tip)
			k, kerr := findMergeBase(newParentMemo(repo), head.Hash(), tip)
			if lerr != nil || kerr != nil {
				errs++
				if (lerr == nil) != (kerr == nil) {
					differ++
				}
				continue
			}
			if l == k {
				same++
			} else {
				differ++
			}
			if best, ok := realBestSet(repo, head.Hash(), tip); ok {
				oracleChecked++
				if best[l] {
					legacyBest++
				}
				if best[k] {
					lockstepBest++
				}
			}
		}
		t.Logf("repo %s: %d refs, %v", filepath.Base(path), len(tips), time.Since(start))
	}
	t.Logf("real-worktree differential: repos=%d pairs=%d identical(legacy==lockstep)=%d divergent=%d (incl. error-mismatch) both-errored-or-mismatched=%d; vs object.Commit.MergeBase over %d pairs: legacy-in-best=%d lockstep-in-best=%d",
		repos, pairs, same, differ, errs, oracleChecked, legacyBest, lockstepBest)
}

func realBestSet(repo *git.Repository, a, b plumbing.Hash) (map[plumbing.Hash]bool, bool) {
	ca, err := repo.CommitObject(a)
	if err != nil {
		return nil, false
	}
	cb, err := repo.CommitObject(b)
	if err != nil {
		return nil, false
	}
	bases, err := ca.MergeBase(cb)
	if err != nil {
		return nil, false
	}
	out := map[plumbing.Hash]bool{}
	for _, c := range bases {
		out[c.Hash] = true
	}
	return out, true
}
