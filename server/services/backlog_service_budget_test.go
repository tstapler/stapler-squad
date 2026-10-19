package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	tslog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// setDefaultBudget sets (or clears, for nil) the global default through the
// production UpdateQuotaConfig RPC.
func setDefaultBudget(t *testing.T, usd *float64) {
	t.Helper()
	settings := &sessionv1.QuotaSettings{}
	if usd != nil {
		settings.DefaultCostBudgetThresholdUsd = proto.Float64(*usd)
	}
	_, err := updateQuota(t, settings)
	require.NoError(t, err)
}

// budgetProbe counts [BudgetWarning] lines from the triage/review site
// (logBudgetWarningIfCrossed) and the work-stage site (checkWorkStageBudget).
// It redirects the warning log once per test: RedirectLogger holds its lock
// until cleanup, so a second redirect in the same test would deadlock.
type budgetProbe struct{ buf *tslog.SyncBuffer }

func newBudgetProbe(t *testing.T) *budgetProbe {
	t.Helper()
	return &budgetProbe{buf: swapWarningLog(t)}
}

// warnings returns the warning counts for an item with the given own threshold.
func (p *budgetProbe) warnings(itemThreshold *float64, spent float64) (inline, workStage int) {
	before := p.buf.String()
	logBudgetWarningIfCrossed("bl_inline", "triage", itemThreshold, []session.ItemSessionSummary{}, spent)
	NewBacklogService(nil, nil, nil, nil, nil, nil).checkWorkStageBudget("bl_work", itemThreshold, spent)
	added := strings.TrimPrefix(p.buf.String(), before)
	return strings.Count(added, "[BudgetWarning] item=bl_inline"), strings.Count(added, "[BudgetWarning] item=bl_work")
}

// warningsForDefault returns a probe function that sets the global default and
// reports the warnings for an item with no threshold of its own at spend 5.20.
func warningsForDefault(t *testing.T) func(setting any) any {
	t.Helper()
	probe := newBudgetProbe(t)
	return func(setting any) any {
		def := setting.(float64)
		setDefaultBudget(t, &def)
		inline, work := probe.warnings(nil, 5.20)
		return [2]int{inline, work}
	}
}

func TestEffectiveBudgetThreshold_WarnsAtBothEvaluationSites_WhenDefaultSetAndItemHasNone(t *testing.T) {
	withIsolatedStateDir(t)
	def := 5.00
	setDefaultBudget(t, &def)

	probe := newBudgetProbe(t)
	inline, work := probe.warnings(nil, 5.20)
	assert.Equal(t, 1, inline, "logBudgetWarningIfCrossed must use the global default")
	assert.Equal(t, 1, work, "checkWorkStageBudget must use the global default")

	own := 10.00
	inline, work = probe.warnings(&own, 5.20)
	assert.Zero(t, inline, "item threshold 10.00 overrides the default")
	assert.Zero(t, work, "item threshold 10.00 overrides the default")
}

func TestEffectiveBudgetThreshold_TreatsZeroAsValidAndNilAsOff(t *testing.T) {
	withIsolatedStateDir(t)

	setDefaultBudget(t, nil)
	probe := newBudgetProbe(t)
	inline, work := probe.warnings(nil, 0.01)
	assert.Zero(t, inline+work, "nil default and nil item threshold is off")

	zero := 0.0
	setDefaultBudget(t, &zero)
	inline, work = probe.warnings(nil, 0.01)
	assert.Equal(t, 1, inline)
	assert.Equal(t, 1, work)
}

func TestBudgetWarning_ChangesWhenDefaultThresholdChangedViaQuotaRpc(t *testing.T) {
	withIsolatedStateDir(t)
	assertConfigChangesBehavior(t, warningsForDefault(t), 10.00, 5.00)
}

func TestConfigSurfaceProof_QuotaDefaultCostBudgetThresholdUSD_ChangesBudgetWarning(t *testing.T) {
	withIsolatedStateDir(t)
	assertConfigChangesBehavior(t, warningsForDefault(t), 100.00, 0.00)
}
