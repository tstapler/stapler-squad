package gogit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// isHEAD mirrors the CLI backend: only the literal names HEAD and @ can be unborn.
func isHEAD(ref backend.RefName) bool { return ref == "HEAD" || ref == "@" }

func notFound(what string) error { return fmt.Errorf("%w: %s", backend.ErrRefNotFound, what) }

// CurrentBranch is `git rev-parse --abbrev-ref HEAD`: HEAD's target in its shortest unambiguous
// form, so a tag that shares the branch's name turns "main" into "heads/main".
func (b *Backend) CurrentBranch(ctx context.Context, loc backend.RepoLocation) (backend.BranchName, error) {
	v, err := b.view(ctx, loc)
	if err != nil {
		return "", err
	}
	h, err := v.head()
	if err != nil {
		return "", err
	}
	switch {
	case h.detached:
		return "", backend.ErrDetachedHead
	case h.unborn:
		return "", fmt.Errorf("%w: %s has no commit yet", backend.ErrUnborn, h.target)
	}
	name, err := shortenUnambiguous(h.target, v.refExists)
	if err != nil {
		return "", err
	}
	return backend.BranchName(name), nil
}

// HeadRef is `git symbolic-ref -q HEAD`: the ref HEAD ends at once aliases are followed, valid on
// an unborn branch.
func (b *Backend) HeadRef(ctx context.Context, loc backend.RepoLocation) (backend.RefName, error) {
	v, err := b.view(ctx, loc)
	if err != nil {
		return "", err
	}
	h, err := v.head()
	if err != nil {
		return "", err
	}
	if h.detached {
		return "", backend.ErrDetachedHead
	}
	return backend.RefName(h.target), nil
}

// ResolveRef is `git rev-parse --verify <ref>^{commit}`: commit-only, tags peeled, and the
// returned SHA's object has been read back.
func (b *Backend) ResolveRef(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
	if err := commitRef(ref); err != nil {
		return "", err
	}
	v, err := b.view(ctx, loc)
	if err != nil {
		return "", err
	}
	commit, found, err := v.revision(ctx, string(ref))
	if err != nil {
		return "", err
	}
	if found {
		return backend.CommitSHA(commit.Hash.String()), nil
	}
	if isHEAD(ref) {
		if h, herr := v.head(); herr == nil && !h.detached && h.unborn {
			return "", fmt.Errorf("%w: %s has no commit yet", backend.ErrUnborn, h.target)
		} else if herr != nil {
			return "", herr
		}
	}
	return "", notFound(string(ref))
}

// RefExists is `git rev-parse --verify --quiet <ref>^{commit}`: false for anything that does not
// name a commit, including an unborn HEAD.
func (b *Backend) RefExists(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (bool, error) {
	if err := commitRef(ref); err != nil {
		return false, err
	}
	v, err := b.view(ctx, loc)
	if err != nil {
		return false, err
	}
	_, found, err := v.revision(ctx, string(ref))
	if err != nil {
		return false, err
	}
	return found, nil
}

// RepoRoot is `git rev-parse --show-toplevel`, as a real path.
func (b *Backend) RepoRoot(ctx context.Context, loc backend.RepoLocation) (backend.RepoRoot, error) {
	lay, err := b.layoutOf(ctx, loc)
	if err != nil {
		return "", err
	}
	if lay.workTree == "" {
		return "", fallbackEligible(errors.New("location has no work tree"))
	}
	return backend.RepoRoot(lay.workTree), nil
}

// GitDir is `git rev-parse --path-format=absolute --git-dir`: the per-worktree admin directory.
func (b *Backend) GitDir(ctx context.Context, loc backend.RepoLocation) (backend.GitDir, error) {
	lay, err := b.layoutOf(ctx, loc)
	if err != nil {
		return "", err
	}
	return backend.GitDir(lay.gitDir), nil
}

// CommonDir is `git rev-parse --path-format=absolute --git-common-dir`.
func (b *Backend) CommonDir(ctx context.Context, loc backend.RepoLocation) (backend.CommonDir, error) {
	lay, err := b.layoutOf(ctx, loc)
	if err != nil {
		return "", err
	}
	return backend.CommonDir(lay.commonDir), nil
}

func (b *Backend) layoutOf(ctx context.Context, loc backend.RepoLocation) (layout, error) {
	if err := ctx.Err(); err != nil {
		return layout{}, err
	}
	dir, err := localDir(loc)
	if err != nil {
		return layout{}, err
	}
	return b.locate(ctx, dir, b.environ())
}

// ListRefs is `git for-each-ref --format=%(refname) [--count=N] [pattern]`: full ref names under
// refs/, sorted bytewise, skipping refs git itself skips (invalid names, objects that are
// missing). A pattern with wildcards is left to the CLI.
func (b *Backend) ListRefs(ctx context.Context, loc backend.RepoLocation, req backend.ListRefsRequest) ([]backend.RefName, error) {
	if req.Pattern != "" {
		if err := optArg("pattern", string(req.Pattern)); err != nil {
			return nil, err
		}
		if strings.ContainsAny(string(req.Pattern), `*?[\`) {
			return nil, fallbackEligible(fmt.Errorf("ref pattern %q uses wildcards", req.Pattern))
		}
	}
	v, err := b.view(ctx, loc)
	if err != nil {
		return nil, err
	}
	names, err := v.listRefNames(ctx, string(req.Pattern))
	if err != nil {
		return nil, err
	}
	if req.Limit > 0 && len(names) > req.Limit {
		names = names[:req.Limit]
	}
	out := make([]backend.RefName, len(names))
	for i, n := range names {
		out[i] = backend.RefName(n)
	}
	return out, nil
}

// matchesPattern is for-each-ref's literal pattern rule: the pattern equals the ref name, is a
// leading run of whole path components of it, or ends in a slash and is a prefix of it.
func matchesPattern(pattern, name string) bool {
	if pattern == "" {
		return true
	}
	if !strings.HasPrefix(name, pattern) {
		return false
	}
	return len(name) == len(pattern) || name[len(pattern)] == '/' || pattern[len(pattern)-1] == '/'
}

func (v *repoView) listRefNames(ctx context.Context, pattern string) ([]string, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		names, err := v.scanRefs(pattern)
		if err == nil {
			sort.Strings(names)
			return names, nil
		}
		lastErr = err
		if !errors.Is(err, backend.ErrObjectMissing) || attempt >= v.b.retries {
			break
		}
		v.b.sleep(ctx, v.b.retryDelay)
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		if rerr := v.reopen(); rerr != nil {
			return nil, rerr
		}
	}
	return nil, lastErr
}

// scanRefs lists matching refs once. A ref whose object is absent is skipped by git (with a
// warning on stderr), but here it may equally be a pack replaced mid-scan, so it surfaces as
// ErrObjectMissing and the caller retries on a fresh handle, then defers to the CLI.
func (v *repoView) scanRefs(pattern string) ([]string, error) {
	all, err := v.walkRefNames()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, name := range all {
		if !matchesPattern(pattern, name) {
			continue
		}
		hash, found, err := v.resolveName(name)
		if err != nil {
			return nil, err
		}
		if !found {
			continue // dangling symbolic ref: git skips it
		}
		if v.repo.Storer.HasEncodedObject(hash) != nil {
			return nil, fallbackEligible(fmt.Errorf("%w: %s -> %s", backend.ErrObjectMissing, name, hash))
		}
		names = append(names, name)
	}
	return names, nil
}

// MergeBase is `git merge-base Left Right`. No common ancestor is ErrNoMergeBase; several equally
// good bases (a criss-cross history, where git's pick depends on its own ordering) and shallow,
// grafted or replaced history are left to the CLI.
func (b *Backend) MergeBase(ctx context.Context, loc backend.RepoLocation, req backend.MergeBaseRequest) (backend.CommitSHA, error) {
	if err := optArg("left", string(req.Left)); err != nil {
		return "", err
	}
	if err := optArg("right", string(req.Right)); err != nil {
		return "", err
	}
	v, err := b.view(ctx, loc)
	if err != nil {
		return "", err
	}
	if err := v.requireIntactHistory(); err != nil {
		return "", err
	}
	left, found, err := v.revision(ctx, string(req.Left))
	if err != nil {
		return "", err
	}
	if !found {
		return "", notFound(string(req.Left))
	}
	right, found, err := v.revision(ctx, string(req.Right))
	if err != nil {
		return "", err
	}
	if !found {
		return "", notFound(string(req.Right))
	}
	bases, err := left.MergeBase(right)
	if err != nil {
		return "", fallbackEligible(fmt.Errorf("%w: merge base walk: %v", backend.ErrTornRead, err))
	}
	switch len(bases) {
	case 0:
		return "", fmt.Errorf("%w: %s and %s", backend.ErrNoMergeBase, req.Left, req.Right)
	case 1:
		return backend.CommitSHA(bases[0].Hash.String()), nil
	}
	return "", fallbackEligible(fmt.Errorf("%d merge bases: git picks one by its own ordering", len(bases)))
}
