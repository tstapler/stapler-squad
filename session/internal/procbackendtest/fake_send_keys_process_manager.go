package procbackendtest

import (
	"sync"

	"github.com/tstapler/stapler-squad/session/instance"
)

// FakeSendKeysProcessManager is a minimal live-pane ProcessManager: it reports the pane live and
// simulates one pane change per SendKeys. It is a trimmed copy of the double in
// session/autonomous's tests, which this package cannot import.
type FakeSendKeysProcessManager struct {
	instance.ProcessManager
	mu            sync.Mutex
	pendingUpdate bool
}

func (f *FakeSendKeysProcessManager) HasSession() bool                         { return true }
func (f *FakeSendKeysProcessManager) IsAlive() bool                            { return true }
func (f *FakeSendKeysProcessManager) HasMeaningfulContent(content string) bool { return false }
func (f *FakeSendKeysProcessManager) FilterBanners(content string) (string, int) {
	return content, 0
}

func (f *FakeSendKeysProcessManager) SendKeys(keys string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingUpdate = true
	return len(keys), nil
}

func (f *FakeSendKeysProcessManager) HasUpdated() (updated bool, hasPrompt bool, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pendingUpdate {
		f.pendingUpdate = false
		return true, false, "updated"
	}
	return false, false, ""
}
