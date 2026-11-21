package server

import (
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session"
)

// attentionFlagsTTL bounds how stale the cached flags can be: a flag flip takes effect
// within one TTL, without re-reading config.json on every 2s poll cycle and startup scan.
const attentionFlagsTTL = 5 * time.Second

type attentionFlagsCache struct {
	ttl  time.Duration
	load func() services.AttentionFlags
	now  func() time.Time

	mu      sync.Mutex
	flags   services.AttentionFlags
	expires time.Time
}

func (c *attentionFlagsCache) get() services.AttentionFlags {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now := c.now(); !now.Before(c.expires) {
		c.flags = c.load()
		c.expires = now.Add(c.ttl)
	}
	return c.flags
}

func (c *attentionFlagsCache) options() (session.DeterminerOptions, session.RuleOptions) {
	f := c.get()
	return session.DeterminerOptions{
			IdleClockOutput: f.IdleClockOutput,
			ProseQuestion:   f.ProseQuestionDetection,
		},
		session.RuleOptions{AckBypassFix: f.AckBypassFix}
}

// newAttentionFlagsCache caches load for attentionFlagsTTL.
func newAttentionFlagsCache(load func() services.AttentionFlags) *attentionFlagsCache {
	return &attentionFlagsCache{ttl: attentionFlagsTTL, load: load, now: time.Now}
}

// newAttentionOptionsSource adapts the live-settable attention flags, resolved
// through reg, to the session package, which cannot import server/services.
func newAttentionOptionsSource(reg *featureflags.Registry) session.FlagOptionsSource {
	return newAttentionFlagsCache(func() services.AttentionFlags {
		return services.LoadAttentionFlags(reg)
	}).options
}
