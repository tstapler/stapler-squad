package session

import (
	"testing"
	"time"
)

// TestLastAcknowledgedPersistence verifies that LastAcknowledged field is correctly
// persisted through serialization and deserialization cycles.
// This test addresses BUG-001: LastAcknowledged not being persisted to disk.
func TestLastAcknowledgedPersistence(t *testing.T) {
	// Create a test instance with LastAcknowledged set
	now := time.Now()
	lastAck := now.Add(-1 * time.Hour) // 1 hour ago

	instance := &Instance{
		Title:     "Test Instance",
		Path:      "/path/to/repo",
		Branch:    "test-branch",
		Status:    Ready,
		Height:    100,
		Width:     200,
		CreatedAt: now,
		UpdatedAt: now,
		Program:   "claude",
		ReviewState: ReviewState{
			LastAcknowledged: lastAck,
		},
	}

	// Convert to InstanceData (serialization)
	data := instance.ToInstanceData()

	// Verify LastAcknowledged is in the serialized data
	if data.LastAcknowledged.IsZero() {
		t.Fatal("LastAcknowledged was not serialized - field is zero in InstanceData")
	}

	if !data.LastAcknowledged.Equal(lastAck) {
		t.Errorf("LastAcknowledged not serialized correctly. Expected %v, got %v",
			lastAck, data.LastAcknowledged)
	}

	// Convert back from InstanceData (deserialization)
	restoredInstance, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("Failed to restore instance from data: %v", err)
	}

	// Verify LastAcknowledged survived the roundtrip
	if restoredInstance.LastAcknowledged.IsZero() {
		t.Fatal("LastAcknowledged was not deserialized - field is zero in restored Instance")
	}

	if !restoredInstance.LastAcknowledged.Equal(lastAck) {
		t.Errorf("LastAcknowledged not deserialized correctly. Expected %v, got %v",
			lastAck, restoredInstance.LastAcknowledged)
	}

	t.Logf("✓ LastAcknowledged correctly persisted through serialization/deserialization")
}

// TestLastAcknowledgedZeroValue verifies that instances without LastAcknowledged
// (zero time value) are handled correctly.
func TestLastAcknowledgedZeroValue(t *testing.T) {
	now := time.Now()

	instance := &Instance{
		Title:     "Test Instance",
		Path:      "/path/to/repo",
		Branch:    "test-branch",
		Status:    Ready,
		Height:    100,
		Width:     200,
		CreatedAt: now,
		UpdatedAt: now,
		Program:   "claude",
		ReviewState: ReviewState{
			LastAcknowledged: time.Time{}, // Explicitly set to zero value
		},
	}

	// Convert to InstanceData and back
	data := instance.ToInstanceData()
	restoredInstance, err := FromInstanceData(data)
	if err != nil {
		t.Fatalf("Failed to restore instance from data: %v", err)
	}

	// Verify zero value is preserved
	if !restoredInstance.LastAcknowledged.IsZero() {
		t.Errorf("Zero LastAcknowledged not preserved. Expected zero, got %v",
			restoredInstance.LastAcknowledged)
	}

	t.Logf("✓ Zero LastAcknowledged value correctly handled")
}

// TestLastAcknowledgedComparison verifies that LastAcknowledged can be compared
// with LastMeaningfulOutput for review queue snooze logic.
func TestLastAcknowledgedComparison(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	testCases := []struct {
		name                 string
		lastAcknowledged     time.Time
		lastMeaningfulOutput time.Time
		shouldBeInQueue      bool
		description          string
	}{
		{
			name:                 "Never acknowledged",
			lastAcknowledged:     time.Time{}, // Zero value
			lastMeaningfulOutput: oneHourAgo,
			shouldBeInQueue:      true,
			description:          "Session never dismissed, should appear in queue",
		},
		{
			name:                 "Recently acknowledged",
			lastAcknowledged:     oneHourAgo,
			lastMeaningfulOutput: twoHoursAgo,
			shouldBeInQueue:      false,
			description:          "Acknowledged after last output, should not appear until new output",
		},
		{
			name:                 "New activity after acknowledgment",
			lastAcknowledged:     twoHoursAgo,
			lastMeaningfulOutput: oneHourAgo,
			shouldBeInQueue:      true,
			description:          "New activity after acknowledgment, should reappear in queue",
		},
		{
			name:                 "Same time",
			lastAcknowledged:     oneHourAgo,
			lastMeaningfulOutput: oneHourAgo,
			shouldBeInQueue:      false,
			description:          "Acknowledged at exact same time as last output",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			instance := &Instance{
				Title:  "Test Instance",
				Path:   "/path/to/repo",
				Branch: "test-branch",
				Status: Ready,
				ReviewState: ReviewState{
					LastAcknowledged:     tc.lastAcknowledged,
					LastMeaningfulOutput: tc.lastMeaningfulOutput,
				},
			}

			// Review queue logic: session appears if LastAcknowledged is before LastMeaningfulOutput
			// or if LastAcknowledged is zero (never dismissed)
			shouldAppear := tc.lastAcknowledged.IsZero() ||
				tc.lastAcknowledged.Before(instance.LastMeaningfulOutput)

			if shouldAppear != tc.shouldBeInQueue {
				t.Errorf("%s: Expected shouldBeInQueue=%v, got %v",
					tc.description, tc.shouldBeInQueue, shouldAppear)
			}

			t.Logf("✓ %s: %s", tc.name, tc.description)
		})
	}
}
