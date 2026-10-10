package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func newGatedFlagService(t *testing.T) *SessionService {
	t.Helper()
	envtest.NewIsolatedStateDir(t)
	svc := newGatedSessionService(createTestStorage(t))
	t.Cleanup(svc.Shutdown)
	return svc
}

func gateFlagUpdate(svc *SessionService, ctx context.Context, enabled bool) error {
	return flagUpdateCtx(ctx, svc.featureFlagSvc, config.HiddenSessionGateFeatureFlag, enabled)
}

func flagUpdateCtx(ctx context.Context, svc *FeatureFlagService, name string, enabled bool) error {
	_, err := svc.UpdateFeatureFlag(ctx, connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: name, Enabled: enabled}))
	return err
}

func TestGateFlag_ShouldBeRegisteredDefaultOffWithStatsWriterDetail_WhenNoWriterRuns(t *testing.T) {
	svc := newGatedFlagService(t)
	require.False(t, featureFlagDefault(config.HiddenSessionGateFeatureFlag))
	assert.Equal(t, statsWriterNotRunning, statusDetailOf(t, svc.featureFlagSvc, config.HiddenSessionGateFeatureFlag))
}

func TestGateFlag_ShouldRefuseEnableWhileStatsWriterNotRunningAndNeverRefuseDisabling_AndAllowWithRunningWriter(t *testing.T) {
	svc := newGatedFlagService(t)
	ctx := context.Background()
	gate := svc.DeliveryGate()

	err := gateFlagUpdate(svc, ctx, true)
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	_, persisted := config.LoadConfig().GetFeatureFlagOverride(config.HiddenSessionGateFeatureFlag)
	assert.False(t, persisted, "a refused enable persists nothing")
	assert.False(t, gate.Flags().Enabled())

	require.NoError(t, gateFlagUpdate(svc, ctx, false), "disabling is never refused by the precondition")

	gate.Stats().SetWriterRunning(true)
	require.NoError(t, gateFlagUpdate(svc, ctx, true))
	assert.True(t, gate.Flags().Enabled(), "the FlagObserver reloads the cache at once, not at the next 5s tick")
	assert.Empty(t, statusDetailOf(t, svc.featureFlagSvc, config.HiddenSessionGateFeatureFlag))

	require.NoError(t, gateFlagUpdate(svc, ctx, false))
	assert.False(t, gate.Flags().Enabled())
}

// T-FL-24: every gate flip appends a flag_change line carrying the request
// record fields and the true previous value.
func TestUpdateFeatureFlag_ShouldAppendFlagChangeLineForEveryHiddenSessionGateFlip_WhenSinkCoreFromPr2a2Present(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	dir, err := config.GetConfigDir()
	require.NoError(t, err)
	svc := newGatedSessionService(createTestStorage(t))
	svc.DeliveryGate().Stats().SetWriterRunning(true)

	var flipErr error
	h := WithRequestRecord(ListenerLocal, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, on := range []bool{true, false, true} {
			if flipErr = gateFlagUpdate(svc, r.Context(), on); flipErr != nil {
				return
			}
		}
	}))
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8543/x", nil)
	req.Host = "localhost:8543"
	h.ServeHTTP(httptest.NewRecorder(), req)
	require.NoError(t, flipErr)
	svc.Shutdown() // joins the audit drain goroutine

	raw, err := os.ReadFile(filepath.Join(dir, "audit", auditFileName))
	require.NoError(t, err)
	ls := lines(raw)
	require.Len(t, ls, 3, "one result line per tightening-path flip")
	prevs := make([]bool, 0, len(ls))
	for i, l := range ls {
		var al AuditLine
		require.NoError(t, json.Unmarshal(l, &al))
		assert.Equal(t, auditKindFlagChg, al.Kind)
		assert.Equal(t, config.HiddenSessionGateFeatureFlag, al.Flag)
		assert.Equal(t, "global", al.Scope)
		assert.Equal(t, auditPhaseResult, al.Phase)
		assert.Equal(t, flagOutcomeApplied, al.Outcome)
		assert.Equal(t, int64(i+1), al.Seq)
		assert.Equal(t, ListenerLocal, al.Listener)
		assert.Equal(t, "localhost:8543", al.Host)
		assert.Equal(t, AuthModeNone, al.AuthMode)
		require.NotNil(t, al.Previous)
		prevs = append(prevs, *al.Previous)
	}
	assert.Equal(t, []bool{false, true, false}, prevs, "true previous values: default off, then on, then off")
	assert.True(t, bytes.HasSuffix(raw, []byte("\n")))
}
