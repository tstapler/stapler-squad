package triggerlimit

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// Recorded from the pre-move limiter in server/services (origin/main) with
// TRIGGERLIMIT_GOLDEN_RECORD=1; this file differs there only by the package clause.
const triggerGoldenPath = "testdata/trigger_rate_limiter.golden"

func TestTriggerRateLimiter_Golden(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	seq := func(label string, lim *TriggerRateLimiter, id uuid.UUID, n int) {
		var sb strings.Builder
		for i := 0; i < n; i++ {
			if lim.Allow(id) {
				sb.WriteByte('1')
			} else {
				sb.WriteByte('0')
			}
		}
		fmt.Fprintf(&b, "%s: %s\n", label, sb.String())
	}
	a := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	c := uuid.MustParse("00000000-0000-0000-0000-00000000000c")

	lim := NewTriggerRateLimiter()
	fmt.Fprintf(&b, "defaults: limit=%v burst=%d\n", lim.limit, lim.burst)
	seq("a burst+overflow", lim, a, 13)
	seq("c isolated", lim, c, 11)
	seq("a still throttled", lim, a, 2)

	var zero TriggerRateLimiter
	seq("zero value", &zero, a, 12)
	fmt.Fprintf(&b, "zero value defaults: limit=%v burst=%d\n", zero.limit, zero.burst)

	custom := &TriggerRateLimiter{limit: rate.Limit(0.001), burst: 3}
	seq("custom burst 3", custom, a, 5)

	// Exactly burst permits across many goroutines contending on one key.
	shared := NewTriggerRateLimiter()
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if shared.Allow(a) {
					allowed.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	fmt.Fprintf(&b, "concurrent allowed=%d buckets=%d\n", allowed.Load(), len(shared.limiters))

	got := b.String()
	if os.Getenv("TRIGGERLIMIT_GOLDEN_RECORD") == "1" {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(triggerGoldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(triggerGoldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
