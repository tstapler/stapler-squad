package services

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCLIAIClient_Complete_UsesStdin verifies that CLIAIClient writes the combined
// prompt to the subprocess's stdin and returns trimmed stdout.
// Uses `cat` as a stand-in AI binary: it echoes stdin to stdout.
func TestCLIAIClient_Complete_UsesStdin(t *testing.T) {
	catPath, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat not available")
	}

	spec := CLIAgentSpec{
		Name:            "cat",
		Binary:          "cat",
		Args:            func() []string { return []string{} },
		PromptSeparator: "\n\n---\n\n",
	}
	c := &CLIAIClient{spec: spec, bin: catPath}

	out, err := c.Complete(context.Background(), "SYSTEM", "USER")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "SYSTEM") || !strings.Contains(out, "USER") {
		t.Errorf("expected both SYSTEM and USER in output, got: %q", out)
	}
	if !strings.Contains(out, "---") {
		t.Errorf("expected separator in output, got: %q", out)
	}
}

// TestCLIAIClient_Complete_CancelsOnCtxDone verifies that context cancellation
// aborts the subprocess before it produces output.
func TestCLIAIClient_Complete_CancelsOnCtxDone(t *testing.T) {
	sleepPath, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}

	spec := CLIAgentSpec{
		Name:   "sleep",
		Binary: "sleep",
		Args:   func() []string { return []string{"30"} },
	}
	c := &CLIAIClient{spec: spec, bin: sleepPath}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = c.Complete(ctx, "sys", "user")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Complete took %v; expected cancellation within 200ms", elapsed)
	}
}

// TestNewCLIAIClient_ReturnsErrorWhenBinaryMissing verifies that a non-existent
// binary returns an error rather than panicking.
func TestNewCLIAIClient_ReturnsErrorWhenBinaryMissing(t *testing.T) {
	_, err := NewCLIAIClient(CLIAgentSpec{
		Name:   "no-such-ai-binary",
		Binary: "no-such-ai-binary-xxxxxx",
		Args:   func() []string { return nil },
	})
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
}

// TestNewBestAvailableAIClient_ReturnsNilWhenNothingAvailable verifies that
// the factory returns (nil, "") when no API key is set and no known CLI is in PATH.
// This test does NOT require any external tools.
func TestNewBestAvailableAIClient_ReturnsNilWhenNothingAvailable(t *testing.T) {
	c, backend := NewBestAvailableAIClient("", nil)
	if c != nil {
		t.Errorf("expected nil client, got %T (backend=%q)", c, backend)
	}
	if backend != "" {
		t.Errorf("expected empty backend, got %q", backend)
	}
}

// TestNewBestAvailableAIClient_FallsBackToAnthropicAPIWhenNoCLI verifies that when
// no CLI agent is in PATH, a non-empty API key produces an AnthropicAIClient.
func TestNewBestAvailableAIClient_FallsBackToAnthropicAPIWhenNoCLI(t *testing.T) {
	c, backend := NewBestAvailableAIClient("sk-ant-test-key", nil) // nil = no CLI agents
	if c == nil {
		t.Fatal("expected non-nil client when API key is set and no CLI available")
	}
	if _, ok := c.(*AnthropicAIClient); !ok {
		t.Errorf("expected *AnthropicAIClient, got %T", c)
	}
	if backend != "anthropic-api" {
		t.Errorf("expected backend %q, got %q", "anthropic-api", backend)
	}
}

// TestNewBestAvailableAIClient_FallsBackToCLIWhenNoKey verifies that with no API key
// the factory falls through to the first available CLI binary.
// Only runs when the cat binary is available (used as a stand-in for a real AI CLI).
func TestNewBestAvailableAIClient_FallsBackToCLIWhenNoKey(t *testing.T) {
	_, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("cat not in PATH")
	}

	specs := []CLIAgentSpec{
		{Name: "cat", Binary: "cat", Args: func() []string { return nil }},
	}

	c, backend := NewBestAvailableAIClient("", specs) // no API key
	if c == nil {
		t.Fatal("expected non-nil CLI client when binary is in PATH")
	}
	if backend != "cli:cat" {
		t.Errorf("expected backend %q, got %q", "cli:cat", backend)
	}
}
