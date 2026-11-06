//go:build !windows

package clihelp

import (
	"bytes"
	"context"
	"syscall"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// limitedBuffer keeps the first max bytes and silently discards the rest, so
// a chatty rc file can never block the shell on a full pipe.
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// runShellScript runs `shell -c script` in its own session with the user's
// real environment (the shell needs HOME etc.), a 2s bound and capped output.
// The whole process group is SIGKILLed on timeout and again after exit, so rc
// background helpers (nvm, ssh-agent) neither outlive us nor hold the pipe.
func runShellScript(ctx context.Context, shell, script string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, loginPathTimeout)
	defer cancel()

	// G204: script is a package constant and shell is the server's own $SHELL, never user text.
	cmd := safeexec.CommandContextPG(ctx, shell, "-c", script) //nolint:gosec
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	out := &limitedBuffer{max: loginPathMaxOut}
	cmd.Stdout = out
	cmd.Stderr = out
	killGroup := func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.Cancel = func() error { killGroup(); return nil }
	cmd.WaitDelay = 200 * time.Millisecond

	if err := cmd.Start(); err != nil {
		return "", err
	}
	_ = cmd.Wait() // exit status and pipe-hold (ErrWaitDelay) are judged by the sentinel parse
	killGroup()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return out.buf.String(), nil
}
