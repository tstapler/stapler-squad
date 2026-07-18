// Package tmux is a minimal stand-in for github.com/tstapler/stapler-squad/session/tmux,
// used only so the tmuxsocketscope analyzer's tests can resolve calls against
// a real *types.Func in package "session/tmux" without depending on the main
// module (tools/lint is a separate Go module).
package tmux

// Binary returns the tmux executable name.
func Binary() string { return "tmux" }

// Socket identifies which tmux server a command targets.
type Socket string

// Args prepends "-L <socket>" to args when s is non-default.
func (s Socket) Args(args ...string) []string {
	if s == "" {
		return args
	}
	return append([]string{"-L", string(s)}, args...)
}

// ResolveSocket resolves a caller-supplied socket to an isolated one in tests.
func ResolveSocket(explicit string) Socket { return Socket(explicit) }

// prependSocket is the same-package helper real code routes non-arg-builder
// call sites through.
func prependSocket(socket string, args []string) []string {
	return ResolveSocket(socket).Args(args...)
}
