//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package capability

import "os"

// executable approximates access(X_OK) from the mode bits where the syscall is unavailable.
func executable(_ string, fi os.FileInfo) bool { return fi.Mode().Perm()&0o111 != 0 }
