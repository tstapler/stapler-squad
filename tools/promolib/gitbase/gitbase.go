// Package gitbase gives the promo tools read-only, go-git based access to a
// repository: the index listing, the PR's changed files against a base commit,
// and blob reads at the base. It never shells out to the git CLI.
//
// In CI the pull_request checkout is the merge commit, whose first parent is the
// base-branch tip, so the default base HEAD^1 yields exactly the PR's changes.
package gitbase

import (
	"errors"
	"fmt"
	"io"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/merkletrie"

	sessiongit "github.com/tstapler/stapler-squad/session/git"
)

// DefaultBase is the first parent of the pull_request merge ref.
const DefaultBase = "HEAD^1"

// ErrNotFound reports a path absent from the base tree. Gate inputs map it to defaults.
var ErrNotFound = errors.New("gitbase: path not found in base tree")

// WrongBaseError is returned when the default base HEAD^1 is used on a HEAD that
// is not a merge commit (a push event or a re-run on a head SHA).
type WrongBaseError struct{ Parents int }

func (e *WrongBaseError) Error() string {
	return fmt.Sprintf("HEAD has %d parent(s) but the default -base %s needs a pull_request merge commit (2 parents); "+
		"this is not a pull_request merge ref, pass an explicit -base <sha>", e.Parents, DefaultBase)
}

// Repo is an opened repository.
type Repo struct{ repo *git.Repository }

// Open opens the repository at path through session/git.OpenRepo.
func Open(path string) (*Repo, error) {
	r, err := sessiongit.OpenRepo(path)
	if err != nil {
		return nil, fmt.Errorf("open repository %q: %w", path, err)
	}
	return &Repo{repo: r}, nil
}

// Files lists every file path in the index, skipping gitlinks (submodules).
func (r *Repo) Files() ([]string, error) {
	idx, err := r.repo.Storer.Index()
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	files := make([]string, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		if e.Mode == filemode.Submodule {
			continue // gitlink: a directory in the working tree, not a readable file
		}
		files = append(files, e.Name)
	}
	return files, nil
}

func (r *Repo) head() (*object.Commit, error) {
	ref, err := r.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	c, err := r.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("load HEAD commit: %w", err)
	}
	return c, nil
}

// ResolveBase turns a -base value into a commit. Empty or DefaultBase means the
// first parent of HEAD and fails with *WrongBaseError unless HEAD has two parents.
func (r *Repo) ResolveBase(base string) (*object.Commit, error) {
	if base == "" || base == DefaultBase {
		head, err := r.head()
		if err != nil {
			return nil, err
		}
		if head.NumParents() < 2 {
			return nil, &WrongBaseError{Parents: head.NumParents()}
		}
		return head.Parent(0)
	}
	h, err := r.repo.ResolveRevision(plumbing.Revision(base))
	if err != nil {
		return nil, fmt.Errorf("resolve -base %q: %w", base, err)
	}
	c, err := r.repo.CommitObject(*h)
	if err != nil {
		return nil, fmt.Errorf("load -base commit %q: %w", base, err)
	}
	return c, nil
}

// MergeBaseWith returns the merge-base of HEAD and rev (local default: origin/main).
func (r *Repo) MergeBaseWith(rev string) (string, error) {
	head, err := r.head()
	if err != nil {
		return "", err
	}
	h, err := r.repo.ResolveRevision(plumbing.Revision(rev))
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", rev, err)
	}
	other, err := r.repo.CommitObject(*h)
	if err != nil {
		return "", fmt.Errorf("load %q: %w", rev, err)
	}
	bases, err := head.MergeBase(other)
	if err != nil {
		return "", fmt.Errorf("merge-base of HEAD and %q: %w", rev, err)
	}
	if len(bases) == 0 {
		return "", fmt.Errorf("no merge-base between HEAD and %q", rev)
	}
	return bases[0].Hash.String(), nil
}

// FileChange is one added or modified path at HEAD.
type FileChange struct {
	Path  string
	Size  int64
	Added bool // false means modified
}

// Changes is the tree diff of base against HEAD. A rename appears as one
// deletion plus one addition.
type Changes struct {
	Changed []FileChange
	Deleted []string
}

// Changes diffs base against HEAD by path.
func (r *Repo) Changes(base string) (Changes, error) {
	var out Changes
	baseCommit, err := r.ResolveBase(base)
	if err != nil {
		return out, err
	}
	head, err := r.head()
	if err != nil {
		return out, err
	}
	baseTree, err := baseCommit.Tree()
	if err != nil {
		return out, fmt.Errorf("load base tree: %w", err)
	}
	headTree, err := head.Tree()
	if err != nil {
		return out, fmt.Errorf("load HEAD tree: %w", err)
	}
	diff, err := object.DiffTree(baseTree, headTree)
	if err != nil {
		return out, fmt.Errorf("diff trees: %w", err)
	}
	for _, c := range diff {
		action, err := c.Action()
		if err != nil {
			return out, fmt.Errorf("classify change: %w", err)
		}
		if err := out.add(headTree, c, action); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c *Changes) add(headTree *object.Tree, ch *object.Change, action merkletrie.Action) error {
	switch action {
	case merkletrie.Delete:
		c.Deleted = append(c.Deleted, ch.From.Name)
	case merkletrie.Insert, merkletrie.Modify:
		f, err := headTree.File(ch.To.Name)
		if err != nil {
			return fmt.Errorf("stat %q at HEAD: %w", ch.To.Name, err)
		}
		c.Changed = append(c.Changed, FileChange{Path: ch.To.Name, Size: f.Size, Added: action == merkletrie.Insert})
	}
	return nil
}

// ReadBlob returns path's content in the base tree, or ErrNotFound. Gate inputs
// are read this way so a PR cannot change the rules it is judged by.
func (r *Repo) ReadBlob(base, path string) ([]byte, error) {
	baseCommit, err := r.ResolveBase(base)
	if err != nil {
		return nil, err
	}
	f, err := baseCommit.File(path)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) || errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, object.ErrEntryNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("read %q at base: %w", path, err)
	}
	rd, err := f.Reader()
	if err != nil {
		return nil, fmt.Errorf("open %q at base: %w", path, err)
	}
	defer rd.Close()
	return io.ReadAll(rd)
}

// ListAt lists every file under the directory prefix (no trailing slash) in the
// base tree, as repo-relative paths. A missing directory yields an empty list.
func (r *Repo) ListAt(base, prefix string) ([]string, error) {
	baseCommit, err := r.ResolveBase(base)
	if err != nil {
		return nil, err
	}
	tree, err := baseCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("load base tree: %w", err)
	}
	sub, err := tree.Tree(prefix)
	if err != nil {
		if errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, object.ErrEntryNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %q at base: %w", prefix, err)
	}
	var out []string
	it := sub.Files()
	defer it.Close()
	for {
		f, err := it.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("walk %q at base: %w", prefix, err)
		}
		out = append(out, prefix+"/"+f.Name)
	}
}
