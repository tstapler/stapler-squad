// Package norawgitcli is a deliberate norawgitcli violation; never import it.
package norawgitcli

import "os/exec"

// Violation invokes the git CLI directly.
func Violation() *exec.Cmd { return exec.Command("git", "status") }
