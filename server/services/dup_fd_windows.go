//go:build windows

package services

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// dupPTYFile duplicates f into an independent *os.File via DuplicateHandle,
// mirroring dup_fd_unix.go's syscall.Dup behavior. Tmux-backed sessions are
// not supported on Windows (see session/tmux/tmux_windows.go), so this path
// isn't expected to be exercised, but the package still needs to build for
// windows/amd64 (.github/workflows/build.yml's release platform matrix).
func dupPTYFile(f *os.File) (*os.File, error) {
	proc := windows.CurrentProcess()
	var dup windows.Handle
	if err := windows.DuplicateHandle(proc, windows.Handle(f.Fd()), proc, &dup, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, fmt.Errorf("failed to duplicate PTY handle: %w", err)
	}
	return os.NewFile(uintptr(dup), f.Name()), nil
}
