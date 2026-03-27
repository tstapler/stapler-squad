//go:build windows

package services

import (
	"fmt"
	"os/exec"
)

// execSyscall on Windows starts a new process (true exec-replace is not available).
func execSyscall(executable string, args []string, env []string) error {
	cmd := exec.Command(executable, args[1:]...)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %w", err)
	}
	return fmt.Errorf("process replacement not supported on Windows; new process started with PID %d", cmd.Process.Pid)
}
