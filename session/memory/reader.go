// Package memory provides session memory measurement for the hibernation sweeper.
package memory

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// panePIDsTimeout bounds the list-panes subprocess used to measure a
// session's memory. Previously this used context.Background() with no bound
// at all -- a hung tmux call here would hold an exec-gate slot indefinitely.
const panePIDsTimeout = 5 * time.Second

// Reader reports system and per-session memory usage.
type Reader interface {
	// SystemMemoryPct returns the percentage of system RAM in use (0–100).
	// Returns 0 on platforms where measurement is unavailable.
	SystemMemoryPct() (float64, error)
	// SessionsRSSMB returns the total RSS (resident set size) in MB for each
	// of the given tmux session names, keyed by name. A session that is not
	// running or whose measurement fails maps to 0 rather than an error, so
	// one bad session doesn't fail the whole batch. Implementations should
	// measure every name from a single shared view of the system process
	// table rather than re-querying it per session -- see GopsutilReader.
	SessionsRSSMB(ctx context.Context, tmuxSessionNames []string) (map[string]int64, error)
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

// SessionsRSSMB returns the summed RSS in MB for each of the given tmux
// sessions. All sessions share one processSnapshot (one system-wide process
// enumeration) instead of each session -- or each node in each session's
// process tree -- re-enumerating the system's process table on its own.
func (g *GopsutilReader) SessionsRSSMB(ctx context.Context, tmuxSessionNames []string) (map[string]int64, error) {
	result := make(map[string]int64, len(tmuxSessionNames))
	if len(tmuxSessionNames) == 0 {
		return result, nil
	}

	snap, err := newProcessSnapshot(ctx)
	if err != nil {
		return result, err
	}

	for _, name := range tmuxSessionNames {
		pids, err := panePIDs(name)
		if err != nil || len(pids) == 0 {
			result[name] = 0
			continue
		}

		seen := make(map[int32]bool)
		var totalKB int64
		for _, pid := range pids {
			totalKB += snap.sumRSS(pid, seen, 0)
		}
		result[name] = totalKB / 1024
	}

	return result, nil
}

// panePIDs runs `tmux list-panes -t <name> -F '#{pane_pid}'` and returns the PIDs.
func panePIDs(sessionName string) ([]int32, error) {
	socket := tmux.ResolveSocket("")
	ctx, cancel := context.WithTimeout(context.Background(), panePIDsTimeout)
	defer cancel()

	release, err := tmux.AcquireExecSlot(ctx, socket.String())
	if err != nil {
		return nil, err
	}
	defer release()

	args := socket.Args("list-panes", "-t", sessionName, "-F", "#{pane_pid}")
	cmd := safeexec.CommandContext(ctx, tmux.Binary(), args...)
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

// processSnapshot is a single point-in-time view of the system's process
// table, built with exactly one process.ProcessesWithContext call.
//
// gopsutil's Process.Children() has no "list children of PID X" syscall on
// Darwin (or Linux): it lists every PID on the machine and checks each one's
// Ppid, per call (see gopsutil's process_darwin.go ChildrenWithContext). The
// previous sumRSS called .Children() at every node while walking a session's
// process tree, so summing one session's RSS did up to maxProcessTreeSize
// full-system-process enumerations. Building the parent->children map once
// here and sharing one snapshot across every session in a sweep (see
// GopsutilReader.SessionsRSSMB) reduces that to one enumeration for the
// entire batch.
type processSnapshot struct {
	children map[int32][]int32
	byPID    map[int32]*process.Process
}

func newProcessSnapshot(ctx context.Context) (*processSnapshot, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	snap := &processSnapshot{
		children: make(map[int32][]int32, len(procs)),
		byPID:    make(map[int32]*process.Process, len(procs)),
	}
	for _, p := range procs {
		snap.byPID[p.Pid] = p
		ppid, err := p.PpidWithContext(ctx)
		if err != nil {
			continue
		}
		snap.children[ppid] = append(snap.children[ppid], p.Pid)
	}
	return snap, nil
}

// sumRSS recursively sums RSS (in KB) for pid and its descendants, using the
// snapshot's precomputed parent->children map instead of a live Children()
// call at every node. depth and seen cap traversal exactly as the original
// implementation did.
func (s *processSnapshot) sumRSS(pid int32, seen map[int32]bool, depth int) int64 {
	if depth > maxProcessTreeDepth || len(seen) >= maxProcessTreeSize || seen[pid] {
		return 0
	}
	seen[pid] = true

	var rssKB int64
	if proc, ok := s.byPID[pid]; ok {
		if info, err := proc.MemoryInfo(); err == nil && info != nil {
			rssKB = int64(info.RSS / 1024)
		}
	}

	for _, childPID := range s.children[pid] {
		rssKB += s.sumRSS(childPID, seen, depth+1)
	}

	return rssKB
}
