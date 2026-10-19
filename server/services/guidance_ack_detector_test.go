package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

type ackMove struct {
	id    string
	route domain.DeliveryRoute
	state domain.DeliveryState
}

type fakeAckStore struct {
	mu    sync.Mutex
	moves []ackMove
	sent  []*session.GuidanceRequestData
}

func (f *fakeAckStore) SetGuidanceDelivery(_ context.Context, id uuid.UUID, route domain.DeliveryRoute, state domain.DeliveryState, _ int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.moves = append(f.moves, ackMove{id.String(), route, state})
	return true, nil
}

func (f *fakeAckStore) ListGuidanceByDelivery(context.Context, domain.DeliveryRoute, domain.DeliveryState) ([]*session.GuidanceRequestData, error) {
	return f.sent, nil
}

func (f *fakeAckStore) states() []domain.DeliveryState {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.DeliveryState, 0, len(f.moves))
	for _, m := range f.moves {
		out = append(out, m.state)
	}
	return out
}

// ackEnv drives the detector with a manual clock and a scripted pane.
type ackEnv struct {
	clock    *manualClock
	store    *fakeAckStore
	det      *GuidanceAckDetector
	rowID    string
	capture  string
	capErr   error
	leased   []bool
	ended    bool
	dialogUp string
	idle     string
}

func newAckEnv(t *testing.T) *ackEnv {
	t.Helper()
	idle, err := os.ReadFile(filepath.Join("..", "..", "session", "testdata", "dialog_presence", "idle_composer.txt"))
	require.NoError(t, err)
	e := &ackEnv{
		clock: newManualClock(), store: &fakeAckStore{}, rowID: uuid.NewString(), idle: string(idle),
		dialogUp: strings.ReplaceAll(replyDialogFixture(t, "question_single_select_3opts.txt"), "2. Green", "2. "+replyColorLabel),
	}
	e.capture = e.dialogUp
	e.det = NewGuidanceAckDetector(e.store, func(_ context.Context, _ string, underLease bool) (string, error) {
		e.leased = append(e.leased, underLease)
		return e.capture, e.capErr
	}, func(string) bool { return e.ended }, e.clock.Now)
	return e
}

func (e *ackEnv) sent(indeterminate bool) {
	e.det.Sent(SentDelivery{
		RowID: e.rowID, SessionUUID: "s1", Dialog: colorDialog, Route: domain.DeliveryRouteLiveKeystroke, Indeterminate: indeterminate,
	})
}

func (e *ackEnv) tick(d time.Duration) {
	e.clock.Advance(d)
	e.det.Tick(context.Background())
}

func TestAckDetector_ShouldAcknowledge_WhenPostToolUseReceiptArrivesForAskUserQuestion(t *testing.T) {
	e := newAckEnv(t)
	e.sent(false)

	e.det.Receipt(context.Background(), "s1", "Bash")
	assert.Empty(t, e.store.states(), "another tool's receipt is not an acknowledgement")

	e.det.Receipt(context.Background(), "s2", askUserQuestion)
	assert.Empty(t, e.store.states(), "another session's receipt is not either")

	e.det.Receipt(context.Background(), "s1", askUserQuestion)
	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateAcknowledged}, e.store.states())
	assert.Equal(t, domain.DeliveryRouteLiveKeystroke, e.store.moves[0].route)
	assert.False(t, e.det.Unconfirmed(e.rowID), "no longer tracked")
}

func TestAckDetector_ShouldAcknowledgeOnTwoConsecutiveCleanCaptures_AndResetOnADialog(t *testing.T) {
	e := newAckEnv(t)
	e.sent(false)

	e.capture = e.idle
	e.tick(3 * time.Second)
	assert.Empty(t, e.store.states(), "one clean capture is not enough")

	e.capture = e.dialogUp
	e.tick(3 * time.Second)
	e.capture = e.idle
	e.tick(3 * time.Second)
	assert.Empty(t, e.store.states(), "the dialog coming back reset the streak")

	e.tick(3 * time.Second)
	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateAcknowledged}, e.store.states())
	assert.Equal(t, []bool{false, false, false, false}, e.leased, "sampling does not take the lease")
}

func TestAckDetector_ShouldNeverCountAnIndeterminateCaptureAsGone(t *testing.T) {
	e := newAckEnv(t)
	e.sent(false)

	e.capture = "build output\nmore output\n"
	e.tick(3 * time.Second)
	e.tick(3 * time.Second)
	e.tick(3 * time.Second)
	e.capErr, e.capture = errors.New("tmux gone"), ""
	e.tick(3 * time.Second)

	assert.Empty(t, e.store.states())
}

func TestAckDetector_ShouldFailWithStillShowing_WhenTheRowsDialogIsUpAt60Seconds(t *testing.T) {
	e := newAckEnv(t)
	e.sent(false)

	e.tick(61 * time.Second)

	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateFailed}, e.store.states())
	assert.Equal(t, []bool{true}, e.leased, "the decision capture is taken under the write lease")
}

func TestAckDetector_ShouldGiveTheSameFailure_WhenTheWriteWasIndeterminate(t *testing.T) {
	e := newAckEnv(t)
	e.sent(true)
	require.True(t, e.det.Unconfirmed(e.rowID), "an indeterminate write is unconfirmed from the start")

	e.tick(61 * time.Second)

	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateFailed}, e.store.states())
}

func TestAckDetector_ShouldAcknowledge_WhenNoDialogIsUpAt60Seconds(t *testing.T) {
	e := newAckEnv(t)
	e.sent(false)
	e.capture = e.idle

	e.tick(61 * time.Second)

	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateAcknowledged}, e.store.states())
}

func TestAckDetector_ShouldStayUnconfirmedThenClear_WhenCaptureFailsAt60SecondsAndAReceiptComesLate(t *testing.T) {
	e := newAckEnv(t)
	e.sent(false)
	e.capErr = errors.New("tmux gone")

	e.tick(61 * time.Second)

	assert.Empty(t, e.store.states(), "the row stays sent")
	assert.True(t, e.det.Unconfirmed(e.rowID), "and carries the Unconfirmed flag")

	e.det.Receipt(context.Background(), "s1", askUserQuestion)

	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateAcknowledged}, e.store.states())
	assert.False(t, e.det.Unconfirmed(e.rowID))
}

func TestAckDetector_ShouldStopTrackingWithoutChangingState_WhenTheSessionEnds(t *testing.T) {
	e := newAckEnv(t)
	e.sent(false)
	e.ended = true

	e.tick(61 * time.Second)

	assert.Empty(t, e.store.states(), "a sent row stays sent; the card reads that the session ended")
	assert.Empty(t, e.leased, "no capture of a dead session")
	e.ended = false
	e.tick(3 * time.Second)
	assert.Empty(t, e.leased, "and it is not tracked again")
}

func TestAckDetector_ShouldResumeTrackingSentRows_WhenRestored(t *testing.T) {
	e := newAckEnv(t)
	e.store.sent = []*session.GuidanceRequestData{{
		ID: e.rowID, AskerSessionUUID: "s1", DeliveryRoute: domain.DeliveryRouteLiveKeystroke, DeliveryState: domain.DeliveryStateSent,
	}}

	e.det.Restore(context.Background())
	e.det.Receipt(context.Background(), "s1", askUserQuestion)

	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateAcknowledged}, e.store.states())
}

// ackCards records where the question's notification cards were moved.
type ackCards struct{ resolved, reopened []string }

func (c *ackCards) ResolveQuestionNotifications(id string) { c.resolved = append(c.resolved, id) }
func (c *ackCards) ReopenQuestionNotifications(id string)  { c.reopened = append(c.reopened, id) }

func TestAckDetector_ShouldKeepTheCardInNeedsYou_WhileUnconfirmedAndRetireItOnALateAcknowledgement(t *testing.T) {
	e := newAckEnv(t)
	cards := &ackCards{}
	e.det.SetNotes(cards)
	e.sent(false)
	e.capErr = errors.New("tmux gone")

	e.tick(61 * time.Second)

	assert.Equal(t, []string{e.rowID}, cards.reopened, "Unconfirmed puts the card back in Needs you")
	assert.Empty(t, cards.resolved)

	e.det.Receipt(context.Background(), "s1", askUserQuestion)

	assert.Equal(t, []string{e.rowID}, cards.resolved, "the late acknowledgement retires it")
}

func TestAckDetector_ShouldReopenTheCard_WhenTheDialogIsStillShowingAt60Seconds(t *testing.T) {
	e := newAckEnv(t)
	cards := &ackCards{}
	e.det.SetNotes(cards)
	e.sent(false)

	e.tick(61 * time.Second)

	assert.Equal(t, []string{e.rowID}, cards.reopened, "row 16 stays in Needs you and in the badge")
}

func TestAckDetector_ShouldRetireTheCard_WhenTheSessionEndsOnASentAnswer(t *testing.T) {
	e := newAckEnv(t)
	cards := &ackCards{}
	e.det.SetNotes(cards)
	e.sent(false)
	e.ended = true

	e.tick(61 * time.Second)

	assert.Equal(t, []string{e.rowID}, cards.resolved, "row 17 moves to Recent activity and leaves the badge")
	assert.Empty(t, cards.reopened)
}
