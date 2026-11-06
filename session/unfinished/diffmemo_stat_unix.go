//go:build unix

package unfinished

import "golang.org/x/sys/unix"

// statFileStamp lstats a regular file; symlinks and other file types are not
// memoized because their content is not what the stat fields describe.
func statFileStamp(path string) (fileStamp, bool) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
		return fileStamp{}, false
	}
	return fileStamp{
		Size:    st.Size,
		MtimeNs: int64(st.Mtim.Sec)*1e9 + int64(st.Mtim.Nsec),
		CtimeNs: int64(st.Ctim.Sec)*1e9 + int64(st.Ctim.Nsec),
		Inode:   uint64(st.Ino),
	}, true
}
