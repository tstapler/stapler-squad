package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

type recordedTurn struct {
	text   string
	source session.TurnEndSource
	kind   session.TurnEndKind
}

// fakeTurnTarget fails the test if anything tries to revive it; the interface offers no
// way to, so this is a tripwire for the interface growing.
type fakeTurnTarget struct {
	mu       sync.Mutex
	program  string
	archived bool
	turns    []recordedTurn
}

func (f *fakeTurnTarget) GetProgram() string { return f.program }
func (f *fakeTurnTarget) IsArchived() bool   { return f.archived }
func (f *fakeTurnTarget) RecordClassifiedTurnEnd(text string, source session.TurnEndSource, kind session.TurnEndKind, _ session.TurnEndConfidence) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.turns = append(f.turns, recordedTurn{text, source, kind})
}

// Start and RecoverFromStopped exist so a future widening of turnEndTarget to include them
// would call into a failing fake.
func (f *fakeTurnTarget) Start(bool) error { panic("archived session revived via Start") }
func (f *fakeTurnTarget) RecoverFromStopped() {
	panic("archived session revived via RecoverFromStopped")
}

type notifyLog struct {
	mu    sync.Mutex
	calls []string
}

func (n *notifyLog) notify(_ context.Context, sessionID, _, _ string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, sessionID)
}

func newTestStopTurnEnds(targets map[string]turnEndTarget, projectsDir string) (*stopTurnEnds, *notifyLog) {
	n := &notifyLog{}
	h := newStopTurnEnds(func(id string) turnEndTarget { return targets[id] }, n.notify)
	h.projectsDir = func() (string, error) { return projectsDir, nil }
	return h, n
}

func TestStopTurnEnds_should_RecordQuestionFromHook_When_PayloadCarriesLastMessage(t *testing.T) {
	t.Parallel()
	inst := &session.Instance{Title: "demo", Program: "claude", Status: session.Running}
	h, n := newTestStopTurnEnds(map[string]turnEndTarget{"demo": inst}, t.TempDir())

	h.handle(context.Background(), "demo", StopPayload{PromptID: "p1", LastAssistantMessage: "Should I use Postgres or SQLite?"})

	got := inst.Snapshot().TurnEnded
	assert.Equal(t, session.TurnEndKindQuestion, got.Kind)
	assert.Equal(t, session.TurnEndSourceHook, got.Source)
	assert.NotEmpty(t, got.Episode)
	assert.LessOrEqual(t, len([]rune(got.Excerpt)), session.MaxTurnEndExcerptChars)
	assert.Equal(t, []string{"demo"}, n.calls)
}

func TestStopTurnEnds_should_FallBackToTranscriptTail_When_PayloadHasNoMessage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tp := filepath.Join(dir, "t.jsonl")
	line := `{"type":"assistant","uuid":"u1","message":{"content":[{"type":"text","text":"Should I use Postgres or SQLite?"}]}}` + "\n"
	require.NoError(t, os.WriteFile(tp, []byte(line), 0o600))
	inst := &session.Instance{Title: "demo", Program: "claude", Status: session.Running}
	h, _ := newTestStopTurnEnds(map[string]turnEndTarget{"demo": inst}, dir)

	h.handle(context.Background(), "demo", StopPayload{PromptID: "p1", TranscriptPath: tp})

	assert.Equal(t, session.TurnEndKindQuestion, inst.Snapshot().TurnEnded.Kind)
}

func TestStopTurnEnds_should_RecordUnknown_When_TranscriptPathEscapes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i, p := range []string{"/etc/passwd", filepath.Join(dir, "..", "..", "etc", "passwd")} {
		inst := &session.Instance{Title: "demo", Program: "claude", Status: session.Running}
		h, n := newTestStopTurnEnds(map[string]turnEndTarget{"demo": inst}, dir)

		h.handle(context.Background(), "demo", StopPayload{PromptID: "p", TranscriptPath: p})

		got := inst.Snapshot().TurnEnded
		assert.Equal(t, session.TurnEndKindUnknown, got.Kind, "case %d", i)
		assert.Empty(t, got.Excerpt)
		assert.Equal(t, []string{"demo"}, n.calls)
	}
}

func TestStopTurnEnds_should_YieldOneFactAndOneNotification_When_StopDeliveredTwice(t *testing.T) {
	t.Parallel()
	f := &fakeTurnTarget{program: "claude"}
	h, n := newTestStopTurnEnds(map[string]turnEndTarget{"demo": f}, t.TempDir())
	body := StopPayload{PromptID: "p1", LastAssistantMessage: "Done."}

	h.handle(context.Background(), "demo", body)
	h.handle(context.Background(), "demo", body)

	assert.Len(t, f.turns, 1)
	assert.Len(t, n.calls, 1)

	h.handle(context.Background(), "demo", StopPayload{PromptID: "p2", LastAssistantMessage: "Done again."})
	assert.Len(t, f.turns, 2)
}

func TestStopTurnEnds_should_DedupOnTimestamp_When_NoPromptID(t *testing.T) {
	t.Parallel()
	f := &fakeTurnTarget{program: "claude"}
	h, _ := newTestStopTurnEnds(map[string]turnEndTarget{"demo": f}, t.TempDir())
	at := time.Unix(1700000000, 0)
	body := StopPayload{LastAssistantMessage: "Done.", ReceivedAt: at}

	h.handle(context.Background(), "demo", body)
	h.handle(context.Background(), "demo", body)

	assert.Len(t, f.turns, 1)
}

func TestStopTurnEnds_should_RecordNothing_When_SessionUnknown(t *testing.T) {
	t.Parallel()
	h, n := newTestStopTurnEnds(map[string]turnEndTarget{}, t.TempDir())

	h.handle(context.Background(), "ghost", StopPayload{LastAssistantMessage: "hi?"})
	h.handle(context.Background(), "ghost", StopPayload{LastAssistantMessage: "hi?"})

	assert.Empty(t, n.calls)
}

func TestStopTurnEnds_should_RecordWithoutReviving_When_SessionArchived(t *testing.T) {
	t.Parallel()
	f := &fakeTurnTarget{program: "claude", archived: true}
	h, n := newTestStopTurnEnds(map[string]turnEndTarget{"old": f}, t.TempDir())

	h.handle(context.Background(), "old", StopPayload{PromptID: "p", LastAssistantMessage: "Want me to continue?"})

	require.Len(t, f.turns, 1)
	assert.Equal(t, session.TurnEndKindQuestion, f.turns[0].kind)
	assert.Empty(t, n.calls, "no notification for an archived session")
}

func TestStopTurnEnds_should_BeFinishedOnly_When_ProgramIsNotClaude(t *testing.T) {
	t.Parallel()
	f := &fakeTurnTarget{program: "aider"}
	h, _ := newTestStopTurnEnds(map[string]turnEndTarget{"a": f}, t.TempDir())

	h.handle(context.Background(), "a", StopPayload{PromptID: "p", LastAssistantMessage: "Should I proceed?"})

	require.Len(t, f.turns, 1)
	assert.Equal(t, session.TurnEndKindFinished, f.turns[0].kind)
}

func TestHandleStop_should_CallObserverWithParsedBody_When_StopPosted(t *testing.T) {
	t.Parallel()
	var got StopPayload
	var gotID string
	h := NewHookReceiver()
	h.SetStopObserver(func(_ context.Context, id string, b StopPayload) { gotID, got = id, b })

	req := httptest.NewRequest(http.MethodPost, "/api/hooks/stop",
		strings.NewReader(`{"prompt_id":"p9","last_assistant_message":"ok","transcript_path":"/x"}`))
	req.Header.Set(sessionIDHeader, "demo")
	rec := httptest.NewRecorder()
	h.HandleStop(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "demo", gotID)
	assert.Equal(t, "p9", got.PromptID)
	assert.Equal(t, "ok", got.LastAssistantMessage)
	assert.False(t, got.ReceivedAt.IsZero())
}

func TestHandleStop_should_Return200_When_BodyMalformedOrNoObserver(t *testing.T) {
	t.Parallel()
	h := NewHookReceiver()
	for _, body := range []string{"{not json", ""} {
		rec := httptest.NewRecorder()
		h.HandleStop(rec, httptest.NewRequest(http.MethodPost, "/api/hooks/stop", strings.NewReader(body)))
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	h.SetStopObserver(func(context.Context, string, StopPayload) {})
	rec := httptest.NewRecorder()
	h.HandleStop(rec, httptest.NewRequest(http.MethodPost, "/api/hooks/stop", strings.NewReader("{not json")))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestInjectDefaultLocalHooks_should_AddStopHookAndKeepOnePermissionHook_When_FreshSession(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	require.NoError(t, injectDefaultLocalHooks(root, "demo", "uuid-1"))
	require.NoError(t, injectDefaultLocalHooks(root, "demo", "uuid-1")) // idempotent

	data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.local.json"))
	require.NoError(t, err)
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(data, &settings))
	count := func(event, url string) int {
		n := 0
		for _, g := range settings.Hooks[event] {
			for _, h := range g.Hooks {
				if hookCommandReferencesURL(h.Command, url) {
					n++
				}
			}
		}
		return n
	}
	assert.Equal(t, 1, count("Stop", hookEndpoints(getHookBaseURLFn())[HookStopNotification]))
	assert.Equal(t, 1, count("PermissionRequest", hookApprovalURL()))
}
