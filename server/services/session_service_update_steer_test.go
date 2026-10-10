package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// steerRecordingPM captures SendKeys traffic and reports one pane change after
// each write so session.SubmitDriverContent's settle/confirm polls succeed.
type steerRecordingPM struct {
	session.ProcessManager
	mu      sync.Mutex
	sent    []string
	pending bool
}

func (r *steerRecordingPM) GetSessionIdentifier() string         { return "" }
func (r *steerRecordingPM) GetPanePID() (int32, error)           { return 0, errors.New("no pane") }
func (r *steerRecordingPM) HasSession() bool                     { return true }
func (r *steerRecordingPM) IsAlive() bool                        { return true }
func (r *steerRecordingPM) HasMeaningfulContent(string) bool     { return false }
func (r *steerRecordingPM) FilterBanners(c string) (string, int) { return c, 0 }
func (r *steerRecordingPM) SendKeys(keys string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, keys)
	r.pending = true
	return len(keys), nil
}

func (r *steerRecordingPM) HasUpdated() (bool, bool, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending {
		r.pending = false
		return true, false, "updated"
	}
	return false, false, ""
}

func (r *steerRecordingPM) writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.sent...)
}

// TestUpdateSessionSteer_ShouldSucceedOnMain_WhenHiddenLinkedReviewSession is the
// Task 5.2d characterization (T-RO-22): UpdateSession accepts a steer to a
// hidden review session linked to a backlog item (on main without any request
// record; since PR 5u through the typed, audited path, which needs the request
// record the real server chain stamps). The read-only guard must keep this
// path working for exactly this shape (O7). It also pins the
// durable "live review link" predicate the typed steer path will implement:
// the session's newest item_session row is a review row with no EndedAt, and
// the instance is live.
//
// The instance is backed by a recording ProcessManager, so the test needs no PTY.
func TestUpdateSessionSteer_ShouldSucceedOnMain_WhenHiddenLinkedReviewSession(t *testing.T) {
	fix := setupForkTestFixture(t)
	ctx := context.Background()
	sink := NewAuditSink(func() (string, error) { return "/cfg", nil }, WithAuditFS(newMemFS()))
	t.Cleanup(sink.Close)
	fix.svc.featureFlagSvc.SetAudit(sink, nil)

	const sessionUUID = "hidden-review-steer-uuid"
	pm := &steerRecordingPM{}
	inst := session.NewStartedInstanceForTest(t, "hidden-review-session", pm)
	inst.UUID = sessionUUID
	inst.Path = t.TempDir()
	inst.Status = session.Active
	inst.Program = "claude"
	inst.Hidden = true
	inst.CreatedAt = time.Now()
	inst.UpdatedAt = time.Now()
	addInstanceToPoller(fix.poller, inst)

	item, err := fix.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "Awaiting review",
		Status: string(session.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = fix.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessionUUID,
		SessionRole: session.SessionRoleReview,
	})
	require.NoError(t, err)

	// Live review link predicate: newest row is an un-ended review row, instance live.
	link, err := fix.storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	require.NoError(t, err)
	assert.Equal(t, session.SessionRoleReview, link.Role)
	assert.Nil(t, link.EndedAt, "the review row must be un-ended")
	assert.True(t, inst.Started(), "the instance must be live")
	assert.True(t, inst.Snapshot().Hidden, "the session is hidden")

	msg := "re-check the acceptance criteria"
	// The hidden-steer path evaluates the rebinding verdict from the stamped
	// request record, as the production chain provides it.
	cfg := verdictServiceConfig("localhost:8543", nil)
	reqCtx := stampedContext(t, &cfg, ListenerLocal, false, "localhost:8543", nil)
	resp, err := fix.svc.UpdateSession(reqCtx, connect.NewRequest(&sessionv1.UpdateSessionRequest{
		Id:           inst.Title,
		SteerMessage: &msg,
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Session)
	assert.Equal(t, []string{msg, session.EnterKeySequence}, pm.writes())
}
