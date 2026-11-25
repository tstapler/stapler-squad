// Package memfs is an in-memory fsops.FSOps with injectable faults, for tests of the
// packages that consume the fsops kernel.
package memfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/services/fsops"
)

// MemFS stores files in memory. A stalled fsync is a channel the test closes;
// nothing here sleeps.
type MemFS struct {
	mu    sync.Mutex
	files map[string][]byte
	Modes map[string]os.FileMode

	FailMkdir  error
	failWrite  error // WriteFileSync and append Write; set via SetFailWrite, read via writeFault
	FailRename error
	// SyncStall, when non-nil, blocks every fsync (WriteFileSync and append
	// Sync) until it is closed; SyncEntered receives one value per blocked call.
	SyncStall   chan struct{}
	SyncEntered chan struct{}
}

// SetFailWrite arms (nil disarms) the write fault; with writeFault it guards failWrite: the audit sink's drain goroutine
// reads it while a test goroutine arms it.
func (m *MemFS) SetFailWrite(err error) {
	m.mu.Lock()
	m.failWrite = err
	m.mu.Unlock()
}

func (m *MemFS) writeFault() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failWrite
}

// New returns an empty MemFS.
func New() *MemFS {
	return &MemFS{files: map[string][]byte{}, Modes: map[string]os.FileMode{}}
}

// Get returns a copy of the file at path.
func (m *MemFS) Get(path string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[path]
	return append([]byte(nil), b...), ok
}

// Put stores a copy of data at path.
func (m *MemFS) Put(path string, data []byte) {
	m.mu.Lock()
	m.files[path] = append([]byte(nil), data...)
	m.mu.Unlock()
}

// Names lists the stored paths with the prefix, sorted.
func (m *MemFS) Names(prefix string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for p := range m.files {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func (m *MemFS) stall() {
	if m.SyncStall == nil {
		return
	}
	if m.SyncEntered != nil {
		select {
		case m.SyncEntered <- struct{}{}:
		default: // never block a stalled call on an unread signal
		}
	}
	<-m.SyncStall
}

type memInfo struct {
	name string
	size int64
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return 0o600 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }

func (m *MemFS) MkdirAll(string, os.FileMode) error { return m.FailMkdir }

func (m *MemFS) Stat(path string) (os.FileInfo, error) {
	b, ok := m.Get(path)
	if !ok {
		return nil, os.ErrNotExist
	}
	return memInfo{name: filepath.Base(path), size: int64(len(b))}, nil
}

func (m *MemFS) ReadFile(path string) ([]byte, error) {
	b, ok := m.Get(path)
	if !ok {
		return nil, os.ErrNotExist
	}
	return b, nil
}

func (m *MemFS) WriteFileSync(path string, data []byte, perm os.FileMode) error {
	if err := m.writeFault(); err != nil {
		return err
	}
	m.stall()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.files[path]; exists {
		return os.ErrExist
	}
	m.files[path] = append([]byte(nil), data...)
	m.Modes[path] = perm
	return nil
}

func (m *MemFS) Rename(oldpath, newpath string) error {
	if m.FailRename != nil {
		return m.FailRename
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[oldpath]
	if !ok {
		return os.ErrNotExist
	}
	m.files[newpath] = b
	m.Modes[newpath] = m.Modes[oldpath]
	delete(m.files, oldpath)
	return nil
}

func (m *MemFS) Remove(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[path]; !ok {
		return os.ErrNotExist
	}
	delete(m.files, path)
	return nil
}

func (m *MemFS) Glob(pattern string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for p := range m.files {
		if ok, _ := filepath.Match(pattern, p); ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

type memAppend struct {
	m    *MemFS
	path string
}

func (m *MemFS) OpenAppend(path string, perm os.FileMode) (fsops.AppendFile, error) {
	m.mu.Lock()
	if _, ok := m.files[path]; !ok {
		m.files[path] = nil
		m.Modes[path] = perm
	}
	m.mu.Unlock()
	return &memAppend{m: m, path: path}, nil
}

func (a *memAppend) Write(p []byte) (int, error) {
	if err := a.m.writeFault(); err != nil {
		return 0, err
	}
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	a.m.files[a.path] = append(a.m.files[a.path], p...)
	return len(p), nil
}

func (a *memAppend) Sync() error {
	a.m.stall()
	return nil
}

func (a *memAppend) Close() error { return nil }
