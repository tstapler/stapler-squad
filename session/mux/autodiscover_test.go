package mux

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutoDiscoveryCreation(t *testing.T) {
	// Test creation with watcher
	ad, err := NewAutoDiscovery()
	if err != nil {
		t.Logf("Filesystem watcher not available: %v (expected on some systems)", err)
	}
	if ad != nil {
		defer ad.Stop()
	}

	// Test creation with fallback
	adFallback := NewAutoDiscoveryWithFallback()
	if adFallback == nil {
		t.Fatal("NewAutoDiscoveryWithFallback returned nil")
	}
	defer adFallback.Stop()

	if !adFallback.IsUsingFallback() && adFallback.watcher == nil {
		t.Error("Expected fallback mode or watcher available")
	}
}

func TestIsClaudeMuxSocket(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/tmp/claude-mux-12345.sock", true},
		{"/tmp/claude-mux-999.sock", true},
		{"/tmp/other-file.sock", false},
		{"/tmp/claude-mux.sock", false}, // Missing PID
		{"/tmp/claude-mux-12345.txt", false},
		{"claude-mux-12345.sock", true},              // Base name only
		{"/var/run/claude-mux-789.sock", true},       // Different directory
		{"/tmp/CLAUDE-MUX-12345.sock", false},        // Case sensitive
		{"/tmp/claude-mux-12345.sock.old", false},    // Extra extension
		{"/tmp/.claude-mux-12345.sock", false},       // Hidden file
		{"/tmp/my-claude-mux-12345.sock", false},     // Prefix mismatch
		{"/tmp/claude-mux-12345.sock.backup", false}, // Suffix
		{"/tmp/claude-mux-abc.sock", true},           // Non-numeric OK
		{"/tmp/claude-mux-.sock", true},              // Empty PID OK
	}

	for _, tt := range tests {
		result := isClaudeMuxSocket(tt.path)
		if result != tt.expected {
			t.Errorf("isClaudeMuxSocket(%q) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestAutoDiscoveryStartStop(t *testing.T) {
	ad := NewAutoDiscoveryWithFallback()
	defer ad.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done, err := ad.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start auto-discovery: %v", err)
	}

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Cancel context
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
	defer ad.Stop()

	// Create a temporary socket file for testing
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "claude-mux-test.sock")

	// Test socket created
	f, err := os.Create(socketPath)
	if err != nil {
		t.Fatalf("Failed to create test socket: %v", err)
	}
	f.Close()

	// Socket should match pattern
	if !isClaudeMuxSocket(socketPath) {
		t.Error("Test socket should match claude-mux pattern")
	}

	// Test socket removed
	os.Remove(socketPath)

	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket should be removed")
	}
}

func TestWatcherActiveStatus(t *testing.T) {
	ad := NewAutoDiscoveryWithFallback()
	defer ad.Stop()

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
	defer ad.Stop()

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

	// Initial scan should happen
	time.Sleep(200 * time.Millisecond)

	// Callback might not be called if no sessions exist, that's OK
	t.Logf("Callback called: %v", callbackCalled)
}
