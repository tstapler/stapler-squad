package testutil

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	expect "github.com/Netflix/go-expect"
	"github.com/creack/pty"
)

// TUISession represents an interactive TUI testing session
type TUISession struct {
	t       *testing.T
	cmd     *exec.Cmd
	console *expect.Console
	pty     *os.File
}

// ExpectSessionConfig configures expect-based TUI session behavior
type ExpectSessionConfig struct {
	// Command to run (defaults to "./stapler-squad")
	Command string
	// Arguments to pass to command
	Args []string
	// Environment variables
	Env []string
	// Working directory
	WorkDir string
	// Timeout for operations (default 30s)
	Timeout time.Duration
	// Width and height of terminal
	Cols, Rows uint16
}

// DefaultExpectConfig returns default expect testing configuration
func DefaultExpectConfig() ExpectSessionConfig {
	// Find the binary - try current dir first, then parent dir
	command := "./stapler-squad"
	if _, err := os.Stat(command); os.IsNotExist(err) {
		command = "../stapler-squad"
	}

	return ExpectSessionConfig{
		Command: command,
		Args:    []string{},
		Env:     os.Environ(),
		WorkDir: ".",
		Timeout: 30 * time.Second,
		Cols:    80,
		Rows:    24,
	}
}

// StartExpectSession starts a new interactive TUI session for testing with expect
func StartExpectSession(t *testing.T, config ExpectSessionConfig) (*TUISession, error) {
	t.Helper()

	// Validate command exists
	if _, err := os.Stat(config.Command); os.IsNotExist(err) {
		return nil, fmt.Errorf("command not found: %s (did you run 'go build'?)", config.Command)
	}

	// Create command
	cmd := exec.Command(config.Command, config.Args...)
	cmd.Env = config.Env
	cmd.Dir = config.WorkDir

	// Start with PTY
	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: config.Rows,
		Cols: config.Cols,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start command with PTY: %w", err)
	}

	// Create expect console
	console, err := expect.NewConsole(
		expect.WithStdin(ptyFile),
		expect.WithStdout(ptyFile),
		expect.WithCloser(ptyFile),
	)
	if err != nil {
		ptyFile.Close()
		cmd.Process.Kill()
		return nil, fmt.Errorf("failed to create expect console: %w", err)
	}

	session := &TUISession{
		t:       t,
		cmd:     cmd,
		console: console,
		pty:     ptyFile,
	}

	// Register cleanup
	t.Cleanup(func() {
		session.Close()
	})

	return session, nil
}

// SendKeys sends a sequence of keys to the TUI
func (s *TUISession) SendKeys(keys string) error {
	s.t.Helper()

	_, err := s.console.Send(keys)
	if err != nil {
		return fmt.Errorf("failed to send keys %q: %w", keys, err)
	}
	return nil
}

// SendLine sends a line of text followed by Enter
func (s *TUISession) SendLine(text string) error {
	s.t.Helper()
	return s.SendKeys(text + "\n")
}

// ExpectString waits for a specific string to appear in the output
func (s *TUISession) ExpectString(str string, timeout time.Duration) error {
	s.t.Helper()

	_, err := s.console.ExpectString(str)
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("TUI exited while waiting for %q", str)
		}
		return fmt.Errorf("timeout waiting for %q: %w", str, err)
	}
	return nil
}

// ExpectPattern waits for output matching a regex pattern
func (s *TUISession) ExpectPattern(pattern string, timeout time.Duration) (string, error) {
	s.t.Helper()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
	}

	match, err := s.console.Expect(expect.Regexp(re))
	if err != nil {
		if err == io.EOF {
			return "", fmt.Errorf("TUI exited while waiting for pattern %q", pattern)
		}
		return "", fmt.Errorf("timeout waiting for pattern %q: %w", pattern, err)
	}

	return match, nil
}

// WaitForPrompt waits for a common prompt pattern
func (s *TUISession) WaitForPrompt(timeout time.Duration) error {
	s.t.Helper()

	// Common prompt patterns
	patterns := []string{
		"Press",    // "Press ? for help"
		"›",        // Selection indicator
		"sessions", // Main view
		"[",        // Key hints like "[n] new"
	}

	for _, pattern := range patterns {
		err := s.ExpectString(pattern, timeout)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("no prompt detected after %v", timeout)
}

// Snapshot captures current screen content
func (s *TUISession) Snapshot() (string, error) {
	s.t.Helper()

	// Read available data without waiting
	buf := make([]byte, 4096)
	n, err := s.pty.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read screen: %w", err)
	}

	return string(buf[:n]), nil
}

// SendCtrlC sends Ctrl+C signal
func (s *TUISession) SendCtrlC() error {
	s.t.Helper()
	return s.SendKeys("\x03")
}

// SendCtrlD sends Ctrl+D (EOF)
func (s *TUISession) SendCtrlD() error {
	s.t.Helper()
	return s.SendKeys("\x04")
}

// SendEscape sends Escape key
func (s *TUISession) SendEscape() error {
	s.t.Helper()
	return s.SendKeys("\x1b")
}

// SendArrowUp sends up arrow key
func (s *TUISession) SendArrowUp() error {
	s.t.Helper()
	return s.SendKeys("\x1b[A")
}

// SendArrowDown sends down arrow key
func (s *TUISession) SendArrowDown() error {
	s.t.Helper()
	return s.SendKeys("\x1b[B")
}

// SendArrowRight sends right arrow key
func (s *TUISession) SendArrowRight() error {
	s.t.Helper()
	return s.SendKeys("\x1b[C")
}

// SendArrowLeft sends left arrow key
func (s *TUISession) SendArrowLeft() error {
	s.t.Helper()
	return s.SendKeys("\x1b[D")
}

// SendEnter sends Enter/Return key
func (s *TUISession) SendEnter() error {
	s.t.Helper()
	return s.SendKeys("\r")
}

// SendTab sends Tab key
func (s *TUISession) SendTab() error {
	s.t.Helper()
	return s.SendKeys("\t")
}

// WaitForExit waits for the TUI to exit
func (s *TUISession) WaitForExit(timeout time.Duration) error {
	s.t.Helper()

	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("TUI did not exit after %v", timeout)
	}
}

// Close terminates the TUI session
func (s *TUISession) Close() error {
	s.t.Helper()

	var errs []string

	// Kill process immediately to avoid cleanup hangs
	// In test scenarios, we don't need graceful shutdown
	if s.cmd != nil && s.cmd.Process != nil {
		if err := s.cmd.Process.Kill(); err != nil {
			// Process might have already exited
			if !strings.Contains(err.Error(), "process already finished") {
				errs = append(errs, fmt.Sprintf("failed to kill process: %v", err))
			}
		}
	}

	// Close console (may already be closed due to process kill)
	if s.console != nil {
		if err := s.console.Close(); err != nil {
			// Ignore errors - console may be already closed
			if !strings.Contains(err.Error(), "already closed") &&
				!strings.Contains(err.Error(), "bad file descriptor") {
				s.t.Logf("Console close warning: %v", err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// IsRunning checks if the TUI process is still running
func (s *TUISession) IsRunning() bool {
	s.t.Helper()

	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}

	// Check if process has exited by checking ProcessState
	// If ProcessState is nil, process hasn't exited yet
	return s.cmd.ProcessState == nil
}

// WaitForCondition waits for a custom condition with polling
func (s *TUISession) WaitForCondition(condition func() bool, timeout time.Duration, pollInterval time.Duration) error {
	s.t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("condition not met after %v", timeout)
}

// ExpectOneOf waits for any of the provided strings to appear
func (s *TUISession) ExpectOneOf(strings []string, timeout time.Duration) (string, error) {
	s.t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, str := range strings {
			err := s.ExpectString(str, 100*time.Millisecond)
			if err == nil {
				return str, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("none of the expected strings appeared after %v", timeout)
}

// GetOutput gets all available output without blocking
func (s *TUISession) GetOutput() (string, error) {
	s.t.Helper()

	// Set read deadline to avoid blocking
	s.pty.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	defer s.pty.SetReadDeadline(time.Time{})

	buf := make([]byte, 8192)
	n, err := s.pty.Read(buf)
	if err != nil {
		if os.IsTimeout(err) {
			// Timeout is expected when no data available
			return "", nil
		}
		if err == io.EOF {
			return "", fmt.Errorf("TUI has exited")
		}
		return "", fmt.Errorf("failed to read output: %w", err)
	}

	return string(buf[:n]), nil
}
