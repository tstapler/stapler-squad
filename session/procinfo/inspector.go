//go:build darwin

package procinfo

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/process"
)

// ProcessInspector wraps gopsutil process inspection for macOS.
type ProcessInspector struct{}

// NewProcessInspector creates a new ProcessInspector.
func NewProcessInspector() *ProcessInspector {
	return &ProcessInspector{}
}

// OpenFiles returns the list of open file paths for the given PID.
// Uses the native macOS proc_pidinfo syscall — no external tools required.
// Returns nil error with empty slice if the process is dead or access is denied.
func (p *ProcessInspector) OpenFiles(pid int32) ([]string, error) {
	// Verify the process exists first using gopsutil.
	_, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("process %d not found: %w", pid, err)
	}

	// Use native macOS syscall to enumerate open file descriptors.
	// Degrade gracefully on permission denied or race with process exit.
	paths, err := openFilesCgo(pid)
	if err != nil {
		return []string{}, nil
	}
	return paths, nil
}

// Cwd returns the current working directory for the given PID.
func (p *ProcessInspector) Cwd(pid int32) (string, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return "", fmt.Errorf("process %d not found: %w", pid, err)
	}
	cwd, err := proc.Cwd()
	if err != nil {
		return "", fmt.Errorf("cannot get cwd for pid %d: %w", pid, err)
	}
	return cwd, nil
}

// CreateTime returns the creation time of the process in epoch milliseconds.
func (p *ProcessInspector) CreateTime(pid int32) (int64, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return 0, fmt.Errorf("process %d not found: %w", pid, err)
	}
	ct, err := proc.CreateTime()
	if err != nil {
		return 0, fmt.Errorf("cannot get create time for pid %d: %w", pid, err)
	}
	return ct, nil
}

// IsAlive returns true if the process with the given PID is alive AND its
// creation time matches expectedCreateTimeMs. This guards against PID reuse.
func (p *ProcessInspector) IsAlive(pid int32, expectedCreateTimeMs int64) bool {
	actual, err := p.CreateTime(pid)
	if err != nil {
		return false
	}
	return actual == expectedCreateTimeMs
}
