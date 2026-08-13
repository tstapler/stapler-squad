//go:build !windows

package services

import "syscall"

// execSyscall replaces the current process with the given executable using syscall.Exec.
func execSyscall(executable string, args []string, env []string) error {
	return syscall.Exec(executable, args, env)
}
