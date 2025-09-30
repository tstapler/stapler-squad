package testutil

import (
	"os/exec"
)

// CommandExecutor defines the interface for executing external commands
// This allows for dependency injection and mocking in tests
type CommandExecutor interface {
	// Command creates a new *exec.Cmd with the given name and arguments
	Command(name string, args ...string) *exec.Cmd

	// Output runs the command and returns its standard output
	Output(cmd *exec.Cmd) ([]byte, error)

	// LookPath searches for an executable named file in the directories listed in the PATH environment variable
	LookPath(file string) (string, error)
}

// RealCommandExecutor implements CommandExecutor using actual system commands
type RealCommandExecutor struct{}

func (r *RealCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func (r *RealCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

func (r *RealCommandExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// MockCommandExecutor implements CommandExecutor for testing
type MockCommandExecutor struct {
	// CommandFunc is called when Command is invoked
	CommandFunc func(name string, args ...string) *exec.Cmd

	// OutputFunc is called when Output is invoked
	OutputFunc func(cmd *exec.Cmd) ([]byte, error)

	// LookPathFunc is called when LookPath is invoked
	LookPathFunc func(file string) (string, error)
}

func (m *MockCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	if m.CommandFunc != nil {
		return m.CommandFunc(name, args...)
	}
	// Return a dummy command that won't actually execute
	return exec.Command("echo", "mock")
}

func (m *MockCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	if m.OutputFunc != nil {
		return m.OutputFunc(cmd)
	}
	return []byte("mock output"), nil
}

func (m *MockCommandExecutor) LookPath(file string) (string, error) {
	if m.LookPathFunc != nil {
		return m.LookPathFunc(file)
	}
	return "/usr/local/bin/" + file, nil
}

// NewMockCommandExecutor creates a new MockCommandExecutor with default behavior
func NewMockCommandExecutor() *MockCommandExecutor {
	return &MockCommandExecutor{}
}

// NewMockCommandExecutorWithClaudeFound creates a mock that simulates finding claude
func NewMockCommandExecutorWithClaudeFound(claudePath string) *MockCommandExecutor {
	return &MockCommandExecutor{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte(claudePath), nil
		},
		LookPathFunc: func(file string) (string, error) {
			if file == "claude" {
				return claudePath, nil
			}
			return "/usr/local/bin/" + file, nil
		},
	}
}

// NewMockCommandExecutorWithClaudeNotFound creates a mock that simulates claude not being found
func NewMockCommandExecutorWithClaudeNotFound() *MockCommandExecutor {
	return &MockCommandExecutor{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, exec.ErrNotFound
		},
		LookPathFunc: func(file string) (string, error) {
			if file == "claude" {
				return "", exec.ErrNotFound
			}
			return "/usr/local/bin/" + file, nil
		},
	}
}