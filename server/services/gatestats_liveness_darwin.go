//go:build darwin

package services

import (
	"time"

	"golang.org/x/sys/unix"
)

// processStart reads the kernel's recorded start time (kern.proc.pid); macOS has no /proc, so
// without this the pid-reuse guard in ProcessWriterAlive would silently never fire.
func processStart(pid int) (time.Time, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(kp.Proc.P_starttime.Sec, int64(kp.Proc.P_starttime.Usec)*1000), true
}
