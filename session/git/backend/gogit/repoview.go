package gogit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native"
)

// maxSymrefDepth is git's symref chain limit (SYMREF_MAXDEPTH).
const maxSymrefDepth = 5

// maxTagDepth bounds tag-of-tag chains; deeper ones are left to the CLI.
const maxTagDepth = 10

var errNotCommit = errors.New("object is not a commit")

// repoView is one call's handle on a repository: a fresh *git.Repository opened through
// native.OpenRepo, discarded when the call returns. It is not safe for concurrent use.
type repoView struct {
	b    *Backend
	lay  layout
	repo *git.Repository
}

// view locates the repository for loc and opens it.
func (b *Backend) view(ctx context.Context, loc backend.RepoLocation) (*repoView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := localDir(loc)
	if err != nil {
		return nil, err
	}
	lay, err := b.locate(dir, b.environ())
	if err != nil {
		return nil, err
	}
	v := &repoView{b: b, lay: lay}
	if err := v.reopen(); err != nil {
		return nil, err
	}
	return v, nil
}

// reopen replaces the handle with a fresh one. It also proves go-git resolved the same git
// directory the layout scan did; if they disagree the CLI decides.
func (v *repoView) reopen() error {
	if v.lay.workTree == "" {
		return fallbackEligible(errors.New("location has no work tree (bare repository or inside a git directory)"))
	}
	repo, err := native.OpenRepo(v.lay.workTree)
	if err != nil {
		return fallbackEligible(fmt.Errorf("open repository: %w", err))
	}
	st, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return fallbackEligible(errors.New("unexpected go-git storage type"))
	}
	root, err := filepath.EvalSymlinks(st.Filesystem().Root())
	if err != nil || root != v.lay.gitDir {
		return fallbackEligible(fmt.Errorf("go-git opened git dir %q, expected %q", root, v.lay.gitDir))
	}
	v.repo = repo
	return nil
}

// resolveName follows the ref name (symbolic refs included, up to git's depth limit) to the hash
// it ends at. found is false when the ref, or the target of a symbolic ref, does not exist: a
// dangling symref counts as absent, as it does for git's READING lookups.
func (v *repoView) resolveName(name string) (hash plumbing.Hash, found bool, err error) {
	hash, found, _, err = v.followRef(name)
	return hash, found, err
}

// followRef is resolveName that also returns the last ref name it visited: the fully
// dereferenced name, which is what `symbolic-ref` and `rev-parse --abbrev-ref` report even when
// HEAD points through an alias, and which exists or not (an unborn branch).
func (v *repoView) followRef(name string) (hash plumbing.Hash, found bool, final string, err error) {
	for depth := 0; depth <= maxSymrefDepth; depth++ {
		ref, err := v.repo.Storer.Reference(plumbing.ReferenceName(name))
		switch {
		case errors.Is(err, plumbing.ErrReferenceNotFound):
			if v.looseRefFileExists(name) {
				// go-git silently treats an unreadable loose ref as absent; a file that is there
				// but unreadable is a writer mid-update, so no "does not exist" answer is safe.
				return plumbing.ZeroHash, false, name, fallbackEligible(fmt.Errorf("%w: loose ref %s exists but could not be read", backend.ErrTornRead, name))
			}
			return plumbing.ZeroHash, false, name, nil
		case err != nil:
			return plumbing.ZeroHash, false, name, fallbackEligible(fmt.Errorf("read ref %s: %w", name, err))
		}
		if ref.Type() == plumbing.HashReference {
			return ref.Hash(), true, name, nil
		}
		name = ref.Target().String()
		if !strings.HasPrefix(name, "refs/") || !validRefName(name, false) {
			return plumbing.ZeroHash, false, name, fallbackEligible(fmt.Errorf("symbolic ref points at %q", name))
		}
	}
	return plumbing.ZeroHash, false, name, fallbackEligible(fmt.Errorf("symbolic ref chain from %s is too deep", name))
}

// perWorktreeRefPrefixes live in the per-worktree git dir; every other ref is shared.
var perWorktreeRefPrefixes = []string{"refs/bisect/", "refs/worktree/", "refs/rewritten/"} //nolint:gochecknoglobals read-only

func (v *repoView) looseRefFileExists(name string) bool {
	dir := v.lay.commonDir
	if !strings.HasPrefix(name, "refs/") {
		dir = v.lay.gitDir
	}
	for _, p := range perWorktreeRefPrefixes {
		if strings.HasPrefix(name, p) {
			dir = v.lay.gitDir
		}
	}
	fi, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(name)))
	return err == nil && !fi.IsDir()
}

// refExists is git's ref_exists for a full name: the ref resolves (the object is not checked).
func (v *repoView) refExists(full string) (bool, error) {
	if !strings.HasPrefix(full, "refs/") && !isPseudoRefName(full) {
		return false, nil // git refuses to read any other name as a ref
	}
	if !validRefName(full, true) {
		return false, nil
	}
	_, found, err := v.resolveName(full)
	return found, err
}

type headState struct {
	detached bool
	unborn   bool
	target   string        // the full ref HEAD ends at once symbolic refs are followed
	hash     plumbing.Hash // the commit HEAD resolves to; zero when unborn
}

func (v *repoView) head() (headState, error) {
	ref, err := v.repo.Storer.Reference(plumbing.HEAD)
	if err != nil {
		return headState{}, fallbackEligible(fmt.Errorf("read HEAD: %w", err))
	}
	if ref.Type() == plumbing.HashReference {
		return headState{detached: true, hash: ref.Hash()}, nil
	}
	target := ref.Target().String()
	if !strings.HasPrefix(target, "refs/") || !validRefName(target, false) {
		return headState{}, fallbackEligible(fmt.Errorf("HEAD points at %q", target))
	}
	hash, found, final, err := v.followRef(target)
	if err != nil {
		return headState{}, err
	}
	return headState{target: final, unborn: !found, hash: hash}, nil
}

// readCommit returns the commit hash names, peeling annotated tags. A missing or unreadable
// object is retried on a fresh handle (a concurrent repack can remove a pack between listing and
// opening it) before it is reported as fallback-eligible torn_read/object_missing, so a SHA whose
// object was never read back is never returned (spike S7).
func (v *repoView) readCommit(ctx context.Context, hash plumbing.Hash) (*object.Commit, error) {
	for attempt := 0; ; attempt++ {
		c, err := peelToCommit(v.repo, hash)
		if err == nil {
			return c, nil
		}
		if errors.Is(err, errNotCommit) {
			return nil, fallbackEligible(fmt.Errorf("%s does not name a commit", hash))
		}
		if attempt >= v.b.retries {
			sentinel := backend.ErrTornRead
			if errors.Is(err, plumbing.ErrObjectNotFound) {
				sentinel = backend.ErrObjectMissing
			}
			return nil, fallbackEligible(fmt.Errorf("%w: %s: %v", sentinel, hash, err))
		}
		v.b.sleep(ctx, v.b.retryDelay)
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		if rerr := v.reopen(); rerr != nil {
			return nil, rerr
		}
	}
}

func peelToCommit(repo *git.Repository, hash plumbing.Hash) (*object.Commit, error) {
	for range maxTagDepth {
		obj, err := repo.Storer.EncodedObject(plumbing.AnyObject, hash)
		if err != nil {
			return nil, err
		}
		switch obj.Type() {
		case plumbing.CommitObject:
			return object.DecodeCommit(repo.Storer, obj)
		case plumbing.TagObject:
			tag, err := object.DecodeTag(repo.Storer, obj)
			if err != nil {
				return nil, err
			}
			hash = tag.Target
		default:
			return nil, errNotCommit
		}
	}
	return nil, fmt.Errorf("tag chain from %s is too deep: %w", hash, errNotCommit)
}
