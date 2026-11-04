// Package norawexec is a deliberate norawexec violation; never import it.
package norawexec

import "os/exec"

// Violation calls exec.Command outside an approved wrapper.
func Violation() *exec.Cmd { return exec.Command("tmux", "list-sessions") }
