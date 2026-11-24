package session

// tmux_backend_test.go: Tests for TmuxBackend ProcessManager delegation.
// T-UNIT-1: GetSessionIdentifier() value correctness
// T-UNIT-2: Delegation coverage for all ~28 ProcessManager methods

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/internal/procbackendtest"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// --- T-UNIT-1: GetSessionIdentifier value correctness ---

func TestTmuxBackend_GetSessionIdentifier_MatchesTmuxSessionName(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{TmuxSessionName: "staplersquad_my-session"}
	b := NewTmuxBackend(mock)
	assert.Equal(t, "staplersquad_my-session", b.GetSessionIdentifier())
	assert.Equal(t, 1, mock.GetSessionNameCalls, "must delegate to GetTmuxSessionName")
}

// --- T-UNIT-2: Delegation coverage for all ProcessManager methods ---

func TestTmuxBackend_DelegatesIsAlive(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{IsAliveReturn: true}
	b := NewTmuxBackend(mock)
	assert.True(t, b.IsAlive())
	assert.Equal(t, 1, mock.IsAliveCalls, "must delegate to mgr.IsAlive()")
}

func TestTmuxBackend_DelegatesHasSession(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{HasSessionReturn: true}
	b := NewTmuxBackend(mock)
	assert.True(t, b.HasSession())
}

func TestTmuxBackend_DelegatesStart(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.Start("/tmp")
	require.NoError(t, err)
	assert.Equal(t, 1, mock.StartCalls)
}

func TestTmuxBackend_DelegatesClose(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.Close()
	require.NoError(t, err)
	assert.Equal(t, 1, mock.CloseCalls)
}

func TestTmuxBackend_DelegatesRestoreWithWorkDir(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.RestoreWithWorkDir("/home/user/project")
	require.NoError(t, err)
	assert.Equal(t, 1, mock.RestoreCalls)
}

func TestTmuxBackend_DelegatesDetachSafely(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.DetachSafely()
	require.NoError(t, err)
	assert.Equal(t, 1, mock.DetachCalls)
}

func TestTmuxBackend_DelegatesGetPTY(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{GetPTYFile: nil, GetPTYReturn: nil}
	b := NewTmuxBackend(mock)
	f, err := b.GetPTY()
	require.NoError(t, err)
	assert.Nil(t, f) // nil is valid for a stopped session
}

func TestTmuxBackend_DelegatesSendKeys(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{SendKeysN: 5}
	b := NewTmuxBackend(mock)
	n, err := b.SendKeys("hello")
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", mock.SendKeysInput)
}

func TestTmuxBackend_DelegatesTapEnter(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.TapEnter()
	require.NoError(t, err)
	assert.Equal(t, 1, mock.TapEnterCalls)
}

func TestTmuxBackend_DelegatesSendPromptWithEnter(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.SendPromptWithEnter("test prompt")
	require.NoError(t, err)
	assert.Equal(t, "test prompt", mock.SendPromptInput)
}

func TestTmuxBackend_DelegatesSendInputViaControlMode(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	data := []byte("ctrl-c")
	err := b.SendInputViaControlMode(context.Background(), data)
	require.NoError(t, err)
	assert.Equal(t, data, mock.SendInputData)
}

func TestTmuxBackend_DelegatesCapturePaneContent(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{CapturePaneReturn: "terminal output"}
	b := NewTmuxBackend(mock)
	content, err := b.CapturePaneContent()
	require.NoError(t, err)
	assert.Equal(t, "terminal output", content)
}

func TestTmuxBackend_DelegatesCapturePaneContentRaw(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{CapturePaneRawReturn: "raw\033[0m"}
	b := NewTmuxBackend(mock)
	content, err := b.CapturePaneContentRaw()
	require.NoError(t, err)
	assert.Equal(t, "raw\033[0m", content)
}

func TestTmuxBackend_DelegatesCapturePaneContentWithOptions(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{CaptureWithOptsReturn: "scrollback"}
	b := NewTmuxBackend(mock)
	content, err := b.CapturePaneContentWithOptions("-100", "-")
	require.NoError(t, err)
	assert.Equal(t, "scrollback", content)
	assert.Equal(t, "-100", mock.CaptureWithOptsStart)
	assert.Equal(t, "-", mock.CaptureWithOptsEnd)
}

func TestTmuxBackend_DelegatesCaptureViewport(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{CaptureViewportReturn: "viewport content"}
	b := NewTmuxBackend(mock)
	content, err := b.CaptureViewport(50)
	require.NoError(t, err)
	assert.Equal(t, "viewport content", content)
	assert.Equal(t, 50, mock.CaptureViewportLines)
}

func TestTmuxBackend_DelegatesGetCursorPosition(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{CursorX: 10, CursorY: 5}
	b := NewTmuxBackend(mock)
	x, y, err := b.GetCursorPosition()
	require.NoError(t, err)
	assert.Equal(t, 10, x)
	assert.Equal(t, 5, y)
}

func TestTmuxBackend_DelegatesGetPaneDimensions(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{PaneWidth: 220, PaneHeight: 50}
	b := NewTmuxBackend(mock)
	w, h, err := b.GetPaneDimensions()
	require.NoError(t, err)
	assert.Equal(t, 220, w)
	assert.Equal(t, 50, h)
}

func TestTmuxBackend_DelegatesSetWindowSize(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.SetWindowSize(200, 40)
	require.NoError(t, err)
	assert.Equal(t, 200, mock.SetWindowCols)
	assert.Equal(t, 40, mock.SetWindowRows)
}

func TestTmuxBackend_DelegatesSetDetachedSize(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.SetDetachedSize(150, 35, "my-session")
	require.NoError(t, err)
	assert.Equal(t, 150, mock.SetDetachedW)
	assert.Equal(t, 35, mock.SetDetachedH)
	assert.Equal(t, "my-session", mock.SetDetachedTitle)
}

func TestTmuxBackend_DelegatesRefreshClient(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.RefreshClient()
	require.NoError(t, err)
	assert.Equal(t, 1, mock.RefreshCalls)
}

func TestTmuxBackend_DelegatesGetPanePID(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{PanePIDReturn: 42}
	b := NewTmuxBackend(mock)
	pid, err := b.GetPanePID()
	require.NoError(t, err)
	assert.Equal(t, int32(42), pid)
}

func TestTmuxBackend_DelegatesHasUpdated(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{
		HasUpdatedUpdated:   true,
		HasUpdatedHasPrompt: true,
		HasUpdatedContent:   "output",
	}
	b := NewTmuxBackend(mock)
	updated, hasPrompt, content := b.HasUpdated()
	assert.True(t, updated)
	assert.True(t, hasPrompt)
	assert.Equal(t, "output", content)
}

func TestTmuxBackend_DelegatesFilterBanners(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{FilterBannersResult: "clean", FilterBannersCount: 2}
	b := NewTmuxBackend(mock)
	result, count := b.FilterBanners("banner content")
	assert.Equal(t, "clean", result)
	assert.Equal(t, 2, count)
	assert.Equal(t, "banner content", mock.FilterBannersInput)
}

func TestTmuxBackend_DelegatesHasMeaningfulContent(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{HasMeaningfulReturn: true}
	b := NewTmuxBackend(mock)
	assert.True(t, b.HasMeaningfulContent("some output"))
	assert.Equal(t, "some output", mock.HasMeaningfulInput)
}

func TestTmuxBackend_DelegatesStartControlMode(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.StartControlMode()
	require.NoError(t, err)
}

func TestTmuxBackend_DelegatesStopControlMode(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	err := b.StopControlMode()
	require.NoError(t, err)
}

func TestTmuxBackend_DelegatesSubscribeToControlModeUpdates(t *testing.T) {
	t.Parallel()
	ch := make(chan []byte, 1)
	mock := &procbackendtest.MockTmuxManager{SubscribeID: "sub-123", SubscribeCh: ch}
	b := NewTmuxBackend(mock)
	id, got := b.SubscribeToControlModeUpdates()
	assert.Equal(t, "sub-123", id)
	assert.Equal(t, ch, got)
}

func TestTmuxBackend_DelegatesUnsubscribeFromControlModeUpdates(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	b.UnsubscribeFromControlModeUpdates("sub-456")
	assert.Equal(t, "sub-456", mock.UnsubscribeID)
}

func TestTmuxBackend_DelegatesAttach(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	mock := &procbackendtest.MockTmuxManager{AttachCh: done}
	b := NewTmuxBackend(mock)
	ch, err := b.Attach()
	require.NoError(t, err)
	assert.Equal(t, done, ch)
}

func TestTmuxBackend_DelegatesSetOnExitCallback(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	called := false
	fn := func(_ string) { called = true }
	b.SetOnExitCallback(fn)
	assert.NotNil(t, mock.ExitCallback)
	mock.ExitCallback("test")
	assert.True(t, called)
}

func TestTmuxBackend_DelegatesResetExitOnce(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	b.ResetExitOnce()
	assert.Equal(t, 1, mock.ResetExitCalls)
}

// GetCurrentWorkingDirectory delegates via *TmuxProcessManager type assertion.
// For a non-TmuxProcessManager manager (procbackendtest.MockTmuxManager), it returns ("", nil).
func TestTmuxBackend_DelegatesGetCurrentWorkingDirectory_MockReturnsEmpty(t *testing.T) {
	t.Parallel()
	mock := &procbackendtest.MockTmuxManager{}
	b := NewTmuxBackend(mock)
	cwd, err := b.GetCurrentWorkingDirectory()
	require.NoError(t, err)
	assert.Equal(t, "", cwd, "mock does not implement real TmuxProcessManager so cwd is empty")
}

// --- Error-path delegation tests ---

func TestTmuxBackend_Start_PropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("tmux: session launch failed")
	mock := &procbackendtest.MockTmuxManager{StartReturn: want}
	b := NewTmuxBackend(mock)
	err := b.Start("/tmp")
	assert.ErrorIs(t, err, want)
	assert.Equal(t, 1, mock.StartCalls)
}

func TestTmuxBackend_Close_PropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("tmux: kill-session failed")
	mock := &procbackendtest.MockTmuxManager{CloseReturn: want}
	b := NewTmuxBackend(mock)
	err := b.Close()
	assert.ErrorIs(t, err, want)
	assert.Equal(t, 1, mock.CloseCalls)
}

func TestTmuxBackend_GetPTY_PropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("tmux: no PTY available")
	mock := &procbackendtest.MockTmuxManager{GetPTYReturn: want}
	b := NewTmuxBackend(mock)
	f, err := b.GetPTY()
	assert.ErrorIs(t, err, want)
	assert.Nil(t, f)
}

func TestTmuxBackend_CapturePaneContent_PropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("tmux: capture-pane failed")
	mock := &procbackendtest.MockTmuxManager{CapturePaneErr: want}
	b := NewTmuxBackend(mock)
	content, err := b.CapturePaneContent()
	assert.ErrorIs(t, err, want)
	assert.Empty(t, content)
}

func TestTmuxBackend_Attach_PropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("tmux: attach-session failed")
	mock := &procbackendtest.MockTmuxManager{AttachErr: want}
	b := NewTmuxBackend(mock)
	ch, err := b.Attach()
	assert.ErrorIs(t, err, want)
	assert.Nil(t, ch)
}

func TestTmuxBackend_SetWindowSize_PropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("tmux: resize-window failed")
	mock := &procbackendtest.MockTmuxManager{SetWindowErr: want}
	b := NewTmuxBackend(mock)
	err := b.SetWindowSize(80, 24)
	assert.ErrorIs(t, err, want)
}

func TestTmuxBackend_SendKeys_PropagatesError(t *testing.T) {
	t.Parallel()
	want := errors.New("tmux: send-keys failed")
	mock := &procbackendtest.MockTmuxManager{SendKeysReturn: want}
	b := NewTmuxBackend(mock)
	n, err := b.SendKeys("hello")
	assert.ErrorIs(t, err, want)
	assert.Zero(t, n)
}

// compile-time check: procbackendtest.MockTmuxManager satisfies tmux.TmuxManager (catches missed method stubs).
var _ tmux.TmuxManager = (*procbackendtest.MockTmuxManager)(nil)
