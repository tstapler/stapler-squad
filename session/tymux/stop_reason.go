package tymux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// StopKind says why squad is stopping tymuxd. Killing the daemon destroys every pane child, so
// each stop must be attributable from the log alone.
type StopKind string

const (
	StopMonitorRecovery StopKind = "monitor_recovery"  // the daemon monitor saw a sustained outage
	StopSquadShutdown   StopKind = "squad_shutdown"    // squad exits and owns the daemon it spawned
	StopVersionReplace  StopKind = "version_replace"   // ReplaceDaemon rolls out a different binary
	StopManual          StopKind = "manual_stop"       // an explicit operator or test request
	StopStaleDaemon     StopKind = "stale_daemon"      // EnsureDaemonRunning clears a dead predecessor before respawning
	StopFailedColdStart StopKind = "failed_cold_start" // reaping a replacement that never became healthy
)

func (k StopKind) human() string {
	switch k {
	case StopMonitorRecovery:
		return "monitor recovery"
	case StopSquadShutdown:
		return "squad shutdown"
	case StopVersionReplace:
		return "version upgrade rollout"
	case StopManual:
		return "manual stop"
	case StopStaleDaemon:
		return "unhealthy daemon cleared before respawn"
	case StopFailedColdStart:
		return "cleanup after failed cold start"
	default:
		return "unspecified"
	}
}

// StopReason is who asked for a stop and the evidence the decision rested on.
type StopReason struct {
	Kind   StopKind
	Detail string       // e.g. "6 consecutive failed probes", "--tymuxd-keep-server=false"
	Probes ProbeSummary // the last TCP and Unix probe results, when the decision was probe-driven
}

// Message is the human-readable form logged and recorded.
func (r StopReason) Message() string {
	if r.Detail == "" {
		return r.Kind.human()
	}
	return r.Kind.human() + ": " + r.Detail
}

// ProbeSummary holds daemonProbeResult names, or probeUnknown when none was recorded.
type ProbeSummary struct {
	TCP, Unix string
}

const (
	probeUnknown   = "unknown"
	probeNotProbed = "not_probed"
)

func (r daemonProbeResult) String() string {
	switch r {
	case daemonProbeNotListening:
		return "not_listening"
	case daemonProbeNoSocketFile:
		return "no_socket_file"
	case daemonProbeUnhealthy:
		return "unhealthy"
	case daemonProbeBusy:
		return "busy"
	case daemonProbeAccessDenied:
		return "access_denied"
	case daemonProbeDegraded:
		return "degraded"
	case daemonProbeHealthy:
		return "healthy"
	default:
		return "invalid(" + strconv.Itoa(int(r)) + ")"
	}
}

// lastProbes remembers the most recent probeDaemonWithTCP outcome per daemon address, so a stop
// can cite the evidence without issuing (and logging) fresh probes.
var lastProbes sync.Map // addr -> ProbeSummary

func recordProbe(addr string, s ProbeSummary) { lastProbes.Store(addr, s) }

func lastProbeFor(addr string) ProbeSummary {
	if v, ok := lastProbes.Load(addr); ok {
		return v.(ProbeSummary)
	}
	return ProbeSummary{TCP: probeUnknown, Unix: probeUnknown}
}

type stopReasonKey struct{}

// withStopReason carries a caller's reason through ensure-style APIs whose signature is fixed.
func withStopReason(ctx context.Context, r StopReason) context.Context {
	return context.WithValue(ctx, stopReasonKey{}, r)
}

func stopReasonFrom(ctx context.Context) (StopReason, bool) {
	r, ok := ctx.Value(stopReasonKey{}).(StopReason)
	return r, ok
}

// StopOutcome is how a stop attempt ended.
type StopOutcome string

const (
	StopAlreadyGone     StopOutcome = "already_gone"
	StopExitedAfterTerm StopOutcome = "exited_after_sigterm"
	StopEscalatedToKill StopOutcome = "escalated_to_sigkill"
	StopSignalFailed    StopOutcome = "signal_failed"
)

// StopEvent is one recorded stop.
type StopEvent struct {
	At       time.Time
	PID      int
	Reason   StopReason
	Age      time.Duration
	AgeKnown bool // false when the process age could not be read
	Outcome  StopOutcome
	Elapsed  time.Duration
	Err      string
}

const stopRingSize = 50

var stopRing struct {
	sync.Mutex
	events []StopEvent
}

func appendStop(e StopEvent) {
	stopRing.Lock()
	defer stopRing.Unlock()
	if len(stopRing.events) == stopRingSize {
		stopRing.events = append(stopRing.events[:0], stopRing.events[1:]...)
	}
	stopRing.events = append(stopRing.events, e)
}

// RecentDaemonStops returns up to the last 50 stop events, oldest first.
func RecentDaemonStops() []StopEvent {
	stopRing.Lock()
	defer stopRing.Unlock()
	return append([]StopEvent(nil), stopRing.events...)
}

// Seams so tests need no real `ps` or wall clock.
var (
	stopNowFn    = time.Now
	processAgeFn = processAge
)

// processAge reports how long pid has been running via `ps -o etime=`, which macOS and Linux share.
func processAge(ctx context.Context, pid int) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, daemonHealthCheckTimeout)
	defer cancel()
	out, err := safeexec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "etime=").Output()
	if err != nil {
		return 0, err
	}
	return parseEtime(string(out))
}

// parseEtime parses ps's [[DD-]HH:]MM:SS.
func parseEtime(s string) (time.Duration, error) {
	orig := strings.TrimSpace(s)
	s = orig
	days := 0
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("bad etime %q", orig)
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, fmt.Errorf("bad etime %q", orig)
	}
	secs := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("bad etime %q", orig)
		}
		secs = secs*60 + n
	}
	return time.Duration(days)*24*time.Hour + time.Duration(secs)*time.Second, nil
}

// stopAttempt logs and records one stop: the request before any signal, the outcome after.
type stopAttempt struct {
	event StopEvent
	start time.Time
}

func beginStop(pid int, reason StopReason) *stopAttempt {
	a := &stopAttempt{start: stopNowFn()}
	a.event = StopEvent{At: a.start, PID: pid, Reason: reason}
	ageField := "unknown"
	if age, err := processAgeFn(context.Background(), pid); err == nil {
		a.event.Age, a.event.AgeKnown = age, true
		ageField = age.Round(time.Second).String()
	}
	tcp, unix := reason.Probes.TCP, reason.Probes.Unix
	if tcp == "" {
		tcp = probeUnknown
	}
	if unix == "" {
		unix = probeUnknown
	}
	log.Warn("[tymux] stopping tymuxd", "pid", pid, "daemon_age", ageField,
		"reason_kind", string(reason.Kind), "reason", reason.Message(),
		"tcp_probe", tcp, "unix_probe", unix)
	return a
}

func (a *stopAttempt) finish(outcome StopOutcome, err error) {
	a.event.Outcome = outcome
	a.event.Elapsed = stopNowFn().Sub(a.start)
	if err != nil {
		a.event.Err = err.Error()
	}
	appendStop(a.event)
	log.Warn("[tymux] tymuxd stop finished", "pid", a.event.PID, "outcome", string(outcome),
		"elapsed", a.event.Elapsed.Round(time.Millisecond), "reason_kind", string(a.event.Reason.Kind), "err", err)
}

// ShutdownStopReason attributes a squad-exit stop of a daemon this process spawned.
var ShutdownStopReason = StopReason{Kind: StopSquadShutdown, Detail: "--tymuxd-keep-server=false"}
