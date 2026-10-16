//go:build unix

package unfinished

import "golang.org/x/sys/unix"

// statPathSig stats path (following symlinks, like os.Stat). ok is false when
// the path cannot be stat'ed.
func statPathSig(path string) (sig statSig, ok bool) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return statSig{}, false
	}
	return statSig{
		mtimeNs: st.Mtim.Nano(),
		ctimeNs: st.Ctim.Nano(),
		size:    st.Size,
		ino:     uint64(st.Ino), //nolint:gosec // inode numbers are non-negative
	}, true
}

// lstatPathSig is statPathSig without following a final symlink.
func lstatPathSig(path string) (sig statSig, ok bool) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return statSig{}, false
	}
	return statSig{
		mtimeNs: st.Mtim.Nano(),
		ctimeNs: st.Ctim.Nano(),
		size:    st.Size,
		ino:     uint64(st.Ino), //nolint:gosec // inode numbers are non-negative
	}, true
}
