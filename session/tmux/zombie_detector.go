package tmux

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// ZombieInfo describes a detected zombie process.
type ZombieInfo struct {
	PID     int
	PPID    int
	Command string
}

// ScanZombies returns zombie processes (state "Z") that are direct children of
// the current process. Only direct children can be reaped via Wait4(-1, WNOHANG),
// so reporting system-wide zombies would produce un-reapable noise.
func ScanZombies() ([]ZombieInfo, error) {
	ourPID := os.Getpid()
	// -axo: all processes, custom columns; state Z = zombie
	psCtx, psCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer psCancel()
	psCmd := safeexec.CommandContext(psCtx, "ps", "-axo", "pid,ppid,stat,comm")
	out, err := psCmd.Output()
	if err != nil {
		return nil, err
	}

	var zombies []ZombieInfo
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		stat := fields[2]
		// Zombie state on macOS and Linux is "Z" or starts with "Z"
		if !strings.HasPrefix(stat, "Z") {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		// Only track zombies we can actually reap (direct children only).
		if ppid != ourPID {
			continue
		}
		zombies = append(zombies, ZombieInfo{
			PID:     pid,
			PPID:    ppid,
			Command: fields[3],
		})
	}
	return zombies, scanner.Err()
}

// StartZombieWatcher starts a background goroutine that periodically scans for zombie
// processes and records them via RecordZombieProcess when found. ctx controls its lifetime.
// interval is how often to scan (recommended: 30s).
//
// The first scan establishes a baseline: zombies already present at startup are logged
// via warnFn but not counted toward the alert threshold. Only zombies that
// appear after the baseline (i.e. growth over time) are recorded and counted toward the
// alert threshold. This prevents a burst of spurious critical alerts on service restart
// when a stable set of zombie children already exists.
func StartZombieWatcher(ctx context.Context, interval time.Duration, warnFn func(string, ...any)) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Track which PIDs we've already reported to avoid repeated alerts for the
		// same long-lived zombie (uncommon but possible on a slow reaper).
		reported := make(map[int]bool)
		firstScan := true

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				zombies, err := ScanZombies()
				if err != nil {
					// ps failure is non-fatal
					continue
				}

				// Build current set so we can evict stale entries
				current := make(map[int]bool, len(zombies))

				if len(zombies) > 0 {
					// Immediately reap rather than waiting for the 60s background tick.
					// Doing this before recording ensure we've at least tried to clean
					// up before the fork pressure monitor evaluates the alert state.
					if n := reapZombieChildren(); n > 0 {
						warnFn("[zombie-reaper] reaped %d zombie child(ren) on detection", n)
					}
				}

				for _, z := range zombies {
					current[z.PID] = true
					if !reported[z.PID] {
						reported[z.PID] = true
						if firstScan {
							// Baseline: log but don't record against the pressure threshold.
							warnFn("[zombie-watcher] baseline zombie at startup: pid=%d comm=%q (not counted toward alert threshold)", z.PID, z.Command)
						} else {
							RecordZombieProcess(z.PID, z.Command, warnFn)
						}
					}
				}

				firstScan = false

				// Evict reaped zombies from the reported set
				for pid := range reported {
					if !current[pid] {
						delete(reported, pid)
					}
				}
			}
		}
	}()
}
