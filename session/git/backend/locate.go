package backend

// Locatable is a Runner that knows whether it executes on another host. A
// session/tmux.CommandRunner satisfies it structurally, so backend needs no tmux import.
type Locatable interface {
	Runner
	IsRemote() bool
}

// Locate maps the runner a caller already owns to the RepoLocation for path: a remote runner
// yields Remote{host, path, runner} (never a Local, so remote git cannot run on this machine),
// anything else yields Local{path}. A nil runner is Local, matching GitWorktree.commandRunner's
// default. host is identity for logs and metrics only and is ignored for Local.
func Locate(r Locatable, host, path string) RepoLocation {
	if r == nil || !r.IsRemote() {
		return Local{Root: RepoRoot(path)}
	}
	return Remote{Host: RemoteHost(host), Path: RemotePath(path), Runner: r}
}
