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
	t.mkParents(rel)
	t.nodes[rel] = &memNode{data: append([]byte(nil), data...), mode: 0o644, mtime: mtime}
}

// AddSymlink creates dir/rel as a symlink to target.
func (m *memBackend) AddSymlink(dir, rel, target string) {
	t := m.tree(dir)
	t.mkParents(rel)
	t.nodes[rel] = &memNode{mode: fs.ModeSymlink | 0o777, target: target}
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
	return &memFileHandle{name: path.Base(string(p)), n: n, r: bytes.NewReader(n.data), short: r.b.ShortRead}, nil
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
	name  string
	n     *memNode
	r     *bytes.Reader
	short int
}

func (h *memFileHandle) clip(p []byte) []byte {
	if h.short > 0 && len(p) > h.short {
		return p[:h.short]
	}
	return p
}

func (h *memFileHandle) Read(p []byte) (int, error)                { return h.r.Read(h.clip(p)) }
func (h *memFileHandle) ReadAt(p []byte, off int64) (int, error)   { return h.r.ReadAt(p, off) }
func (h *memFileHandle) Seek(off int64, whence int) (int64, error) { return h.r.Seek(off, whence) }
func (h *memFileHandle) Close() error                              { return nil }
func (h *memFileHandle) Stat() (os.FileInfo, error)                { return memFileInfo{name: h.name, n: h.n}, nil }
