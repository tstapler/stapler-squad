// Package healthnotify holds the periodic operator-health notifiers: stale sessions and
// cgroup memory pressure. Both only publish to the event bus.
package healthnotify

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// staleSessionNotifierCheckInterval is how often the sweeper re-evaluates every live
// instance for staleness. 60s is fine-grained enough that a session crossing the
// (minutes-scale) configured threshold is flagged promptly, without the subprocess/lock
// overhead of the review queue poller's own multi-second cadence -- this sweeper only reads
// in-memory instance state (GetStatus/GetTimeSinceLastMeaningfulOutput), no I/O.
const staleSessionNotifierCheckInterval = 60 * time.Second

// InstancePoller is the notifiers' view of the live sessions.
type InstancePoller interface {
	GetInstances() []*instance.Instance
}

// Ports are the hub functions the notifiers need.
type Ports struct {
	// DerivePriority maps the (urgent, important) axes to a NotificationPriority value.
	// Nil is a wiring bug: the constructors log it and notify at unspecified priority.
	DerivePriority func(urgent, important bool) int32
}

func (p Ports) orUnspecified() Ports {
	if p.DerivePriority == nil {
		log.Error("healthnotify: Ports.DerivePriority is not wired; notifications publish at unspecified priority")
		p.DerivePriority = func(bool, bool) int32 {
			return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_UNSPECIFIED)
		}
	}
	return p
}

// StaleSessionNotifier is a small, independent, periodic sweeper that fires an
// edge-triggered, self-clearing notification the first time an ACTIVE session crosses the
// configured stale threshold (config.StaleSessionConfig), and re-arms after recovery so a
// later episode of staleness on the same session notifies again.
//
// Deliberately separate from the three pre-existing, independently-tuned staleness
// detectors elsewhere in this codebase (the review queue's 5-minute ReasonStale badge in
// session/review_queue_determiner.go, the rework-block gate's 15-minute check, and the
// 2-hour stuck-backlog-item detector's maxWorkSessionStaleness in
// session/backlog_lifecycle_stale.go) -- this sweeper does not read, modify, or share
// dedup state with any of them. It exists purely to raise an operator-facing notification
// event, not to change queue membership or backlog lifecycle state.
type StaleSessionNotifier struct {
	poller   InstancePoller
	eventBus *events.EventBus
	ports    Ports

	mu sync.Mutex
	// notifiedSessions tracks sessions that have already fired an ACTUAL notification for
	// the CURRENT stale episode, keyed by stable ID (UUID, falling back to Title). An entry
	// is written only when a notification is actually sent -- if notify_enabled is false
	// when a session first crosses threshold, no entry is recorded, so the session remains
	// eligible to notify later in that same episode once the flag is turned on (see the
	// *_should_FireOnceOnceEnabled_When_NotifyWasDisabledDuringInitialStaleCrossing test).
	// An entry is removed (re-arming the session) when it recovers below the threshold, or
	// when it transitions away from ACTIVE for any reason -- including a pause-then-resume
	// that never dropped below threshold, so that sequence notifies again on resume rather
	// than staying silently suppressed forever (see the
	// *_should_ReNotify_When_SessionPausesThenResumesStillStale test).
	notifiedSessions map[string]time.Time
}

// NewStaleSessionNotifier constructs a notifier. poller supplies the live set of instances
// to evaluate (via GetInstances()); eventBus receives the notification events. Neither
// dependency is optional in production, but a nil eventBus is tolerated (notify becomes a
// no-op) so this can be safely constructed before the event bus exists during startup
// wiring.
func NewStaleSessionNotifier(poller InstancePoller, eventBus *events.EventBus, ports Ports) *StaleSessionNotifier {
	return &StaleSessionNotifier{
		poller:           poller,
		eventBus:         eventBus,
		ports:            ports.orUnspecified(),
		notifiedSessions: make(map[string]time.Time),
	}
}

// Start runs the periodic check loop. Blocks until ctx is cancelled. Mirrors
// SessionRetentionSweeper.Start's shape: run once immediately, then on every tick.
func (n *StaleSessionNotifier) Start(ctx context.Context) {
	ticker := time.NewTicker(staleSessionNotifierCheckInterval)
	defer ticker.Stop()

	log.Info("stale session notifier started", "check_interval", staleSessionNotifierCheckInterval)

	// Run immediately on start rather than waiting for the first tick.
	n.checkAll()

	for {
		select {
		case <-ctx.Done():
			log.Info("stale session notifier stopped")
			return
		case <-ticker.C:
			n.checkAll()
		}
	}
}

// checkAll evaluates every live instance against the current configured threshold.
//
// config.LoadConfig() is called fresh on every invocation -- a cheap local JSON file
// read -- rather than caching a *config.Config on the struct at construction time, so a
// threshold or notify-enabled change made via the Settings UI takes effect on the very
// next tick with no server restart required.
func (n *StaleSessionNotifier) checkAll() {
	cfg := config.LoadConfig()
	threshold := time.Duration(cfg.StaleSession.ThresholdMinutesOrDefault()) * time.Minute
	notifyEnabled := cfg.StaleSession.NotifyEnabledOrDefault()

	for _, inst := range n.poller.GetInstances() {
		stableID := inst.GetStableID()

		if domain.Status(inst.GetStatus()) != domain.Active {
			// Clear the dedup entry on ANY active->non-active transition, not just
			// idle-time recovery -- a session paused while still stale and later
			// resumed while still past threshold must notify again (see doc comment
			// on notifiedSessions).
			n.mu.Lock()
			delete(n.notifiedSessions, stableID)
			n.mu.Unlock()
			continue
		}

		idle := inst.GetTimeSinceLastMeaningfulOutput()

		shouldNotify := false
		n.mu.Lock()
		_, alreadyNotified := n.notifiedSessions[stableID]
		switch {
		case idle > threshold && !alreadyNotified && notifyEnabled:
			// Only record the dedup entry when a notification is actually about to be
			// sent. If notifyEnabled is false here, deliberately leave no entry so this
			// same stale episode can still fire once the flag is turned on later,
			// instead of being permanently swallowed.
			n.notifiedSessions[stableID] = time.Now()
			shouldNotify = true
		case idle <= threshold && alreadyNotified:
			delete(n.notifiedSessions, stableID)
		}
		n.mu.Unlock()

		if shouldNotify {
			n.notify(inst, idle)
		}
	}
}

// notify publishes an operator-facing WARNING/MEDIUM notification event for inst going
// stale. Called outside n.mu -- eventBus.Publish does its own synchronization and must
// never run while n.mu is held.
func (n *StaleSessionNotifier) notify(inst *instance.Instance, idle time.Duration) {
	if n.eventBus == nil {
		return
	}
	sessionID := inst.GetStableID()
	n.eventBus.Publish(events.NewNotificationEvent(
		sessionID, inst.Title, uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		n.ports.DerivePriority(false, false), // urgent, important — routine self-monitoring telemetry, not actionable on its own
		"Session went stale",
		fmt.Sprintf("%s has produced no output for %s.", inst.Title, idle.Round(time.Second)),
		map[string]string{"session_id": sessionID, "reason": "stale"},
	))
}
