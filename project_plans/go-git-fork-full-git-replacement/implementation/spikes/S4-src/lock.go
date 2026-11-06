// Package s4lock is a throwaway spike of an operation-scoped, CLI-compatible
// index.lock (and buffered <ref>.lock) around whole go-git operations.
package s4lock

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// ErrLocked mirrors git's "Unable to create index.lock: File exists".
type ErrLocked struct {
	Path string
	Age  time.Duration
}

func (e *ErrLocked) Error() string {
	return fmt.Sprintf("lock held: %s (age %s)", e.Path, e.Age.Round(time.Millisecond))
}

type Options struct {
	// NoReadYourWrites reproduces the naive design: lock taken, but Index()
	// always reads the real index. Used only to show the stale-read hazard.
	NoReadYourWrites bool
	// BeforeCommitPhase is a test hook run after fn succeeded, before any rename.
	BeforeCommitPhase func() error
	// BetweenRenames is a test hook run after the index rename, before ref renames.
	BetweenRenames func()
}

type bufRef struct {
	ref  *plumbing.Reference
	lock string // relative path of <ref>.lock inside dotgit fs
}

// scopedStorer wraps *filesystem.Storage; only Index/SetIndex and the ref
// writers are overridden. Everything else (objects, config) is promoted.
type scopedStorer struct {
	*filesystem.Storage
	fs   billy.Filesystem
	opts Options

	mu        sync.Mutex
	lockFile  billy.File
	wroteIdx  bool
	refs      map[plumbing.ReferenceName]*bufRef
	refOrder  []plumbing.ReferenceName
	created   []string // relative lock paths we created
	committed bool     // set before first rename (ambiguity rule)
}

const indexLockName = "index.lock"

func (s *scopedStorer) acquire(name string) (billy.File, error) {
	f, err := s.fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			age := time.Duration(0)
			if fi, serr := s.fs.Stat(name); serr == nil {
				age = time.Since(fi.ModTime())
			}
			return nil, &ErrLocked{Path: s.fs.Join(s.fs.Root(), name), Age: age}
		}
		return nil, err
	}
	s.created = append(s.created, name)
	return f, nil
}

func (s *scopedStorer) Index() (*index.Index, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.wroteIdx || s.opts.NoReadYourWrites {
		return s.Storage.Index()
	}
	// Decode the pending content from the lock file every time.
	f, err := s.fs.Open(indexLockName)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	idx := &index.Index{Version: 2}
	if err := index.NewDecoder(f).Decode(idx); err != nil {
		return nil, err
	}
	return idx, nil
}

func (s *scopedStorer) SetIndex(idx *index.Index) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.lockFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := s.lockFile.Truncate(0); err != nil {
		return err
	}
	bw := bufio.NewWriter(s.lockFile)
	if err := index.NewEncoder(bw).Encode(idx); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	s.wroteIdx = true
	return nil
}

// --- buffered refs (main-repo layout only in this spike) ---

func (s *scopedStorer) bufferRef(ref *plumbing.Reference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.refs[ref.Name()]
	if !ok {
		lock := string(ref.Name()) + ".lock"
		if err := s.fs.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
			return err
		}
		f, err := s.acquire(lock)
		if err != nil {
			return err
		}
		f.Close()
		b = &bufRef{lock: lock}
		s.refs[ref.Name()] = b
		s.refOrder = append(s.refOrder, ref.Name())
	}
	b.ref = ref
	var content string
	if ref.Type() == plumbing.SymbolicReference {
		content = "ref: " + ref.Target().String() + "\n"
	} else {
		content = ref.Hash().String() + "\n"
	}
	f, err := s.fs.OpenFile(b.lock, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write([]byte(content)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (s *scopedStorer) SetReference(ref *plumbing.Reference) error { return s.bufferRef(ref) }

func (s *scopedStorer) CheckAndSetReference(n, old *plumbing.Reference) error {
	if old != nil {
		cur, err := s.Reference(n.Name())
		if err != nil || cur.Hash() != old.Hash() {
			return storage_ErrReferenceHasChanged
		}
	}
	return s.bufferRef(n)
}

var storage_ErrReferenceHasChanged = errors.New("reference has changed concurrently")

func (s *scopedStorer) Reference(n plumbing.ReferenceName) (*plumbing.Reference, error) {
	s.mu.Lock()
	b, ok := s.refs[n]
	s.mu.Unlock()
	if ok {
		return b.ref, nil
	}
	return s.Storage.Reference(n)
}

func (s *scopedStorer) cleanup() {
	if s.lockFile != nil {
		s.lockFile.Close()
	}
	for _, p := range s.created {
		_ = s.fs.Remove(p)
	}
	s.created = nil
}

func syncFile(f billy.File) error {
	if sf, ok := f.(interface{ Sync() error }); ok {
		return sf.Sync()
	}
	return nil
}

// Scope is handed to fn.
type Scope struct {
	Repo *git.Repository
	WT   *git.Worktree
	s    *scopedStorer
}

// Wrote reports whether the commit phase began (set BEFORE the first rename).
func (sc *Scope) Wrote() bool { return sc.s.committed }

// WithIndexLock takes index.lock O_EXCL before fn, runs fn against a scoped
// storer, then (on success) fsync+rename index first, then buffered refs.
// Any error/panic before the first rename removes only locks this scope created.
// Never retries and never deletes a pre-existing lock.
func WithIndexLock(repoPath string, opts Options, fn func(*Scope) error) (retErr error) {
	base, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}
	st, ok := base.Storer.(*filesystem.Storage)
	if !ok {
		return errors.New("not a filesystem storer")
	}
	wt, err := base.Worktree()
	if err != nil {
		return err
	}
	s := &scopedStorer{Storage: st, fs: st.Filesystem(), opts: opts, refs: map[plumbing.ReferenceName]*bufRef{}}
	lf, err := s.acquire(indexLockName)
	if err != nil {
		return err
	}
	s.lockFile = lf

	defer func() {
		if p := recover(); p != nil {
			s.cleanup()
			panic(p)
		}
		if retErr != nil {
			s.cleanup() // created[] only holds locks not yet renamed
		}
	}()

	repo, err := git.Open(s, wt.Filesystem)
	if err != nil {
		return err
	}
	w, err := repo.Worktree()
	if err != nil {
		return err
	}
	if err := fn(&Scope{Repo: repo, WT: w, s: s}); err != nil {
		return err
	}
	if opts.BeforeCommitPhase != nil {
		if err := opts.BeforeCommitPhase(); err != nil {
			return err
		}
	}

	// Commit phase. Wrote()=true BEFORE the first rename (a failed rename is ambiguous).
	s.committed = true
	if err := syncFile(s.lockFile); err != nil {
		s.committed = false
		return err
	}
	s.lockFile.Close()
	s.lockFile = nil
	for _, n := range s.refOrder { // fsync ref locks
		f, err := s.fs.OpenFile(s.refs[n].lock, os.O_RDWR, 0)
		if err != nil {
			s.committed = false
			return err
		}
		_ = syncFile(f)
		f.Close()
	}
	if s.wroteIdx {
		if err := s.fs.Rename(indexLockName, "index"); err != nil {
			return err
		}
		s.created = removeStr(s.created, indexLockName)
	} else {
		_ = s.fs.Remove(indexLockName) // read-only scope: index untouched
		s.created = removeStr(s.created, indexLockName)
	}
	if opts.BetweenRenames != nil {
		opts.BetweenRenames()
	}
	order := append([]plumbing.ReferenceName(nil), s.refOrder...)
	sort.SliceStable(order, func(i, j int) bool { // branches first, HEAD last
		return order[i] != plumbing.HEAD && order[j] == plumbing.HEAD
	})
	for _, n := range order {
		b := s.refs[n]
		if err := s.fs.Rename(b.lock, string(n)); err != nil {
			return err
		}
		s.created = removeStr(s.created, b.lock)
	}
	return nil
}

func removeStr(l []string, v string) []string {
	out := l[:0]
	for _, x := range l {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
