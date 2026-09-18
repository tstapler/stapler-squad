//go:build windows

package session

import "fmt"

// SuspendOriginalProcess is not supported on Windows -- SIGSTOP/SIGCONT have
// no Windows equivalent exposed via syscall.Kill.
func SuspendOriginalProcess(_ int32) error {
	return fmt.Errorf("SuspendOriginalProcess: not supported on Windows")
}

// ResumeOriginalProcess is not supported on Windows.
func ResumeOriginalProcess(_ int32) error {
	return fmt.Errorf("ResumeOriginalProcess: not supported on Windows")
}
