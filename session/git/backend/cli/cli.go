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

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/redact"
	"github.com/tstapler/stapler-squad/telemetry"
)

// Backend is the CLI implementation of backend.Backend.
type Backend struct {
	local  backend.Runner
	spawns *backend.SpawnCounter
}

// Option configures New.
type Option func(*Backend)

// WithMeter registers git_backend_cli_spawn_total on meter instead of telemetry.GetMeter().
func WithMeter(meter metric.Meter) Option {
	return func(b *Backend) { b.spawns = backend.NewSpawnCounter(meter) }
}

// New returns a CLI backend. local serves backend.Local locations; it may be nil if the
// backend only ever sees Remote locations (Local locations then fail with ErrNoLocalRunner).
func New(local backend.Runner, opts ...Option) *Backend {
	b := &Backend{local: local}
	for _, opt := range opts {
		opt(b)
	}
	if b.spawns == nil {
		b.spawns = backend.NewSpawnCounter(telemetry.GetMeter())
	}
	return b
}

const maxErrorOutput = 2000

// target is where one git invocation runs.
type target struct {
	runner backend.Runner
	dir    string
	spawns *backend.SpawnCounter
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
		return target{runner: b.local, dir: string(l.Root), spawns: b.spawns}, nil
	case backend.Remote:
		if l.Runner == nil {
			return target{}, backend.ErrNoRemoteRunner
		}
		if l.Path == "" {
			return target{}, fmt.Errorf("%w: empty remote path", backend.ErrInvalidArgument)
		}
		return target{runner: l.Runner, dir: string(l.Path), spawns: b.spawns}, nil
	default:
		return target{}, fmt.Errorf("%w: unsupported location %T", backend.ErrInvalidArgument, loc)
	}
}

// result is one finished invocation.
type result struct {
	out      []byte // stdout, or combined output with warning/hint noise stripped
	combined bool   // out came from a runner without StdoutRunner
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
	return t.exec(ctx, op, true, args)
}

// exec runs git. When the runner has no StdoutRunner, combined output is noise-stripped only if
// stripNoise is set; NUL-delimited (-z) callers pass false because stripping lines would eat or
// invent records, and validate the raw bytes themselves.
func (t target) exec(ctx context.Context, op backend.OperationName, strip bool, args []string) (result, error) {
	var (
		out []byte
		err error
	)
	// The one sanctioned git spawn site: every Run of git is counted here, once.
	t.spawns.Count(ctx, op)
	if sr, ok := t.runner.(backend.StdoutRunner); ok {
		out, err = sr.RunStdout(ctx, t.dir, "git", args...)
	} else {
		out, err = t.runner.Run(ctx, t.dir, "git", args...)
		if strip {
			out = stripNoise(out)
		}
	}
	if err != nil {
		return result{out: out}, classify(op, out, err)
	}
	return result{out: out, combined: !isStdoutRunner(t.runner)}, nil
}

func isStdoutRunner(r backend.Runner) bool {
	_, ok := r.(backend.StdoutRunner)
	return ok
}

// gitZ runs a command whose output is NUL-delimited. See checkZ for what it guarantees.
func (b *Backend) gitZ(ctx context.Context, loc backend.RepoLocation, op backend.OperationName, args ...string) (result, error) {
	t, err := b.resolve(loc)
	if err != nil {
		return result{}, err
	}
	return t.exec(ctx, op, false, args)
}

// checkZ rejects combined output that cannot be a clean NUL-terminated record stream: git ends
// every -z record with NUL, so output that does not (a trailing stderr banner) is suspect, and
// with leadingNoiseIsAmbiguous a leading warning:/hint: line could be a real file name. Both
// become ErrNoisyOutput instead of silently wrong data. A StdoutRunner is always trusted.
func checkZ(res result, op backend.OperationName, leadingNoiseIsAmbiguous bool) error {
	if !res.combined || len(res.out) == 0 {
		return nil
	}
	if res.out[len(res.out)-1] != 0 || (leadingNoiseIsAmbiguous && hasNoisePrefix(string(res.out))) {
		return fmt.Errorf("%w: git %s", backend.ErrNoisyOutput, op)
	}
	return nil
}

var noisePrefixes = []string{"warning:", "hint:", "advice:"}

func hasNoisePrefix(s string) bool {
	lower := strings.ToLower(s)
	for _, p := range noisePrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// stripNoise drops warning:/hint: lines git (or an ssh banner) wrote to stderr into combined output.
func stripNoise(out []byte) []byte {
	if len(out) == 0 {
		return out
	}
	lines := strings.Split(string(out), "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if !hasNoisePrefix(strings.TrimSpace(ln)) {
			kept = append(kept, ln)
		}
	}
	return []byte(strings.Join(kept, "\n"))
}

var lockPath = regexp.MustCompile(`Unable to create '([^']+\.lock)'`)

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
	shown := redact.Git(text) // redact first: truncating before could cut a credential in half
	if len(shown) > maxErrorOutput {
		shown = strings.ToValidUTF8(shown[:maxErrorOutput], "") + "..."
	}
	cerr := &backend.CommandError{Operation: op, Output: shown, Err: err}
	if s := sentinelFor(lower, text); s != nil {
		return fmt.Errorf("%w: %w", s, cerr)
	}
	return cerr
}

func sentinelFor(lower, original string) error {
	switch {
	case strings.Contains(lower, "not a git repository"):
		return backend.ErrNotARepo
	case strings.Contains(lower, "does not have any commits yet"):
		return backend.ErrUnborn
	case strings.Contains(lower, ".lock': file exists"), strings.Contains(lower, "another git process seems to be running"):
		l := backend.ErrLocked{}
		if m := lockPath.FindStringSubmatch(original); m != nil {
			l.Path = m[1]
		}
		return l
	case strings.Contains(lower, "nothing to commit"), strings.Contains(lower, "nothing added to commit"),
		strings.Contains(lower, "no changes added to commit"):
		return backend.ErrNothingToCommit
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

// refspecArg validates a branch name that git will read as (part of) a refspec, as push, pull
// and fetch do. It is an allow-list, not a deny-list: the name must satisfy git's
// check-ref-format rules, so globs (refs/heads/*), ':' (delete) and similar cannot get through,
// and a leading '+' (force) is refused as well.
func refspecArg(name, v string) error {
	if err := optArg(name, v); err != nil {
		return err
	}
	if strings.HasPrefix(v, "+") || !validRefName(v) {
		return fmt.Errorf("%w: %s %q is not a plain ref name", backend.ErrInvalidArgument, name, v)
	}
	return nil
}

// validRefName implements `git check-ref-format` (without --allow-onelevel restrictions
// relaxed: a single component such as "main" is accepted, as for branch names).
func validRefName(v string) bool {
	if v == "" || v == "@" || strings.HasPrefix(v, "/") || strings.HasSuffix(v, "/") ||
		strings.HasSuffix(v, ".") || strings.Contains(v, "..") || strings.Contains(v, "@{") ||
		strings.Contains(v, "//") {
		return false
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(" ~^:?*[\\", r) {
			return false
		}
	}
	for _, comp := range strings.Split(v, "/") {
		if strings.HasPrefix(comp, ".") || strings.HasSuffix(comp, ".lock") {
			return false
		}
	}
	return true
}

func paths(ps []backend.RepoPath) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p)
	}
	return out
}
