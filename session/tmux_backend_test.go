package session

// tmux_backend_test.go: Tests for TmuxBackend ProcessManager delegation.
// T-UNIT-1: GetSessionIdentifier() value correctness
// T-UNIT-2: Delegation coverage for all ~28 ProcessManager methods

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// mockTmuxManager is a test double for TmuxManager.
// Embeds TmuxManager so only the methods under test need explicit implementations.
// All other methods use the zero-value embed (nil interface method calls would panic
// but that's intentional — if a test hits an unexpected method, it fails visibly).
type mockTmuxManager struct {
	TmuxManager

	// IsAlive
	isAliveReturn bool
	isAliveCalls  int

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
	paneExitCode   int
	paneExitSignal string
	paneExitDead   bool
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

// PaneExitStatus
func (m *mockTmuxManager) PaneExitStatus() (code int, signal string, dead bool) {
	return m.paneExitCode, m.paneExitSignal, m.paneExitDead
}

// --- T-UNIT-1: GetSessionIdentifier value correctness ---

func TestTmuxBackend_GetSessionIdentifier_MatchesTmuxSessionName(t *testing.T) {
	mock := &mockTmuxManager{tmuxSessionName: "staplersquad_my-session"}
	b := NewTmuxBackend(mock)
	assert.Equal(t, "staplersquad_my-session", b.GetSessionIdentifier())
	assert.Equal(t, 1, mock.getSessionNameCalls, "must delegate to GetTmuxSessionName")
}

// --- T-UNIT-2: Delegation coverage for all ProcessManager methods ---

func TestTmuxBackend_DelegatesIsAlive(t *testing.T) {
	mock := &mockTmuxManager{isAliveReturn: true}
	b := NewTmuxBackend(mock)
	assert.True(t, b.IsAlive())
	assert.Equal(t, 1, mock.isAliveCalls, "must delegate to mgr.IsAlive()")
}

func TestTmuxBackend_DelegatesHasSession(t *testing.T) {
	mock := &mockTmuxManager{hasSessionReturn: true}
	b := NewTmuxBackend(mock)
	assert.True(t, b.HasSession())
}

func TestTmuxBackend_DelegatesStart(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.Start("/tmp")
	require.NoError(t, err)
	assert.Equal(t, 1, mock.startCalls)
}

func TestTmuxBackend_DelegatesClose(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.Close()
	require.NoError(t, err)
	assert.Equal(t, 1, mock.closeCalls)
}

func TestTmuxBackend_DelegatesRestoreWithWorkDir(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.RestoreWithWorkDir("/home/user/project")
	require.NoError(t, err)
	assert.Equal(t, 1, mock.restoreCalls)
}

func TestTmuxBackend_DelegatesDetachSafely(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.DetachSafely()
	require.NoError(t, err)
	assert.Equal(t, 1, mock.detachCalls)
}

func TestTmuxBackend_DelegatesGetPTY(t *testing.T) {
	mock := &mockTmuxManager{getPTYFile: nil, getPTYReturn: nil}
	b := NewTmuxBackend(mock)
	f, err := b.GetPTY()
	require.NoError(t, err)
	assert.Nil(t, f) // nil is valid for a stopped session
}

func TestTmuxBackend_DelegatesSendKeys(t *testing.T) {
	mock := &mockTmuxManager{sendKeysN: 5}
	b := NewTmuxBackend(mock)
	n, err := b.SendKeys("hello")
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", mock.sendKeysInput)
}

func TestTmuxBackend_DelegatesTapEnter(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.TapEnter()
	require.NoError(t, err)
	assert.Equal(t, 1, mock.tapEnterCalls)
}

func TestTmuxBackend_DelegatesSendPromptWithEnter(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.SendPromptWithEnter("test prompt")
	require.NoError(t, err)
	assert.Equal(t, "test prompt", mock.sendPromptInput)
}

func TestTmuxBackend_DelegatesSendInputViaControlMode(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	data := []byte("ctrl-c")
	err := b.SendInputViaControlMode(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, data, mock.sendInputData)
}

func TestTmuxBackend_DelegatesCapturePaneContent(t *testing.T) {
	mock := &mockTmuxManager{capturePaneReturn: "terminal output"}
	b := NewTmuxBackend(mock)
	content, err := b.CapturePaneContent()
	require.NoError(t, err)
	assert.Equal(t, "terminal output", content)
}

func TestTmuxBackend_DelegatesCapturePaneContentRaw(t *testing.T) {
	mock := &mockTmuxManager{capturePaneRawReturn: "raw\033[0m"}
	b := NewTmuxBackend(mock)
	content, err := b.CapturePaneContentRaw()
	require.NoError(t, err)
	assert.Equal(t, "raw\033[0m", content)
}

func TestTmuxBackend_DelegatesCapturePaneContentWithOptions(t *testing.T) {
	mock := &mockTmuxManager{captureWithOptsReturn: "scrollback"}
	b := NewTmuxBackend(mock)
	content, err := b.CapturePaneContentWithOptions("-100", "-")
	require.NoError(t, err)
	assert.Equal(t, "scrollback", content)
	assert.Equal(t, "-100", mock.captureWithOptsStart)
	assert.Equal(t, "-", mock.captureWithOptsEnd)
}

func TestTmuxBackend_DelegatesCaptureViewport(t *testing.T) {
	mock := &mockTmuxManager{captureViewportReturn: "viewport content"}
	b := NewTmuxBackend(mock)
	content, err := b.CaptureViewport(50)
	require.NoError(t, err)
	assert.Equal(t, "viewport content", content)
	assert.Equal(t, 50, mock.captureViewportLines)
}

func TestTmuxBackend_DelegatesGetCursorPosition(t *testing.T) {
	mock := &mockTmuxManager{cursorX: 10, cursorY: 5}
	b := NewTmuxBackend(mock)
	x, y, err := b.GetCursorPosition()
	require.NoError(t, err)
	assert.Equal(t, 10, x)
	assert.Equal(t, 5, y)
}

func TestTmuxBackend_DelegatesGetPaneDimensions(t *testing.T) {
	mock := &mockTmuxManager{paneWidth: 220, paneHeight: 50}
	b := NewTmuxBackend(mock)
	w, h, err := b.GetPaneDimensions()
	require.NoError(t, err)
	assert.Equal(t, 220, w)
	assert.Equal(t, 50, h)
}

func TestTmuxBackend_DelegatesSetWindowSize(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.SetWindowSize(200, 40)
	require.NoError(t, err)
	assert.Equal(t, 200, mock.setWindowCols)
	assert.Equal(t, 40, mock.setWindowRows)
}

func TestTmuxBackend_DelegatesSetDetachedSize(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.SetDetachedSize(150, 35, "my-session")
	require.NoError(t, err)
	assert.Equal(t, 150, mock.setDetachedW)
	assert.Equal(t, 35, mock.setDetachedH)
	assert.Equal(t, "my-session", mock.setDetachedTitle)
}

func TestTmuxBackend_DelegatesRefreshClient(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.RefreshClient()
	require.NoError(t, err)
	assert.Equal(t, 1, mock.refreshCalls)
}

func TestTmuxBackend_DelegatesGetPanePID(t *testing.T) {
	mock := &mockTmuxManager{panePIDReturn: 42}
	b := NewTmuxBackend(mock)
	pid, err := b.GetPanePID()
	require.NoError(t, err)
	assert.Equal(t, int32(42), pid)
}

func TestTmuxBackend_DelegatesHasUpdated(t *testing.T) {
	mock := &mockTmuxManager{
		hasUpdatedUpdated:   true,
		hasUpdatedHasPrompt: true,
		hasUpdatedContent:   "output",
	}
	b := NewTmuxBackend(mock)
	updated, hasPrompt, content := b.HasUpdated()
	assert.True(t, updated)
	assert.True(t, hasPrompt)
	assert.Equal(t, "output", content)
}

func TestTmuxBackend_DelegatesFilterBanners(t *testing.T) {
	mock := &mockTmuxManager{filterBannersResult: "clean", filterBannersCount: 2}
	b := NewTmuxBackend(mock)
	result, count := b.FilterBanners("banner content")
	assert.Equal(t, "clean", result)
	assert.Equal(t, 2, count)
	assert.Equal(t, "banner content", mock.filterBannersInput)
}

func TestTmuxBackend_DelegatesHasMeaningfulContent(t *testing.T) {
	mock := &mockTmuxManager{hasMeaningfulReturn: true}
	b := NewTmuxBackend(mock)
	assert.True(t, b.HasMeaningfulContent("some output"))
	assert.Equal(t, "some output", mock.hasMeaningfulInput)
}

func TestTmuxBackend_DelegatesStartControlMode(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.StartControlMode()
	require.NoError(t, err)
}

func TestTmuxBackend_DelegatesStopControlMode(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.StopControlMode()
	require.NoError(t, err)
}

func TestTmuxBackend_DelegatesSubscribeToControlModeUpdates(t *testing.T) {
	ch := make(chan []byte, 1)
	mock := &mockTmuxManager{subscribeID: "sub-123", subscribeCh: ch}
	b := NewTmuxBackend(mock)
	id, got := b.SubscribeToControlModeUpdates()
	assert.Equal(t, "sub-123", id)
	assert.Equal(t, ch, got)
}

func TestTmuxBackend_DelegatesUnsubscribeFromControlModeUpdates(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	b.UnsubscribeFromControlModeUpdates("sub-456")
	assert.Equal(t, "sub-456", mock.unsubscribeID)
}

func TestTmuxBackend_DelegatesAttach(t *testing.T) {
	done := make(chan struct{})
	mock := &mockTmuxManager{attachCh: done}
	b := NewTmuxBackend(mock)
	ch, err := b.Attach()
	require.NoError(t, err)
	assert.Equal(t, done, ch)
}

func TestTmuxBackend_DelegatesSetOnExitCallback(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	called := false
	fn := func(_ string) { called = true }
	b.SetOnExitCallback(fn)
	assert.NotNil(t, mock.exitCallback)
	mock.exitCallback("test")
	assert.True(t, called)
}

func TestTmuxBackend_DelegatesResetExitOnce(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	b.ResetExitOnce()
	assert.Equal(t, 1, mock.resetExitCalls)
}

// GetCurrentWorkingDirectory delegates via *TmuxProcessManager type assertion.
// For a non-TmuxProcessManager manager (mockTmuxManager), it returns ("", nil).
func TestTmuxBackend_DelegatesGetCurrentWorkingDirectory_MockReturnsEmpty(t *testing.T) {
	mock := &mockTmuxManager{}
	b := NewTmuxBackend(mock)
	cwd, err := b.GetCurrentWorkingDirectory()
	require.NoError(t, err)
	assert.Equal(t, "", cwd, "mock does not implement real TmuxProcessManager so cwd is empty")
}

// --- Error-path delegation tests ---

func TestTmuxBackend_Start_PropagatesError(t *testing.T) {
	want := errors.New("tmux: session launch failed")
	mock := &mockTmuxManager{startReturn: want}
	b := NewTmuxBackend(mock)
	err := b.Start("/tmp")
	assert.ErrorIs(t, err, want)
	assert.Equal(t, 1, mock.startCalls)
}

func TestTmuxBackend_Close_PropagatesError(t *testing.T) {
	want := errors.New("tmux: kill-session failed")
	mock := &mockTmuxManager{closeReturn: want}
	b := NewTmuxBackend(mock)
	err := b.Close()
	assert.ErrorIs(t, err, want)
	assert.Equal(t, 1, mock.closeCalls)
}

func TestTmuxBackend_GetPTY_PropagatesError(t *testing.T) {
	want := errors.New("tmux: no PTY available")
	mock := &mockTmuxManager{getPTYReturn: want}
	b := NewTmuxBackend(mock)
	f, err := b.GetPTY()
	assert.ErrorIs(t, err, want)
	assert.Nil(t, f)
}

func TestTmuxBackend_CapturePaneContent_PropagatesError(t *testing.T) {
	want := errors.New("tmux: capture-pane failed")
	mock := &mockTmuxManager{capturePaneErr: want}
	b := NewTmuxBackend(mock)
	content, err := b.CapturePaneContent()
	assert.ErrorIs(t, err, want)
	assert.Empty(t, content)
}

func TestTmuxBackend_Attach_PropagatesError(t *testing.T) {
	want := errors.New("tmux: attach-session failed")
	mock := &mockTmuxManager{attachErr: want}
	b := NewTmuxBackend(mock)
	ch, err := b.Attach()
	assert.ErrorIs(t, err, want)
	assert.Nil(t, ch)
}

func TestTmuxBackend_SetWindowSize_PropagatesError(t *testing.T) {
	want := errors.New("tmux: resize-window failed")
	mock := &mockTmuxManager{setWindowErr: want}
	b := NewTmuxBackend(mock)
	err := b.SetWindowSize(80, 24)
	assert.ErrorIs(t, err, want)
}

func TestTmuxBackend_SendKeys_PropagatesError(t *testing.T) {
	want := errors.New("tmux: send-keys failed")
	mock := &mockTmuxManager{sendKeysReturn: want}
	b := NewTmuxBackend(mock)
	n, err := b.SendKeys("hello")
	assert.ErrorIs(t, err, want)
	assert.Zero(t, n)
}

// compile-time check: mockTmuxManager satisfies TmuxManager (catches missed method stubs).
var _ TmuxManager = (*mockTmuxManager)(nil)
