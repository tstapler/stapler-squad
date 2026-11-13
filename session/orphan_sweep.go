package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// ReconcileOrphanedTmuxSessions kills staplersquad_ tmux sessions that have no
// corresponding record in the current workspace DB.
//
// Orphans accumulate when DeleteSession removes the DB record but the server is
// restarted before (or while) the live in-memory instance is available — leaving
// the Claude process running inside a tmux pane with no owner. This was originally
// called only once during server startup, after all DB sessions have been loaded
// and re-adopted (steps 6/6b of BuildRuntimeDeps), specifically because at that
// point in the sequence no new session creation can be racing it. OrphanedTmuxSweeper
// now also calls this periodically during live operation to catch orphans that
// accumulate between restarts (e.g. a crash mid-DeleteSession, or a KillTmuxPaneOnly
// call in archiveItemWorkSessions that silently failed) — see its doc comment for
// why minAge exists and is mandatory there.
//
// Identification strategy:
//  1. Tmux session has STAPLER_SESSION_UUID env var → compare against known UUIDs.
//  2. No env var (pre-UUID sessions, and all shell sibling sessions) → compare the
//     tmux session name against known instance tmux names and known shell tmux
//     names. If none of the above match, the session is an orphan.
//
// The staplersquad_keepalive sentinel is always preserved — it keeps the tmux
// server alive between sessions and is never tracked in the DB.
//
// minAge is a grace period: a candidate orphan whose tmux session is younger than
// minAge is never killed, regardless of whether it appears in instances. This
// closes an unavoidable race for any periodic (post-startup) caller — CreateSession
// calls instance.Start() (which creates the tmux session) before it registers the
// new instance with the live provider a periodic sweep reads from (AddInstance),
// so a sweep tick landing in that window would otherwise see a legitimate
// brand-new session as an orphan. Pass 0 only for the one-time startup call, where
// this race cannot occur (no new session creation is possible before BuildRuntimeDeps
// finishes). Any periodic caller MUST pass a nonzero minAge (production passes
// orphanTmuxMinAge, 5m). The unparseable-created-time skip applies only when minAge > 0.
func ReconcileOrphanedTmuxSessions(instances []*Instance, minAge time.Duration) {
	// Resolve once, here -- not per-command below. This is what actually makes this
	// function test-safe: inside a `go test` binary it targets a per-process
	// isolated socket instead of the real shared default, so it can never again
	// enumerate-and-kill sessions belonging to some other, currently-running
	// stapler-squad process on the same machine. See tmux.ResolveSocket's doc
	// comment for the incident history this closes.
	socket := tmux.ResolveSocket("")
	socketArgs := socket.Args
	exec := func(ctx context.Context, args ...string) ([]byte, string, error) {
		cmd := safeexec.CommandContext(ctx, tmux.Binary(), socketArgs(args...)...)
		cmd.Env = tmux.UTF8Env(os.Environ())
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return stdout.Bytes(), stderr.String(), err
	}
	run := newControlModeFirstRunner(controlModeSessionsOn(instances, socket.String()), exec)
	reconcileOrphanedTmuxSessions(instances, minAge, run, time.Now)
}

// controlModeSessionsOn returns the live tmux sessions whose control-mode client is
// attached to the server on socket, so the sweep can reuse them instead of forking.
func controlModeSessionsOn(instances []*Instance, socket string) []*tmux.TmuxSession {
	var out []*tmux.TmuxSession
	for _, inst := range instances {
		if ts := inst.GetTmuxSession(); ts != nil && ts.ServerSocket() == socket {
			out = append(out, ts)
		}
	}
	return out
}

// controlModeCommander is the slice of *tmux.TmuxSession the sweep needs.
type controlModeCommander interface {
	ServerCommand(ctx context.Context, args ...string) (string, error)
}

// cmReadAttemptTimeout bounds each control-mode attempt of a read-only command so a
// wedged client falls through to the next client or the exec fallback instead of
// consuming the whole step budget.
var cmReadAttemptTimeout = 3 * time.Second

func isReadOnlyTmuxCommand(args []string) bool {
	return len(args) > 0 && (args[0] == "list-sessions" || args[0] == "show-environment")
}

// newControlModeFirstRunner routes each tmux command over the first control-mode
// client that accepts it and falls back to fallback (a subprocess) when none does.
// Read-only commands also move on after a client timeout; a command that reached tmux
// and failed, and any kill-session timeout, is returned as-is (never retried), so a
// failure stays fail-closed.
func newControlModeFirstRunner[C controlModeCommander](clients []C, fallback tmuxRunFunc) tmuxRunFunc {
	return func(ctx context.Context, args ...string) ([]byte, string, error) {
		cmArgs, quoteErr := quoteControlModeArgs(args)
		if quoteErr != nil {
			// exec argv is not re-parsed by tmux, so it handles names control mode can't.
			return fallback(ctx, args...)
		}
		readOnly := isReadOnlyTmuxCommand(args)
		for _, c := range clients {
			attemptCtx, cancel := ctx, context.CancelFunc(func() {})
			if readOnly {
				attemptCtx, cancel = context.WithTimeout(ctx, cmReadAttemptTimeout)
			}
			body, err := c.ServerCommand(attemptCtx, cmArgs...)
			cancel()
			if errors.Is(err, tmux.ErrControlModeNotRunning) || errors.Is(err, tmux.ErrControlModeStopped) {
				continue
			}
			if readOnly && errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				continue
			}
			if err != nil {
				// Control mode reports tmux's error text as the error; surface it as stderr.
				return nil, err.Error(), err
			}
			return []byte(body), "", nil
		}
		return fallback(ctx, args...)
	}
}

var plainCMArgRe = regexp.MustCompile(`^[A-Za-z0-9_=.-]+$`)

// quoteControlModeArgs single-quotes args for a control-mode command line, which
// tmux re-parses (unlike exec argv): '#', ';' and spaces would otherwise be
// comments, command separators or word breaks. An arg containing a single quote,
// backslash or control character is refused rather than escaped.
func quoteControlModeArgs(args []string) ([]string, error) {
	out := make([]string, len(args))
	for i, a := range args {
		switch {
		case plainCMArgRe.MatchString(a):
			out[i] = a
		case strings.ContainsAny(a, "'\\") || strings.IndexFunc(a, unicode.IsControl) >= 0:
			return nil, fmt.Errorf("orphan sweep: refusing to send unquotable tmux argument %q over control mode", a)
		default:
			out[i] = "'" + a + "'"
		}
	}
	return out, nil
}

// tmuxRunFunc runs one tmux command and returns its stdout, stderr and error.
type tmuxRunFunc func(ctx context.Context, args ...string) (stdout []byte, stderr string, err error)

var lastSweepFailureLogUnix atomic.Int64

// logSweepFailure warns about a skipped sweep at most once per 10 minutes, so a
// persistent failure is visible without a line every 5-minute tick.
func logSweepFailure(at time.Time, msg string, err error) {
	last := lastSweepFailureLogUnix.Load()
	if at.Unix()-last < 600 || !lastSweepFailureLogUnix.CompareAndSwap(last, at.Unix()) {
		return
	}
	log.Warn("orphan sweep: "+msg, "err", err)
}

const (
	orphanListTimeout = 10 * time.Second
	orphanStepTimeout = 5 * time.Second
)

// mangledSuffixRe matches the "_<unix-seconds>" tail produced when tmux prints the
// list format's separator as "_" (no UTF-8 locale) and the created time fuses onto
// the name.
var safeSessionNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// mangledSuffixRe is belt-and-suspenders: with the '|' separator a mangled line is
// already rejected by the missing-separator check; this catches a name+epoch that
// still carries a separator.
var mangledSuffixRe = regexp.MustCompile(`_\d{9,}$`)

// sweepVerdict is the decision for one candidate tmux session.
type sweepVerdict int

const (
	verdictIgnore sweepVerdict = iota // not ours, or known: leave alone silently
	verdictUnsure                     // can't prove it is an orphan: skip and count
	verdictYoung                      // inside the grace period
	verdictOrphan                     // proven orphan: eligible for the pre-kill recheck
)

// knownSessions is what the live instance set says exists.
type knownSessions struct {
	uuids map[string]struct{}
	names map[string]struct{}
}

func collectKnownSessions(instances []*Instance) knownSessions {
	k := knownSessions{
		uuids: make(map[string]struct{}, len(instances)),
		names: map[string]struct{}{tmux.TmuxPrefix + "keepalive": {}},
	}
	for _, inst := range instances {
		if inst.UUID != "" {
			k.uuids[inst.UUID] = struct{}{}
		}
		if name := inst.GetTmuxSessionName(); name != "" {
			k.names[name] = struct{}{}
		}
		// Shells are independent sibling tmux sessions (see instance_shells.go) with
		// no Instance-level identity of their own — without this, every shell is
		// unconditionally treated as an orphan and killed on the next sweep/restart.
		for _, shell := range inst.ListShellsInMemory() {
			if shell.TmuxSessionName != "" {
				k.names[shell.TmuxSessionName] = struct{}{}
			}
		}
	}
	return k
}

// sessionLine is one parsed list-sessions line.
type sessionLine struct {
	raw, name, createdRaw string
}

// parseSessionLine splits on the LAST separator so the created time is always the
// final field. ok is false for a line that can't be parsed (separator missing/mangled).
func parseSessionLine(line string) (sessionLine, bool) {
	cut := strings.LastIndex(line, tmux.FormatSep)
	if cut < 0 {
		return sessionLine{}, false
	}
	return sessionLine{raw: line, name: line[:cut], createdRaw: line[cut+len(tmux.FormatSep):]}, true
}

// classifyByName applies the name-only rules: ours?, known?, fused-epoch artifact?
func (k knownSessions) classifyByName(name string) sweepVerdict {
	if name == "" || !strings.HasPrefix(name, tmux.TmuxPrefix) {
		return verdictIgnore
	}
	if !safeSessionNameRe.MatchString(name) {
		// Not a name this app creates (control chars, spaces, ...): never act on it.
		return verdictUnsure
	}
	if _, known := k.names[name]; known {
		return verdictIgnore
	}
	// Round-trip guard: a known name plus a fused "_<epoch>" tail is a parsing
	// artifact of a live session, not an orphan.
	if base := mangledSuffixRe.ReplaceAllString(name, ""); base != name {
		if _, known := k.names[base]; known {
			log.Warn("orphan sweep: skipping name that looks like a known session with a fused timestamp", "session", name)
			return verdictUnsure
		}
	}
	return verdictOrphan
}

// classifyByAge applies the grace period: a session younger than minAge is never an
// orphan (see ReconcileOrphanedTmuxSessions' doc). An unparseable creation time fails
// closed. minAge == 0 disables the check.
func classifyByAge(name, createdRaw string, minAge time.Duration, now time.Time) sweepVerdict {
	if minAge <= 0 {
		return verdictOrphan
	}
	createdUnix, err := strconv.ParseInt(createdRaw, 10, 64)
	if err != nil {
		return verdictUnsure
	}
	if age := now.Sub(time.Unix(createdUnix, 0)); age < minAge {
		log.Debug("orphan sweep: skipping recently created session, still within grace period",
			"session", name, "age", age.Round(time.Second))
		return verdictYoung
	}
	return verdictOrphan
}

// classifyByUUID reads STAPLER_SESSION_UUID from the session environment. Only an
// explicit "unknown variable" reply means "no UUID"; any other failure is unsure.
func (k knownSessions) classifyByUUID(run tmuxRunFunc, target string) sweepVerdict {
	ctx, cancel := context.WithTimeout(context.Background(), orphanStepTimeout)
	defer cancel()
	out, stderr, err := run(ctx, "show-environment", "-t", target, "STAPLER_SESSION_UUID")
	switch {
	case err == nil:
		uuid, ok := strings.CutPrefix(strings.TrimSpace(string(out)), "STAPLER_SESSION_UUID=")
		if !ok || uuid == "" {
			return verdictUnsure // success with an unexpected body
		}
		if _, known := k.uuids[uuid]; known {
			// A DB session that was renamed or sanitized differently. Don't kill it.
			return verdictIgnore
		}
		return verdictOrphan
	case strings.Contains(stderr, "unknown variable: STAPLER_SESSION_UUID"):
		return verdictOrphan // pre-UUID session
	default:
		return verdictUnsure // timeout, vanished session, or any other failure
	}
}

// stillSameSession re-reads the target right before the kill: the name must still
// exist with the same creation time (guards against a replaced session or stale list).
func stillSameSession(run tmuxRunFunc, c sessionLine) bool {
	ctx, cancel := context.WithTimeout(context.Background(), orphanStepTimeout)
	defer cancel()
	out, _, err := run(ctx, "list-sessions", "-F", "#{session_name}"+tmux.FormatSep+"#{session_created}",
		"-f", "#{==:#{session_name},"+c.name+"}")
	return err == nil && strings.TrimSpace(string(out)) == c.raw
}

// killSession kills the session by exact-match target and reports success.
func killSession(run tmuxRunFunc, name string) bool {
	log.Info("orphan sweep: killing stale tmux session with no DB record", "session", name)
	ctx, cancel := context.WithTimeout(context.Background(), orphanStepTimeout)
	defer cancel()
	if _, _, err := run(ctx, "kill-session", "-t", "="+name); err != nil {
		log.Warn("orphan sweep: failed to kill stale session", "session", name, "err", err)
		return false
	}
	return true
}

// decide runs every orphan test for one candidate in order; the first non-orphan
// verdict wins. Only verdictOrphan lets the caller proceed to the recheck and kill.
func (k knownSessions) decide(run tmuxRunFunc, c sessionLine, minAge time.Duration, now time.Time) sweepVerdict {
	if v := k.classifyByName(c.name); v != verdictOrphan {
		return v
	}
	if v := classifyByAge(c.name, c.createdRaw, minAge, now); v != verdictOrphan {
		return v
	}
	return k.classifyByUUID(run, "="+c.name) // "=" targets the exact name; plain -t prefix/glob-matches
}

func reconcileOrphanedTmuxSessions(instances []*Instance, minAge time.Duration, run tmuxRunFunc, now func() time.Time) {
	known := collectKnownSessions(instances)

	// #{session_created} (unix seconds) rides along in the same call so the age
	// check needs no extra per-candidate subprocess round-trip.
	listCtx, listCancel := context.WithTimeout(context.Background(), orphanListTimeout)
	out, _, err := run(listCtx, "list-sessions", "-F", "#{session_name}"+tmux.FormatSep+"#{session_created}")
	listCancel()
	if err != nil {
		// tmux not running, no sessions, or the call timed out — fail closed.
		logSweepFailure(now(), "list-sessions failed, skipping sweep", err)
		return
	}

	killed, skippedYoung, skippedUnsure := 0, 0, 0
	for _, raw := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		c, ok := parseSessionLine(raw)
		if !ok {
			skippedUnsure++ // unparseable line: never act on it
			continue
		}
		switch known.decide(run, c, minAge, now()) {
		case verdictUnsure:
			skippedUnsure++
		case verdictYoung:
			skippedYoung++
		case verdictOrphan:
			if !stillSameSession(run, c) {
				skippedUnsure++
			} else if killSession(run, c.name) {
				killed++
			}
		}
	}

	if killed > 0 || skippedYoung > 0 || skippedUnsure > 0 {
		log.Info("orphan sweep: complete", "killed", killed,
			"skipped_within_grace_period", skippedYoung, "skipped_unsure", skippedUnsure)
	}
}
