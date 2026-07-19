//go:build linux

package safeexec

import (
	"os/exec"
	"syscall"
)

// EnsurePdeathsig arranges for cmd's child process to be SIGKILLed by the
// kernel if this process dies for any reason, including SIGKILL — a signal
// Go code cannot intercept to run its own cleanup (ctx cancellation and
// deferred kill/Wait calls never execute in that case).
//
// Use this for long-running children (e.g. tmux control-mode attach) whose
// lifecycle is otherwise "managed by the caller via ctx" — that management
// only works if the caller gets to run its own shutdown code, which isn't
// guaranteed for short-lived processes (e.g. one `stapler-squad --mcp`
// process per Claude Code session) that their own parent may SIGKILL.
//
// Preserves any SysProcAttr fields already set (e.g. Setpgid) by mutating in
// place rather than replacing the struct — call after other SysProcAttr setup
// but before cmd.Start()/pty.Start(cmd).
func EnsurePdeathsig(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
