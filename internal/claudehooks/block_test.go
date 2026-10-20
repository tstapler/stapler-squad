package claudehooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCapHookBlockMs_ShouldClampToTheSpikeCeiling(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want int }{
		{-5, 0}, {0, 0}, {2000, 2000}, {HookBlockMsCeiling, HookBlockMsCeiling}, {HookBlockMsCeiling + 1, HookBlockMsCeiling}, {1 << 30, HookBlockMsCeiling},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, CapHookBlockMs(tt.in), tt.in)
	}
}

func TestHookBlockMsFromEnv_ShouldDefaultToZero_AndCapWhatItReads(t *testing.T) {
	t.Parallel()
	env := func(v string, ok bool) func(string) (string, bool) {
		return func(k string) (string, bool) {
			assert.Equal(t, HookBlockMsEnv, k)
			return v, ok
		}
	}
	assert.Zero(t, HookBlockMsFromEnv(env("", false)), "default never blocks")
	assert.Zero(t, HookBlockMsFromEnv(env("soon", true)))
	assert.Equal(t, 2000, HookBlockMsFromEnv(env("2000", true)))
	assert.Equal(t, HookBlockMsCeiling, HookBlockMsFromEnv(env("9999999", true)))
}

func TestHookTimeoutSeconds_ShouldExceedTheWaitAndStayBelowClaudesDefault(t *testing.T) {
	t.Parallel()
	for _, ms := range []int{0, 1, 2000, 59999, HookBlockMsCeiling, 10 * HookBlockMsCeiling} {
		got := HookTimeoutSeconds(ms)
		assert.Greater(t, got*1000, CapHookBlockMs(ms), "timeout must outlast the wait for %d ms", ms)
		assert.Less(t, got, 600)
	}
}
