package services

import "time"

// PidLiveness reports whether pid is still the process that wrote the stats
// file at writtenAt.
type PidLiveness func(pid int, writtenAt time.Time) bool

// ProcessWriterAlive is a pid-exists probe (signal 0 on unix) plus, where the OS exposes a process start time (Linux, macOS), a pid-reuse guard: a process
// that started after the file was written cannot be its writer.
func ProcessWriterAlive(pid int, writtenAt time.Time) bool {
	if pid <= 0 {
		return false
	}
	if !pidExists(pid) {
		return false
	}
	if start, ok := processStart(pid); ok && start.After(writtenAt.Add(time.Second)) {
		return false
	}
	return true
}
