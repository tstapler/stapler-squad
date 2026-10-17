package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// Atomic replace (ADR-031): write a temp file in the target's own directory (rename is
// atomic only within one filesystem), fsync it, chmod to the original mode, rename over
// the target and fsync the directory. Everything goes through os.Root, so nothing can
// land outside the worktree. Residual, accepted in ADR-031: a crash between create and
// rename leaves a .ssq-tmp-* file until the next write in that directory.

const (
	atomicTempMarker   = ".ssq-tmp-"
	atomicStaleAfter   = 10 * time.Minute
	atomicDefaultMode  = fs.FileMode(0o644)
	atomicTempMode     = fs.FileMode(0o600)
	atomicTempAttempts = 8
)

// isAtomicTempName matches ".<name>.ssq-tmp-<rand>".
func isAtomicTempName(name string) bool {
	return strings.HasPrefix(name, ".") && strings.Contains(name, atomicTempMarker)
}

// withoutAtomicTemps hides in-flight and orphaned temp files from listings. It returns
// entries itself when there is nothing to hide, so cached slices are never mutated.
func withoutAtomicTemps(entries []os.DirEntry) []os.DirEntry {
	for i, e := range entries {
		if !isAtomicTempName(e.Name()) {
			continue
		}
		kept := append([]os.DirEntry(nil), entries[:i]...)
		for _, rest := range entries[i+1:] {
			if !isAtomicTempName(rest.Name()) {
				kept = append(kept, rest)
			}
		}
		return kept
	}
	return entries
}

// atomicWriter replaces files under one os.Root. The fields after root are test seams.
type atomicWriter struct {
	root     *os.Root
	now      func() time.Time
	writeAll func(f *os.File, data []byte) error
}

func newAtomicWriter(root *WorktreeRoot) atomicWriter {
	return atomicWriter{root: root.root, now: time.Now, writeAll: func(f *os.File, data []byte) error {
		_, err := f.Write(data)
		return err
	}}
}

// atomicReplace writes data over rel (or creates it), keeping an existing file's mode.
// Callers hold the per-path lock (pathLocks) around their own check-then-write sequence.
func atomicReplace(root *WorktreeRoot, rel ResolvedPath, data []byte) error {
	return newAtomicWriter(root).replace(rel, data)
}

func (w atomicWriter) replace(rel ResolvedPath, data []byte) error {
	return w.replaceChecked(rel, data, nil)
}

// replaceChecked is replace with a hook that runs once the temp file is durable and
// just before the rename; its error aborts the swap (the temp is removed) and is returned as is.
func (w atomicWriter) replaceChecked(rel ResolvedPath, data []byte, beforeRename func() error) (err error) {
	dir, base := path.Split(string(rel))
	if dir == "" {
		dir = "."
	}
	mode, err := w.targetMode(string(rel))
	if err != nil {
		return err
	}
	w.sweepStale(dir)

	tmpRel, tmp, err := w.createTemp(dir, base)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = w.root.Remove(tmpRel)
		}
	}()
	if err := w.writeAll(tmp, data); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := w.root.Chmod(tmpRel, mode); err != nil {
		return fmt.Errorf("chmod temp: %w", err)
	}
	if beforeRename != nil {
		if err := beforeRename(); err != nil {
			return err
		}
	}
	if err := w.root.Rename(tmpRel, string(rel)); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	committed = true
	return w.syncDir(dir)
}

// targetMode returns the existing regular file's permission bits, or the default for a new file.
func (w atomicWriter) targetMode(rel string) (fs.FileMode, error) {
	info, err := w.root.Lstat(rel)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return atomicDefaultMode, nil
	case err != nil:
		return 0, fmt.Errorf("stat target: %w", err)
	case !info.Mode().IsRegular():
		return 0, fmt.Errorf("target is not a regular file: %s", rel)
	}
	return info.Mode().Perm(), nil
}

func (w atomicWriter) createTemp(dir, base string) (string, *os.File, error) {
	for range atomicTempAttempts {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", nil, fmt.Errorf("temp name: %w", err)
		}
		rel := path.Join(dir, "."+base+atomicTempMarker+hex.EncodeToString(suffix[:]))
		f, err := w.root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, atomicTempMode)
		if err == nil {
			return rel, f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, fmt.Errorf("create temp: %w", err)
		}
	}
	return "", nil, errors.New("create temp: name collisions")
}

// sweepStale removes orphaned temps older than atomicStaleAfter from dir only. Younger
// ones may belong to a concurrent writer. Best effort: failures never fail the write.
func (w atomicWriter) sweepStale(dir string) {
	d, err := w.root.Open(dir)
	if err != nil {
		return
	}
	entries, err := d.ReadDir(-1)
	_ = d.Close()
	if err != nil {
		return
	}
	cutoff := w.now().Add(-atomicStaleAfter)
	for _, e := range entries {
		if !isAtomicTempName(e.Name()) || !e.Type().IsRegular() {
			continue
		}
		if info, ierr := e.Info(); ierr == nil && info.ModTime().Before(cutoff) {
			_ = w.root.Remove(path.Join(dir, e.Name()))
		}
	}
}

// syncDir makes the rename durable. Opening a directory read-only and syncing it is
// supported on Linux (os_root_semantics_test.go) and macOS.
func (w atomicWriter) syncDir(dir string) error {
	d, err := w.root.Open(dir)
	if err != nil {
		return fmt.Errorf("open dir for sync: %w", err)
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync dir: %w", err)
	}
	return nil
}

// pathKey identifies one file for write serialization: two sessions editing README.md
// in different worktrees must not block each other.
type pathKey struct {
	ActiveDir string
	Path      ResolvedPath
}

// pathMutexRegistry hands out one mutex per pathKey and drops entries when idle, so it
// cannot grow without bound. It serializes writers inside this process only; an agent
// process writing the same file is not covered (ADR-031).
type pathMutexRegistry struct {
	mu      sync.Mutex
	entries map[pathKey]*pathLock
}

type pathLock struct {
	mu   sync.Mutex
	refs int
}

// pathLocks is the process-wide registry the FileWriter locks through.
var pathLocks = &pathMutexRegistry{}

// Lock blocks until key is free and returns its unlock.
func (r *pathMutexRegistry) Lock(key pathKey) (unlock func()) {
	r.mu.Lock()
	if r.entries == nil {
		r.entries = map[pathKey]*pathLock{}
	}
	l := r.entries[key]
	if l == nil {
		l = &pathLock{}
		r.entries[key] = l
	}
	l.refs++
	r.mu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		r.mu.Lock()
		defer r.mu.Unlock()
		if l.refs--; l.refs == 0 {
			delete(r.entries, key)
		}
	}
}

// size reports live entries; tests use it to prove idle entries are released.
func (r *pathMutexRegistry) size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}
