//go:build windows

package executor

import "os/exec"

// applyProcessGroup is a no-op on Windows. Windows does not support POSIX
// process groups. On Windows, use Job Objects (via CreateJobObject) for
// process group isolation — but that is out of scope for this iteration.
func applyProcessGroup(_ *exec.Cmd) {}
