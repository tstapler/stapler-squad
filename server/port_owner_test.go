package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNewRemoteBindError_should_IncludePortOwner_When_AddressIsInUse(t *testing.T) {
	binDir := t.TempDir()
	lsofPath := filepath.Join(binDir, "lsof")
	const lsofOutput = "COMMAND PID USER NAME\nstapler-squad 4242 user TCP *:8444 (LISTEN)"
	if err := os.WriteFile(lsofPath, []byte("#!/bin/sh\nprintf '%s\\n' '"+lsofOutput+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	err := newRemoteBindError("0.0.0.0:8444", syscall.EADDRINUSE)

	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("newRemoteBindError() lost the original error: %v", err)
	}
	if !strings.Contains(err.Error(), "process listening on port 8444") {
		t.Fatalf("newRemoteBindError() = %q, want port-owner context", err)
	}
	if !strings.Contains(err.Error(), "stapler-squad 4242") {
		t.Fatalf("newRemoteBindError() = %q, want lsof output", err)
	}
}

func TestNewRemoteBindError_should_NotLookupPortOwner_When_ErrorIsNotAddressInUse(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	originalErr := syscall.EACCES

	err := newRemoteBindError("0.0.0.0:8444", originalErr)

	if !errors.Is(err, originalErr) {
		t.Fatalf("newRemoteBindError() lost the original error: %v", err)
	}
	if strings.Contains(err.Error(), "process listening on port") {
		t.Fatalf("newRemoteBindError() unexpectedly included port-owner context: %v", err)
	}
}
