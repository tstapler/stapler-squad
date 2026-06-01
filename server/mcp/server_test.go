package mcp

import (
	"os"
	"strings"
	"testing"
)

// TestWriteToSessionDescriptionContainsUnfiltered verifies that the
// write_to_session tool description includes the word "unfiltered", making
// clear to callers that input is forwarded verbatim to the PTY (U-4.11).
func TestWriteToSessionDescriptionContainsUnfiltered(t *testing.T) {
	data, err := os.ReadFile("tools_terminal.go")
	if err != nil {
		t.Fatalf("read tools_terminal.go: %v", err)
	}
	if !strings.Contains(string(data), "unfiltered") {
		t.Error("write_to_session tool description must contain 'unfiltered'")
	}
}

// TestToolRegistrationCount verifies that exactly 16 tools are registered
// across all core tool source files by counting s.AddTool( call sites.
// (15 original + 1 for steer_session added in the InitialPrompt work.)
func TestToolRegistrationCount(t *testing.T) {
	files := []string{
		"server.go",
		"tools_discovery.go",
		"tools_lifecycle.go",
		"tools_terminal.go",
		"tools_vcs.go",
	}
	count := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Errorf("could not read %s: %v", f, err)
			continue
		}
		count += strings.Count(string(data), "s.AddTool(")
	}
	if count != 16 {
		t.Errorf("expected 16 AddTool calls across tool files, got %d", count)
	}
}
