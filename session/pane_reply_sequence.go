package session

// pane_reply_sequence.go writes a short fixed key sequence into a dialog that
// needs several keys (the tabbed multi-question AskUserQuestion, Story 3.5).
// One write lease covers the whole sequence; before every key the live capture
// must match that step's expected frame, so any drift aborts with no further
// writes. There is no retry: a key that may have landed makes the result
// indeterminate, and a partly typed answer is reported as such.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ReplyStep is one key write and the frame the pane must show right before it.
type ReplyStep struct {
	Keys   string
	Expect func(capture string) bool
}

// Keys the tabbed dialog takes (tmux key names Enter and Down, written as the
// bytes a terminal sends).
const (
	KeyEnter = "\r"
	KeyDown  = "\x1b[B"
)

// SequenceWindow bounds how long a step waits for the pane to show its frame
// after the previous key (the first step never waits).
const SequenceWindow = 2 * time.Second

var (
	// ErrSequenceStale wraps a failure of a step's frame check; no key of that step was written.
	ErrSequenceStale = errors.New("pane no longer shows the expected dialog frame")
	// ErrSequenceEmpty is returned for a sequence with no steps or a step without a frame check.
	ErrSequenceEmpty = errors.New("reply sequence needs steps that each check a frame")
)

// SequenceOptions configures SubmitReplySequence. Zero values pick the defaults.
type SequenceOptions struct {
	// StillOpen reports whether capture still shows the dialog; it decides
	// StageDialogClosed vs StageDialogStillOpen after the last key.
	StillOpen       func(capture string) bool
	BeforeFirstByte func() error
	SendTimeout     time.Duration
	PollEvery       time.Duration
	StepWindow      time.Duration
	ClosedWindow    time.Duration
	After           func(time.Duration) <-chan time.Time
}

// SequenceResult says how far a sequence got.
type SequenceResult struct {
	// Stage is the final stage: StageNotStarted (no key written), StageWriteEntered
	// (a key may have landed), StageDialogClosed or StageDialogStillOpen.
	Stage ReplyStage
	// Written is how many steps' writes completed.
	Written int
	Total   int
	// Stale is true when a step's frame check failed; Err wraps ErrSequenceStale.
	Stale bool
	Err   error
}

// Partial reports an answer that was typed in part and not finished.
func (r SequenceResult) Partial() bool {
	return r.Written > 0 && r.Written < r.Total
}

func (o SequenceOptions) withDefaults() SequenceOptions {
	ro := ReplyOptions{SendTimeout: o.SendTimeout, PollEvery: o.PollEvery, ClosedWindow: o.ClosedWindow, After: o.After}.withDefaults()
	o.SendTimeout, o.PollEvery, o.ClosedWindow, o.After = ro.SendTimeout, ro.PollEvery, ro.ClosedWindow, ro.After
	if o.StepWindow <= 0 {
		o.StepWindow = SequenceWindow
	}
	return o
}

// SubmitReplySequence runs steps under lease and releases it exactly once. See
// the file comment for the guarantees; SubmitReplyOnce's single-byte contract is unchanged.
func SubmitReplySequence(ctx context.Context, pane replyPane, lease *HeldLease, steps []ReplyStep, opts SequenceOptions) SequenceResult {
	total := len(steps)
	fail := func(err error) SequenceResult {
		lease.Release()
		return SequenceResult{Stage: StageNotStarted, Total: total, Err: err}
	}
	if err := lease.check(pane); err != nil {
		return fail(err)
	}
	if total == 0 {
		return fail(ErrSequenceEmpty)
	}
	for _, s := range steps {
		if s.Expect == nil || s.Keys == "" {
			return fail(ErrSequenceEmpty)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(fmt.Errorf("reply cancelled before sending: %w", err))
	}
	run := &sequenceRun{pane: pane, steps: steps, opts: opts.withDefaults(), done: make(chan SequenceResult, 1)}
	go func() {
		defer lease.Release()
		res := run.execute(ctx)
		lease.Release() // before the result is visible, so a caller's next Try is not spuriously busy
		run.done <- res
	}()
	return run.await(ctx)
}

// sequenceRun is shared by the caller and the writing goroutine. mu orders the
// caller's abort against each write: after abort() no further write starts.
type sequenceRun struct {
	pane  replyPane
	steps []ReplyStep
	opts  SequenceOptions
	done  chan SequenceResult

	mu       sync.Mutex
	aborted  bool
	inflight bool
	written  int
}

func (r *sequenceRun) await(ctx context.Context) SequenceResult {
	// The whole sequence is bounded by one send timeout per step plus the waits.
	budget := time.Duration(len(r.steps)) * (r.opts.SendTimeout + r.opts.StepWindow)
	budget += r.opts.ClosedWindow
	select {
	case res := <-r.done:
		return res
	case <-r.opts.After(budget):
		return r.abort(fmt.Errorf("reply sequence did not finish in time: %w", ErrReplyTimeout))
	case <-ctx.Done():
		return r.abort(fmt.Errorf("reply cancelled: %w", ctx.Err()))
	}
}

// abort stops further writes and reports what is known: a write inside the
// syscall makes the result indeterminate.
func (r *sequenceRun) abort(err error) SequenceResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aborted = true
	stage := StageNotStarted
	if r.inflight || r.written > 0 {
		stage = StageWriteEntered
	}
	return SequenceResult{Stage: stage, Written: r.written, Total: len(r.steps), Err: err}
}

func (r *sequenceRun) execute(ctx context.Context) SequenceResult {
	res := func(stage ReplyStage, stale bool, err error) SequenceResult {
		r.mu.Lock()
		defer r.mu.Unlock()
		return SequenceResult{Stage: stage, Written: r.written, Total: len(r.steps), Stale: stale, Err: err}
	}
	for i, step := range r.steps {
		if err := r.awaitFrame(ctx, i, step); err != nil {
			return res(r.stageBefore(), errors.Is(err, ErrSequenceStale), err)
		}
		if v, ok := r.pane.(paneOwnerVerifier); ok {
			if err := v.VerifyPaneOwner(ctx); err != nil {
				return res(r.stageBefore(), false, fmt.Errorf("pane owner not verified, step %d not sent: %w", i+1, err))
			}
		}
		if i == 0 && r.opts.BeforeFirstByte != nil {
			if err := r.opts.BeforeFirstByte(); err != nil {
				return res(StageNotStarted, false, err)
			}
		}
		if stage, err := r.write(step.Keys); err != nil {
			return res(stage, false, fmt.Errorf("step %d of %d: %w", i+1, len(r.steps), err))
		}
	}
	return r.finish(ctx, res)
}

// stageBefore is the stage when a failure happens between writes.
func (r *sequenceRun) stageBefore() ReplyStage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.written > 0 {
		return StageWriteEntered
	}
	return StageNotStarted
}

// awaitFrame requires the step's frame: at once for the first step, within
// StepWindow for later ones (the pane redraws after the previous key).
func (r *sequenceRun) awaitFrame(ctx context.Context, i int, step ReplyStep) error {
	window := time.Duration(0)
	if i > 0 {
		window = r.opts.StepWindow
	}
	var elapsed time.Duration
	for {
		capture, err := r.pane.CapturePaneContent()
		if err == nil && step.Expect(capture) {
			return nil
		}
		if elapsed >= window {
			if err != nil {
				return fmt.Errorf("step %d: capture failed: %w", i+1, err)
			}
			return fmt.Errorf("step %d: %w", i+1, ErrSequenceStale)
		}
		select {
		case <-r.opts.After(r.opts.PollEvery):
		case <-ctx.Done():
			return fmt.Errorf("step %d: %w", i+1, ctx.Err())
		}
		elapsed += r.opts.PollEvery
	}
}

// write is one key write. It returns StageNotStarted only when nothing reached
// the pane and no earlier key did either.
func (r *sequenceRun) write(keys string) (ReplyStage, error) {
	r.mu.Lock()
	if r.aborted {
		stage := StageNotStarted
		if r.written > 0 {
			stage = StageWriteEntered
		}
		r.mu.Unlock()
		return stage, ErrReplyAborted
	}
	r.inflight = true
	r.mu.Unlock()

	n, err := r.pane.SendKeysN(keys)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.inflight = false
	switch {
	case err != nil && n == 0:
		if r.written > 0 {
			return StageWriteEntered, fmt.Errorf("write failed before any byte of this key: %w", err)
		}
		return StageNotStarted, fmt.Errorf("write failed before any byte: %w", err)
	case err != nil:
		return StageWriteEntered, fmt.Errorf("write failed after the key: %w", err)
	case n == 0:
		return StageWriteEntered, errors.New("write reported zero bytes without an error")
	}
	r.written++
	return StageWriteEntered, nil
}

// finish polls for the dialog to close after the last key.
func (r *sequenceRun) finish(ctx context.Context, res func(ReplyStage, bool, error) SequenceResult) SequenceResult {
	var elapsed time.Duration
	for {
		if capture, err := r.pane.CapturePaneContent(); err == nil && (r.opts.StillOpen == nil || !r.opts.StillOpen(capture)) {
			return res(StageDialogClosed, false, nil)
		}
		if elapsed >= r.opts.ClosedWindow {
			return res(StageDialogStillOpen, false, nil)
		}
		select {
		case <-r.opts.After(r.opts.PollEvery):
		case <-ctx.Done():
			return res(StageDialogStillOpen, false, nil)
		}
		elapsed += r.opts.PollEvery
	}
}
