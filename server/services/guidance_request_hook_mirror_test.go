package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const mirrorSession = "7c1e0000-0000-4000-8000-0000000000aa"

type mirrorCounts struct {
	mu      sync.Mutex
	failed  []string
	cleared []string
}

func (c *mirrorCounts) onFailed(r string) { c.mu.Lock(); c.failed = append(c.failed, r); c.mu.Unlock() }
func (c *mirrorCounts) onCleared(r string) {
	c.mu.Lock()
	c.cleared = append(c.cleared, r)
	c.mu.Unlock()
}
func (c *mirrorCounts) failures() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.failed...)
}

func newTestMirror(t *testing.T) (*GuidanceHookMirror, *session.Storage, *PendingQuestionStore, *mirrorCounts) {
	t.Helper()
	storage := createTestStorage(t)
	pending := NewPendingQuestionStore(nil)
	counts := &mirrorCounts{}
	return newRealStoreMirror(storage, pending, counts), storage, pending, counts
}

// newRealStoreMirror lifts the 250 ms production bound: a race-instrumented
// SQLite write under a parallel test run can exceed it, which is not what these tests probe.
func newRealStoreMirror(storage *session.Storage, pending *PendingQuestionStore, counts *mirrorCounts) *GuidanceHookMirror {
	m := NewGuidanceHookMirror(storage, pending, counts.onFailed, counts.onCleared)
	m.bound = 10 * time.Second
	return m
}

func askedFromBody(t *testing.T, question string, labels ...string) AskedQuestion {
	t.Helper()
	opts := make([]interface{}, len(labels))
	for i, l := range labels {
		opts[i] = map[string]interface{}{"label": l, "description": "d"}
	}
	return ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": question, "header": "Color", "multiSelect": false, "options": opts,
	}}})
}

func mirrorReq(t *testing.T, callID, question string, replyable bool) HookMirrorRequest {
	t.Helper()
	return HookMirrorRequest{SessionUUID: mirrorSession, HookCallID: callID, Asked: askedFromBody(t, question, "Red", "Green"), Replyable: replyable}
}

func guidanceRowOf(t *testing.T, s *session.Storage, id string) *session.GuidanceRequestData {
	t.Helper()
	row, err := s.GetGuidanceRequest(context.Background(), uuid.MustParse(id))
	require.NoError(t, err)
	return row
}

func TestMirror_should_WriteHookRowWithAttributionBasedRoute(t *testing.T) {
	t.Parallel()
	m, storage, _, _ := newTestMirror(t)

	verified, ok := m.Mirror(mirrorReq(t, "toolu_1", "Which color?", true))
	require.True(t, ok)
	row := guidanceRowOf(t, storage, verified)
	assert.Equal(t, domain.QuestionSourceHook, row.Source)
	assert.Equal(t, domain.RequestScopeSession, row.Scope)
	assert.Equal(t, mirrorSession, row.AskerSessionUUID)
	assert.Equal(t, domain.DeliveryRouteLiveKeystroke, row.DeliveryRoute)
	assert.Equal(t, domain.HookCallID("toolu_1"), row.HookCallID)

	viewOnly, ok := m.Mirror(mirrorReq(t, "toolu_2", "Which size?", false))
	require.True(t, ok)
	assert.Empty(t, guidanceRowOf(t, storage, viewOnly).DeliveryRoute, "view-only rows stay unrouted, never replyable")
}

func TestMirror_should_BeIdempotent_PerHookCallID(t *testing.T) {
	t.Parallel()
	m, _, _, _ := newTestMirror(t)
	first, ok := m.Mirror(mirrorReq(t, "toolu_1", "Which color?", true))
	require.True(t, ok)
	again, ok := m.Mirror(mirrorReq(t, "toolu_1", "Which color?", true))
	require.True(t, ok)
	assert.Equal(t, first, again)
	other, ok := m.Mirror(mirrorReq(t, "toolu_2", "Which color?", true))
	require.True(t, ok)
	assert.NotEqual(t, first, other, "identical text with a different call id is a different question")
}

// ADR-003 rule 1 accepted limitation: without a call id the payload hash is the key.
func TestMirror_should_CoalesceIdenticalPayloads_WhenNoHookCallID(t *testing.T) {
	t.Parallel()
	m, _, _, _ := newTestMirror(t)
	first, ok := m.Mirror(mirrorReq(t, "", "Which color?", true))
	require.True(t, ok)
	second, ok := m.Mirror(mirrorReq(t, "", "Which color?", true))
	require.True(t, ok)
	assert.Equal(t, first, second)
}

func TestMirror_should_RedactSecretsInQuestionsJSON(t *testing.T) {
	t.Parallel()
	m, storage, _, _ := newTestMirror(t)
	secret := "ghp_" + strings.Repeat("a", 36)
	asked := ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": "Use token " + secret + "?", "header": "Tok", "multiSelect": false,
		"options": []interface{}{map[string]interface{}{"label": "Yes", "description": "token " + secret}},
	}}})
	id, ok := m.Mirror(HookMirrorRequest{SessionUUID: mirrorSession, HookCallID: "toolu_s", Asked: asked, Replyable: true})
	require.True(t, ok)
	row := guidanceRowOf(t, storage, id)
	assert.NotContains(t, row.QuestionsJSON, secret)
	assert.NotContains(t, row.QuestionText, secret)
	assert.Contains(t, row.QuestionsJSON, "[redacted]")
}

// blockingMirrorStore blocks CreateMirroredGuidanceRequest until released.
type blockingMirrorStore struct {
	guidanceMirrorStore
	release chan struct{}
	err     error
}

func (b *blockingMirrorStore) CreateMirroredGuidanceRequest(ctx context.Context, req domain.GuidanceRequest, capLimit int) (*session.GuidanceRequestData, error) {
	if b.release != nil {
		<-b.release
	}
	return nil, b.err
}

func TestMirror_should_FailOpenWithinBound_WhenRepositoryBlocksOrErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		store  *blockingMirrorStore
		reason string
	}{
		"blocks": {&blockingMirrorStore{release: make(chan struct{})}, mirrorFailTimeout},
		"errors": {&blockingMirrorStore{err: errors.New("database is locked")}, mirrorFailError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			counts := &mirrorCounts{}
			m := NewGuidanceHookMirror(tc.store, NewPendingQuestionStore(nil), counts.onFailed, counts.onCleared)
			if tc.store.release != nil {
				t.Cleanup(func() { close(tc.store.release) })
			}
			start := time.Now()
			id, ok := m.Mirror(mirrorReq(t, "toolu_1", "Which color?", true))
			assert.Less(t, time.Since(start), 300*time.Millisecond)
			assert.False(t, ok)
			assert.Empty(t, id)
			assert.Equal(t, []string{tc.reason}, counts.failures(), "counted exactly once")
		})
	}
}

func TestRehydrate_should_RestoreReplyableRows_AndFailClosedOnADifferentDialog(t *testing.T) {
	t.Parallel()
	m, _, _, _ := newTestMirror(t)
	frame, err := os.ReadFile("../../session/testdata/reply_dialogs/question_single_select_3opts.txt")
	require.NoError(t, err)
	asked := ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": "Which color should the spike use?", "header": "Color", "multiSelect": false,
		"options": []interface{}{
			map[string]interface{}{"label": "Red"}, map[string]interface{}{"label": "Green"}, map[string]interface{}{"label": "Blue"},
		},
	}}})
	id, ok := m.Mirror(HookMirrorRequest{SessionUUID: mirrorSession, HookCallID: "toolu_1", Asked: asked, Replyable: true})
	require.True(t, ok)
	_, viewOnly := m.Mirror(mirrorReq(t, "toolu_ro", "Read only?", false))
	require.True(t, viewOnly)

	restarted := NewPendingQuestionStore(nil) // the service restart: the in-memory cache is empty
	m2 := NewGuidanceHookMirror(m.store, restarted, nil, nil)
	n, err := m2.Rehydrate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the read_only row is not replyable")

	q, found := restarted.Peek(QuestionKey{SessionUUID: mirrorSession, QuestionID: id})
	require.True(t, found, "the card's question id is the row id")
	assert.True(t, session.DialogMatch(string(frame), q.Dialog), "the same dialog still verifies")
	other, err := os.ReadFile("../../session/testdata/reply_dialogs/question_wrapped_2opts.txt")
	require.NoError(t, err)
	assert.False(t, session.DialogMatch(string(other), q.Dialog), "a different dialog on screen: DialogMatch fails, so nothing is written")
}

func TestEnsureRegistered_should_ReRegisterAnOpenRow_AfterTheCacheTTL(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	now := time.Now()
	clock := func() time.Time { return now }
	cache := NewPendingQuestionStore(clock)
	m := newRealStoreMirror(storage, cache, &mirrorCounts{})
	id, ok := m.Mirror(mirrorReq(t, "toolu_1", "Which color?", true))
	require.True(t, ok)
	key := QuestionKey{SessionUUID: mirrorSession, QuestionID: id}
	require.False(t, peeked(cache, key), "never registered by Mirror itself")

	require.True(t, m.EnsureRegistered(context.Background(), key))
	now = now.Add(45 * time.Minute)
	assert.False(t, peeked(cache, key), "past the 30-minute TTL")
	assert.True(t, m.EnsureRegistered(context.Background(), key), "the durable row re-registers it")

	require.NoError(t, m.Clear(context.Background(), id, domain.CancelReasonDialogGone))
	now = now.Add(45 * time.Minute)
	assert.False(t, m.EnsureRegistered(context.Background(), key), "a cleared row is never re-registered")
}

func TestClear_should_FreeCapSlots_SoASixthQuestionMirrors(t *testing.T) {
	t.Parallel()
	m, storage, _, counts := newTestMirror(t)
	ctx := context.Background()
	var ids []string
	for i := 1; i <= 5; i++ {
		id, ok := m.Mirror(mirrorReq(t, fmt.Sprintf("toolu_%d", i), fmt.Sprintf("Question %d?", i), true))
		require.Truef(t, ok, "question %d mirrors because the earlier ones were cleared", i)
		require.NoError(t, m.Clear(ctx, id, domain.CancelReasonAnsweredInTerminal))
		ids = append(ids, id)
	}
	sixth, ok := m.Mirror(mirrorReq(t, "toolu_6", "Question 6?", true))
	require.True(t, ok)
	assert.Empty(t, counts.failures())
	assert.Len(t, counts.cleared, 5)
	for _, id := range ids {
		row := guidanceRowOf(t, storage, id)
		assert.Equal(t, domain.CancelReasonAnsweredInTerminal, row.CancelReason)
		assert.NotNil(t, row.CancelledAt)
	}
	assert.Nil(t, guidanceRowOf(t, storage, sixth).CancelledAt)
}

func TestMirror_should_HitTheCap_WhenNothingIsCleared(t *testing.T) {
	t.Parallel()
	m, _, _, counts := newTestMirror(t)
	for i := 1; i <= 4; i++ {
		_, ok := m.Mirror(mirrorReq(t, fmt.Sprintf("toolu_%d", i), fmt.Sprintf("Question %d?", i), true))
		require.True(t, ok)
	}
	_, ok := m.Mirror(mirrorReq(t, "toolu_5", "Question 5?", true))
	assert.False(t, ok)
	assert.Equal(t, []string{mirrorFailCapped}, counts.failures())
}

func TestClearByHookCall_should_CancelOnlyThatCallsRow(t *testing.T) {
	t.Parallel()
	m, storage, _, _ := newTestMirror(t)
	a, _ := m.Mirror(mirrorReq(t, "toolu_a", "A?", true))
	b, _ := m.Mirror(mirrorReq(t, "toolu_b", "B?", true))
	require.NoError(t, m.ClearByHookCall(context.Background(), mirrorSession, "toolu_a", domain.CancelReasonAnsweredInTerminal))
	assert.NotNil(t, guidanceRowOf(t, storage, a).CancelledAt)
	assert.Nil(t, guidanceRowOf(t, storage, b).CancelledAt)
}

func TestNotifyAnswered_should_BeSuppressed_ForHookSourceRows(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	bus := events.NewEventBus(10)
	svc := NewGuidanceRequestService(storage, bus, nil)
	m := newRealStoreMirror(storage, NewPendingQuestionStore(nil), &mirrorCounts{})
	id, ok := m.Mirror(mirrorReq(t, "toolu_1", "Which color?", true))
	require.True(t, ok)
	ctx := context.Background()
	ch, subID := bus.Subscribe(ctx)
	defer bus.Unsubscribe(subID)

	resp, err := svc.AnswerGuidanceRequest(ctx, connect.NewRequest(&sessionv1.AnswerGuidanceRequestRequest{Id: id, Answer: "Red"}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetApplied())
	select {
	case ev := <-ch:
		t.Fatalf("unexpected notification for a hook-source answer: %+v", ev)
	default:
	}
}

func peeked(s *PendingQuestionStore, k QuestionKey) bool {
	_, ok := s.Peek(k)
	return ok
}
