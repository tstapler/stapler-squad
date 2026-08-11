package services

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/search"
)

// TestNewSessionService_TestMode_NeverTouchesRealSearchIndex guards the
// server/services and server/mcp CI timeout fix: NewSessionService used to always
// call search.NewIndexStore() and gob-decode whatever index was on disk,
// which under a `go test` binary meant every test in the package shared and
// grew one persisted index for the life of the binary -- slow enough under
// -race to blow CI's 150s budget. Under config.IsTestMode() (true for every
// go test binary), NewSessionService must use an in-memory search engine and
// never touch disk for the index at all.
//
// STAPLER_SQUAD_TEST_DIR is deliberately left unset so config.GetConfigDir()
// falls through to its IsTestMode() branch (config/config.go:159-166), which
// resolves to $HOME/.stapler-squad/test/test-<pid> -- the exact isolated
// directory a regression back to the old search.NewIndexStore()-always path
// would write search_index/ into (session/search/index_store.go:49-56). $HOME
// is stubbed to a t.TempDir() so that specific search_index/ subdirectory can
// be asserted precisely, rather than checking the production-only
// $HOME/.claude/search_index path, which config.IsTestMode() makes
// unreachable from any go test binary regardless of this fix. The parent
// test-<pid> dir itself is not asserted against, since other NewSessionService
// dependencies (approval store, capacity config, etc.) legitimately use
// config.GetConfigDir() for their own unrelated state.
func TestNewSessionService_TestMode_NeverTouchesRealSearchIndex(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	// t.Setenv can only set a value, not unset one, so clear any inherited
	// STAPLER_SQUAD_TEST_DIR explicitly and restore it afterward -- config.GetConfigDir()
	// currently uses os.Getenv (config/config.go:125), for which "" and unset are
	// equivalent, but os.Unsetenv keeps this test correct even if that ever changes to
	// os.LookupEnv.
	if orig, ok := os.LookupEnv("STAPLER_SQUAD_TEST_DIR"); ok {
		os.Unsetenv("STAPLER_SQUAD_TEST_DIR")
		t.Cleanup(func() { os.Setenv("STAPLER_SQUAD_TEST_DIR", orig) })
	}

	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	if svc == nil {
		t.Fatal("NewSessionService returned nil")
	}

	pidTestDir := filepath.Join(fakeHome, ".stapler-squad", "test", "test-"+strconv.Itoa(os.Getpid()))
	searchIndexDir := filepath.Join(pidTestDir, "search_index")
	if _, err := os.Stat(searchIndexDir); !os.IsNotExist(err) {
		t.Errorf("NewSessionService created %q in test mode; want no disk persistence for the search index at all (err=%v)", searchIndexDir, err)
	}
}

// TestNewSessionServiceWithSearchEngine_UsesInjectedEngine confirms the DI seam actually
// wires the caller's engine through, rather than silently falling back to
// newDefaultSearchEngine()'s config.IsTestMode() branching.
func TestNewSessionServiceWithSearchEngine_UsesInjectedEngine(t *testing.T) {
	storage := createTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	injected := search.NewSearchEngine()

	svc := NewSessionServiceWithSearchEngine(storage, bus, injected)
	t.Cleanup(func() { svc.Shutdown() })
	if svc == nil {
		t.Fatal("NewSessionServiceWithSearchEngine returned nil")
	}
	if svc.searchSvc.searchEngine != injected {
		t.Error("NewSessionServiceWithSearchEngine did not wire through the injected search engine")
	}
}
