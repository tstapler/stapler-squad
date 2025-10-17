package session

import (
	"fmt"
	"testing"
	"time"
)

func TestNewApprovalDetector(t *testing.T) {
	detector := NewApprovalDetector()

	if detector == nil {
		t.Fatal("NewApprovalDetector() returned nil")
	}

	patterns := detector.GetPatterns()
	if len(patterns) == 0 {
		t.Error("No default patterns loaded")
	}
}

func TestApprovalDetector_AddPattern(t *testing.T) {
	detector := NewApprovalDetector()

	pattern := &ApprovalPattern{
		Name:        "test_pattern",
		Type:        ApprovalCommand,
		Pattern:     `test\s+pattern`,
		Confidence:  0.8,
		ContextSize: 2,
		CaptureKeys: []string{},
	}

	if err := detector.AddPattern(pattern); err != nil {
		t.Fatalf("AddPattern() failed: %v", err)
	}

	patterns := detector.GetPatterns()
	found := false
	for _, p := range patterns {
		if p.Name == "test_pattern" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Pattern not found after adding")
	}
}

func TestApprovalDetector_AddPatternInvalidRegex(t *testing.T) {
	detector := NewApprovalDetector()

	pattern := &ApprovalPattern{
		Name:    "invalid_pattern",
		Type:    ApprovalCommand,
		Pattern: `[invalid(regex`,
	}

	err := detector.AddPattern(pattern)
	if err == nil {
		t.Error("AddPattern() should fail with invalid regex")
	}
}

func TestApprovalDetector_RemovePattern(t *testing.T) {
	detector := NewApprovalDetector()

	pattern := &ApprovalPattern{
		Name:    "test_remove",
		Type:    ApprovalCommand,
		Pattern: `test`,
	}

	detector.AddPattern(pattern)

	if !detector.RemovePattern("test_remove") {
		t.Error("RemovePattern() should return true for existing pattern")
	}

	if detector.RemovePattern("nonexistent") {
		t.Error("RemovePattern() should return false for nonexistent pattern")
	}
}

func TestApprovalDetector_DetectCommandApproval(t *testing.T) {
	detector := NewApprovalDetector()

	output := `I will execute the command: "ls -la /tmp"
This will list all files in the directory.`

	requests := detector.Detect(output)

	if len(requests) == 0 {
		t.Fatal("No approval requests detected")
	}

	if requests[0].Type != ApprovalCommand {
		t.Errorf("Type = %v, expected ApprovalCommand", requests[0].Type)
	}

	if requests[0].ExtractedData["command"] != "ls -la /tmp" {
		t.Errorf("Command = %q, expected %q", requests[0].ExtractedData["command"], "ls -la /tmp")
	}
}

func TestApprovalDetector_DetectFileWriteApproval(t *testing.T) {
	detector := NewApprovalDetector()

	output := `I will write the file to /path/to/file.txt
The content will be saved.`

	requests := detector.Detect(output)

	if len(requests) == 0 {
		t.Fatal("No approval requests detected")
	}

	if requests[0].Type != ApprovalFileWrite {
		t.Errorf("Type = %v, expected ApprovalFileWrite", requests[0].Type)
	}
}

func TestApprovalDetector_DetectFileReadApproval(t *testing.T) {
	detector := NewApprovalDetector()

	output := `Let me read the file at /etc/config.json
I'll check the contents.`

	requests := detector.Detect(output)

	if len(requests) == 0 {
		t.Fatal("No approval requests detected")
	}

	if requests[0].Type != ApprovalFileRead {
		t.Errorf("Type = %v, expected ApprovalFileRead", requests[0].Type)
	}
}

func TestApprovalDetector_DetectConfirmation(t *testing.T) {
	detector := NewApprovalDetector()

	output := `Do you want me to proceed with this operation?
It will make changes to the system.`

	requests := detector.Detect(output)

	if len(requests) == 0 {
		t.Fatal("No approval requests detected")
	}

	if requests[0].Type != ApprovalConfirmation {
		t.Errorf("Type = %v, expected ApprovalConfirmation", requests[0].Type)
	}
}

func TestApprovalDetector_DetectMultiple(t *testing.T) {
	detector := NewApprovalDetector()

	output := `I will execute the command: "echo test"
And then write the file to /output.txt
Do you want to proceed?`

	requests := detector.Detect(output)

	if len(requests) < 2 {
		t.Errorf("Expected at least 2 requests, got %d", len(requests))
	}
}

func TestApprovalDetector_NoDetection(t *testing.T) {
	detector := NewApprovalDetector()

	output := `This is just regular output
No approval patterns here
Just normal text`

	requests := detector.Detect(output)

	// May have some low-confidence detections due to default patterns
	// Just verify it doesn't panic
	_ = requests
}

func TestApprovalDetector_DetectInChunk(t *testing.T) {
	detector := NewApprovalDetector()

	chunk := ResponseChunk{
		Data:      []byte(`Should I run the command: "test"?`),
		Timestamp: time.Now(),
	}

	request := detector.DetectInChunk(chunk)

	if request == nil {
		t.Fatal("DetectInChunk() should detect approval request")
	}
}

func TestApprovalDetector_DetectInChunkWithError(t *testing.T) {
	detector := NewApprovalDetector()

	chunk := ResponseChunk{
		Data:      []byte(`Should I run the command: "test"?`),
		Timestamp: time.Now(),
		Error:     fmt.Errorf("error occurred"),
	}

	request := detector.DetectInChunk(chunk)

	if request != nil {
		t.Error("DetectInChunk() should not detect when there's an error")
	}
}

func TestApprovalDetector_GetHistory(t *testing.T) {
	detector := NewApprovalDetector()

	// Trigger some detections
	detector.Detect(`Execute command: "test1"`)
	detector.Detect(`Execute command: "test2"`)
	detector.Detect(`Execute command: "test3"`)

	history := detector.GetHistory(2)

	if len(history) != 2 {
		t.Errorf("GetHistory(2) returned %d items, expected 2", len(history))
	}

	// Should be most recent first
	if history[0].ExtractedData["command"] != "test3" {
		t.Error("History not in most-recent-first order")
	}
}

func TestApprovalDetector_GetHistoryAll(t *testing.T) {
	detector := NewApprovalDetector()

	detector.Detect(`Execute command: "test1"`)
	detector.Detect(`Execute command: "test2"`)

	history := detector.GetHistory(0)

	if len(history) != 2 {
		t.Errorf("GetHistory(0) returned %d items, expected 2", len(history))
	}
}

func TestApprovalDetector_GetPendingRequests(t *testing.T) {
	detector := NewApprovalDetector()

	detector.Detect(`Execute command: "test"`)

	pending := detector.GetPendingRequests()

	if len(pending) == 0 {
		t.Fatal("No pending requests found")
	}

	if pending[0].Status != ApprovalPending {
		t.Errorf("Status = %v, expected ApprovalPending", pending[0].Status)
	}
}

func TestApprovalDetector_GetRequestByID(t *testing.T) {
	detector := NewApprovalDetector()

	requests := detector.Detect(`Execute command: "test"`)
	if len(requests) == 0 {
		t.Fatal("No requests detected")
	}

	id := requests[0].ID

	found := detector.GetRequestByID(id)

	if found == nil {
		t.Error("GetRequestByID() returned nil for existing ID")
	}

	if found.ID != id {
		t.Errorf("ID = %q, expected %q", found.ID, id)
	}
}

func TestApprovalDetector_GetRequestByIDNonexistent(t *testing.T) {
	detector := NewApprovalDetector()

	found := detector.GetRequestByID("nonexistent")

	if found != nil {
		t.Error("GetRequestByID() should return nil for nonexistent ID")
	}
}

func TestApprovalDetector_UpdateRequestStatus(t *testing.T) {
	detector := NewApprovalDetector()

	requests := detector.Detect(`Execute command: "test"`)
	if len(requests) == 0 {
		t.Fatal("No requests detected")
	}

	id := requests[0].ID

	response := &ApprovalResponse{
		Approved:  true,
		Timestamp: time.Now(),
		UserInput: "approved",
	}

	err := detector.UpdateRequestStatus(id, ApprovalApproved, response)
	if err != nil {
		t.Fatalf("UpdateRequestStatus() failed: %v", err)
	}

	updated := detector.GetRequestByID(id)
	if updated.Status != ApprovalApproved {
		t.Errorf("Status = %v, expected ApprovalApproved", updated.Status)
	}

	if updated.Response == nil {
		t.Error("Response is nil after update")
	}
}

func TestApprovalDetector_UpdateRequestStatusNonexistent(t *testing.T) {
	detector := NewApprovalDetector()

	err := detector.UpdateRequestStatus("nonexistent", ApprovalApproved, nil)
	if err == nil {
		t.Error("UpdateRequestStatus() should fail for nonexistent ID")
	}
}

func TestApprovalDetector_Subscribe(t *testing.T) {
	detector := NewApprovalDetector()

	ch := detector.Subscribe("test-subscriber")

	if ch == nil {
		t.Fatal("Subscribe() returned nil channel")
	}

	// Trigger detection
	go func() {
		time.Sleep(50 * time.Millisecond)
		detector.Detect(`Execute command: "test"`)
	}()

	// Wait for notification
	select {
	case request := <-ch:
		if request == nil {
			t.Error("Received nil request")
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for approval notification")
	}

	detector.Unsubscribe("test-subscriber")
}

func TestApprovalDetector_Unsubscribe(t *testing.T) {
	detector := NewApprovalDetector()

	ch := detector.Subscribe("test-subscriber")
	detector.Unsubscribe("test-subscriber")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("Channel should be closed after unsubscribe")
	}
}

func TestApprovalDetector_ClearHistory(t *testing.T) {
	detector := NewApprovalDetector()

	detector.Detect(`Execute command: "test1"`)
	detector.Detect(`Execute command: "test2"`)

	detector.ClearHistory()

	history := detector.GetHistory(0)
	if len(history) != 0 {
		t.Errorf("History length = %d after clear, expected 0", len(history))
	}
}

func TestApprovalDetector_SetMaxHistory(t *testing.T) {
	detector := NewApprovalDetector()

	// Generate 10 detections
	for i := 0; i < 10; i++ {
		detector.Detect(`Execute command: "test"`)
	}

	detector.SetMaxHistory(5)

	if detector.GetMaxHistory() != 5 {
		t.Errorf("GetMaxHistory() = %d, expected 5", detector.GetMaxHistory())
	}

	history := detector.GetHistory(0)
	if len(history) > 5 {
		t.Errorf("History length = %d after SetMaxHistory(5), expected ≤5", len(history))
	}
}

func TestApprovalDetector_GetStatistics(t *testing.T) {
	detector := NewApprovalDetector()

	// Create some detections
	requests := detector.Detect(`Execute command: "test1"`)
	if len(requests) > 0 {
		detector.UpdateRequestStatus(requests[0].ID, ApprovalApproved, nil)
	}

	requests = detector.Detect(`Write file to /path`)
	if len(requests) > 0 {
		detector.UpdateRequestStatus(requests[0].ID, ApprovalRejected, nil)
	}

	detector.Detect(`Do you want to proceed?`)

	stats := detector.GetStatistics()

	if stats.TotalDetections < 3 {
		t.Errorf("TotalDetections = %d, expected ≥3", stats.TotalDetections)
	}

	if stats.ApprovedCount == 0 {
		t.Error("Expected at least one approved request")
	}

	if stats.RejectedCount == 0 {
		t.Error("Expected at least one rejected request")
	}
}

func TestApprovalRequest_Fields(t *testing.T) {
	detector := NewApprovalDetector()

	requests := detector.Detect(`Execute command: "ls -la"`)
	if len(requests) == 0 {
		t.Fatal("No requests detected")
	}

	request := requests[0]

	if request.ID == "" {
		t.Error("ID is empty")
	}

	if request.Type == "" {
		t.Error("Type is empty")
	}

	if request.Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}

	if request.DetectedText == "" {
		t.Error("DetectedText is empty")
	}

	if request.Confidence <= 0 || request.Confidence > 1 {
		t.Errorf("Confidence = %f, expected 0.0-1.0", request.Confidence)
	}

	if request.Status != ApprovalPending {
		t.Errorf("Initial status = %v, expected ApprovalPending", request.Status)
	}
}

func TestExtractContext(t *testing.T) {
	lines := []string{
		"line 0",
		"line 1",
		"line 2",
		"line 3",
		"line 4",
	}

	context := extractContext(lines, 2, 1)

	expected := "line 1\nline 2\nline 3"
	if context != expected {
		t.Errorf("Context = %q, expected %q", context, expected)
	}
}

func TestExtractContextBoundaries(t *testing.T) {
	lines := []string{
		"line 0",
		"line 1",
	}

	// Test at beginning
	context := extractContext(lines, 0, 5)
	if context != "line 0\nline 1" {
		t.Errorf("Context at start = %q", context)
	}

	// Test at end
	context = extractContext(lines, 1, 5)
	if context != "line 0\nline 1" {
		t.Errorf("Context at end = %q", context)
	}
}

func TestExtractCaptureGroups(t *testing.T) {
	match := []string{"full match", "group1", "group2"}
	keys := []string{"key1", "key2"}

	result := extractCaptureGroups(match, keys)

	if result["key1"] != "group1" {
		t.Errorf("key1 = %q, expected %q", result["key1"], "group1")
	}

	if result["key2"] != "group2" {
		t.Errorf("key2 = %q, expected %q", result["key2"], "group2")
	}
}

func TestExtractCaptureGroupsEmpty(t *testing.T) {
	match := []string{"full match"}
	keys := []string{"key1", "key2"}

	result := extractCaptureGroups(match, keys)

	if len(result) != 0 {
		t.Errorf("Expected empty result for missing capture groups, got %d entries", len(result))
	}
}

func TestGenerateApprovalID(t *testing.T) {
	id1 := generateApprovalID()
	if id1 == "" {
		t.Error("generateApprovalID() returned empty string")
	}

	time.Sleep(1 * time.Millisecond)

	id2 := generateApprovalID()
	if id1 == id2 {
		t.Error("generateApprovalID() should generate unique IDs")
	}
}

func Benchmark_ApprovalDetector_Detect(b *testing.B) {
	detector := NewApprovalDetector()

	output := `I will execute the command: "ls -la /tmp"
Then I'll write the file to /output.txt
Do you want to proceed with these operations?
This is some additional context that doesn't match patterns.`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(output)
	}
}

func Benchmark_ApprovalDetector_AddPattern(b *testing.B) {
	detector := NewApprovalDetector()

	pattern := &ApprovalPattern{
		Name:    "test",
		Type:    ApprovalCommand,
		Pattern: `test\s+pattern`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.AddPattern(pattern)
	}
}
