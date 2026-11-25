package flagsvc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
)

const scopedFlag = NotificationInlineActionsFlagName

var errApply = errors.New("apply refused")

// scopedFake is a controller that records ApplyScope calls and can refuse them.
type scopedFake struct {
	recordingController
	applied []bool
	fail    error
}

func (s *scopedFake) ApplyScope(scope string, enabled bool) error {
	s.applied = append(s.applied, enabled)
	return s.fail
}

func setScopedConfig(t *testing.T, global *bool, scope string, override *bool) {
	t.Helper()
	require.NoError(t, config.UpdateConfig(func(c *config.Config) error {
		c.FeatureFlags, c.FeatureFlagScopes = nil, nil
		if global != nil {
			c.FeatureFlags = map[string]bool{scopedFlag: *global}
		}
		if override != nil {
			c.ApplyFeatureFlagScope(scopedFlag, scope, *override)
		}
		return nil
	}))
}

func scopeOverride(t *testing.T, scope string) (value, ok bool) {
	t.Helper()
	return config.LoadConfig().GetFeatureFlagScopedOverride(scopedFlag, scope)
}

func scopeRequest(mut sessionv1.FlagMutation, scope string) *connect.Request[sessionv1.UpdateFeatureFlagRequest] {
	return connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: scopedFlag, Mutation: mut, Scope: scope})
}

func auditedService(t *testing.T) (*FeatureFlagService, *memfs.MemFS, *auditkit.AuditSink) {
	t.Helper()
	mfs := memfs.New()
	sink := auditkit.NewAuditSink(func() (string, error) { return "/cfg", nil },
		auditkit.WithAuditFS(mfs), auditkit.WithAuditClock(func() time.Time { return time.Unix(1700000000, 0).UTC() }))
	svc := NewFeatureFlagService()
	svc.SetAudit(sink, NotificationActionsFlagAuditPolicies())
	return svc, mfs, sink
}

func auditLines(t *testing.T, mfs *memfs.MemFS) []auditkit.AuditLine {
	t.Helper()
	raw, ok := mfs.Get("/cfg/audit/" + auditkit.AuditFileName)
	if !ok {
		return nil
	}
	var out []auditkit.AuditLine
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var al auditkit.AuditLine
		require.NoError(t, json.Unmarshal([]byte(l), &al))
		out = append(out, al)
	}
	return out
}

func resultLine(t *testing.T, lines []auditkit.AuditLine) auditkit.AuditLine {
	t.Helper()
	for _, l := range lines {
		if l.Phase == auditkit.AuditPhaseResult {
			return l
		}
	}
	t.Fatalf("no result line in %v", lines)
	return auditkit.AuditLine{}
}

func TestClose_ShouldBeSafe_WhenNoAuditSinkWasWired(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	require.NotPanics(t, func() { NewFeatureFlagService().Close() })
	svc, _, _ := auditedService(t)
	require.NotPanics(t, svc.Close)
}

func TestClearTurnsKindOn_ShouldOnlyReportAClearOfAnExplicitFalseUnderAnEnabledGlobal(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	reg := testFlagRegistry()
	scope := ScopeChannelRemote
	clear := flagOp{name: scopedFlag, scope: scope, mutation: sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE}
	on, off := ptrBool(true), ptrBool(false)

	cases := []struct {
		name     string
		global   *bool
		override *bool
		op       flagOp
		want     bool
	}{
		{"explicit false under global on", on, off, clear, true},
		{"explicit true under global on", on, on, clear, false},
		{"no override under global on", on, nil, clear, false},
		{"explicit false under global off", off, off, clear, false},
		{"a set is never a clear", on, off, flagOp{name: scopedFlag, scope: scope, mutation: sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED}, false},
	}
	for _, tc := range cases {
		setScopedConfig(t, tc.global, scope, tc.override)
		assert.Equal(t, tc.want, clearTurnsKindOn(tc.op, reg), tc.name)
	}
}

func TestPersistScoped_ShouldReportTheEffectiveValueToTheController_WhenAScopeIsSetOrCleared(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc, mfs, sink := auditedService(t)
	ctrl := &scopedFake{}
	svc.SetFeatureController(scopedFlag, ctrl)
	scope := ScopeChannelRemote

	setScopedConfig(t, ptrBool(true), scope, ptrBool(false))
	_, err := svc.UpdateFeatureFlag(context.Background(), scopeRequest(sessionv1.FlagMutation_FLAG_MUTATION_CLEAR_SCOPE, scope))
	require.NoError(t, err)
	_, has := scopeOverride(t, scope)
	assert.False(t, has, "the key is gone")
	require.Equal(t, []bool{true}, ctrl.applied, "a clear reports what the scope now inherits from global")

	_, err = svc.UpdateFeatureFlag(context.Background(), scopeRequest(sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED, scope))
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false}, ctrl.applied, "a set reports the value it set")

	sink.Close()
	var results []auditkit.AuditLine
	for _, l := range auditLines(t, mfs) {
		if l.Phase == auditkit.AuditPhaseResult {
			results = append(results, l)
		}
	}
	require.Len(t, results, 2)
	assert.Equal(t, auditkit.FlagOutcomeApplied, results[0].Outcome)
	assert.False(t, *results[0].Previous, "previous is the explicit override that was cleared")
	assert.True(t, *results[0].New)
	assert.True(t, *results[1].Previous, "no key: previous is the inherited global")
	assert.False(t, *results[1].New)
}

func TestPersistScoped_ShouldRestoreTheOverride_WhenTheControllerRefusesTheScope(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc, mfs, sink := auditedService(t)
	obs := &recordingObserver{}
	svc.SetFlagObserver(obs)
	ctrl := &scopedFake{fail: errApply}
	svc.SetFeatureController(scopedFlag, ctrl)
	scope := ScopeChannelRemote

	setScopedConfig(t, ptrBool(true), scope, ptrBool(false))
	_, err := svc.UpdateFeatureFlag(context.Background(), scopeRequest(sessionv1.FlagMutation_FLAG_MUTATION_SET_ENABLED, scope))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "rollback also failed")
	v, ok := scopeOverride(t, scope)
	assert.True(t, ok && !v, "an existing override is restored to its own value, not to the global one")

	setScopedConfig(t, ptrBool(true), scope, nil)
	_, err = svc.UpdateFeatureFlag(context.Background(), scopeRequest(sessionv1.FlagMutation_FLAG_MUTATION_SET_DISABLED, scope))
	require.Error(t, err)
	_, ok = scopeOverride(t, scope)
	assert.False(t, ok, "a key that did not exist is removed again")
	assert.Equal(t, []string{scopedFlag, scopedFlag}, obs.names, "the observer hears about each rolled-back write")

	sink.Close()
	assert.Equal(t, auditkit.FlagOutcomeRolledBack, resultLine(t, auditLines(t, mfs)).Outcome)
}

func TestUpdateFeatureFlag_ShouldRollBackAndAuditIt_WhenTheGlobalControllerRefuses(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc, mfs, sink := auditedService(t)
	obs := &recordingObserver{}
	svc.SetFlagObserver(obs)
	svc.SetFeatureController(BackgroundActivityFeedFlagName, refusingController{})

	_, err := svc.UpdateFeatureFlag(context.Background(), connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: BackgroundActivityFeedFlagName, Enabled: false}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to disable feature")
	assert.NotContains(t, err.Error(), "rollback also failed")
	_, persisted := config.LoadConfig().GetFeatureFlagOverride(BackgroundActivityFeedFlagName)
	assert.False(t, persisted, "the refused flip leaves no key behind")
	assert.Equal(t, []string{BackgroundActivityFeedFlagName}, obs.names)

	sink.Close()
	assert.Equal(t, auditkit.FlagOutcomeRolledBack, resultLine(t, auditLines(t, mfs)).Outcome)
}

type refusingController struct{}

func (refusingController) Enable(context.Context) error { return errApply }
func (refusingController) Disable() error               { return errApply }
func (refusingController) IsEnabled() bool              { return true }

func TestWorkspacePeersBlockFor_ShouldReturnNothing_WhenTheFlagIsOffOrThereIsNoStorage(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	assert.Empty(t, WorkspacePeersBlockFor(context.Background(), nil, "/repo"))
	require.NoError(t, config.UpdateConfig(func(c *config.Config) error {
		c.FeatureFlags = map[string]bool{featureflags.WorkspacePeersNudgeFlagName: true}
		return nil
	}))
	assert.Empty(t, WorkspacePeersBlockFor(context.Background(), nil, "/repo"), "flag on but no storage")
}

func TestInjectedRegistry_ShouldBeNilOnlyForANilReceiver(t *testing.T) {
	var none *FeatureFlagService
	assert.Nil(t, none.InjectedRegistry())
	svc := NewFeatureFlagService()
	assert.Same(t, svc.Registry(), svc.InjectedRegistry())
	assert.Nil(t, svc.AuditSink())
}

func TestSetStatusDetailProvider_ShouldReplaceItsOwnSlotAndKeepTheOtherSources(t *testing.T) {
	svctest.WithIsolatedStateDir(t)
	svc := NewFeatureFlagService()
	const name = "backlog"
	svc.AddStatusDetailSource(name, func() string { return "source" })
	svc.SetStatusDetailProvider(name, func() string { return "first" })
	svc.SetStatusDetailProvider(name, func() string { return "second" })
	svc.AddStatusDetailSource(name, func() string { return "later source" })

	assert.Equal(t, "source; second; later source", svc.statusDetailFor(name, config.LoadConfig()))
}

func TestStartupFlagValue_ShouldReadTheConfigFileStrictly(t *testing.T) {
	dir := svctest.WithIsolatedStateDir(t)
	cfgPath := filepath.Join(dir, config.ConfigFileName)
	_ = os.Remove(cfgPath)
	assert.True(t, startupFlagValue("any", true), "no config file: the default")
	assert.False(t, startupFlagValue("any", false))

	require.NoError(t, os.WriteFile(cfgPath, []byte(`{"feature_flags":{"any":false}}`), 0o600))
	assert.False(t, startupFlagValue("any", true), "an explicit value wins over the default")

	require.NoError(t, os.WriteFile(cfgPath, []byte(`{not json`), 0o600))
	assert.False(t, startupFlagValue("any", true), "an unreadable config is off whatever the default")
}

func TestSetWakeFlagListener_ShouldNotifyUntilCleared(t *testing.T) {
	calls := 0
	t.Cleanup(func() { SetWakeFlagListener(nil) })
	require.NotPanics(t, notifyWakeFlagChanged, "no listener installed")
	SetWakeFlagListener(func() { calls++ })
	notifyWakeFlagChanged()
	require.Equal(t, 1, calls)
	SetWakeFlagListener(nil)
	notifyWakeFlagChanged()
	assert.Equal(t, 1, calls, "a nil listener clears the registration")
}

func TestFeatureFlagAuditPolicies_ShouldMergeIntoAnUnsetPolicyMap(t *testing.T) {
	svc := NewFeatureFlagService()
	svc.MergeAuditPolicies(SessionMultiPRFlagAuditPolicies())
	svc.MergeAuditPolicies(NotificationActionsFlagAuditPolicies())
	assert.Contains(t, svc.auditPolicies, config.SessionMultiPRFlag)
	assert.Contains(t, svc.auditPolicies, AgentWakeTriggersFlagName)
}

func TestFlagReader_ShouldPanic_WhenNoRegistryWasInjected(t *testing.T) {
	assert.Panics(t, func() { FlagReader{}.Enabled(&config.Config{}, "x") })
	assert.NotPanics(t, func() { FlagReader{Reg: testFlagRegistry()}.Enabled(&config.Config{}, "x") })
}
