package headless

import (
	"context"
	"io"
	"strings"
	"sync"
)

// FakeRunner is a test double for ClaudeRunner. It returns scripted responses
// and records call arguments for inspection.
//
// When the args contain "--output-format" followed by "json", the response must
// be valid JSON matching firstCallJSONResult schema:
//
//	{"session_id":"...","result":"...","cost_usd":0.0}
//
// Otherwise the response is returned as plain text, line by line.
type FakeRunner struct {
	mu        sync.Mutex
	responses []string
	errors    []error
	index     int

	// Calls records every set of args passed to Run, in order.
	Calls [][]string

	// Stdins records the full stdin content passed to Run, in order. The user
	// prompt is passed via stdin (not args) so it doesn't appear in /proc/<pid>/cmdline
	// — see Pool.call in caller.go — so tests asserting on prompt content must
	// inspect this rather than Calls/ArgsForCall.
	Stdins [][]byte
}

// NewFakeRunner creates a FakeRunner that returns responses in order.
// If responses is empty the runner returns an empty string for each call.
func NewFakeRunner(responses ...string) *FakeRunner {
	return &FakeRunner{responses: responses}
}

// Run returns the next scripted response (or error). It records args in Calls
// and the full stdin content in Stdins. The stop function is a no-op.
func (f *FakeRunner) Run(_ context.Context, args []string, stdin io.Reader) (io.ReadCloser, func() error, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Record args.
	argsCopy := make([]string, len(args))
	copy(argsCopy, args)
	f.Calls = append(f.Calls, argsCopy)

	// Record stdin (the user prompt is passed this way — see caller.go).
	var stdinBytes []byte
	if stdin != nil {
		stdinBytes, _ = io.ReadAll(stdin)
	}
	f.Stdins = append(f.Stdins, stdinBytes)

	idx := f.index
	f.index++

	// Return scripted error if provided.
	if f.errors != nil && idx < len(f.errors) && f.errors[idx] != nil {
		return nil, nil, f.errors[idx]
	}

	// Determine response text.
	var text string
	if idx < len(f.responses) {
		text = f.responses[idx]
	}

	stop := func() error { return nil }
	return io.NopCloser(strings.NewReader(text)), stop, nil
}

// SetErrors configures per-call errors. A nil entry means no error for that call.
func (f *FakeRunner) SetErrors(errs ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors = errs
}

// CallCount returns how many times Run has been called.
func (f *FakeRunner) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.index
}

// ArgsForCall returns the args recorded for the nth call (0-indexed).
// Returns nil if call n has not happened yet.
func (f *FakeRunner) ArgsForCall(n int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n >= len(f.Calls) {
		return nil
	}
	return f.Calls[n]
}

// StdinForCall returns the stdin content (the user prompt) recorded for the nth
// call (0-indexed). Returns "" if call n has not happened yet.
func (f *FakeRunner) StdinForCall(n int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n >= len(f.Stdins) {
		return ""
	}
	return string(f.Stdins[n])
}

// HasArg returns true if any recorded call contains arg.
func (f *FakeRunner) HasArg(arg string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, call := range f.Calls {
		for _, a := range call {
			if a == arg {
				return true
			}
		}
	}
	return false
}

// ArgsContainSequence returns true if the nth call's args contain the given sequence.
func (f *FakeRunner) ArgsContainSequence(n int, seq ...string) bool {
	args := f.ArgsForCall(n)
	for i := 0; i <= len(args)-len(seq); i++ {
		match := true
		for j, s := range seq {
			if args[i+j] != s {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// NewShellWrappedProcessRunnerForTesting constructs a ProcessRunner that execs
// scriptPath through "sh" instead of forking/exec'ing scriptPath directly.
// Use this whenever a test writes its own fake-claude shell script to a
// freshly-created temp file: direct exec-by-path
// of a just-written, just-chmod'd script can be refused by OS-level exec
// restrictions (Gatekeeper, TCC, or third-party endpoint security software) on
// some platforms, even though the exec bit and shebang line are both correct.
// Invoking through the pre-existing, already-trusted "sh" binary sidesteps
// that restriction because the OS is never asked to approve a freshly-written
// file for direct execution.
func NewShellWrappedProcessRunnerForTesting(scriptPath string) *ProcessRunner {
	return &ProcessRunner{claudeBin: scriptPath, interpreter: "sh"}
}
