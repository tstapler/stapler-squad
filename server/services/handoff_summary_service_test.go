package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/handoffsummary"
)

// newTestHandoffSummaryEntClient constructs a fresh in-memory sqlite-backed
// *ent.Client for tests, mirroring newTestSessionSummaryEntClient
// (session_summary_service_test.go).
func newTestHandoffSummaryEntClient(t *testing.T) *ent.Client {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	return repo.GetEntClient()
}

// getHandoffRow fetches the HandoffSummary row for sessionID, failing the test
// if it does not exist.
func getHandoffRow(t *testing.T, client *ent.Client, sessionID string) *ent.HandoffSummary {
	t.Helper()
	row, err := client.HandoffSummary.Query().Where(handoffsummary.SessionID(sessionID)).Only(context.Background())
	require.NoError(t, err)
	return row
}

// writeHandoffConversationFixture points $HOME at a fresh temp directory and
// writes a per-session conversation JSONL file at the exact path
// HandoffSummaryGenerator.GenerateAndPersist reads from
// ($HOME/.claude/projects/<any-dir>/<file>.jsonl). Duplicated from
// session/handoff_summary_service_test.go's unexported helper of the same
// shape, since it isn't reachable from this package.
//
// Callers must not also call t.Parallel() — t.Setenv forbids parallel tests.
func writeHandoffConversationFixture(t *testing.T, sessionID string) {
	t.Helper()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	projectDir := filepath.Join(tempHome, ".claude", "projects", "test-project")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	path := filepath.Join(projectDir, sessionID+".jsonl")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	roles := []string{"user", "assistant", "user", "assistant"}
	contents := []string{
		"Please add input validation to the signup form.",
		"Sure — I'll start by looking at the existing form component.",
		"Also make sure email format is checked.",
		"Added a regex-based email check in validateSignup.go.",
	}
	for i, role := range roles {
		entry := map[string]any{
			"type":      role,
			"uuid":      uuid.New().String(),
			"sessionId": sessionID,
			"timestamp": time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			"cwd":       tempHome,
			"message": map[string]any{
				"role":    role,
				"content": contents[i],
			},
		}
		line, err := json.Marshal(entry)
		require.NoError(t, err)
		_, err = f.Write(append(line, '\n'))
		require.NoError(t, err)
	}
}

// ---- GetHandoffSummary ----

func TestGetHandoffSummary_ReturnsNilWhenNoRowExists(t *testing.T) {
	t.Parallel()
	entClient := newTestHandoffSummaryEntClient(t)
	gen := session.NewHandoffSummaryGenerator(entClient, &fakePoolClient{})
	svc := NewHandoffSummaryService(gen)

	resp, err := svc.GetHandoffSummary(context.Background(), connect.NewRequest(&sessionv1.GetHandoffSummaryRequest{SessionId: "does-not-exist"}))
	require.NoError(t, err)
	require.Nil(t, resp.Msg.Summary)
}

func TestGetHandoffSummary_ReconcilesStaleGeneratingRowToError_When_PastStaleTimeout(t *testing.T) {
	t.Parallel()
	entClient := newTestHandoffSummaryEntClient(t)
	gen := session.NewHandoffSummaryGenerator(entClient, &fakePoolClient{})
	svc := NewHandoffSummaryService(gen)

	sessionID := "sess-stale"
	staleStart := time.Now().Add(-10 * time.Minute) // past the 5-minute staleGenerationTimeout
	_, err := entClient.HandoffSummary.Create().
		SetID("row-stale").
		SetSessionID(sessionID).
		SetSessionTitle("stale title").
		SetStatus(string(session.HandoffSummaryStatusGenerating)).
		SetGenerationStartedAt(staleStart).
		Save(context.Background())
	require.NoError(t, err)

	resp, err := svc.GetHandoffSummary(context.Background(), connect.NewRequest(&sessionv1.GetHandoffSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	require.Equal(t, sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_ERROR, resp.Msg.Summary.Status)
	require.Equal(t, "stale", resp.Msg.Summary.ErrorStage)
	require.Equal(t, "generation did not complete (server restart or hung call)", resp.Msg.Summary.ErrorMessage)

	// Confirm the write actually landed, not just the in-memory response.
	row := getHandoffRow(t, entClient, sessionID)
	require.Equal(t, string(session.HandoffSummaryStatusError), row.Status)
	require.Equal(t, "stale", row.ErrorStage)
}

func TestGetHandoffSummary_LeavesFreshGeneratingRowUnchanged(t *testing.T) {
	t.Parallel()
	entClient := newTestHandoffSummaryEntClient(t)
	gen := session.NewHandoffSummaryGenerator(entClient, &fakePoolClient{})
	svc := NewHandoffSummaryService(gen)

	sessionID := "sess-fresh-generating"
	_, err := entClient.HandoffSummary.Create().
		SetID("row-fresh").
		SetSessionID(sessionID).
		SetSessionTitle("fresh title").
		SetStatus(string(session.HandoffSummaryStatusGenerating)).
		SetGenerationStartedAt(time.Now()).
		Save(context.Background())
	require.NoError(t, err)

	resp, err := svc.GetHandoffSummary(context.Background(), connect.NewRequest(&sessionv1.GetHandoffSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	require.Equal(t, sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_GENERATING, resp.Msg.Summary.Status)
}

// ---- TriggerHandoffSummary ----

func TestTriggerHandoffSummary_ReturnsFailedPreconditionWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)
	configPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"handoff_summary": {"enabled": false}}`), 0600))

	entClient := newTestHandoffSummaryEntClient(t)
	gen := session.NewHandoffSummaryGenerator(entClient, &fakePoolClient{})
	svc := NewHandoffSummaryService(gen)

	sessionID := "sess-disabled"
	_, err := svc.TriggerHandoffSummary(context.Background(), connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: sessionID}))
	require.Error(t, err)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	// No row should have been created or dispatched.
	_, findErr := gen.FindRowBySessionID(context.Background(), sessionID)
	require.ErrorIs(t, findErr, session.ErrNotFound)
}

// TestTriggerHandoffSummary_DispatchesAsyncAndReturnsGeneratingRow covers the
// core async-dispatch contract: TriggerHandoffSummary must not block on the
// LLM call, and must reflect the current GENERATING row rather than the
// eventual READY/ERROR outcome. A first generation is started directly and
// blocked mid-pool-call (mirroring
// TestRegenerateSessionSummary_should_NotTriggerSecondPipeline_When_AlreadyGenerating
// in session_summary_service_test.go) so the row is deterministically already
// GENERATING — with the in-flight guard held — before TriggerHandoffSummary is
// called, removing the race between GenerateAndPersist's interim upsert and
// this test's own read.
func TestTriggerHandoffSummary_DispatchesAsyncAndReturnsGeneratingRow(t *testing.T) {
	sessionID := "sess-trigger-generating"
	writeHandoffConversationFixture(t, sessionID)

	entClient := newTestHandoffSummaryEntClient(t)
	pool := &fakePoolClient{block: true, started: make(chan struct{}, 1)}
	gen := session.NewHandoffSummaryGenerator(entClient, pool)
	svc := NewHandoffSummaryService(gen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		gen.GenerateAndPersist(ctx, sessionID, "title")
	}()

	select {
	case <-pool.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first generation to reach the pool call")
	}
	require.Equal(t, 1, pool.callCount())

	start := time.Now()
	resp, err := svc.TriggerHandoffSummary(context.Background(), connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: sessionID}))
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Less(t, elapsed, 500*time.Millisecond, "TriggerHandoffSummary must return immediately, not block on the LLM call")
	require.NotNil(t, resp.Msg.Summary)
	require.Equal(t, sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_GENERATING, resp.Msg.Summary.Status)

	// The dedup guard inside GenerateAndPersist must have rejected the second
	// (TriggerHandoffSummary-dispatched) goroutine outright — no second pool call.
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, 1, pool.callCount(), "expected the dedup guard to reject the second GenerateAndPersist call")

	cancel()
	<-firstDone
}

// TestTriggerHandoffSummary_SynthesizesPendingResponse_When_NoRowExistsYet
// covers the narrow race documented on TriggerHandoffSummary: if no prior row
// exists, the handler dispatches generation but does not wait for the
// interim GENERATING upsert to land before its own read — so a not-found
// result must produce a synthesized PENDING response, not an error.
//
// Note: TriggerHandoffSummary always dispatches GenerateAndPersist with
// context.Background(), detached from this test's own context (by design —
// see the handler's doc comment), so this test cannot cancel it. $HOME is
// isolated to an empty temp dir (no conversation fixture) purely so the
// dispatched goroutine's transcript lookup fails fast and deterministically
// instead of scanning whatever real ~/.claude/projects data happens to exist
// on the machine running the test; the test then waits for that goroutine to
// reach a terminal state before returning, so it can't race this test's own
// ent-client cleanup.
func TestTriggerHandoffSummary_SynthesizesPendingResponse_When_NoRowExistsYet(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	entClient := newTestHandoffSummaryEntClient(t)
	gen := session.NewHandoffSummaryGenerator(entClient, &fakePoolClient{})
	svc := NewHandoffSummaryService(gen)

	sessionID := "sess-no-prior-row"
	resp, err := svc.TriggerHandoffSummary(context.Background(), connect.NewRequest(&sessionv1.TriggerHandoffSummaryRequest{SessionId: sessionID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Summary)
	require.Equal(t, sessionID, resp.Msg.Summary.SessionId)

	// Either outcome is contractually valid the instant the handler returns
	// (see TriggerHandoffSummary's doc comment): the interim GENERATING upsert
	// may or may not have landed yet. What must never happen is an error or a
	// terminal (READY/ERROR) status this early.
	require.Contains(t,
		[]sessionv1.HandoffSummaryStatus{
			sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_PENDING,
			sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_GENERATING,
		},
		resp.Msg.Summary.Status,
	)

	deadline := time.Now().Add(2 * time.Second)
	for {
		row, findErr := gen.FindRowBySessionID(context.Background(), sessionID)
		if findErr == nil && row.Status == string(session.HandoffSummaryStatusError) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for the dispatched generation to reach a terminal state (last err=%v)", findErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
