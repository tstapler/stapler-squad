package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Copies of server/services test fixtures, which a leaf cannot import.

func writeSettingsFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func newTestNotificationStore(t *testing.T) *notifications.NotificationHistoryStore {
	t.Helper()
	store, err := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))
	require.NoError(t, err)
	return store
}

type fakeApprovalLiveInstanceFinder struct{ inst *instance.Instance }

func (f *fakeApprovalLiveInstanceFinder) FindLiveInstance(id string) *instance.Instance {
	if f.inst != nil && (f.inst.UUID == id || f.inst.Title == id) {
		return f.inst
	}
	return nil
}

// failingCIInstance is a session on PR #42 with failing CI.
func failingCIInstance(sessionID string) *instance.Instance {
	return &instance.Instance{Title: sessionID, UUID: sessionID, GitHubPRNumber: 42, GitHubCheckConclusion: "failure"}
}
