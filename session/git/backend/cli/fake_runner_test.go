package cli_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// call is one recorded Runner invocation.
type call struct {
	Dir  string
	Name string
	Args []string
}

func (c call) argv() string { return c.Name + " " + strings.Join(c.Args, " ") }

type reply struct {
	out []byte
	err error
}

// fakeRunner records calls and answers from a table keyed by the joined git args; unknown
// commands succeed with empty output. It is combined-output only (no StdoutRunner).
type fakeRunner struct {
	mu      sync.Mutex
	calls   []call
	replies map[string]reply
}

func newFake() *fakeRunner { return &fakeRunner{replies: map[string]reply{}} }

func (f *fakeRunner) on(args string, out string, err error) *fakeRunner {
	f.replies[args] = reply{out: []byte(out), err: err}
	return f
}

func (f *fakeRunner) Run(_ context.Context, dir, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call{Dir: dir, Name: name, Args: append([]string(nil), args...)})
	r := f.replies[strings.Join(args, " ")]
	return r.out, r.err
}

func (f *fakeRunner) last() call { return f.calls[len(f.calls)-1] }

// fakeStdoutRunner additionally implements backend.StdoutRunner and records which entry point ran.
type fakeStdoutRunner struct {
	*fakeRunner
	stdoutCalls int
	stdoutOut   string
}

func (f *fakeStdoutRunner) RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	f.stdoutCalls++
	if _, err := f.Run(ctx, dir, name, args...); err != nil {
		return nil, err
	}
	return []byte(f.stdoutOut), nil
}

// exitErr mimics *exec.ExitError for exit-status classification.
type exitErr struct{ code int }

func (e exitErr) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e exitErr) ExitCode() int { return e.code }
