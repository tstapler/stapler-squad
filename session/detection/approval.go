package session

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ApprovalType represents different types of approvals Claude might request.
type ApprovalType string

const (
	ApprovalCommand      ApprovalType = "command"      // Shell command approval
	ApprovalFileWrite    ApprovalType = "file_write"   // File write/edit approval
	ApprovalFileRead     ApprovalType = "file_read"    // File read approval
	ApprovalToolUse      ApprovalType = "tool_use"     // Tool/API usage approval
	ApprovalConfirmation ApprovalType = "confirmation" // Generic confirmation request
	ApprovalUnknown      ApprovalType = "unknown"      // Unrecognized approval pattern
)

// ApprovalRequest represents a detected approval request from Claude.
type ApprovalRequest struct {
	ID            string                `json:"id"`
	Type          ApprovalType          `json:"type"`
	Timestamp     time.Time             `json:"timestamp"`
	DetectedText  string                `json:"detected_text"`  // The text that matched the pattern
	Context       string                `json:"context"`        // Surrounding context
	ExtractedData map[string]string     `json:"extracted_data"` // Pattern capture groups
	Confidence    float64               `json:"confidence"`     // 0.0-1.0 confidence score
	Status        ApprovalRequestStatus `json:"status"`
	Response      *ApprovalResponse     `json:"response,omitempty"`
}

// ApprovalRequestStatus tracks the lifecycle of an approval request.
type ApprovalRequestStatus string

const (
	ApprovalPending  ApprovalRequestStatus = "pending"
	ApprovalApproved ApprovalRequestStatus = "approved"
	ApprovalRejected ApprovalRequestStatus = "rejected"
	ApprovalExpired  ApprovalRequestStatus = "expired"
	ApprovalIgnored  ApprovalRequestStatus = "ignored"
)

// ApprovalResponse contains the user's response to an approval request.
type ApprovalResponse struct {
	Approved  bool      `json:"approved"`
	Timestamp time.Time `json:"timestamp"`
	UserInput string    `json:"user_input,omitempty"` // Optional user comment
}

// ApprovalPattern defines a pattern for detecting approval requests.
type ApprovalPattern struct {
	Name        string       `json:"name"`
	Type        ApprovalType `json:"type"`
	Pattern     string       `json:"pattern"`      // Regex pattern
	Confidence  float64      `json:"confidence"`   // Base confidence score
	ContextSize int          `json:"context_size"` // Lines of context to capture
	CaptureKeys []string     `json:"capture_keys"` // Names for regex capture groups
	compiled    *regexp.Regexp
}

// ApprovalDetector detects approval requests in command output.
type ApprovalDetector struct {
	patterns    []*ApprovalPattern
	mu          sync.RWMutex
	history     []*ApprovalRequest
	maxHistory  int
	subscribers map[string]chan<- *ApprovalRequest
}

// NewApprovalDetector creates a new approval detector with default patterns.
func NewApprovalDetector() *ApprovalDetector {
	detector := &ApprovalDetector{
		patterns:    make([]*ApprovalPattern, 0),
		history:     make([]*ApprovalRequest, 0),
		maxHistory:  1000,
		subscribers: make(map[string]chan<- *ApprovalRequest),
	}

	// Load default patterns
	detector.loadDefaultPatterns()

	return detector
}

// loadDefaultPatterns loads commonly used approval detection patterns.
func (ad *ApprovalDetector) loadDefaultPatterns() {
	defaultPatterns := []*ApprovalPattern{
		{
			Name:        "bash_command_approval",
			Type:        ApprovalCommand,
			Pattern:     "(?i)(?:execute|run|do you want me to|should I|may I)\\s+(?:the\\s+)?(?:command|following)?:?\\s*[`'\"]\\s*([^`'\"]+)\\s*[`'\"]",
			Confidence:  0.9,
			ContextSize: 2,
			CaptureKeys: []string{"command"},
		},
		{
			Name:        "file_write_approval",
			Type:        ApprovalFileWrite,
			Pattern:     "(?i)(?:write|save|create|update|modify)\\s+(?:the\\s+)?(?:file|content)\\s+(?:to\\s+)?[`'\"]?([^`'\"?\\s]+)[`'\"]?",
			Confidence:  0.85,
			ContextSize: 2,
			CaptureKeys: []string{"file_path"},
		},
		{
			Name:        "file_read_approval",
			Type:        ApprovalFileRead,
			Pattern:     "(?i)(?:read|view|check|look at|examine)\\s+(?:the\\s+)?(?:file|content)\\s+(?:at\\s+)?[`'\"]?([^`'\"?\\s]+)[`'\"]?",
			Confidence:  0.8,
			ContextSize: 2,
			CaptureKeys: []string{"file_path"},
		},
		{
			Name:        "tool_use_approval",
			Type:        ApprovalToolUse,
			Pattern:     "(?i)(?:use|call|invoke)\\s+(?:the\\s+)?(?:tool|api|function)\\s+[`'\"]?([^`'\"?\\s]+)[`'\"]?",
			Confidence:  0.85,
			ContextSize: 2,
			CaptureKeys: []string{"tool_name"},
		},
		{
			Name:        "confirmation_request",
			Type:        ApprovalConfirmation,
			Pattern:     `(?i)(?:(?:do you want|would you like|should I|may I|can I|is it okay|is it ok)\s+(?:me\s+)?to|confirm|proceed\s+with|continue\s+with)`,
			Confidence:  0.75,
			ContextSize: 3,
			CaptureKeys: []string{},
		},
		{
			Name:        "yes_no_question",
			Type:        ApprovalConfirmation,
			Pattern:     `(?i)\?[^\?]*$`,
			Confidence:  0.6,
			ContextSize: 1,
			CaptureKeys: []string{},
		},
	}

	for _, pattern := range defaultPatterns {
		if err := ad.AddPattern(pattern); err != nil {
			// Silently ignore pattern compilation errors for defaults
			continue
		}
	}
}

// AddPattern adds a new approval detection pattern.
func (ad *ApprovalDetector) AddPattern(pattern *ApprovalPattern) error {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	// Compile regex
	compiled, err := regexp.Compile(pattern.Pattern)
	if err != nil {
		return fmt.Errorf("failed to compile pattern '%s': %w", pattern.Name, err)
	}

	pattern.compiled = compiled
	ad.patterns = append(ad.patterns, pattern)

	return nil
}

// RemovePattern removes a pattern by name.
func (ad *ApprovalDetector) RemovePattern(name string) bool {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	for i, pattern := range ad.patterns {
		if pattern.Name == name {
			ad.patterns = append(ad.patterns[:i], ad.patterns[i+1:]...)
			return true
		}
	}

	return false
}

// GetPatterns returns all registered patterns.
func (ad *ApprovalDetector) GetPatterns() []*ApprovalPattern {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	result := make([]*ApprovalPattern, len(ad.patterns))
	copy(result, ad.patterns)
	return result
}

// Detect scans output for approval requests.
func (ad *ApprovalDetector) Detect(output string) []*ApprovalRequest {
	ad.mu.RLock()
	patterns := make([]*ApprovalPattern, len(ad.patterns))
	copy(patterns, ad.patterns)
	ad.mu.RUnlock()

	var detected []*ApprovalRequest

	lines := strings.Split(output, "\n")

	for lineIdx, line := range lines {
		for _, pattern := range patterns {
			if match := pattern.compiled.FindStringSubmatch(line); match != nil {
				request := &ApprovalRequest{
					ID:            generateApprovalID(),
					Type:          pattern.Type,
					Timestamp:     time.Now(),
					DetectedText:  match[0],
					Context:       extractContext(lines, lineIdx, pattern.ContextSize),
					ExtractedData: extractCaptureGroups(match, pattern.CaptureKeys),
					Confidence:    pattern.Confidence,
					Status:        ApprovalPending,
				}

				detected = append(detected, request)

				// Notify subscribers
				ad.notifySubscribers(request)
			}
		}
	}

	// Add to history
	if len(detected) > 0 {
		ad.addToHistory(detected...)
	}

	return detected
}

// DetectInChunk processes a single response chunk for approval patterns.
func (ad *ApprovalDetector) DetectInChunk(chunk ResponseChunk) *ApprovalRequest {
	if chunk.Error != nil {
		return nil
	}

	requests := ad.Detect(string(chunk.Data))
	if len(requests) > 0 {
		return requests[0] // Return first detected request
	}

	return nil
}

// extractContext extracts surrounding lines for context.
func extractContext(lines []string, lineIdx, contextSize int) string {
	start := lineIdx - contextSize
	if start < 0 {
		start = 0
	}

	end := lineIdx + contextSize + 1
	if end > len(lines) {
		end = len(lines)
	}

	contextLines := lines[start:end]
	return strings.Join(contextLines, "\n")
}

// extractCaptureGroups extracts named capture groups from regex match.
func extractCaptureGroups(match []string, keys []string) map[string]string {
	result := make(map[string]string)

	// Skip first element (full match), map remaining to keys
	for i, key := range keys {
		if i+1 < len(match) {
			result[key] = match[i+1]
		}
	}

	return result
}

// addToHistory adds requests to the detection history.
func (ad *ApprovalDetector) addToHistory(requests ...*ApprovalRequest) {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	ad.history = append(ad.history, requests...)

	// Enforce max history limit
	if ad.maxHistory > 0 && len(ad.history) > ad.maxHistory {
		ad.history = ad.history[len(ad.history)-ad.maxHistory:]
	}
}

// GetHistory returns recent approval detection history.
func (ad *ApprovalDetector) GetHistory(limit int) []*ApprovalRequest {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	if limit <= 0 || limit > len(ad.history) {
		limit = len(ad.history)
	}

	// Return most recent first
	result := make([]*ApprovalRequest, limit)
	for i := 0; i < limit; i++ {
		result[i] = ad.history[len(ad.history)-1-i]
	}

	return result
}

// GetPendingRequests returns all pending approval requests.
func (ad *ApprovalDetector) GetPendingRequests() []*ApprovalRequest {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	var pending []*ApprovalRequest
	for _, request := range ad.history {
		if request.Status == ApprovalPending {
			pending = append(pending, request)
		}
	}

	return pending
}

// GetRequestByID retrieves a specific approval request by ID.
func (ad *ApprovalDetector) GetRequestByID(id string) *ApprovalRequest {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	for _, request := range ad.history {
		if request.ID == id {
			return request
		}
	}

	return nil
}

// UpdateRequestStatus updates the status of an approval request.
func (ad *ApprovalDetector) UpdateRequestStatus(id string, status ApprovalRequestStatus, response *ApprovalResponse) error {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	for _, request := range ad.history {
		if request.ID == id {
			request.Status = status
			request.Response = response
			return nil
		}
	}

	return fmt.Errorf("approval request '%s' not found", id)
}

// Subscribe creates a subscription for approval detection events.
func (ad *ApprovalDetector) Subscribe(subscriberID string) <-chan *ApprovalRequest {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	ch := make(chan *ApprovalRequest, 100)
	ad.subscribers[subscriberID] = ch

	return ch
}

// Unsubscribe removes a subscription.
func (ad *ApprovalDetector) Unsubscribe(subscriberID string) {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	if ch, exists := ad.subscribers[subscriberID]; exists {
		close(ch)
		delete(ad.subscribers, subscriberID)
	}
}

// notifySubscribers sends approval requests to all subscribers.
func (ad *ApprovalDetector) notifySubscribers(request *ApprovalRequest) {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	for _, ch := range ad.subscribers {
		select {
		case ch <- request:
		default:
			// Don't block if subscriber is slow
		}
	}
}

// ClearHistory removes all approval detection history.
func (ad *ApprovalDetector) ClearHistory() {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	ad.history = make([]*ApprovalRequest, 0)
}

// SetMaxHistory sets the maximum number of history entries to keep.
func (ad *ApprovalDetector) SetMaxHistory(max int) {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	ad.maxHistory = max

	// Trim if necessary
	if max > 0 && len(ad.history) > max {
		ad.history = ad.history[len(ad.history)-max:]
	}
}

// GetMaxHistory returns the current max history setting.
func (ad *ApprovalDetector) GetMaxHistory() int {
	ad.mu.RLock()
	defer ad.mu.RUnlock()
	return ad.maxHistory
}

// GetStatistics returns statistics about approval detection.
func (ad *ApprovalDetector) GetStatistics() ApprovalStatistics {
	ad.mu.RLock()
	defer ad.mu.RUnlock()

	stats := ApprovalStatistics{
		TotalDetections: len(ad.history),
	}

	for _, request := range ad.history {
		switch request.Status {
		case ApprovalPending:
			stats.PendingCount++
		case ApprovalApproved:
			stats.ApprovedCount++
		case ApprovalRejected:
			stats.RejectedCount++
		case ApprovalExpired:
			stats.ExpiredCount++
		case ApprovalIgnored:
			stats.IgnoredCount++
		}

		// Count by type
		switch request.Type {
		case ApprovalCommand:
			stats.CommandApprovals++
		case ApprovalFileWrite:
			stats.FileWriteApprovals++
		case ApprovalFileRead:
			stats.FileReadApprovals++
		case ApprovalToolUse:
			stats.ToolUseApprovals++
		case ApprovalConfirmation:
			stats.ConfirmationApprovals++
		}
	}

	return stats
}

// ApprovalStatistics provides summary statistics.
type ApprovalStatistics struct {
	TotalDetections       int
	PendingCount          int
	ApprovedCount         int
	RejectedCount         int
	ExpiredCount          int
	IgnoredCount          int
	CommandApprovals      int
	FileWriteApprovals    int
	FileReadApprovals     int
	ToolUseApprovals      int
	ConfirmationApprovals int
}

// generateApprovalID generates a unique ID for approval requests.
func generateApprovalID() string {
	return fmt.Sprintf("approval_%d", time.Now().UnixNano())
}
