package session

import (
	"github.com/tstapler/stapler-squad/session/tmux"
	"testing"
	"time"
)

// TestUpdateTerminalTimestamps_SignatureBasedPreservation verifies that timestamps
// are NOT updated when terminal content is unchanged (signature matching).
// This test addresses BUG-002: Timestamp refresh reset on startup.
func TestUpdateTerminalTimestamps_SignatureBasedPreservation(t *testing.T) {
	opts := InstanceOptions{
		Title:   "test-signature",
		Path:    "/tmp/test-signature",
		Program: "claude",
	}
	instance, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	// Mock tmux session with banner filter
	mockTmux := tmux.NewTmuxSession("test-signature", "claude")
	instance.SetTmuxSession(mockTmux)

	// Simulate historical timestamp (2 hours ago)
	historicalTime := time.Now().Add(-2 * time.Hour)
	instance.LastMeaningfulOutput = historicalTime

	// Set up initial signature
	unchangedContent := "Hello, world!\nThis is unchanged terminal content\nStill here!"
	instance.UpdateTerminalTimestamps(unchangedContent, false)

	// Record the timestamp after first update
	firstUpdateTime := instance.LastMeaningfulOutput
	firstSignature := instance.LastOutputSignature

	if firstSignature == "" {
		t.Fatal("Signature should be set after first update")
	}

	// Wait a bit to ensure timestamps would differ if updated
	time.Sleep(50 * time.Millisecond)

	// Test 1: Update with SAME content - timestamp should NOT change
	instance.UpdateTerminalTimestamps(unchangedContent, false)

	if !instance.LastMeaningfulOutput.Equal(firstUpdateTime) {
		t.Errorf("LastMeaningfulOutput should be preserved when content unchanged. "+
			"Expected %v, got %v",
			firstUpdateTime, instance.LastMeaningfulOutput)
	}

	if instance.LastOutputSignature != firstSignature {
		t.Errorf("Signature should remain unchanged when content unchanged. "+
			"Expected %s, got %s",
			firstSignature, instance.LastOutputSignature)
	}

	// Wait again
	time.Sleep(50 * time.Millisecond)

	// Test 2: Update with DIFFERENT content - timestamp SHOULD change
	changedContent := "Hello, world!\nThis is CHANGED terminal content\nNew stuff here!"
	instance.UpdateTerminalTimestamps(changedContent, false)

	if !instance.LastMeaningfulOutput.After(firstUpdateTime) {
		t.Error("LastMeaningfulOutput should be updated when content changes")
	}

	if instance.LastOutputSignature == firstSignature {
		t.Error("Signature should change when content changes")
	}

	t.Log("✓ Signature-based timestamp preservation working correctly")
}

// TestPreview_PreservesHistoricalTimestamps verifies that Preview() preserves
// historical timestamps when terminal content hasn't changed.
// This is the core scenario described in BUG-002.
func TestPreview_PreservesHistoricalTimestamps(t *testing.T) {
	opts := InstanceOptions{
		Title:   "test-preview-preserve",
		Path:    "/tmp/test-preview-preserve",
		Program: "claude",
	}
	instance, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	// Mock tmux session
	mockTmux := tmux.NewTmuxSession("test-preview-preserve", "claude")
	instance.SetTmuxSession(mockTmux)

	// Simulate historical timestamp (2 hours ago) and existing signature
	historicalTime := time.Now().Add(-2 * time.Hour)
	instance.LastMeaningfulOutput = historicalTime

	// Set up signature for existing content
	existingContent := "Some terminal output that hasn't changed"
	instance.UpdateTerminalTimestamps(existingContent, false)
	storedSignature := instance.LastOutputSignature

	// Verify timestamp was set
	initialTimestamp := instance.LastMeaningfulOutput

	// Wait to ensure timestamps would differ if updated
	time.Sleep(50 * time.Millisecond)

	// Simulate app restart + Preview() call with SAME content
	// (This is what happens in review_queue_poller.go refreshAllSessionsInQueue)
	instance.UpdateTerminalTimestamps(existingContent, false)

	// Verify timestamp was PRESERVED (not reset to current time)
	if !instance.LastMeaningfulOutput.Equal(initialTimestamp) {
		t.Errorf("Preview() should preserve historical timestamp when content unchanged. "+
			"Expected %v, got %v",
			initialTimestamp, instance.LastMeaningfulOutput)
	}

	if instance.LastOutputSignature != storedSignature {
		t.Errorf("Signature should remain unchanged. Expected %s, got %s",
			storedSignature, instance.LastOutputSignature)
	}

	t.Log("✓ Historical timestamp preserved correctly during Preview() refresh")
}

// TestForceUpdate_AlwaysUpdatesTimestamp verifies that forceUpdate=true
// bypasses signature checking but still uses signatures for change detection.
func TestForceUpdate_SignatureChangeDetection(t *testing.T) {
	opts := InstanceOptions{
		Title:   "test-force-update",
		Path:    "/tmp/test-force-update",
		Program: "claude",
	}
	instance, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	mockTmux := tmux.NewTmuxSession("test-force-update", "claude")
	instance.SetTmuxSession(mockTmux)

	// Set initial content and timestamp
	initialContent := "Initial terminal content"
	instance.UpdateTerminalTimestamps(initialContent, true) // forceUpdate=true
	initialTimestamp := instance.LastMeaningfulOutput
	initialSignature := instance.LastOutputSignature

	time.Sleep(50 * time.Millisecond)

	// Test 1: forceUpdate=true with SAME content should NOT update timestamp
	// (signature checking is still active in forceUpdate path)
	instance.UpdateTerminalTimestamps(initialContent, true)

	if !instance.LastMeaningfulOutput.Equal(initialTimestamp) {
		t.Error("forceUpdate=true should still preserve timestamp when content unchanged")
	}

	if instance.LastOutputSignature != initialSignature {
		t.Error("Signature should remain unchanged")
	}

	time.Sleep(50 * time.Millisecond)

	// Test 2: forceUpdate=true with DIFFERENT content SHOULD update timestamp
	changedContent := "Changed terminal content"
	instance.UpdateTerminalTimestamps(changedContent, true)

	if !instance.LastMeaningfulOutput.After(initialTimestamp) {
		t.Error("forceUpdate=true should update timestamp when content changes")
	}

	if instance.LastOutputSignature == initialSignature {
		t.Error("Signature should change when content changes")
	}

	t.Log("✓ forceUpdate signature-based change detection working correctly")
}

// TestSignatureStability verifies that the same content always produces
// the same signature (hash stability).
func TestSignatureStability(t *testing.T) {
	content := "Test terminal content\nLine 2\nLine 3"

	signature1 := computeContentSignature(content)
	signature2 := computeContentSignature(content)

	if signature1 != signature2 {
		t.Errorf("Same content should produce same signature. Got %s and %s",
			signature1, signature2)
	}

	// Different content should produce different signature
	differentContent := "Different terminal content\nLine 2\nLine 3"
	signature3 := computeContentSignature(differentContent)

	if signature1 == signature3 {
		t.Error("Different content should produce different signatures")
	}

	t.Log("✓ Content signature hashing is stable and deterministic")
}

// TestReviewQueueRefreshScenario simulates the exact scenario from BUG-002:
// 1. Session has old timestamp (2 hours ago)
// 2. App restarts
// 3. Review queue poller calls Preview() to refresh timestamps
// 4. Content hasn't changed, so timestamp should be preserved
func TestReviewQueueRefreshScenario(t *testing.T) {
	opts := InstanceOptions{
		Title:   "test-review-refresh",
		Path:    "/tmp/test-review-refresh",
		Program: "claude",
	}
	instance, err := NewInstance(opts)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	mockTmux := tmux.NewTmuxSession("test-review-refresh", "claude")
	instance.SetTmuxSession(mockTmux)

	// STEP 1: Session was active 2 hours ago
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	terminalContent := "Claude is waiting for input\n$ "

	instance.LastMeaningfulOutput = twoHoursAgo
	instance.LastTerminalUpdate = twoHoursAgo
	instance.LastOutputSignature = computeContentSignature(terminalContent)

	// STEP 2: Simulate app restart - timestamps loaded from disk
	// (already set above - in real scenario, loaded via FromInstanceData)

	// STEP 3: Review queue poller checks if timestamps are stale
	timeSinceLastUpdate := time.Since(instance.LastTerminalUpdate)
	if timeSinceLastUpdate <= 30*time.Second {
		t.Skip("Test requires stale timestamp (> 30s), but timestamp is fresh")
	}

	// STEP 4: Poller calls Preview() to refresh timestamps
	// (Preview internally calls UpdateTerminalTimestamps with forceUpdate=false)
	instance.UpdateTerminalTimestamps(terminalContent, false)

	// CRITICAL VERIFICATION: Timestamp should be PRESERVED (not reset to now)
	timeDiff := time.Since(instance.LastMeaningfulOutput)
	expectedDiff := time.Since(twoHoursAgo)

	// Allow 1 second tolerance for test execution time
	if timeDiff < expectedDiff-time.Second {
		t.Errorf("Timestamp was incorrectly updated! "+
			"Expected to preserve ~2h old timestamp, but got timestamp %v ago. "+
			"This indicates BUG-002 is present.",
			timeDiff)
	}

	// Verify signature was used for comparison
	if instance.LastOutputSignature != computeContentSignature(terminalContent) {
		t.Error("Signature should match original content signature")
	}

	t.Logf("✓ Review queue refresh correctly preserved historical timestamp (%v old)",
		time.Since(instance.LastMeaningfulOutput))
}
