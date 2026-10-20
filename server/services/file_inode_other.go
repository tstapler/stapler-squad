//go:build windows

package services

import "os"

// fileInode is unavailable on Windows; the size and mtime parts of the key remain.
func fileInode(os.FileInfo) uint64 { return 0 }
