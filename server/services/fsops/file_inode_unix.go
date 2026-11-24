//go:build !windows

package fsops

import (
	"os"
	"syscall"
)

// FileInode returns the inode of info, or 0 when the backend has none.
func FileInode(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Ino) //nolint:unconvert // Ino is uint32 on some platforms
	}
	return 0
}
