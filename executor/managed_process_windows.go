//go:build windows

package executor

import "syscall"

// killProcessGroup on Windows sends a termination signal to the process
// identified by pid. Windows does not have POSIX process groups or kill(-pgid),
// so only the direct process is killed. Grandchildren are not reached by this
// call; use Job Objects for group termination on Windows.
func killProcessGroup(pid int, _ syscall.Signal) error {
	// On Windows we don't have access to the *exec.Cmd here, so we use
	// OpenProcess + TerminateProcess via the unsafe syscall path.
	// For now, this is a best-effort implementation.
	handle, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer func() { _ = syscall.CloseHandle(handle) }()
	return syscall.TerminateProcess(handle, 1)
}

// buildSysProcAttr on Windows returns nil (no Unix process group attributes).
// Windows process isolation uses Job Objects, which are out of scope here.
func buildSysProcAttr(_ processConfig) *syscall.SysProcAttr {
	return nil
}
