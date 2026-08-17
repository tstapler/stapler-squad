package tmux

// remoteEnvUnset/remoteEnvTerm are the environment normalization applied by
// wrapRemoteCommand to every command run over a remote CommandRunner (an
// SSHRunner). Named separately from the literal args below so the intent of
// each flag reads clearly at the call site of wrapRemoteCommand's tests.
const (
	// remoteEnvTermValue is the $TERM value forced on every remote-bound
	// tmux invocation. sshd's AcceptEnv is disabled by default in most
	// configs, so the remote host's own default $TERM (which may lack a
	// terminfo entry the local client understands, especially on minimal
	// container base images) would otherwise be used instead of whatever
	// the local shell's $TERM is. See research/pitfalls.md §2.
	remoteEnvTermValue = "xterm-256color"
)

// wrapRemoteCommand rewrites name/args into an invocation of that command
// prefixed with "env -u TMUX TERM=xterm-256color", for use only when the
// command is about to run on a remote host over an SSHRunner (gated by the
// caller checking CommandRunner.IsRemote() first -- this function itself
// does not know or care whether it's being used on a local or remote
// runner).
//
// Two distinct pitfalls this closes (research/pitfalls.md §2):
//   - Nested-tmux misdetection: if $TMUX leaks over the SSH connection (e.g.
//     the local dashboard process itself happens to be running inside a
//     tmux pane), the remote tmux client can think it's already inside a
//     session and refuse to nest, or attach to the wrong socket. "-u TMUX"
//     unsets it unconditionally in the remote command's own environment
//     regardless of what SSH did or didn't pass through.
//   - $TERM mismatch: forcing a known-good, widely-supported terminfo entry
//     explicitly in the command line itself, rather than relying on SSH env
//     passthrough (AcceptEnv), which is disabled by default in most sshd
//     configs and would otherwise silently fall back to the remote host's
//     own default $TERM.
//
// This does not shell-quote name/args itself -- callers pass the result to
// CommandRunner.Run/Start, whose own argv (SSHRunner.Run/Start) or *exec.Cmd
// (LocalRunner) construction handles quoting/escaping; wrapRemoteCommand only
// rewrites which program and argv are being invoked.
func wrapRemoteCommand(name string, args []string) (string, []string) {
	wrapped := make([]string, 0, len(args)+4)
	wrapped = append(wrapped, "-u", "TMUX", "TERM="+remoteEnvTermValue, name)
	wrapped = append(wrapped, args...)
	return "env", wrapped
}

// WrapRemoteCommand is wrapRemoteCommand, exported for callers outside this
// package that build remote tmux invocations against a RemotePtyFactory/
// CommandRunner directly -- e.g. session.Instance.GetPTYSession (Task
// 4.4.1d), which needs the exact same $TMUX-unset/$TERM-forced treatment
// for a remote "tmux attach-session" raw-PTY attach that
// startRemoteControlMode (this package) already applies to a remote "tmux
// -C attach-session".
func WrapRemoteCommand(name string, args []string) (string, []string) {
	return wrapRemoteCommand(name, args)
}
