package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// resetDefaultLeaseFlag keeps a test that flips the process-wide flag from
// leaking its state into other tests in the package.
func resetDefaultLeaseFlag(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { session.DefaultLeaseFlag.SetEnabled(true) })
}

func leaseFlagEnabledInRegistry(t *testing.T, svc *FeatureFlagService) bool {
	t.Helper()
	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	for _, f := range resp.Msg.Flags {
		if f.Name == terminalWriteLeaseFlagName {
			return f.Enabled
		}
	}
	t.Fatalf("%s is not registered in knownFeatureFlags", terminalWriteLeaseFlagName)
	return false
}

// T-WL-11 (server half).
func TestTerminalWriteLeaseFlag_ShouldBeRegisteredDefaultOnGlobalOnlyAndApplyEveryFlipAtOnce(t *testing.T) {
	resetDefaultLeaseFlag(t)
	svc := newGatedFlagService(t)
	require.True(t, featureFlagDefault(terminalWriteLeaseFlagName), "default on")
	assert.True(t, leaseFlagEnabledInRegistry(t, svc.featureFlagSvc))
	assert.True(t, session.DefaultLeaseFlag.Enabled())

	require.NoError(t, flagUpdateCtx(context.Background(), svc.featureFlagSvc, terminalWriteLeaseFlagName, false))
	assert.False(t, session.DefaultLeaseFlag.Enabled(), "the off flip applies at once, no reload")
	assert.False(t, leaseFlagEnabledInRegistry(t, svc.featureFlagSvc))

	require.NoError(t, flagUpdateCtx(context.Background(), svc.featureFlagSvc, terminalWriteLeaseFlagName, true))
	assert.True(t, session.DefaultLeaseFlag.Enabled())
}

func TestTerminalWriteLeaseFlag_ShouldStartOffOnlyForAnExplicitPersistedFalse(t *testing.T) {
	resetDefaultLeaseFlag(t)
	svc := newGatedFlagService(t)
	require.True(t, session.DefaultLeaseFlag.Enabled(), "no persisted key: on")

	require.NoError(t, config.LoadConfig().SetFeatureFlag(terminalWriteLeaseFlagName, false))
	svc.wireLeaseFlag()
	assert.False(t, session.DefaultLeaseFlag.Enabled())

	require.NoError(t, config.LoadConfig().DeleteFeatureFlag(terminalWriteLeaseFlagName))
	svc.wireLeaseFlag()
	assert.True(t, session.DefaultLeaseFlag.Enabled(), "deleting the key returns to the registry default")
}

// Off is an escape hatch: persisted first, recorded with only the degraded
// log record when the sink is down, never refused with Internal.
func TestTerminalWriteLeaseFlag_ShouldPersistOffWithOnlyTheFallbackRecordAndNeverFailInternal_WhenTheAuditSinkIsDown(t *testing.T) {
	resetDefaultLeaseFlag(t)
	e := newAuditEnv(t)
	e.svc.SetAudit(e.sink, map[string]FlagAuditPolicy{terminalWriteLeaseFlagName: terminalWriteLeaseAuditPolicy})
	e.svc.SetFeatureController(terminalWriteLeaseFlagName, leaseFlagController{flag: session.DefaultLeaseFlag})
	e.mfs.failWrite = errDiskFull

	require.NoError(t, e.flip(terminalWriteLeaseFlagName, false))
	v, ok := overrideOf(terminalWriteLeaseFlagName)
	assert.True(t, ok && !v, "persisted")
	assert.False(t, session.DefaultLeaseFlag.Enabled(), "applied")
	assert.GreaterOrEqual(t, e.degradedCount(auditModeFallbackLog), 1)

	// The on flip is a tightening flip: it never waits on or fails with the sink.
	require.NoError(t, e.flip(terminalWriteLeaseFlagName, true))
	assert.True(t, session.DefaultLeaseFlag.Enabled())
}

// Every flip is a flag_change line carrying the true previous value.
func TestTerminalWriteLeaseFlag_ShouldAppendFlagChangeLineWithTruePreviousValueForEveryFlip(t *testing.T) {
	resetDefaultLeaseFlag(t)
	e := newAuditEnv(t)
	e.svc.SetAudit(e.sink, map[string]FlagAuditPolicy{terminalWriteLeaseFlagName: terminalWriteLeaseAuditPolicy})
	e.svc.SetFeatureController(terminalWriteLeaseFlagName, leaseFlagController{flag: session.DefaultLeaseFlag})

	require.NoError(t, e.flip(terminalWriteLeaseFlagName, false))
	require.NoError(t, e.flip(terminalWriteLeaseFlagName, true))
	e.sink.Close()

	var results []AuditLine
	for _, l := range e.lines(t) {
		assert.Equal(t, auditKindFlagChg, l.Kind)
		assert.Equal(t, terminalWriteLeaseFlagName, l.Flag)
		if l.Phase == auditPhaseResult {
			results = append(results, l)
		}
	}
	require.Len(t, results, 2)
	require.NotNil(t, results[0].Previous)
	assert.True(t, *results[0].Previous, "default on: the true previous value, not false")
	require.NotNil(t, results[1].Previous)
	assert.False(t, *results[1].Previous)
}
