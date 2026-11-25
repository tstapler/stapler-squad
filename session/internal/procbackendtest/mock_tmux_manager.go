// Package procbackendtest holds the shared test doubles for the tmux, tymux and
// process-manager backends. It is imported only from _test.go files (see the
// procbackendtest depguard row) and never imports session.
package procbackendtest

import (
	"context"
	"os"

	"github.com/tstapler/stapler-squad/session/tmux"
)

// MockTmuxManager is a test double for tmux.TmuxManager.
// Embeds tmux.TmuxManager so only the methods under test need explicit implementations.
// All other methods use the zero-value embed (nil interface method calls would panic
// but that's intentional — if a test hits an unexpected method, it fails visibly).
type MockTmuxManager struct {
	tmux.TmuxManager

	// IsAlive
	IsAliveReturn bool
	IsAliveCalls  int

	// DoesSessionExistNoCache — deliberately independent of isAliveReturn
	// (not just an alias for it) so tests can prove a caller reaches for
	// the no-cache check specifically, e.g. a stale cached "alive" that a
	// fresh check would report dead.
	ExistsNoCacheReturn bool

	// HasSession
	HasSessionReturn bool

	// GetTmuxSessionName
	TmuxSessionName     string
	GetSessionNameCalls int

	// Start / Close / RestoreWithWorkDir / DetachSafely
	StartReturn   error
	CloseReturn   error
	RestoreReturn error
	DetachReturn  error
	StartCalls    int
	CloseCalls    int
	RestoreCalls  int
	DetachCalls   int

	// GetPTY
	GetPTYFile   *os.File
	GetPTYReturn error

	// SendKeys
	SendKeysInput  string
	SendKeysN      int
	SendKeysReturn error

	// TapEnter
	TapEnterReturn error
	TapEnterCalls  int

	// SendPromptWithEnter
	SendPromptInput  string
	SendPromptReturn error

	// SendInputViaControlMode
	SendInputData   []byte
	SendInputReturn error

	// CapturePaneContent
	CapturePaneReturn string
	CapturePaneErr    error

	// CapturePaneContentRaw
	CapturePaneRawReturn string
	CapturePaneRawErr    error

	// CapturePaneContentWithOptions
	CaptureWithOptsStart  string
	CaptureWithOptsEnd    string
	CaptureWithOptsReturn string
	CaptureWithOptsErr    error

	// CaptureViewport
	CaptureViewportLines  int
	CaptureViewportReturn string
	CaptureViewportErr    error

	// GetCursorPosition
	CursorX   int
	CursorY   int
	CursorErr error

	// GetPaneDimensions
	PaneWidth  int
	PaneHeight int
	PaneDimErr error

	// SetWindowSize
	SetWindowCols int
	SetWindowRows int
	SetWindowErr  error

	// SetDetachedSize
	SetDetachedW      int
	SetDetachedH      int
	SetDetachedTitle  string
	SetDetachedReturn error

	// RefreshClient
	RefreshReturn error
	RefreshCalls  int

	// GetPanePID
	PanePIDReturn int32
	PanePIDErr    error

	// HasUpdated
	HasUpdatedUpdated   bool
	HasUpdatedHasPrompt bool
	HasUpdatedContent   string

	// FilterBanners
	FilterBannersInput  string
	FilterBannersResult string
	FilterBannersCount  int

	// HasMeaningfulContent
	HasMeaningfulInput  string
	HasMeaningfulReturn bool

	// StartControlMode / StopControlMode
	StartCtrlReturn error
	StopCtrlReturn  error

	// SubscribeToControlModeUpdates
	SubscribeID string
	SubscribeCh chan []byte

	// UnsubscribeFromControlModeUpdates
	UnsubscribeID string

	// Attach
	AttachCh  chan struct{}
	AttachErr error

	// SetOnExitCallback
	ExitCallback func(string)

	// ResetExitOnce
	ResetExitCalls int

	// PaneExitStatus
	PaneExitCode        int
	PaneExitSignal      string
	PaneExitDead        bool
	PaneExitStatusCalls int
}

func (m *MockTmuxManager) IsAlive() bool {
	m.IsAliveCalls++
	return m.IsAliveReturn
}

func (m *MockTmuxManager) HasSession() bool { return m.HasSessionReturn }

func (m *MockTmuxManager) GetTmuxSessionName() string {
	m.GetSessionNameCalls++
	return m.TmuxSessionName
}

func (m *MockTmuxManager) Start(dir string) error {
	m.StartCalls++
	return m.StartReturn
}

func (m *MockTmuxManager) Close() error {
	m.CloseCalls++
	return m.CloseReturn
}

func (m *MockTmuxManager) RestoreWithWorkDir(w string) error {
	m.RestoreCalls++
	return m.RestoreReturn
}

func (m *MockTmuxManager) DetachSafely() error {
	m.DetachCalls++
	return m.DetachReturn
}

func (m *MockTmuxManager) GetPTY() (*os.File, error) {
	return m.GetPTYFile, m.GetPTYReturn
}

func (m *MockTmuxManager) SendKeys(keys string) (int, error) {
	m.SendKeysInput = keys
	return m.SendKeysN, m.SendKeysReturn
}

func (m *MockTmuxManager) TapEnter() error {
	m.TapEnterCalls++
	return m.TapEnterReturn
}

func (m *MockTmuxManager) SendPromptWithEnter(p string) error {
	m.SendPromptInput = p
	return m.SendPromptReturn
}

func (m *MockTmuxManager) SendInputViaControlMode(_ context.Context, data []byte) error {
	m.SendInputData = data
	return m.SendInputReturn
}

func (m *MockTmuxManager) CapturePaneContent() (string, error) {
	return m.CapturePaneReturn, m.CapturePaneErr
}

func (m *MockTmuxManager) CapturePaneContentRaw() (string, error) {
	return m.CapturePaneRawReturn, m.CapturePaneRawErr
}

func (m *MockTmuxManager) CapturePaneContentWithOptions(start, end string) (string, error) {
	m.CaptureWithOptsStart = start
	m.CaptureWithOptsEnd = end
	return m.CaptureWithOptsReturn, m.CaptureWithOptsErr
}

func (m *MockTmuxManager) CaptureViewport(lines int) (string, error) {
	m.CaptureViewportLines = lines
	return m.CaptureViewportReturn, m.CaptureViewportErr
}

func (m *MockTmuxManager) GetCursorPosition() (int, int, error) {
	return m.CursorX, m.CursorY, m.CursorErr
}

func (m *MockTmuxManager) GetPaneDimensions() (int, int, error) {
	return m.PaneWidth, m.PaneHeight, m.PaneDimErr
}

func (m *MockTmuxManager) SetWindowSize(cols, rows int) error {
	m.SetWindowCols = cols
	m.SetWindowRows = rows
	return m.SetWindowErr
}

func (m *MockTmuxManager) SetDetachedSize(w, h int, title string) error {
	m.SetDetachedW = w
	m.SetDetachedH = h
	m.SetDetachedTitle = title
	return m.SetDetachedReturn
}

func (m *MockTmuxManager) RefreshClient() error {
	m.RefreshCalls++
	return m.RefreshReturn
}

func (m *MockTmuxManager) GetPanePID() (int32, error) {
	return m.PanePIDReturn, m.PanePIDErr
}

func (m *MockTmuxManager) HasUpdated() (bool, bool, string) {
	return m.HasUpdatedUpdated, m.HasUpdatedHasPrompt, m.HasUpdatedContent
}

func (m *MockTmuxManager) FilterBanners(content string) (string, int) {
	m.FilterBannersInput = content
	return m.FilterBannersResult, m.FilterBannersCount
}

func (m *MockTmuxManager) HasMeaningfulContent(content string) bool {
	m.HasMeaningfulInput = content
	return m.HasMeaningfulReturn
}

func (m *MockTmuxManager) StartControlMode() error { return m.StartCtrlReturn }
func (m *MockTmuxManager) StopControlMode() error  { return m.StopCtrlReturn }

func (m *MockTmuxManager) SubscribeToControlModeUpdates() (string, chan []byte) {
	return m.SubscribeID, m.SubscribeCh
}

func (m *MockTmuxManager) UnsubscribeFromControlModeUpdates(id string) {
	m.UnsubscribeID = id
}

func (m *MockTmuxManager) Attach() (chan struct{}, error) {
	return m.AttachCh, m.AttachErr
}

func (m *MockTmuxManager) SetOnExitCallback(fn func(string)) {
	m.ExitCallback = fn
}

func (m *MockTmuxManager) ResetExitOnce() {
	m.ResetExitCalls++
}

// Provide a no-op Session/SetSession so tests embedding MockTmuxManager
// that go through code paths accessing tb.Session() don't panic.
func (m *MockTmuxManager) Session() *tmux.TmuxSession     { return nil }
func (m *MockTmuxManager) SetSession(_ *tmux.TmuxSession) {}
func (m *MockTmuxManager) DoesSessionExist() bool         { return m.IsAliveReturn }
func (m *MockTmuxManager) DoesSessionExistNoCache() bool  { return m.ExistsNoCacheReturn }

// PaneExitStatus
func (m *MockTmuxManager) PaneExitStatus() (code int, signal string, dead bool) {
	m.PaneExitStatusCalls++
	return m.PaneExitCode, m.PaneExitSignal, m.PaneExitDead
}
