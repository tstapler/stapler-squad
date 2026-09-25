package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	entErrorEvent "github.com/tstapler/stapler-squad/session/ent/errorevent"
)

func TestErrorRegistry_Record_DedupesAcrossDynamicMessageDetail(t *testing.T) {
	repo := session.NewTestEntRepository(t)
	client := repo.GetEntClient()
	registry := NewErrorRegistry(client, true)
	ctx := context.Background()

	registry.Record(ctx, errors.New("session a1b2c3d4-e5f6-7890-abcd-ef1234567890 not found"), "GetSession")
	registry.Record(ctx, errors.New("session ffffffff-1111-2222-3333-444444444444 not found"), "GetSession")

	events, err := registry.List(ctx, true)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected the two errors (differing only by embedded UUID) to dedupe into one event, got %d", len(events))
	}
	if events[0].OccurrenceCount != 2 {
		t.Fatalf("expected occurrence_count 2, got %d", events[0].OccurrenceCount)
	}
}

func TestErrorRegistry_Record_PrunesStaleEvents(t *testing.T) {
	repo := session.NewTestEntRepository(t)
	client := repo.GetEntClient()
	registry := NewErrorRegistry(client, true)
	ctx := context.Background()

	old := time.Now().Add(-60 * 24 * time.Hour)
	_, err := client.ErrorEvent.Create().
		SetFingerprint("stale-fingerprint").
		SetErrorType("rpc_error").
		SetMessage("an old error").
		SetStackTrace("").
		SetOccurrenceCount(1).
		SetFirstSeen(old).
		SetLastSeen(old).
		Save(ctx)
	if err != nil {
		t.Fatalf("seeding stale event: %v", err)
	}

	registry.Record(ctx, errors.New("a fresh error"), "SomeProcedure")

	assertEventCount(t, ctx, client.ErrorEvent.Query().Where(entErrorEvent.FingerprintEQ("stale-fingerprint")), 0, "stale event should have been pruned")
	assertEventCount(t, ctx, client.ErrorEvent.Query(), 1, "only the fresh event should remain")
}

func assertEventCount(t *testing.T, ctx context.Context, q *ent.ErrorEventQuery, want int, msg string) {
	t.Helper()
	got, err := q.Count(ctx)
	if err != nil {
		t.Fatalf("%s: count query failed: %v", msg, err)
	}
	if got != want {
		t.Fatalf("%s: got %d rows, want %d", msg, got, want)
	}
}
