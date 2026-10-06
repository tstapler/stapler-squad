//go:build !windows

package streamhub

import (
	"io/fs"
	"syscall"
)

// ownedByUID reports whether fi is owned by uid. It fails closed when the
// platform stat has no owner.
func ownedByUID(fi fs.FileInfo, uid int) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == uid
}
