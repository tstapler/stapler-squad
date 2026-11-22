package session

import (
	"sync"

	"github.com/tstapler/stapler-squad/session/instance"
)

// fakeSendKeysProcessManager is a minimal live-pane ProcessManager: it reports the pane live and
// simulates one pane change per SendKeys. It is a trimmed copy of the double in
// session/autonomous's tests, which this package cannot import.
type fakeSendKeysProcessManager struct {
	instance.ProcessManager
	mu            sync.Mutex
	pendingUpdate bool
}

func (f *fakeSendKeysProcessManager) HasSession() bool                         { return true }
func (f *fakeSendKeysProcessManager) IsAlive() bool                            { return true }
func (f *fakeSendKeysProcessManager) HasMeaningfulContent(content string) bool { return false }
func (f *fakeSendKeysProcessManager) FilterBanners(content string) (string, int) {
	return content, 0
}

func (f *fakeSendKeysProcessManager) SendKeys(keys string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingUpdate = true
	return len(keys), nil
}

func (f *fakeSendKeysProcessManager) HasUpdated() (updated bool, hasPrompt bool, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pendingUpdate {
		f.pendingUpdate = false
		return true, false, "updated"
	}
	return false, false, ""
}
