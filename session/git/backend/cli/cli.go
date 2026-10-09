// Package cli implements backend.Backend by running the git binary through the
// backend.Runner port. It never starts a process itself: a local run goes through the Runner
// supplied at construction (in production a wrapper over tmux.LocalRunner, wired in
// session/gitwiring), a remote run through the Runner carried by backend.Remote.
//
// Argv construction here mirrors the layers it replaces (session/git runGitCommand/ops.go,
// session/vc GitProvider, session/vcs GitClient and the one-off call sites in server/ and
// session/); those layers are not migrated onto this package yet.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// Backend is the CLI implementation of backend.Backend.
type Backend struct {
	local backend.Runner
}

// New returns a CLI backend. local serves backend.Local locations; it may be nil if the
// backend only ever sees Remote locations (Local locations then fail with ErrNoLocalRunner).
func New(local backend.Runner) *Backend { return &Backend{local: local} }

const maxErrorOutput = 2000

// target is where one git invocation runs.
type target struct {
	runner backend.Runner
	dir    string
}

func (b *Backend) resolve(loc backend.RepoLocation) (target, error) {
	switch l := loc.(type) {
	case backend.Local:
		if l.Root == "" {
			return target{}, fmt.Errorf("%w: empty local root", backend.ErrInvalidArgument)
		}
		if b.local == nil {
			return target{}, backend.ErrNoLocalRunner
		}
		return target{runner: b.local, dir: string(l.Root)}, nil
	case backend.Remote:
		if l.Runner == nil {
			return target{}, backend.ErrNoRemoteRunner
		}
		if l.Path == "" {
			return target{}, fmt.Errorf("%w: empty remote path", backend.ErrInvalidArgument)
		}
		return target{runner: l.Runner, dir: string(l.Path)}, nil
	default:
		return target{}, fmt.Errorf("%w: unsupported location %T", backend.ErrInvalidArgument, loc)
	}
}

// result is one finished invocation.
type result struct {
	out []byte // stdout, or combined output with warning/hint noise stripped
}

func (r result) text() string { return strings.TrimSpace(string(r.out)) }

// git runs `git args...` for op. Output is stdout when the runner offers it, otherwise combined
// output with warning/hint lines removed, so parsers never see stderr noise. A non-zero exit
// comes back as a *backend.CommandError, wrapped with a sentinel when the output identifies one.
func (b *Backend) git(ctx context.Context, loc backend.RepoLocation, op backend.OperationName, args ...string) (result, error) {
	t, err := b.resolve(loc)
	if err != nil {
		return result{}, err
	}
	return t.git(ctx, op, args...)
}

func (t target) git(ctx context.Context, op backend.OperationName, args ...string) (result, error) {
	var (
		out []byte
		err error
	)
	if sr, ok := t.runner.(backend.StdoutRunner); ok {
		out, err = sr.RunStdout(ctx, t.dir, "git", args...)
	} else {
		out, err = t.runner.Run(ctx, t.dir, "git", args...)
		out = stripNoise(out)
	}
	if err != nil {
		return result{out: out}, classify(op, out, err)
	}
	return result{out: out}, nil
}

var noisePrefixes = []string{"warning:", "hint:", "advice:"}

// stripNoise drops warning:/hint: lines git (or an ssh banner) wrote to stderr into combined output.
func stripNoise(out []byte) []byte {
	if len(out) == 0 {
		return out
	}
	lines := strings.Split(string(out), "\n")
	kept := lines[:0]
	for _, ln := range lines {
		lower := strings.ToLower(strings.TrimSpace(ln))
		noisy := false
		for _, p := range noisePrefixes {
			if strings.HasPrefix(lower, p) {
				noisy = true
				break
			}
		}
		if !noisy {
			kept = append(kept, ln)
		}
	}
	return []byte(strings.Join(kept, "\n"))
}

var credentialInURL = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@`)

// scrub hides URL userinfo (tokens, passwords) before output reaches an error message.
// Stand-in for session/git/redact.Git until that package exists (plan Story 1.3.1).
func scrub(s string) string { return credentialInURL.ReplaceAllString(s, "${1}***@") }

// classify turns a runner error into *CommandError, additionally wrapping a sentinel the
// output identifies. The raw runner error stays reachable through errors.As/Unwrap.
func classify(op backend.OperationName, out []byte, err error) error {
	text := string(out)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		text += "\n" + string(exitErr.Stderr)
	}
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	if len(text) > maxErrorOutput {
		text = text[:maxErrorOutput] + "..."
	}
	cerr := &backend.CommandError{Operation: op, Output: scrub(text), Err: err}
	if s := sentinelFor(lower); s != nil {
		return fmt.Errorf("%w: %w", s, cerr)
	}
	return cerr
}

func sentinelFor(lower string) error {
	switch {
	case strings.Contains(lower, "not a git repository"):
		return backend.ErrNotARepo
	case strings.Contains(lower, "does not have any commits yet"):
		return backend.ErrUnborn
	case strings.Contains(lower, ".lock': file exists"), strings.Contains(lower, "another git process seems to be running"):
		return backend.ErrLocked
	case strings.Contains(lower, "bad object"), strings.Contains(lower, "unable to read tree"), strings.Contains(lower, "missing object"):
		return backend.ErrObjectNotFound
	case strings.Contains(lower, "unknown revision"), strings.Contains(lower, "needed a single revision"),
		strings.Contains(lower, "bad revision"), strings.Contains(lower, "not a valid object name"),
		strings.Contains(lower, "invalid reference"):
		return backend.ErrRefNotFound
	}
	return nil
}

type exitCoder interface{ ExitCode() int }

// exitCode returns the process exit status carried by err, or -1 when it carries none
// (a dial failure, a cancelled context).
func exitCode(err error) int {
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return -1
}

// lastLine returns the last non-empty line, the value of a single-valued query once noise is gone.
func lastLine(r result) string {
	lines := strings.Split(strings.TrimSpace(string(r.out)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// optArg guards a free-form argument against being parsed by git as an option. Empty values
// are rejected too: callers omit optional arguments instead of passing "".
func optArg(name, v string) error {
	if v == "" {
		return fmt.Errorf("%w: %s is empty", backend.ErrInvalidArgument, name)
	}
	if strings.HasPrefix(v, "-") {
		return fmt.Errorf("%w: %s %q starts with '-'", backend.ErrInvalidArgument, name, v)
	}
	return nil
}

func paths(ps []backend.RepoPath) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p)
	}
	return out
}
