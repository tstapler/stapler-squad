package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// TestReconcileOrphanedTmuxSessions_UsesIsolatedSocket is the regression guard for the
// incident where ReconcileOrphanedTmuxSessions issued raw `tmux list-sessions` /
// `show-environment` / `kill-session` with no socket argument, always targeting the
// shared, machine-wide default tmux socket. That meant any test process calling
// BuildDependencies() on the same machine as a running production instance would
// enumerate and kill *that* instance's sessions. This replaces the tmux binary with a
// fake script that records its argv and asserts every invocation is scoped to the
// resolved per-process isolated socket rather than the shared default.
func TestReconcileOrphanedTmuxSessions_UsesIsolatedSocket(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")
	fakeTmux := filepath.Join(dir, "tmux")

	script := `#!/bin/sh
echo "$@" >> "` + logPath + `"
case "$*" in
  *list-sessions*) echo "staplersquad_orphan-test|1"; exit 0 ;;
  *show-environment*) echo "unknown variable: STAPLER_SESSION_UUID" >&2; exit 1 ;;
  *kill-session*) exit 0 ;;
esac
exit 1
`
	require.NoError(t, os.WriteFile(fakeTmux, []byte(script), 0o755))
	t.Setenv("TMUX_BIN", fakeTmux)

	// No known instances -- the fake "staplersquad_orphan-test" session is an orphan,
	// which drives all three call sites (list-sessions, show-environment, kill-session).
	// minAge=0 so the (ancient) created time never grace-skips it.
	ReconcileOrphanedTmuxSessions(nil, 0)

	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err, "expected the fake tmux binary to have been invoked")

	lines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "expected list-sessions, show-environment, and kill-session calls")

	for _, line := range lines {
		require.True(t, strings.HasPrefix(line, "-L "), "every tmux invocation must be socket-scoped, got: %q", line)
		fields := strings.Fields(line)
		require.GreaterOrEqual(t, len(fields), 2)
		socket := fields[1]
		require.NotEmpty(t, socket, "socket must never resolve to empty and fall through to the shared default")
		require.Contains(t, socket, "test-isolated-", "socket must be the per-process isolated socket, not the shared default")
	}
}

// TestReconcileOrphanedTmuxSessions_PreservesLiveShellSessions is the regression guard
// for the bug where shell sibling tmux sessions (staplersquad_{parent}_shell_{shellID})
// were unconditionally treated as orphans: the sweep only ever checked instance-level
// tmux names/UUIDs and never looked at an instance's shells, so a freshly-spawned shell
// with no Instance-level identity of its own was killed on the very next sweep/restart.
func TestReconcileOrphanedTmuxSessions_PreservesLiveShellSessions(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "argv.log")
	fakeTmux := filepath.Join(dir, "tmux")

	script := `#!/bin/sh
echo "$@" >> "` + logPath + `"
case "$*" in
  *list-sessions*) echo "staplersquad_parent_shell_shell-1|1"; exit 0 ;;
  *show-environment*) echo "unknown variable: STAPLER_SESSION_UUID" >&2; exit 1 ;;
  *kill-session*) exit 0 ;;
esac
exit 1
`
	require.NoError(t, os.WriteFile(fakeTmux, []byte(script), 0o755))
	t.Setenv("TMUX_BIN", fakeTmux)

	inst := &instance.Instance{}
	inst.AddStoppedShellForTest(&instance.Shell{
		ID:              "shell-1",
		TmuxSessionName: "staplersquad_parent_shell_shell-1",
	})

	ReconcileOrphanedTmuxSessions([]*instance.Instance{inst}, 0)

	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)

	logStr := string(logBytes)
	require.Contains(t, logStr, "list-sessions", "expected list-sessions to have been called")
	require.NotContains(t, logStr, "kill-session", "the known shell session must not be killed")
}

// newFakeTmuxWithSessionAge writes a fake tmux script that reports one orphan-looking
// session ("staplersquad_orphan-test") created createdAgo in the past, and returns the
// argv-log path plus the script path (for TMUX_BIN).
func newFakeTmuxWithSessionAge(t *testing.T, createdAgo time.Duration) (logPath, fakeTmux string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "argv.log")
	fakeTmux = filepath.Join(dir, "tmux")

	createdUnix := time.Now().Add(-createdAgo).Unix()
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> "%s"
case "$*" in
  *list-sessions*) echo "staplersquad_orphan-test|%d"; exit 0 ;;
  *show-environment*) echo "unknown variable: STAPLER_SESSION_UUID" >&2; exit 1 ;;
  *kill-session*) exit 0 ;;
esac
exit 1
`, logPath, createdUnix)
	require.NoError(t, os.WriteFile(fakeTmux, []byte(script), 0o755))
	return logPath, fakeTmux
}

// TestReconcileOrphanedTmuxSessions_GracePeriodProtectsNewSession is the regression
// guard for the registration race a periodic (post-startup) sweep introduces: a tmux
// session created moments ago — inside CreateSession's window between instance.Start()
// and the new instance being registered with the live provider — must never be killed
// as a false-positive orphan just because it doesn't appear in instances yet.
func TestReconcileOrphanedTmuxSessions_GracePeriodProtectsNewSession(t *testing.T) {
	logPath, fakeTmux := newFakeTmuxWithSessionAge(t, 10*time.Second)
	t.Setenv("TMUX_BIN", fakeTmux)

	ReconcileOrphanedTmuxSessions(nil, 5*time.Minute)

	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.NotContains(t, string(logBytes), "kill-session",
		"a session younger than minAge must never be killed, regardless of DB knowledge")
}

// TestReconcileOrphanedTmuxSessions_KillsOldOrphanDespiteGracePeriod is the companion
// case: a genuinely stale, unknown session older than minAge is still killed — the
// grace period must not silently disable orphan detection altogether.
func TestReconcileOrphanedTmuxSessions_KillsOldOrphanDespiteGracePeriod(t *testing.T) {
	logPath, fakeTmux := newFakeTmuxWithSessionAge(t, 20*time.Minute)
	t.Setenv("TMUX_BIN", fakeTmux)

	ReconcileOrphanedTmuxSessions(nil, 5*time.Minute)

	logBytes, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Contains(t, string(logBytes), "kill-session",
		"a session older than minAge with no DB record must still be killed")
}

// fakeTmux is an injected tmux runner: it serves canned list-sessions output and
// records every command so tests can assert kill-session was (not) called.
type fakeTmux struct {
	mu       sync.Mutex
	list     string
	listErr  error
	envErr   string // stderr returned for show-environment ("" = succeed with no UUID line)
	envFails bool
	calls    [][]string
}

func (f *fakeTmux) run(_ context.Context, args ...string) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, args)
	switch args[0] {
	case "list-sessions":
		if len(args) > 4 && args[3] == "-f" { // pre-kill recheck: filter to one exact name
			want := strings.TrimSuffix(strings.TrimPrefix(args[4], "#{==:#{session_name},"), "}")
			for _, l := range strings.Split(f.list, "\n") {
				if strings.HasPrefix(l, want+"|") {
					return []byte(l + "\n"), "", nil
				}
			}
			return nil, "", nil
		}
		return []byte(f.list), "", f.listErr
	case "show-environment":
		if f.envFails {
			return nil, f.envErr, errors.New("exit status 1")
		}
		return nil, "", nil
	}
	return nil, "", nil
}

func (f *fakeTmux) killTargets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if c[0] == "kill-session" {
			out = append(out, c[len(c)-1])
		}
	}
	return out
}

// namedPM gives a bare Instance a fixed tmux session name.
type namedPM struct {
	instance.ProcessManager
	id string
}

func (p namedPM) GetSessionIdentifier() string { return p.id }

func liveInstance(tmuxName string) *instance.Instance {
	return (&instance.Instance{Title: tmuxName}).WithProcessManagerForTest(namedPM{id: tmuxName})
}

func sweepNow() time.Time { return time.Unix(2_000_000_000, 0) }

func TestReconcileOrphans_should_NotKillLiveSessions_When_TmuxMangledSeparatorToUnderscore(t *testing.T) {
	t.Parallel()
	// Observed 2026-11-05: tmux 3.8 without a UTF-8 locale printed the tab as "_",
	// so every live session looked like "<name>_<created epoch>".
	live := liveInstance("staplersquad_dotfiles")
	f := &fakeTmux{list: "staplersquad_dotfiles_1793909015\nstaplersquad_keepalive_1793846962\n", envFails: true, envErr: "no such session"}

	reconcileOrphanedTmuxSessions([]*instance.Instance{live}, 5*time.Minute, f.run, sweepNow)

	require.Empty(t, f.killTargets(), "mangled name_epoch lines must never reach kill-session")
}

func TestReconcileOrphans_should_ReapTrueOrphan_When_OlderThanMinAge(t *testing.T) {
	t.Parallel()
	live := liveInstance("staplersquad_dotfiles")
	f := &fakeTmux{list: "staplersquad_dotfiles|1793909015\nstaplersquad_keepalive|1793846962\nstaplersquad_ghost|1793909015\n", envFails: true, envErr: "unknown variable: STAPLER_SESSION_UUID"}

	reconcileOrphanedTmuxSessions([]*instance.Instance{live}, 5*time.Minute, f.run, sweepNow)

	require.Equal(t, []string{"=staplersquad_ghost"}, f.killTargets(), "only the true orphan is killed, with an exact-match target")
}

func TestReconcileOrphans_should_SkipOrphan_When_YoungOrCreatedUnparseable(t *testing.T) {
	t.Parallel()
	young := sweepNow().Add(-time.Minute).Unix()
	f := &fakeTmux{
		list:     fmt.Sprintf("staplersquad_young|%d\nstaplersquad_badtime|abc\nstaplersquad_nosep\n", young),
		envFails: true, envErr: "unknown variable: STAPLER_SESSION_UUID",
	}

	reconcileOrphanedTmuxSessions(nil, 5*time.Minute, f.run, sweepNow)

	require.Empty(t, f.killTargets())
}

func TestReconcileOrphans_should_FailClosed_When_ListOrEnvLookupErrors(t *testing.T) {
	t.Parallel()
	listFail := &fakeTmux{listErr: context.DeadlineExceeded}
	reconcileOrphanedTmuxSessions(nil, 0, listFail.run, sweepNow)
	require.Empty(t, listFail.killTargets())

	for name, stderr := range map[string]string{"timeout": "signal: killed", "vanished": "no such session: =x"} {
		f := &fakeTmux{list: "staplersquad_x|1793909015\n", envFails: true, envErr: stderr}
		reconcileOrphanedTmuxSessions(nil, 0, f.run, sweepNow)
		require.Empty(t, f.killTargets(), name)
	}
}

func TestReconcileOrphans_should_NotKill_When_UUIDMatchesKnownSession(t *testing.T) {
	t.Parallel()
	live := liveInstance("staplersquad_dotfiles")
	live.UUID = "uuid-1"
	f := &uuidFake{fakeTmux: fakeTmux{list: "staplersquad_renamed|1793909015\n"}}

	reconcileOrphanedTmuxSessions([]*instance.Instance{live}, 5*time.Minute, f.run, sweepNow)

	require.Empty(t, f.killTargets())
}

func TestReconcileOrphans_should_NotKill_When_NameIsKnownNameWithFusedEpoch(t *testing.T) {
	t.Parallel()
	live := liveInstance("staplersquad_dotfiles")
	f := &fakeTmux{list: "staplersquad_dotfiles_1793909015|1793909015\n", envFails: true, envErr: "unknown variable: STAPLER_SESSION_UUID"}

	reconcileOrphanedTmuxSessions([]*instance.Instance{live}, 5*time.Minute, f.run, sweepNow)

	require.Empty(t, f.killTargets())
}

func TestReconcileOrphans_should_NotKill_When_ShowEnvironmentSucceedsWithUnexpectedBody(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "-STAPLER_SESSION_UUID", "garbage"} {
		f := &bodyFake{fakeTmux: fakeTmux{list: "staplersquad_x|1793909015\n"}, envBody: body}
		reconcileOrphanedTmuxSessions(nil, 0, f.run, sweepNow)
		require.Empty(t, f.killTargets(), body)
	}
}

type bodyFake struct {
	fakeTmux
	envBody string
}

func (f *bodyFake) run(ctx context.Context, args ...string) ([]byte, string, error) {
	if args[0] == "show-environment" {
		return []byte(f.envBody), "", nil
	}
	return f.fakeTmux.run(ctx, args...)
}

// uuidFake answers show-environment with a fixed STAPLER_SESSION_UUID.
type uuidFake struct{ fakeTmux }

func (f *uuidFake) run(ctx context.Context, args ...string) ([]byte, string, error) {
	if args[0] == "show-environment" {
		f.mu.Lock()
		f.calls = append(f.calls, args)
		f.mu.Unlock()
		return []byte("STAPLER_SESSION_UUID=uuid-1\n"), "", nil
	}
	return f.fakeTmux.run(ctx, args...)
}

type fakeCM struct {
	mu    sync.Mutex
	calls [][]string
	body  string
	err   error
}

func (f *fakeCM) ServerCommand(_ context.Context, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, args)
	return f.body, f.err
}

func TestControlModeRunner_should_UseControlModeAndSkipFallback_When_Available(t *testing.T) {
	t.Parallel()
	cm := &fakeCM{body: "staplersquad_a|1"}
	fallbackCalls := 0
	run := newControlModeFirstRunner([]*fakeCM{cm}, func(context.Context, ...string) ([]byte, string, error) {
		fallbackCalls++
		return nil, "", nil
	})

	out, _, err := run(context.Background(), "list-sessions", "-F", "#{session_name}|#{session_created}")

	require.NoError(t, err)
	require.Equal(t, "staplersquad_a|1", string(out))
	require.Zero(t, fallbackCalls)
	require.Equal(t, []string{"list-sessions", "-F", "'#{session_name}|#{session_created}'"}, cm.calls[0],
		"format must be single-quoted: tmux re-parses control-mode lines")
}

func TestControlModeRunner_should_FallBackToExec_When_NoClientAcceptsTheCommand(t *testing.T) {
	t.Parallel()
	cm := &fakeCM{err: tmux.ErrControlModeNotRunning}
	fallbackCalls := 0
	run := newControlModeFirstRunner([]*fakeCM{cm}, func(context.Context, ...string) ([]byte, string, error) {
		fallbackCalls++
		return []byte("x"), "", nil
	})

	out, _, err := run(context.Background(), "list-sessions")

	require.NoError(t, err)
	require.Equal(t, "x", string(out))
	require.Equal(t, 1, fallbackCalls)
}

func TestControlModeRunner_should_NotRetryOnExec_When_TmuxRejectedTheCommand(t *testing.T) {
	t.Parallel()
	cm := &fakeCM{err: errors.New("tmux: no such session: =x")}
	fallbackCalls := 0
	run := newControlModeFirstRunner([]*fakeCM{cm}, func(context.Context, ...string) ([]byte, string, error) {
		fallbackCalls++
		return nil, "", nil
	})

	_, stderr, err := run(context.Background(), "kill-session", "-t", "=x")

	require.Error(t, err)
	require.Contains(t, stderr, "no such session")
	require.Zero(t, fallbackCalls, "a command tmux already rejected must stay failed (fail closed)")
}

func TestReconcileOrphans_should_IssueOneTmuxCommand_When_AllSessionsAreKnown(t *testing.T) {
	t.Parallel()
	var instances []*instance.Instance
	var list strings.Builder
	for i := 0; i < 20; i++ {
		title := fmt.Sprintf("sess%d", i)
		instances = append(instances, liveInstance("staplersquad_"+title))
		fmt.Fprintf(&list, "staplersquad_%s|1793909015\n", title)
	}
	f := &fakeTmux{list: list.String()}

	reconcileOrphanedTmuxSessions(instances, 5*time.Minute, f.run, sweepNow)

	require.Len(t, f.calls, 1, "steady state must be O(1) tmux commands per round, not O(sessions)")
}

// accessorPM exposes a fixed *tmux.TmuxSession, like TmuxBackend does.
type accessorPM struct {
	instance.ProcessManager
	ts *tmux.TmuxSession
}

func (p accessorPM) TmuxSession() *tmux.TmuxSession   { return p.ts }
func (p accessorPM) SetTmuxSession(*tmux.TmuxSession) {}

func TestControlModeSessionsOn_should_FilterBySocketAndSkipInstancesWithoutTmuxSession(t *testing.T) {
	t.Parallel()
	onDefault := tmux.NewTmuxSessionWithServerSocket("a", "true", tmux.TmuxPrefix, "ssq-default")
	onOther := tmux.NewTmuxSessionWithServerSocket("b", "true", tmux.TmuxPrefix, "ssq-other")
	instances := []*instance.Instance{
		(&instance.Instance{}).WithProcessManagerForTest(accessorPM{ts: onDefault}),
		(&instance.Instance{}).WithProcessManagerForTest(accessorPM{ts: onOther}),
		(&instance.Instance{}).WithProcessManagerForTest(accessorPM{ts: nil}),
		liveInstance("staplersquad_noaccessor"),
	}

	require.Equal(t, []*tmux.TmuxSession{onDefault}, controlModeSessionsOn(instances, "ssq-default"))
	require.Equal(t, []*tmux.TmuxSession{onOther}, controlModeSessionsOn(instances, "ssq-other"))
}

func TestReconcileOrphans_should_NotKill_When_TargetChangedBeforeKill(t *testing.T) {
	t.Parallel()
	f := &recheckFake{fakeTmux: fakeTmux{list: "staplersquad_x|1793909015\n", envFails: true, envErr: "unknown variable: STAPLER_SESSION_UUID"}}

	reconcileOrphanedTmuxSessions(nil, 5*time.Minute, f.run, sweepNow)

	require.Empty(t, f.killTargets(), "a session recreated (new created time) between list and kill must be spared")
}

type recheckFake struct{ fakeTmux }

func (f *recheckFake) run(ctx context.Context, args ...string) ([]byte, string, error) {
	if args[0] == "list-sessions" && len(args) > 4 && args[3] == "-f" {
		return []byte("staplersquad_x|1793999999\n"), "", nil
	}
	return f.fakeTmux.run(ctx, args...)
}

func TestReconcileOrphans_should_SkipNames_When_TheyContainControlCharacters(t *testing.T) {
	t.Parallel()
	f := &fakeTmux{list: "staplersquad_a\rb|1793909015\n", envFails: true, envErr: "unknown variable: STAPLER_SESSION_UUID"}
	reconcileOrphanedTmuxSessions(nil, 0, f.run, sweepNow)
	require.Empty(t, f.killTargets())
}

func TestReconcileOrphans_should_NotKill_When_UnknownVariableTextIsForADifferentVariable(t *testing.T) {
	t.Parallel()
	f := &fakeTmux{list: "staplersquad_x|1793909015\n", envFails: true, envErr: "unknown variable: OTHER"}
	reconcileOrphanedTmuxSessions(nil, 0, f.run, sweepNow)
	require.Empty(t, f.killTargets())
}

func TestControlModeRunner_should_TryFallbackOnReadTimeout_ButNeverRetryKill(t *testing.T) {
	t.Parallel()
	wedged := &fakeCM{err: context.DeadlineExceeded}
	fallbackCalls := 0
	run := newControlModeFirstRunner([]*fakeCM{wedged}, func(context.Context, ...string) ([]byte, string, error) {
		fallbackCalls++
		return nil, "", nil
	})

	_, _, err := run(context.Background(), "list-sessions")
	require.NoError(t, err)
	require.Equal(t, 1, fallbackCalls, "a wedged client must not block a read-only command")

	_, _, err = run(context.Background(), "kill-session", "-t", "=x")
	require.Error(t, err)
	require.Equal(t, 1, fallbackCalls, "kill-session is never retried after a timeout")
}

func TestControlModeRunner_should_FallBackToExec_When_ArgumentIsUnquotable(t *testing.T) {
	t.Parallel()
	cm := &fakeCM{}
	fallbackCalls := 0
	run := newControlModeFirstRunner([]*fakeCM{cm}, func(context.Context, ...string) ([]byte, string, error) {
		fallbackCalls++
		return nil, "", nil
	})

	_, _, err := run(context.Background(), "list-sessions", "-f", "a\rb")

	require.NoError(t, err)
	require.Equal(t, 1, fallbackCalls)
	require.Empty(t, cm.calls)
}
