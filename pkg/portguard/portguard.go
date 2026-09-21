// Package portguard ensures a set of TCP ports are actually free — and, if a
// matching stale process is still holding one, terminates it — before a
// caller starts a new process that needs to bind them.
//
// This replaces two independent shell implementations of the same
// precondition (scripts/lib/wait_for_port_release.sh, reimplemented ad hoc by
// scripts/dev-restart-guard.sh before it was consolidated back onto the
// shared helper) that each accumulated their own incident history: a 10s
// "wait, then proceed anyway" timeout that left a genuinely stuck process
// racing the next instance (2026-09-08), a pkill pattern that once matched
// the live launchd-managed service as collateral damage (2026-09-04), and a
// launchctl bootstrap/load domain mismatch that silently broke the rollback
// path (2026-08-18). None of those were caught by a test, because shell has
// no practical way to exercise "spawn a real process holding a real port,
// then assert the reaper actually frees it" — see portguard_test.go, which
// does exactly that against this package.
package portguard

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Options configures a single EnsureReleased call.
type Options struct {
	// Ports are the TCP ports that must be unbound before EnsureReleased
	// returns successfully.
	Ports []int

	// ProcessNameContains matches candidate processes by substring against
	// each process's executable name (not full cmdline — a full-cmdline
	// substring match is what let a prior shell pkill pattern accidentally
	// match unrelated processes). Only processes whose listening port
	// appears in Ports are ever considered, so this is a second, narrower
	// gate on top of that, not the primary filter.
	ProcessNameContains string

	// ExcludePID is never signaled, even if it happens to hold one of the
	// target ports (e.g. the caller's own process during a self-test).
	ExcludePID int32

	// Timeout bounds the initial graceful-exit wait after a SIGTERM. Zero
	// uses DefaultTimeout.
	Timeout time.Duration

	// PollInterval bounds how often port state is rechecked. Zero uses
	// DefaultPollInterval.
	PollInterval time.Duration
}

const (
	// DefaultTimeout is how long EnsureReleased waits for a SIGTERM'd
	// process to exit and release its ports before escalating to SIGKILL.
	DefaultTimeout = 10 * time.Second

	// DefaultPollInterval is how often port state is rechecked while waiting.
	DefaultPollInterval = 200 * time.Millisecond

	// killGracePeriod bounds how long EnsureReleased waits, after a SIGKILL,
	// for the kernel to actually tear down the listening socket.
	killGracePeriod = 5 * time.Second
)

// PortFree reports whether nothing is listening on port on any interface.
// Implemented as a real bind attempt (not a parse of lsof/ss output) so it
// needs no external tool and can't be fooled by a platform's differing
// lsof/ss output format.
func PortFree(port int) bool {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// allFree reports whether every port in ports is currently free.
func allFree(ports []int) bool {
	for _, p := range ports {
		if !PortFree(p) {
			return false
		}
	}
	return true
}

// EnsureReleased blocks until every port in opts.Ports is free, actively
// terminating matching processes rather than just waiting and giving up.
//
// Sequence: poll for natural release up to opts.Timeout. If ports are still
// held, find processes matching opts.ProcessNameContains that are actually
// listening on one of opts.Ports, SIGTERM them, and poll again for the same
// duration. If still held, SIGKILL the survivors and poll up to
// killGracePeriod. Returns an error only if ports remain bound after all of
// that — the caller then knows definitively that starting the next process
// would race a real, unkillable holder (e.g. permission denied) rather than
// silently proceeding into a crash loop.
func EnsureReleased(ctx context.Context, opts Options) error {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	poll := opts.PollInterval
	if poll <= 0 {
		poll = DefaultPollInterval
	}

	if waitUntilFree(ctx, opts.Ports, timeout, poll) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("portguard: waiting for ports %v: %w", opts.Ports, err)
	}

	victims, err := findPortHolders(ctx, opts.Ports, opts.ProcessNameContains, opts.ExcludePID)
	if err != nil {
		return fmt.Errorf("portguard: locating processes on %v: %w", opts.Ports, err)
	}

	for _, v := range victims {
		_ = v.SendSignalWithContext(ctx, syscall.SIGTERM)
	}

	if waitUntilFree(ctx, opts.Ports, timeout, poll) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("portguard: waiting after SIGTERM for ports %v: %w", opts.Ports, err)
	}

	for _, v := range victims {
		_ = v.KillWithContext(ctx)
	}

	if waitUntilFree(ctx, opts.Ports, killGracePeriod, poll) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("portguard: waiting after SIGKILL for ports %v: %w", opts.Ports, err)
	}

	return fmt.Errorf("portguard: port(s) %v still bound after terminating %d matching process(es)", stillBound(opts.Ports), len(victims))
}

func stillBound(ports []int) []int {
	var bound []int
	for _, p := range ports {
		if !PortFree(p) {
			bound = append(bound, p)
		}
	}
	return bound
}

func waitUntilFree(ctx context.Context, ports []int, timeout, poll time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()

	for {
		if allFree(ports) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return allFree(ports)
		case <-ticker.C:
		}
	}
}

// findPortHolders returns every process that (a) is listening on one of
// ports, (b) has an executable name containing nameContains (empty matches
// any name), and (c) is not excludePID or this process's own PID.
func findPortHolders(ctx context.Context, ports []int, nameContains string, excludePID int32) ([]*process.Process, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	self := int32(os.Getpid())
	wantPorts := make(map[int]bool, len(ports))
	for _, p := range ports {
		wantPorts[p] = true
	}

	var holders []*process.Process
	for _, p := range procs {
		if p.Pid == self || p.Pid == excludePID {
			continue
		}
		if nameContains != "" {
			name, err := p.NameWithContext(ctx)
			if err != nil || !strings.Contains(name, nameContains) {
				continue
			}
		}
		conns, err := p.ConnectionsWithContext(ctx)
		if err != nil {
			continue
		}
		for _, c := range conns {
			if c.Status == "LISTEN" && wantPorts[int(c.Laddr.Port)] {
				holders = append(holders, p)
				break
			}
		}
	}
	return holders, nil
}
