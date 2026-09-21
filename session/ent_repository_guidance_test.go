package session

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

// createTestBacklogItemForGuidance creates a minimal BacklogItem to link a
// backlog-item-scoped GuidanceRequest against, returning its parsed UUID.
func createTestBacklogItemForGuidance(t *testing.T, repo *EntRepository) uuid.UUID {
	t.Helper()
	created, err := repo.CreateBacklogItem(context.Background(), BacklogItemData{Title: "guidance test item"})
	require.NoError(t, err)
	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	return id
}

// TestGuidanceRequestData_Status_should_ReturnPending_When_AnsweredAtAndCancelledAtAreNil
// is the pure, no-DB happy path for the derived-status accessor.
func TestGuidanceRequestData_Status_should_ReturnPending_When_AnsweredAtAndCancelledAtAreNil(t *testing.T) {
	t.Parallel()
	data := GuidanceRequestData{}
	assert.Equal(t, GuidanceRequestStatusPending, data.Status())
}

// TestGuidanceRequestData_Status_should_ReturnCancelled_When_CancelledAtSetEvenIfAnsweredAtAlsoSet
// confirms cancelled is terminal and dominates over answered per the Domain
// Glossary's "cancelled, terminal" rule.
func TestGuidanceRequestData_Status_should_ReturnCancelled_When_CancelledAtSetEvenIfAnsweredAtAlsoSet(t *testing.T) {
	t.Parallel()
	now := time.Now()
	data := GuidanceRequestData{AnsweredAt: &now, CancelledAt: &now}
	assert.Equal(t, GuidanceRequestStatusCancelled, data.Status())
}

// TestGuidanceRequestData_Status_should_ReturnAnswered_When_OnlyAnsweredAtSet
// rounds out the third derived-status branch.
func TestGuidanceRequestData_Status_should_ReturnAnswered_When_OnlyAnsweredAtSet(t *testing.T) {
	t.Parallel()
	now := time.Now()
	data := GuidanceRequestData{AnsweredAt: &now}
	assert.Equal(t, GuidanceRequestStatusAnswered, data.Status())
}

// TestCreateGuidanceRequest_should_SurviveRepositoryReopen_When_ReadBackAfterRestart
// simulates a full service restart: a GuidanceRequest row is inserted, the
// EntRepository is closed and a fresh one reopened against the SAME on-disk
// SQLite file, and GetGuidanceRequest must return the identical row (AC1).
// Uses a real on-disk file (not NewTestEntRepository's shared-cache in-memory
// DB, which cannot be reopened) since the whole point is to close and reopen.
func TestCreateGuidanceRequest_should_SurviveRepositoryReopen_When_ReadBackAfterRestart(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "guidance-reopen.db")
	ctx := context.Background()

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)

	itemID := createTestBacklogItemForGuidance(t, repo)
	created, err := repo.CreateGuidanceRequest(ctx, CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeBacklogItem,
		ItemID:       &itemID,
		QuestionText: "Should triage merge PR #780 into this item's branch?",
		QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	reopened, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer reopened.Close() //nolint:errcheck

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	fetched, err := reopened.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)

	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, domain.RequestScopeBacklogItem, fetched.Scope)
	assert.Equal(t, itemID, *fetched.ItemID)
	assert.Equal(t, "Should triage merge PR #780 into this item's branch?", fetched.QuestionText)
	assert.Equal(t, domain.QuestionTypeYesNo, fetched.QuestionType)
	assert.Nil(t, fetched.AnsweredAt)
	assert.Equal(t, GuidanceRequestStatusPending, fetched.Status())
}

// backlogItemGuidanceInput builds a CreateGuidanceRequestInput for a
// backlog-item-scoped question, factored out to keep the cap/dedup test
// bodies below under the house function-length gate.
func backlogItemGuidanceInput(itemID uuid.UUID, questionText string, cap int) CreateGuidanceRequestInput {
	return CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeBacklogItem,
		ItemID:       &itemID,
		QuestionText: questionText,
		QuestionType: domain.QuestionTypeYesNo,
		Cap:          cap,
	}
}

// questionTextForGoroutine returns a distinct question text per goroutine
// index, for tests that must NOT trigger the dedup path.
func questionTextForGoroutine(i int) string {
	return "Question " + uuid.New().String()[:8] + "-" + string(rune('0'+i))
}

// classifyGuidanceRequestErr buckets a CreateGuidanceRequest result into one
// of three counters, tallying outcomes across goroutines under one mutex.
func classifyGuidanceRequestErr(err error, successes, capErrors, otherErrors *int) {
	switch {
	case err == nil:
		*successes++
	case errors.Is(err, ErrPendingCapExceeded):
		*capErrors++
	default:
		*otherErrors++
	}
}

// TestCreateGuidanceRequest_ConcurrentCapEnforcement fires 10 concurrent
// CreateGuidanceRequest calls, each with its OWN distinct question_text (so no
// dedup collision occurs between them), against a cap of 4. Exactly 4 must
// succeed and the remaining 6 must fail with ErrPendingCapExceeded — proving
// the cap-check is race-safe, not just correct for a single caller.
func TestCreateGuidanceRequest_ConcurrentCapEnforcement(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)

	const n = 10
	const cap = 4
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes, capErrors, otherErrors := 0, 0, 0

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionTextForGoroutine(i), cap))
			mu.Lock()
			defer mu.Unlock()
			classifyGuidanceRequestErr(err, &successes, &capErrors, &otherErrors)
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 0, otherErrors, "expected no errors other than ErrPendingCapExceeded")
	assert.Equal(t, cap, successes, "expected exactly cap successes")
	assert.Equal(t, n-cap, capErrors, "expected the remainder to be cap-rejected")

	rows, count, gotCap, err := repo.ListPendingGuidanceRequests(ctx, domain.RequestScopeBacklogItem, itemID.String(), cap)
	require.NoError(t, err)
	assert.Len(t, rows, cap)
	assert.Equal(t, cap, count)
	assert.Equal(t, cap, gotCap)
}

// TestCreateGuidanceRequest_ConcurrentDedupCollision fires 10 concurrent
// CreateGuidanceRequest calls with an IDENTICAL question_text (same scope/
// key) against a cap of 4. Exactly 1 row must exist afterward, all 10 calls
// must succeed with that same row's id, and NONE may return
// ErrPendingCapExceeded — proving the dedup-before-cap ordering
// (CreateGuidanceRequest's doc comment, adversarial-review.md Blocker 2).
func TestCreateGuidanceRequest_ConcurrentDedupCollision(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)

	const n = 10
	const cap = 4
	const questionText = "Use approach A or B?"
	var wg sync.WaitGroup
	var mu sync.Mutex
	ids := make(map[string]int)
	capErrors, otherErrors, successes := 0, 0, 0

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionText, cap))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				ids[row.ID]++
			}
			classifyGuidanceRequestErr(err, &successes, &capErrors, &otherErrors)
		}()
	}
	wg.Wait()

	assert.Equal(t, 0, otherErrors, "expected no errors other than ErrPendingCapExceeded")
	assert.Equal(t, 0, capErrors, "dedup must never cap-reject a re-ask of the identical open question")
	require.Len(t, ids, 1, "expected exactly one distinct row id across all 10 calls")

	rows, count, _, err := repo.ListPendingGuidanceRequests(ctx, domain.RequestScopeBacklogItem, itemID.String(), cap)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.Equal(t, 1, count)
}

// TestCreateGuidanceRequest_should_ResolveToExistingRow_When_IdenticalOpenQuestionReAskedAtCap
// exercises the sequential (non-concurrent) shape of the dedup-before-cap
// ordering: with a scope already AT its cap of 4, re-asking one of those same
// 4 open questions (identical question_text) must resolve to the existing row
// — not ErrPendingCapExceeded — since it creates no new row.
func TestCreateGuidanceRequest_should_ResolveToExistingRow_When_IdenticalOpenQuestionReAskedAtCap(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)
	const cap = 4

	var first *GuidanceRequestData
	for i := 0; i < cap; i++ {
		row, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionTextForGoroutine(i), cap))
		require.NoError(t, err)
		if i == 0 {
			first = row
		}
	}

	reAsked, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, first.QuestionText, cap))
	require.NoError(t, err, "re-asking an identical already-open question at cap must resolve, not error")
	assert.Equal(t, first.ID, reAsked.ID)

	rows, count, _, err := repo.ListPendingGuidanceRequests(ctx, domain.RequestScopeBacklogItem, itemID.String(), cap)
	require.NoError(t, err)
	assert.Len(t, rows, cap, "re-asking an existing question must not add a row")
	assert.Equal(t, cap, count)
}

// TestCreateGuidanceRequest_should_RejectWithCapExceeded_When_NewDistinctQuestionAskedAtCap
// is the counterpart scenario: with a scope already AT its cap of 4, a
// GENUINELY NEW distinct question (not matching any open row) must be
// rejected with ErrPendingCapExceeded, and must not leave a 5th row behind
// (the transaction rolls back the just-created row).
func TestCreateGuidanceRequest_should_RejectWithCapExceeded_When_NewDistinctQuestionAskedAtCap(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)
	const cap = 4

	for i := 0; i < cap; i++ {
		_, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionTextForGoroutine(i), cap))
		require.NoError(t, err)
	}

	_, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, "a genuinely new, never-before-asked question", cap))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPendingCapExceeded)

	rows, count, _, err := repo.ListPendingGuidanceRequests(ctx, domain.RequestScopeBacklogItem, itemID.String(), cap)
	require.NoError(t, err)
	assert.Len(t, rows, cap, "the rejected 5th row must have been rolled back, not left behind")
	assert.Equal(t, cap, count)
}

// TestAnswerGuidanceRequest_should_PersistExactlyOneAnswer_When_TwoGoroutinesRaceToAnswerSameRow
// (Task 1.1.4b's "concurrent double-answer" test): two goroutines race to
// answer the same open row with different values. Exactly one must apply.
func TestAnswerGuidanceRequest_should_PersistExactlyOneAnswer_When_TwoGoroutinesRaceToAnswerSameRow(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()

	created, err := repo.CreateGuidanceRequest(ctx, CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeSession,
		SessionUUID:  "7c1e-session",
		QuestionText: "Use approach A or B?",
		QuestionType: domain.QuestionTypeShortAnswer,
	})
	require.NoError(t, err)
	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	var wg sync.WaitGroup
	var mu sync.Mutex
	appliedCount := 0
	wg.Add(2)
	for _, answer := range []string{"yes", "no"} {
		go func(answer string) {
			defer wg.Done()
			applied, err := repo.AnswerGuidanceRequest(ctx, id, answer)
			require.NoError(t, err)
			if applied {
				mu.Lock()
				appliedCount++
				mu.Unlock()
			}
		}(answer)
	}
	wg.Wait()

	assert.Equal(t, 1, appliedCount, "exactly one of the two racing answers must apply")

	fetched, err := repo.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, GuidanceRequestStatusAnswered, fetched.Status())
	assert.Contains(t, []string{"yes", "no"}, fetched.Answer)
}

// TestGuidanceRequest_should_SurviveHardDeleteOfOwningBacklogItem_When_NoCascadeConfigured
// proves the guidance_requests->backlog_items edge has no cascade-delete
// (Task 1.1.1b): hard-deleting the owning BacklogItem must not destroy an
// existing open GuidanceRequest row.
func TestGuidanceRequest_should_SurviveHardDeleteOfOwningBacklogItem_When_NoCascadeConfigured(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)

	created, err := repo.CreateGuidanceRequest(ctx, CreateGuidanceRequestInput{
		Scope:        domain.RequestScopeBacklogItem,
		ItemID:       &itemID,
		QuestionText: "Merge PR #780 first?",
		QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)

	require.NoError(t, repo.DeleteBacklogItem(ctx, itemID.String()))

	fetched, err := repo.GetGuidanceRequest(ctx, id)
	require.NoError(t, err, "GuidanceRequest row must survive its owning BacklogItem's hard delete")
	assert.Equal(t, GuidanceRequestStatusPending, fetched.Status())
}

// TestCreateGuidanceRequest_should_CreateFreshRow_When_IdenticalQuestionReAskedAfterAnswered
// guards the dedup-open-rows-only fix: the (scope, scope_key, question_text)
// index is no longer unique across ALL rows, only queried against OPEN ones
// (upsertGuidanceRequest). Re-asking an identical question after the first
// instance was answered must create a genuinely NEW pending row, not resolve
// back to the stale answered one.
func TestCreateGuidanceRequest_should_CreateFreshRow_When_IdenticalQuestionReAskedAfterAnswered(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)
	const questionText = "Merge PR #780 first?"

	first, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionText, 4))
	require.NoError(t, err)
	firstID, err := uuid.Parse(first.ID)
	require.NoError(t, err)

	applied, err := repo.AnswerGuidanceRequest(ctx, firstID, "yes")
	require.NoError(t, err)
	require.True(t, applied)

	second, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionText, 4))
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID, "re-asking an identical question after the first was answered must create a NEW row, not resolve to the stale answered one")
	assert.Equal(t, GuidanceRequestStatusPending, second.Status())

	// The stale answered row must still be intact and readable, unaffected by
	// the new row's creation.
	refetchedFirst, err := repo.GetGuidanceRequest(ctx, firstID)
	require.NoError(t, err)
	assert.Equal(t, GuidanceRequestStatusAnswered, refetchedFirst.Status())
	assert.Equal(t, "yes", refetchedFirst.Answer)
}

// TestCreateGuidanceRequest_should_CreateFreshRow_When_IdenticalQuestionReAskedAfterCancelled
// is TestCreateGuidanceRequest_should_CreateFreshRow_When_IdenticalQuestionReAskedAfterAnswered's
// counterpart for the cancelled terminal state.
func TestCreateGuidanceRequest_should_CreateFreshRow_When_IdenticalQuestionReAskedAfterCancelled(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)
	const questionText = "Rebase onto main first?"

	first, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionText, 4))
	require.NoError(t, err)
	firstID, err := uuid.Parse(first.ID)
	require.NoError(t, err)

	applied, err := repo.CancelGuidanceRequest(ctx, firstID, "owning session ended")
	require.NoError(t, err)
	require.True(t, applied)

	second, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionText, 4))
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID, "re-asking an identical question after the first was cancelled must create a NEW row")
	assert.Equal(t, GuidanceRequestStatusPending, second.Status())
}

// TestCreateGuidanceRequest_should_CoalesceToExistingRow_When_TwoConcurrentIdenticalOpenQuestions
// is the sequential-shape regression companion to
// TestCreateGuidanceRequest_ConcurrentDedupCollision: it must keep working
// after the dedup-open-rows-only fix — two identical open questions on the
// same scope still coalesce to one row.
func TestCreateGuidanceRequest_should_CoalesceToExistingRow_When_TwoConcurrentIdenticalOpenQuestions(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)
	const questionText = "Use approach A or B?"

	first, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionText, 4))
	require.NoError(t, err)

	second, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionText, 4))
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "two identical OPEN questions for the same scope must coalesce to one row")

	rows, count, _, err := repo.ListPendingGuidanceRequests(ctx, domain.RequestScopeBacklogItem, itemID.String(), 4)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.Equal(t, 1, count)
}

// TestListGuidanceRequestsForScope_should_IncludePendingRow_When_ManyNewerAnsweredRowsWouldPushItOffANaiveWindow
// guards the pending-row-visibility fix: a pending row created before
// listGuidanceRequestsForScopeLimit+1 newer answered rows must still appear
// in the result — a naive single `ORDER BY created_at DESC LIMIT N` query
// over all non-cancelled rows would silently drop it.
func TestListGuidanceRequestsForScope_should_IncludePendingRow_When_ManyNewerAnsweredRowsWouldPushItOffANaiveWindow(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)

	// Create the pending row FIRST, so it's the oldest row for this scope.
	pending, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, "the old still-pending question", 1000))
	require.NoError(t, err)

	// Create and immediately answer enough NEWER rows to exceed
	// listGuidanceRequestsForScopeLimit, so a naive LIMIT-50-over-all-rows
	// query would push the pending row off the page.
	for i := 0; i < listGuidanceRequestsForScopeLimit+5; i++ {
		row, err := repo.CreateGuidanceRequest(ctx, backlogItemGuidanceInput(itemID, questionTextForGoroutine(i), 1000))
		require.NoError(t, err)
		id, err := uuid.Parse(row.ID)
		require.NoError(t, err)
		applied, err := repo.AnswerGuidanceRequest(ctx, id, "answered")
		require.NoError(t, err)
		require.True(t, applied)
	}

	rows, pendingCount, cap, err := repo.ListGuidanceRequestsForScope(ctx, domain.RequestScopeBacklogItem, itemID.String(), 7)
	require.NoError(t, err)
	assert.Equal(t, 1, pendingCount)
	assert.Equal(t, 7, cap)

	var found bool
	for _, r := range rows {
		if r.ID == pending.ID {
			found = true
			assert.Equal(t, GuidanceRequestStatusPending, r.Status())
		}
	}
	assert.True(t, found, "the old still-pending row must still be present in the result despite many newer answered rows")
	assert.LessOrEqual(t, len(rows), 1+listGuidanceRequestsForScopeLimit, "result must include the pending row plus at most listGuidanceRequestsForScopeLimit answered rows")
}

// TestListGuidanceRequestsForScope_should_UseCapParam_When_ProvidedNonZero
// guards the ListPendingGuidanceRequests-consistency fix: cap is a caller
// parameter, not a hardcoded DefaultGuidanceRequestPendingCap.
func TestListGuidanceRequestsForScope_should_UseCapParam_When_ProvidedNonZero(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	itemID := createTestBacklogItemForGuidance(t, repo)

	_, _, cap, err := repo.ListGuidanceRequestsForScope(ctx, domain.RequestScopeBacklogItem, itemID.String(), 12)
	require.NoError(t, err)
	assert.Equal(t, 12, cap)

	_, _, defaultedCap, err := repo.ListGuidanceRequestsForScope(ctx, domain.RequestScopeBacklogItem, itemID.String(), 0)
	require.NoError(t, err)
	assert.Equal(t, DefaultGuidanceRequestPendingCap, defaultedCap, "zero must fall back to DefaultGuidanceRequestPendingCap")
}
