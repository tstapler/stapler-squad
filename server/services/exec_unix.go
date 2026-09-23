//go:build !windows

package services

import "syscall"

// execSyscall replaces the current process with the given executable using syscall.Exec.
func execSyscall(executable string, args []string, env []string) error {
	// #nosec G204 -- execRestart, the only caller, passes os.Executable()/os.Args/os.Environ() (this process's own image), not external input.
	return syscall.Exec(executable, args, env)
}
