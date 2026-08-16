//go:build !windows

package safeexec

import "github.com/shirou/gopsutil/v4/process"

// procStateSnapshot returns a short, human-readable process-state word (e.g.
// "running", "sleep", "idle", "zombie" — gopsutil normalizes the raw OS
// status letters, it does not return them verbatim) for pid via gopsutil's
// cross-platform inspection (procfs on Linux, libproc on Darwin) — never a
// "ps" subshell, which would itself risk the same D-state/high-load hang
// this snapshot exists to help diagnose. Any
// lookup failure (process already gone, permission error, platform quirk)
// degrades to "unknown"; this is a best-effort forensic aid attached to a
// SIGKILL-escalation log line, never something the caller should treat as
// fatal, block on, or retry.
//
// Known limitation: pid could theoretically be recycled between the caller
// deciding to escalate and this snapshot running (TOCTOU) on a sufficiently
// busy host, within the grace window. Accepted as best-effort, not a
// guarantee — callers take the snapshot as close to the kill as possible.
func procStateSnapshot(pid int) string {
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
