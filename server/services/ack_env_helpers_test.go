package services

// ackEnv here serves the tabbed-answer tests, whose rows come from this package's
// router fixtures; the guidance package's own tests have their own copy.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/session/domain"
)

// ackDecideAfter is the detector's decision deadline.
const ackDecideAfter = 60 * time.Second

type fakeAckStore struct {
	mu    sync.Mutex
	moved []domain.DeliveryState
	sent  []*domain.GuidanceRequestData
}

func (f *fakeAckStore) SetGuidanceDelivery(_ context.Context, _ uuid.UUID, _ domain.DeliveryRoute, state domain.DeliveryState, _ int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.moved = append(f.moved, state)
	return true, nil
}

func (f *fakeAckStore) ListGuidanceByDelivery(context.Context, domain.DeliveryRoute, domain.DeliveryState) ([]*domain.GuidanceRequestData, error) {
	return f.sent, nil
}

func (f *fakeAckStore) states() []domain.DeliveryState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.DeliveryState{}, f.moved...)
}

// ackEnv drives the detector with a manual clock and a scripted pane.
type ackEnv struct {
	clock   *manualClock
	store   *fakeAckStore
	det     *guidance.GuidanceAckDetector
	rowID   string
	capture string
}

func newAckEnv(t *testing.T) *ackEnv {
	t.Helper()
	e := &ackEnv{clock: newManualClock(), store: &fakeAckStore{}, rowID: uuid.NewString()}
	e.det = guidance.NewGuidanceAckDetector(e.store, func(context.Context, string, bool) (string, error) {
		return e.capture, nil
	}, nil, e.clock.Now)
	return e
}

func (e *ackEnv) tick(d time.Duration) {
	e.clock.Advance(d)
	e.det.Tick(context.Background())
}
