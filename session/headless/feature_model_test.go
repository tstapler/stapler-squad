package headless

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Precedence: explicit opts.Model > FeatureModel pin > DefaultModel, asserted on argv.
func TestPool_FeatureModel_PinsPerFeatureAndYieldsToExplicit(t *testing.T) {
	t.Parallel()
	pins := map[FeatureKey]string{"cheap": "haiku"}
	cfg := PoolConfig{
		DefaultModel: "default-model",
		FeatureModel: func(k FeatureKey) string { return pins[k] },
	}
	cases := []struct {
		name  string
		key   FeatureKey
		opts  CallOptions
		model string
	}{
		{"pinned feature", "cheap", CallOptions{}, "haiku"},
		{"explicit wins", "cheap", CallOptions{Model: "opus"}, "opus"},
		{"unpinned falls to default", "other", CallOptions{}, "default-model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewFakeRunner(firstCallJSON("s1", "ok"))
			pool := newTestPool(cfg, runner)
			_, err := pool.CallBlocking(context.Background(), tc.key, "sys", "p", tc.opts, DiscardCost)
			require.NoError(t, err)
			assert.True(t, runner.ArgsContainSequence(0, "--model", tc.model), "argv: %v", runner.ArgsForCall(0))
		})
	}
}
