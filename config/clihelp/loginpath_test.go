//go:build !windows

package clihelp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

// writeStubShell writes an executable /bin/sh script named name that ignores
// its arguments and runs body.
func writeStubShell(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return path
}

func sentinelPrint(path string) string {
	return fmt.Sprintf("printf '%s%%s%s' '%s'", pathStartMarker, pathEndMarker, path)
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestSource(shell string, run shellRunner, clock *fakeClock) *loginPathSource {
	return newLoginPathSource(
		func() string { return shell },
		func() string { return "/srv/bin:/usr/bin" },
		func() string { return "/home/t" },
		run, clock.Now)
}

func TestDeriveLoginPath_should_PutShellDirsFirstAndDedupe_When_StubZshEchoesPath(t *testing.T) {
	binDir := t.TempDir()
	shell := writeStubShell(t, "zsh", sentinelPrint(binDir+":/usr/bin"))
	src := newTestSource(shell, runShellScript, &fakeClock{t: time.Unix(0, 0)})

	src.Refresh(context.Background())

	assert.Equal(t, []string{binDir, "/usr/bin", "/srv/bin", "/home/t/.local/bin", "/usr/local/bin"}, src.Dirs())
}

func TestDeriveLoginPath_should_NotCacheFailureAndRetryAfter30s_When_ShellTimesOut(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	var calls atomic.Int32
	run := func(context.Context, string, string) (string, error) {
		if calls.Add(1) == 1 {
			return "", context.DeadlineExceeded
		}
		return "x" + pathStartMarker + "/opt/shell/bin" + pathEndMarker, nil
	}
	src := newTestSource("/bin/zsh", run, clock)

	src.Refresh(context.Background())
	assert.NotContains(t, src.Dirs(), "/opt/shell/bin", "failure serves server PATH plus fallbacks")
	assert.Contains(t, src.Dirs(), "/srv/bin")

	clock.Advance(29 * time.Second)
	src.Refresh(context.Background())
	assert.EqualValues(t, 1, calls.Load(), "retry window not yet elapsed")

	clock.Advance(time.Second)
	src.Refresh(context.Background())
	assert.EqualValues(t, 2, calls.Load())
	assert.Equal(t, "/opt/shell/bin", src.Dirs()[0])

	src.Refresh(context.Background())
	assert.EqualValues(t, 2, calls.Load(), "success is cached")
	clock.Advance(loginPathTTL)
	src.Refresh(context.Background())
	assert.EqualValues(t, 3, calls.Load(), "success expires after the TTL")
}

func TestDeriveLoginPath_should_UseServerPathPlusFallbackDirs_When_ShellUnsetOrBashReturnsEarly(t *testing.T) {
	want := []string{"/srv/bin", "/usr/bin", "/home/t/.local/bin", "/usr/local/bin"}
	t.Run("shell unset runs nothing", func(t *testing.T) {
		var calls atomic.Int32
		run := func(context.Context, string, string) (string, error) { calls.Add(1); return "", nil }
		src := newTestSource("", run, &fakeClock{t: time.Unix(0, 0)})
		src.Refresh(context.Background())
		assert.Equal(t, want, src.Dirs())
		assert.Zero(t, calls.Load())
	})
	t.Run("plain sh runs nothing", func(t *testing.T) {
		var calls atomic.Int32
		run := func(context.Context, string, string) (string, error) { calls.Add(1); return "", nil }
		src := newTestSource("/bin/sh", run, &fakeClock{t: time.Unix(0, 0)})
		src.Refresh(context.Background())
		assert.Zero(t, calls.Load())
	})
	t.Run("bash rc returning early yields server PATH", func(t *testing.T) {
		shell := writeStubShell(t, "bash", sentinelPrint("/srv/bin:/usr/bin"))
		src := newTestSource(shell, runShellScript, &fakeClock{t: time.Unix(0, 0)})
		src.Refresh(context.Background())
		assert.Equal(t, want, src.Dirs())
	})
}

func TestLoginPathScript_should_SourceRcOnlyForZshAndBash_When_ShellVaries(t *testing.T) {
	zsh, ok := loginPathScript("/usr/bin/zsh")
	assert.True(t, ok)
	assert.True(t, strings.HasPrefix(zsh, "source ~/.zshrc &>/dev/null || true; printf '"+pathStartMarker))
	bash, _ := loginPathScript("/bin/bash")
	assert.Contains(t, bash, "source ~/.bashrc")
	other, ok := loginPathScript("/usr/bin/fish")
	assert.True(t, ok)
	assert.NotContains(t, other, "source")
	_, ok = loginPathScript("")
	assert.False(t, ok)
	_, ok = loginPathScript("/bin/sh")
	assert.False(t, ok)
}

func TestDeriveLoginPath_should_ReturnWithinTimeoutAndKillGroup_When_ShellBackgroundsSleep(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "sleep.pid")
	shell := writeStubShell(t, "zsh", "sleep 60 &\necho $! > "+pidFile+"\n"+sentinelPrint("/opt/x/bin"))
	src := newTestSource(shell, runShellScript, &fakeClock{t: time.Unix(0, 0)})

	start := time.Now()
	src.Refresh(context.Background())
	assert.Less(t, time.Since(start), wait.ScaleTimeout(loginPathTimeout+time.Second))
	assert.Equal(t, "/opt/x/bin", src.Dirs()[0], "output before the pipe-holding helper is still used")

	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	wait.RequireEventually(t, func() bool { return processGone(pid) }, 2*time.Second, 20*time.Millisecond, "background sleep must be killed")
}

func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
		return true
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	return err == nil && strings.Contains(string(stat), ") Z ")
}

func TestDeriveLoginPath_should_UseOnlySentinelSpan_When_RcPrintsBannerToStdout(t *testing.T) {
	body := "echo 'p10k: instant prompt /fake/bin:/evil'\necho '/banner/dir'\n" + sentinelPrint("/opt/x/bin")
	shell := writeStubShell(t, "zsh", body)
	src := newTestSource(shell, runShellScript, &fakeClock{t: time.Unix(0, 0)})
	src.Refresh(context.Background())
	dirs := src.Dirs()
	assert.Equal(t, "/opt/x/bin", dirs[0])
	assert.NotContains(t, dirs, "/banner/dir")
	assert.NotContains(t, dirs, "/evil")
}

func TestDeriveLoginPath_should_TreatMissingMarkersAsFailureAndNotCache_When_StubPrintsNoSentinels(t *testing.T) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	shell := writeStubShell(t, "zsh", "echo /opt/x/bin")
	var calls atomic.Int32
	src := newTestSource(shell, func(ctx context.Context, sh, script string) (string, error) {
		calls.Add(1)
		return runShellScript(ctx, sh, script)
	}, clock)

	src.Refresh(context.Background())
	assert.NotContains(t, src.Dirs(), "/opt/x/bin")
	clock.Advance(loginPathRetry)
	src.Refresh(context.Background())
	assert.EqualValues(t, 2, calls.Load(), "failure was not cached")
}

func TestDeriveLoginPath_should_ReturnError_When_RunnerFails(t *testing.T) {
	_, err := deriveLoginPath(context.Background(), "/bin/zsh", func(context.Context, string, string) (string, error) {
		return "", errors.New("boom")
	})
	assert.Error(t, err)
	_, err = deriveLoginPath(context.Background(), "", nil)
	assert.ErrorIs(t, err, errNoLoginShell)
}

func TestNewProber_should_SpawnNoGoroutine_When_ConstructedAndOnlyStartDerivesLoginPath(t *testing.T) {
	binDir := t.TempDir()
	shell := writeStubShell(t, "zsh", sentinelPrint(binDir))
	p := NewProber(WithShell(func() string { return shell }), WithHome("/home/t"))
	goleak.VerifyNone(t)
	assert.NotContains(t, p.loginPath(), binDir, "constructor derives nothing")

	p.StartLoginPathDerivation()
	wait.RequireEventually(t, func() bool { return p.loginPath()[0] == binDir }, 5*time.Second, 20*time.Millisecond)
	assert.True(t, strings.HasPrefix(p.LoginPathEnv(), binDir+":"))
	wait.RequireEventually(t, func() bool {
		return goleak.Find() == nil
	}, 5*time.Second, 20*time.Millisecond, "derivation goroutine exits")
}
