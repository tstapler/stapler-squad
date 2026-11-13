package session

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tymux"
)

type migrationLaunchManager struct {
	fakeTymuxManagerForSetCommand
	callback        func(string)
	resetCalls      int
	callbackAtStart bool
}

func (f *migrationLaunchManager) SetOnExitCallback(fn func(string)) { f.callback = fn }
func (f *migrationLaunchManager) ResetExitOnce()                    { f.resetCalls++ }
func (f *migrationLaunchManager) Start(string) error {
	f.callbackAtStart = f.callback != nil
	return errors.New("stop before controller startup")
}

type migrationClosingManager struct {
	seamFakeTymux
	closeErr error
}

func (f *migrationClosingManager) Close() error {
	f.closeCalls++
	return f.closeErr
}

func TestMigrationFromTymux_RequiresDaemonReadyBeforeClosingOldSession(t *testing.T) {
	unavailable := errors.New("local daemon still unreachable")
	ready := false
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		if !ready {
			return tymux.TymuxdReady{}, unavailable
		}
		return tymux.TymuxdReady{}, nil
	})()
	closeErr := errors.New("old session kill rejected")
	fake := &migrationClosingManager{seamFakeTymux: seamFakeTymux{id: "old-daemon-session"}, closeErr: closeErr}
	inst := &Instance{Title: "migration-unreachable", Path: t.TempDir(), Program: "bash", Status: domain.Active, Backend: domain.BackendTymux}
	inst.MarkStarted()
	oldPM := NewTymuxBackend(fake)
	inst.SetProcessManagerForTest(oldPM)

	for attempt := 0; attempt < 2; attempt++ {
		err := inst.Restart(false, domain.BackendTmux)
		require.ErrorIs(t, err, unavailable)
		assert.Zero(t, fake.closeCalls, "failed preflight must not consume kill ownership")
		assert.Same(t, oldPM, inst.ProcessManagerForTest())
		assert.Equal(t, domain.BackendTymux, inst.Snapshot().Backend)
	}
	ready = true
	err := inst.Restart(false, domain.BackendTmux)
	require.ErrorIs(t, err, closeErr)
	assert.Equal(t, 1, fake.closeCalls, "recovered daemon must still confirm deletion before migration")
	assert.Same(t, oldPM, inst.ProcessManagerForTest())
	assert.Equal(t, domain.BackendTymux, inst.Snapshot().Backend)
}

func TestMigrationToTymux_RegistersExitCallbackAndEnvironmentBeforeStart(t *testing.T) {
	fake := &migrationLaunchManager{}
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return fake }
	t.Cleanup(func() { newTymuxSessionFn = orig })
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		return tymux.TymuxdReady{}, nil
	})()
	inst := &Instance{Title: "migration-callback", UUID: "migration-proof", Path: t.TempDir(), Program: "bash", Status: domain.Active, Backend: domain.BackendTmux, EnvVars: map[string]string{"MIGRATION_CUSTOM": "present"}}
	inst.MarkStarted()
	require.NoError(t, initializeBaselineForRestartTest(inst))

	err := inst.Restart(false, domain.BackendTymux)

	require.ErrorContains(t, err, "stop before controller startup")
	assert.True(t, fake.callbackAtStart, "push-only liveness needs a callback before the new manager starts")
	assert.Equal(t, 1, fake.resetCalls)
	assert.Contains(t, fake.gotEnv, "STAPLER_SESSION_UUID=migration-proof")
	assert.Contains(t, fake.gotEnv, "MIGRATION_CUSTOM=present")
}
