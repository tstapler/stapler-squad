package unfinished

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// hasUntrackedFiles reports whether any file under root is absent from the indexed set.
// It skips the .git directory and, when matcher is non-nil, skips gitignored files and
// whole subtrees (matcher is built from the worktree's .gitignore files by
// getOrBuildUntrackedMatcher — pass nil to disable gitignore filtering).
// For the mtime-stat approach this is a best-effort check sufficient for typical use.
func hasUntrackedFiles(root string, indexed map[string]struct{}, matcher gitignore.Matcher) (bool, error) {
	w := untrackedWalker{indexed: indexed, matcher: matcher, buf: make([]byte, 0, 256)}
	return w.hasUntracked(root)
}

// untrackedWalker threads one path-parts slice and one byte buffer through the walk
// instead of allocating a relative path (filepath.Rel + strings.Split + a joined
// string) per entry: the matcher takes the parts, and the indexed-set lookup uses
// string(buf), which Go does not allocate for a map index. Single-goroutine,
// depth-first: a child's append/truncate completes before its sibling reuses the slot.
type untrackedWalker struct {
	indexed map[string]struct{}
	matcher gitignore.Matcher
	parts   []string
	buf     []byte
}

// enter pushes name onto the relative-path state and returns a func-free undo token.
func (w *untrackedWalker) enter(name string) (prevBuf int) {
	w.parts = append(w.parts, name)
	prevBuf = len(w.buf)
	if prevBuf > 0 {
		w.buf = append(w.buf, '/')
	}
	w.buf = append(w.buf, name...)
	return prevBuf
}

func (w *untrackedWalker) leave(prevBuf int) {
	w.parts = w.parts[:len(w.parts)-1]
	w.buf = w.buf[:prevBuf]
}

func (w *untrackedWalker) hasUntracked(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		prev := w.enter(name)
		isDir := de.IsDir()
		found, err := false, error(nil)
		switch {
		case w.matcher != nil && w.matcher.Match(w.parts, isDir):
			// gitignored — for a dir this skips the whole subtree
		case isDir:
			found, err = w.hasUntracked(filepath.Join(dir, name))
		default:
			_, tracked := w.indexed[string(w.buf)]
			found = !tracked
		}
		w.leave(prev)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// walkUntracked calls fn for every file under root that is not in indexed and
// not matched by matcher (pass nil to skip gitignore filtering entirely).
// Skips the .git directory. Stops after maxUntrackedFiles files to avoid
// unbounded walks in large build trees.
func walkUntracked(root string, indexed map[string]struct{}, matcher gitignore.Matcher, fn func(absPath string)) error {
	w := untrackedWalker{indexed: indexed, matcher: matcher, buf: make([]byte, 0, 256)}
	count := 0
	err := w.walk(root, &count, fn)
	if errors.Is(err, errStopWalk) {
		return nil
	}
	return err
}

// walk recurses depth-first over untrackedWalker's shared parts/buf state (see its
// doc comment for why reusing them across siblings is safe).
func (w *untrackedWalker) walk(dir string, count *int, fn func(string)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, de := range entries {
		name := de.Name()
		if name == ".git" {
			continue
		}
		prev := w.enter(name)
		isDir := de.IsDir()
		var walkErr error
		switch {
		case w.matcher != nil && w.matcher.Match(w.parts, isDir):
			// gitignored — for a dir this skips the whole subtree
		case isDir:
			walkErr = w.walk(filepath.Join(dir, name), count, fn)
		default:
			if _, tracked := w.indexed[string(w.buf)]; !tracked {
				*count++
				if *count > maxUntrackedFiles {
					walkErr = errStopWalk
				} else {
					fn(filepath.Join(dir, name))
				}
			}
		}
		w.leave(prev)
		if walkErr != nil {
			return walkErr
		}
	}
	return nil
}
