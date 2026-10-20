package services

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/notifications"
)

// groupedApprovals is twenty pending approvals of one session whose twenty
// notifications collapsed into the single "x20" history record the tray shows.
type groupedApprovals struct {
	history    *notifications.NotificationHistoryStore
	store      *ApprovalStore
	dispatcher *NotificationActionDispatcher
}

const groupedSession = "session-X"

func newGroupedApprovals(t *testing.T, n int) *groupedApprovals {
	t.Helper()
	withIsolatedStateDir(t)
	history, err := notifications.NewNotificationHistoryStore(filepath.Join(t.TempDir(), "notifications.json"))
	require.NoError(t, err)
	store := NewApprovalStore("")
	svc := NewApprovalService(store)
	svc.SetNotificationStore(history)
	base := time.Now()
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("appr-%02d", i)
		a := newTestPendingApproval(id, groupedSession, "Bash")
		a.ToolInput = map[string]interface{}{"command": "make step-" + id}
		require.NoError(t, store.Create(a))
		require.NoError(t, history.Append(&notifications.NotificationRecord{
			ID: id, SessionID: groupedSession, NotificationType: 1, CreatedAt: base.Add(time.Duration(i) * time.Second),
			Title: "Permission Required: Bash", Message: a.ToolInput["command"].(string),
			Metadata: map[string]string{"approval_id": id, "tool_name": "Bash", "tool_input_command": a.ToolInput["command"].(string)},
		}))
	}
	g := &groupedApprovals{history: history, store: store}
	g.dispatcher = NewNotificationActionDispatcher(ActionDelegates{
		Approvals: NewApprovalResolver(svc, store),
		Resolver:  NewNotificationResolver(history),
	}, func() actionAudit { return &recordingAudit{} })
	g.dispatcher.flags, g.dispatcher.metrics = &fakeActionFlags{off: map[string]bool{}}, &recordingMetrics{}
	return g
}

func (g *groupedApprovals) records(t *testing.T) []*notifications.NotificationRecord {
	t.Helper()
	recs, _, err := g.history.List(notifications.ListOptions{Limit: 50})
	require.NoError(t, err)
	return recs
}

func (g *groupedApprovals) approve(t *testing.T, id string) *sessionv1.InvokeNotificationActionResponse {
	t.Helper()
	resp, err := g.dispatcher.Invoke(localCtx(t), ActionCall{Peer: "127.0.0.1:50000", Msg: &sessionv1.InvokeNotificationActionRequest{
		ActionKind: sessionv1.ActionKind_ACTION_KIND_APPROVE, TargetKeyType: sessionv1.ActionTargetKey_ACTION_TARGET_KEY_APPROVAL_ID,
		TargetKey: id, IdempotencyKey: "k-" + id,
	}})
	require.NoError(t, err)
	return resp
}

// Question: when twenty approvals are grouped into one row and the operator
// approves the group, do the other nineteen stay pending? They do: the group
// row addresses one approval, and the other nineteen are neither approved nor
// lost from the tray.
func TestApproveGroupedRow_should_ResolveOneApprovalAndKeepTheOtherNineteenPending(t *testing.T) {
	g := newGroupedApprovals(t, 20)
	recs := g.records(t)
	require.Len(t, recs, 1, "twenty notifications of one session collapse into one row")
	require.Equal(t, 20, recs[0].OccurrenceCount)
	shown := recs[0].Metadata["approval_id"]
	require.Equal(t, "appr-20", shown, "the row addresses the most recent approval only")

	resp := g.approve(t, shown)

	require.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, resp.Outcome)
	pending := g.store.ListAll()
	assert.Len(t, pending, 19, "the other nineteen are still waiting for a decision")
	for _, a := range pending {
		assert.NotEqual(t, shown, a.ID)
	}
}

// The row must not disappear while nineteen approvals still wait: it advances
// to the next pending approval and its count follows the real number waiting.
func TestApproveGroupedRow_should_KeepTheRowInNeedsYouAndAdvanceItToTheNextPendingApproval(t *testing.T) {
	g := newGroupedApprovals(t, 20)
	first := g.records(t)[0].Metadata["approval_id"]

	g.approve(t, first)

	recs := g.records(t)
	require.Len(t, recs, 1)
	rec := recs[0]
	assert.False(t, rec.IsRead, "nineteen approvals still need a decision")
	assert.Equal(t, 19, rec.OccurrenceCount)
	next := rec.Metadata["approval_id"]
	assert.NotEqual(t, first, next)
	_, pending := g.store.Get(next)
	assert.True(t, pending, "the row now addresses an approval that is still pending")
	assert.Equal(t, next, rec.ID, "outcome stamping finds the record by the approval id")
	assert.NotEmpty(t, rec.Metadata["tool_input_command"], "the command to review travels with it")
	assert.Empty(t, rec.Metadata["approval_decision"], "the new head is undecided")
}

func TestApproveGroupedRow_should_RetireTheRowOnlyWhenTheLastApprovalIsDecided(t *testing.T) {
	g := newGroupedApprovals(t, 3)
	for i := 0; i < 3; i++ {
		recs := g.records(t)
		require.Len(t, recs, 1)
		require.Falsef(t, recs[0].IsRead, "round %d: still pending", i)
		g.approve(t, recs[0].Metadata["approval_id"])
	}

	recs := g.records(t)
	require.Len(t, recs, 1)
	assert.True(t, recs[0].IsRead)
	assert.Empty(t, g.store.ListAll())
}
