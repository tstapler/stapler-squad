package driver

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Test doubles and fixtures the driver tests share. They are copies of the unexported helpers
// that stay in package session's tests, which a leaf cannot import.

// mockTmuxManager is a test double for TmuxManager.
// Embeds TmuxManager so only the methods under test need explicit implementations.
// All other methods use the zero-value embed (nil interface method calls would panic
// but that's intentional — if a test hits an unexpected method, it fails visibly).
type mockTmuxManager struct {
	session.TmuxManager

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

// newFakeLease returns a held lease bound to fixturetest.FakeLeaseOwner, taken from a
// throwaway instance so each call owns an independent lease.
func newFakeLease(t *testing.T) *instance.HeldLease {
	t.Helper()
	inst := &instance.Instance{UUID: fixturetest.FakeLeaseOwner}
	l, ok := inst.TryTerminalWriteLease(instance.LeaseWriterOther)
	require.True(t, ok)
	return l
}

// makeTestInstance creates a minimal Instance for testing (no tmux, not started).
func makeTestInstance(title string) *instance.Instance {
	return &instance.Instance{
		Title:  title,
		Status: domain.Active,
	}
}

// createTestStorage returns a Storage over a fresh in-memory repository; the repository closes with t.
func createTestStorage(t *testing.T) *session.Storage {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return storage
}

func newPRTestInstance(t *testing.T) *instance.Instance {
	t.Helper()
	inst := &instance.Instance{
		Title: "multi-pr", UUID: "uuid-multi-pr", Path: t.TempDir(), Program: "echo", Branch: "feat/x",
		GitHubOwner: "acme", GitHubRepo: "app",
	}
	instance.FinishInstanceConstruction(inst)
	return inst
}

func attachNumbers(t *testing.T, inst *instance.Instance) []int {
	t.Helper()
	prs := inst.Snapshot().GitHub.PRs
	out := make([]int, 0, len(prs))
	for _, v := range prs {
		out = append(out, v.Ref.Number())
	}
	return out
}

// persistedPRSession adds a started session with a uuid to a fresh storage and
// returns both plus the instance.
func persistedPRSession(t *testing.T) (*session.Storage, *instance.Instance) {
	t.Helper()
	storage := createTestStorage(t)
	inst := &instance.Instance{Title: "pr-session", UUID: "uuid-pr-session", Path: t.TempDir(), Program: "echo",
		GitHubOwner: "acme", GitHubRepo: "app", Branch: "feat/x"}
	instance.FinishInstanceConstruction(inst)
	require.NoError(t, storage.AddInstance(inst))
	return storage, inst
}

// countLinks counts the session's PR links as the storage reports them.
func countLinks(t *testing.T, storage *session.Storage, uuid string) int {
	t.Helper()
	views, err := storage.ListSessionPRViewsByUUIDs(context.Background(), []string{uuid})
	require.NoError(t, err)
	return len(views[uuid])
}

// prLoadFixture creates a persisted Paused session row with an optional legacy PR.
func prLoadFixture(t *testing.T, storage *session.Storage, title, uuid, owner, repo string, legacyNumber int) {
	t.Helper()
	data := domain.InstanceData{
		Title: title, UUID: uuid, Path: "/tmp/test", Status: domain.Paused, Program: "claude",
		CreatedAt: time.Now(), UpdatedAt: time.Now(), GitHubOwner: owner, GitHubRepo: repo,
	}
	if legacyNumber > 0 {
		data.GitHubPRNumber = legacyNumber
		data.GitHubPRURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, legacyNumber)
	}
	require.NoError(t, storage.Repo.Create(context.Background(), data))
}

// fakePRPersister records AttachSessionPR calls; err and res are returned as set.
type fakePRPersister struct {
	mu    sync.Mutex
	calls []fakePRCall
	err   error
	res   domain.PRLinkResult
	// terminalWrites records UpdateGitHubPRStatusTerminal calls; terminalErr is returned from them.
	terminalWrites []terminalWrite
	terminalErr    error
}

type terminalWrite struct {
	title    string
	terminal bool
}

type fakePRCall struct {
	uuid   string
	ref    domain.PRRef
	link   domain.PRLink
	mirror instance.PRMirror
}

func (f *fakePRPersister) AttachSessionPR(_ context.Context, uuid string, ref domain.PRRef, link domain.PRLink, m instance.PRMirror) (domain.PRLinkResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakePRCall{uuid, ref, link, m})
	return f.res, f.err
}

func (f *fakePRPersister) UpdateGitHubPRStatusTerminal(title string, terminal bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminalWrites = append(f.terminalWrites, terminalWrite{title, terminal})
	return f.terminalErr
}

func (f *fakePRPersister) PersistPRMirror(string, string, int, bool) error { return nil }

func (f *fakePRPersister) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func checkPTYAvailable(t *testing.T) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "bash", "-c", "exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if _, err := pty.Start(cmd); err != nil {
		_ = cmd.Wait()
		t.Skipf("PTY+Setpgid not available in this environment: %v", err)
	}
	_ = cmd.Wait()
}

var _ session.TmuxManager = (*mockTmuxManager)(nil)
