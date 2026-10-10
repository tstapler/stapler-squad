package services

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func testQuestion(sessionUUID, id string, labels ...string) PendingQuestion {
	if len(labels) == 0 {
		labels = []string{"A", "B"}
	}
	return PendingQuestion{
		QuestionID: id, SessionUUID: sessionUUID,
		Dialog: session.QuestionDialog{Question: "Q " + id + "?", Header: "H", Labels: labels},
	}
}

// T-RP-01
func TestPendingQuestionStore_ShouldRegisterBoundEvictAndExpire_WhenQuestionsAreAskedAnsweredOrLeave(t *testing.T) {
	clock := newGateTestClock()
	s := NewPendingQuestionStore(clock.Now)

	s.Register(testQuestion("s1", "q1"))
	got, ok := s.Peek(QuestionKey{"s1", "q1"})
	require.True(t, ok)
	assert.Equal(t, "q1", got.QuestionID)
	_, ok = s.Peek(QuestionKey{"other", "q1"})
	assert.False(t, ok, "keyed by the session UUID")

	// 8 per session: the 9th evicts the oldest.
	for i := 2; i <= 9; i++ {
		s.Register(testQuestion("s1", fmt.Sprintf("q%d", i), fmt.Sprintf("L%d", i)))
	}
	_, ok = s.Peek(QuestionKey{"s1", "q1"})
	assert.False(t, ok, "oldest evicted at the per-session bound")
	_, ok = s.Peek(QuestionKey{"s1", "q9"})
	assert.True(t, ok)

	// A same-question registration replaces the live entry (one card, not two).
	s.Register(PendingQuestion{QuestionID: "q9b", SessionUUID: "s1", Dialog: session.QuestionDialog{Question: "Q q9?", Header: "H", Labels: []string{"L9"}}})
	_, ok = s.Peek(QuestionKey{"s1", "q9"})
	assert.False(t, ok)
	_, ok = s.Peek(QuestionKey{"s1", "q9b"})
	assert.True(t, ok)

	// 256 total.
	for i := 0; i < 300; i++ {
		s.Register(testQuestion(fmt.Sprintf("bulk-%d", i), "x", "L"))
	}
	assert.LessOrEqual(t, s.Len(), pendingQuestionMaxTotal)

	// TTL 30 minutes.
	clock.Advance(pendingQuestionTTL + time.Second)
	_, ok = s.Peek(QuestionKey{"s1", "q9b"})
	assert.False(t, ok)
	assert.Zero(t, s.Len())
}

// T-RP-12, T-RP-33: one critical section; exactly one claim wins.
func TestClaim_ShouldAllowExactlyOneClaim_WhenManyGoroutinesClaimTheSameQuestion(t *testing.T) {
	s := NewPendingQuestionStore(nil)
	s.Register(testQuestion("s1", "q1"))
	var wg sync.WaitGroup
	start := make(chan struct{})
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, r := s.Claim(QuestionKey{"s1", "q1"}); r == ClaimOK {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()
	assert.Equal(t, 1, wins)
}

func TestClaim_ShouldRefuseAnotherSessionsQuestionAndAZeroClaimIsInvalid(t *testing.T) {
	s := NewPendingQuestionStore(nil)
	s.Register(testQuestion("s1", "q1"))
	_, r := s.Claim(QuestionKey{"s2", "q1"})
	assert.Equal(t, ClaimNoPending, r)
	assert.False(t, PendingQuestionClaim{}.Valid())
	assert.True(t, PendingQuestionClaim{}.Deleted())
}

// T-RP-55: release returns the entry; a delete while claimed never returns it.
func TestClaim_ShouldReturnToRegisteredOnRelease_ButNeverWhenSessionWasDeletedWhileClaimed(t *testing.T) {
	s := NewPendingQuestionStore(nil)
	s.Register(testQuestion("s1", "q1"))
	c, r := s.Claim(QuestionKey{"s1", "q1"})
	require.Equal(t, ClaimOK, r)
	_, again := s.Claim(QuestionKey{"s1", "q1"})
	assert.Equal(t, ClaimNoPending, again, "a claimed entry is not claimable")
	s.Release(c)
	_, ok := s.Peek(QuestionKey{"s1", "q1"})
	assert.True(t, ok)

	c, r = s.Claim(QuestionKey{"s1", "q1"})
	require.Equal(t, ClaimOK, r)
	s.SessionDeleted("s1")
	assert.True(t, c.Deleted(), "the before-first-byte checks stop the write")
	s.Release(c)
	_, ok = s.Peek(QuestionKey{"s1", "q1"})
	assert.False(t, ok, "claimed-deleted never returns to registered")
}

func TestClaim_ShouldConsumeOnConsumeAndRemoveRegisteredEntriesOnSessionDelete(t *testing.T) {
	s := NewPendingQuestionStore(nil)
	s.Register(testQuestion("s1", "q1"))
	s.Register(testQuestion("s1", "q2", "C"))
	c, _ := s.Claim(QuestionKey{"s1", "q1"})
	s.Consume(c)
	_, ok := s.Peek(QuestionKey{"s1", "q1"})
	assert.False(t, ok)
	s.SessionDeleted("s1")
	assert.Zero(t, s.Len())
}

func TestParseAskUserQuestion_ShouldClassifyTheRealPayloadShapes(t *testing.T) {
	opts := func(labels ...string) []interface{} {
		out := make([]interface{}, 0, len(labels))
		for _, l := range labels {
			out = append(out, map[string]interface{}{"label": l, "description": "d"})
		}
		return out
	}
	q := func(multi interface{}, labels ...string) map[string]interface{} {
		m := map[string]interface{}{"question": "Pick one?", "header": "Pick", "options": opts(labels...)}
		if multi != nil {
			m["multiSelect"] = multi
		}
		return m
	}
	long := make([]byte, 81)
	for i := range long {
		long[i] = 'x'
	}
	cases := []struct {
		name  string
		input map[string]interface{}
		want  QuestionShape
	}{
		{"single", map[string]interface{}{"questions": []interface{}{q(false, "A", "B")}}, QuestionShapeSingle},
		{"seven options", map[string]interface{}{"questions": []interface{}{q(false, "1", "2", "3", "4", "5", "6", "7")}}, QuestionShapeSingle},
		{"multiSelect", map[string]interface{}{"questions": []interface{}{q(true, "A", "B")}}, QuestionShapeMulti},
		{"two questions", map[string]interface{}{"questions": []interface{}{q(false, "A"), q(true, "B")}}, QuestionShapeMulti},
		{"multiSelect absent", map[string]interface{}{"questions": []interface{}{q(nil, "A", "B")}}, QuestionShapeUnknown},
		{"eight options", map[string]interface{}{"questions": []interface{}{q(false, "1", "2", "3", "4", "5", "6", "7", "8")}}, QuestionShapeUnknown},
		{"zero options", map[string]interface{}{"questions": []interface{}{q(false)}}, QuestionShapeUnknown},
		{"empty label", map[string]interface{}{"questions": []interface{}{q(false, "A", " ")}}, QuestionShapeUnknown},
		{"label too long", map[string]interface{}{"questions": []interface{}{q(false, string(long))}}, QuestionShapeUnknown},
		{"legacy prompt key", map[string]interface{}{"prompt": "Which?"}, QuestionShapeUnknown},
		{"nil", nil, QuestionShapeUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseAskUserQuestion(tc.input).Shape)
		})
	}
	got := ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{q(false, "A", "B")}})
	assert.Equal(t, "Pick one?", got.Question)
	assert.Equal(t, "Pick", got.Header)
	assert.Equal(t, []string{"A", "B"}, got.Labels)
}
