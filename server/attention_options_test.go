package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/server/services"
)

func TestAttentionFlagsCache_Should_ReloadOnlyAfterTTL_When_FlagFlips(t *testing.T) {
	t.Parallel()
	clock := time.Unix(1000, 0)
	loads := 0
	ackBypass := false
	c := &attentionFlagsCache{
		ttl:  5 * time.Second,
		now:  func() time.Time { return clock },
		load: func() services.AttentionFlags { loads++; return services.AttentionFlags{AckBypassFix: ackBypass} },
	}

	_, r := c.options()
	assert.False(t, r.AckBypassFix)

	ackBypass = true
	clock = clock.Add(4 * time.Second)
	_, r = c.options()
	assert.False(t, r.AckBypassFix, "served from cache within the TTL")
	assert.Equal(t, 1, loads)

	clock = clock.Add(2 * time.Second)
	_, r = c.options()
	assert.True(t, r.AckBypassFix, "flip visible once the TTL lapses")
	assert.Equal(t, 2, loads)
}
