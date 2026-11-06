package session

// diagnose_nudge_dispatch_guard_test.go is the regression suite for gap #4:
// prior to Storage.ClaimDiagnoseNudgeAttempt, there was no "has this specific
// diagnostic dispatch already attempted its one nudge write" concept anywhere
// — only the item-level diagnose_nudge_count cap (diagnose_nudge.go), which
// bounds total nudges per item across many dispatches but does not stop a
// SINGLE dispatch from writing twice if its own LLM retries after an
// ambiguous MCP tool response.

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClaimDiagnoseNudgeAttempt_SecondClaimFails proves the guard's core
// invariant: the same dispatched session's second claim attempt fails even
// though nothing about the item-level cap changed.
func TestClaimDiagnoseNudgeAttempt_SecondClaimFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage, sessUUID := newItemSessionRoleFixture(t, SessionRoleDiagnose, "dup-write-guard item")

	first, err := storage.ClaimDiagnoseNudgeAttempt(ctx, sessUUID)
	require.NoError(t, err)
	assert.True(t, first, "the first attempt from a fresh dispatch must succeed")

	second, err := storage.ClaimDiagnoseNudgeAttempt(ctx, sessUUID)
	require.NoError(t, err)
	assert.False(t, second, "a second attempt from the SAME dispatched session must be refused")
}

// TestClaimDiagnoseNudgeAttempt_NonDiagnoseRole_NeverClaims proves the claim
// only ever applies to a diagnose-role ItemSession — a work/review/triage
// session (which never calls diagnose_nudge_session at all) must not somehow
// pick up a stray claim row.
func TestClaimDiagnoseNudgeAttempt_NonDiagnoseRole_NeverClaims(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage, sessUUID := newItemSessionRoleFixture(t, SessionRoleWork, "work session item")

	claimed, err := storage.ClaimDiagnoseNudgeAttempt(ctx, sessUUID)
	require.NoError(t, err)
	assert.False(t, claimed, "a non-diagnose-role session must never be able to claim a nudge attempt")
}

// TestClaimDiagnoseNudgeAttempt_ExactlyOneWinsConcurrently is the -race
// regression proving the claim itself is atomic: N goroutines racing to
// claim the same dispatched session's one nudge attempt must produce exactly
// one winner, never zero and never more than one. Run with `go test -race`.
func TestClaimDiagnoseNudgeAttempt_ExactlyOneWinsConcurrently(t *testing.T) {
	storage, sessUUID := newItemSessionRoleFixture(t, SessionRoleDiagnose, "concurrent claim item")

	const concurrentAttempts = 10
	var wg sync.WaitGroup
	results := make([]bool, concurrentAttempts)
	errs := make([]error, concurrentAttempts)
	wg.Add(concurrentAttempts)
	for i := 0; i < concurrentAttempts; i++ {
		go func(idx int) {
			defer wg.Done()
			claimed, claimErr := storage.ClaimDiagnoseNudgeAttempt(context.Background(), sessUUID)
			results[idx] = claimed
			errs[idx] = claimErr
		}(i)
	}
	wg.Wait()

	wins := 0
	for i, claimed := range results {
		require.NoError(t, errs[i])
		if claimed {
			wins++
		}
	}
	assert.Equal(t, 1, wins, "exactly one concurrent claim attempt must win")
}
