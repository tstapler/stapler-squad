//go:build windows

package streamhub

import "io/fs"

// ownedByUID is a no-op on Windows, where Unix ownership does not apply.
func ownedByUID(fs.FileInfo, int) bool { return true }
