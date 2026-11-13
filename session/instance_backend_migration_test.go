package session

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tymux"
)

func TestMigrationToTmux_WiresInstanceEnvironment(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "migration-env", UUID: "migration-proof", Backend: BackendTymux, EnvVars: map[string]string{"MIGRATION_CUSTOM": "present"}}
	require.NoError(t, inst.updateProcessManagerForRestart(true, BackendTmux, shellLaunch("bash")))
	sess := inst.GetTmuxSession()
	require.NotNil(t, sess)
	field := reflect.ValueOf(sess).Elem().FieldByName("extraEnv")
	var env []string
	for idx := 0; idx < field.Len(); idx++ {
		env = append(env, field.Index(idx).String())
	}
	assert.Contains(t, env, "STAPLER_SESSION_UUID=migration-proof")
	assert.Contains(t, env, "MIGRATION_CUSTOM=present")
}

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
	inst := &Instance{Title: "migration-unreachable", Path: t.TempDir(), Program: "bash", Status: domain.Active, Backend: BackendTymux}
	inst.started.Store(true)
	oldPM := NewTymuxBackend(fake)
	inst.processManager = oldPM

	for attempt := 0; attempt < 2; attempt++ {
		err := inst.Restart(false, BackendTmux)
		require.ErrorIs(t, err, unavailable)
		assert.Zero(t, fake.closeCalls, "failed preflight must not consume kill ownership")
		assert.Same(t, oldPM, inst.pm())
		assert.Equal(t, BackendTymux, inst.Snapshot().Backend)
	}
	ready = true
	err := inst.Restart(false, BackendTmux)
	require.ErrorIs(t, err, closeErr)
	assert.Equal(t, 1, fake.closeCalls, "recovered daemon must still confirm deletion before migration")
	assert.Same(t, oldPM, inst.pm())
	assert.Equal(t, BackendTymux, inst.Snapshot().Backend)
}

func TestMigrationToTymux_RegistersExitCallbackAndEnvironmentBeforeStart(t *testing.T) {
	fake := &migrationLaunchManager{}
	orig := newTymuxSessionFn
	newTymuxSessionFn = func() tymux.TymuxManager { return fake }
	t.Cleanup(func() { newTymuxSessionFn = orig })
	defer stubEnsureDaemonRunning(func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
		return tymux.TymuxdReady{}, nil
	})()
	inst := &Instance{Title: "migration-callback", UUID: "migration-proof", Path: t.TempDir(), Program: "bash", Status: domain.Active, Backend: BackendTmux, EnvVars: map[string]string{"MIGRATION_CUSTOM": "present"}}
	inst.started.Store(true)
	require.NoError(t, inst.initializeBaselineForRestartTest())

	err := inst.Restart(false, BackendTymux)

	require.ErrorContains(t, err, "stop before controller startup")
	assert.True(t, fake.callbackAtStart, "push-only liveness needs a callback before the new manager starts")
	assert.Equal(t, 1, fake.resetCalls)
	assert.Contains(t, fake.gotEnv, "STAPLER_SESSION_UUID=migration-proof")
	assert.Contains(t, fake.gotEnv, "MIGRATION_CUSTOM=present")
}
