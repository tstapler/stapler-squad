//go:build windows

package inbox

import "os"

// ownedByCurrentUser is a no-op on Windows: file ownership is ACL-based and
// the 0600 mode check is not meaningful there either.
func ownedByCurrentUser(os.FileInfo) bool { return true }
