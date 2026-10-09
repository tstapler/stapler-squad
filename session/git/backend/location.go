package backend

import "context"

// RepoLocation says where a repository lives and, for remote hosts, how to reach it. It is a
// sum type: Local or Remote, nothing else. Every Backend method takes one as its first
// argument after the context.
type RepoLocation interface {
	isRepoLocation()
	// Dir is the working directory git runs in (or the in-process open path).
	Dir() string
}

// Local is a repository on this machine. Root may be any directory inside the repository.
type Local struct {
	Root RepoRoot
}

// Remote is a repository on another host, reached only through Runner. A nil Runner is
// ErrNoRemoteRunner; it never falls through to a local run.
type Remote struct {
	Host   RemoteHost // identity for logs and metrics only
	Path   RemotePath
	Runner Runner
}

func (Local) isRepoLocation()  {}
func (Remote) isRepoLocation() {}

// Dir implements RepoLocation.
func (l Local) Dir() string { return string(l.Root) }

// Dir implements RepoLocation.
func (r Remote) Dir() string { return string(r.Path) }

// RequiredGitEnv is the environment every Runner must give the git processes it starts. LC_ALL=C
// pins git's messages to English, which error classification (ErrUnborn, ErrNotARepo, ...) reads;
// GIT_TERMINAL_PROMPT=0 makes a missing credential fail instead of blocking on a prompt. The
// caller appends it to its base environment. A fresh slice is returned each call.
func RequiredGitEnv() []string {
	return []string{"LC_ALL=C", "GIT_TERMINAL_PROMPT=0"}
}

// Runner is the execution port: run name with args in dir, return combined stdout and stderr.
// It is byte-for-byte the Run method of session/tmux.CommandRunner, so any tmux.CommandRunner
// satisfies it without an adapter (asserted in session/gitwiring, never here).
//
// Contract: the process runs with RequiredGitEnv; on a non-zero exit the returned error is (or
// wraps) the process exit status, exposing ExitCode() int, so exit-status checks work; the
// output carries git's stderr text, which classification reads.
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// StdoutRunner is an optional Runner extension returning stdout only. Backends prefer it for
// parsed output so stderr warnings cannot corrupt a parse; runners without it get combined
// output and the backend strips warning/hint noise itself (and refuses NUL-delimited output it
// cannot trust, ErrNoisyOutput). The same Runner contract applies, with one addition: on a
// non-zero exit the error must be an *exec.ExitError whose Stderr holds git's stderr, because the
// returned bytes are stdout only and classification needs the message.
type StdoutRunner interface {
	RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}
