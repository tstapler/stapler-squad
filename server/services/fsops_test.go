package services

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// memFS is an in-memory fsOps with injectable faults. A stalled fsync is a
// channel the test closes; nothing here sleeps.
type memFS struct {
	mu    sync.Mutex
	files map[string][]byte
	modes map[string]os.FileMode

	failMkdir  error
	failWrite  error // WriteFileSync and append Write
	failRename error
	// syncStall, when non-nil, blocks every fsync (WriteFileSync and append
	// Sync) until it is closed; syncEntered receives one value per blocked call.
	syncStall   chan struct{}
	syncEntered chan struct{}
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}, modes: map[string]os.FileMode{}}
}

func (m *memFS) get(path string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[path]
	return append([]byte(nil), b...), ok
}

func (m *memFS) put(path string, data []byte) {
	m.mu.Lock()
	m.files[path] = append([]byte(nil), data...)
	m.mu.Unlock()
}

func (m *memFS) names(prefix string) []string {
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

func (m *memFS) stall() {
	if m.syncStall == nil {
		return
	}
	if m.syncEntered != nil {
		select {
		case m.syncEntered <- struct{}{}:
		default: // never block a stalled call on an unread signal
		}
	}
	<-m.syncStall
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

func (m *memFS) MkdirAll(string, os.FileMode) error { return m.failMkdir }

func (m *memFS) Stat(path string) (os.FileInfo, error) {
	b, ok := m.get(path)
	if !ok {
		return nil, os.ErrNotExist
	}
	return memInfo{name: filepath.Base(path), size: int64(len(b))}, nil
}

func (m *memFS) ReadFile(path string) ([]byte, error) {
	b, ok := m.get(path)
	if !ok {
		return nil, os.ErrNotExist
	}
	return b, nil
}

func (m *memFS) WriteFileSync(path string, data []byte, perm os.FileMode) error {
	if m.failWrite != nil {
		return m.failWrite
	}
	m.stall()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.files[path]; exists {
		return os.ErrExist
	}
	m.files[path] = append([]byte(nil), data...)
	m.modes[path] = perm
	return nil
}

func (m *memFS) Rename(oldpath, newpath string) error {
	if m.failRename != nil {
		return m.failRename
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[oldpath]
	if !ok {
		return os.ErrNotExist
	}
	m.files[newpath] = b
	m.modes[newpath] = m.modes[oldpath]
	delete(m.files, oldpath)
	return nil
}

func (m *memFS) Remove(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[path]; !ok {
		return os.ErrNotExist
	}
	delete(m.files, path)
	return nil
}

func (m *memFS) Glob(pattern string) ([]string, error) {
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
	m    *memFS
	path string
}

func (m *memFS) OpenAppend(path string, perm os.FileMode) (appendFile, error) {
	m.mu.Lock()
	if _, ok := m.files[path]; !ok {
		m.files[path] = nil
		m.modes[path] = perm
	}
	m.mu.Unlock()
	return &memAppend{m: m, path: path}, nil
}

func (a *memAppend) Write(p []byte) (int, error) {
	if a.m.failWrite != nil {
		return 0, a.m.failWrite
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

var errDiskFull = errors.New("no space left on device")

func lines(b []byte) [][]byte {
	return bytes.Split(bytes.TrimSpace(b), []byte("\n"))
}
