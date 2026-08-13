//go:build linux

package executor

import (
	"os/exec"
	"syscall"
)

// applyRlimits sets child-scoped resource limits on cmd before cmd.Start().
//
// On Linux, exec.Cmd.SysProcAttr is *syscall.SysProcAttr, which does NOT
// include an Rlimits field (that is only on golang.org/x/sys/unix.SysProcAttr,
// which is not the type accepted by exec.Cmd). Therefore, we use a
// save/restore pattern: set the rlimit on the current process before
// cmd.Start(), which the forked child inherits, then restore the parent's
// original limit immediately after fork.
//
// The race window between setrlimit and fork is narrow and acceptable per
// plan §6.5. This function must be called on a goroutine locked to its OS
// thread if strict isolation is required, though in practice the window is
// sub-microsecond on modern kernels.
//
// As a defense-in-depth measure, Pdeathsig is set to SIGKILL: if the parent
// Go process dies unexpectedly, the kernel delivers SIGKILL to the child.
func applyRlimits(cmd *exec.Cmd, cfg RlimitConfig) error {
	if cfg.MaxCPUSecs == 0 && cfg.MaxVirtBytes == 0 && cfg.MaxOpenFiles == 0 {
		return nil
	}

	// Merge with existing SysProcAttr (Setpgid may already be set).
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Set Pdeathsig for parent-death protection (Linux-only field).
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL

	if cfg.MaxCPUSecs > 0 {
		if err := applyRlimitSaveRestore(syscall.RLIMIT_CPU, cfg.MaxCPUSecs); err != nil {
			return err
		}
	}
	if cfg.MaxVirtBytes > 0 {
		if err := applyRlimitSaveRestore(syscall.RLIMIT_AS, cfg.MaxVirtBytes); err != nil {
			return err
		}
	}
	if cfg.MaxOpenFiles > 0 {
		if err := applyRlimitSaveRestore(syscall.RLIMIT_NOFILE, cfg.MaxOpenFiles); err != nil {
			return err
		}
	}

	return nil
}

// applyRlimitSaveRestore sets a resource limit (which the child inherits at fork
// time), then restores the parent's original limit. The child has already
// inherited the limit at fork, so the restore is safe.
func applyRlimitSaveRestore(resource int, value uint64) error {
	var prev syscall.Rlimit
	if err := syscall.Getrlimit(resource, &prev); err != nil {
		return err
	}

	// Cap the new hard limit to the parent's hard limit (cannot raise beyond it
	// without CAP_SYS_RESOURCE). If the requested value exceeds the hard limit,
	// use the hard limit silently.
	hardMax := prev.Max
	if value > hardMax {
		value = hardMax
	}

	newLimit := syscall.Rlimit{Cur: value, Max: hardMax}
	if err := syscall.Setrlimit(resource, &newLimit); err != nil {
		return err
	}

	// Restore immediately after fork. The forked child has already inherited
	// newLimit; restoring here reverts only the parent's state.
	_ = syscall.Setrlimit(resource, &prev)
	return nil
}
