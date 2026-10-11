package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/events"
)

const askQuestionBody = `{"tool_name":"AskUserQuestion","cwd":"/work","tool_input":{"questions":[{"question":"Which color should the spike use?","header":"Color","options":[{"label":"Red","description":"r"},{"label":"Green","description":"g"}],"multiSelect":false}]}}`

type questionHarness struct {
	t      *testing.T
	h      *ApprovalHandler
	store  *PendingQuestionStore
	proofs *HookProofs
	events <-chan *events.Event
	causes []string
}

func newQuestionHarness(t *testing.T) *questionHarness {
	t.Helper()
	bus := events.NewEventBus(8)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch, _ := bus.Subscribe(ctx)
	h := NewApprovalHandler(NewApprovalStore(""), nil, bus)
	proofs, err := NewHookProofs(t.TempDir())
	require.NoError(t, err)
	q := &questionHarness{t: t, h: h, store: NewPendingQuestionStore(nil), proofs: proofs, events: ch}
	h.SetHookProofs(proofs)
	h.SetQuestionRegistry(q.store, func(cause string) { q.causes = append(q.causes, cause) })
	h.questions.sessionLive = func(id string) bool { return id == "sess-uuid" || id == "other-uuid" }
	return q
}

func (q *questionHarness) post(body string, headers map[string]string) *events.Event {
	q.t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", strings.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	q.h.HandlePermissionRequest(w, r)
	return requireOneNotification(q.t, q.events)
}

// T-RP-18: the real payload shape (questions[], no "prompt") registers a
// question and stamps question_id, question_shape and question_options.
func TestBroadcastQuestionNotification_ShouldReadQuestionsArrayAndStampQuestionIdShapeAndOptionsMetadata_WhenAskUserQuestionFires(t *testing.T) {
	q := newQuestionHarness(t)
	ev := q.post(askQuestionBody, map[string]string{
		"X-CS-Session-ID": "title-ignored",
		hookProofHeader:   q.proofs.HeaderValue("sess-uuid"),
	})

	assert.Equal(t, "Claude has a question", ev.NotificationTitle)
	assert.Equal(t, "Which color should the spike use?", ev.NotificationMessage)
	assert.Equal(t, "sess-uuid", ev.SessionID, "a verified proof's UUID is the attribution; the title header is ignored")
	md := ev.NotificationMetadata
	assert.Equal(t, "single", md[metaQuestionShape])
	assert.Len(t, md[metaQuestionID], 32, "128 random bits")
	var labels []string
	require.NoError(t, json.Unmarshal([]byte(md[metaQuestionOptions]), &labels))
	assert.Equal(t, []string{"Red", "Green"}, labels)
	assert.NotContains(t, md, metaReplyUnavailable)

	got, ok := q.store.Peek(QuestionKey{SessionUUID: "sess-uuid", QuestionID: md[metaQuestionID]})
	require.True(t, ok)
	assert.Equal(t, "Color", got.Dialog.Header)
	assert.Equal(t, []string{"Red", "Green"}, got.Dialog.Labels)
	assert.Equal(t, "Which color should the spike use?", got.Token)
	assert.Empty(t, q.causes)
}

// T-RP-62, T-RP-77: no proof, a bad proof or a proof for an unknown session
// raises the toast, registers nothing replyable and labels the cause.
func TestHookProof_ShouldRegisterNothingReplyable_WhenProofIsMissingInvalidOrForUnknownSession(t *testing.T) {
	q := newQuestionHarness(t)
	cases := []struct {
		name    string
		headers map[string]string
		cause   string
	}{
		{"no proof", map[string]string{"X-CS-Session-ID": "title"}, replyCauseNoProof},
		{"no proof, no header", map[string]string{}, replyCausePathOnly},
		{"bad proof", map[string]string{"X-CS-Session-ID": "title", hookProofHeader: "sess-uuid.deadbeef"}, replyCauseBadProof},
		{"proof of another session secret", map[string]string{hookProofHeader: "x-uuid." + strings.Repeat("0", 64)}, replyCauseBadProof},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := q.post(askQuestionBody, tc.headers)
			assert.Equal(t, tc.cause, ev.NotificationMetadata[metaReplyUnavailable])
			assert.NotContains(t, ev.NotificationMetadata, metaQuestionID)
			assert.Equal(t, "Claude has a question", ev.NotificationTitle, "the toast is still raised")
		})
	}
	assert.Zero(t, q.store.Len())

	// A valid proof for a session that is not known is bad_proof too.
	unknown := q.proofs.HeaderValue("not-a-session")
	ev := q.post(askQuestionBody, map[string]string{hookProofHeader: unknown})
	assert.Equal(t, replyCauseBadProof, ev.NotificationMetadata[metaReplyUnavailable])
	assert.Zero(t, q.store.Len())
}

// T-RP-42: multi and unknown shapes are never registered, even with a proof.
func TestBroadcastQuestionNotification_ShouldMarkMultiOrUnknownShapeNotReplyable_WhenQuestionsHasTwoEntriesMultiSelectOrAnEmptyLabel(t *testing.T) {
	q := newQuestionHarness(t)
	proof := map[string]string{hookProofHeader: q.proofs.HeaderValue("sess-uuid")}
	bodies := map[string]string{
		"multiSelect": strings.Replace(askQuestionBody, `"multiSelect":false`, `"multiSelect":true`, 1),
		"two questions": `{"tool_name":"AskUserQuestion","tool_input":{"questions":[` +
			`{"question":"A?","header":"A","options":[{"label":"x"}],"multiSelect":false},` +
			`{"question":"B?","header":"B","options":[{"label":"y"}],"multiSelect":false}]}}`,
		"empty label": strings.Replace(askQuestionBody, `"label":"Red"`, `"label":""`, 1),
		"legacy":      `{"tool_name":"AskUserQuestion","tool_input":{"prompt":"Which approach?"}}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			ev := q.post(body, proof)
			assert.Equal(t, replyCauseShape, ev.NotificationMetadata[metaReplyUnavailable])
			assert.NotContains(t, ev.NotificationMetadata, metaQuestionID)
		})
	}
	assert.Zero(t, q.store.Len())
}

// T-RP-74: only the proofed senders can be replyable. The per-session curl hook
// and the single-hook curl carry the proof branch; the other senders do not.
func TestHookSenders_ShouldBeProofedOrNotReplyableAsDecided_ForEachKnownSenderOfXCSSessionId(t *testing.T) {
	for _, name := range []string{"session/mux/hooks.go", "session/sshremote/approval_relay.go", "scripts/ssq-hook-handler"} {
		raw := readRepoFile(t, name)
		assert.NotContains(t, raw, hookProofHeader, "%s is not a proofed sender, so its questions are not replyable", name)
		assert.Contains(t, raw, "X-CS-Session-ID", "%s is a known sender of the header; a new sender fails this table until decided", name)
	}
	assert.Contains(t, buildLocalHookCommand("http://x/y", "t"), "STAPLER_SESSION_UUID",
		"the per-session curl hook and the single-hook curl share one producer with the proof branch")
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	require.NoError(t, err)
	return string(b)
}
