// Package clean is the control for spike S12: no git CLI call.
package clean

import "os/exec"

// Echo runs a non-git command.
func Echo() *exec.Cmd { return exec.Command("echo", "ok") }
