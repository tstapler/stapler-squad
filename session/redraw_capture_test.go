package session

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// redrawManager is a TymuxManager whose only working method is CapturePaneRedraw.
type redrawManager struct {
	tymux.TymuxManager
	data      string
	altScreen bool
	ok        bool
	err       error
	calls     int
}

func (m *redrawManager) CapturePaneRedraw(context.Context) (string, bool, bool, error) {
	m.calls++
	return m.data, m.altScreen, m.ok, m.err
}

func redrawInstance(mgr tymux.TymuxManager, status Status) *Instance {
	inst := &Instance{Title: "redraw", UUID: "uuid-redraw", Status: status}
	inst.processManager = NewTymuxBackend(mgr)
	inst.started.Store(true)
	return inst
}

func TestTymuxBackend_CapturePaneRedraw_should_PassThroughEveryResult(t *testing.T) {
	t.Parallel()
	cases := map[string]*redrawManager{
		"redraw":          {data: "d", altScreen: true, ok: true},
		"declined":        {ok: false},
		"capture failure": {err: errors.New("daemon gone")},
	}
	for name, mgr := range cases {
		mgr := mgr
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, alt, ok, err := NewTymuxBackend(mgr).CapturePaneRedraw(context.Background())
			assert.Equal(t, mgr.data, data)
			assert.Equal(t, mgr.altScreen, alt)
			assert.Equal(t, mgr.ok, ok)
			assert.Equal(t, mgr.err, err)
		})
	}
}

func TestInstance_CapturePaneRedraw_should_ReturnFrameAndRecordAltScreen(t *testing.T) {
	t.Parallel()
	mgr := &redrawManager{data: "\x1b[?1049hredraw", altScreen: true, ok: true}
	inst := redrawInstance(mgr, Running)

	snap, ok, err := inst.CapturePaneRedraw(context.Background())

	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, streamhub.FullStateSnapshot{Data: "\x1b[?1049hredraw", AltScreen: true}, snap)
	assert.True(t, inst.GetAltScreenBootstrapped())
	assert.True(t, inst.GetAltScreenActive(), "AppScrollGate's alt-screen flag must follow the daemon's")
}

func TestInstance_CapturePaneRedraw_should_DeclineWithoutTouchingAltScreen_When_BackendHasNoState(t *testing.T) {
	t.Parallel()
	mgr := &redrawManager{altScreen: true, ok: false} // older daemon: no Modes
	inst := redrawInstance(mgr, Running)

	snap, ok, err := inst.CapturePaneRedraw(context.Background())

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, streamhub.FullStateSnapshot{}, snap)
	assert.False(t, inst.GetAltScreenBootstrapped(), "a declined redraw must not stamp alt-screen state")
}

func TestInstance_CapturePaneRedraw_should_PropagateCaptureError(t *testing.T) {
	t.Parallel()
	inst := redrawInstance(&redrawManager{ok: true, err: errors.New("daemon gone")}, Running)

	_, ok, err := inst.CapturePaneRedraw(context.Background())

	require.Error(t, err)
	assert.False(t, ok)
	assert.False(t, inst.GetAltScreenBootstrapped())
}

func TestInstance_CapturePaneRedraw_should_NotTouchPane_When_NotStartedOrPaused(t *testing.T) {
	t.Parallel()
	notStarted := redrawInstance(&redrawManager{ok: true}, Running)
	notStarted.started.Store(false)
	cases := map[string]*Instance{
		"not started": notStarted,
		"paused":      redrawInstance(&redrawManager{ok: true}, Paused),
	}
	for name, inst := range cases {
		inst := inst
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, ok, err := inst.CapturePaneRedraw(context.Background())
			assert.ErrorIs(t, err, streamhub.ErrSessionNotStarted)
			assert.False(t, ok)
			assert.Equal(t, 0, inst.processManager.(*TymuxBackend).mgr.(*redrawManager).calls)
		})
	}
}

func TestInstance_CapturePaneRedraw_should_DeclineWithoutTouchingPane_When_BackendHasNoCapability(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "tmuxish", UUID: "uuid-tmuxish", Status: Running}
	inst.processManager = &fakeSendKeysProcessManager{}
	inst.started.Store(true)

	snap, ok, err := inst.CapturePaneRedraw(context.Background())

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, streamhub.FullStateSnapshot{}, snap)
}
