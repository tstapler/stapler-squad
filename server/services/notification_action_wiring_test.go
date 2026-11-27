package services

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/notifications"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/auditkit"
	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
	"github.com/tstapler/stapler-squad/server/services/guidance"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

type wiredFakes struct {
	moved, wakes, peers []string
}

func (f *wiredFakes) TransitionBacklogItemStatus(_ context.Context, req *connect.Request[sessionv1.TransitionBacklogItemStatusRequest]) (*connect.Response[sessionv1.TransitionBacklogItemStatusResponse], error) {
	f.moved = append(f.moved, req.Msg.GetExpectedStatus()+">"+req.Msg.GetTargetStatus())
	return connect.NewResponse(&sessionv1.TransitionBacklogItemStatusResponse{}), nil
}

func (f *wiredFakes) CancelWake(_ context.Context, id string) (questionkit.ActionResult, error) {
	f.wakes = append(f.wakes, id)
	return questionkit.Applied(), nil
}

func (f *wiredFakes) CancelPeerHelp(_ context.Context, id string) (questionkit.ActionResult, error) {
	f.peers = append(f.peers, id)
	return questionkit.Applied(), nil
}

type wiredEnv struct {
	svc      *SessionService
	history  *notifications.NotificationHistoryStore
	guidance *guidance.GuidanceRequestService
	fakes    *wiredFakes
	gid      string
	itemID   string
	storage  *session.Storage
	auditFS  *memfs.MemFS
}

// newWiredEnv wires every optional collaborator and seeds one card each for an
// approval, a backlog decision and a guidance request.
func newWiredEnv(t *testing.T) *wiredEnv {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	ctx := context.Background()
	cfg := config.LoadConfig()
	for _, sc := range []string{
		"action:APPROVE", "action:ANSWER_GUIDANCE", "action:BACKLOG_DECISION", "action:ANSWER_QUESTION",
		"action:CANCEL_WAKE", "action:CANCEL_PEER_HELP", "channel:remote",
	} {
		require.NoError(t, cfg.SetFeatureFlagScope("notification_inline_actions", sc, true))
	}
	for _, st := range []string{"stage:render", "stage:answer"} {
		require.NoError(t, cfg.SetFeatureFlagScope("askuserquestion_structured", st, true))
	}

	storage := svctest.CreateTestStorage(t)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	e := &wiredEnv{svc: NewSessionService(storage, bus), fakes: &wiredFakes{}, guidance: guidance.NewGuidanceRequestService(storage, nil, nil)}
	t.Cleanup(e.svc.Shutdown)
	history, err := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))
	require.NoError(t, err)
	e.history = history
	e.svc.SetNotificationStore(history)
	require.NoError(t, e.svc.GetApprovalStore().Create(&approvalkit.PendingApproval{ID: "appr-1", SessionID: "s1", ToolName: "Bash"}))
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "t", Status: "review"})
	require.NoError(t, err)
	created, err := e.guidance.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope: "standalone", QuestionText: "Which target?", QuestionType: "short-answer",
	}))
	require.NoError(t, err)
	e.gid = created.Msg.GetRequest().GetId()
	e.itemID = item.ID
	e.storage = storage
	e.auditFS = memfs.New()
	sink := auditkit.NewAuditSink(func() (string, error) { return "/cfg", nil }, auditkit.WithAuditFS(e.auditFS))
	t.Cleanup(sink.Close)
	e.svc.featureFlagSvc.SetAudit(sink, nil)
	e.seedCards(t)
	e.svc.WireNotificationActions(NotificationActionWiring{Storage: storage, Guidance: e.guidance, Backlog: e.fakes, Cancels: e.fakes})
	return e
}

// linkWorkSession gives the item a work session no instance backs, then confirms
// a merge: the answer comes from the real merger, SessionService.MergePR.
func (e *wiredEnv) linkWorkSession(t *testing.T, kind sessionv1.ActionKind) string {
	t.Helper()
	_, err := e.storage.CreateItemSession(context.Background(), backlogdto.ItemSessionData{
		ItemID: e.itemID, SessionUUID: "gone-session", SessionRole: instance.SessionRoleWork,
	})
	require.NoError(t, err)
	return e.invoke(t, kind, e.itemID, "k-merge", "merge", true)
}

func (e *wiredEnv) seedCards(t *testing.T) {
	t.Helper()
	for i, md := range []map[string]string{
		{"approval_id": "appr-1"},
		{"item_id": e.itemID, "decision_schema": "backlog_review"},
		{"guidance_request_id": e.gid},
	} {
		n := strconv.Itoa(i + 1)
		require.NoError(t, e.history.Append(&notifications.NotificationRecord{
			ID: "n" + n, SessionID: "s" + n, NotificationType: 1, Title: "t" + n,
			CreatedAt: time.Unix(int64(1_800_000_000+i), 0), Metadata: md,
		}))
	}
}

// cards renders each history row with its read state and offered actions.
func (e *wiredEnv) cards(t *testing.T) string {
	t.Helper()
	resp, err := e.svc.GetNotificationHistory(context.Background(), connect.NewRequest(&sessionv1.GetNotificationHistoryRequest{}))
	require.NoError(t, err)
	var rows []string
	for _, r := range resp.Msg.GetNotifications() {
		row := r.GetId()
		if r.GetIsRead() {
			row += " read"
		}
		var kinds []string
		for _, a := range r.GetActions() {
			kinds = append(kinds, strings.TrimPrefix(a.GetKind().String(), "ACTION_KIND_"))
		}
		rows = append(rows, row+" "+strings.Join(kinds, ","))
	}
	sort.Strings(rows)
	return strings.Join(rows, " | ")
}

func (e *wiredEnv) invoke(t *testing.T, kind sessionv1.ActionKind, key, idem, choice string, confirm bool) string {
	t.Helper()
	resp, err := e.svc.InvokeNotificationAction(context.Background(), connect.NewRequest(&sessionv1.InvokeNotificationActionRequest{
		ActionKind: kind, TargetKey: key, IdempotencyKey: idem, SessionId: "nobody",
		Params: &sessionv1.NotificationActionParams{Answer: "prod", Choice: choice, Confirm: confirm},
	}))
	require.NoError(t, err)
	return strings.Join([]string{
		strings.TrimPrefix(kind.String(), "ACTION_KIND_"), strings.TrimPrefix(resp.Msg.GetOutcome().String(), "ACTION_OUTCOME_"),
		resp.Msg.GetStaleReason(), resp.Msg.GetDetail(),
	}, "/")
}

// The collaborators WireNotificationActions hands the dispatcher must reach their
// action kinds, and the non-question lookups their cards.
func TestWireNotificationActions_ShouldReachEveryWiredCollaborator(t *testing.T) {
	e := newWiredEnv(t)
	assert.Equal(t, "n1 APPROVE,OPEN | n2 OPEN | n3 ANSWER_GUIDANCE,OPEN", e.cards(t))

	const (
		approve  = sessionv1.ActionKind_ACTION_KIND_APPROVE
		guide    = sessionv1.ActionKind_ACTION_KIND_ANSWER_GUIDANCE
		decision = sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION
	)
	got := []string{
		e.invoke(t, approve, "appr-1", "k1", "", false),
		e.invoke(t, approve, "appr-1", "k2", "", false),
		e.invoke(t, guide, e.gid, "k3", "", false),
		e.invoke(t, guide, e.gid, "k4", "", false),
		e.invoke(t, decision, "missing", "k5", "accept", false),
		e.invoke(t, decision, e.itemID, "k6", "merge", false),
		e.invoke(t, decision, e.itemID, "k7", "merge", true),
		e.linkWorkSession(t, decision),
		e.invoke(t, decision, e.itemID, "k8", "accept", false),
		e.invoke(t, sessionv1.ActionKind_ACTION_KIND_CANCEL_WAKE, "w1", "k9", "", false),
		e.invoke(t, sessionv1.ActionKind_ACTION_KIND_CANCEL_PEER_HELP, "p1", "k10", "", false),
		e.invoke(t, sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, "q1", "k11", "", false),
		e.invoke(t, sessionv1.ActionKind_ACTION_KIND_OPEN, "", "k12", "", false),
		e.invoke(t, approve, "", "k13", "", false),
	}

	assert.Equal(t, []string{
		"APPROVE/APPLIED//",
		"APPROVE/ALREADY_RESOLVED//",
		"ANSWER_GUIDANCE/APPLIED//",
		"ANSWER_GUIDANCE/ALREADY_RESOLVED//prod",
		"BACKLOG_DECISION/STALE/item_gone/",
		"BACKLOG_DECISION/INVALID//merge needs confirmation",
		"BACKLOG_DECISION/INVALID//the item has no work session to merge",
		"BACKLOG_DECISION/STALE/item_gone/", // MergePR: no such session
		"BACKLOG_DECISION/APPLIED//",
		"CANCEL_WAKE/APPLIED//",
		"CANCEL_PEER_HELP/APPLIED//",
		"ANSWER_QUESTION/STALE/session_ended/",
		"OPEN/APPLIED//",
		"APPROVE/INVALID//target_key is required",
	}, got)
	audit, err := e.auditFS.ReadFile("/cfg/audit/hidden-session-replies.jsonl")
	require.NoError(t, err, "the dispatcher writes through the service's audit sink")
	assert.Contains(t, string(audit), `"action_kind":"APPROVE"`)
	assert.Equal(t, "n1 read OPEN | n2 read OPEN | n3 OPEN", e.cards(t), "decided cards are retired")
	assert.Equal(t, &wiredFakes{moved: []string{"review>done"}, wakes: []string{"w1"}, peers: []string{"p1"}}, e.fakes)
}
