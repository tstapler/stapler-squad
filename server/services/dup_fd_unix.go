//go:build !windows

package services

import (
	"fmt"
	"os"
	"syscall"
)

// dupPTYFile duplicates f into an independent *os.File. The dup gets its own
// poll.FD, so setting a read deadline on (or closing) the returned file has
// no effect on f or its other readers — the underlying open file description
// is only released once every fd referencing it is closed.
func dupPTYFile(f *os.File) (*os.File, error) {
	dupFd, err := syscall.Dup(int(f.Fd()))
	if err != nil {
		return nil, fmt.Errorf("failed to duplicate PTY fd: %w", err)
	}
	return os.NewFile(uintptr(dupFd), f.Name()), nil
}
