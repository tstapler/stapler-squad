package server

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
)

// Adapters from the server/wakes ports onto the services they stand in for.
// server/wakes never imports server/services; this file is the seam
// (ADR-006 rule 3).

// wakeSteerer writes wakes through SessionService, adding the wake-write
// safety step of ADR-006 rule 8 (lease, fresh capture, dialog check).
type wakeSteerer struct{ svc *services.SessionService }

func (s wakeSteerer) IsReadyForSteer(sessionUUID string) bool {
	return s.svc.IsReadyForSteer(sessionUUID)
}

func (s wakeSteerer) WriteWake(ctx context.Context, sessionUUID, text string) (wakes.WriteOutcome, error) {
	inst := s.svc.FindLiveInstance(sessionUUID)
	if inst == nil {
		return wakes.WriteNotAttempted, nil
	}
	return wakes.WriteIfIdle(ctx, instancePane{inst: inst}, text)
}

type instancePane struct{ inst *session.Instance }

func (p instancePane) Lock() (wakes.PaneLease, bool) {
	lease, ok := p.inst.TryTerminalWriteLease(session.LeaseWriterSteer)
	if !ok {
		return nil, false
	}
	return &instanceLease{inst: p.inst, lease: lease}, true
}

type instanceLease struct {
	inst  *session.Instance
	lease *session.HeldLease
}

func (l *instanceLease) Capture() (string, error) { return l.inst.CapturePaneContent() }

func (l *instanceLease) Release() { l.lease.Release() }

// Submit verifies the tmux pane still belongs to the session (a tmux-name
// collision would otherwise write into a stranger's pane), then writes.
func (l *instanceLease) Submit(ctx context.Context, text string) error {
	if err := session.VerifyPaneOwnershipBeforeWrite(ctx, l.inst); err != nil {
		l.lease.Release()
		return err
	}
	return session.SubmitContentWithEnter(ctx, l.inst, l.lease, text)
}

// wakeAdmission is BacklogService.Admit (the WIP cap); without a backlog
// service nothing is admitted, so a stopped owner is never revived.
type wakeAdmission struct{ backlog *services.BacklogService }

func (a wakeAdmission) Admit(ctx context.Context) (bool, error) {
	if a.backlog == nil {
		return false, nil
	}
	return a.backlog.Admit(ctx)
}

// wakeOwners resolves owner facts from the live registry, falling back to
// storage for sessions the registry no longer tracks. A stored but untracked
// owner is "exists, not live, no revive target": its wake can be skipped with
// a reason but not revived from here.
type wakeOwners struct {
	svc     *services.SessionService
	storage *session.Storage
}

func (o wakeOwners) Owner(ctx context.Context, sessionUUID string) (wakes.OwnerFacts, error) {
	var f wakes.OwnerFacts
	if inst := o.svc.FindLiveInstance(sessionUUID); inst != nil {
		snap := inst.Snapshot()
		f = wakes.OwnerFacts{
			Exists:   true,
			Archived: snap.ArchivedAt != nil,
			OneShot:  session.IsOneShot(inst),
			Live:     snap.Status == session.Active,
			Starting: isStartingStatus(snap.Status),

			WorkspaceKey: inst.WorkspaceKey(),
		}
		if inst.IsHotRestoreRecoverable() {
			f.Revive = inst
		}
	} else {
		data, err := o.storage.FindInstanceDataByID(sessionUUID)
		switch {
		case errors.Is(err, session.ErrInstanceDataNotFound):
			return wakes.OwnerFacts{}, nil
		case err != nil:
			return wakes.OwnerFacts{}, err
		}
		f = wakes.OwnerFacts{
			Exists:   true,
			Archived: data.ArchivedAt != nil,
			OneShot:  slices.Contains(data.Tags, "backlog:triage") || slices.Contains(data.Tags, "backlog:review"),

			WorkspaceKey: data.WorkspaceKey(),
		}
	}
	f.ItemTerminal = o.itemTerminal(ctx, sessionUUID)
	return f, nil
}

// isStartingStatus: a session that is still coming up is neither live nor stopped.
func isStartingStatus(s session.Status) bool {
	return s == session.Creating || s == session.Restoring
}

// itemTerminal reports whether the backlog item this session works is done or
// archived. A lookup failure reads as "not terminal": the wake is delivered.
func (o wakeOwners) itemTerminal(ctx context.Context, sessionUUID string) bool {
	link, err := o.storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		return false
	}
	item, err := o.storage.GetBacklogItem(ctx, link.BacklogItemID)
	if err != nil {
		return false
	}
	return item.Status == string(session.BacklogStatusDone) || item.Status == string(session.BacklogStatusArchived)
}

// wakeBacklogState reads the persisted item state reconcile predicates use.
type wakeBacklogState struct{ storage *session.Storage }

func (b wakeBacklogState) ItemState(ctx context.Context, itemID string) (wakes.ItemState, error) {
	item, err := b.storage.GetBacklogItem(ctx, itemID)
	if errors.Is(err, session.ErrNotFound) {
		return wakes.ItemState{}, nil
	}
	if err != nil {
		return wakes.ItemState{}, err
	}
	st := wakes.ItemState{
		Exists:   true,
		Archived: item.ArchivedAt != nil || item.Status == string(session.BacklogStatusArchived),
		Status:   item.Status,
	}
	for _, s := range item.ItemSessions {
		if s.ReviewVerdict != nil {
			st.HasVerdict = true
		}
		st.SessionRoles = append(st.SessionRoles, s.Role)
	}
	return st, nil
}

// wakeNotifier publishes operator notifications on the event bus. A reminder
// is INFO with no session binding (so the hidden-session gate cannot suppress
// it) and links its session through metadata.target_session_uuid. A failure is
// ERROR, bound to the owner when there is one, so a hidden-session owner still
// reaches the operator.
type wakeNotifier struct{ bus *events.EventBus }

func (n wakeNotifier) Notify(note wakes.Notification) {
	if n.bus == nil {
		return
	}
	meta := map[string]string{"target_session_uuid": note.OwnerSessionUUID}
	sessionID, ntype := "", sessionv1.NotificationType_NOTIFICATION_TYPE_INFO
	priority := sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
	notificationID := uuid.New().String()
	switch note.Kind {
	case wakes.NotifyFailure:
		sessionID, ntype = note.OwnerSessionUUID, sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
		priority = sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		meta[events.MetadataKeyDeliveryClass] = events.DeliveryClassFailure
	case wakes.NotifyPeerStalled:
		// Needs-human class, bound to the asker, keyed on the request so it is
		// one row per request however often it is raised.
		sessionID, ntype = note.OwnerSessionUUID, sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED
		priority = sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		meta[services.MetaGuidanceRequestID] = note.GuidanceRequestID
		meta[services.MetaPeerTitle] = note.PeerTitle
		notificationID = "peer-help-stalled:" + note.GuidanceRequestID
	}
	n.bus.Publish(events.NewNotificationEvent(
		sessionID, "", notificationID, int32(ntype), int32(priority), note.Title, note.Message, meta,
	))
	log.Info("wake notification", "kind", note.Kind, "title", note.Title)
}
