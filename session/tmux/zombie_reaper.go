//go:build !windows

package tmux

import (
	"context"
	"sync"
	"syscall"
	"time"
)

// StartZombieReaper starts a background goroutine that periodically reaps zombie
// child processes by draining Wait4(-1, WNOHANG).
//
// This complements StartZombieWatcher: the watcher detects and alerts; the reaper
// actually cleans up. A zombie (Z state) by definition has no outstanding Wait4
// caller—if cmd.Wait() had been called, the zombie would already be gone—so the
// WNOHANG wildcard wait is safe to issue without racing active Cmd goroutines.
//
// Recommended interval: 60s (half the watcher period is plenty; slower means
// fewer interference opportunities with in-flight cmd.Wait calls).
//
// wg.Add(1) is called before the goroutine starts, and the goroutine calls wg.Done() on exit
// so callers can join it (e.g. server.Server.Shutdown()) via a bounded wg.Wait(). Closes the
// signal-vs-join gap noted alongside StartForkPressureLogger (fork_metrics.go) and
// PTYDiscovery.Stop() (session/pty_discovery.go); tracked as backlog item
// 81e82fee-9528-4dc9-a513-1040b4dee2ec.
func StartZombieReaper(ctx context.Context, interval time.Duration, logFn func(string, ...any), wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n := reapZombieChildren(); n > 0 {
					logFn("[zombie-reaper] reaped %d zombie child(ren) via waitpid(-1, WNOHANG)", n)
				}
			}
		}
	}()
}

// reapZombieChildren drains all zombie direct children of the current process.
// It loops on Wait4(-1, WNOHANG) until there are no more zombies to collect.
// Returns the number reaped this pass.
func reapZombieChildren() int {
	count := 0
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if pid <= 0 || err != nil {
			break
		}
		count++
	}
	return count
}
