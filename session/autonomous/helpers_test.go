package autonomous

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Test doubles and fixtures the autonomous tests share. They are copies of the unexported helpers
// that stay in package session's tests, which a leaf cannot import.

// mockTmuxManager is a test double for tmux.TmuxManager.
// Embeds tmux.TmuxManager so only the methods under test need explicit implementations.
// All other methods use the zero-value embed (nil interface method calls would panic
// but that's intentional — if a test hits an unexpected method, it fails visibly).
type mockTmuxManager struct {
	tmux.TmuxManager

	// IsAlive
	isAliveReturn bool
	isAliveCalls  int

	// DoesSessionExistNoCache — deliberately independent of isAliveReturn
	// (not just an alias for it) so tests can prove a caller reaches for
	// the no-cache check specifically, e.g. a stale cached "alive" that a
	// fresh check would report dead.
	existsNoCacheReturn bool

	// HasSession
	hasSessionReturn bool

	// GetTmuxSessionName
	tmuxSessionName     string
	getSessionNameCalls int

	// Start / Close / RestoreWithWorkDir / DetachSafely
	startReturn   error
	closeReturn   error
	restoreReturn error
	detachReturn  error
	startCalls    int
	closeCalls    int
	restoreCalls  int
	detachCalls   int

	// GetPTY
	getPTYFile   *os.File
	getPTYReturn error

	// SendKeys
	sendKeysInput  string
	sendKeysN      int
	sendKeysReturn error

	// TapEnter
	tapEnterReturn error
	tapEnterCalls  int

	// SendPromptWithEnter
	sendPromptInput  string
	sendPromptReturn error

	// SendInputViaControlMode
	sendInputData   []byte
	sendInputReturn error

	// CapturePaneContent
	capturePaneReturn string
	capturePaneErr    error

	// CapturePaneContentRaw
	capturePaneRawReturn string
	capturePaneRawErr    error

	// CapturePaneContentWithOptions
	captureWithOptsStart  string
	captureWithOptsEnd    string
	captureWithOptsReturn string
	captureWithOptsErr    error

	// CaptureViewport
	captureViewportLines  int
	captureViewportReturn string
	captureViewportErr    error

	// GetCursorPosition
	cursorX   int
	cursorY   int
	cursorErr error

	// GetPaneDimensions
	paneWidth  int
	paneHeight int
	paneDimErr error

	// SetWindowSize
	setWindowCols int
	setWindowRows int
	setWindowErr  error

	// SetDetachedSize
	setDetachedW      int
	setDetachedH      int
	setDetachedTitle  string
	setDetachedReturn error

	// RefreshClient
	refreshReturn error
	refreshCalls  int

	// GetPanePID
	panePIDReturn int32
	panePIDErr    error

	// HasUpdated
	hasUpdatedUpdated   bool
	hasUpdatedHasPrompt bool
	hasUpdatedContent   string

	// FilterBanners
	filterBannersInput  string
	filterBannersResult string
	filterBannersCount  int

	// HasMeaningfulContent
	hasMeaningfulInput  string
	hasMeaningfulReturn bool

	// StartControlMode / StopControlMode
	startCtrlReturn error
	stopCtrlReturn  error

	// SubscribeToControlModeUpdates
	subscribeID string
	subscribeCh chan []byte

	// UnsubscribeFromControlModeUpdates
	unsubscribeID string

	// Attach
	attachCh  chan struct{}
	attachErr error

	// SetOnExitCallback
	exitCallback func(string)

	// ResetExitOnce
	resetExitCalls int

	// PaneExitStatus
	paneExitCode        int
	paneExitSignal      string
	paneExitDead        bool
	paneExitStatusCalls int
}

func (m *mockTmuxManager) IsAlive() bool {
	m.isAliveCalls++
	return m.isAliveReturn
}

func (m *mockTmuxManager) HasSession() bool { return m.hasSessionReturn }

func (m *mockTmuxManager) GetTmuxSessionName() string {
	m.getSessionNameCalls++
	return m.tmuxSessionName
}

func (m *mockTmuxManager) Start(dir string) error {
	m.startCalls++
	return m.startReturn
}

func (m *mockTmuxManager) Close() error {
	m.closeCalls++
	return m.closeReturn
}

func (m *mockTmuxManager) RestoreWithWorkDir(w string) error {
	m.restoreCalls++
	return m.restoreReturn
}

func (m *mockTmuxManager) DetachSafely() error {
	m.detachCalls++
	return m.detachReturn
}

func (m *mockTmuxManager) GetPTY() (*os.File, error) {
	return m.getPTYFile, m.getPTYReturn
}

func (m *mockTmuxManager) SendKeys(keys string) (int, error) {
	m.sendKeysInput = keys
	return m.sendKeysN, m.sendKeysReturn
}

func (m *mockTmuxManager) TapEnter() error {
	m.tapEnterCalls++
	return m.tapEnterReturn
}

func (m *mockTmuxManager) SendPromptWithEnter(p string) error {
	m.sendPromptInput = p
	return m.sendPromptReturn
}

func (m *mockTmuxManager) SendInputViaControlMode(_ context.Context, data []byte) error {
	m.sendInputData = data
	return m.sendInputReturn
}

func (m *mockTmuxManager) CapturePaneContent() (string, error) {
	return m.capturePaneReturn, m.capturePaneErr
}

func (m *mockTmuxManager) CapturePaneContentRaw() (string, error) {
	return m.capturePaneRawReturn, m.capturePaneRawErr
}

func (m *mockTmuxManager) CapturePaneContentWithOptions(start, end string) (string, error) {
	m.captureWithOptsStart = start
	m.captureWithOptsEnd = end
	return m.captureWithOptsReturn, m.captureWithOptsErr
}

func (m *mockTmuxManager) CaptureViewport(lines int) (string, error) {
	m.captureViewportLines = lines
	return m.captureViewportReturn, m.captureViewportErr
}

func (m *mockTmuxManager) GetCursorPosition() (int, int, error) {
	return m.cursorX, m.cursorY, m.cursorErr
}

func (m *mockTmuxManager) GetPaneDimensions() (int, int, error) {
	return m.paneWidth, m.paneHeight, m.paneDimErr
}

func (m *mockTmuxManager) SetWindowSize(cols, rows int) error {
	m.setWindowCols = cols
	m.setWindowRows = rows
	return m.setWindowErr
}

func (m *mockTmuxManager) SetDetachedSize(w, h int, title string) error {
	m.setDetachedW = w
	m.setDetachedH = h
	m.setDetachedTitle = title
	return m.setDetachedReturn
}

func (m *mockTmuxManager) RefreshClient() error {
	m.refreshCalls++
	return m.refreshReturn
}

func (m *mockTmuxManager) GetPanePID() (int32, error) {
	return m.panePIDReturn, m.panePIDErr
}

func (m *mockTmuxManager) HasUpdated() (bool, bool, string) {
	return m.hasUpdatedUpdated, m.hasUpdatedHasPrompt, m.hasUpdatedContent
}

func (m *mockTmuxManager) FilterBanners(content string) (string, int) {
	m.filterBannersInput = content
	return m.filterBannersResult, m.filterBannersCount
}

func (m *mockTmuxManager) HasMeaningfulContent(content string) bool {
	m.hasMeaningfulInput = content
	return m.hasMeaningfulReturn
}

func (m *mockTmuxManager) StartControlMode() error { return m.startCtrlReturn }
func (m *mockTmuxManager) StopControlMode() error  { return m.stopCtrlReturn }

func (m *mockTmuxManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	return m.subscribeID, m.subscribeCh
}

func (m *mockTmuxManager) UnsubscribeFromControlModeUpdates(id string) {
	m.unsubscribeID = id
}

func (m *mockTmuxManager) Attach() (chan struct{}, error) {
	return m.attachCh, m.attachErr
}

func (m *mockTmuxManager) SetOnExitCallback(fn func(string)) {
	m.exitCallback = fn
}

func (m *mockTmuxManager) ResetExitOnce() {
	m.resetExitCalls++
}

// Provide a no-op Session/SetSession so tests embedding mockTmuxManager
// that go through code paths accessing tb.Session() don't panic.
func (m *mockTmuxManager) Session() *tmux.TmuxSession     { return nil }
func (m *mockTmuxManager) SetSession(_ *tmux.TmuxSession) {}
func (m *mockTmuxManager) DoesSessionExist() bool         { return m.isAliveReturn }
func (m *mockTmuxManager) DoesSessionExistNoCache() bool  { return m.existsNoCacheReturn }

// PaneExitStatus
func (m *mockTmuxManager) PaneExitStatus() (code int, signal string, dead bool) {
	m.paneExitStatusCalls++
	return m.paneExitCode, m.paneExitSignal, m.paneExitDead
}

// makeTestInstance creates a minimal Instance for testing (no tmux, not started).
func makeTestInstance(title string) *instance.Instance {
	return &instance.Instance{
		Title:  title,
		Status: domain.Active,
	}
}

// minimalInstance builds a bare Instance safe for unit tests (no tmux, no disk I/O).
func minimalInstance(t *testing.T) *instance.Instance {
	t.Helper()
	i := &instance.Instance{
		Title:       t.Name(),
		Status:      domain.Creating,
		Program:     "echo",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		IsManaged:   true,
		Permissions: domain.GetManagedPermissions(),
	}
	i.InitTagManagerForTest()
	instance.FinishInstanceConstruction(i)
	return i
}

// newDiagnoseNudgeTestFixture spins up a Storage (and its backing repository) with one open
// BacklogStuckState row (reason: bouncing) for a fresh backlog item.
func newDiagnoseNudgeTestFixture(t *testing.T) (storage *session.Storage, repo *session.EntRepository, itemID string, ctx context.Context) {
	t.Helper()
	repo = session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx = context.Background()

	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:  "stuck-state test item",
		Status: string(domain.BacklogStatusInProgress),
	})
	require.NoError(t, err)
	itemID = item.ID
	_, err = repo.MarkStuck(ctx, itemID, domain.StuckReasonBouncing, domain.BacklogStatusInProgress, "bouncing")
	require.NoError(t, err)
	return storage, repo, itemID, ctx
}

// fakeTmuxSocketQuerier is an in-memory tmux.SocketQuerier for tests. It lets tests
// declare independent live-session sets and down/up states per socket, so
// multi-socket behavior (e.g. reconcileSessions, CheckAllSessions) is exercisable
// without a real tmux server.
type fakeTmuxSocketQuerier struct {
	mu       sync.Mutex
	sessions map[string]map[string]bool // socket -> set of live session names
	down     map[string]bool            // socket -> is server down
	calls    []string                   // sockets passed to ListSessions/IsServerDown, in call order
}

func newFakeTmuxSocketQuerier() *fakeTmuxSocketQuerier {
	return &fakeTmuxSocketQuerier{
		sessions: make(map[string]map[string]bool),
		down:     make(map[string]bool),
	}
}

func (f *fakeTmuxSocketQuerier) setDown(serverSocket string, down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down[serverSocket] = down
}

// socketsQueried returns the distinct sockets passed to either method, in first-seen
// order. Tests use this to assert each socket was queried independently rather than
// one socket's result being reused for another.
func (f *fakeTmuxSocketQuerier) socketsQueried() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := make(map[string]bool, len(f.calls))
	out := make([]string, 0, len(f.calls))
	for _, s := range f.calls {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (f *fakeTmuxSocketQuerier) ListSessions(serverSocket string) (map[string]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, serverSocket)
	if f.down[serverSocket] {
		return nil, tmux.ErrServerDown
	}
	set := f.sessions[serverSocket]
	result := make(map[string]bool, len(set))
	for k, v := range set {
		result[k] = v
	}
	return result, nil
}

func (f *fakeTmuxSocketQuerier) IsServerDown(serverSocket string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, serverSocket)
	return f.down[serverSocket]
}
