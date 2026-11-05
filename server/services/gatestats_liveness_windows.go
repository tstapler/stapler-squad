//go:build windows

package services

import "os"

// pidExists relies on os.FindProcess opening a process handle on Windows, which fails for a pid that is gone.
func pidExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
