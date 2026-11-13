package blktest

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// MockReviewGateSpawner is a mock implementation of reviewgate.ReviewGateSpawner for testing.
// Guarded by mu since SpawnReviewSession can be invoked from a goroutine (e.g. via
// onSessionExited's bounded-semaphore fan-out).
type MockReviewGateSpawner struct {
	mu              sync.Mutex
	spawnCalled     bool
	callCount       int
	lastItem        *backlogdto.BacklogItemData
	lastItemSession string
	lastPrompt      string

	// Err, when non-nil, is returned by SpawnReviewSession instead of a synthesized
	// Instance — used to test the spawner-error path.
	Err error
	// Instance, when non-nil, is returned by SpawnReviewSession on success instead of
	// a synthesized one with a random UUID — used to assert the returned Instance's
	// UUID is exactly what gets linked into the resulting ItemSession.
	Instance *session.Instance
}

func (m *MockReviewGateSpawner) SpawnReviewSession(ctx context.Context, item *backlogdto.BacklogItemData, itemSessionID string, prompt string) (*session.Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spawnCalled = true
	m.callCount++
	m.lastItem = item
	m.lastItemSession = itemSessionID
	m.lastPrompt = prompt
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Instance != nil {
		return m.Instance, nil
	}
	return &session.Instance{UUID: uuid.New().String()}, nil
}

// CallCount returns the number of times SpawnReviewSession has been called, under
// the mock's own lock — safe to read concurrently with in-flight calls.
func (m *MockReviewGateSpawner) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// SetErr changes the error SpawnReviewSession returns, safely against in-flight calls.
func (m *MockReviewGateSpawner) SetErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Err = err
}

// LastPrompt returns the prompt passed to the most recent SpawnReviewSession call.
func (m *MockReviewGateSpawner) LastPrompt() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastPrompt
}
