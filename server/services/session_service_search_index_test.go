package services

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewSessionService_TestMode_NeverTouchesRealSearchIndex guards the
// server/services/server/mcp CI timeout fix: NewSessionService used to always
// call search.NewIndexStore() and gob-decode whatever index was on disk,
// which under a `go test` binary meant every test in the package shared and
// grew one persisted index for the life of the binary -- slow enough under
// -race to blow CI's 150s budget. Under config.IsTestMode() (true for every
// go test binary), NewSessionService must use an in-memory search engine and
// never touch disk for the index at all, regardless of $HOME or
// STAPLER_SQUAD_TEST_DIR.
func TestNewSessionService_TestMode_NeverTouchesRealSearchIndex(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	if svc == nil {
		t.Fatal("NewSessionService returned nil")
	}

	realSearchIndexDir := filepath.Join(fakeHome, ".claude", "search_index")
	if _, err := os.Stat(realSearchIndexDir); !os.IsNotExist(err) {
		t.Errorf("NewSessionService created %q under test mode; want no disk persistence at all (err=%v)", realSearchIndexDir, err)
	}

	testConfigDir := os.Getenv("STAPLER_SQUAD_TEST_DIR")
	if testConfigDir != "" {
		isolatedSearchIndexDir := filepath.Join(testConfigDir, "search_index")
		if _, err := os.Stat(isolatedSearchIndexDir); !os.IsNotExist(err) {
			t.Errorf("NewSessionService created %q under test mode; want in-memory search engine, not disk persistence to an isolated dir either (err=%v)", isolatedSearchIndexDir, err)
		}
	}
}
