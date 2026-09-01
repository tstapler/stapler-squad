package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/telemetry"
)

// memoryPressureCheckInterval mirrors StaleSessionNotifier's cadence — this
// reads two small cgroup files (or, in tests, calls an injected func), so
// there's no I/O-cost reason to check less often.
const memoryPressureCheckInterval = 60 * time.Second

// memoryPressureWarnRatio/memoryPressureClearRatio are the rising/falling
// edges of memory.current as a fraction of memory.high. The gap between them
// (hysteresis) exists because the 2026-08-25 investigation found usage
// sitting pinned right at the ceiling for extended periods — a single
// threshold would either never re-arm (chronic 90%+) or flap on every tick.
const (
	memoryPressureWarnRatio  = 0.90
	memoryPressureClearRatio = 0.75
)

// memoryPressureIdleFloor is how long a session must have produced no output
// before it's suggested as a pause candidate in the notification body. Higher
// than a "just thinking" pause, low enough to still be a useful list.
const memoryPressureIdleFloor = 10 * time.Minute

// MemoryPressureNotifier is a small, independent, periodic sweeper — same
// shape as StaleSessionNotifier — that fires an edge-triggered,
// self-clearing operator notification when this process's own cgroup memory
// usage crosses memoryPressureWarnRatio of its MemoryHigh ceiling
// (scripts/install-service.sh), and re-arms once usage recovers below
// memoryPressureClearRatio. The notification body lists idle sessions as
// concrete "reclaim memory" options (pausing a session's tmux/Claude process
// frees whatever it was holding) rather than just raising an alarm with
// nothing actionable attached.
type MemoryPressureNotifier struct {
	poller    *session.ReviewQueuePoller
	eventBus  *events.EventBus
	ratioFunc func() (ratio float64, ok bool)

	mu       sync.Mutex
	notified bool
}

// NewMemoryPressureNotifier constructs a notifier using the real cgroup
// reader (telemetry.CgroupMemoryUsageRatio). A nil eventBus is tolerated
// (notify becomes a no-op), matching StaleSessionNotifier.
func NewMemoryPressureNotifier(poller *session.ReviewQueuePoller, eventBus *events.EventBus) *MemoryPressureNotifier {
	return &MemoryPressureNotifier{
		poller:    poller,
		eventBus:  eventBus,
		ratioFunc: telemetry.CgroupMemoryUsageRatio,
	}
}

// Start runs the periodic check loop. Blocks until ctx is cancelled. Mirrors
// StaleSessionNotifier.Start's shape.
func (n *MemoryPressureNotifier) Start(ctx context.Context) {
	ticker := time.NewTicker(memoryPressureCheckInterval)
	defer ticker.Stop()

	log.Info("memory pressure notifier started", "check_interval", memoryPressureCheckInterval,
		"warn_ratio", memoryPressureWarnRatio, "clear_ratio", memoryPressureClearRatio)

	n.checkOnce()

	for {
		select {
		case <-ctx.Done():
			log.Info("memory pressure notifier stopped")
			return
		case <-ticker.C:
			n.checkOnce()
		}
	}
}

// checkOnce evaluates the current ratio against the warn/clear thresholds
// and fires or re-arms the single (host-wide, not per-session) notification.
func (n *MemoryPressureNotifier) checkOnce() {
	ratio, ok := n.ratioFunc()
	if !ok {
		return // non-Linux, or memory.high is unset ("max") — nothing to warn about
	}

	shouldNotify := false
	n.mu.Lock()
	switch {
	case ratio >= memoryPressureWarnRatio && !n.notified:
		n.notified = true
		shouldNotify = true
	case ratio < memoryPressureClearRatio && n.notified:
		n.notified = false
	}
	n.mu.Unlock()

	if shouldNotify {
		n.notify(ratio)
	}
}

// notify publishes a host-wide WARNING/HIGH notification event listing idle
// sessions as pause candidates. Called outside n.mu, matching
// StaleSessionNotifier.notify's rule — eventBus.Publish must never run while
// n.mu is held.
func (n *MemoryPressureNotifier) notify(ratio float64) {
	if n.eventBus == nil {
		return
	}
	n.eventBus.Publish(events.NewNotificationEvent(
		"system", "System", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH),
		"Memory usage near limit",
		fmt.Sprintf("Process memory is at %.0f%% of its configured limit. %s Dashboard: http://localhost:3000",
			ratio*100, n.reclaimOptionsText()),
		map[string]string{"reason": "memory_pressure"},
	))
}

// reclaimOptionsText lists idle sessions (no output for at least
// memoryPressureIdleFloor) as concrete pause candidates, sorted longest-idle
// first so the most obviously-safe-to-pause sessions lead the list.
func (n *MemoryPressureNotifier) reclaimOptionsText() string {
	if n.poller == nil {
		return ""
	}
	type candidate struct {
		title string
		idle  time.Duration
	}
	var candidates []candidate
	for _, inst := range n.poller.GetInstances() {
		if session.Status(inst.GetStatus()) != session.Active {
			continue
		}
		if idle := inst.GetTimeSinceLastMeaningfulOutput(); idle >= memoryPressureIdleFloor {
			candidates = append(candidates, candidate{inst.Title, idle})
		}
	}
	if len(candidates) == 0 {
		return "No idle sessions found to suggest pausing."
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].idle > candidates[j].idle })

	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = fmt.Sprintf("%s (idle %s)", c.title, c.idle.Round(time.Minute))
	}
	return fmt.Sprintf("%d idle session(s) could be paused to free memory: %s.", len(candidates), strings.Join(names, ", "))
}
