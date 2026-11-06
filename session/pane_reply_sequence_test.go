package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seqAfter fires every poll timer at once and never the overall budget, so
// the driver runs without sleeping.
func seqAfter(d time.Duration) <-chan time.Time {
	if d != ReplyClosedPollEvery {
		return nil
	}
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func seqOpts() SequenceOptions {
	return SequenceOptions{StillOpen: TabbedStillOpen, After: seqAfter}
}

func recordedSteps(t *testing.T) []ReplyStep {
	t.Helper()
	steps, err := PlanTabbedAnswer(twoQuestions, [][]string{{"Redis"}, {"A", "B"}})
	require.NoError(t, err)
	return steps
}

func newSim() *tabbedSim { return &tabbedSim{owner: replyOwner} }

func TestSubmitReplySequence_ShouldWriteTheRecordedKeysAndReportClosed_WhenTheDialogFollowsTheFixture(t *testing.T) {
	sim := newSim()
	inst, lease := replyInstanceAndLease(t)
	res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), seqOpts())
	require.NoError(t, res.Err)
	assert.Equal(t, StageDialogClosed, res.Stage)
	assert.Equal(t, 9, res.Written)
	assert.False(t, res.Partial())
	assert.Equal(t, recordedKeys(t), sim.written())
	assert.Equal(t, [3]bool{true, true, false}, sim.checked, "the simulator ended with A and B toggled")
	l, ok := inst.TryTerminalWriteLease(LeaseWriterReply)
	require.True(t, ok, "the lease is released after the sequence")
	l.Release()
}

func TestSubmitReplySequence_ShouldAbortStaleWithNoFurtherWrites_WhenTheFrameChangesAfterStep1(t *testing.T) {
	sim := newSim()
	sim.override = map[int]string{1: "\n" + tabRule + "\n❯ \n" + tabRule + "\n  ? for shortcuts\n"}
	_, lease := replyInstanceAndLease(t)
	res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), seqOpts())
	assert.True(t, res.Stale)
	assert.ErrorIs(t, res.Err, ErrSequenceStale)
	assert.Equal(t, StageWriteEntered, res.Stage, "one key was typed")
	assert.Equal(t, 1, res.Written)
	assert.True(t, res.Partial())
	assert.Len(t, sim.written(), 1, "no write after the failed check")
}

func TestSubmitReplySequence_ShouldWriteNothing_WhenTheFirstFrameIsNotTheExpectedDialog(t *testing.T) {
	sim := newSim()
	sim.override = map[int]string{0: renderQ2(true, [3]bool{}, 0)} // Q2 is up although the plan starts at Q1
	_, lease := replyInstanceAndLease(t)
	res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), seqOpts())
	assert.True(t, res.Stale)
	assert.Equal(t, StageNotStarted, res.Stage)
	assert.Zero(t, res.Written)
	assert.False(t, res.Partial())
	assert.Empty(t, sim.written())
}

func TestSubmitReplySequence_ShouldReportNotSentVsSendIndeterminatePerStage_When_FaultInjectedAtEachStep(t *testing.T) {
	total := len(recordedKeys(t))
	for k := 1; k <= total; k++ {
		t.Run("key "+string(rune('0'+k)), func(t *testing.T) {
			t.Run("provable zero-byte failure", func(t *testing.T) {
				sim := newSim()
				sim.failAt, sim.failErr = k, errors.New("pty closed")
				_, lease := replyInstanceAndLease(t)
				res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), seqOpts())
				require.Error(t, res.Err)
				assert.Equal(t, k-1, res.Written)
				assert.Len(t, sim.written(), k-1, "nothing written after the failure")
				if k == 1 {
					assert.Equal(t, StageNotStarted, res.Stage, "NOT_SENT: no byte of the answer was handed over")
					assert.False(t, res.Partial())
					return
				}
				assert.Equal(t, StageWriteEntered, res.Stage, "earlier keys landed: never NOT_SENT")
				assert.True(t, res.Partial())
			})
			t.Run("partial write", func(t *testing.T) {
				sim := newSim()
				sim.failAt, sim.failN, sim.failErr = k, 1, errors.New("short write")
				_, lease := replyInstanceAndLease(t)
				res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), seqOpts())
				require.Error(t, res.Err)
				assert.Equal(t, StageWriteEntered, res.Stage, "SEND_INDETERMINATE: the key may have landed")
				assert.Len(t, sim.written(), k)
			})
		})
	}
}

func TestSubmitReplySequence_ShouldReportStillOpen_When_TheDialogStaysAfterTheLastKey(t *testing.T) {
	sim := newSim()
	sim.override = map[int]string{9: renderReview("Redis", "A, B")}
	_, lease := replyInstanceAndLease(t)
	res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), seqOpts())
	require.NoError(t, res.Err)
	assert.Equal(t, StageDialogStillOpen, res.Stage)
	assert.Equal(t, 9, res.Written)
	assert.False(t, res.Partial())
}

func TestSubmitReplySequence_ShouldNotWrite_When_BeforeFirstByteRefuses(t *testing.T) {
	sim := newSim()
	_, lease := replyInstanceAndLease(t)
	refusal := errors.New("not waiting")
	opts := seqOpts()
	opts.BeforeFirstByte = func() error { return refusal }
	res := SubmitReplySequence(context.Background(), sim, lease, recordedSteps(t), opts)
	assert.ErrorIs(t, res.Err, refusal)
	assert.Equal(t, StageNotStarted, res.Stage)
	assert.Empty(t, sim.written())
}

func TestSubmitReplySequence_ShouldRefuseEmptyStepsAndForeignLease(t *testing.T) {
	sim := newSim()
	inst, lease := replyInstanceAndLease(t)
	res := SubmitReplySequence(context.Background(), sim, lease, nil, seqOpts())
	assert.ErrorIs(t, res.Err, ErrSequenceEmpty)
	res = SubmitReplySequence(context.Background(), sim, nil, recordedSteps(t), seqOpts())
	assert.ErrorIs(t, res.Err, ErrNoLease)
	other := &tabbedSim{owner: "someone-else"}
	l, ok := inst.TryTerminalWriteLease(LeaseWriterReply)
	require.True(t, ok)
	res = SubmitReplySequence(context.Background(), other, l, recordedSteps(t), seqOpts())
	assert.ErrorIs(t, res.Err, ErrLeaseMismatch)
	assert.Empty(t, sim.written())
	assert.Empty(t, other.written())
}

// gatedSim blocks the nth write so a test can cancel while a key is in the syscall.
type gatedSim struct {
	*tabbedSim
	at      int
	entered chan struct{}
	gate    chan struct{}
}

func (g *gatedSim) SendKeysN(keys string) (int, error) {
	if len(g.written())+1 == g.at {
		g.entered <- struct{}{}
		<-g.gate
	}
	return g.tabbedSim.SendKeysN(keys)
}

func TestSubmitReplySequence_ShouldReportIndeterminateAndStopWriting_When_CancelledDuringAWrite(t *testing.T) {
	g := &gatedSim{tabbedSim: newSim(), at: 2, entered: make(chan struct{}, 1), gate: make(chan struct{})}
	inst, lease := replyInstanceAndLease(t)
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan SequenceResult, 1)
	go func() { got <- SubmitReplySequence(ctx, g, lease, recordedSteps(t), seqOpts()) }()
	<-g.entered
	cancel()
	res := <-got
	assert.Equal(t, StageWriteEntered, res.Stage)
	assert.Equal(t, 1, res.Written)
	require.Error(t, res.Err)
	close(g.gate) // the in-flight key lands, then the goroutine sees the abort
	require.Eventually(t, func() bool {
		l, ok := inst.TryTerminalWriteLease(LeaseWriterReply)
		if ok {
			l.Release()
		}
		return ok
	}, 2*time.Second, 5*time.Millisecond, "the abandoned goroutine releases the lease")
	assert.Len(t, g.written(), 2, "the in-flight key landed and nothing after it")
}
