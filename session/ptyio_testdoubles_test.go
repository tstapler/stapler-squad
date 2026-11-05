package session

// Copies of the session/ptyio test doubles that package session's tests use; ptyio's own are in its _test.go files.

import (
	"context"
	"os"
	"sync"

	"github.com/tstapler/stapler-squad/pkg/analytics"
)

// mockPTY creates a pipe that simulates a PTY for testing.
func mockPTY() (*os.File, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return reader, writer, nil
}

// escapeEventSpy is a test EscapeEventWriter that records events.
type escapeEventSpy struct {
	mu     sync.Mutex
	events []analytics.EscapeEventRecord
}

func (s *escapeEventSpy) WriteEscapeEvent(_ context.Context, event analytics.EscapeEventRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *escapeEventSpy) snapshot() []analytics.EscapeEventRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]analytics.EscapeEventRecord, len(s.events))
	copy(out, s.events)
	return out
}
