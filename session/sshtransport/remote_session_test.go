package sshtransport

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// remoteSessionCount lists sessions on the isolated tmux socket over the
// given runner (or, if runner is nil, via a direct local exec -- used as a
// verification path independent of whichever SSHRunner/pool a test is
// exercising) and returns how many match name exactly.
func remoteSessionCount(t *testing.T, socket, name string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := safeexec.CommandContext(ctx, tmux.Binary(), "-L", socket, "list-sessions", "-F", "#{session_name}").CombinedOutput()
	if err != nil {
		// "no server running" / no sessions yet -- zero matches, not a test failure.
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == name {
			count++
		}
	}
	return count
}

// newRemoteTestTmuxSession builds a TmuxSession backed by runner and an
// isolated tmux -L socket (so this test can never collide with the real
// dev/CI tmux server), returning the session plus the socket name for
// cleanup and out-of-band verification.
func newRemoteTestTmuxSession(t *testing.T, sessionName string, runner tmux.CommandRunner) (*tmux.TmuxSession, string) {
	t.Helper()
	socket := fmt.Sprintf("test_remote_%s_%d_%d", sessionName, os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = safeexec.CommandContext(ctx, tmux.Binary(), "-L", socket, "kill-server").Run()
	})
	sess := tmux.NewTmuxSessionWithServerSocket(sessionName, "sleep 60", tmux.TmuxPrefix, socket, tmux.WithCommandRunner(runner), tmux.WithRegistry(nil))
	return sess, socket
}

// TestEnsureRemoteSession_CreatesWhenAbsent_ThenReusesWhenPresent exercises
// the two branches of Story 2.3.2's logic against a real in-process test
// sshd and a real tmux server: the first call finds no existing session and
// creates one; the second call finds it via has-session and reuses it
// without creating a duplicate. Either way, exactly one matching session
// exists on the remote tmux socket afterward.
func TestEnsureRemoteSession_CreatesWhenAbsent_ThenReusesWhenPresent(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	srv := startRealExecTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	runner := newTestSSHRunner(t, "ensure-remote-basic", srv.Addr, cfg)

	sess, socket := newRemoteTestTmuxSession(t, "ensure-remote-basic-session", runner)
	workDir := t.TempDir()
	ctx := context.Background()

	if err := sess.EnsureRemoteSession(ctx, workDir); err != nil {
		t.Fatalf("first EnsureRemoteSession() error = %v", err)
	}
	if got := remoteSessionCount(t, socket, sess.GetSanitizedName()); got != 1 {
		t.Fatalf("after first EnsureRemoteSession(): got %d matching sessions, want 1", got)
	}

	if err := sess.EnsureRemoteSession(ctx, workDir); err != nil {
		t.Fatalf("second (reuse) EnsureRemoteSession() error = %v", err)
	}
	if got := remoteSessionCount(t, socket, sess.GetSanitizedName()); got != 1 {
		t.Fatalf("after second EnsureRemoteSession(): got %d matching sessions, want 1 (duplicate created)", got)
	}
}

// TestEnsureRemoteSession_RetryAfterConnectionDrop_DoesNotDuplicate is the
// Task 2.3.2b integration test: kill the underlying *ssh.Client mid-create,
// retry, and assert exactly one remote tmux session exists afterward --
// proving research/pitfalls.md §1's failure mode (an SSH channel drop
// misread by the caller as "the remote command failed") does not produce a
// duplicate remote tmux session once EnsureRemoteSession's has-session/
// new-session -A logic is in the retry path.
func TestEnsureRemoteSession_RetryAfterConnectionDrop_DoesNotDuplicate(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	srv := startRealExecTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	pool := NewSSHClientPool()
	target := SSHTarget{Name: "ensure-remote-retry", Addr: srv.Addr}
	runner := newTestSSHRunner(t, target.Name, target.Addr, cfg, WithSSHClientPool(pool))

	sess, socket := newRemoteTestTmuxSession(t, "ensure-remote-retry-session", runner)
	workDir := t.TempDir()
	ctx := context.Background()

	// First attempt: start EnsureRemoteSession in the background, then wait
	// until its "new-session" call has actually created the session on the
	// remote tmux server before severing the connection -- this
	// deterministically reproduces pitfalls.md §1's exact failure mode ("a
	// remote command succeeds, but the SSH channel/connection dies before
	// the caller can observe that success and is left treating it as a
	// failure") instead of depending on exactly which sub-step of
	// EnsureRemoteSession a fixed sleep happens to land in.
	firstErrCh := make(chan error, 1)
	go func() {
		firstErrCh <- sess.EnsureRemoteSession(ctx, workDir)
	}()
	// 20s, not 5s: this spins up a real sshd plus a real tmux subprocess, and
	// under -race with the full session/tmux package running concurrently
	// (as in `go test ./session/tmux/... -race`) that setup can take longer
	// than 5s purely from scheduler/CPU contention, not from any bug --
	// confirmed by this test passing 3/3 in isolation (`-run` this test
	// alone) while intermittently missing a 5s deadline under full-package
	// -race load.
	deadline := time.Now().Add(20 * time.Second)
	sessionTick := time.NewTicker(2 * time.Millisecond)
	defer sessionTick.Stop()
	for remoteSessionCount(t, socket, sess.GetSanitizedName()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the first EnsureRemoteSession attempt to create the remote session")
		}
		<-sessionTick.C
	}
	if client, ok := pool.Peek(target.Name); ok {
		if err := client.Close(); err != nil {
			t.Logf("closing pooled ssh client: %v", err)
		}
	}
	<-firstErrCh // don't care whether this attempt reported success or failure -- the point is the remote session already exists despite it

	// Retry: the pool redials (Peek found nothing, or a dead entry gets
	// evicted by its Client.Wait() watcher shortly after Close()), then
	// EnsureRemoteSession's has-session check must find the session the
	// first attempt may have already created remotely, and reuse it via
	// new-session -A instead of creating a second one.
	if err := waitForEnsureRemoteSessionRetry(ctx, sess, workDir); err != nil {
		t.Fatalf("retry EnsureRemoteSession() error = %v", err)
	}

	if got := remoteSessionCount(t, socket, sess.GetSanitizedName()); got != 1 {
		t.Fatalf("after kill-mid-create + retry: got %d matching remote tmux sessions, want exactly 1", got)
	}
}

// TestNewSessionA_AgainstExistingSession_FailsOverNonPTYChannel locks in,
// as an executable regression test, the empirical claim EnsureRemoteSession's
// doc comment relies on: SSHRunner.Run never requests a PTY (no RequestPty
// call anywhere in ssh_runner.go), so "tmux new-session -A -d" against an
// already-existing session fails instead of silently attaching when run
// over it. If a future SSHRunner change ever starts requesting a PTY for
// Run, this test's expectation of a non-nil error would need to flip too --
// which is exactly the signal that EnsureRemoteSession's recheck-on-failure
// logic (not "-A" alone) is what closes the race, and needs re-evaluating
// alongside it.
func TestNewSessionA_AgainstExistingSession_FailsOverNonPTYChannel(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	srv := startRealExecTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	runner := newTestSSHRunner(t, "new-session-a-non-pty", srv.Addr, cfg)

	sess, socket := newRemoteTestTmuxSession(t, "new-session-a-non-pty-session", runner)
	workDir := t.TempDir()
	ctx := context.Background()

	// Create the session for real first.
	if err := sess.EnsureRemoteSession(ctx, workDir); err != nil {
		t.Fatalf("initial EnsureRemoteSession() error = %v", err)
	}
	if got := remoteSessionCount(t, socket, sess.GetSanitizedName()); got != 1 {
		t.Fatalf("after initial EnsureRemoteSession(): got %d matching sessions, want 1", got)
	}

	// Run "new-session -A -d" against it directly (bypassing the
	// has-session pre-check EnsureRemoteSession normally does), mirroring
	// exactly what createRemoteSession sends.
	newArgs := []string{"new-session", "-A", "-d", "-s", sess.GetSanitizedName(), "-c", workDir, "sleep 60"}
	newArgs = tmux.Socket(socket).Args(newArgs...)
	newName, newArgs := tmux.WrapRemoteCommand(tmux.Binary(), newArgs)
	out, err := runner.Run(ctx, "", newName, newArgs...)
	if err == nil {
		t.Fatalf("new-session -A against an already-existing session succeeded over a non-PTY channel (out: %s) -- expected it to fail with something like \"open terminal failed: not a terminal\"; if SSHRunner now requests a PTY, EnsureRemoteSession's doc comment and recovery logic need updating alongside this test", out)
	}
	t.Logf("new-session -A against existing session failed as expected: %v (output: %s)", err, out)
}

// concurrentCreatorRunner creates the session out-of-band right after the first
// has-session reports it absent, so a concurrent creator always wins the race.
type concurrentCreatorRunner struct {
	tmux.CommandRunner
	create    []string
	createErr error
	hasChecks int
}

func (r *concurrentCreatorRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	out, err := r.CommandRunner.Run(ctx, dir, name, args...)
	if slices.Contains(args, "has-session") {
		r.hasChecks++
		if r.hasChecks == 1 {
			createName, createArgs := tmux.WrapRemoteCommand(tmux.Binary(), r.create)
			_, r.createErr = r.CommandRunner.Run(ctx, "", createName, createArgs...)
		}
	}
	return out, err
}

// TestEnsureRemoteSession_RaceWindow_ConcurrentCreatorWins_RecoversViaHasSessionRecheck
// is the narrower regression test the reviewer asked for: has-session
// reports absent, a concurrent creator wins before this call's own
// new-session -A runs, that new-session -A fails (per
// TestNewSessionA_AgainstExistingSession_FailsOverNonPTYChannel), and
// the recheck must find the session and return nil rather than surfacing
// the error.
func TestEnsureRemoteSession_RaceWindow_ConcurrentCreatorWins_RecoversViaHasSessionRecheck(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping real tmux test")
	}

	srv := startRealExecTestSSHServer(t)
	cfg := newTestClientConfig(t, srv.HostKey)
	runner := &concurrentCreatorRunner{CommandRunner: newTestSSHRunner(t, "race-window-recheck", srv.Addr, cfg)}

	sess, socket := newRemoteTestTmuxSession(t, "race-window-recheck-session", runner)
	workDir := t.TempDir()
	runner.create = tmux.Socket(socket).Args("new-session", "-d", "-s", sess.GetSanitizedName(), "-c", workDir, "sleep 60")

	if err := sess.EnsureRemoteSession(context.Background(), workDir); err != nil {
		t.Fatalf("EnsureRemoteSession() error = %v, want nil (recheck should have found the session created by the simulated concurrent creator)", err)
	}
	if runner.createErr != nil {
		t.Fatalf("out-of-band new-session -d (simulating the concurrent creator) failed: %v", runner.createErr)
	}
	if runner.hasChecks != 2 {
		t.Fatalf("has-session ran %d times, want 2 (pre-check, then the recheck after new-session -A failed)", runner.hasChecks)
	}
	if got := remoteSessionCount(t, socket, sess.GetSanitizedName()); got != 1 {
		t.Fatalf("after race-window recovery: got %d matching remote tmux sessions, want exactly 1 (no duplicate)", got)
	}
}

// waitForEnsureRemoteSessionRetry retries EnsureRemoteSession a few times
// with a short backoff. The pool's dead-connection eviction
// (SSHClientPool.register's Client.Wait() watcher) runs in its own
// goroutine and is not synchronized with the test's Close() call above, so
// the very next EnsureRemoteSession call can occasionally still observe the
// stale, now-dead pooled client via Peek before the watcher evicts it (a
// narrow window, not a design flaw in EnsureRemoteSession itself -- the
// eventual retry always succeeds once the watcher catches up).
func waitForEnsureRemoteSessionRetry(ctx context.Context, sess *tmux.TmuxSession, workDir string) error {
	var lastErr error
	retryTick := time.NewTicker(50 * time.Millisecond)
	defer retryTick.Stop()
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			<-retryTick.C
		}
		if err := sess.EnsureRemoteSession(ctx, workDir); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}
