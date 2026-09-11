package git

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// headTreeHashes walks repo's HEAD tree and returns a path -> blob hash map, without
// loading any blob content. object.NewTreeWalker visits tree objects only; the more
// obvious object.Tree.Files()/FileIter alternative calls GetBlob() per entry and loads
// full file content, which is exactly the generic-tree-diff cost this file exists to
// avoid (see worktreeIsDirtyFast's doc comment). Returns (nil, nil) for an unborn HEAD
// (no commits yet) — a valid "empty tree" result, not an error.
func headTreeHashes(repo *git.Repository) (map[string]plumbing.Hash, error) {
	headRef, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, nil //nolint:nilnil // documented sentinel: unborn HEAD has no tree to compare against
		}
		return nil, fmt.Errorf("head: %w", err)
	}
	headCommit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("head commit: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("head tree: %w", err)
	}

	hashes := make(map[string]plumbing.Hash, len(headTree.Entries))
	tw := object.NewTreeWalker(headTree, true, nil)
	defer tw.Close()
	for {
		name, te, twErr := tw.Next()
		if errors.Is(twErr, io.EOF) {
			break
		}
		if twErr != nil {
			return nil, fmt.Errorf("walk head tree: %w", twErr)
		}
		if te.Mode == filemode.Dir {
			continue
		}
		hashes[name] = te.Hash
	}
	return hashes, nil
}

// worktreeStagedDirty reports whether the index differs from HEAD -- a staged
// addition, modification, deletion, or an unresolved merge conflict stage.
// O(index size) hash comparisons, zero file I/O. headHashes == nil (unborn HEAD)
// is treated as "nothing to compare against yet", matching
// session/unfinished.GoGitVCSReader.hasUncommittedGoGitPhase's identical rule.
func worktreeStagedDirty(idx *index.Index, headHashes map[string]plumbing.Hash) bool {
	if headHashes == nil {
		return false
	}
	indexNames := make(map[string]struct{}, len(idx.Entries))
	for _, e := range idx.Entries {
		if e.Stage != 0 {
			return true // unresolved merge conflict
		}
		indexNames[e.Name] = struct{}{}
		if h, ok := headHashes[e.Name]; !ok || h != e.Hash {
			return true // new or modified staged file
		}
	}
	for name := range headHashes {
		if _, ok := indexNames[name]; !ok {
			return true // staged deletion
		}
	}
	return false
}

// worktreeUnstagedDirty reports whether any tracked file's on-disk size or full-precision
// mtime differs from its index record. O(index size) stat calls, no file reads/hashing.
func worktreeUnstagedDirty(worktreePath string, idx *index.Index) (bool, error) {
	for _, e := range idx.Entries {
		info, statErr := os.Lstat(filepath.Join(worktreePath, e.Name))
		if statErr != nil {
			if os.IsNotExist(statErr) {
				return true, nil // tracked file deleted from the worktree
			}
			return false, fmt.Errorf("stat %s: %w", e.Name, statErr)
		}
		if info.Size() != int64(e.Size) || !info.ModTime().Equal(e.ModifiedAt) {
			return true, nil
		}
	}
	return false, nil
}

// worktreeHasUntrackedFiles reports whether any file under worktreePath is absent from
// indexed and not matched by matcher (gitignored). matcher == nil disables gitignore
// filtering. Mirrors session/unfinished.hasUntrackedFilesRec.
func worktreeHasUntrackedFiles(worktreePath string, indexed map[string]struct{}, matcher gitignore.Matcher) (bool, error) {
	return worktreeHasUntrackedFilesRec(worktreePath, worktreePath, indexed, matcher)
}

func worktreeHasUntrackedFilesRec(root, dir string, indexed map[string]struct{}, matcher gitignore.Matcher) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read dir %s: %w", dir, err)
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		full := filepath.Join(dir, name)
		rel, relErr := filepath.Rel(root, full)
		if relErr != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if matcher != nil && matcher.Match(strings.Split(rel, "/"), de.IsDir()) {
			continue // gitignored — for a dir this skips the whole subtree
		}
		if de.IsDir() {
			has, err := worktreeHasUntrackedFilesRec(root, full, indexed, matcher)
			if err != nil {
				return false, err // already wrapped with path context at the failing os.ReadDir call
			}
			if has {
				return true, nil
			}
			continue
		}
		if _, ok := indexed[rel]; !ok {
			return true, nil
		}
	}
	return false, nil
}

// worktreeIsDirtyFast reports whether the worktree at path has any staged or unstaged
// change, via the same mtime/hash short-circuit strategy as
// session/unfinished.GoGitVCSReader.HasUncommitted, instead of go-git's
// Worktree.Status(). Status() always computes a *full* tree diff -- building
// noder-wrapped trees for HEAD, the index, and the worktree, then recursively
// comparing every node via package merkletrie -- to answer a question this function
// only needs as a boolean. Live production profiling (Pyroscope, 30min window) showed
// merkletrie.DiffTree/diffNodes at 16.58% cum CPU inside worktreeIsDirtyWithFS, larger
// than the gitignore-pattern cost gitignoreFSCache already fixed. session/git can't
// import session/unfinished's GoGitVCSReader directly (its test files already import
// session/git, which would make that direction a cycle), so this is a self-contained
// reimplementation of the same proven, tested algorithm rather than a shared import.
//
// cache (may be nil) is reused for the gitignore-pattern filesystem reads the
// untracked-files walk needs — see gitignoreFSCache's doc comment.
func worktreeIsDirtyFast(path string, cache *gitignoreFSCache) (bool, error) {
	repo, err := OpenRepo(path)
	if err != nil {
		return false, fmt.Errorf("failed to open git repo at %s: %w", path, err)
	}

	idx, err := repo.Storer.Index()
	if err != nil {
		return false, fmt.Errorf("read index at %s: %w", path, err)
	}

	headHashes, err := headTreeHashes(repo)
	if err != nil {
		return false, fmt.Errorf("resolve head tree at %s: %w", path, err)
	}

	if worktreeStagedDirty(idx, headHashes) {
		return true, nil
	}

	unstagedDirty, err := worktreeUnstagedDirty(path, idx)
	if err != nil {
		return false, fmt.Errorf("check unstaged changes at %s: %w", path, err)
	}
	if unstagedDirty {
		return true, nil
	}

	indexed := make(map[string]struct{}, len(idx.Entries))
	for _, e := range idx.Entries {
		indexed[e.Name] = struct{}{}
	}
	return worktreeHasUntrackedFiles(path, indexed, worktreeUntrackedMatcher(path, cache))
}

// worktreeUntrackedMatcher builds the gitignore matcher worktreeHasUntrackedFiles uses
// to skip ignored files/subtrees. Returns nil (matches nothing) if patterns can't be
// read, matching worktreeIsDirty's existing tolerance for a missing/unreadable
// .gitignore rather than failing the whole dirty check over it.
func worktreeUntrackedMatcher(path string, cache *gitignoreFSCache) gitignore.Matcher {
	fs := newCachedFilesystem(osfs.New(path), cache)
	patterns, err := gitignore.ReadPatterns(fs, nil)
	if err != nil {
		return nil
	}
	return gitignore.NewMatcher(patterns)
}
