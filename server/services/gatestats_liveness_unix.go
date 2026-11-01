//go:build !windows

package services

import (
	"errors"
	"syscall"
)

// pidExists sends signal 0; EPERM means the process exists but belongs to another user.
func pidExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
