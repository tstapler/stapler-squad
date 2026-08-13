//go:build !windows

package session

import "syscall"

// SuspendOriginalProcess sends SIGSTOP to pid, freezing it so it cannot write
// to the shared Claude JSONL transcript while the resumed, managed session
// starts writing to the same file (see Story 1.2.1, Task 1.2.1e). The caller
// is responsible for persisting a SuspendedProcessRecord before calling this
// so the suspension survives a server restart (see
// ReconcileSuspendedProcesses).
func SuspendOriginalProcess(pid int32) error {
	return syscall.Kill(int(pid), syscall.SIGSTOP)
}

// ResumeOriginalProcess sends SIGCONT to pid, unfreezing a process previously
// suspended by SuspendOriginalProcess. Used on commit failure (resume
// immediately), on CancelPendingKill (resume after compensating delete), and
// by ReconcileSuspendedProcesses on startup.
func ResumeOriginalProcess(pid int32) error {
	return syscall.Kill(int(pid), syscall.SIGCONT)
}
