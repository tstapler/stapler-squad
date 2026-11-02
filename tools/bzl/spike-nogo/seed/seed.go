// Package seed holds a deliberate norawgitcli violation (spike S12); never import it.
package seed

import "os/exec"

// Violation is the seeded git CLI call the nogo analyzer must flag.
func Violation() *exec.Cmd { return exec.Command("git", "status") }
