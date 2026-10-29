package server

import (
	"errors"
	"os"
	"path/filepath"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/session"
)

// prwatchNotifier publishes PR-watch operator notifications on the event bus
// (D15), mirroring wakeNotifier. DedupeKey is the notification ID so a repeat
// overwrites one row. Informational kinds are unbound from any session so the
// hidden-session gate cannot suppress them; failure kinds are bound to the
// session and stamped as failure class so a hidden-session owner still
// reaches the operator.
type prwatchNotifier struct{ bus *events.EventBus }

var _ prwatch.Notifier = prwatchNotifier{}

// Notify implements prwatch.Notifier.
func (n prwatchNotifier) Notify(note prwatch.Notification) error {
	if n.bus == nil {
		return errors.New("prwatch notifier: no event bus")
	}
	if note.DedupeKey == "" {
		return errors.New("prwatch notifier: empty dedupe key")
	}
	meta := map[string]string{}
	sessionID, ntype := "", sessionv1.NotificationType_NOTIFICATION_TYPE_INFO
	priority := sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
	switch note.Kind {
	case prwatch.NotifySteerFailed, prwatch.NotifyHeldDropped:
		sessionID, ntype = note.SessionUUID, sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
		priority = sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		meta[events.MetadataKeyDeliveryClass] = events.DeliveryClassFailure
	case prwatch.NotifyInboxState:
		// Problem states are errors; the recovery notice and the CI-only
		// notice (State ok or empty) stay informational.
		if note.State != "" && note.State != prwatch.InboxStateOK {
			ntype = sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
		}
	}
	if note.SessionUUID != "" {
		meta["target_session_uuid"] = note.SessionUUID
	}
	n.bus.Publish(events.NewNotificationEvent(
		sessionID, "", note.DedupeKey, int32(ntype), int32(priority), note.Title, note.Message, meta,
	))
	return nil
}

// newPRWatchNotifications builds the notification builders over the bus. The
// CoverageGap reason (third-party text) goes through session.SanitizeUntrusted.
func newPRWatchNotifications(bus *events.EventBus, clock prwatch.Clock, s prwatch.Settings) *prwatch.Notifications {
	return prwatch.NewNotifications(prwatchNotifier{bus: bus}, clock, s, session.SanitizeUntrusted)
}

// prwatchGuidePath returns the absolute path of the operator doc. buildinfo
// has no checkout root (Worktree is the string "true"/"false", set only by
// the Makefile), so it walks up from the running binary and the working
// directory; failing that it returns the fixed fallback text.
func prwatchGuidePath() string {
	var starts []string
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		starts = append(starts, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	return prwatch.ResolveGuidePath(starts, nil)
}
