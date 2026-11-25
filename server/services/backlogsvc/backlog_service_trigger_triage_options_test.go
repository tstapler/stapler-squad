package backlogsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/pipelinemode"
)

func TestTriageCallOptions_FanoutCeilingOnlyForSDD(t *testing.T) {
	t.Parallel()

	sdd := triageCallOptions(&config.Config{}, pipelinemode.DefaultSDDPipelineModeSlug, "/w", "m", nil)
	assert.Equal(t, config.DefaultHeadlessTriageMaxTurns, sdd.MaxTurns)
	assert.Equal(t, config.DefaultHeadlessTriageMaxSubagents, sdd.MaxSubagents)
	assert.Equal(t, "/w", sdd.WorkDir)

	for _, mode := range []string{"", "default", "other"} {
		got := triageCallOptions(&config.Config{}, mode, "/w", "m", nil)
		assert.Zero(t, got.MaxTurns, "mode %q must stay unbounded (AC4)", mode)
		assert.Zero(t, got.MaxSubagents, "mode %q must stay unbounded (AC4)", mode)
	}
}

func TestTriageCallOptions_ConfigOverrides(t *testing.T) {
	t.Parallel()

	tuned := triageCallOptions(&config.Config{HeadlessTriageMaxTurns: 50, HeadlessTriageMaxSubagents: 7}, pipelinemode.DefaultSDDPipelineModeSlug, "/w", "m", nil)
	assert.Equal(t, 50, tuned.MaxTurns)
	assert.Equal(t, 7, tuned.MaxSubagents)

	disabled := triageCallOptions(&config.Config{HeadlessTriageMaxTurns: -1, HeadlessTriageMaxSubagents: -1}, pipelinemode.DefaultSDDPipelineModeSlug, "/w", "m", nil)
	assert.Zero(t, disabled.MaxTurns)
	assert.Zero(t, disabled.MaxSubagents)

	nilCfg := triageCallOptions(nil, pipelinemode.DefaultSDDPipelineModeSlug, "/w", "m", nil)
	assert.Zero(t, nilCfg.MaxTurns)
}

func TestTriageCallOptions_CostCeilingAppliesToEveryMode(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"", pipelinemode.DefaultSDDPipelineModeSlug} {
		got := triageCallOptions(&config.Config{}, mode, "/w", "m", nil)
		assert.Equal(t, config.HeadlessTriageMaxCostUSDDefault, got.MaxCostUSD, "mode %q", mode)
	}
}
