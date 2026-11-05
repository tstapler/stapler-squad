package services

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func TestAnnotateDuplicatePending_UsesPreloadedSessions(t *testing.T) {
	sessions := []session.ItemSessionSummary{{VerificationNotes: "duplicate_ref=abc123 reason"}}

	review := &sessionv1.BacklogItem{Status: string(session.BacklogStatusReview)}
	annotateDuplicatePending(review, sessions)
	if !review.DuplicatePending || review.DuplicateRef != "abc123" {
		t.Fatalf("review item: pending=%v ref=%q", review.DuplicatePending, review.DuplicateRef)
	}

	other := &sessionv1.BacklogItem{Status: string(session.BacklogStatusInProgress)}
	annotateDuplicatePending(other, sessions)
	if other.DuplicatePending {
		t.Fatal("non-review item must not be marked pending")
	}
}
