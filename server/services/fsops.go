package services

import (
	"os"
	"path/filepath"
)

// fsOps is the file-system seam shared by the delivery-gate stats store and the
// audit sink, so tests inject faults (full disk, failed rename, stalled fsync)
// without touching disk.
type fsOps interface {
	MkdirAll(path string, perm os.FileMode) error
	Stat(path string) (os.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	// WriteFileSync creates path exclusively, writes data, fsyncs and closes it.
	WriteFileSync(path string, data []byte, perm os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(path string) error
	Glob(pattern string) ([]string, error)
	OpenAppend(path string, perm os.FileMode) (appendFile, error)
}

// appendFile is an O_APPEND file handle.
type appendFile interface {
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

type osFS struct{}

func (osFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osFS) Stat(path string) (os.FileInfo, error)        { return os.Stat(path) }
func (osFS) ReadFile(path string) ([]byte, error)         { return os.ReadFile(path) }
func (osFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (osFS) Remove(path string) error                     { return os.Remove(path) }
func (osFS) Glob(pattern string) ([]string, error)        { return filepath.Glob(pattern) }

func (osFS) WriteFileSync(path string, data []byte, perm os.FileMode) error {
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

func (osFS) OpenAppend(path string, perm os.FileMode) (appendFile, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, perm)
}
