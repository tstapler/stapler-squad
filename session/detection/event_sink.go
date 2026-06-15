package detection

import "time"

// DetectionEventSink owns the ring buffer and session ID for detection events.
// StatusDetector delegates all event recording and retrieval to this component.
type DetectionEventSink struct {
	sessionID string
	ring      eventRing
}

// SetSessionID sets the session identifier embedded in all future DetectionEvents.
func (s *DetectionEventSink) SetSessionID(id string) {
	s.ring.mu.Lock()
	s.sessionID = id
	s.ring.mu.Unlock()
}

// Record appends a detection event to the ring buffer.
func (s *DetectionEventSink) Record(status DetectedStatus, patternName, cleanedText string) {
	snippet := cleanedText
	if len(snippet) > TailSnippetBytes {
		snippet = snippet[len(snippet)-TailSnippetBytes:]
	}
	s.ring.mu.Lock()
	defer s.ring.mu.Unlock()
	s.ring.pushLocked(DetectionEvent{
		SessionID:       s.sessionID,
		Timestamp:       time.Now(),
		MatchedPattern:  patternName,
		MatchedCategory: categoryName(status),
		ResultStatus:    status,
		TailSnippet:     snippet,
	})
}

// Recent returns up to n most-recent DetectionEvents, newest-first.
func (s *DetectionEventSink) Recent(n int) []DetectionEvent {
	return s.ring.recent(n)
}
