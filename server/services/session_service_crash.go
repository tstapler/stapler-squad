package services

import (
	"fmt"
	"strings"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// Crash notifications (Story 2.10, decision O3): a session entering Crashed
// had no bus producer, so it was silent on every channel. sessionExitedPublisher
// now publishes one FAILURE for it, which the gate delivers (failure class)
// even for hidden sessions.
const (
	crashWindow             = time.Minute
	crashIndividualPerWin   = 3
	crashSummaryTitles      = 5
	maxCrashReasonRunes     = 160
	crashSummaryIDPrefix    = "session-crashed-summary-"
	crashNotificationIDBase = "session-crashed-"
)

// crashLimiter is the minimal storm guard: at most crashIndividualPerWin
// individual notifications per crashWindow, the rest folded into one summary.
// One process-wide limiter, so hidden and visible crashes share it.
type crashLimiter struct {
	mu          sync.Mutex
	now         func() time.Time // nil means time.Now; tests inject a fake
	windowStart time.Time
	individual  int
	folded      int
	titles      []string
}

type crashDecision struct {
	Individual  bool
	WindowStart time.Time
	Folded      int
	Titles      []string
	More        int // folded crashes beyond the listed titles
}

func (c *crashLimiter) admit(title string) crashDecision {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	if c.windowStart.IsZero() || now.Sub(c.windowStart) >= crashWindow {
		c.windowStart, c.individual, c.folded, c.titles = now, 0, 0, nil
	}
	if c.individual < crashIndividualPerWin {
		c.individual++
		return crashDecision{Individual: true}
	}
	c.folded++
	if len(c.titles) < crashSummaryTitles {
		c.titles = append(c.titles, title)
	}
	return crashDecision{
		WindowStart: c.windowStart, Folded: c.folded,
		Titles: append([]string(nil), c.titles...), More: c.folded - len(c.titles),
	}
}

// NewSessionCrashEvent builds the FAILURE notification for one crashed session.
// The id carries the exit time so a resumed session that crashes again
// re-fires (the notification store coalesces history by session and type).
func NewSessionCrashEvent(uuid, title, exitReason string, exitedAt time.Time) *events.Event {
	msg := fmt.Sprintf("Session %q crashed", sanitizeNotificationText(title))
	if r := strings.TrimSpace(sanitizeNotificationText(exitReason)); r != "" {
		msg += ": " + truncateString(r, maxCrashReasonRunes)
	}
	return events.NewNotificationEvent(
		uuid, title, fmt.Sprintf("%s%s-%d", crashNotificationIDBase, uuid, exitedAt.Unix()),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE),
		derivePriority(true, true),
		"Session crashed", msg,
		events.SessionScopedMetadata(nil, ""),
	)
}

func newCrashSummaryEvent(d crashDecision) *events.Event {
	titles := make([]string, len(d.Titles))
	for i, t := range d.Titles {
		titles[i] = sanitizeNotificationText(t)
	}
	msg := fmt.Sprintf("%d more sessions crashed: %s", d.Folded, strings.Join(titles, ", "))
	if d.More > 0 {
		msg += fmt.Sprintf(" and %d more", d.More)
	}
	return events.NewNotificationEvent(
		"", "", fmt.Sprintf("%s%d", crashSummaryIDPrefix, d.WindowStart.Unix()),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE),
		derivePriority(true, true),
		"Sessions crashed", msg, nil,
	)
}

// publishCrash publishes the crash notification for snap, or nothing when the
// snapshot is not Crashed (another exit source). It takes a snapshot, never a
// live Instance, so the status read is the same lock-free read everywhere.
func (s *SessionService) publishCrash(snap *session.InstanceSnapshot) {
	if snap.Status != session.Crashed {
		log.Debug("session exited without being crashed; no crash notification",
			"session", snap.Title, "status", snap.Status.String())
		return
	}
	exitedAt := snap.UpdatedAt
	if exitedAt.IsZero() {
		exitedAt = time.Now()
	}
	log.Warn("session crashed", "session", snap.Title, "uuid", snap.UUID, "hidden", snap.Hidden, "exit_reason", snap.ExitReason)
	d := s.crashes.admit(snap.Title)
	if d.Individual {
		s.eventBus.Publish(NewSessionCrashEvent(snap.UUID, snap.Title, snap.ExitReason, exitedAt))
		return
	}
	if s.deliveryGate != nil {
		s.deliveryGate.Metrics().Add(deliverygate.CounterCrashCoalesced)
	}
	log.Info("crash notification coalesced into summary", "session", snap.Title, "folded", d.Folded)
	s.eventBus.Publish(newCrashSummaryEvent(d))
}
