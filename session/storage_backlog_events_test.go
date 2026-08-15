package session_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgevents "github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// This file covers the Epic 2.2 publish hooks that live in
// session/storage_backlog.go (Stories 2.2.3, 2.2.4, 2.2.5). It lives in the
// external session_test package for the same reason
// ent_repository_backlog_events_test.go does: these tests need to wire the
// real server/services.BacklogItemEventPublisher adapter, which imports
// session, so an internal `package session` test file cannot import it
// without an import-cycle build error.

// createBacklogItemForEvents seeds a minimal BacklogItem for the tests below.
func createBacklogItemForEvents(t *testing.T, repo *session.EntRepository, ctx context.Context) *session.BacklogItemData {
	t.Helper()
	item, err := repo.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "item for storage_backlog event test",
		Status: string(session.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	return item
}

// TestCreateItemSession_should_publishSessionAttachedWithSessionID_When_SessionIsCreated
// asserts CreateItemSession publishes ChangeSessionAttached with the correct
// SessionID (Task 2.2.4a/c, R8 happy path).
func TestCreateItemSession_should_publishSessionAttachedWithSessionID_When_SessionIsCreated(t *testing.T) {
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item := createBacklogItemForEvents(t, repo, ctx)
	sessionUUID := uuid.NewString()

	_, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: "work",
	})
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeSessionAttached, ev.BacklogItemPayload.Kind)
		assert.Equal(t, sessionUUID, ev.BacklogItemPayload.SessionID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestUpdateItemSessionSessionUUID_should_notPublish_When_ItemSessionIDDoesNotExist
// confirms calling UpdateItemSessionSessionUUID with a non-existent
// itemSessionID returns an error and never reaches the publish call (Task
// 2.2.4b, R8 error path).
func TestUpdateItemSessionSessionUUID_should_notPublish_When_ItemSessionIDDoesNotExist(t *testing.T) {
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	err := repo.UpdateItemSessionSessionUUID(ctx, uuid.NewString(), uuid.NewString())
	require.Error(t, err)

	select {
	case ev := <-sub:
		t.Fatalf("expected no event when the item session does not exist, got: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event within the timeout.
	}
}

// TestDequeueSweep_should_publishTwoSessionAttachedEvents_When_CreateThenUpdateUUIDBothRun
// mirrors the real dequeue-sweep call sequence: CreateItemSession followed
// later by UpdateItemSessionSessionUUID. Asserts a subscriber sees two
// ChangeSessionAttached events in order, each with the correct SessionID/UUID
// (Task 2.2.4c, R8 integration).
func TestDequeueSweep_should_publishTwoSessionAttachedEvents_When_CreateThenUpdateUUIDBothRun(t *testing.T) {
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item := createBacklogItemForEvents(t, repo, ctx)
	placeholderUUID := uuid.NewString()

	created, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: placeholderUUID,
		SessionRole: "work",
	})
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeSessionAttached, ev.BacklogItemPayload.Kind)
		assert.Equal(t, placeholderUUID, ev.BacklogItemPayload.SessionID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first BacklogItemChanged event")
	}

	tmuxUUID := uuid.NewString()
	err = repo.UpdateItemSessionSessionUUID(ctx, created.ID, tmuxUUID)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeSessionAttached, ev.BacklogItemPayload.Kind)
		assert.Equal(t, tmuxUUID, ev.BacklogItemPayload.SessionID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second BacklogItemChanged event")
	}
}

// TestSaveReviewVerdict_should_publishVerdictInlineOnPayload_When_VerdictIsSaved
// asserts SaveReviewVerdict publishes ChangeVerdictRecorded with the verdict
// populated directly from the write, not derived by re-reading item_sessions
// (Task 2.2.3a/c, R7 happy path).
func TestSaveReviewVerdict_should_publishVerdictInlineOnPayload_When_VerdictIsSaved(t *testing.T) {
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item := createBacklogItemForEvents(t, repo, ctx)
	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "work",
	})
	require.NoError(t, err)

	// Drain the CreateItemSession publish before exercising SaveReviewVerdict.
	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's BacklogItemChanged event")
	}

	err = repo.SaveReviewVerdict(ctx, is.ID, session.ReviewVerdictData{
		OverallOutcome: session.ReviewOutcomePass,
		Summary:        "looks good",
	})
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeVerdictRecorded, ev.BacklogItemPayload.Kind)
		require.NotNil(t, ev.BacklogItemPayload.Verdict)
		assert.Equal(t, session.ReviewOutcomePass, ev.BacklogItemPayload.Verdict.OverallOutcome)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestSaveReviewVerdict_should_notPublish_When_UnderlyingWriteFails simulates a
// write failure (a malformed itemSessionID that fails uuid.Parse before any
// DB write happens) and asserts the method returns its error and no event is
// published (Task 2.2.3a, R7 error path).
func TestSaveReviewVerdict_should_notPublish_When_UnderlyingWriteFails(t *testing.T) {
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	err := repo.SaveReviewVerdict(ctx, "not-a-valid-uuid", session.ReviewVerdictData{
		OverallOutcome: session.ReviewOutcomePass,
	})
	require.Error(t, err)

	select {
	case ev := <-sub:
		t.Fatalf("expected no event when the underlying write fails, got: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// Expected: no event within the timeout.
	}
}

// TestVerdictRecordingPaths_should_convergeOnSameEventKind_When_RPCPathAndMCPPathBothRun
// exercises both verdict-recording entry points — SaveReviewVerdict (the RPC
// path) and CreateItemSessionWithVerdict (the MCP submit_review_verdict path)
// — against a real bus subscriber and asserts both produce
// ChangeVerdictRecorded events with correctly populated Verdict fields (Task
// 2.2.3c, R7 integration).
func TestVerdictRecordingPaths_should_convergeOnSameEventKind_When_RPCPathAndMCPPathBothRun(t *testing.T) {
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	// RPC path: SaveReviewVerdict against an existing ItemSession.
	rpcItem := createBacklogItemForEvents(t, repo, ctx)
	rpcSession, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      rpcItem.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "work",
	})
	require.NoError(t, err)
	select { // drain CreateItemSession's own publish
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's BacklogItemChanged event")
	}

	err = repo.SaveReviewVerdict(ctx, rpcSession.ID, session.ReviewVerdictData{
		OverallOutcome: session.ReviewOutcomePass,
		Summary:        "rpc path verdict",
	})
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeVerdictRecorded, ev.BacklogItemPayload.Kind)
		require.NotNil(t, ev.BacklogItemPayload.Verdict)
		assert.Equal(t, session.ReviewOutcomePass, ev.BacklogItemPayload.Verdict.OverallOutcome)
		assert.Equal(t, "rpc path verdict", ev.BacklogItemPayload.Verdict.Summary)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for RPC-path BacklogItemChanged event")
	}

	// MCP path: CreateItemSessionWithVerdict combines both writes.
	mcpItem := createBacklogItemForEvents(t, repo, ctx)
	_, err = repo.CreateItemSessionWithVerdict(ctx,
		session.ItemSessionData{
			ItemID:      mcpItem.ID,
			SessionUUID: uuid.NewString(),
			SessionRole: "review",
		},
		session.ReviewVerdictData{
			OverallOutcome: session.ReviewOutcomeFail,
			Summary:        "mcp path verdict",
		},
	)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeVerdictRecorded, ev.BacklogItemPayload.Kind)
		require.NotNil(t, ev.BacklogItemPayload.Verdict)
		assert.Equal(t, session.ReviewOutcomeFail, ev.BacklogItemPayload.Verdict.OverallOutcome)
		assert.Equal(t, "mcp path verdict", ev.BacklogItemPayload.Verdict.Summary)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for MCP-path BacklogItemChanged event")
	}
}

// TestUpdateItemSessionTriageResult_should_publishTriageProgressUpdated_When_TriageResultIsSaved
// seeds a BacklogItem + ItemSession linked via the backlog_item edge,
// subscribes to a real bus, calls UpdateItemSessionTriageResult, and asserts
// the received event's Kind/UpdatedFields/Item.ID (Task 2.2.5b, mirroring
// 2.2.3c/2.2.4c's style).
func TestUpdateItemSessionTriageResult_should_publishTriageProgressUpdated_When_TriageResultIsSaved(t *testing.T) {
	repo, cleanup := newTestEntRepositoryForEvents(t)
	defer cleanup()
	ctx := context.Background()

	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})

	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item := createBacklogItemForEvents(t, repo, ctx)
	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "triage",
	})
	require.NoError(t, err)

	// Drain the CreateItemSession publish before exercising the triage hook.
	select {
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's BacklogItemChanged event")
	}

	err = repo.UpdateItemSessionTriageResult(ctx, is.ID, `{"summary":"triage in progress"}`)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		require.NotNil(t, ev.BacklogItemPayload)
		assert.Equal(t, pkgevents.BacklogChangeTriageProgressUpdated, ev.BacklogItemPayload.Kind)
		assert.Equal(t, []string{"triageResultSummary"}, ev.BacklogItemPayload.UpdatedFields)
		require.NotNil(t, ev.BacklogItemPayload.Item)
		assert.Equal(t, item.ID, ev.BacklogItemPayload.Item.ID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for BacklogItemChanged event")
	}
}

// TestUpdateItemSessionTriageResult_should_stillSucceed_When_OwningItemLookupFails
// breaks an ItemSession's backlog_item edge via a second raw connection with
// foreign-key enforcement disabled (simulating the owning item having been
// removed out from under the session without going through
// DeleteBacklogItem's cascading cleanup — the edge case Story 2.2.5's AC
// describes) and asserts UpdateItemSessionTriageResult still returns success
// for the triage_result write itself, with the publish step silently skipped
// rather than failing the call (Task 2.2.5a).
func TestUpdateItemSessionTriageResult_should_stillSucceed_When_OwningItemLookupFails(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("test-%d.db", time.Now().UnixNano()))

	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer func() {
		repo.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()

	ctx := context.Background()
	bus := pkgevents.NewEventBus(10)
	repo.SetItemChangePublisher(&services.BacklogItemEventPublisher{Bus: bus})
	sub, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	item := createBacklogItemForEvents(t, repo, ctx)
	is, err := repo.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: uuid.NewString(),
		SessionRole: "triage",
	})
	require.NoError(t, err)

	select { // drain CreateItemSession's own publish
	case <-sub:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CreateItemSession's BacklogItemChanged event")
	}

	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	defer rawDB.Close()
	_, err = rawDB.Exec("PRAGMA foreign_keys = OFF")
	require.NoError(t, err)
	_, err = rawDB.Exec("UPDATE item_sessions SET backlog_item_item_sessions = ? WHERE id = ?", uuid.NewString(), is.ID)
	require.NoError(t, err)

	err = repo.UpdateItemSessionTriageResult(ctx, is.ID, `{"summary":"orphaned session"}`)
	require.NoError(t, err)

	select {
	case ev := <-sub:
		t.Fatalf("expected no publish when the owning item lookup fails, got: %+v", ev)
	case <-time.After(200 * time.Millisecond):
		// Expected: publish is skipped (logged, not fatal), but the write above
		// already returned success.
	}
}
