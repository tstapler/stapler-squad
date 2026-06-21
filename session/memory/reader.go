// Package memory provides session memory measurement for the hibernation sweeper.
package memory

import (
	"context"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// Reader reports system and per-session memory usage.
type Reader interface {
	// SystemMemoryPct returns the percentage of system RAM in use (0–100).
	// Returns 0 on platforms where measurement is unavailable.
	SystemMemoryPct() (float64, error)
	// SessionRSSMB returns the total RSS (resident set size) in MB for all
	// processes belonging to the given tmux session name.
	// Returns 0 if the session is not running or measurement fails.
	SessionRSSMB(tmuxSessionName string) (int64, error)
}

// GopsutilReader implements Reader using gopsutil and tmux.
type GopsutilReader struct{}

// NewGopsutilReader creates a new GopsutilReader.
func NewGopsutilReader() *GopsutilReader {
	return &GopsutilReader{}
}

// SystemMemoryPct returns used memory percentage via gopsutil.
func (g *GopsutilReader) SystemMemoryPct() (float64, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}
	return vm.UsedPercent, nil
}

// SessionRSSMB returns the summed RSS in MB for all processes in a tmux session.
func (g *GopsutilReader) SessionRSSMB(tmuxSessionName string) (int64, error) {
	pids, err := panePIDs(tmuxSessionName)
	if err != nil || len(pids) == 0 {
		return 0, nil
	}

	seen := make(map[int32]bool)
	var totalKB int64

	for _, pid := range pids {
		totalKB += sumRSS(pid, seen, 0)
	}

	return totalKB / 1024, nil
}

// panePIDs runs `tmux list-panes -t <name> -F '#{pane_pid}'` and returns the PIDs.
func panePIDs(sessionName string) ([]int32, error) {
	ctx := context.Background()
	cmd := safeexec.CommandContext(ctx, "tmux", "list-panes", "-t", sessionName, "-F", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var pids []int32
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, err := strconv.ParseInt(line, 10, 32)
		if err != nil {
			continue
		}
		pids = append(pids, int32(n))
	}
	return pids, nil
}

const (
	// maxProcessTreeDepth limits recursive process tree traversal.
	maxProcessTreeDepth = 8
	// maxProcessTreeSize limits total processes visited per session.
	maxProcessTreeSize = 50
)

// sumRSS recursively sums RSS (in KB) for a process and its children.
// depth caps at maxProcessTreeDepth to avoid runaway traversal.
func sumRSS(pid int32, seen map[int32]bool, depth int) int64 {
	if depth > maxProcessTreeDepth || len(seen) >= maxProcessTreeSize || seen[pid] {
		return 0
	}
	seen[pid] = true

	proc, err := process.NewProcess(pid)
	if err != nil {
		return 0
	}

	info, err := proc.MemoryInfo()
	var rssKB int64
	if err == nil && info != nil {
		rssKB = int64(info.RSS / 1024)
	}

	children, err := proc.Children()
	if err == nil {
		for _, child := range children {
			rssKB += sumRSS(child.Pid, seen, depth+1)
		}
	}

	return rssKB
}
