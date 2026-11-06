package services

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path"
	"sync"
	"time"
)

// memBackend is an in-memory FileBackend for tests, with an injectable short-read fault.
type memBackend struct {
	mu    sync.Mutex
	roots map[string]*memTree
	// ShortRead, when > 0, caps the bytes any single Read returns (ReadAt must stay whole by contract).
	ShortRead int
	// onRead, when set, runs before every handle Read with the buffer length (fault
	// injection: change the file mid-pass). Set it through SetOnRead; Open reads it under mu.
	onRead func(readLen int)
}

type memNode struct {
	data   []byte
	mode   os.FileMode
	mtime  time.Time
	target string
}

type memTree struct{ nodes map[string]*memNode }

func newMemBackend() *memBackend { return &memBackend{roots: map[string]*memTree{}} }

func (m *memBackend) tree(dir string) *memTree {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.roots[dir]
	if !ok {
		t = &memTree{nodes: map[string]*memNode{".": {mode: fs.ModeDir | 0o755}}}
		m.roots[dir] = t
	}
	return t
}

func (t *memTree) mkParents(rel string) {
	for d := path.Dir(rel); d != "." && d != "/"; d = path.Dir(d) {
		if _, ok := t.nodes[d]; !ok {
			t.nodes[d] = &memNode{mode: fs.ModeDir | 0o755}
		}
	}
}

// AddFile creates dir/rel (and its parents) with the given bytes and mtime.
func (m *memBackend) AddFile(dir, rel string, data []byte, mtime time.Time) {
	t := m.tree(dir)
	m.mu.Lock()
	defer m.mu.Unlock()
	t.mkParents(rel)
	t.nodes[rel] = &memNode{data: append([]byte(nil), data...), mode: 0o644, mtime: mtime}
}

// AddSymlink creates dir/rel as a symlink to target.
func (m *memBackend) AddSymlink(dir, rel, target string) {
	t := m.tree(dir)
	t.mkParents(rel)
	t.nodes[rel] = &memNode{mode: fs.ModeSymlink | 0o777, target: target}
}

// SetOnRead installs (or, with nil, clears) the pre-Read fault-injection hook.
func (m *memBackend) SetOnRead(fn func(readLen int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRead = fn
}

func (m *memBackend) readHook() func(int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.onRead
}

func (m *memBackend) OpenRoot(dir string) (FileRoot, error) {
	m.mu.Lock()
	t, ok := m.roots[dir]
	m.mu.Unlock()
	if !ok {
		return nil, os.ErrNotExist
	}
	return &memRoot{b: m, t: t}, nil
}

type memRoot struct {
	b *memBackend
	t *memTree
}

func (r *memRoot) Close() error { return nil }

func (r *memRoot) node(rel string) (*memNode, error) {
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	n, ok := r.t.nodes[path.Clean(rel)]
	if !ok {
		return nil, &fs.PathError{Op: "lstat", Path: rel, Err: fs.ErrNotExist}
	}
	return n, nil
}

func (r *memRoot) lstatRel(rel string) (os.FileInfo, error) {
	n, err := r.node(rel)
	if err != nil {
		return nil, err
	}
	return memFileInfo{name: path.Base(rel), n: n}, nil
}

func (r *memRoot) Lstat(p ResolvedPath) (os.FileInfo, error) { return r.lstatRel(string(p)) }

func (r *memRoot) Readlink(rel string) (string, error) {
	n, err := r.node(rel)
	if err != nil {
		return "", err
	}
	if n.mode&fs.ModeSymlink == 0 {
		return "", &fs.PathError{Op: "readlink", Path: rel, Err: errors.New("not a symlink")}
	}
	return n.target, nil
}

func (r *memRoot) Open(p ResolvedPath) (FileHandle, error) {
	n, err := r.node(string(p))
	if err != nil {
		return nil, err
	}
	if n.mode.IsDir() {
		return nil, &fs.PathError{Op: "open", Path: string(p), Err: errors.New("is a directory")}
	}
	return &memFileHandle{name: path.Base(string(p)), n: n, r: bytes.NewReader(n.data), short: r.b.ShortRead, onRead: r.b.readHook()}, nil
}

type memFileInfo struct {
	name string
	n    *memNode
}

func (i memFileInfo) Name() string       { return i.name }
func (i memFileInfo) Size() int64        { return int64(len(i.n.data)) }
func (i memFileInfo) Mode() fs.FileMode  { return i.n.mode }
func (i memFileInfo) ModTime() time.Time { return i.n.mtime }
func (i memFileInfo) IsDir() bool        { return i.n.mode.IsDir() }
func (i memFileInfo) Sys() any           { return nil }

type memFileHandle struct {
	name   string
	n      *memNode
	r      *bytes.Reader
	short  int
	onRead func(readLen int)
}

func (h *memFileHandle) clip(p []byte) []byte {
	if h.short > 0 && len(p) > h.short {
		return p[:h.short]
	}
	return p
}

func (h *memFileHandle) Read(p []byte) (int, error) {
	if h.onRead != nil {
		h.onRead(len(p))
	}
	return h.r.Read(h.clip(p))
}

func (h *memFileHandle) ReadAt(p []byte, off int64) (int, error)   { return h.r.ReadAt(p, off) }
func (h *memFileHandle) Seek(off int64, whence int) (int64, error) { return h.r.Seek(off, whence) }
func (h *memFileHandle) Close() error                              { return nil }
func (h *memFileHandle) Stat() (os.FileInfo, error)                { return memFileInfo{name: h.name, n: h.n}, nil }

// Replace swaps the node's bytes after the pre-rename check, keeping its mode.
func (r *memRoot) Replace(p ResolvedPath, data []byte, beforeRename func() error) error {
	if beforeRename != nil {
		if err := beforeRename(); err != nil {
			return err
		}
	}
	r.b.mu.Lock()
	defer r.b.mu.Unlock()
	key := path.Clean(string(p))
	mode := os.FileMode(0o644)
	if old, ok := r.t.nodes[key]; ok {
		if !old.mode.IsRegular() {
			return &fs.PathError{Op: "replace", Path: key, Err: errors.New("not a regular file")}
		}
		mode = old.mode
	}
	r.t.mkParents(key)
	r.t.nodes[key] = &memNode{data: append([]byte(nil), data...), mode: mode, mtime: time.Now()}
	return nil
}
