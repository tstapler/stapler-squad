// Package headless provides a subprocess-based interface for running claude -p
// headlessly. It manages session pools for prefix-cache reuse, streaming output,
// and clean subprocess lifecycle management.
package headless

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"strings"

	"github.com/tstapler/stapler-squad/executor"
)

// StreamChunk is a single unit of output from a headless LLM call.
type StreamChunk struct {
	Text    string
	Err     error
	Done    bool
	CostUSD float64 // non-zero only on the final chunk from a first-call JSON response
}

// ClaudeRunner abstracts how claude -p subprocesses are started.
// Implementors: ProcessRunner (real), FakeRunner (tests).
type ClaudeRunner interface {
	// Run starts claude -p with the given args. stdin provides the user prompt so
	// it does not appear in /proc/<pid>/cmdline. Returns a ReadCloser for stdout,
	// a stop function to kill the process, and an error if the process fails to start.
	// The caller must call stop() to release resources even when the ReadCloser is drained.
	Run(ctx context.Context, args []string, stdin io.Reader) (stdout io.ReadCloser, stop func() error, err error)
}

// Error sentinels returned in StreamChunk.Err or from CallBlocking.
var (
	// ErrClaudeNotFound is returned when the claude binary is not in PATH.
	ErrClaudeNotFound = errors.New("claude binary not found in PATH")
	// ErrLLMError is returned when claude exits with code 1 (LLM-level error).
	ErrLLMError = errors.New("claude LLM error (exit 1)")
	// ErrUsageError is returned when claude exits with code 2 (bad usage / bad flags).
	ErrUsageError = errors.New("claude usage error (exit 2)")
	// ErrInterrupted is returned when claude exits with code 130 (SIGINT).
	ErrInterrupted = errors.New("claude interrupted (exit 130)")
)

// ProcessRunner implements ClaudeRunner using executor.StartProcess.
type ProcessRunner struct {
	claudeBin       string
	workDir         string // optional working directory; empty = inherit from parent
	allowedTools    string // optional --allowedTools value; empty = not passed
	permissionMode  string // optional --permission-mode value; empty = not passed
	disallowedTools string // optional --disallowedTools value; empty = not passed
	// interpreter, when non-empty, is exec'd instead of claudeBin, with claudeBin
	// prepended to argv. Opt-in, test-only — see NewShellWrappedProcessRunnerForTesting
	// in fake_runner.go. Zero value for every production ProcessRunner.
	interpreter string
}

// WithWorkDir returns a copy of this ProcessRunner that sets the subprocess working
// directory to workDir, preserving any existing allowedTools/permissionMode/
// disallowedTools. Used by CallBlocking for per-call directory override.
func (r *ProcessRunner) WithWorkDir(workDir string) *ProcessRunner {
	return &ProcessRunner{claudeBin: r.claudeBin, workDir: workDir, allowedTools: r.allowedTools, permissionMode: r.permissionMode, disallowedTools: r.disallowedTools, interpreter: r.interpreter}
}

// WithToolAccess returns a copy of this ProcessRunner with allowedTools/permissionMode/
// disallowedTools set, preserving any existing workDir.
func (r *ProcessRunner) WithToolAccess(allowedTools, permissionMode, disallowedTools string) *ProcessRunner {
	return &ProcessRunner{claudeBin: r.claudeBin, workDir: r.workDir, allowedTools: allowedTools, permissionMode: permissionMode, disallowedTools: disallowedTools, interpreter: r.interpreter}
}

// toolAccessArgs returns the --allowedTools/--permission-mode/--disallowedTools flag
// pairs for r's configured values, in that order.
func (r *ProcessRunner) toolAccessArgs() []string {
	var extra []string
	if r.allowedTools != "" {
		extra = append(extra, "--allowedTools", r.allowedTools)
	}
	if r.permissionMode != "" {
		extra = append(extra, "--permission-mode", r.permissionMode)
	}
	if r.disallowedTools != "" {
		extra = append(extra, "--disallowedTools", r.disallowedTools)
	}
	return extra
}

// claudeAllowedEnvPrefixes lists the env-var prefixes that are forwarded to the
// claude subprocess. Everything else is stripped to avoid leaking credentials,
// database URLs, or other secrets from the parent process.
var claudeAllowedEnvPrefixes = []string{
	"HOME=", "PATH=", "USER=", "LOGNAME=", "SHELL=",
	"TMPDIR=", "TEMP=", "TMP=",
	"XDG_", "CLAUDE_", "ANTHROPIC_",
	"LANG=", "LANGUAGE=", "LC_",
}

// filteredEnv returns os.Environ() with only the vars that match claudeAllowedEnvPrefixes.
func filteredEnv() []string {
	all := os.Environ()
	out := make([]string, 0, len(all))
	for _, kv := range all {
		for _, prefix := range claudeAllowedEnvPrefixes {
			if strings.HasPrefix(kv, prefix) {
				out = append(out, kv)
				break
			}
		}
	}
	return out
}

// Run starts the claude binary with args and returns a ReadCloser for stdout.
// stdin provides the user prompt to the subprocess so it does not appear in
// /proc/<pid>/cmdline. The stop function terminates the subprocess and must
// always be called.
func (r *ProcessRunner) Run(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, func() error, error) {
	args = append(args, r.toolAccessArgs()...)
	var stderrBuf bytes.Buffer
	opts := []executor.ProcessOption{
		executor.WithNewSession(),
		executor.WithProcessReplaceEnv(filteredEnv()),
		executor.WithConsumeStderr(&stderrBuf),
	}
	if r.workDir != "" {
		opts = append(opts, executor.WithProcessDir(r.workDir))
	}
	if stdin != nil {
		opts = append(opts, executor.WithProcessStdin(stdin))
	}
	name := r.claudeBin
	if r.interpreter != "" {
		name = r.interpreter
		args = append([]string{r.claudeBin}, args...)
	}
	proc, err := executor.StartProcess(ctx, name, args, opts...)
	if err != nil {
		return nil, nil, err
	}

	stdout := proc.Stdout()
	stop := func() error {
		stopErr := proc.Stop()
		if s := strings.TrimSpace(stderrBuf.String()); s != "" {
			log.Printf("ERROR: claude headless stderr: %s", s)
		}
		return stopErr
	}
	return io.NopCloser(stdout), stop, nil
}
