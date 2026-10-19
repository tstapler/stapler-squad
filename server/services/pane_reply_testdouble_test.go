package services

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// Shared pane-reply test double: the visible-session and answer-router tests
// reuse it. Fault cases: sendErr with sendN == 0 is a provable NOT_SENT; sendN > 0
// is a partial write (SEND_INDETERMINATE); keepOpen leaves the dialog on screen.

// replyDialogFixture loads a verbatim capture of a real Claude Code dialog.
func replyDialogFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "session", "testdata", "reply_dialogs", name))
	require.NoError(t, err)
	return string(b)
}

// replyPM is a recording pane: it shows `dialog` until a key was written, then
// `after` (or still `dialog` when keepOpen), and notes how many audit lines
// were durable at the instant of each write.
type replyPM struct {
	session.ProcessManager
	mu       sync.Mutex
	sent     []string
	dialog   string
	after    string
	keepOpen bool
	sendErr  error
	sendN    int
	probe    func() int // audit lines visible when a write happens
	probed   []int
	gate     chan struct{} // when set, SendKeys blocks until closed
	entered  chan struct{}
	capErr   error
}

func (r *replyPM) GetSessionIdentifier() string     { return "" }
func (r *replyPM) GetPanePID() (int32, error)       { return 0, errors.New("no pane") }
func (r *replyPM) HasSession() bool                 { return true }
func (r *replyPM) IsAlive() bool                    { return true }
func (r *replyPM) HasMeaningfulContent(string) bool { return false }

func (r *replyPM) SendKeys(keys string) (int, error) {
	if r.entered != nil {
		r.entered <- struct{}{}
	}
	if r.gate != nil {
		<-r.gate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.probe != nil {
		r.probed = append(r.probed, r.probe())
	}
	if r.sendErr != nil && r.sendN == 0 {
		return 0, r.sendErr // provably nothing reached the pane
	}
	r.sent = append(r.sent, keys)
	if r.sendErr != nil {
		return r.sendN, r.sendErr
	}
	return len(keys), nil
}

func (r *replyPM) CapturePaneContent() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.capErr != nil {
		return "", r.capErr
	}
	if len(r.sent) > 0 && !r.keepOpen {
		return r.after, nil
	}
	return r.dialog, nil
}

func (r *replyPM) writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}
