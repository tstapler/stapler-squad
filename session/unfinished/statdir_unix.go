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
	root   string
	curDir string
	fd     int // -1 when no directory is open
}

func newDirStatter(root string) *dirStatter { return &dirStatter{root: root, fd: -1} }

func (d *dirStatter) close() {
	if d.fd >= 0 {
		_ = unix.Close(d.fd)
		d.fd = -1
	}
}

// lstat returns the size and mtime of root/rel without following a final symlink,
// with the same errors os.Lstat(filepath.Join(root, rel)) would return for a
// missing file (errors.Is(err, fs.ErrNotExist)).
func (d *dirStatter) lstat(rel string) (size int64, mtime time.Time, err error) {
	dir, name := filepath.Split(rel)
	if d.fd < 0 || dir != d.curDir {
		d.close()
		fd, openErr := unix.Open(filepath.Join(d.root, dir), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if openErr != nil {
			return 0, time.Time{}, openErr
		}
		d.fd, d.curDir = fd, dir
	}
	var st unix.Stat_t
	if err := unix.Fstatat(d.fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return 0, time.Time{}, err
	}
	return st.Size, time.Unix(int64(st.Mtim.Sec), int64(st.Mtim.Nsec)), nil
}
