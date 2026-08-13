//go:build !windows

package executor

import "syscall"

// killProcessGroup sends sig to the process group of pid. It uses
// syscall.Getpgid to find the process group ID, then kills the entire group
// via kill(-pgid, sig). This ensures grandchildren (and all descendants) in
// the same group receive the signal, preventing orphaned processes.
//
// If Getpgid fails (e.g. process already exited), it falls back to sending
// the signal directly to pid.
func killProcessGroup(pid int, sig syscall.Signal) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		// Process may have already exited; attempt direct kill as fallback.
		return syscall.Kill(pid, sig)
	}
	// Negative PID means "send to process group pgid".
	return syscall.Kill(-pgid, sig)
}
