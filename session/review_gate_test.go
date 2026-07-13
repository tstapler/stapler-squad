package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/headless"
)

// TestReviewGateRunner_SkipReviewGate verifies that Run returns immediately
// without calling the pool or onPass when item.SkipReviewGate is true.
func TestReviewGateRunner_SkipReviewGate(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	// Construct an item with SkipReviewGate set — pool and onPass must not be called.
	item := &BacklogItemData{
		ID:             uuid.New().String(),
		RepoPath:       "/some/repo",
		SkipReviewGate: true,
	}
	is := ItemSessionSummary{
		ID:          uuid.New().String(),
		SessionUUID: uuid.New().String(),
	}

	var poolCalled atomic.Bool
	var onPassCalled atomic.Bool

	// If the pool is consulted, panic so the test fails loudly.
	getPool := func() *headless.Pool {
		poolCalled.Store(true)
		return nil
	}
	getAutoReopener := func() AutoReopenSpawner { return nil }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, nil)

	runner.Run(context.Background(), item, is, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.False(t, poolCalled.Load(), "pool getter must not be consulted when SkipReviewGate is true")
	assert.False(t, onPassCalled.Load(), "onPass must not be called when SkipReviewGate is true")
}

// TestReviewGateRunner_HeadlessPassPath verifies the happy path where the headless
// pool returns a PASS verdict: onPass is called and a review ItemSession with a
// PASS verdict is persisted to storage.
func TestReviewGateRunner_HeadlessPassPath(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	// Persist a BacklogItem so CreateItemSessionWithVerdict can satisfy its FK.
	itemData := BacklogItemData{
		Title:              "Headless Pass Test",
		Description:        "Testing the headless PASS path",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           t.TempDir(), // non-git dir; GetGitDiff will error gracefully
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	// Persist a work ItemSession so the runner can look it up.
	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      createdItemData.ID,
		SessionUUID: workSessionUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	item := &BacklogItemData{
		ID:       createdItemData.ID,
		RepoPath: createdItemData.RepoPath,
	}

	// Build the JSON response expected by pool.CallBlockingWithCost.
	// The outer envelope is firstCallJSONResult; its "result" field contains the
	// verdict JSON that ParseHeadlessVerdictResult will parse.
	verdictJSON := `{"overall":"PASS","summary":"all criteria met","verdicts":[]}`
	verdictJSONEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	fakeResponse := fmt.Sprintf(`{"session_id":"test-s1","result":%s,"cost_usd":0.01}`, verdictJSONEncoded)

	fakeRunner := headless.NewFakeRunner(fakeResponse)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, nil)

	var onPassCalled atomic.Bool
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
		onPassCalled.Store(true)
	})

	assert.True(t, onPassCalled.Load(), "onPass must be called when headless pool returns PASS")
	assert.Equal(t, 1, fakeRunner.CallCount(), "pool must be called exactly once")
}

// TestReviewGateRunner_ThreadsVerificationNotesIntoPrompt verifies that verification
// evidence recorded on the work session (via request_review's verification_notes
// argument) reaches the headless reviewer prompt, not just the diff and AC list.
// This is the regression guard for the UNVERIFIABLE-despite-real-verification gap:
// criteria describing test runs or manual UI checks are invisible in the diff, so the
// reviewer's only window into that evidence is this threaded-through text.
func TestReviewGateRunner_ThreadsVerificationNotesIntoPrompt(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	itemData := BacklogItemData{
		Title:              "Verification Notes Threading Test",
		Description:        "Testing that verification_notes reaches the reviewer prompt",
		AcceptanceCriteria: `[]`,
		Priority:           1,
		Status:             string(BacklogStatusInProgress),
		RepoPath:           t.TempDir(),
	}
	createdItemData, err := storage.CreateBacklogItem(ctx, itemData)
	require.NoError(t, err)

	verificationNotes := "ran `go test ./session/...` -> ok (41 tests); confirmed via UI that sessions group under Category=Backlog"

	workSessionUUID := uuid.New().String()
	workIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:            createdItemData.ID,
		SessionUUID:       workSessionUUID,
		SessionRole:       SessionRoleWork,
		VerificationNotes: verificationNotes,
	})
	require.NoError(t, err)

	item := &BacklogItemData{
		ID:       createdItemData.ID,
		RepoPath: createdItemData.RepoPath,
	}

	verdictJSON := `{"overall":"PASS","summary":"all criteria met","verdicts":[]}`
	verdictJSONEncoded, marshalErr := json.Marshal(verdictJSON)
	require.NoError(t, marshalErr)
	fakeResponse := fmt.Sprintf(`{"session_id":"test-s1","result":%s,"cost_usd":0.01}`, verdictJSONEncoded)

	fakeRunner := headless.NewFakeRunner(fakeResponse)
	pool := headless.NewPoolWithRunner(headless.PoolConfig{
		MaxCallsPerSession:    1,
		MaxConcurrentSessions: 1,
	}, fakeRunner)

	getPool := func() *headless.Pool { return pool }
	getAutoReopener := func() AutoReopenSpawner { return nil }

	runner := NewReviewGateRunner(storage, getPool, getAutoReopener, nil)
	runner.Run(ctx, item, workIS, func(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {})

	require.Equal(t, 1, fakeRunner.CallCount(), "pool must be called exactly once")

	// The user prompt is passed via stdin (not args) so it doesn't leak into
	// /proc/<pid>/cmdline — see Pool.call in headless/caller.go.
	prompt := fakeRunner.StdinForCall(0)
	assert.True(t,
		strings.Contains(prompt, "Verification Evidence") && strings.Contains(prompt, "Category=Backlog"),
		"reviewer prompt must contain the labeled Verification Evidence section with the work session's reported notes; got prompt: %s", prompt)
}
