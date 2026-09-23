package tmux

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestTmuxSocketFilePath_EmptySocket_UsesDefaultName(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", "")
	path, err := tmuxSocketFilePath("")
	if err != nil {
		t.Fatalf("tmuxSocketFilePath(\"\") error = %v", err)
	}
	want := filepath.Join("/tmp", "tmux-"+strconv.Itoa(os.Getuid()), "default")
	if path != want {
		t.Errorf("tmuxSocketFilePath(\"\") = %q, want %q", path, want)
	}
}

func TestTmuxSocketFilePath_NamedSocket(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", "")
	path, err := tmuxSocketFilePath("my-socket")
	if err != nil {
		t.Fatalf("tmuxSocketFilePath(\"my-socket\") error = %v", err)
	}
	want := filepath.Join("/tmp", "tmux-"+strconv.Itoa(os.Getuid()), "my-socket")
	if path != want {
		t.Errorf("tmuxSocketFilePath(\"my-socket\") = %q, want %q", path, want)
	}
}

// TestTmuxSocketFilePath_HonorsTMUXTMPDIR guards the override tmux itself
// supports -- must not be hardcoded away in favor of the "/tmp" default.
func TestTmuxSocketFilePath_HonorsTMUXTMPDIR(t *testing.T) {
	t.Setenv("TMUX_TMPDIR", "/custom/tmpdir")
	path, err := tmuxSocketFilePath("")
	if err != nil {
		t.Fatalf("tmuxSocketFilePath(\"\") error = %v", err)
	}
	want := filepath.Join("/custom/tmpdir", "tmux-"+strconv.Itoa(os.Getuid()), "default")
	if path != want {
		t.Errorf("tmuxSocketFilePath(\"\") = %q, want %q", path, want)
	}
}

// TestResolveClientForSocket_TMUXBinEnvAlwaysWins guards the explicit
// operator override -- must short-circuit before any OS-level lookup.
func TestResolveClientForSocket_TMUXBinEnvAlwaysWins(t *testing.T) {
	t.Setenv("TMUX_BIN", "/custom/tmux/binary")
	if got := ResolveClientForSocket("anything"); got != "/custom/tmux/binary" {
		t.Errorf("ResolveClientForSocket() = %q, want the TMUX_BIN override", got)
	}
}

// TestResolveClientForSocket_TestMode_SkipsOSLookup is the regression test
// for the reason ResolveClientForSocket checks config.IsTestMode() at all:
// without it, every uncached socket triggers a real process-table scan
// (OS-native syscalls or /proc reads depending on platform) from ordinary
// unit tests that build a tmux command, making the whole suite slower and
// non-deterministic. Under test mode it must return Binary() immediately.
func TestResolveClientForSocket_TestMode_SkipsOSLookup(t *testing.T) {
	t.Setenv("TMUX_BIN", "")
	// config.IsTestMode() detects the test binary itself (see its doc
	// comment) -- already true for this test process, no setup needed.
	got := ResolveClientForSocket("some-socket-name-nothing-listens-on")
	if got != Binary() {
		t.Errorf("ResolveClientForSocket() under test mode = %q, want Binary() = %q", got, Binary())
	}
}
