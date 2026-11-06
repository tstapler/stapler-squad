package server

import (
	"context"
	"fmt"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// prSessionLookup resolves the sessions linked to a PR (session.Storage satisfies it).
type prSessionLookup interface {
	SessionUUIDsByPR(ctx context.Context, ref session.PRRef) ([]string, error)
}

const prUnknownLookupTimeout = 5 * time.Second

// newPRUnknownNotifier returns the PRStatusPoller.SetOnPRUnknown callback: it
// raises one warning per session linked to the PR, or one keyed by the PR when
// only an item holds it. The poller already fires this once per transition.
func newPRUnknownNotifier(lookup prSessionLookup, n session.Notifier) func(session.PRView) {
	return func(v session.PRView) {
		ctx, cancel := context.WithTimeout(context.Background(), prUnknownLookupTimeout)
		defer cancel()
		uuids, err := lookup.SessionUUIDsByPR(ctx, v.Ref)
		if err != nil {
			log.Warn("PR unknown notification: session lookup failed", "pr", v.Ref, "err", err)
		}
		if len(uuids) == 0 {
			uuids = []string{v.Ref.String()}
		}
		msg := fmt.Sprintf("GitHub status for %s could not be read (%s); it blocks marking work done until it refreshes or you stop tracking it.", v.Ref, v.PollError)
		for _, id := range uuids {
			n.NotifySession(id, "Pull request status unknown", msg, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING), false, true)
		}
	}
}
