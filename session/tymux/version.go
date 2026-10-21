package tymux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/mod/semver"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// VersionDirection is what replacing the running daemon with the configured binary would do.
type VersionDirection int

const (
	VersionUnknown VersionDirection = iota
	VersionSame
	VersionUpgrade
	VersionDowngrade
)

// DaemonVersionInfo compares the tymuxd process serving cfg.Addr with the binary
// stapler-squad would spawn (cfg.BinaryPath). Empty version strings mean undeterminable.
type DaemonVersionInfo struct {
	Running          bool
	PID              int
	RunningVersion   string
	InstalledVersion string
	Direction        VersionDirection
}

// ErrLiveSessionsWouldBeInterrupted is returned by ReplaceDaemon when sessions are
// live and the caller did not opt in: a restarted daemon respawns each pane's process.
var ErrLiveSessionsWouldBeInterrupted = errors.New("tymux: replacing the daemon would interrupt live sessions")

const (
	versionProbeTimeout    = 5 * time.Second
	daemonStopGracePeriod  = 10 * time.Second
	daemonStopPollInterval = 100 * time.Millisecond
)

// Injection seams so tests need no real daemon, lsof, or /proc.
var (
	binaryVersionFn = binaryVersion
	listenerPIDFn   = listenerPID
	processExeFn    = processExe
	liveSessionsFn  = countLiveSessions
	signalProcessFn = func(pid int, sig os.Signal) error {
		proc, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return proc.Signal(sig)
	}
)

// ParseTymuxdVersion extracts the semver from `tymuxd --version` output ("tymuxd 1.3.0").
func ParseTymuxdVersion(output string) (string, error) {
	fields := strings.Fields(output)
	if len(fields) != 2 || fields[0] != "tymuxd" || !semver.IsValid("v"+fields[1]) {
		return "", fmt.Errorf("tymux: unrecognized tymuxd --version output %q", strings.TrimSpace(output))
	}
	return fields[1], nil
}

// CompareVersions reports what moving from the running version to the installed one means.
func CompareVersions(running, installed string) VersionDirection {
	if !semver.IsValid("v"+running) || !semver.IsValid("v"+installed) {
		return VersionUnknown
	}
	switch semver.Compare("v"+installed, "v"+running) {
	case 0:
		return VersionSame
	case 1:
		return VersionUpgrade
	default:
		return VersionDowngrade
	}
}

func binaryVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()
	out, err := safeexec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("tymux: %s --version: %w", path, err)
	}
	return ParseTymuxdVersion(string(out))
}

var ssPIDPattern = regexp.MustCompile(`pid=(\d+)`)

// listenerPID finds the process listening on addr's port. The pid file can't be used:
// it records only the last daemon this process spawned and goes stale across restarts.
func listenerPID(ctx context.Context, addr string) (int, error) {
	hostPort, err := addrToHostPort(addr)
	if err != nil {
		return 0, err
	}
	port := hostPort[strings.LastIndex(hostPort, ":")+1:]
	ctx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()

	if out, err := safeexec.CommandContext(ctx, "lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN", "-t").Output(); err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])); perr == nil {
			return pid, nil
		}
	}
	out, err := safeexec.CommandContext(ctx, "ss", "-ltnpH", "sport = :"+port).Output()
	if err != nil {
		return 0, fmt.Errorf("tymux: cannot find the listener on port %s (tried lsof, ss): %w", port, err)
	}
	if m := ssPIDPattern.FindStringSubmatch(string(out)); m != nil {
		if pid, perr := strconv.Atoi(m[1]); perr == nil {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("tymux: no listener found on port %s", port)
}

// processExe returns the executable path of pid. /proc/<pid>/exe still execs
// after the file on disk was replaced; macOS falls back to ps.
func processExe(ctx context.Context, pid int) (string, error) {
	if p, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		if strings.HasSuffix(p, " (deleted)") {
			return fmt.Sprintf("/proc/%d/exe", pid), nil
		}
		return p, nil
	}
	ctx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()
	out, err := safeexec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return "", fmt.Errorf("tymux: cannot resolve executable of pid %d", pid)
	}
	return strings.TrimSpace(string(out)), nil
}

// DetectDaemonVersions probes the running daemon and the configured binary. Individual
// probe failures leave the matching field empty rather than failing the whole call.
func DetectDaemonVersions(ctx context.Context, cfg DaemonConfig) DaemonVersionInfo {
	info := DaemonVersionInfo{}
	if v, err := binaryVersionFn(ctx, cfg.BinaryPath); err == nil {
		info.InstalledVersion = v
	} else {
		log.Debug("[tymux] could not read installed tymuxd version", "err", err)
	}
	pid, err := listenerPIDFn(ctx, cfg.Addr)
	if err != nil {
		return info
	}
	info.Running, info.PID = true, pid
	exe, err := processExeFn(ctx, pid)
	if err != nil {
		return info
	}
	if v, err := binaryVersionFn(ctx, exe); err == nil {
		info.RunningVersion = v
	}
	info.Direction = CompareVersions(info.RunningVersion, info.InstalledVersion)
	return info
}

func countLiveSessions(ctx context.Context, cfg DaemonConfig) (int, error) {
	resp, err := NewRealTransport(cfg.Addr).ListSessions(ctx, connect.NewRequest(&v1.ListSessionsRequest{}))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, s := range resp.Msg.GetSessions() {
		if s.GetLiveness() == v1.Liveness_LIVENESS_LIVE {
			n++
		}
	}
	return n, nil
}

// ReplaceDaemon stops the daemon serving cfg.Addr and starts cfg.BinaryPath in its place,
// which upgrades or downgrades it depending on that binary. Live sessions survive in
// tymuxd's persisted state but each pane's process is respawned on revive, so unless
// allowInterrupt is set it refuses (ErrLiveSessionsWouldBeInterrupted) while any are live.
func ReplaceDaemon(ctx context.Context, cfg DaemonConfig, allowInterrupt bool) (DaemonVersionInfo, error) {
	before := DetectDaemonVersions(ctx, cfg)
	if before.Running {
		live, err := liveSessionsFn(ctx, cfg)
		if err != nil {
			return before, fmt.Errorf("tymux: cannot count live sessions before replacing the daemon: %w", err)
		}
		if live > 0 && !allowInterrupt {
			return before, fmt.Errorf("%w (%d live)", ErrLiveSessionsWouldBeInterrupted, live)
		}
		if err := stopListener(before.PID, cfg); err != nil {
			return before, err
		}
	}
	if _, err := EnsureDaemonRunning(ctx, cfg); err != nil {
		return before, err
	}
	log.Info("[tymux] replaced tymuxd", "from", before.RunningVersion, "to", cfg.BinaryPath)
	return DetectDaemonVersions(ctx, cfg), nil
}

// stopListener SIGTERMs pid (tymuxd flushes its session records on SIGTERM), waits for
// the port to free, then escalates to SIGKILL.
func stopListener(pid int, cfg DaemonConfig) error {
	if err := signalProcessFn(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("tymux: SIGTERM pid %d: %w", pid, err)
	}
	if waitPortFree(cfg, daemonStopGracePeriod) {
		return nil
	}
	log.Warn("[tymux] tymuxd ignored SIGTERM, escalating to SIGKILL", "pid", pid)
	if err := signalProcessFn(pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("tymux: SIGKILL pid %d: %w", pid, err)
	}
	if !waitPortFree(cfg, daemonStopGracePeriod) {
		return fmt.Errorf("tymux: pid %d still holds %s after SIGKILL", pid, cfg.Addr)
	}
	return nil
}

func waitPortFree(cfg DaemonConfig, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !portListeningFn(cfg) {
			return true
		}
		time.Sleep(daemonStopPollInterval)
	}
	return !portListeningFn(cfg)
}
