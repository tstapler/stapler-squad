package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/session/instance"
)

// resetDefaultLeaseFlag keeps a test that flips the process-wide flag from
// leaking its state into other tests in the package.
func resetDefaultLeaseFlag(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { instance.DefaultLeaseFlag.SetEnabled(true) })
}

func leaseFlagEnabledInRegistry(t *testing.T, svc *flagsvc.FeatureFlagService) bool {
	t.Helper()
	resp, err := svc.GetFeatureFlags(context.Background(), connect.NewRequest(&sessionv1.GetFeatureFlagsRequest{}))
	require.NoError(t, err)
	for _, f := range resp.Msg.Flags {
		if f.Name == featureflags.TerminalWriteLeaseFlagName {
			return f.Enabled
		}
	}
	t.Fatalf("%s is not registered in the flag registry", featureflags.TerminalWriteLeaseFlagName)
	return false
}

// T-WL-11 (server half).
func TestTerminalWriteLeaseFlag_ShouldBeRegisteredDefaultOnGlobalOnlyAndApplyEveryFlipAtOnce(t *testing.T) {
	resetDefaultLeaseFlag(t)
	svc := newGatedFlagService(t)
	require.True(t, testFlagRegistry().Default(featureflags.TerminalWriteLeaseFlagName), "default on")
	assert.True(t, leaseFlagEnabledInRegistry(t, svc.featureFlagSvc))
	assert.True(t, instance.DefaultLeaseFlag.Enabled())

	require.NoError(t, flagUpdateCtx(context.Background(), svc.featureFlagSvc, featureflags.TerminalWriteLeaseFlagName, false))
	assert.False(t, instance.DefaultLeaseFlag.Enabled(), "the off flip applies at once, no reload")
	assert.False(t, leaseFlagEnabledInRegistry(t, svc.featureFlagSvc))

	require.NoError(t, flagUpdateCtx(context.Background(), svc.featureFlagSvc, featureflags.TerminalWriteLeaseFlagName, true))
	assert.True(t, instance.DefaultLeaseFlag.Enabled())
}

func TestTerminalWriteLeaseFlag_ShouldStartOffOnlyForAnExplicitPersistedFalse(t *testing.T) {
	resetDefaultLeaseFlag(t)
	svc := newGatedFlagService(t)
	require.True(t, instance.DefaultLeaseFlag.Enabled(), "no persisted key: on")

	require.NoError(t, config.LoadConfig().SetFeatureFlag(featureflags.TerminalWriteLeaseFlagName, false))
	svc.wireLeaseFlag()
	assert.False(t, instance.DefaultLeaseFlag.Enabled())

	require.NoError(t, config.LoadConfig().DeleteFeatureFlag(featureflags.TerminalWriteLeaseFlagName))
	svc.wireLeaseFlag()
	assert.True(t, instance.DefaultLeaseFlag.Enabled(), "deleting the key returns to the registry default")
}

// Off is an escape hatch: persisted first, recorded with only the degraded
// log record when the sink is down, never refused with Internal.
func TestTerminalWriteLeaseFlag_ShouldPersistOffWithOnlyTheFallbackRecordAndNeverFailInternal_WhenTheAuditSinkIsDown(t *testing.T) {
	resetDefaultLeaseFlag(t)
	e := newAuditEnv(t)
	e.svc.SetAudit(e.sink, map[string]featureflags.FlagAuditPolicy{featureflags.TerminalWriteLeaseFlagName: terminalWriteLeaseAuditPolicy})
	e.svc.SetFeatureController(featureflags.TerminalWriteLeaseFlagName, leaseFlagController{flag: instance.DefaultLeaseFlag})
	e.mfs.SetFailWrite(errDiskFull)

	require.NoError(t, e.flip(featureflags.TerminalWriteLeaseFlagName, false))
	v, ok := overrideOf(featureflags.TerminalWriteLeaseFlagName)
	assert.True(t, ok && !v, "persisted")
	assert.False(t, instance.DefaultLeaseFlag.Enabled(), "applied")
	assert.GreaterOrEqual(t, e.degradedCount(auditkit.AuditModeFallbackLog), 1)

	// The on flip is a tightening flip: it never waits on or fails with the sink.
	require.NoError(t, e.flip(featureflags.TerminalWriteLeaseFlagName, true))
	assert.True(t, instance.DefaultLeaseFlag.Enabled())
}

// Every flip is a flag_change line carrying the true previous value.
func TestTerminalWriteLeaseFlag_ShouldAppendFlagChangeLineWithTruePreviousValueForEveryFlip(t *testing.T) {
	resetDefaultLeaseFlag(t)
	e := newAuditEnv(t)
	e.svc.SetAudit(e.sink, map[string]featureflags.FlagAuditPolicy{featureflags.TerminalWriteLeaseFlagName: terminalWriteLeaseAuditPolicy})
	e.svc.SetFeatureController(featureflags.TerminalWriteLeaseFlagName, leaseFlagController{flag: instance.DefaultLeaseFlag})

	require.NoError(t, e.flip(featureflags.TerminalWriteLeaseFlagName, false))
	require.NoError(t, e.flip(featureflags.TerminalWriteLeaseFlagName, true))
	e.sink.Close()

	var results []auditkit.AuditLine
	for _, l := range e.lines(t) {
		assert.Equal(t, auditkit.AuditKindFlagChg, l.Kind)
		assert.Equal(t, featureflags.TerminalWriteLeaseFlagName, l.Flag)
		if l.Phase == auditkit.AuditPhaseResult {
			results = append(results, l)
		}
	}
	require.Len(t, results, 2)
	require.NotNil(t, results[0].Previous)
	assert.True(t, *results[0].Previous, "default on: the true previous value, not false")
	require.NotNil(t, results[1].Previous)
	assert.False(t, *results[1].Previous)
}
