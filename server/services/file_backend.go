package services

import (
	"io"
	"os"
)

// FileHandle is a read-only handle on one resolved, non-directory file.
type FileHandle interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
	Stat() (os.FileInfo, error)
}

// FileRoot is one worktree directory behind a FileBackend. Apart from the
// resolver-only linkProber probes, every path argument is a ResolvedPath, so a
// backend never sees a path the resolver has not confined.
type FileRoot interface {
	linkProber
	Open(p ResolvedPath) (FileHandle, error)
	Lstat(p ResolvedPath) (os.FileInfo, error)
	Close() error
}

// FileBackend is the storage port under FileOps: no policy, only bytes and
// metadata. osRootBackend is production; memBackend is for tests.
type FileBackend interface {
	OpenRoot(dir string) (FileRoot, error)
}
