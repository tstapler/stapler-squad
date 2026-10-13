//go:build linux || darwin || freebsd || netbsd || openbsd

package capability

import (
	"os"
	"syscall"
)

const xOK = 1 // access(2) X_OK

// executable reports whether the current user may execute path, as git's find_hook does with
// access(X_OK).
func executable(path string, _ os.FileInfo) bool { return syscall.Access(path, xOK) == nil }
