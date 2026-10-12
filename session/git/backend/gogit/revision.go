package gogit

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	fullHexPattern  = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	shortHexPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,39}$`)
	revOpsPattern   = regexp.MustCompile(`^(?:[~^][0-9]{0,6})+$`)
	revOpPattern    = regexp.MustCompile(`[~^][0-9]{0,6}`)
)

// revOp is one "~N" (N-th first-parent ancestor) or "^N" (N-th parent) step.
type revOp struct {
	ancestor bool // ~ rather than ^
	n        int
}

// parseRev splits a revision into a base name and its ~/^ steps, dropping a trailing ^{commit}
// (everything resolved here is a commit anyway). ok is false for any other syntax (@{...},
// ranges, empty base): the caller falls back.
func parseRev(rev string) (base string, ops []revOp, ok bool) {
	rev = strings.TrimSuffix(rev, "^{commit}")
	i := strings.IndexAny(rev, "~^")
	if i < 0 {
		return rev, nil, rev != ""
	}
	base, rest := rev[:i], rev[i:]
	if base == "" || !revOpsPattern.MatchString(rest) {
		return "", nil, false
	}
	for _, m := range revOpPattern.FindAllString(rest, -1) {
		n := 1
		if len(m) > 1 {
			n, _ = strconv.Atoi(m[1:])
		}
		ops = append(ops, revOp{ancestor: m[0] == '~', n: n})
	}
	return base, ops, true
}

// dwim expands a short ref name the way git's repo_dwim_ref does: the first of the rev-parse
// rules that resolves wins (a branch and a tag of the same name resolve to the tag).
func (v *repoView) dwim(base string) (plumbing.Hash, bool, error) {
	for _, rule := range revParseRules {
		var full string
		if rule == "%s" {
			if !strings.HasPrefix(base, "refs/") {
				continue // git reads a bare name as a ref only under refs/ or as a pseudo-ref
			}
			full = base
		} else {
			full = applyRule(rule, base)
		}
		if !validRefName(full, false) {
			continue
		}
		hash, found, err := v.resolveName(full)
		if err != nil {
			return plumbing.ZeroHash, false, err
		}
		if found {
			return hash, true, nil
		}
	}
	return plumbing.ZeroHash, false, nil
}

// revision resolves rev to a commit with the semantics of `git rev-parse --verify rev^{commit}`.
// found is false where git says the revision does not exist; an error is returned for anything
// else, fallback-eligible whenever this resolver is not certain to match git (abbreviated SHAs,
// pseudo-refs, @{...} syntax, shallow or grafted history for ~/^ steps).
func (v *repoView) revision(ctx context.Context, rev string) (commit *object.Commit, found bool, err error) {
	base, ops, ok := parseRev(rev)
	if !ok {
		return nil, false, fallbackEligible(fmt.Errorf("revision syntax %q is not handled in-process", rev))
	}
	var hash plumbing.Hash
	switch {
	case base == "HEAD" || base == "@":
		h, err := v.head()
		if err != nil {
			return nil, false, err
		}
		if h.unborn {
			return nil, false, nil
		}
		hash = h.hash
	case fullHexPattern.MatchString(base):
		hash = plumbing.NewHash(strings.ToLower(base))
	default:
		if isPseudoRefName(base) || !validRefName("refs/"+base, false) {
			return nil, false, fallbackEligible(fmt.Errorf("revision %q is a pseudo-ref or not a plain name", base))
		}
		h, ok, err := v.dwim(base)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			if shortHexPattern.MatchString(base) {
				return nil, false, fallbackEligible(fmt.Errorf("%q may be an abbreviated object name", base))
			}
			return nil, false, nil
		}
		hash = h
	}
	if len(ops) > 0 {
		if err := v.requireIntactHistory(); err != nil {
			return nil, false, err
		}
	}
	commit, err = v.readCommit(ctx, hash)
	if err != nil {
		return nil, false, err
	}
	for _, op := range ops {
		var ok bool
		if commit, ok, err = v.step(ctx, commit, op); err != nil || !ok {
			return nil, false, err
		}
	}
	return commit, true, nil
}

// step applies one ~N or ^N; ok is false when the history ends before the step is complete.
func (v *repoView) step(ctx context.Context, c *object.Commit, op revOp) (next *object.Commit, ok bool, err error) {
	if !op.ancestor {
		if op.n == 0 {
			return c, true, nil
		}
		return v.parent(ctx, c, op.n)
	}
	for range op.n {
		if c, ok, err = v.parent(ctx, c, 1); err != nil || !ok {
			return nil, false, err
		}
	}
	return c, true, nil
}

func (v *repoView) parent(ctx context.Context, c *object.Commit, n int) (parent *object.Commit, ok bool, err error) {
	if n > len(c.ParentHashes) {
		return nil, false, nil
	}
	parent, err = v.readCommit(ctx, c.ParentHashes[n-1])
	return parent, err == nil, err
}

func (v *repoView) requireIntactHistory() error {
	rewritten, err := historyRewritten(v.lay)
	if err != nil {
		return fallbackEligible(err)
	}
	if rewritten {
		return fallbackEligible(errors.New("shallow, grafted or replaced history: parents differ from the CLI's"))
	}
	return nil
}
