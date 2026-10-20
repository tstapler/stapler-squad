package notifications

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func questionRecord(id, session, text, options, questionID string) *NotificationRecord {
	return &NotificationRecord{
		ID: id, SessionID: session, NotificationType: notifTypeInputRequired, Message: text, CreatedAt: time.Now(),
		Metadata: map[string]string{metaQuestionShape: "single", metaQuestionOptions: options, metaQuestionID: questionID},
	}
}

func TestAppend_ShouldKeepBothQuestions_WhenOneSessionRaisesTwoDistinctQuestions(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.Append(questionRecord("n1", "s1", "Cache?", `["Redis","Memcached"]`, "q1")))
	require.NoError(t, store.Append(questionRecord("n2", "s1", "Region?", `["eu","us"]`, "q2")))

	got, total, err := store.List(ListOptions{Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.ElementsMatch(t, []string{"q1", "q2"}, []string{got[0].Metadata[metaQuestionID], got[1].Metadata[metaQuestionID]})
}

func TestAppend_ShouldCollapse_WhenSameQuestionTextAndLabelsAreAskedAgain(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.Append(questionRecord("n1", "s1", "Cache?", `["Redis"]`, "q1")))
	require.NoError(t, store.Append(questionRecord("n2", "s1", "Cache?", `["Redis"]`, "q2")))

	got, total, err := store.List(ListOptions{Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, 2, got[0].OccurrenceCount)
	assert.Equal(t, "q2", got[0].Metadata[metaQuestionID], "the newest id is the actionable one")
}

func TestAppend_ShouldNotCollapse_WhenSameTextButDifferentLabels(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.Append(questionRecord("n1", "s1", "Cache?", `["Redis"]`, "q1")))
	require.NoError(t, store.Append(questionRecord("n2", "s1", "Cache?", `["Redis","Memcached"]`, "q2")))

	_, total, err := store.List(ListOptions{Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
}

func TestDeduplicateExisting_ShouldKeepDistinctQuestions_WhenStoreReloads(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	store.mu.Lock()
	store.records = []*NotificationRecord{
		questionRecord("n2", "s1", "Region?", `["eu"]`, "q2"),
		questionRecord("n1", "s1", "Cache?", `["Redis"]`, "q1"),
	}
	require.NoError(t, store.deduplicateExisting())
	store.mu.Unlock()

	_, total, err := store.List(ListOptions{Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
}

func TestResolveQuestion_ShouldRetireOnlyThatQuestion_WhenIdMatches(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.Append(questionRecord("n1", "s1", "Cache?", `["Redis"]`, "q1")))
	require.NoError(t, store.Append(questionRecord("n2", "s1", "Region?", `["eu"]`, "q2")))

	n, err := store.ResolveQuestion("q1")

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	unread, _, _ := store.List(ListOptions{UnreadOnly: true, Limit: 10})
	require.Len(t, unread, 1)
	assert.Equal(t, "q2", unread[0].Metadata[metaQuestionID])
	again, err := store.ResolveQuestion("q1")
	require.NoError(t, err)
	assert.Zero(t, again, "a second resolve is a no-op")
	none, err := store.ResolveQuestion("")
	require.NoError(t, err)
	assert.Zero(t, none, "an empty id matches nothing")
}

func TestResolveApproval_ShouldRetireTheApprovalRecord(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.Append(&NotificationRecord{
		ID: "a1", SessionID: "s1", NotificationType: notifTypeApprovalNeeded, CreatedAt: time.Now(),
		Metadata: map[string]string{metaApprovalID: "a1"},
	}))

	n, err := store.ResolveApproval("a1")

	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestReopenQuestion_should_MarkOnlyThatQuestionsReadRecordsUnread(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	require.NoError(t, store.Append(questionRecord("n1", "s1", "Which one?", "a|b", "q1")))
	require.NoError(t, store.Append(questionRecord("n2", "s2", "Which two?", "a|b", "q2")))
	_, err := store.ResolveQuestion("q1")
	require.NoError(t, err)
	_, err = store.ResolveQuestion("q2")
	require.NoError(t, err)

	n, err := store.ReopenQuestion("q1")
	require.NoError(t, err)

	assert.Equal(t, 1, n)
	q1, _ := store.GetByID("n1")
	q2, _ := store.GetByID("n2")
	assert.False(t, q1.IsRead)
	assert.True(t, q2.IsRead)
	again, err := store.ReopenQuestion("q1")
	require.NoError(t, err)
	assert.Zero(t, again, "an unread record is left alone")
}

func TestResolveGuidance_should_RetireAnUnreplyableCardThatHasNoQuestionID(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	rec := questionRecord("n1", "s1", "Which regions?", "", "")
	delete(rec.Metadata, metaQuestionID)
	rec.Metadata[metaGuidanceID] = "g1"
	require.NoError(t, store.Append(rec))

	n, err := store.ResolveGuidance("g1")

	require.NoError(t, err)
	assert.Equal(t, 1, n)
	got, _ := store.GetByID("n1")
	assert.True(t, got.IsRead)
}
