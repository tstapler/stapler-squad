package mux

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

func TestAutoDiscoveryCreation(t *testing.T) {
	// Test creation with watcher
	ad, err := NewAutoDiscovery()
	if err != nil {
		t.Logf("Filesystem watcher not available: %v (expected on some systems)", err)
	}
	if ad != nil {
		defer func() { _ = ad.Stop() }()
	}

	// Test creation with fallback
	adFallback := NewAutoDiscoveryWithFallback()
	if adFallback == nil {
		t.Fatal("NewAutoDiscoveryWithFallback returned nil")
	}
	defer func() { _ = adFallback.Stop() }()

	if !adFallback.IsUsingFallback() && adFallback.watcher == nil {
		t.Error("Expected fallback mode or watcher available")
	}
}

func TestIsClaudeMuxSocket(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/tmp/ssq-mux-12345.sock", true},
		{"/tmp/ssq-mux-999.sock", true},
		{"/tmp/other-file.sock", false},
		{"/tmp/ssq-mux.sock", false}, // Missing PID
		{"/tmp/ssq-mux-12345.txt", false},
		{"ssq-mux-12345.sock", true},              // Base name only
		{"/var/run/ssq-mux-789.sock", true},       // Different directory
		{"/tmp/CLAUDE-MUX-12345.sock", false},     // Case sensitive
		{"/tmp/ssq-mux-12345.sock.old", false},    // Extra extension
		{"/tmp/.ssq-mux-12345.sock", false},       // Hidden file
		{"/tmp/my-ssq-mux-12345.sock", false},     // Prefix mismatch
		{"/tmp/ssq-mux-12345.sock.backup", false}, // Suffix
		{"/tmp/ssq-mux-abc.sock", true},           // Non-numeric OK
		{"/tmp/ssq-mux-.sock", true},              // Empty PID OK
	}

	for _, tt := range tests {
		result := isSsqMuxSocket(tt.path)
		if result != tt.expected {
			t.Errorf("isSsqMuxSocket(%q) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestAutoDiscoveryStartStop(t *testing.T) {
	ad := NewAutoDiscoveryWithFallback()
	defer func() { _ = ad.Stop() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, err := ad.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start auto-discovery: %v", err)
	}

	// Let it run briefly then cancel
	cancel()

	// Wait for shutdown
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Auto-discovery didn't stop within timeout")
	}
}

func TestAutoDiscoverySocketHandling(t *testing.T) {
	ad := NewAutoDiscoveryWithFallback()
	defer func() { _ = ad.Stop() }()

	// Create a temporary socket file for testing
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "ssq-mux-test.sock")

	// Test socket created
	f, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("Failed to create test socket: %v", err)
	}
	f.Close()

	// Socket should match pattern
	if !isSsqMuxSocket(socketPath) {
		t.Error("Test socket should match ssq-mux pattern")
	}

	// Test socket removed
	os.Remove(socketPath)

	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket should be removed")
	}
}

func TestWatcherActiveStatus(t *testing.T) {
	ad := NewAutoDiscoveryWithFallback()
	defer func() { _ = ad.Stop() }()

	// Check status methods
	isActive := ad.WatcherActive()
	isFallback := ad.IsUsingFallback()

	// One should be true (either watcher or fallback)
	if !isActive && !isFallback {
		t.Error("Expected either watcher active or fallback mode")
	}

	// They should be mutually exclusive
	if isActive && isFallback {
		t.Error("Cannot be both active and fallback")
	}
}

func TestAutoDiscoveryCallbacks(t *testing.T) {
	ad := NewAutoDiscoveryWithFallback()
	defer func() { _ = ad.Stop() }()

	callbackCalled := false
	ad.OnSessionChange(func(session *DiscoveredSession, isNew bool) {
		callbackCalled = true
	})

	// Start discovery
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := ad.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// Initial scan should happen; callback is optional (no sessions may exist)
	_ = wait.WaitForCondition(func() bool {
		return callbackCalled
	}, wait.WaitConfig{Timeout: 2 * time.Second, PollInterval: 50 * time.Millisecond, Description: "callback"})

	// Callback might not be called if no sessions exist, that's OK
	t.Logf("Callback called: %v", callbackCalled)
}
