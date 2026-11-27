package tmux

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// The behavioural tests below only reproduce the bug on tmux 3.4; newer versions honor -x/-y.
// This argv check is what fails on every version if the resize chain is dropped.
func TestSizedNewSessionArgs_should_ChainResizeAfterProgram(t *testing.T) {
	t.Parallel()
	want := "prog ; resize-window -t s -x 220 -y 50 ; set-option -w -t s -u window-size"

	args := (&TmuxSession{sanitizedName: "s"}).newSessionArgs("/w", "prog")
	require.True(t, strings.HasSuffix(strings.Join(args, " "), want), "%q", args)

	var spawned []string
	handle := NewShellTmuxHandle("s", "", nil, MockCmdExec{RunFunc: func(cmd *exec.Cmd) error {
		spawned = cmd.Args
		return nil
	}})
	require.NoError(t, handle.Spawn("/w", "prog"))
	require.True(t, strings.HasSuffix(strings.Join(spawned, " "), want), "%q", spawned)
}

// privateTmux is a throwaway tmux server on its own socket.
type privateTmux struct {
	t           *testing.T
	bin, socket string
}

var privateTmuxSeq atomic.Int32

func newPrivateTmux(t *testing.T) privateTmux {
	t.Helper()
	bin, err := exec.LookPath(Binary())
	if err != nil {
		t.Skip("tmux not installed")
	}
	p := privateTmux{t: t, bin: bin, socket: fmt.Sprintf("test_newsize_%d_%d", os.Getpid(), privateTmuxSeq.Add(1))}
	t.Cleanup(func() { _ = p.command("kill-server").Run() })
	p.run("new-session", "-d", "-s", "base", "sleep 60")
	return p
}

func (p privateTmux) command(args ...string) *exec.Cmd {
	return safeexec.CommandContext(context.Background(), p.bin, append([]string{"-L", p.socket, "-f", "/dev/null"}, args...)...)
}

func (p privateTmux) run(args ...string) string {
	p.t.Helper()
	out, err := p.command(args...).CombinedOutput()
	require.NoError(p.t, err, "tmux %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

func (p privateTmux) exists(session string) bool {
	return p.command("has-session", "-t", "="+session).Run() == nil
}

// executor runs the real tmux commands; breakResize makes the chained resize-window fail at run time.
func (p privateTmux) executor(breakResize bool) MockCmdExec {
	run := func(cmd *exec.Cmd) error {
		if i := slices.Index(cmd.Args, "resize-window"); breakResize && i >= 0 {
			cmd.Args[slices.Index(cmd.Args[i:], "-x")+i+1] = "0"
		}
		return cmd.Run()
	}
	return MockCmdExec{
		RunFunc:            run,
		OutputFunc:         func(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() },
		CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) { return cmd.CombinedOutput() },
	}
}

func TestNewSessionArgs_should_PinSize_When_AnotherClientIsTiny(t *testing.T) {
	t.Parallel()
	server := newPrivateTmux(t)

	attach := server.command("attach-session", "-t", "base")
	attach.Env = append(os.Environ(), "TERM=xterm-256color", "TMUX=")
	tty, err := pty.StartWithSize(attach, &pty.Winsize{Cols: 10, Rows: 6})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = attach.Process.Kill()
		_ = attach.Wait()
		_ = tty.Close()
	})
	go func() { _, _ = io.Copy(io.Discard, tty) }()
	require.Eventually(t, func() bool { return server.run("list-clients", "-t", "base") != "" }, 5*time.Second, 10*time.Millisecond)

	session := &TmuxSession{sanitizedName: "sized", ExtraEnv: []string{"SIZE_PROBE=kept"}}
	server.run(session.newSessionArgs(t.TempDir(), "echo env=$SIZE_PROBE; sleep 60")...)

	require.Equal(t, "220x50", server.run("display-message", "-p", "-t", "sized", "#{window_width}x#{window_height}"))
	require.Empty(t, server.run("show-options", "-w", "-t", "sized", "window-size"), "window-size must stay inherited")
	require.Eventually(t, func() bool {
		return strings.Contains(server.run("capture-pane", "-p", "-t", "sized"), "env=kept")
	}, 5*time.Second, 10*time.Millisecond)

	// A duplicate create must fail and leave the live pane alone.
	pid := server.run("display-message", "-p", "-t", "sized", "#{pane_pid}")
	require.Error(t, server.command(session.newSessionArgs(t.TempDir(), "sleep 60")...).Run())
	require.Equal(t, pid, server.run("display-message", "-p", "-t", "sized", "#{pane_pid}"))
}

func TestRunNewSessionCommand_should_KeepTheRealProgram_When_ResizeFails(t *testing.T) {
	t.Parallel()
	server := newPrivateTmux(t)
	session := newTmuxSessionWithSocket("resize-fails", "sleep 61", NewMockPtyFactory(t), server.executor(true), TmuxPrefix, server.socket, WithRegistry(nil))
	cmd := session.buildTmuxCommand(session.newSessionArgs(t.TempDir(), "sleep 61")...)

	require.NoError(t, session.runNewSessionCommand(cmd, t.TempDir(), "sleep 61"))

	require.Contains(t, server.run("display-message", "-p", "-t", session.sanitizedName, "#{pane_start_command}"), "sleep 61")
}

func TestShellSpawn_should_LeaveNoSession_When_ResizeFails(t *testing.T) {
	t.Parallel()
	server := newPrivateTmux(t)
	handle := NewShellTmuxHandle("shell-resize-fails", server.socket, nil, server.executor(true))

	require.Error(t, handle.Spawn(t.TempDir(), "sleep 60"))

	require.False(t, server.exists("shell-resize-fails"))
	require.True(t, server.exists("base"))
}

func TestShellSpawn_should_KeepTheExistingSession_When_NameIsTaken(t *testing.T) {
	t.Parallel()
	server := newPrivateTmux(t)
	handle := NewShellTmuxHandle("base", server.socket, nil, server.executor(false))
	pid := server.run("display-message", "-p", "-t", "base", "#{pane_pid}")

	require.ErrorContains(t, handle.Spawn(t.TempDir(), "sleep 60"), "duplicate session")

	require.Equal(t, pid, server.run("display-message", "-p", "-t", "base", "#{pane_pid}"))
}

// start() kills what it half-created, but never a duplicate or a session it cannot prove it made.
func TestStart_should_KillOnlyItsOwnSession_When_NewSessionFails(t *testing.T) {
	t.Parallel()
	absent := "can't find session: staplersquad_x"
	for _, tc := range []struct {
		name, hasSession, stderr string
		wantKill                 bool
	}{
		{"duplicate", absent, "duplicate session: staplersquad_x", false},
		{"resize failed after create", absent, "width too small", true},
		{"stderr lost and existence unknown beforehand", "lost server", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var created, killed atomic.Bool
			cmdExec := MockCmdExec{
				RunFunc: func(cmd *exec.Cmd) error {
					switch {
					case slices.Contains(cmd.Args, "new-session"):
						created.Store(true)
						_, _ = cmd.Stderr.Write([]byte(tc.stderr))
						return fmt.Errorf("exit status 1")
					case slices.Contains(cmd.Args, "kill-session"):
						killed.Store(true)
					case slices.Contains(cmd.Args, "has-session") && !created.Load():
						return fmt.Errorf("no session")
					}
					return nil
				},
				CombinedOutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
					switch {
					case slices.Contains(cmd.Args, "has-session") && !created.Load():
						assert.Contains(t, cmd.Args, "=staplersquad_x", "exact match, or a prefix sibling reads as ours")
						return []byte(tc.hasSession), fmt.Errorf("exit status 1")
					case created.Load():
						return []byte("staplersquad_x"), nil
					}
					return nil, fmt.Errorf("no server running")
				},
				OutputFunc: func(*exec.Cmd) ([]byte, error) {
					if created.Load() {
						return []byte("staplersquad_x"), nil
					}
					return nil, fmt.Errorf("no server running")
				},
			}
			session := newTmuxSessionWithSocket("x", "echo", NewMockPtyFactory(t), cmdExec, TmuxPrefix, "", WithRegistry(nil))

			require.ErrorContains(t, session.Start(t.TempDir()), tc.stderr)
			require.Equal(t, tc.wantKill, killed.Load())
		})
	}
}

// Without stderr a duplicate looks like any other failure; the earlier has-session must protect it.
func TestShellSpawn_should_KeepTheExistingSession_When_StderrCannotBeCaptured(t *testing.T) {
	server := newPrivateTmux(t)
	workDir := t.TempDir()
	pid := server.run("display-message", "-p", "-t", "base", "#{pane_pid}")
	t.Setenv("TMPDIR", "/nonexistent-tmpdir") // not parallel: process-global

	require.Error(t, NewShellTmuxHandle("base", server.socket, nil, server.executor(false)).Spawn(workDir, "sleep 60"))

	require.Equal(t, pid, server.run("display-message", "-p", "-t", "base", "#{pane_pid}"))
}

func TestShellSpawn_should_KillNothing_When_ExistenceIsUnknown(t *testing.T) {
	t.Parallel()
	var killed atomic.Bool
	handle := NewShellTmuxHandle("s", "", nil, MockCmdExec{
		RunFunc: func(cmd *exec.Cmd) error {
			if slices.Contains(cmd.Args, "kill-session") {
				killed.Store(true)
			}
			return fmt.Errorf("signal: killed")
		},
		CombinedOutputFunc: func(*exec.Cmd) ([]byte, error) { return nil, fmt.Errorf("signal: killed") },
	})

	require.Error(t, handle.Spawn("/w", "prog"))
	require.False(t, killed.Load())
}

// "=" on kill-session: without it, a shell that was never created takes a prefix sibling down.
func TestShellSpawn_should_SpareAPrefixSibling_When_NothingWasCreated(t *testing.T) {
	t.Parallel()
	server := newPrivateTmux(t)
	server.run("new-session", "-d", "-s", "shell-x-longer", "sleep 60")
	exe := server.executor(false)
	run := exe.RunFunc
	exe.RunFunc = func(cmd *exec.Cmd) error {
		if slices.Contains(cmd.Args, "new-session") {
			return fmt.Errorf("exit status 1")
		}
		return run(cmd)
	}

	require.Error(t, NewShellTmuxHandle("shell-x", server.socket, nil, exe).Spawn(t.TempDir(), "sleep 60"))

	require.True(t, server.exists("shell-x-longer"))
}

// "=" on has-session: without it, a prefix sibling reads as "already there" and the shell is orphaned.
func TestShellSpawn_should_CleanUpItsOwnSession_When_APrefixSiblingExists(t *testing.T) {
	t.Parallel()
	server := newPrivateTmux(t)
	server.run("new-session", "-d", "-s", "shell-y-longer", "sleep 60")

	require.Error(t, NewShellTmuxHandle("shell-y", server.socket, nil, server.executor(true)).Spawn(t.TempDir(), "sleep 60"))

	require.False(t, server.exists("shell-y"))
	require.True(t, server.exists("shell-y-longer"))
}

func TestShellSpawn_should_AddressTheSessionItCreated_When_NameHasTargetSyntax(t *testing.T) {
	t.Parallel()
	server := newPrivateTmux(t)

	handle := NewShellTmuxHandle("v1.2:x", server.socket, nil, server.executor(false))
	require.NoError(t, handle.Spawn(t.TempDir(), "sleep 60"))
	require.Equal(t, "220x50", server.run("display-message", "-p", "-t", "v1_2_x", "#{window_width}x#{window_height}"))
	require.True(t, handle.DoesSessionExist())

	require.Error(t, NewShellTmuxHandle("v3.4", server.socket, nil, server.executor(true)).Spawn(t.TempDir(), "sleep 60"))
	require.False(t, server.exists("v3_4"), "a failed spawn must not orphan the shell")
}

func TestToStaplerSquadTmuxName_should_ReplaceTargetSyntaxInThePrefix(t *testing.T) {
	t.Parallel()
	require.Equal(t, "v1_2_x_title", toStaplerSquadTmuxNameWithPrefix("title", "v1.2:x_"))
}

type resettableExec struct {
	MockCmdExec
	resets *atomic.Int32
}

func (r resettableExec) Reset() { r.resets.Add(1) }

// The tolerated recreate path keeps what a clean create does, and says why tmux complained.
func TestRunNewSessionCommand_should_FinishSetupAndLogStderr_When_ResizeFails(t *testing.T) {
	server := newPrivateTmux(t)
	var logs log.SyncBuffer
	previous := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&logs, nil))) // not parallel: process-global
	t.Cleanup(func() { log.SetSlogDefaultForTest(previous) })
	exe := resettableExec{MockCmdExec: server.executor(true), resets: &atomic.Int32{}}
	session := newTmuxSessionWithSocket("recreate", "sleep 61", NewMockPtyFactory(t), exe, TmuxPrefix, server.socket, WithRegistry(nil))
	cmd := session.buildTmuxCommand(session.newSessionArgs(t.TempDir(), "sleep 61")...)

	require.NoError(t, session.runNewSessionCommand(cmd, t.TempDir(), "sleep 61"))

	require.Equal(t, int32(1), exe.resets.Load())
	require.Equal(t, "remain-on-exit on", server.run("show-options", "-w", "-t", session.sanitizedName, "remain-on-exit"))
	require.Contains(t, logs.String(), "width too small")
}
