//go:build !linux

package safeexec

import "os/exec"

// EnsurePdeathsig is a no-op on non-Linux platforms: Pdeathsig has no
// equivalent in syscall.SysProcAttr on darwin/windows.
//
// ponytail: no macOS/Windows parent-death reaping implemented here — orphaned
// children on those platforms still rely on a periodic sweep. Add a
// platform-specific mechanism (e.g. a watchdog process) if non-Linux orphan
// accumulation becomes a real problem; see safeexec_pdeathsig_linux.go for
// the Linux implementation this mirrors.
func EnsurePdeathsig(cmd *exec.Cmd) {}
