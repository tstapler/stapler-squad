package services

import (
	"errors"
	"syscall"
	"time"
)

// PidLiveness reports whether pid is still the process that wrote the stats
// file at writtenAt.
type PidLiveness func(pid int, writtenAt time.Time) bool

// ProcessWriterAlive is signal 0 plus, where the OS exposes a process start time (Linux, macOS), a pid-reuse guard: a process
// that started after the file was written cannot be its writer.
func ProcessWriterAlive(pid int, writtenAt time.Time) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if start, ok := processStart(pid); ok && start.After(writtenAt.Add(time.Second)) {
		return false
	}
	return true
}
