package services

// hook_injector_remote_test.go unit-tests InjectHookConfigRemote
// (ssh-remote-workspaces Phase 5 correction, Part B) against a fake
// tmux.CommandRunner -- no real SSH connection needed, since
// InjectHookConfigRemote's only interaction with "the remote host" is
// through the CommandRunner seam (session/tmux.CommandRunner), exactly the
// abstraction ADR-002 introduced for this purpose. The real SSH-backed
// proof that the generated hook command actually reaches
// RemoteApprovalRelay correctly is TestApprovalHandler_
// SatisfiesRelayHandlerEndToEnd (approval_handler_remote_relay_test.go).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// fakeRemoteCommandRunner is a minimal tmux.CommandRunner test double.
// Run/Start are keyed by the exact "name arg1 arg2..." string so a test can
// script per-command responses without needing a real shell.
type fakeRemoteCommandRunner struct {
	runOutput map[string][]byte
	runErr    error

	startErr    error
	waitErr     error
	startCalls  []string
	writtenData []byte
	waitCalls   int

	// writeErr/closeErr, when set, make the io.WriteCloser returned by Start
	// fail on Write/Close respectively -- used by
	// TestInjectHookConfigRemote_CallsWait_EvenWhenStdinWriteOrCloseFails to
	// prove wait() still runs (and releases the connection pool reference)
	// on that early-return path.
	writeErr error
	closeErr error
}

func (f *fakeRemoteCommandRunner) commandKey(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}

func (f *fakeRemoteCommandRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	if f.runErr != nil {
		return nil, f.runErr
	}
	return f.runOutput[f.commandKey(name, args...)], nil
}

func (f *fakeRemoteCommandRunner) Start(_ context.Context, _ string, name string, args ...string) (io.WriteCloser, io.ReadCloser, func() error, error) {
	if f.startErr != nil {
		return nil, nil, nil, f.startErr
	}
	f.startCalls = append(f.startCalls, f.commandKey(name, args...))
	return &captureWriteCloser{f: f}, nil, func() error {
		f.waitCalls++
		return f.waitErr
	}, nil
}

func (f *fakeRemoteCommandRunner) IsRemote() bool { return true }

// captureWriteCloser records everything written to it into the owning
// fakeRemoteCommandRunner's writtenData, standing in for the piped stdin
// InjectHookConfigRemote writes the merged settings JSON to. If writeErr/
// closeErr is set on the owning fake, Write/Close fail instead -- see
// TestInjectHookConfigRemote_CallsWait_EvenWhenStdinWriteOrCloseFails.
type captureWriteCloser struct {
	f *fakeRemoteCommandRunner
}

func (c *captureWriteCloser) Write(p []byte) (int, error) {
	if c.f.writeErr != nil {
		return 0, c.f.writeErr
	}
	c.f.writtenData = append(c.f.writtenData, p...)
	return len(p), nil
}

func (c *captureWriteCloser) Close() error { return c.f.closeErr }

func TestInjectHookConfigRemote_WritesFreshSettings_WhenNoneExist(t *testing.T) {
	runner := &fakeRemoteCommandRunner{runOutput: map[string][]byte{}} // "if [ -e ... ]" is false, script exits 0 with empty output (file doesn't exist)
	target := RemoteHookTarget{SocketPath: "/home/agent/work/.stapler-squad-approval.sock", BearerToken: "test-token-123"}

	if err := InjectHookConfigRemote(context.Background(), runner, "/home/agent/work", "session-1", target); err != nil {
		t.Fatalf("InjectHookConfigRemote() error: %v", err)
	}

	if len(runner.startCalls) != 1 {
		t.Fatalf("Start() called %d times, want 1", len(runner.startCalls))
	}
	if !strings.Contains(runner.startCalls[0], "mkdir -p") || !strings.Contains(runner.startCalls[0], "cat >") {
		t.Errorf("Start() command = %q, want it to mkdir -p the .claude dir and cat > the settings file", runner.startCalls[0])
	}
	if !strings.Contains(runner.startCalls[0], "/home/agent/work/.claude") {
		t.Errorf("Start() command = %q, want it to reference the remote .claude dir under rootDir", runner.startCalls[0])
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(runner.writtenData, &settings); err != nil {
		t.Fatalf("written settings did not parse as JSON: %v (data: %s)", err, runner.writtenData)
	}
	var hooksMap map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooksMap); err != nil {
		t.Fatalf("parse hooks map: %v", err)
	}
	if !bytes.Contains(hooksMap["PermissionRequest"], []byte("UNIX-CONNECT:"+target.SocketPath)) {
		t.Errorf("PermissionRequest hooks = %s, want a command targeting %s via UNIX-CONNECT", hooksMap["PermissionRequest"], target.SocketPath)
	}
	if !bytes.Contains(hooksMap["PermissionRequest"], []byte(target.BearerToken)) {
		t.Errorf("PermissionRequest hooks = %s, want the bearer token %q embedded", hooksMap["PermissionRequest"], target.BearerToken)
	}
	// The remote-aware command must NOT be a curl/HTTP command -- confirms
	// this test actually exercised the socat-based remoteApprovalHookCommand
	// path, not accidentally the local hookApprovalURL()-based one.
	if bytes.Contains(hooksMap["PermissionRequest"], []byte("curl")) {
		t.Errorf("PermissionRequest hooks = %s, want no curl command (remote sessions route via socat, not HTTP)", hooksMap["PermissionRequest"])
	}
}

func TestInjectHookConfigRemote_NoOpWhenAlreadyPresent(t *testing.T) {
	target := RemoteHookTarget{SocketPath: "/home/agent/work/.stapler-squad-approval.sock", BearerToken: "test-token-123"}
	existingCmd := remoteApprovalHookCommand(target)
	existing := `{"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":` + jsonQuote(existingCmd) + `,"timeout":300}]}]}}`

	runner := &fakeRemoteCommandRunner{
		runOutput: map[string][]byte{
			"sh -c if [ -e '/home/agent/work/.claude/settings.local.json' ]; then cat '/home/agent/work/.claude/settings.local.json'; fi": []byte(existing),
		},
	}

	if err := InjectHookConfigRemote(context.Background(), runner, "/home/agent/work", "session-1", target); err != nil {
		t.Fatalf("InjectHookConfigRemote() error: %v", err)
	}
	if len(runner.startCalls) != 0 {
		t.Errorf("Start() called %d times, want 0 (hook already present, nothing should be written)", len(runner.startCalls))
	}
}

func TestInjectHookConfigRemote_PropagatesReadError(t *testing.T) {
	runner := &fakeRemoteCommandRunner{runErr: errors.New("boom: ssh channel closed")}
	target := RemoteHookTarget{SocketPath: "/x/.sock", BearerToken: "t"}

	err := InjectHookConfigRemote(context.Background(), runner, "/home/agent/work", "session-1", target)
	if err == nil {
		t.Fatal("InjectHookConfigRemote() error = nil, want the read failure to propagate")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want it to wrap the underlying read failure", err)
	}
}

// TestInjectHookConfigRemote_ReadScript_DistinguishesMissingFileFromUnreadableFile is a
// regression guard for a review-caught asymmetry with the local InjectHookConfig: an earlier
// version of the read step (`cat path 2>/dev/null || true`) forced exit 0 unconditionally,
// so a permission-denied read (file exists but can't be read) was silently treated the same
// as "file doesn't exist" -- and the subsequent write step would then clobber a file it
// couldn't actually see the contents of. os.ReadFile+os.IsNotExist (the local path) does not
// have this gap: a real permission error there is returned, not absorbed. This test proves
// the read script itself distinguishes the two cases at the shell level, independent of the
// fake runner's plumbing: "does not exist" evaluates false and produces empty output with
// exit 0, while a file that DOES exist has its cat invoked, so cat's own exit code (whatever
// it is) becomes the script's exit code -- i.e. the fix is that failure is possible at all
// once the file exists, not swallowed by a trailing `|| true`.
func TestInjectHookConfigRemote_ReadScript_DistinguishesMissingFileFromUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	missing := dir + "/does-not-exist"
	present := dir + "/present"
	if err := os.WriteFile(present, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	readScript := func(path string) string {
		return "if [ -e '" + path + "' ]; then cat '" + path + "'; fi"
	}

	missingOut, err := safeexec.CommandContext(context.Background(), "sh", "-c", readScript(missing)).CombinedOutput()
	if err != nil {
		t.Fatalf("missing-file script exit = %v, want success (empty output, no error)", err)
	}
	if len(missingOut) != 0 {
		t.Errorf("missing-file script output = %q, want empty", missingOut)
	}

	presentOut, err := safeexec.CommandContext(context.Background(), "sh", "-c", readScript(present)).CombinedOutput()
	if err != nil {
		t.Fatalf("present-file script exit = %v, want success", err)
	}
	if string(presentOut) != "hi" {
		t.Errorf("present-file script output = %q, want %q", presentOut, "hi")
	}

	// The critical property: once the file exists, its own exit code (here, a deliberately
	// unreadable target directory instead of a file forces cat to fail) surfaces as the
	// script's exit code -- unlike the old "2>/dev/null || true" form, which would have
	// forced exit 0 here too and silently returned empty output for this failure as well.
	unreadableDir := dir + "/exists-but-is-a-dir-cat-cant-read"
	if err := os.Mkdir(unreadableDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	_, err = safeexec.CommandContext(context.Background(), "sh", "-c", readScript(unreadableDir)).CombinedOutput()
	if err == nil {
		t.Fatal("script exit = nil for a path that exists but cat cannot read (a directory), want a non-nil error propagated rather than silently swallowed")
	}
}

func TestInjectHookConfigRemote_PropagatesWriteError(t *testing.T) {
	runner := &fakeRemoteCommandRunner{
		runOutput: map[string][]byte{},
		startErr:  errors.New("boom: ssh channel rejected"),
	}
	target := RemoteHookTarget{SocketPath: "/x/.sock", BearerToken: "t"}

	err := InjectHookConfigRemote(context.Background(), runner, "/home/agent/work", "session-1", target)
	if err == nil {
		t.Fatal("InjectHookConfigRemote() error = nil, want the write-start failure to propagate")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want it to wrap the underlying write failure", err)
	}
}

// TestInjectHookConfigRemote_CallsWait_EvenWhenStdinWriteOrCloseFails is a
// regression guard for a review-caught resource leak (final sdd:6-verify
// holistic pass, MUST FIX 1): after a successful runner.Start, wait() was
// only called on the happy path -- if the subsequent stdin.Write or
// stdin.Close failed, InjectHookConfigRemote returned immediately without
// ever calling wait(), leaking the SSH connection pool's reference count
// (acquire/Release never balance) and leaving the remote session/channel
// open, per SSHRunner.Start's own doc comment. wait() must now run on every
// exit path once Start has succeeded.
func TestInjectHookConfigRemote_CallsWait_EvenWhenStdinWriteOrCloseFails(t *testing.T) {
	target := RemoteHookTarget{SocketPath: "/x/.sock", BearerToken: "t"}

	t.Run("write fails", func(t *testing.T) {
		runner := &fakeRemoteCommandRunner{
			runOutput: map[string][]byte{},
			writeErr:  errors.New("boom: stdin write failed"),
		}
		err := InjectHookConfigRemote(context.Background(), runner, "/home/agent/work", "session-1", target)
		if err == nil {
			t.Fatal("InjectHookConfigRemote() error = nil, want the stdin write failure to propagate")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("error = %v, want it to wrap the underlying write failure", err)
		}
		if runner.waitCalls != 1 {
			t.Errorf("wait() called %d times, want 1 (must still run to release the pool reference and remote channel)", runner.waitCalls)
		}
	})

	t.Run("close fails", func(t *testing.T) {
		runner := &fakeRemoteCommandRunner{
			runOutput: map[string][]byte{},
			closeErr:  errors.New("boom: stdin close failed"),
		}
		err := InjectHookConfigRemote(context.Background(), runner, "/home/agent/work", "session-1", target)
		if err == nil {
			t.Fatal("InjectHookConfigRemote() error = nil, want the stdin close failure to propagate")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("error = %v, want it to wrap the underlying close failure", err)
		}
		if runner.waitCalls != 1 {
			t.Errorf("wait() called %d times, want 1 (must still run to release the pool reference and remote channel)", runner.waitCalls)
		}
	})

	t.Run("happy path calls wait exactly once", func(t *testing.T) {
		runner := &fakeRemoteCommandRunner{runOutput: map[string][]byte{}}
		if err := InjectHookConfigRemote(context.Background(), runner, "/home/agent/work", "session-1", target); err != nil {
			t.Fatalf("InjectHookConfigRemote() error: %v", err)
		}
		if runner.waitCalls != 1 {
			t.Errorf("wait() called %d times, want exactly 1 (not skipped, and not double-invoked by the safety-net defer)", runner.waitCalls)
		}
	})
}

// jsonQuote returns s as a JSON-quoted string literal (with surrounding
// quotes), used to embed a raw command string into a hand-built JSON
// fixture above without hand-escaping it.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
