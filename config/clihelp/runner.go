package clihelp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HelpText is the merged, ANSI-stripped stdout+stderr of `<path> --help`.
type HelpText string

// Limits bound one probe run.
type Limits struct {
	Timeout  time.Duration
	MaxBytes int
}

// DefaultLimits is 3s and 256KiB.
func DefaultLimits() Limits { return Limits{Timeout: 3 * time.Second, MaxBytes: 256 << 10} }

// RunOutput is what a run produced. The exit code is deliberately absent:
// many tools exit non-zero from --help.
type RunOutput struct {
	Text      HelpText
	Truncated bool
	TimedOut  bool
}

// RunFunc runs `<path> --help` within lim. Injected into Prober for tests.
type RunFunc func(ctx context.Context, path ResolvedPath, lim Limits) (RunOutput, error)

const (
	testEnvPrefix = "CLIHELP_TEST_"
	waitDelay     = 200 * time.Millisecond
)

// runSpec is the test seam behind Run: production always builds
// args=["--help"], extraEnv=nil (see helpSpec).
type runSpec struct {
	path ResolvedPath
	args []string
	// extraEnv entries are kept only with the CLIHELP_TEST_ prefix.
	extraEnv []string
	// parentEnv feeds probeEnv; nil means os.Environ().
	parentEnv []string
	// loginPath becomes the child PATH; empty falls back to the parent's PATH.
	loginPath string
	lim       Limits
}

func helpSpec(path ResolvedPath, lim Limits) runSpec {
	return runSpec{path: path, args: []string{"--help"}, lim: lim}
}

// Run executes `<path> --help` with a sanitized environment, an empty temp cwd
// and a private HOME, in its own session, bounded by lim.
func Run(ctx context.Context, path ResolvedPath, lim Limits) (RunOutput, error) {
	return runWith(ctx, helpSpec(path, lim))
}

// probeEnv is an allowlist: nothing from parent except PATH (when loginPath
// is empty) reaches the child. home is a per-probe temp dir because some CLIs
// write state on --help (gemini creates ~/.gemini tmp files).
func probeEnv(parent []string, loginPath, home string) []string {
	if loginPath == "" {
		for _, kv := range parent {
			if v, ok := strings.CutPrefix(kv, "PATH="); ok {
				loginPath = v
			}
		}
	}
	return []string{
		"PATH=" + loginPath,
		"HOME=" + home,
		"TERM=dumb",
		"NO_COLOR=1",
		"CI=1",
		"COLUMNS=200",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"PAGER=cat",
		"MANPAGER=cat",
	}
}

func runWith(ctx context.Context, spec runSpec) (RunOutput, error) {
	lim := spec.lim
	if lim.Timeout <= 0 || lim.MaxBytes <= 0 {
		lim = DefaultLimits()
	}
	ctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()

	root, err := os.MkdirTemp("", "clihelp-probe-*")
	if err != nil {
		return RunOutput{}, fmt.Errorf("create probe dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	cwd, home := filepath.Join(root, "cwd"), filepath.Join(root, "home")
	for _, d := range []string{cwd, home} {
		if err := os.Mkdir(d, 0o700); err != nil {
			return RunOutput{}, fmt.Errorf("create probe dir: %w", err)
		}
	}

	parent := spec.parentEnv
	if parent == nil {
		parent = os.Environ()
	}
	cmd := newProbeCmd(ctx, spec.path, spec.args)
	cmd.Dir = cwd
	cmd.Env = probeEnv(parent, spec.loginPath, home)
	for _, kv := range spec.extraEnv {
		if strings.HasPrefix(kv, testEnvPrefix) {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cw := &capWriter{max: lim.MaxBytes}
	cw.onOverflow = func() { killGroup(cmd) }
	cmd.Stdout, cmd.Stderr = cw, cw
	cmd.Cancel = func() error { killGroup(cmd); return nil }
	cmd.WaitDelay = waitDelay

	if err := cmd.Start(); err != nil {
		return RunOutput{}, fmt.Errorf("start %s: %w", spec.path, err)
	}
	_ = cmd.Wait() // exit status is ignored; ErrWaitDelay just means a grandchild held the pipe
	killGroup(cmd) // grandchildren that outlived a normal exit

	if errors.Is(ctx.Err(), context.Canceled) {
		return RunOutput{}, ctx.Err()
	}
	return RunOutput{
		Text:      HelpText(stripANSI(cw.buf.String())),
		Truncated: cw.truncated,
		TimedOut:  errors.Is(ctx.Err(), context.DeadlineExceeded),
	}, nil
}
