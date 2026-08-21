// Package a contains test fixtures for the silenttransition analyzer.
package a

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session"
)

type fakeBus struct{}

func (b *fakeBus) Publish(event string) {}

type svc struct {
	storage *session.Storage
	bus     *fakeBus
}

func logIt(format string, args ...interface{}) {}

// BAD1: if-with-init shape, log-only, no return, no notify — the exact
// spawnSessionAfterGates shape (finding 1).
func (s *svc) bad1(ctx context.Context, itemID string) {
	if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, "in_progress", nil, "user"); transErr != nil { // want `error from TransitionBacklogItemStatus/UpdateItemSessionEnded is only logged here`
		logIt("failed to transition item to in_progress: %v", transErr)
	}
}

// BAD2: separate assign + if shape, log-only — the TriggerReReview shape
// (finding 2).
func (s *svc) bad2(ctx context.Context, itemID string) {
	_, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, "done", nil, "system") // want `error from TransitionBacklogItemStatus/UpdateItemSessionEnded is only logged here`
	if transErr != nil {
		logIt("PASS but transition to done failed: %v", transErr)
	}
}

// BAD3: UpdateItemSessionEnded, log-only — the autonomous orchestration shape
// (finding 4).
func (s *svc) bad3(ctx context.Context, itemID string) {
	if endErr := s.storage.UpdateItemSessionEnded(ctx, itemID, 0); endErr != nil { // want `error from TransitionBacklogItemStatus/UpdateItemSessionEnded is only logged here`
		logIt("UpdateItemSessionEnded failed: %v", endErr)
	}
}

// GOOD1: routes through a notify-shaped helper — no diagnostic.
func (s *svc) good1(ctx context.Context, itemID string) {
	if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, "in_progress", nil, "user"); transErr != nil {
		logIt("failed to transition item to in_progress: %v", transErr)
		s.notifyTransitionFailed(itemID, transErr)
	}
}

// GOOD2: routes through a direct event-bus Publish call — no diagnostic.
func (s *svc) good2(ctx context.Context, itemID string) {
	if endErr := s.storage.UpdateItemSessionEnded(ctx, itemID, 0); endErr != nil {
		logIt("UpdateItemSessionEnded failed: %v", endErr)
		s.bus.Publish(fmt.Sprintf("transition-failed-%s", itemID))
	}
}

// GOOD3: the error is propagated to the caller via return — no diagnostic.
func (s *svc) good3(ctx context.Context, itemID string) error {
	if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, "done", nil, "system"); transErr != nil {
		logIt("transition failed: %v", transErr)
		return transErr
	}
	return nil
}

// GOOD4: explicitly justified with a //nolint:silenttransition comment — no
// diagnostic.
func (s *svc) good4(ctx context.Context, itemID string) {
	if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, "review", nil, "system"); transErr != nil { //nolint:silenttransition this is a best-effort rollback already inside an error-reporting branch
		logIt("rollback transition failed: %v", transErr)
	}
}

func (s *svc) notifyTransitionFailed(itemID string, err error) {
	s.bus.Publish(fmt.Sprintf("%s: %v", itemID, err))
}
