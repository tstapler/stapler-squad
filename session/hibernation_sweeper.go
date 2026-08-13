package session

import (
	"context"
	"sort"
	"sync"
	"time"

	appconfig "github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/memory"
)

// LiveInstancesProvider is satisfied by ReviewQueuePoller. It returns the live
// in-memory instances without constructing new Instance objects or spawning PTY
// processes. HibernationSweeper uses this as a fast path to avoid LoadInstances().
type LiveInstancesProvider interface {
	GetInstances() []*Instance
}

// MemoryCacheReader is implemented by HibernationSweeper so SessionService can
// read cached RSS values without importing the sweeper concretely.
type MemoryCacheReader interface {
	GetCachedRSSMB(sessionUUID string) int64
	SystemMemoryPct() (float64, error)
}

// sweepInterval is how often the sweeper checks for idle or pressure-driven hibernation.
const sweepInterval = 5 * time.Minute

// cacheTTL is the duration a per-session RSS reading remains valid before refresh.
const cacheTTL = 30 * time.Second

// pressureGracePeriod is the minimum idle time before a session is eligible for
// resource-pressure hibernation (prevents interrupting recently active sessions).
const pressureGracePeriod = 5 * time.Minute

// sysMemCacheTTL is how long a system memory percentage reading remains valid.
const sysMemCacheTTL = 5 * time.Second

// rssWarmTimeout caps total wall-clock time spent warming the RSS cache per sweep
// so a hung /proc read cannot block the sweep goroutine indefinitely.
const rssWarmTimeout = 30 * time.Second

// memoryCacheEntry holds one cached RSS measurement.
type memoryCacheEntry struct {
	rssMB     int64
	fetchedAt time.Time
}

// sessionMemoryCache is a TTL cache mapping session UUID → RSS in MB.
type sessionMemoryCache struct {
	mu      sync.Mutex
	entries map[string]memoryCacheEntry
}

func newSessionMemoryCache() *sessionMemoryCache {
	return &sessionMemoryCache{entries: make(map[string]memoryCacheEntry)}
}

// GetOrFetch returns the cached RSS for uuid, or calls fetchFn if the entry is
// absent or expired. The mutex is released before calling fetchFn to avoid
// holding the lock during potentially slow I/O.
func (c *sessionMemoryCache) GetOrFetch(uuid string, fetchFn func() int64) int64 {
	c.mu.Lock()
	entry, ok := c.entries[uuid]
	if ok && time.Since(entry.fetchedAt) < cacheTTL {
		c.mu.Unlock()
		return entry.rssMB
	}
	c.mu.Unlock()

	val := fetchFn()

	c.mu.Lock()
	// Re-check: another goroutine may have fetched while we computed.
	if e, ok := c.entries[uuid]; ok && time.Since(e.fetchedAt) < cacheTTL {
		c.mu.Unlock()
		return e.rssMB
	}
	c.entries[uuid] = memoryCacheEntry{rssMB: val, fetchedAt: time.Now()}
	c.mu.Unlock()
	return val
}

// Get returns the cached RSS for uuid, or 0 if absent/expired.
func (c *sessionMemoryCache) Get(uuid string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[uuid]
	if !ok || time.Since(entry.fetchedAt) >= cacheTTL {
		return 0
	}
	return entry.rssMB
}

// Invalidate removes the cache entry for uuid, forcing a re-fetch on the next call.
func (c *sessionMemoryCache) Invalidate(uuid string) {
	c.mu.Lock()
	delete(c.entries, uuid)
	c.mu.Unlock()
}

// HibernationSweeper periodically checks all sessions and hibernates those
// that have been idle longer than the configured timeout or that are consuming
// memory while the system is under pressure.
type HibernationSweeper struct {
	storage      *Storage
	cfg          *appconfig.Config
	liveProvider LiveInstancesProvider
	memReader    memory.Reader
	memCache     *sessionMemoryCache

	sysMemPct float64
	sysMemAt  time.Time
	sysMemMu  sync.Mutex
}

// NewHibernationSweeper creates a HibernationSweeper using the given storage, config,
// and memory reader.
func NewHibernationSweeper(storage *Storage, cfg *appconfig.Config, reader memory.Reader) *HibernationSweeper {
	return &HibernationSweeper{
		storage:   storage,
		cfg:       cfg,
		memReader: reader,
		memCache:  newSessionMemoryCache(),
	}
}

// SetLiveProvider wires the fast-path instance source. Call this after constructing
// the ReviewQueuePoller so that sweep() uses live in-memory instances instead of
// calling LoadInstances() (which spawns PTY/tmux subprocesses).
func (s *HibernationSweeper) SetLiveProvider(p LiveInstancesProvider) {
	s.liveProvider = p
}

// GetCachedRSSMB returns the last-measured RSS in MB for the given session UUID.
// Returns 0 if not yet measured or entry expired. Implements MemoryCacheReader.
func (s *HibernationSweeper) GetCachedRSSMB(sessionUUID string) int64 {
	return s.memCache.Get(sessionUUID)
}

// SystemMemoryPct returns the current system memory usage percentage.
// The result is cached for sysMemCacheTTL to avoid a syscall on every
// ListSessions request. The mutex is released before calling the reader to
// avoid holding the lock during /proc I/O. Implements MemoryCacheReader.
func (s *HibernationSweeper) SystemMemoryPct() (float64, error) {
	if s.memReader == nil {
		return 0, nil
	}
	s.sysMemMu.Lock()
	if time.Since(s.sysMemAt) < sysMemCacheTTL {
		cached := s.sysMemPct
		s.sysMemMu.Unlock()
		return cached, nil
	}
	s.sysMemMu.Unlock()

	pct, err := s.memReader.SystemMemoryPct()
	if err != nil {
		return 0, err
	}

	s.sysMemMu.Lock()
	// Re-check: another goroutine may have refreshed while we read /proc.
	if time.Since(s.sysMemAt) >= sysMemCacheTTL {
		s.sysMemPct = pct
		s.sysMemAt = time.Now()
	}
	s.sysMemMu.Unlock()
	return pct, nil
}

// Start runs the periodic sweep loop. Blocks until ctx is cancelled.
func (s *HibernationSweeper) Start(ctx context.Context) {
	interval := sweepInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Info("hibernation sweeper started",
		"idle_timeout_minutes", s.cfg.Hibernation.IdleTimeoutMinutes,
		"check_interval", interval)

	// Run immediately on start rather than waiting for the first tick.
	s.sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("hibernation sweeper stopped")
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep examines all Active sessions and hibernates those that have exceeded the
// idle timeout or are consuming memory while the system is under pressure.
func (s *HibernationSweeper) sweep(ctx context.Context) {
	if !s.cfg.Hibernation.Enabled {
		return
	}

	var instances []*Instance
	if s.liveProvider != nil {
		instances = s.liveProvider.GetInstances()
	} else {
		var err error
		instances, err = s.storage.LoadInstances()
		if err != nil {
			log.Error("hibernation sweeper: failed to load instances", "err", err)
			return
		}
	}

	// Idle-timeout hibernation.
	// A session is eligible only when: active, idle ≥ timeout, AND not in the
	// review queue (which would mean Claude is waiting for user input — hibernating
	// that session would silently discard a pending approval or question).
	idleTimeout := time.Duration(s.cfg.Hibernation.IdleTimeoutMinutes) * time.Minute
	if idleTimeout > 0 {
		for _, inst := range instances {
			if !inst.IsActive() {
				continue
			}
			if inst.NeedsReview() {
				continue
			}
			idle := inst.TimeSinceLastMeaningfulOutput(inst.CreatedAt)
			if idle >= idleTimeout {
				log.Info("auto-hibernating idle session",
					"session", inst.Title,
					"idle_duration", idle.Round(time.Minute))
				inst.SetHibernateReason("idle")
				if err := inst.Hibernate(ctx); err != nil {
					log.Warn("auto-hibernate failed", "session", inst.Title, "err", err)
					continue
				}
				s.memCache.Invalidate(inst.UUID)
				if err := s.storage.SaveInstances(instances); err != nil {
					log.Warn("auto-hibernate: failed to save instance state",
						"session", inst.Title, "err", err)
				}
			}
		}
	}

	// Warm per-session RSS cache so GetCachedRSSMB returns current values to
	// ListSessions regardless of whether resource-pressure hibernation is enabled.
	s.warmRSSCache(ctx, instances)

	// Resource-pressure hibernation (new behaviour).
	s.sweepResourcePressure(ctx, instances)
}

// warmRSSCache reads current RSS for all active sessions and stores results in
// the cache. Called from sweep() on every tick — not gated on ResourcePressureThreshold
// — so that memory_rss_mb in the UI is always populated when memReader is configured.
func (s *HibernationSweeper) warmRSSCache(ctx context.Context, instances []*Instance) {
	if s.memReader == nil {
		return
	}
	deadline := time.Now().Add(rssWarmTimeout)
	for _, inst := range instances {
		if !inst.IsActive() {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		if time.Now().After(deadline) {
			log.Warn("hibernation sweeper: RSS warming timed out, some sessions skipped")
			return
		}
		capturedName := inst.GetTmuxSessionName()
		capturedUUID := inst.UUID
		s.memCache.GetOrFetch(capturedUUID, func() int64 {
			rss, err := s.memReader.SessionRSSMB(capturedName)
			if err != nil {
				return 0
			}
			return rss
		})
	}
}

// sweepResourcePressure hibernates the single longest-idle eligible session when
// system memory usage is at or above the configured threshold. It does nothing if
// the threshold is 0 (disabled) or the system memory percentage is 0 (macOS/error).
func (s *HibernationSweeper) sweepResourcePressure(ctx context.Context, instances []*Instance) {
	threshold := s.cfg.Hibernation.ResourcePressureThreshold
	if threshold <= 0 || s.memReader == nil {
		return
	}

	sysPct, err := s.SystemMemoryPct()
	if err != nil {
		log.Warn("hibernation sweeper: cannot read system memory", "err", err)
		return
	}
	if sysPct == 0 {
		// Treat 0 as "unavailable" (macOS sentinel or measurement error).
		return
	}
	if sysPct < float64(threshold) {
		return
	}

	log.Info("system memory pressure detected",
		"used_pct", sysPct,
		"threshold", threshold)

	// Collect eligible candidates: Active sessions idle > pressureGracePeriod
	// that are NOT waiting for user input in the review queue.
	type candidate struct {
		inst    *Instance
		idleFor time.Duration
	}
	var candidates []candidate
	for _, inst := range instances {
		if !inst.IsActive() {
			continue
		}
		if inst.NeedsReview() {
			continue
		}
		idle := inst.TimeSinceLastMeaningfulOutput(inst.CreatedAt)
		if idle < pressureGracePeriod {
			continue
		}
		candidates = append(candidates, candidate{inst: inst, idleFor: idle})
	}

	if len(candidates) == 0 {
		return
	}

	// Sort descending by idle time — hibernate the longest-idle session first.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].idleFor > candidates[j].idleFor
	})

	target := candidates[0].inst
	log.Info("auto-hibernating session for resource pressure",
		"session", target.Title,
		"idle_duration", candidates[0].idleFor.Round(time.Minute),
		"system_memory_pct", sysPct)

	target.SetHibernateReason("resource_pressure")
	if err := target.Hibernate(ctx); err != nil {
		log.Warn("resource-pressure hibernate failed", "session", target.Title, "err", err)
		return
	}

	s.memCache.Invalidate(target.UUID)

	if err := s.storage.SaveInstances(instances); err != nil {
		log.Warn("resource-pressure hibernate: failed to save instance state",
			"session", target.Title, "err", err)
	}
}
