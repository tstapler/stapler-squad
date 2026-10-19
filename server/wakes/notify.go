package wakes

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	payloadSnippetRunes = 80
	digestOwnersListed  = 5
	digestOwnerInterval = time.Hour
	ownerGoneText       = "Session no longer exists"
)

func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > payloadSnippetRunes {
		return string(r[:payloadSnippetRunes]) + "..."
	}
	return s
}

// failureNotification tells the operator a scheduled prompt ended without
// reaching its owner. It is failure class (ERROR) and bound to the owner so a
// hidden-session owner still gets through the delivery gate (a routine type
// would be suppressed for hidden sessions).
func failureNotification(row *session.WakeTriggerData, state domain.WakeStatus, reason domain.WakeReason) Notification {
	return Notification{
		Kind:             NotifyFailure,
		OwnerSessionUUID: row.OwnerSessionUUID,
		Title:            fmt.Sprintf("Scheduled prompt %s", state),
		Message:          fmt.Sprintf("%s (%s): %q", state, reason, snippet(row.Action.Payload)),
	}
}

// reminderNotification is the INFO notification of a delivered notify_operator
// wake. The owner is only a link target: the notification carries no session
// binding, so the hidden-session gate cannot suppress a reminder about a
// finished review session.
func reminderNotification(row *session.WakeTriggerData, ownerExists bool) Notification {
	msg := snippet(row.Action.Payload)
	if !ownerExists {
		msg = ownerGoneText + ": " + msg
	}
	return Notification{Kind: NotifyReminder, OwnerSessionUUID: row.OwnerSessionUUID, Title: "Reminder", Message: msg}
}

// digest folds the missed and skipped rows of one reconcile run into a single
// failure-class notification, however many rows ended (pre-mortem #6).
type digest struct {
	mu      sync.Mutex
	byOwner map[string]int
	total   int
}

func newDigest() *digest { return &digest{byOwner: map[string]int{}} }

func (dg *digest) add(owner string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	dg.byOwner[owner]++
	dg.total++
}

// build returns the digest notification, or ok=false when nothing is left.
// Owners digested within the last hour are left out (recent maps owner to its
// last digest time) and are stamped now otherwise.
func (dg *digest) build(now time.Time, recent map[string]time.Time) (Notification, bool) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	owners := make([]string, 0, len(dg.byOwner))
	total := 0
	for o, n := range dg.byOwner {
		if last, ok := recent[o]; ok && now.Sub(last) < digestOwnerInterval {
			continue
		}
		owners = append(owners, o)
		total += n
	}
	if total == 0 {
		return Notification{}, false
	}
	sort.Strings(owners)
	for _, o := range owners {
		recent[o] = now
	}
	listed := owners
	if len(listed) > digestOwnersListed {
		listed = listed[:digestOwnersListed]
	}
	msg := fmt.Sprintf("%d scheduled prompts were missed or skipped during the restart. Sessions: %s", total, strings.Join(listed, ", "))
	if rest := len(owners) - len(listed); rest > 0 {
		msg += fmt.Sprintf(" and %d more", rest)
	}
	return Notification{Kind: NotifyFailure, Title: "Scheduled prompts missed", Message: msg}, true
}
