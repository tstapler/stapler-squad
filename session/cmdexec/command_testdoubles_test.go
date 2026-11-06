package cmdexec

import "os"

// pipePTY creates a pipe that simulates a PTY for the command executor tests.
func pipePTY() (*os.File, *os.File, error) { return os.Pipe() }
