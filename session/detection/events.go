package detection

import (
	"sync"
	"time"
)

// DetectionEvent records a single invocation of Detect() or DetectWithContext().
type DetectionEvent struct {
	SessionID       string
	Timestamp       time.Time
	MatchedPattern  string         // Pattern Name field, or "<none>" if StatusUnknown
	MatchedCategory string         // "active", "processing", "idle", etc., or "unknown"
	ResultStatus    DetectedStatus
	TailSnippet     string // Last TailSnippetBytes of cleaned terminal output
}

const (
	// EventRingCap is the maximum number of DetectionEvents retained per StatusDetector.
	EventRingCap = 500
	// TailSnippetBytes is the maximum bytes captured in TailSnippet.
	TailSnippetBytes = 512
)

// eventRing is a fixed-capacity ring buffer of DetectionEvents.
type eventRing struct {
	mu     sync.Mutex
	events [EventRingCap]DetectionEvent
	head   int // next write position
	count  int // total filled slots (capped at EventRingCap)
}

// pushLocked adds an event to the ring, overwriting the oldest entry when full.
// Callers MUST hold r.mu before calling pushLocked.
func (r *eventRing) pushLocked(e DetectionEvent) {
	r.events[r.head] = e
	r.head = (r.head + 1) % EventRingCap
	if r.count < EventRingCap {
		r.count++
	}
}

// recent returns up to n most-recent events, newest-first.
func (r *eventRing) recent(n int) []DetectionEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || r.count == 0 {
		return nil
	}
	if n > r.count {
		n = r.count
	}
	out := make([]DetectionEvent, n)
	for i := 0; i < n; i++ {
		idx := (r.head - 1 - i + EventRingCap) % EventRingCap
		out[i] = r.events[idx]
	}
	return out
}

// categoryName maps a DetectedStatus to the string stored in DetectionEvent.MatchedCategory.
func categoryName(s DetectedStatus) string {
	switch s {
	case StatusActive:
		return "active"
	case StatusProcessing:
		return "processing"
	case StatusIdle:
		return "idle"
	case StatusError:
		return "error"
	case StatusSuccess:
		return "success"
	case StatusNeedsApproval:
		return "needs_approval"
	case StatusInputRequired:
		return "input_required"
	case StatusTestsFailing:
		return "tests_failing"
	case StatusReady:
		return "ready"
	default:
		return "unknown"
	}
}
