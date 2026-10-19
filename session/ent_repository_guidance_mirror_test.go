package session

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	mirrorAsker       = "7c1e0000-0000-4000-8000-000000000001"
	mirrorPeer        = "7c1e0000-0000-4000-8000-000000000002"
	mirrorGoroutines  = 8
	mirrorCapForTests = 4
)

func mirrorQuestions(text string) domain.QuestionPayload {
	p, err := domain.NewQuestionPayload([]domain.Question{{Text: text, Header: "Q", Options: []domain.Option{{Label: "A"}, {Label: "B"}}}})
	if err != nil {
		panic(err)
	}
	return p
}

func hookRequest(t *testing.T, callID, text string) domain.GuidanceRequest {
	t.Helper()
	req, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: mirrorAsker, Source: domain.QuestionSourceHook,
		AskerSessionUUID: mirrorAsker, Payload: mirrorQuestions(text), HookCallID: domain.HookCallID(callID),
	})
	require.NoError(t, err)
	return req
}

func peerRequest(t *testing.T, text string) domain.GuidanceRequest {
	t.Helper()
	req, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: mirrorAsker, Source: domain.QuestionSourcePeer,
		AskerSessionUUID: mirrorAsker, AnswererKind: domain.AnswererKindSession, AnswererSessionUUID: mirrorPeer,
		QuestionText: text, QuestionType: domain.QuestionTypeShortAnswer,
	})
	require.NoError(t, err)
	return req
}

func agentSessionInput(text string) CreateGuidanceRequestInput {
	return CreateGuidanceRequestInput{
		Scope: domain.RequestScopeSession, SessionUUID: mirrorAsker, QuestionText: text, QuestionType: domain.QuestionTypeYesNo,
	}
}

func countGuidanceRows(t *testing.T, repo *EntRepository) int {
	t.Helper()
	n, err := repo.GetEntClient().GuidanceRequest.Query().Count(context.Background())
	require.NoError(t, err)
	return n
}

// Existing rows survive the additive migration with the defaults.
func TestGuidanceRequest_should_ReadLegacyRowWithDefaults_When_NewColumnsAreMigratedIn(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "guidance-migrate.db")
	ctx := context.Background()

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	itemID := createTestBacklogItemForGuidance(t, repo)
	created, err := repo.CreateGuidanceRequest(ctx, CreateGuidanceRequestInput{
		Scope: domain.RequestScopeBacklogItem, ItemID: &itemID, QuestionText: "Merge?", QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	// Rebuild the pre-feature shape: drop the new index and columns on a raw connection.
	raw, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_timeout=5000&_fk=1")
	require.NoError(t, err)
	_, err = raw.Exec(`DROP INDEX IF EXISTS guidancerequest_asker_session_uuid_hook_call_id`)
	require.NoError(t, err)
	for _, col := range []string{"source", "asker_session_uuid", "answerer_kind", "answerer_session_uuid", "multi_select",
		"questions_json", "delivery_route", "delivery_state", "delivery_attempts", "hook_call_id", "expires_at"} {
		_, err = raw.Exec(fmt.Sprintf(`ALTER TABLE guidance_requests DROP COLUMN %s`, col))
		require.NoError(t, err, col)
	}
	require.NoError(t, raw.Close())

	migrated, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err, "auto-migration must add the columns to a table that has rows")
	defer migrated.Close() //nolint:errcheck

	id, err := uuid.Parse(created.ID)
	require.NoError(t, err)
	got, err := migrated.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.RequestScopeBacklogItem, got.Scope)
	assert.Equal(t, domain.QuestionTypeYesNo, got.QuestionType)
	assert.Equal(t, domain.QuestionSourceAgentMCP, got.Source)
	assert.Equal(t, domain.AnswererKindHuman, got.AnswererKind)
	assert.False(t, got.MultiSelect)
	assert.Empty(t, got.DeliveryState)
	assert.Zero(t, got.DeliveryAttempts)

	ok, err := migrated.SetDelivery(ctx, id, domain.DeliveryRouteResume, domain.DeliveryStatePending, 0)
	require.NoError(t, err)
	assert.True(t, ok, "a migrated row (NULL delivery_state) is a legal start state")
}

func TestCreateMirroredGuidanceRequest_should_YieldOneRow_When_EightGoroutinesCreateTheSameHookCall(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	req := hookRequest(t, "toolu_01", "Which cache?")

	ids := make([]string, mirrorGoroutines)
	errs := make([]error, mirrorGoroutines)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range mirrorGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			row, err := repo.CreateMirroredGuidanceRequest(ctx, req, 0)
			errs[i] = err
			if row != nil {
				ids[i] = row.ID
			}
		}()
	}
	close(start)
	wg.Wait()

	for i := range ids {
		require.NoError(t, errs[i])
		assert.Equal(t, ids[0], ids[i], "every caller receives the one row's id")
	}
	assert.Equal(t, 1, countGuidanceRows(t, repo))
}

func TestCreateMirroredGuidanceRequest_should_YieldTwoRows_When_IdenticalTextHasDifferentHookCallIDs(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()

	first, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_01", "Which cache?"), 0)
	require.NoError(t, err)
	second, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_02", "Which cache?"), 0)
	require.NoError(t, err)

	assert.NotEqual(t, first.ID, second.ID, "the legacy (scope, scope_key, question_text) coalesce is bypassed")
	open, err := repo.ListOpenGuidanceRequestsByAsker(ctx, mirrorAsker)
	require.NoError(t, err)
	assert.Len(t, open, 2)
}

func TestCreateMirroredGuidanceRequest_should_StoreMirrorColumns_When_HookCreated(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	row, err := repo.CreateMirroredGuidanceRequest(context.Background(), hookRequest(t, "toolu_01", "Which cache?"), 0)
	require.NoError(t, err)

	assert.Equal(t, domain.RequestScopeSession, row.Scope)
	assert.Equal(t, domain.QuestionSourceHook, row.Source)
	assert.Equal(t, mirrorAsker, row.AskerSessionUUID)
	assert.Equal(t, domain.HookCallID("toolu_01"), row.HookCallID)
	assert.Equal(t, "Which cache?", row.QuestionText, "question 1 stays readable by old readers")
	payload, err := domain.ParseQuestionPayload([]byte(row.QuestionsJSON))
	require.NoError(t, err)
	assert.False(t, payload.IsZero())
	assert.Nil(t, row.ExpiresAt, "hook rows are cleared by the mirror rules, not a TTL")
}

func TestCreateMirroredGuidanceRequest_should_ResolveToTheExistingRow_When_HookCallWasAlreadyCancelled(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	req := hookRequest(t, "toolu_01", "Which cache?")

	first, err := repo.CreateMirroredGuidanceRequest(ctx, req, 0)
	require.NoError(t, err)
	id := uuid.MustParse(first.ID)
	ok, err := repo.CancelGuidanceRequest(ctx, id, "dialog_gone")
	require.NoError(t, err)
	require.True(t, ok)

	again, err := repo.CreateMirroredGuidanceRequest(ctx, req, 0)
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "a tool_use_id names one call; a resolved row is not recreated")
	assert.Equal(t, GuidanceRequestStatusCancelled, again.Status())
}

func TestCreateMirroredGuidanceRequest_should_CoalesceOnPayloadAmongOpenRows_When_HookCallIDIsMissing(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	req := hookRequest(t, "", "Which cache?")

	first, err := repo.CreateMirroredGuidanceRequest(ctx, req, 0)
	require.NoError(t, err)
	second, err := repo.CreateMirroredGuidanceRequest(ctx, req, 0)
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID)

	other, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "", "Which queue?"), 0)
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, other.ID)

	_, err = repo.CancelGuidanceRequest(ctx, uuid.MustParse(first.ID), "answered_in_terminal")
	require.NoError(t, err)
	reasked, err := repo.CreateMirroredGuidanceRequest(ctx, req, 0)
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, reasked.ID, "a resolved payload may be asked again")
}

func TestCreateMirroredGuidanceRequest_should_CreatePeerRow_When_RequestHelp(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	before := time.Now()
	row, err := repo.CreateMirroredGuidanceRequest(context.Background(), peerRequest(t, "How do I run the linter?"), 0)
	require.NoError(t, err)

	assert.Equal(t, domain.RequestScopeSession, row.Scope)
	assert.Equal(t, mirrorAsker, row.SessionUUID)
	assert.Equal(t, domain.QuestionSourcePeer, row.Source)
	assert.Equal(t, domain.AnswererKindSession, row.AnswererKind)
	assert.Equal(t, mirrorPeer, row.AnswererSessionUUID)
	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(t, before.Add(domain.PeerHelpTTL), *row.ExpiresAt, time.Minute)

	again, err := repo.CreateMirroredGuidanceRequest(context.Background(), peerRequest(t, "How do I run the linter?"), 0)
	require.NoError(t, err)
	assert.Equal(t, row.ID, again.ID, "an identical open peer question coalesces")
}

func TestCreateMirroredGuidanceRequest_should_WriteNothing_When_CombinationIsIllegal(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)

	_, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: mirrorAsker, Source: domain.QuestionSourcePeer, AskerSessionUUID: mirrorAsker,
		AnswererKind: domain.AnswererKindHuman, QuestionText: "q", QuestionType: domain.QuestionTypeShortAnswer,
	})
	require.ErrorIs(t, err, domain.ErrInvalidGuidanceRequest)

	_, err = domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: mirrorAsker, Source: domain.QuestionSourceHook, AskerSessionUUID: mirrorAsker,
		Payload: mirrorQuestions("q"), DeliveryRoute: domain.DeliveryRouteReadOnly, DeliveryState: domain.DeliveryStateSent,
	})
	require.ErrorIs(t, err, domain.ErrInvalidGuidanceRequest)

	assert.Zero(t, countGuidanceRows(t, repo))
}

func fillClass(t *testing.T, repo *EntRepository, n int, create func(i int) error) {
	t.Helper()
	for i := range n {
		require.NoError(t, create(i), "row %d", i)
	}
}

func hookCreator(t *testing.T, repo *EntRepository) func(int) error {
	return func(i int) error {
		_, err := repo.CreateMirroredGuidanceRequest(context.Background(), hookRequest(t, fmt.Sprintf("toolu_%02d", i), fmt.Sprintf("hook q%d", i)), 0)
		return err
	}
}

func peerCreator(t *testing.T, repo *EntRepository) func(int) error {
	return func(i int) error {
		_, err := repo.CreateMirroredGuidanceRequest(context.Background(), peerRequest(t, fmt.Sprintf("peer q%d", i)), 0)
		return err
	}
}

func agentCreator(repo *EntRepository) func(int) error {
	return func(i int) error {
		_, err := repo.CreateGuidanceRequest(context.Background(), agentSessionInput(fmt.Sprintf("agent q%d", i)))
		return err
	}
}

func TestGuidanceRequestPendingCap_should_CountPerSourceClass(t *testing.T) {
	t.Parallel()
	classes := map[string]func(*testing.T, *EntRepository) func(int) error{
		"agent_mcp": func(_ *testing.T, r *EntRepository) func(int) error { return agentCreator(r) },
		"hook":      hookCreator,
		"peer":      peerCreator,
	}
	for fullName, full := range classes {
		for otherName, other := range classes {
			t.Run(fullName+" at cap, then "+otherName, func(t *testing.T) {
				t.Parallel()
				repo := NewTestEntRepository(t)
				fillClass(t, repo, mirrorCapForTests, full(t, repo))

				err := other(t, repo)(mirrorCapForTests) // one more of the other class (or this one's 5th)
				if fullName == otherName {
					assert.ErrorIs(t, err, ErrPendingCapExceeded, "a class still refuses its own 5th row")
				} else {
					assert.NoError(t, err, "a full %s class does not block %s", fullName, otherName)
				}
			})
		}
	}
}

func TestGuidanceRequestPendingCap_should_NotRaiseCapError_When_HookRowIsReRegisteredAtCap(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	fillClass(t, repo, mirrorCapForTests, hookCreator(t, repo))

	existing, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_00", "hook q0"), 0)
	require.NoError(t, err, "a dedup hit never raises PENDING_CAP_EXCEEDED")
	open, err := repo.ListOpenGuidanceRequestsByAsker(ctx, mirrorAsker, domain.QuestionSourceHook)
	require.NoError(t, err)
	assert.Len(t, open, mirrorCapForTests)
	assert.NotEmpty(t, existing.ID)
}

func TestListOpenGuidanceRequestsByAsker_should_FilterBySourceAndOpenStatus(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	hook, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_01", "hook q"), 0)
	require.NoError(t, err)
	_, err = repo.CreateMirroredGuidanceRequest(ctx, peerRequest(t, "peer q"), 0)
	require.NoError(t, err)
	_, err = repo.CreateGuidanceRequest(ctx, agentSessionInput("agent q"))
	require.NoError(t, err)

	all, err := repo.ListOpenGuidanceRequestsByAsker(ctx, mirrorAsker)
	require.NoError(t, err)
	assert.Len(t, all, 2, "the agent_mcp row has no asker_session_uuid")

	hooks, err := repo.ListOpenGuidanceRequestsByAsker(ctx, mirrorAsker, domain.QuestionSourceHook)
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	assert.Equal(t, hook.ID, hooks[0].ID)

	_, err = repo.CancelGuidanceRequest(ctx, uuid.MustParse(hook.ID), "dialog_gone")
	require.NoError(t, err)
	hooks, err = repo.ListOpenGuidanceRequestsByAsker(ctx, mirrorAsker, domain.QuestionSourceHook)
	require.NoError(t, err)
	assert.Empty(t, hooks)
}

func TestSetDelivery_should_FollowTheGuardedTransitionTable(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	row, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_01", "Which cache?"), 0)
	require.NoError(t, err)
	id := uuid.MustParse(row.ID)
	live := domain.DeliveryRouteLiveKeystroke

	steps := []struct {
		name  string
		state domain.DeliveryState
		add   int
		want  bool
	}{
		{"untracked -> pending", domain.DeliveryStatePending, 0, true},
		{"pending -> acknowledged skips sent", domain.DeliveryStateAcknowledged, 0, false},
		{"pending -> sent", domain.DeliveryStateSent, 1, true},
		{"sent -> sent again", domain.DeliveryStateSent, 1, false},
		{"sent -> acknowledged", domain.DeliveryStateAcknowledged, 0, true},
		{"acknowledged -> pending", domain.DeliveryStatePending, 0, false},
	}
	attempts := 0
	for _, st := range steps {
		applied, err := repo.SetDelivery(ctx, id, live, st.state, st.add)
		require.NoError(t, err, st.name)
		assert.Equal(t, st.want, applied, st.name)
		if applied {
			attempts += st.add
		}
	}
	got, err := repo.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.DeliveryStateAcknowledged, got.DeliveryState)
	assert.Equal(t, live, got.DeliveryRoute)
	assert.Equal(t, attempts, got.DeliveryAttempts)
}

func TestSetDelivery_should_RejectIllegalPairsWithoutWriting(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	row, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_01", "Which cache?"), 0)
	require.NoError(t, err)
	id := uuid.MustParse(row.ID)

	_, err = repo.SetDelivery(ctx, id, domain.DeliveryRouteReadOnly, domain.DeliveryStateSent, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidDelivery)
	_, err = repo.SetDelivery(ctx, id, "", domain.DeliveryStatePending, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidDelivery)
	_, err = repo.SetDelivery(ctx, id, domain.DeliveryRouteResume, "delivered", 0)
	assert.ErrorIs(t, err, domain.ErrInvalidDelivery)
	_, err = repo.SetDelivery(ctx, id, domain.DeliveryRouteResume, domain.DeliveryStatePending, -1)
	assert.Error(t, err)

	got, err := repo.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)
	assert.Empty(t, got.DeliveryState)
	assert.Empty(t, got.DeliveryRoute)
}

func TestSetDelivery_should_LetExactlyOneWriterWin_When_EightGoroutinesRaceTheSameTransition(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	row, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_01", "Which cache?"), 0)
	require.NoError(t, err)
	id := uuid.MustParse(row.ID)
	_, err = repo.SetDelivery(ctx, id, domain.DeliveryRouteLiveKeystroke, domain.DeliveryStatePending, 0)
	require.NoError(t, err)

	var wg sync.WaitGroup
	var won atomic.Int32
	start := make(chan struct{})
	for range mirrorGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			applied, err := repo.SetDelivery(ctx, id, domain.DeliveryRouteLiveKeystroke, domain.DeliveryStateSent, 1)
			if err != nil {
				t.Errorf("SetDelivery: %v", err)
			}
			if applied {
				won.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, won.Load())
	got, err := repo.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 1, got.DeliveryAttempts, "only the winner's attempt was counted")
}
