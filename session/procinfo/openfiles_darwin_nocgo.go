//go:build darwin && !cgo

package procinfo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// openFilesCgo is the CGO-disabled fallback used when CGO is unavailable
// (e.g. CGO_ENABLED=0 release builds). gopsutil's darwin/bsd OpenFiles is
// unconditionally common.ErrNotImplementedError (process_bsd.go), so it can't
// be used here; this shells out to lsof directly instead of the native
// proc_pidinfo syscalls used in openfiles_darwin.go, preserving functionality
// (at the cost of a subprocess) rather than silently degrading to empty.
func openFilesCgo(pid int32) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "lsof", "-p", strconv.Itoa(int(pid)), "-Fn")
	out, err := cmd.Output()
	if err != nil {
		// lsof exits non-zero for permission-denied or no-matching-fds cases
		// (e.g. inspecting another user's process) — treat that as "no files"
		// like gopsutil would, rather than propagating an error. A context
		// deadline (the process genuinely didn't respond in time, observed
		// as a flake under heavy parallel test load) is different: silently
		// returning empty there masks a real failure as "no open files", so
		// propagate it instead.
		if ctx.Err() != nil {
			return nil, fmt.Errorf("lsof timed out for pid %d: %w", pid, ctx.Err())
		}
		return []string{}, nil
	}

	var paths []string
	for _, line := range bytes.Split(out, []byte("\n")) {
		if len(line) > 1 && line[0] == 'n' {
			paths = append(paths, string(line[1:]))
		}
	}
	return paths, nil
}
