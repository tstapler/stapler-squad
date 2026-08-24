package tmux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestLocalRunner_Run_Success verifies Run returns the combined stdout of a
// successful command with a nil error.
func TestLocalRunner_Run_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := (LocalRunner{}).Run(ctx, "", "echo", "-n", "hello")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if string(out) != "hello" {
		t.Errorf("Run output = %q, want %q", out, "hello")
	}
}

// TestLocalRunner_Run_Failure verifies Run surfaces a non-nil error for a
// command that exits non-zero, matching safeexec.CommandContext(...).CombinedOutput().
func TestLocalRunner_Run_Failure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := (LocalRunner{}).Run(ctx, "", "false")
	if err == nil {
		t.Fatal("Run against a failing command returned nil error")
	}
}

// TestLocalRunner_Run_MatchesSafeexecCombinedOutput is the permanent parity
// regression guard required by Phase 1's characterization tests: LocalRunner.Run
// must return byte-identical output to the pre-refactor
// safeexec.CommandContext(...).CombinedOutput() call it replaces at every
// migrated call site in tmux.go/worktree_git.go.
func TestLocalRunner_Run_MatchesSafeexecCombinedOutput(t *testing.T) {
	if _, err := safeexec.CommandContext(context.Background(), "tmux", "-V").CombinedOutput(); err != nil {
		t.Skip("tmux not available, skipping parity test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	want, wantErr := safeexec.CommandContext(ctx, "tmux", "-V").CombinedOutput()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	got, gotErr := (LocalRunner{}).Run(ctx2, "", "tmux", "-V")

	if (wantErr == nil) != (gotErr == nil) {
		t.Fatalf("error presence mismatch: safeexec err=%v, LocalRunner err=%v", wantErr, gotErr)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("LocalRunner.Run output = %q, want byte-identical to safeexec CombinedOutput %q", got, want)
	}
}

// TestLocalRunner_Start_RoundTripsBytesBeforeWait verifies Start returns a live
// stdin/stdout pipe pair backed by a real subprocess: bytes written to stdin
// are readable from stdout before wait() is called.
func TestLocalRunner_Start_RoundTripsBytesBeforeWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdin, stdout, wait, err := (LocalRunner{}).Start(ctx, "", "cat")
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	const payload = "round-trip-test\n"
	if _, err := stdin.Write([]byte(payload)); err != nil {
		t.Fatalf("failed to write to stdin: %v", err)
	}

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(stdout, buf); err != nil {
		t.Fatalf("failed to read from stdout: %v", err)
	}
	if string(buf) != payload {
		t.Errorf("stdout = %q, want %q", buf, payload)
	}

	if err := stdin.Close(); err != nil {
		t.Fatalf("failed to close stdin: %v", err)
	}
	if err := wait(); err != nil {
		t.Fatalf("wait() returned error: %v", err)
	}
}

// TestLocalRunner_Start_FailsForMissingBinary verifies Start surfaces an error
// (rather than panicking) when the named binary does not exist.
func TestLocalRunner_Start_FailsForMissingBinary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdin, stdout, wait, err := (LocalRunner{}).Start(ctx, "", "stapler-squad-nonexistent-binary-xyz")
	if err == nil {
		t.Fatal("Start against a nonexistent binary returned nil error")
	}
	if stdin != nil || stdout != nil || wait != nil {
		t.Error("Start returned non-nil pipes/wait alongside a non-nil error")
	}
}

// TestLocalRunner_Run_HonorsDir verifies Run executes the command with its
// working directory set to dir, matching (*exec.Cmd).Dir semantics —
// required for session/git's worktree-scoped git/gh invocations.
func TestLocalRunner_Run_HonorsDir(t *testing.T) {
	dir := t.TempDir()
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for %q: %v", dir, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := (LocalRunner{}).Run(ctx, dir, "pwd")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	assertPwdMatchesDir(t, string(out), resolvedDir)
}

// TestLocalRunner_Start_HonorsDir verifies Start executes the command with
// its working directory set to dir, matching (*exec.Cmd).Dir semantics.
func TestLocalRunner_Start_HonorsDir(t *testing.T) {
	dir := t.TempDir()
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for %q: %v", dir, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdin, stdout, wait, err := (LocalRunner{}).Start(ctx, dir, "pwd")
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	_ = stdin.Close()

	out, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}
	if err := wait(); err != nil {
		t.Fatalf("wait() returned error: %v", err)
	}
	assertPwdMatchesDir(t, string(out), resolvedDir)
}

// assertPwdMatchesDir compares pwd's output against resolvedDir (an
// already-EvalSymlinks'd path) after also resolving pwdOutput. On macOS,
// /bin/pwd defaults to POSIX logical mode: once it confirms $PWD names the
// same directory as the process's actual cwd, it echoes $PWD verbatim
// (e.g. "/var/folders/...") rather than the physically-resolved path
// ("/private/var/folders/..."), even though the command genuinely ran in
// the requested directory. Resolving both sides verifies the directory was
// honored without depending on that platform-specific string form.
func assertPwdMatchesDir(t *testing.T, pwdOutput, resolvedDir string) {
	t.Helper()
	got := strings.TrimSpace(pwdOutput)
	resolvedGot, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("failed to resolve symlinks for pwd output %q: %v", got, err)
	}
	if resolvedGot != resolvedDir {
		t.Errorf("pwd output = %q (resolved %q), want %q (dir was not honored)", got, resolvedGot, resolvedDir)
	}
}

// TestLocalRunner_IsRemote verifies LocalRunner always reports local execution.
func TestLocalRunner_IsRemote(t *testing.T) {
	if (LocalRunner{}).IsRemote() {
		t.Error("LocalRunner.IsRemote() = true, want false")
	}
}

// spyRunCall records one CommandRunner.Run invocation.
type spyRunCall struct {
	dir  string
	name string
	args []string
}

// spyCommandRunner is a test CommandRunner spy: it records every Run call
// and returns a scripted response. Used to prove a CommandRunner injected
// via WithCommandRunner is actually consulted by TmuxSession's methods, not
// merely stored on the struct.
type spyCommandRunner struct {
	runCalls []spyRunCall
	runOut   []byte
	runErr   error
}

func (s *spyCommandRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	s.runCalls = append(s.runCalls, spyRunCall{dir: dir, name: name, args: append([]string(nil), args...)})
	return s.runOut, s.runErr
}

func (s *spyCommandRunner) Start(context.Context, string, string, ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	return nil, nil, nil, fmt.Errorf("spyCommandRunner.Start not implemented")
}

func (s *spyCommandRunner) IsRemote() bool { return false }

// TestWithCommandRunner_InjectedRunnerIsActuallyUsed is VIOLATION 1's required
// regression guard: constructs a TmuxSession via NewTmuxSessionWithDeps with
// WithCommandRunner(spy), forces RefreshClient down its SIGWINCH-via-kill
// fallback path (session/tmux/tmux.go), and asserts the spy -- not
// LocalRunner -- actually received the "kill -WINCH <pid>" call. Proves
// injection is wired all the way through to a real call site, not just
// stored on the struct.
func TestWithCommandRunner_InjectedRunnerIsActuallyUsed(t *testing.T) {
	spy := &spyCommandRunner{}
	fakeCmdExec := MockCmdExec{
		// refresh-client fails, forcing RefreshClient's fallback path.
		RunFunc: func(*exec.Cmd) error {
			return fmt.Errorf("refresh-client unavailable")
		},
		// display-message succeeds, reporting a fake pane PID.
		OutputFunc: func(*exec.Cmd) ([]byte, error) {
			return []byte("54321\n"), nil
		},
	}

	sess := NewTmuxSessionWithDeps("spy-test-session", ProgramClaude, nil, fakeCmdExec, WithCommandRunner(spy))

	if err := sess.RefreshClient(); err != nil {
		t.Fatalf("RefreshClient returned error: %v", err)
	}

	if len(spy.runCalls) != 1 {
		t.Fatalf("spy.Run call count = %d, want 1 (RefreshClient's SIGWINCH fallback should route through the injected CommandRunner)", len(spy.runCalls))
	}
	call := spy.runCalls[0]
	if call.name != "kill" || len(call.args) != 2 || call.args[0] != "-WINCH" || call.args[1] != "54321" {
		t.Errorf("spy.Run call = %+v, want {name: \"kill\", args: [\"-WINCH\", \"54321\"]}", call)
	}
}
