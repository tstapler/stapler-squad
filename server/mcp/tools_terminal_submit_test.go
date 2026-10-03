package mcp

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// recordingPM captures SendKeys traffic and reports one pane change after each
// write, so session.SubmitDriverContent's settle/confirm polls succeed.
type recordingPM struct {
	session.ProcessManager
	mu      sync.Mutex
	sent    []string
	pending bool
}

func (r *recordingPM) HasSession() bool                     { return true }
func (r *recordingPM) IsAlive() bool                        { return true }
func (r *recordingPM) HasMeaningfulContent(string) bool     { return false }
func (r *recordingPM) FilterBanners(c string) (string, int) { return c, 0 }
func (r *recordingPM) SendKeys(keys string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, keys)
	r.pending = true
	return len(keys), nil
}

func (r *recordingPM) HasUpdated() (bool, bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending {
		r.pending = false
		return true, false, "updated"
	}
	return false, false, ""
}

func (r *recordingPM) writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

func newSubmitHarness(t *testing.T) (*terminalHandlers, *recordingPM) {
	t.Helper()
	pm := &recordingPM{}
	inst := session.NewStartedInstanceForTest("submit-s1", pm)
	th := &terminalHandlers{
		store:       &stubStore{instances: []*session.Instance{inst}},
		scrollback:  makeScrollbackMgr(t),
		writeLim:    newTokenBucket(10, 10),
		capturePane: func(*session.Instance) (string, error) { return "", nil },
	}
	return th, pm
}

// Long enough that a single concatenated write would land the Enter inside
// Claude Code's paste-detection window (BUG-031).
var longInput = strings.Repeat("a long paragraph of pasted text. ", 100)

func TestWriteToSession_PressEnter_SendsContentThenBareEnter(t *testing.T) {
	t.Parallel()
	th, pm := newSubmitHarness(t)
	res, err := th.writeToSession(context.Background(), makeToolReq(map[string]interface{}{
		"session_id": "submit-s1", "input": longInput,
	}))
	require.NoError(t, err)
	assert.Equal(t, true, parseResult(t, res)["success"])
	assert.Equal(t, []string{longInput, session.EnterKeySequence}, pm.writes())
}

func TestWriteToSession_PressEnterFalse_SendsSingleWriteWithoutEnter(t *testing.T) {
	t.Parallel()
	th, pm := newSubmitHarness(t)
	res, err := th.writeToSession(context.Background(), makeToolReq(map[string]interface{}{
		"session_id": "submit-s1", "input": longInput, "press_enter": false,
	}))
	require.NoError(t, err)
	assert.Equal(t, true, parseResult(t, res)["success"])
	assert.Equal(t, []string{longInput}, pm.writes())
}

func TestRunCommand_SendsCommandThenBareEnter(t *testing.T) {
	t.Parallel()
	th, pm := newSubmitHarness(t)
	res, err := th.runCommand(context.Background(), makeToolReq(map[string]interface{}{
		"session_id": "submit-s1", "command": "echo " + longInput[:200], "timeout_seconds": float64(1),
	}))
	require.NoError(t, err)
	assert.Equal(t, true, parseResult(t, res)["success"])
	assert.Equal(t, []string{"echo " + longInput[:200], session.EnterKeySequence}, pm.writes())
}

func TestSteerSession_PTYFallback_SendsMessageThenBareEnter(t *testing.T) {
	t.Parallel()
	th, pm := newSubmitHarness(t)
	res, err := th.steerSession(context.Background(), makeToolReq(map[string]interface{}{
		"session_id": "submit-s1", "message": longInput,
	}))
	require.NoError(t, err)
	m := parseResult(t, res)
	assert.Equal(t, true, m["success"])
	assert.Equal(t, "send_keys", m["method"])
	assert.Equal(t, []string{longInput, session.EnterKeySequence}, pm.writes())
}
