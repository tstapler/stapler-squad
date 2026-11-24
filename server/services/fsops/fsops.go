package fsops

import (
	"os"
	"path/filepath"
)

// FSOps is the file-system seam shared by the delivery-gate stats store and the
// audit sink, so tests inject faults (full disk, failed rename, stalled fsync)
// without touching disk.
type FSOps interface {
	MkdirAll(path string, perm os.FileMode) error
	Stat(path string) (os.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	// WriteFileSync creates path exclusively, writes data, fsyncs and closes it.
	WriteFileSync(path string, data []byte, perm os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(path string) error
	Glob(pattern string) ([]string, error)
	OpenAppend(path string, perm os.FileMode) (AppendFile, error)
}

// AppendFile is an O_APPEND file handle.
type AppendFile interface {
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

type OSFS struct{}

func (OSFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (OSFS) Stat(path string) (os.FileInfo, error)        { return os.Stat(path) }
func (OSFS) ReadFile(path string) ([]byte, error)         { return os.ReadFile(path) }
func (OSFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (OSFS) Remove(path string) error                     { return os.Remove(path) }
func (OSFS) Glob(pattern string) ([]string, error)        { return filepath.Glob(pattern) }

func (OSFS) WriteFileSync(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (OSFS) OpenAppend(path string, perm os.FileMode) (AppendFile, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, perm)
}
