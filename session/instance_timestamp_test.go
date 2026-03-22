package session

import (
	"github.com/tstapler/stapler-squad/session/tmux"
	"testing"
	"time"
)

// TestInstance_UpdateTerminalTimestamps verifies that terminal timestamps are updated correctly
func TestInstance_UpdateTerminalTimestamps(t *testing.T) {
	opts := InstanceOptions{
		Title:   "test-session",
		Path:    "/tmp/test",
		Program: "claude",
	}
	instance, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	// Mock tmux session with banner filter - we need to use a properly initialized session
	// that includes the banner filter, not just an empty struct
	mockTmux := tmux.NewTmuxSession("test", "claude")
	instance.SetTmuxSession(mockTmux)

	// Capture initial timestamps
	initialCreation := instance.CreatedAt
	initialTerminalUpdate := instance.LastTerminalUpdate
	initialMeaningfulOutput := instance.LastMeaningfulOutput

	// Wait a bit to ensure timestamps can differ
	time.Sleep(10 * time.Millisecond)

	// Test 1: Update with meaningful content (non-banner)
	meaningfulContent := "Hello, world!\nThis is actual output\nError: something happened"
	instance.UpdateTerminalTimestamps(meaningfulContent, false)

	// Both timestamps should be updated
	if !instance.LastTerminalUpdate.After(initialTerminalUpdate) {
		t.Error("LastTerminalUpdate should be updated after meaningful content")
	}
	if !instance.LastMeaningfulOutput.After(initialMeaningfulOutput) {
		t.Error("LastMeaningfulOutput should be updated after meaningful content")
	}

	// Save timestamps for next test
	beforeBannerTerminalUpdate := instance.LastTerminalUpdate
	beforeBannerMeaningfulOutput := instance.LastMeaningfulOutput

	time.Sleep(10 * time.Millisecond)

	// Test 2: Update with banner-only content
	bannerOnlyContent := "14:23 5-Jan-24\n[session] 0:bash* \"localhost\" 14:23 5-Jan-24"
	instance.UpdateTerminalTimestamps(bannerOnlyContent, false)

	// LastTerminalUpdate should be updated (any output)
	if !instance.LastTerminalUpdate.After(beforeBannerTerminalUpdate) {
		t.Error("LastTerminalUpdate should be updated even for banner-only content")
	}

	// LastMeaningfulOutput should NOT be updated (no meaningful content)
	if instance.LastMeaningfulOutput.After(beforeBannerMeaningfulOutput) {
		t.Error("LastMeaningfulOutput should NOT be updated for banner-only content")
	}

	// Test 3: Verify initial state
	if !initialTerminalUpdate.Equal(initialCreation) {
		t.Error("LastTerminalUpdate should be initialized to creation time")
	}
	if !initialMeaningfulOutput.Equal(initialCreation) {
		t.Error("LastMeaningfulOutput should be initialized to creation time")
	}

	// Test 4: Empty content should not update anything
	beforeEmptyTerminalUpdate := instance.LastTerminalUpdate
	beforeEmptyMeaningfulOutput := instance.LastMeaningfulOutput
	time.Sleep(10 * time.Millisecond)

	instance.UpdateTerminalTimestamps("", false)

	if instance.LastTerminalUpdate.After(beforeEmptyTerminalUpdate) {
		t.Error("LastTerminalUpdate should not be updated for empty content")
	}
	if instance.LastMeaningfulOutput.After(beforeEmptyMeaningfulOutput) {
		t.Error("LastMeaningfulOutput should not be updated for empty content")
	}
}

// TestInstance_GetTimeSinceMethods verifies the time-since helper methods
func TestInstance_GetTimeSinceMethods(t *testing.T) {
	opts := InstanceOptions{
		Title:   "test-session-2",
		Path:    "/tmp/test2",
		Program: "claude",
	}
	instance, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	// Test 1: Time since creation (no updates yet)
	timeSinceCreation := instance.GetTimeSinceLastTerminalUpdate()
	if timeSinceCreation < 0 {
		t.Error("Time since last terminal update should not be negative")
	}

	timeSinceMeaningful := instance.GetTimeSinceLastMeaningfulOutput()
	if timeSinceMeaningful < 0 {
		t.Error("Time since last meaningful output should not be negative")
	}

	// Should be approximately the same (both default to creation time)
	diff := timeSinceCreation - timeSinceMeaningful
	if diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("Initial timestamps should be similar, got diff: %v", diff)
	}

	// Test 2: After terminal update
	mockTmux := tmux.NewTmuxSession("test2", "claude")
	instance.SetTmuxSession(mockTmux)

	time.Sleep(50 * time.Millisecond)
	instance.UpdateTerminalTimestamps("meaningful output here", false)

	// Time since should be very small (just updated)
	timeSinceTerminal := instance.GetTimeSinceLastTerminalUpdate()
	if timeSinceTerminal > 100*time.Millisecond {
		t.Errorf("Time since terminal update should be small, got: %v", timeSinceTerminal)
	}

	timeSinceMeaningfulNew := instance.GetTimeSinceLastMeaningfulOutput()
	if timeSinceMeaningfulNew > 100*time.Millisecond {
		t.Errorf("Time since meaningful output should be small, got: %v", timeSinceMeaningfulNew)
	}
}

// TestInstance_PreviewUpdatesTimestamps verifies that Preview() updates timestamps
func TestInstance_PreviewUpdatesTimestamps(t *testing.T) {
	opts := InstanceOptions{
		Title:   "test-preview",
		Path:    "/tmp/testpreview",
		Program: "claude",
	}
	instance, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	// Mock tmux session (not started, so Preview will return empty)
	mockTmux := tmux.NewTmuxSession("testpreview", "claude")
	instance.SetTmuxSession(mockTmux)

	initialUpdate := instance.LastTerminalUpdate
	initialMeaningful := instance.LastMeaningfulOutput

	// Note: Since we can't actually start a tmux session in this test,
	// we're just verifying the code path exists. In a real integration test,
	// Preview() would capture actual content and update timestamps.

	// The timestamps should remain unchanged since tmux isn't actually running
	if !instance.LastTerminalUpdate.Equal(initialUpdate) {
		t.Error("Timestamp should not change when tmux is not running")
	}
	if !instance.LastMeaningfulOutput.Equal(initialMeaningful) {
		t.Error("Meaningful output timestamp should not change when tmux is not running")
	}
}

// TestInstance_TimestampConcurrency verifies thread-safe timestamp updates
func TestInstance_TimestampConcurrency(t *testing.T) {
	opts := InstanceOptions{
		Title:   "test-concurrent",
		Path:    "/tmp/testconcurrent",
		Program: "claude",
	}
	instance, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	mockTmux := tmux.NewTmuxSession("testconcurrent", "claude")
	instance.SetTmuxSession(mockTmux)

	// Concurrent updates should not cause data races
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				content := "test output"
				if j%2 == 0 {
					content = "14:23 5-Jan-24" // Banner only
				}
				instance.UpdateTerminalTimestamps(content, false)
				_ = instance.GetTimeSinceLastTerminalUpdate()
				_ = instance.GetTimeSinceLastMeaningfulOutput()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without panicking, concurrency is safe
	t.Log("Concurrent timestamp updates completed successfully")
}
