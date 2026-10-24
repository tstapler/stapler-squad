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
	if _, transErr := s.storage.RawTransitionBacklogItemStatus(ctx, itemID, "in_progress", nil, "user"); transErr != nil { // want `error from RawTransitionBacklogItemStatus is only logged here`
		logIt("failed to transition item to in_progress: %v", transErr)
	}
}

// BAD2: separate assign + if shape, log-only — the TriggerReReview shape
// (finding 2).
func (s *svc) bad2(ctx context.Context, itemID string) {
	_, transErr := s.storage.RawTransitionBacklogItemStatus(ctx, itemID, "done", nil, "system") // want `error from RawTransitionBacklogItemStatus is only logged here`
	if transErr != nil {
		logIt("PASS but transition to done failed: %v", transErr)
	}
}

// BAD3: UpdateItemSessionEnded, log-only — the autonomous orchestration shape
// (finding 4).
func (s *svc) bad3(ctx context.Context, itemID string) {
	if endErr := s.storage.UpdateItemSessionEnded(ctx, itemID, 0); endErr != nil { // want `error from UpdateItemSessionEnded is only logged here`
		logIt("UpdateItemSessionEnded failed: %v", endErr)
	}
}

// BAD4: a log-only Raw* write is flagged (no reporter exists on that path).
func (s *svc) bad4(ctx context.Context, itemID string) {
	if _, err := s.storage.RawTransitionBacklogItemStatus(ctx, itemID, "done", nil, "system"); err != nil { // want `error from RawTransitionBacklogItemStatus is only logged here`
		logIt("transition failed: %v", err)
	}
	if _, err := s.storage.RawArchiveBacklogItem(ctx, itemID, nil, "system", ""); err != nil { // want `error from RawArchiveBacklogItem is only logged here`
		logIt("archive failed: %v", err)
	}
}

// BAD5: a log-only Gated* denial for an operator-class caller is flagged, so a
// human's refused click cannot vanish into the log.
func (s *svc) bad5(ctx context.Context, itemID string) {
	if _, err := s.storage.GatedTransition(ctx, session.TransitionRequest{Caller: session.CallerOperatorArchiveRPC, ItemID: itemID, To: "archived"}); err != nil { // want `error from GatedTransition is only logged here`
		logIt("archive denied: %v", err)
	}
}

// BAD6: a Caller that is not a constant is treated as operator-class.
func (s *svc) bad6(ctx context.Context, itemID string, caller session.TransitionCaller) {
	if err := s.storage.GatedSetPRAndTransition(ctx, session.TransitionRequest{Caller: caller, ItemID: itemID}, session.PRAssociation{}); err != nil { // want `error from GatedSetPRAndTransition is only logged here`
		logIt("pr denied: %v", err)
	}
}

// BAD7: a request held in a variable cannot be resolved, so it is in scope.
func (s *svc) bad7(ctx context.Context, req session.TransitionRequest) {
	if _, err := s.storage.GatedTransition(ctx, req); err != nil { // want `error from GatedTransition is only logged here`
		logIt("transition denied: %v", err)
	}
}

// GOOD1: routes through a notify-shaped helper — no diagnostic.
func (s *svc) good1(ctx context.Context, itemID string) {
	if _, transErr := s.storage.RawTransitionBacklogItemStatus(ctx, itemID, "in_progress", nil, "user"); transErr != nil {
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
	if _, transErr := s.storage.RawTransitionBacklogItemStatus(ctx, itemID, "done", nil, "system"); transErr != nil {
		logIt("transition failed: %v", transErr)
		return transErr
	}
	return nil
}

// GOOD4: explicitly justified with a //nolint:silenttransition comment — no
// diagnostic.
func (s *svc) good4(ctx context.Context, itemID string) {
	if _, transErr := s.storage.RawTransitionBacklogItemStatus(ctx, itemID, "review", nil, "system"); transErr != nil { //nolint:silenttransition this is a best-effort rollback already inside an error-reporting branch
		logIt("rollback transition failed: %v", transErr)
	}
}

// GOOD5: an operator-class Gated* call that returns the denial — no diagnostic.
func (s *svc) good5(ctx context.Context, itemID string) error {
	if _, err := s.storage.GatedTransition(ctx, session.TransitionRequest{Caller: session.CallerOperatorArchiveRPC, ItemID: itemID, To: "archived"}); err != nil {
		logIt("archive denied: %v", err)
		return err
	}
	return nil
}

// GOOD6: a log-only Gated* call for an agent-class caller is deliberately not
// flagged: the gate reports an agent denial itself (Story 1.5.3's gate test).
func (s *svc) good6(ctx context.Context, itemID string) {
	if _, err := s.storage.GatedTransition(ctx, session.TransitionRequest{Caller: session.CallerMCPResumeWork, ItemID: itemID, To: "in_progress"}); err != nil {
		logIt("resume denied: %v", err)
	}
}

// GOOD7: the same for a system-class caller.
func (s *svc) good7(ctx context.Context, itemID string) {
	_, err := s.storage.GatedTransition(ctx, session.TransitionRequest{Caller: session.CallerSessionExited, ItemID: itemID, To: "review"})
	if err != nil {
		logIt("session exit transition denied: %v", err)
	}
}

// GOOD8: a log-only Raw* write with a justification — no diagnostic.
func (s *svc) good8(ctx context.Context, itemID string) {
	if _, err := s.storage.RawTransitionBacklogItemStatus(ctx, itemID, "review", nil, "system"); err != nil { //nolint:silenttransition retried on the next sweep tick
		logIt("transition failed: %v", err)
	}
}

func (s *svc) notifyTransitionFailed(itemID string, err error) {
	s.bus.Publish(fmt.Sprintf("%s: %v", itemID, err))
}

// OLDNAME: the pre-1.5.4 name left the target set (Task 1.5.9a), so a log-only
// error from it is no longer this analyzer's business (the inventory test owns it).
func (s *svc) oldNameNotWatched(ctx context.Context, itemID string) {
	if _, err := s.storage.TransitionBacklogItemStatus(ctx, itemID, "done", nil, "system"); err != nil {
		logIt("ignored: %v", err)
	}
}

// BAD-DISCARD: a bare call and blank-assigned errors are flagged.
func (s *svc) badDiscarded(ctx context.Context, itemID string) {
	s.storage.UpdateItemSessionEnded(ctx, itemID, 0)                                                                           // want `error from UpdateItemSessionEnded is discarded`
	_ = s.storage.UpdateItemSessionEnded(ctx, itemID, 0)                                                                       // want `error from UpdateItemSessionEnded is discarded`
	_, _ = s.storage.RawTransitionBacklogItemStatus(ctx, itemID, "done", nil, "system")                                        // want `error from RawTransitionBacklogItemStatus is discarded`
	_, _ = s.storage.GatedTransition(ctx, session.TransitionRequest{Caller: session.CallerOperatorArchiveRPC, ItemID: itemID}) // want `error from GatedTransition is discarded`
}

// GOOD-DISCARD: a justified discard is allowed.
func (s *svc) goodDiscarded(ctx context.Context, itemID string) {
	_ = s.storage.UpdateItemSessionEnded(ctx, itemID, 0) //nolint:silenttransition bookkeeping timestamp only
}
