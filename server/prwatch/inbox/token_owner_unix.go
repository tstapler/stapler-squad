//go:build !windows

package inbox

import (
	"os"
	"syscall"
)

// ownedByCurrentUser reports whether info's owner is the running user. A token
// planted by another account must not be trusted even at mode 0600.
func ownedByCurrentUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
