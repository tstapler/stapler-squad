package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session/headless"
)

func TestHeadlessPoolModelConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate config.LoadConfig from the real config
	cfg := headlessPoolModelConfig(headless.PoolConfig{})
	fam := workflows.DefaultModelFamilies()

	assert.Equal(t, fam["sonnet"], cfg.DefaultModel, "pool default must be non-empty and not opus")
	for key := range headlessFeaturePolicyKeys {
		assert.Equal(t, fam["haiku"], cfg.ModelForFeature(key), string(key))
	}
	assert.Empty(t, cfg.ModelForFeature(headless.FeatureKeyReview), "unmapped features use pool default")
}
