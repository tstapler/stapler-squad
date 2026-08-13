//go:build !linux

package executor

import "os/exec"

// applyRlimits is a no-op stub on non-Linux platforms. RlimitConfig values
// are accepted by the API but have no effect. macOS supports a subset of
// rlimits via setrlimit(2), but the save/restore approach used on Linux
// has higher risk of interference on macOS due to its different scheduler
// semantics. A future iteration may add macOS rlimit support via a
// platform-specific implementation.
func applyRlimits(_ *exec.Cmd, _ RlimitConfig) error {
	return nil
}
