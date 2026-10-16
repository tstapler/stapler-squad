//go:build unix

package gogit

import (
	"os"
	"syscall"
)

// fileIDs returns the device and owner of fi. ok is false when the platform does not say.
func fileIDs(fi os.FileInfo) (dev uint64, uid uint32, ok bool) {
	st, isStat := fi.Sys().(*syscall.Stat_t)
	if !isStat {
		return 0, 0, false
	}
	return uint64(st.Dev), st.Uid, true //nolint:gosec,unconvert Dev is int32 on darwin
}

func effectiveUID() int { return os.Geteuid() }
