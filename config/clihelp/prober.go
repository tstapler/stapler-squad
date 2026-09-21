package clihelp

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Prober resolves a command to a safe executable. In this milestone it never
// runs the program: Probe stops after resolve and the file checks.
type Prober struct {
	lookPath     func(name string) (string, error)
	stat         func(path string) (fs.FileInfo, error)
	evalSymlinks func(path string) (string, error)
	home         string
	now          func() time.Time
	shell        func() string
	loginPath    func() []string
	login        *loginPathSource // nil when WithLoginPath supplied its own
	shellRun     shellRunner
}

// Option configures NewProber.
type Option func(*Prober)

func WithLookPath(f func(name string) (string, error)) Option {
	return func(p *Prober) { p.lookPath = f }
}
func WithStat(f func(path string) (fs.FileInfo, error)) Option { return func(p *Prober) { p.stat = f } }
func WithEvalSymlinks(f func(path string) (string, error)) Option {
	return func(p *Prober) { p.evalSymlinks = f }
}
func WithHome(home string) Option           { return func(p *Prober) { p.home = home } }
func WithClock(now func() time.Time) Option { return func(p *Prober) { p.now = now } }

// WithLoginPath replaces the lookup directory list; login-shell derivation is then unused.
func WithLoginPath(f func() []string) Option { return func(p *Prober) { p.loginPath = f } }

// WithShell overrides $SHELL for login-PATH derivation.
func WithShell(f func() string) Option { return func(p *Prober) { p.shell = f } }

// NewProber is hermetic: it spawns no process and no goroutine. Until
// StartLoginPathDerivation is called, lookups use server PATH plus fallback dirs.
func NewProber(opts ...Option) *Prober {
	p := &Prober{
		stat:         os.Stat,
		evalSymlinks: filepath.EvalSymlinks,
		now:          time.Now,
		shell:        func() string { return os.Getenv("SHELL") },
		shellRun:     runShellScript,
	}
	if home, err := os.UserHomeDir(); err == nil {
		p.home = home
	}
	for _, o := range opts {
		o(p)
	}
	if p.loginPath == nil {
		p.login = newLoginPathSource(p.shell, serverPathFromEnv, func() string { return p.home }, p.shellRun, p.now)
		p.loginPath = p.login.Dirs
	}
	if p.lookPath == nil {
		p.lookPath = func(name string) (string, error) { return lookInDirs(name, p.loginPath()) }
	}
	return p
}

// StartLoginPathDerivation begins deriving the user's login-shell PATH in the
// background. Production wiring calls it once; a probe arriving earlier uses
// server PATH plus fallback dirs. No-op when WithLoginPath was supplied.
func (p *Prober) StartLoginPathDerivation() {
	if p.login != nil {
		p.login.Start()
	}
}

// LoginPathEnv is the merged lookup list as a PATH value, for use as a child's PATH.
func (p *Prober) LoginPathEnv() string {
	return strings.Join(p.loginPath(), string(filepath.ListSeparator))
}

// Probe resolves command and reports whether it names a usable executable.
// Only the first non-env-assignment token is ever consulted.
func (p *Prober) Probe(ctx context.Context, command string, opts ProbeOpts) ProbeResult {
	start := p.now()
	res := p.probe(ctx, command, opts)
	logProbe(res, opts, p.now().Sub(start))
	return res
}

func (p *Prober) probe(ctx context.Context, command string, opts ProbeOpts) ProbeResult {
	target, rerr := Resolve(command, p.home)
	if rerr != ResolveOK {
		return ProbeResult{Status: ProbeStatusNotFound}
	}
	path, ok := p.locate(target)
	if !ok {
		return ProbeResult{Status: ProbeStatusNotFound}
	}
	if target.Wrapper {
		return ProbeResult{Status: ProbeStatusFoundNoFlags, ResolvedPath: path, IsWrapper: true}
	}
	return p.execute(ctx, path, opts)
}

// execute is the seam for the --help run (cache, gate, flight, parse). Without
// a runner it reports the program as found with no flags.
func (p *Prober) execute(_ context.Context, path ResolvedPath, _ ProbeOpts) ProbeResult {
	return ProbeResult{Status: ProbeStatusFoundNoFlags, ResolvedPath: path}
}

// locate maps a target to a checked, symlink-resolved path. Any lookup error,
// including exec.ErrDot, means not found.
func (p *Prober) locate(t Target) (ResolvedPath, bool) {
	name := t.Name
	if !t.IsAbsolute {
		found, err := p.lookPath(name)
		if err != nil {
			return "", false
		}
		name = found
	}
	if !filepath.IsAbs(name) {
		return "", false
	}
	resolved, err := p.evalSymlinks(name)
	if err != nil {
		return "", false
	}
	info, err := p.stat(resolved)
	if err != nil || !usableExecutable(info) {
		return "", false
	}
	return ResolvedPath(resolved), true
}

// usableExecutable: regular file, some execute bit, not world-writable.
func usableExecutable(info fs.FileInfo) bool {
	m := info.Mode()
	return m.IsRegular() && m.Perm()&0o111 != 0 && m.Perm()&0o002 == 0
}

// lookInDirs finds name in the given absolute dirs, skipping non-regular and
// non-executable candidates.
func lookInDirs(name string, dirs []string) (string, error) {
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) {
			continue
		}
		cand := filepath.Join(dir, name)
		if info, err := os.Stat(cand); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return cand, nil
		}
	}
	return "", fs.ErrNotExist
}

// logProbe emits the single audit line per call; it never includes args or env values.
func logProbe(res ProbeResult, opts ProbeOpts, took time.Duration) {
	level := slog.LevelInfo
	if res.Status == ProbeStatusBusy {
		level = slog.LevelWarn
	}
	slog.LogAttrs(context.Background(), level, "program_probe",
		slog.String("resolved_path", string(res.ResolvedPath)),
		slog.String("status", res.Status.String()),
		slog.Int("flags", 0),
		slog.Duration("duration", took),
		slog.Bool("cache_hit", false),
		slog.Bool("truncated", false),
		slog.Bool("confirmed", opts.ConfirmExecute),
		slog.Bool("resolve_only", opts.ResolveOnly),
	)
}
