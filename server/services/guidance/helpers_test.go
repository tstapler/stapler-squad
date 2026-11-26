package guidance

// Copies of the server/services test helpers the moved tests share with the hub's own tests.

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/panereply"
)

// busNotifier is the test Notifier: it publishes to bus so a test can observe the notification.
type busNotifier struct{ bus *events.EventBus }

func (n *busNotifier) Notify(itemID, title, message string, notificationType int32, _, _ bool) {
	n.bus.Publish(events.NewNotificationEvent(itemID, "", uuid.NewString(), notificationType, 0, title, message, nil))
}

// manualClock is a settable clock for tests that must not sleep.
type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func newManualClock() *manualClock { return &manualClock{t: time.Unix(1_800_000_000, 0)} }

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

const replyColorLabel = "Green-label-sentinel"

var colorDialog = panereply.QuestionDialog{
	Question: "Which color should the spike use?",
	Header:   "Color",
	Labels:   []string{"Red", replyColorLabel, "Blue"},
}

// replyDialogFixture loads a verbatim capture of a real Claude Code dialog.
func replyDialogFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "session", "testdata", "reply_dialogs", name))
	require.NoError(t, err)
	return string(b)
}

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

// newRealStoreMirror lifts the 250 ms production bound: a race-instrumented
// SQLite write under a parallel test run can exceed it, which is not what these tests probe.
func newRealStoreMirror(storage *session.Storage, pending *questionkit.PendingQuestionStore, counts *mirrorCounts) *questionkit.GuidanceHookMirror {
	m := questionkit.NewGuidanceHookMirror(storage, pending, counts.onFailed, counts.onCleared, func() bool { return true })
	m.SetBound(10 * time.Second)
	return m
}

func mirrorReq(t *testing.T, callID, question string, replyable bool) questionkit.HookMirrorRequest {
	t.Helper()
	options := []interface{}{
		map[string]interface{}{"label": "Red", "description": "d"},
		map[string]interface{}{"label": "Green", "description": "d"},
	}
	asked := questionkit.ParseAskUserQuestion(map[string]interface{}{"questions": []interface{}{map[string]interface{}{
		"question": question, "header": "Color", "multiSelect": false, "options": options,
	}}})
	return questionkit.HookMirrorRequest{SessionUUID: mirrorSession, HookCallID: callID, Asked: asked, Replyable: replyable}
}
