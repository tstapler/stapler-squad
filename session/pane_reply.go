package session

// pane_reply.go is the Reply write path for a hidden session's AskUserQuestion
// dialog (ADR-010 decisions 2d, 3, 4, 5): a positive structural match of a live
// capture (DialogMatch) and a one-byte, no-Enter, no-retry write guarded by the
// per-instance write lease (SubmitReplyOnce).

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

// QuestionDialog is what DialogMatch compares a live capture against: the
// question text, header and ordered option labels of one single-select question.
type QuestionDialog struct {
	Question string
	Header   string
	Labels   []string
}

const (
	dialogTypeSomethingRow = "Type something."
	dialogChatAboutRow     = "Chat about this"
	dialogFooterPrefix     = "Enter to select"
	dialogHeaderGlyph      = "☐ "
	dialogGutter           = "│"
	dialogSelectionGlyph   = '❯'
)

var (
	ansiEscape  = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-Za-z0-9]`)
	numberedRow = regexp.MustCompile(`^([0-9]+)\. (.*)$`)
)

// normalizeDialogLine strips ANSI, trailing whitespace, a leading 2-cell `│ `
// gutter and the `❯` selection glyph in the first two cells, then collapses
// whitespace runs.
func normalizeDialogLine(line string) string {
	line = ansiEscape.ReplaceAllString(line, "")
	line = strings.TrimRightFunc(line, unicode.IsSpace)
	runes := []rune(line)
	if len(runes) > 0 && string(runes[0]) == dialogGutter {
		runes = runes[1:]
		if len(runes) > 0 && runes[0] == ' ' {
			runes = runes[1:]
		}
	}
	for i := 0; i < len(runes) && i < 2; i++ {
		if runes[i] == dialogSelectionGlyph {
			runes[i] = ' '
		}
	}
	return strings.Join(strings.Fields(string(runes)), " ")
}

func normalizeDialogText(s string) string { return strings.Join(strings.Fields(s), " ") }

// DialogMatch reports whether capture shows exactly q's AskUserQuestion dialog
// and nothing else below it. It is the dialog's identity and the pre-write
// guard; status is never an input. Any layout it does not recognize fails closed.
//
// Bottom anchor: the LAST footer line (`Enter to select ...`) must be the last
// non-blank line of the capture, so a numbered row, a `Do you want` line, a
// composer prompt or any other text after it is no match. Text an agent controls
// (a command, a diff, a transcript) can reproduce the header, question, rows and
// footer, but the real dialog's own rows or the composer are drawn after it.
func DialogMatch(capture string, q QuestionDialog) bool {
	n := len(q.Labels)
	if n < 1 || q.Question == "" || q.Header == "" {
		return false
	}
	raw := strings.Split(capture, "\n")
	lines := make([]string, len(raw))
	for i, l := range raw {
		lines[i] = normalizeDialogLine(l)
	}
	footer := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], dialogFooterPrefix) {
			footer = i
			break
		}
	}
	if footer < 0 {
		return false
	}
	for _, l := range lines[footer+1:] {
		if l != "" {
			return false
		}
	}
	header := -1
	want := dialogHeaderGlyph + normalizeDialogText(q.Header)
	for i := footer - 1; i >= 0; i-- {
		if lines[i] == want {
			header = i
			break
		}
	}
	if header < 0 {
		return false
	}
	return matchDialogRegion(lines[header+1:footer], q)
}

func matchDialogRegion(region []string, q QuestionDialog) bool {
	var questionParts []string
	var rows []numberedLine
	for _, l := range region {
		if m := numberedRow.FindStringSubmatch(l); m != nil {
			idx, err := strconv.Atoi(m[1])
			if err != nil {
				return false
			}
			rows = append(rows, numberedLine{index: idx, text: m[2]})
			continue
		}
		if l == "" || len(rows) > 0 {
			continue // blanks, and unnumbered description or separator lines under a row
		}
		questionParts = append(questionParts, l)
	}
	if strings.Join(questionParts, " ") != normalizeDialogText(q.Question) {
		return false
	}
	n := len(q.Labels)
	if len(rows) != n+2 {
		return false
	}
	for i, r := range rows {
		if r.index != i+1 {
			return false
		}
	}
	for i, label := range q.Labels {
		if rows[i].text != normalizeDialogText(label) {
			return false
		}
	}
	return rows[n].text == dialogTypeSomethingRow && rows[n+1].text == dialogChatAboutRow
}

type numberedLine struct {
	index int
	text  string
}

// ReplyStage is how far SubmitReplyOnce got. Only StageNotStarted means no byte
// was handed to the pane.
type ReplyStage int

const (
	// StageNotStarted: failure strictly before the first byte.
	StageNotStarted ReplyStage = iota
	// StageWriteEntered: the byte was or may have been handed over; the result is indeterminate.
	StageWriteEntered
	// StageDialogClosed: the digit was written and the dialog no longer matches.
	StageDialogClosed
	// StageDialogStillOpen: the digit was written and the dialog was still on screen at the deadline.
	StageDialogStillOpen
)

func (s ReplyStage) String() string {
	switch s {
	case StageNotStarted:
		return "not_started"
	case StageWriteEntered:
		return "write_entered"
	case StageDialogClosed:
		return "dialog_closed"
	case StageDialogStillOpen:
		return "dialog_still_open"
	}
	return "unknown"
}

const (
	// ReplySendTimeout bounds the one-byte write (INFERRED: a healthy write is
	// milliseconds; SendKeysWithTimeout's 5s default is the repo precedent).
	ReplySendTimeout = DefaultSendKeysTimeout
	// ReplyClosedPollEvery and ReplyClosedWindow bound the post-write check
	// (the pane updated within 0.5 s in Spike 1.3g; the rest is margin).
	ReplyClosedPollEvery = 100 * time.Millisecond
	ReplyClosedWindow    = 2 * time.Second
)

var (
	// ErrReplyDigit is returned for a byte that is not an ASCII digit 1-9.
	ErrReplyDigit = errors.New("reply must be one ASCII digit 1-9")
	// ErrReplyTimeout means the caller gave up before the byte was written.
	ErrReplyTimeout = errors.New("reply was not written before its deadline")
	// ErrReplyAborted means the write goroutine saw the abort and wrote nothing.
	ErrReplyAborted = errors.New("reply aborted before the first byte")
)

// replyPane is the narrow surface SubmitReplyOnce needs; *Instance satisfies it.
type replyPane interface {
	leaseOwner
	// SendKeysN returns the byte count so a proven 0, err is not-sent.
	SendKeysN(keys string) (int, error)
	// CapturePaneContent is a live capture of the pane, never a cached status.
	CapturePaneContent() (string, error)
}

// ReplyOptions configures SubmitReplyOnce. Zero values pick the defaults.
type ReplyOptions struct {
	// Dialog is the identity the post-write closed check re-matches.
	Dialog QuestionDialog
	// BeforeFirstByte runs under the lease, immediately before the write; a
	// non-nil error aborts with zero bytes written.
	BeforeFirstByte func() error
	SendTimeout     time.Duration
	PollEvery       time.Duration
	ClosedWindow    time.Duration
	// After is the injected timer (tests never sleep); nil is time.After.
	After func(time.Duration) <-chan time.Time
}

const (
	replyIdle int32 = iota
	replyWriting
	replyAborted
)

type replyResult struct {
	stage ReplyStage
	err   error
}

// SubmitReplyOnce writes exactly one ASCII digit to the pane and then watches
// for the dialog to close. There is no Enter, no content write, no settle wait,
// no pre-Enter compare and no retry: a digit selects immediately, and a stray
// Enter would pick the default of whatever dialog follows (Spike 1.3g (c)).
//
// lease is the caller's held write lease; the writing goroutine releases it
// exactly once, so a caller that timed out leaves it held until the abandoned
// goroutine exits. The abort is one compare-and-swap word (idle -> writing in
// the goroutine, idle -> aborted on the caller's timeout; the loser backs off),
// so after the caller returns at most the one write already inside the syscall
// can still land.
func SubmitReplyOnce(ctx context.Context, pane replyPane, lease *HeldLease, digit byte, opts ReplyOptions) (ReplyStage, error) {
	if err := lease.check(pane); err != nil {
		lease.Release()
		return StageNotStarted, err
	}
	if digit < '1' || digit > '9' {
		lease.Release()
		return StageNotStarted, ErrReplyDigit
	}
	if err := ctx.Err(); err != nil {
		lease.Release()
		return StageNotStarted, fmt.Errorf("reply cancelled before sending: %w", err)
	}
	o := opts.withDefaults()

	run := &replyRun{
		pane: pane, digit: digit, opts: o,
		written: make(chan struct{}, 1), done: make(chan replyResult, 1),
	}
	go func() {
		defer lease.Release()
		res := run.execute(ctx)
		lease.Release() // before the result is visible, so a caller's next Try is not spuriously busy
		run.done <- res
	}()
	return run.await(ctx)
}

// replyRun is the state shared by the caller and the writing goroutine.
type replyRun struct {
	pane    replyPane
	digit   byte
	opts    ReplyOptions
	state   atomic.Int32
	written chan struct{}
	done    chan replyResult
}

// await is the caller side: the write has the send timeout, then the bounded
// closed check runs to completion (the goroutine is the only writer of done).
func (r *replyRun) await(ctx context.Context) (ReplyStage, error) {
	select {
	case <-r.written:
	case res := <-r.done:
		return res.stage, res.err
	case <-r.opts.After(r.opts.SendTimeout):
		return abortOrIndeterminate(&r.state)
	case <-ctx.Done():
		stage, err := abortOrIndeterminate(&r.state)
		if stage == StageNotStarted {
			err = fmt.Errorf("reply cancelled before sending: %w", ctx.Err())
		}
		return stage, err
	}
	res := <-r.done
	return res.stage, res.err
}

func abortOrIndeterminate(state *atomic.Int32) (ReplyStage, error) {
	if state.CompareAndSwap(replyIdle, replyAborted) {
		return StageNotStarted, ErrReplyTimeout
	}
	return StageWriteEntered, fmt.Errorf("reply write did not finish in time: %w", ErrReplyTimeout)
}

func (o ReplyOptions) withDefaults() ReplyOptions {
	if o.SendTimeout <= 0 {
		o.SendTimeout = ReplySendTimeout
	}
	if o.PollEvery <= 0 {
		o.PollEvery = ReplyClosedPollEvery
	}
	if o.ClosedWindow <= 0 {
		o.ClosedWindow = ReplyClosedWindow
	}
	if o.After == nil {
		o.After = time.After
	}
	return o
}

func (r *replyRun) execute(ctx context.Context) replyResult {
	pane, o := r.pane, r.opts
	if v, ok := pane.(paneOwnerVerifier); ok {
		if err := v.VerifyPaneOwner(ctx); err != nil {
			return replyResult{StageNotStarted, fmt.Errorf("pane owner not verified, reply not sent: %w", err)}
		}
	}
	if o.BeforeFirstByte != nil {
		if err := o.BeforeFirstByte(); err != nil {
			return replyResult{StageNotStarted, err}
		}
	}
	if !r.state.CompareAndSwap(replyIdle, replyWriting) {
		return replyResult{StageNotStarted, ErrReplyAborted}
	}
	n, err := pane.SendKeysN(string([]byte{r.digit}))
	r.written <- struct{}{}
	switch {
	case err != nil && n == 0:
		return replyResult{StageNotStarted, fmt.Errorf("reply write failed before any byte: %w", err)}
	case err != nil:
		return replyResult{StageWriteEntered, fmt.Errorf("reply write failed after the byte: %w", err)}
	case n == 0:
		return replyResult{StageWriteEntered, errors.New("reply write reported zero bytes without an error")}
	}
	return awaitDialogClosed(ctx, pane, o)
}

// awaitDialogClosed re-captures every PollEvery for at most ClosedWindow. The
// elapsed time is counted in poll intervals so an injected timer drives it.
func awaitDialogClosed(ctx context.Context, pane replyPane, o ReplyOptions) replyResult {
	var elapsed time.Duration
	for {
		if capture, err := pane.CapturePaneContent(); err == nil && !DialogMatch(capture, o.Dialog) {
			return replyResult{StageDialogClosed, nil}
		}
		if elapsed >= o.ClosedWindow {
			return replyResult{StageDialogStillOpen, nil}
		}
		select {
		case <-o.After(o.PollEvery):
		case <-ctx.Done():
			return replyResult{StageDialogStillOpen, nil}
		}
		elapsed += o.PollEvery
	}
}
