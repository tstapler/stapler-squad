package gogit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// maxSymrefDepth is git's symref chain limit (SYMREF_MAXDEPTH).
const maxSymrefDepth = 5

// maxTagDepth bounds tag-of-tag chains; deeper ones are left to the CLI.
const maxTagDepth = 10

var errNotCommit = errors.New("object is not a commit")

// repoView is one call's handle on a repository: a fresh *git.Repository (see openStorerOnly), discarded when the call returns. It is not safe for concurrent use.
type repoView struct {
	b      *Backend
	lay    layout
	repo   *git.Repository
	packed map[string]plumbing.Hash // parsed packed-refs, read lazily once per handle
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
	lay, err := b.locate(ctx, dir, b.environ())
	if err != nil {
		return nil, err
	}
	v := &repoView{b: b, lay: lay}
	if err := v.reopen(); err != nil {
		return nil, err
	}
	return v, nil
}

// reopen replaces the handle with a fresh one over exactly the directories the layout scan
// resolved, so go-git and the layout can never disagree about where the repository is, and a
// symlinked git directory, objects/ or config is followed as git follows it (see hostFS).
func (v *repoView) reopen() error {
	if v.lay.workTree == "" {
		return fallbackEligible(errors.New("location has no work tree (bare repository or inside a git directory)"))
	}
	repo, err := openStorerOnly(v.lay)
	if err != nil {
		return fallbackEligible(fmt.Errorf("open repository: %w", err))
	}
	v.repo = repo
	v.packed = nil
	return nil
}

// openStorerOnly opens lay's repository for object reads. There is no work tree: nothing here
// reads or writes one. A linked worktree's shared directories come from its common directory,
// as with native.OpenRepo's EnableDotGitCommonDir.
func openStorerOnly(lay layout) (*git.Repository, error) {
	var repoFS billy.Filesystem = hostFS{root: lay.gitDir}
	if lay.commonDir != lay.gitDir {
		repoFS = dotgit.NewRepositoryFilesystem(repoFS, hostFS{root: lay.commonDir})
	}
	st := filesystem.NewStorageWithOptions(repoFS, cache.NewObjectLRUDefault(), filesystem.Options{
		AlternatesFS: hostFS{root: string(filepath.Separator)}, // alternates are absolute host paths
	})
	return git.Open(st, nil)
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
// HEAD points through an alias, and which exists or not (an unborn branch). git reads at most
// maxSymrefDepth refs along a chain and the last must be direct.
func (v *repoView) followRef(name string) (hash plumbing.Hash, found bool, final string, err error) {
	return v.followRefN(name, maxSymrefDepth)
}

func (v *repoView) followRefN(name string, reads int) (hash plumbing.Hash, found bool, final string, err error) {
	for range reads {
		ref, ok, err := v.readRef(name)
		if err != nil || !ok {
			return plumbing.ZeroHash, false, name, err
		}
		if !ref.symbolic() {
			return ref.hash, true, name, nil
		}
		name = ref.target
		if !strings.HasPrefix(name, "refs/") || !validRefName(name, false) {
			return plumbing.ZeroHash, false, name, fallbackEligible(fmt.Errorf("symbolic ref points at %q", name))
		}
	}
	// git reports a chain this long differently per command (dangling, "No such ref", ...)
	return plumbing.ZeroHash, false, name, fallbackEligible(fmt.Errorf("symbolic ref chain from %s is longer than %d reads", name, reads))
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
	data, err := readHeadFile(filepath.Join(v.lay.gitDir, "HEAD"))
	if err != nil {
		return headState{}, fallbackEligible(fmt.Errorf("read HEAD: %w", err))
	}
	ref, ok := parseRefContent(data)
	if !ok {
		return headState{}, fallbackEligible(errors.New("HEAD is not in a form git writes"))
	}
	if !ref.symbolic() {
		return headState{detached: true, hash: ref.hash}, nil
	}
	if !strings.HasPrefix(ref.target, "refs/") || !validRefName(ref.target, false) {
		return headState{}, fallbackEligible(fmt.Errorf("HEAD points at %q", ref.target))
	}
	hash, found, final, err := v.followRefN(ref.target, maxSymrefDepth-1) // reading HEAD was the first read
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
