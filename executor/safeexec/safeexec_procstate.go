//go:build !windows

package safeexec

import "github.com/shirou/gopsutil/v4/process"

// procStateSnapshot returns a short process-state word for pid via gopsutil
// (never a "ps" subshell, which risks the same D-state hang this exists to
// diagnose). Best-effort forensic aid only — degrades to "unknown" on any
// failure, and is subject to pid-reuse TOCTOU on a busy host; never treat
// as authoritative. See PR #514 for the full investigation.
func procStateSnapshot(pid int) string {
	// #nosec G115 -- OS PIDs fit comfortably in int32 (Linux pid_max < 2^22, macOS/BSD < 2^20)
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return "unknown"
	}
	statuses, err := proc.Status()
	if err != nil || len(statuses) == 0 {
		return "unknown"
	}
	return statuses[0]
}
