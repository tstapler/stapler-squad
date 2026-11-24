//go:build windows

package fsops

import "os"

// FileInode is unavailable on Windows; the size and mtime parts of the key remain.
func FileInode(os.FileInfo) uint64 { return 0 }
