package mux

import "github.com/tstapler/stapler-squad/session/tmux"

// prependIsolatedSocket prepends "-L <socket>" to a tmux argv when running inside a
// `go test` binary (via tmux.ResolveSocket), and returns args unchanged in production.
//
// ssq-mux's list-sessions-style enumeration calls historically had no socket argument
// at all, always targeting the real, shared default tmux socket -- the same class of
// bug closed for the session package's list-sessions/kill-session call sites (see
// tmux.ResolveSocket's doc comment for the incident). Every package-level function in
// this file that enumerates ALL sessions on the default socket must route through
// this helper.
func prependIsolatedSocket(args []string) []string {
	return tmux.ResolveSocket("").Args(args...)
}
