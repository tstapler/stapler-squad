//go:build linux

package tmux

import (
	"fmt"
	"syscall"
)

// SetSubreaper makes the current process the subreaper for its entire descendant
// tree on Linux. When any descendant process exits and its direct parent has not
// yet called wait(), the kernel reparents the zombie to the nearest subreaper
// ancestor rather than to init (PID 1). Our existing Wait4(-1, WNOHANG) reaper
// then collects those zombies too, including tmux's direct children.
//
// This is a no-op on non-Linux platforms; call it unconditionally at startup.
func SetSubreaper() error {
	// PR_SET_CHILD_SUBREAPER = 36
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, 36, 1, 0); errno != 0 {
		return fmt.Errorf("prctl(PR_SET_CHILD_SUBREAPER): %w", errno)
	}
	return nil
}
