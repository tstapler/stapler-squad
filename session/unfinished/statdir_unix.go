//go:build unix

package unfinished

import (
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// dirStatter lstats many files under one root, resolving each relative to an open
// directory fd instead of re-walking the root's absolute path per file. rel paths
// from a git index are sorted, so consecutive files usually share a directory and
// the fd is reused. Not safe for concurrent use.
type dirStatter struct {
	root    string
	curDir  string
	fd      int    // -1 when no directory is open
	failDir string // last directory that could not be opened; its files use the full-path fallback
	failed  bool
}

func newDirStatter(root string) *dirStatter { return &dirStatter{root: root, fd: -1} }

func (d *dirStatter) close() {
	if d.fd >= 0 {
		_ = unix.Close(d.fd)
		d.fd = -1
	}
}

// lstat returns the size and mtime of root/rel without following a final symlink,
// with the same results and errors as os.Lstat(filepath.Join(root, rel)).
// Opening a directory needs read permission, which a stat of a file inside it does
// not (a search-only 0111 directory): when the open fails, fall back to a full-path
// lstat so the file is still compared and the error matches the old behaviour.
func (d *dirStatter) lstat(rel string) (size int64, mtime time.Time, err error) {
	dir, name := filepath.Split(rel)
	var st unix.Stat_t
	if d.failed && dir == d.failDir {
		return d.lstatFullPath(rel, &st)
	}
	if d.fd < 0 || dir != d.curDir {
		d.close()
		fd, openErr := unix.Open(filepath.Join(d.root, dir), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if openErr != nil {
			d.failDir, d.failed = dir, true
			return d.lstatFullPath(rel, &st)
		}
		d.fd, d.curDir, d.failed = fd, dir, false
	}
	if err := unix.Fstatat(d.fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return 0, time.Time{}, err
	}
	return statSizeMtime(&st), statMtime(&st), nil
}

func (d *dirStatter) lstatFullPath(rel string, st *unix.Stat_t) (int64, time.Time, error) {
	if err := unix.Lstat(filepath.Join(d.root, rel), st); err != nil {
		return 0, time.Time{}, err
	}
	return statSizeMtime(st), statMtime(st), nil
}

func statSizeMtime(st *unix.Stat_t) int64 { return st.Size }

func statMtime(st *unix.Stat_t) time.Time { return time.Unix(int64(st.Mtim.Sec), int64(st.Mtim.Nsec)) }
