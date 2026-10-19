package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

func (q *questionHarness) withMirror(t *testing.T) (*mirrorCounts, func(id, asker string) bool) {
	t.Helper()
	storage := createTestStorage(t)
	counts := &mirrorCounts{}
	q.h.SetGuidanceMirror(newRealStoreMirror(storage, q.store, counts))
	return counts, func(id, asker string) bool {
		row := guidanceRowOf(t, storage, id)
		return row.Source == domain.QuestionSourceHook && row.AskerSessionUUID == asker
	}
}

func TestRegisterQuestion_should_MirrorAndStampGuidanceRequestID_WhenProofVerified(t *testing.T) {
	q := newQuestionHarness(t)
	_, isHookRow := q.withMirror(t)

	ev := q.post(askQuestionBody, map[string]string{hookProofHeader: q.proofs.HeaderValue("sess-uuid")})
	md := ev.NotificationMetadata
	require.NotEmpty(t, md[metaGuidanceRequestID])
	assert.Equal(t, md[metaGuidanceRequestID], md[metaQuestionID], "the durable row id is the question id")
	assert.True(t, isHookRow(md[metaGuidanceRequestID], "sess-uuid"))
	_, ok := q.store.Peek(QuestionKey{SessionUUID: "sess-uuid", QuestionID: md[metaQuestionID]})
	assert.True(t, ok)
}

func TestRegisterQuestion_should_MirrorViewOnlyAndRegisterNothing_WhenAttributionIsUnverified(t *testing.T) {
	q := newQuestionHarness(t)
	_, isHookRow := q.withMirror(t)

	ev := q.post(askQuestionBody, map[string]string{"X-CS-Session-ID": "other-uuid"})
	md := ev.NotificationMetadata
	assert.Equal(t, replyCauseNoProof, md[metaReplyUnavailable])
	assert.NotContains(t, md, metaQuestionID)
	assert.Zero(t, q.store.Len())
	require.NotEmpty(t, md[metaGuidanceRequestID], "an unverified question still mirrors, view-only")
	assert.True(t, isHookRow(md[metaGuidanceRequestID], "other-uuid"))
}

func TestRegisterQuestion_should_NotMirror_WhenSessionHasNoUUID(t *testing.T) {
	q := newQuestionHarness(t)
	q.withMirror(t)
	ev := q.post(askQuestionBody, map[string]string{})
	assert.NotContains(t, ev.NotificationMetadata, metaGuidanceRequestID)
}

func TestBroadcastQuestionNotification_should_RedactSecretsInTheMessage(t *testing.T) {
	q := newQuestionHarness(t)
	secret := "ghp_" + strings.Repeat("b", 36)
	body := `{"tool_name":"AskUserQuestion","cwd":"/work","tool_input":{"questions":[{"question":"Use ` + secret + `?","header":"Tok","options":[{"label":"Yes","description":"y"}],"multiSelect":false}]}}`
	ev := q.post(body, map[string]string{hookProofHeader: q.proofs.HeaderValue("sess-uuid")})
	assert.NotContains(t, ev.NotificationMessage, secret)
	assert.Contains(t, ev.NotificationMessage, "[redacted]")
}

// Pre-mortem #7: a repository that blocks must not hold the hook's defer reply back.
func TestAskUserQuestionHook_should_ReplyDeferWithinBound_WhenMirrorRepositoryBlocks(t *testing.T) {
	q := newQuestionHarness(t)
	store := &blockingMirrorStore{release: make(chan struct{})}
	t.Cleanup(func() { close(store.release) })
	counts := &mirrorCounts{}
	blocked := NewGuidanceHookMirror(store, q.store, counts.onFailed, counts.onCleared)
	blocked.enabled = mirrorFlagOn
	q.h.SetGuidanceMirror(blocked)

	srv := httptest.NewServer(http.HandlerFunc(q.h.HandlePermissionRequest))
	t.Cleanup(srv.Close)
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(askQuestionBody))
	require.NoError(t, err)
	req.Header.Set(hookProofHeader, q.proofs.HeaderValue("sess-uuid"))

	start := time.Now()
	resp, err := srv.Client().Do(req) // returns when the response headers arrive
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Less(t, time.Since(start), 300*time.Millisecond)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Content-Type"), "an empty body defers to the native dialog")

	ev := requireOneNotification(t, q.events)
	assert.NotContains(t, ev.NotificationMetadata, metaGuidanceRequestID)
	assert.Equal(t, []string{mirrorFailTimeout}, counts.failures())
}
