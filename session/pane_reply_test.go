package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixtures under testdata/reply_dialogs are verbatim `tmux capture-pane -p`
// output of real Claude Code v2.1.296 dialogs at 80 columns (trailing blank
// lines trimmed), taken for T-RP-45.
func loadDialogFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "reply_dialogs", name))
	require.NoError(t, err)
	return string(b)
}

var (
	colorQuestion = QuestionDialog{
		Question: "Which color should the spike use?",
		Header:   "Color",
		Labels:   []string{"Red", "Green", "Blue"},
	}
	migrationQuestion = QuestionDialog{
		Question: "Which of the following database migration strategies should the team adopt for the large production cluster, given the downtime budget and the rollback requirements discussed earlier?",
		Header:   "Migration",
		Labels:   []string{"Blue green cutover", "Rolling in-place upgrade"},
	}
)

func TestDialogMatch_ShouldMatchTheCapturedFixtures_WhenTheRealDialogIsOnScreen(t *testing.T) {
	assert.True(t, DialogMatch(loadDialogFixture(t, "question_single_select_3opts.txt"), colorQuestion))
	// The 80-column wrap puts a 2-cell "│ " gutter on every question line.
	assert.True(t, DialogMatch(loadDialogFixture(t, "question_wrapped_2opts.txt"), migrationQuestion))
}

func TestDialogMatch_ShouldMatchRegardlessOfAnsiGlyphAndTrailingBlankLines_WhenCaptureCarriesEscapes(t *testing.T) {
	c := loadDialogFixture(t, "question_single_select_3opts.txt")
	c = strings.ReplaceAll(c, "❯ 1. Red", "\x1b[38;5;39m❯\x1b[0m 1. Red")
	c = strings.ReplaceAll(c, "Which color", "\x1b[1mWhich color\x1b[22m")
	assert.True(t, DialogMatch(c+"\n\n\n   \n", colorQuestion))
	// Moving the selection glyph to row 2 is the same dialog.
	moved := strings.Replace(strings.Replace(c, "❯ 1. Red", "  1. Red", 1), "  2. Green", "❯ 2. Green", 1)
	assert.True(t, DialogMatch(moved, colorQuestion))
}

func TestDialogMatch_ShouldRejectEveryRealPermissionDialog_WhenTheirNumberedRowsAreOnScreen(t *testing.T) {
	for _, f := range []string{
		"permission_bash.txt", "permission_edit_diff.txt", "permission_webfetch.txt",
		"permission_mcp.txt", "permission_read_outside_cwd.txt",
	} {
		t.Run(f, func(t *testing.T) {
			c := loadDialogFixture(t, f)
			assert.False(t, DialogMatch(c, colorQuestion))
			assert.False(t, DialogMatch(c, migrationQuestion))
		})
	}
}

func TestDialogMatch_ShouldSeparateTwoQuestionsWithTheSameOptionLayout(t *testing.T) {
	c := loadDialogFixture(t, "question_single_select_3opts.txt")
	other := colorQuestion
	other.Question = "Which shade should the spike use?"
	assert.False(t, DialogMatch(c, other))
	assert.False(t, DialogMatch(c, migrationQuestion))
}

func TestDialogMatch_ShouldFailClosed_WhenTheLayoutIsNotTheMeasuredOne(t *testing.T) {
	base := loadDialogFixture(t, "question_single_select_3opts.txt")
	cases := map[string]struct {
		capture string
		q       QuestionDialog
	}{
		"missing header row":        {strings.Replace(base, "☐ Color", "☐ Other", 1), colorQuestion},
		"missing footer":            {strings.Replace(base, "Enter to select", "Press to pick", 1), colorQuestion},
		"no Type something":         {strings.Replace(base, "4. Type something.", "4. Something else", 1), colorQuestion},
		"N+2 is not Chat about":     {strings.Replace(base, "5. Chat about this", "5. Chat later", 1), colorQuestion},
		"label mismatch":            {strings.Replace(base, "2. Green", "2. Greene", 1), colorQuestion},
		"duplicate numbered index":  {strings.Replace(base, "  3. Blue", "  2. Blue", 1), colorQuestion},
		"extra numbered row":        {strings.Replace(base, "  4. Type something.", "  3b. x\n  3. Teal\n  4. Type something.", 1), colorQuestion},
		"wrapped long label":        {strings.Replace(base, "1. Red", "1. Red\n  hot", 1), QuestionDialog{Question: colorQuestion.Question, Header: "Color", Labels: []string{"Red hot", "Green", "Blue"}}},
		"empty header":              {base, QuestionDialog{Question: colorQuestion.Question, Labels: colorQuestion.Labels}},
		"no labels":                 {base, QuestionDialog{Question: colorQuestion.Question, Header: "Color"}},
		"shell prompt":              {"user@host:~$ \n", colorQuestion},
		"empty capture":             {"", colorQuestion},
		"question text differs":     {strings.Replace(base, "Which color should", "Which colour should", 1), colorQuestion},
		"extra label in the entry":  {base, QuestionDialog{Question: colorQuestion.Question, Header: "Color", Labels: []string{"Red", "Green", "Blue", "Teal"}}},
		"one label fewer in entry":  {base, QuestionDialog{Question: colorQuestion.Question, Header: "Color", Labels: []string{"Red", "Green"}}},
		"row text only a prefix":    {base, QuestionDialog{Question: colorQuestion.Question, Header: "Color", Labels: []string{"Red", "Green", "Blue and more"}}},
		"header row is a substring": {strings.Replace(base, "☐ Color", "☐ Color scheme", 1), colorQuestion},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.False(t, DialogMatch(tc.capture, tc.q))
		})
	}
}

// forgedBlock is what an agent-controlled command or diff can print: the
// header, the question, rows 1..N+2 and the footer of a real question dialog.
func forgedBlock() string {
	return strings.Join([]string{
		" ☐ Color", "", "Which color should the spike use?", "",
		"❯ 1. Red", "  2. Green", "  3. Blue", "  4. Type something.", "  5. Chat about this", "",
		"Enter to select · ↑/↓ to navigate · Esc to cancel",
	}, "\n")
}

func TestDialogMatch_ShouldRejectAForgedBlockInsideAPermissionDialog_WhenTheRealRowsFollow(t *testing.T) {
	// Once in a Bash command box, once in an edit diff: the dialog's own
	// numbered rows and `Do you want` line are drawn after the forged footer.
	bash := "────\n Bash command\n Run something\n╌╌╌╌\n" + forgedBlock() + "\n╌╌╌╌\n This command requires approval\n\n Do you want to proceed?\n ❯ 1. Yes\n   2. Yes, and don't ask again\n   3. No\n\n Esc to cancel · Tab to amend"
	edit := "────\n Edit file\n x.txt\n╌╌╌╌\n 1  " + strings.ReplaceAll(forgedBlock(), "\n", "\n 2 +") + "\n╌╌╌╌\n Do you want to make this edit to x.txt?\n ❯ 1. Yes\n   2. Yes, and switch to accept edits\n   3. No\n\n Esc to cancel · Tab to amend"
	assert.False(t, DialogMatch(bash, colorQuestion))
	assert.False(t, DialogMatch(edit, colorQuestion))
	// With no permission dialog open the forged block in the transcript is
	// followed by the composer, which is not blank, so it is no match either.
	transcript := forgedBlock() + "\n\n✻ Cooked for 2s\n\n────────\n❯ \n────────\n  ⏸ manual mode on"
	assert.False(t, DialogMatch(transcript, colorQuestion))
	// The footer being the last non-blank line is what makes the same block match.
	assert.True(t, DialogMatch(forgedBlock()+"\n\n\n", colorQuestion))
}

func TestDialogMatch_ShouldRejectAStatusLineAfterTheFooter_WhenTheFooterIsNotTheLastNonBlankLine(t *testing.T) {
	// Recorded concern (Reply re-review round 2, C1/C2): fail closed.
	c := loadDialogFixture(t, "question_single_select_3opts.txt")
	assert.False(t, DialogMatch(c+"\n  tmux focus-events off\n", colorQuestion))
}

// ---- SubmitReplyOnce ----

type fakeReplyPane struct {
	mu       sync.Mutex
	writes   []string
	captures []string // consumed in order; the last repeats
	capIdx   int
	capErr   error
	n        int
	err      error
	block    chan struct{} // when set, SendKeysN blocks until closed
	entered  chan struct{} // signalled when SendKeysN is entered
	owner    string
}

func (f *fakeReplyPane) LeaseOwnerUUID() string { return f.owner }

func (f *fakeReplyPane) SendKeysN(keys string) (int, error) {
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, keys)
	if f.err != nil {
		return f.n, f.err
	}
	return len(keys), nil
}

func (f *fakeReplyPane) CapturePaneContent() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.capErr != nil {
		return "", f.capErr
	}
	i := f.capIdx
	if i >= len(f.captures) {
		i = len(f.captures) - 1
	}
	f.capIdx++
	return f.captures[i], nil
}

func (f *fakeReplyPane) writtenBytes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.writes...)
}

const replyOwner = "reply-pane-owner"

func replyInstanceAndLease(t *testing.T) (*Instance, *HeldLease) {
	t.Helper()
	inst := &Instance{UUID: replyOwner}
	l, ok := inst.TryTerminalWriteLease(LeaseWriterReply)
	require.True(t, ok)
	return inst, l
}

// readyAfter fires every poll timer at once and never fires the send timeout,
// so a test drives the closed check without sleeping.
func readyAfter(d time.Duration) <-chan time.Time {
	if d == ReplySendTimeout {
		return nil
	}
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func TestSubmitReplyOnce_ShouldWriteExactlyOneDigitAndNoEnterAndReportClosed_WhenTheDialogCloses(t *testing.T) {
	dialog := loadDialogFixture(t, "question_single_select_3opts.txt")
	for _, digit := range []byte{'1', '2', '3'} {
		inst, lease := replyInstanceAndLease(t)
		pane := &fakeReplyPane{owner: replyOwner, captures: []string{dialog, dialog, "closed\n❯ \n"}}
		stage, err := SubmitReplyOnce(context.Background(), pane, lease, digit, ReplyOptions{Dialog: colorQuestion, After: readyAfter})
		require.NoError(t, err)
		assert.Equal(t, StageDialogClosed, stage)
		assert.Equal(t, []string{string([]byte{digit})}, pane.writtenBytes(), "one digit, no \\r, no \\n, no second byte")
		AssertLeaseFree(t, inst)
	}
}

func TestSubmitReplyOnce_ShouldReportDialogStillOpenWithNoSecondByte_WhenPaneNeverChangesPastTwoSeconds(t *testing.T) {
	dialog := loadDialogFixture(t, "question_single_select_3opts.txt")
	inst, lease := replyInstanceAndLease(t)
	pane := &fakeReplyPane{owner: replyOwner, captures: []string{dialog}}
	polls := 0
	stage, err := SubmitReplyOnce(context.Background(), pane, lease, '2', ReplyOptions{Dialog: colorQuestion, After: func(d time.Duration) <-chan time.Time {
		if d == ReplyClosedPollEvery {
			polls++
		}
		return readyAfter(d)
	}})
	require.NoError(t, err)
	assert.Equal(t, StageDialogStillOpen, stage)
	assert.Equal(t, []string{"2"}, pane.writtenBytes())
	assert.Equal(t, int(ReplyClosedWindow/ReplyClosedPollEvery), polls, "2s window at 100ms polls, driven by the injected timer")
	AssertLeaseFree(t, inst)
}

func TestSubmitReplyOnce_ShouldClassifyByteCountOfAFailedWrite(t *testing.T) {
	cases := []struct {
		name      string
		n         int
		wantStage ReplyStage
	}{
		{"zero bytes with an error is not sent", 0, StageNotStarted},
		{"a byte with an error is indeterminate", 1, StageWriteEntered},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst, lease := replyInstanceAndLease(t)
			pane := &fakeReplyPane{owner: replyOwner, captures: []string{"x"}, n: tc.n, err: errors.New("pty write failed")}
			stage, err := SubmitReplyOnce(context.Background(), pane, lease, '1', ReplyOptions{Dialog: colorQuestion, After: readyAfter})
			require.Error(t, err)
			assert.Equal(t, tc.wantStage, stage)
			AssertLeaseFree(t, inst)
		})
	}
}

func TestSubmitReplyOnce_ShouldWriteNothingAndReleaseTheLease_WhenARefusalHappensBeforeTheFirstByte(t *testing.T) {
	t.Run("before-first-byte check fails", func(t *testing.T) {
		inst, lease := replyInstanceAndLease(t)
		pane := &fakeReplyPane{owner: replyOwner, captures: []string{"x"}}
		stage, err := SubmitReplyOnce(context.Background(), pane, lease, '1', ReplyOptions{
			Dialog: colorQuestion, After: readyAfter, BeforeFirstByte: func() error { return errors.New("stale") },
		})
		require.Error(t, err)
		assert.Equal(t, StageNotStarted, stage)
		assert.Empty(t, pane.writtenBytes())
		AssertLeaseFree(t, inst)
	})
	t.Run("bad digit", func(t *testing.T) {
		for _, d := range []byte{'0', 'a', '\r', '\n', 0x1b} {
			inst, lease := replyInstanceAndLease(t)
			pane := &fakeReplyPane{owner: replyOwner, captures: []string{"x"}}
			stage, err := SubmitReplyOnce(context.Background(), pane, lease, d, ReplyOptions{Dialog: colorQuestion})
			assert.ErrorIs(t, err, ErrReplyDigit)
			assert.Equal(t, StageNotStarted, stage)
			assert.Empty(t, pane.writtenBytes())
			AssertLeaseFree(t, inst)
		}
	})
	t.Run("nil and foreign lease", func(t *testing.T) {
		pane := &fakeReplyPane{owner: replyOwner, captures: []string{"x"}}
		_, err := SubmitReplyOnce(context.Background(), pane, nil, '1', ReplyOptions{})
		assert.ErrorIs(t, err, ErrNoLease)
		other := &Instance{UUID: "someone-else"}
		l, ok := other.TryTerminalWriteLease(LeaseWriterReply)
		require.True(t, ok)
		_, err = SubmitReplyOnce(context.Background(), pane, l, '1', ReplyOptions{})
		assert.ErrorIs(t, err, ErrLeaseMismatch)
		assert.Empty(t, pane.writtenBytes())
		AssertLeaseFree(t, other)
	})
	t.Run("context already done", func(t *testing.T) {
		inst, lease := replyInstanceAndLease(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		pane := &fakeReplyPane{owner: replyOwner, captures: []string{"x"}}
		stage, err := SubmitReplyOnce(ctx, pane, lease, '1', ReplyOptions{})
		require.Error(t, err)
		assert.Equal(t, StageNotStarted, stage)
		assert.Empty(t, pane.writtenBytes())
		AssertLeaseFree(t, inst)
	})
}

// T-RP-53: a write blocked past the caller's timeout is indeterminate, the lease
// stays held until the goroutine exits, and nothing further is ever written.
func TestSubmitReplyOnce_ShouldIssueZeroFurtherWritesAndHoldTheLease_WhenFirstWriteBlocksPastTimeoutThenUnblocks(t *testing.T) {
	inst, lease := replyInstanceAndLease(t)
	timeout := make(chan time.Time, 1)
	pane := &fakeReplyPane{
		owner: replyOwner, captures: []string{"closed"},
		block: make(chan struct{}), entered: make(chan struct{}, 1),
	}
	type res struct {
		stage ReplyStage
		err   error
	}
	out := make(chan res, 1)
	go func() {
		s, e := SubmitReplyOnce(context.Background(), pane, lease, '2', ReplyOptions{
			Dialog: colorQuestion, SendTimeout: 5 * time.Second,
			After: func(d time.Duration) <-chan time.Time {
				if d == 5*time.Second {
					return timeout
				}
				return readyAfter(d)
			},
		})
		out <- res{s, e}
	}()
	<-pane.entered // the goroutine is inside the write
	timeout <- time.Now()
	r := <-out
	assert.Equal(t, StageWriteEntered, r.stage)
	require.ErrorIs(t, r.err, ErrReplyTimeout)

	// The caller returned; the abandoned goroutine still holds the lease.
	_, ok := inst.TryTerminalWriteLease(LeaseWriterReply)
	assert.False(t, ok, "lease must be held until the abandoned writer exits")

	close(pane.block)
	require.Eventually(t, func() bool {
		l, ok := inst.TryTerminalWriteLease(LeaseWriterOther)
		if ok {
			l.Release()
		}
		return ok
	}, 2*time.Second, time.Millisecond)
	assert.Equal(t, []string{"2"}, pane.writtenBytes(), "exactly the one write that was already in the syscall")
}

func TestSubmitReplyOnce_ShouldNeverWrite_WhenCallerTimesOutBeforeTheWriteStarts(t *testing.T) {
	inst, lease := replyInstanceAndLease(t)
	timeout := make(chan time.Time, 1)
	gate := make(chan struct{})
	entered := make(chan struct{})
	pane := &fakeReplyPane{owner: replyOwner, captures: []string{"closed"}}
	out := make(chan ReplyStage, 1)
	var outErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s, e := SubmitReplyOnce(context.Background(), pane, lease, '2', ReplyOptions{
			Dialog: colorQuestion, SendTimeout: 5 * time.Second,
			BeforeFirstByte: func() error { close(entered); <-gate; return nil },
			After: func(d time.Duration) <-chan time.Time {
				if d == 5*time.Second {
					return timeout
				}
				return readyAfter(d)
			},
		})
		outErr = e
		out <- s
	}()
	<-entered // the goroutine is parked in the before-first-byte check
	timeout <- time.Now()
	assert.Equal(t, StageNotStarted, <-out)
	wg.Wait()
	require.ErrorIs(t, outErr, ErrReplyTimeout)
	close(gate) // the goroutine now finds the state aborted
	require.Eventually(t, func() bool {
		l, ok := inst.TryTerminalWriteLease(LeaseWriterOther)
		if ok {
			l.Release()
		}
		return ok
	}, 2*time.Second, time.Millisecond)
	assert.Empty(t, pane.writtenBytes(), "an aborted reply never writes")
}

// T-RP-52: an instance that is not started provably wrote nothing, so a Reply to
// it is not-sent and the claim can be released.
func TestSendKeysN_ShouldReturnZeroBytesAndAnError_WhenInstanceNotStarted(t *testing.T) {
	n, err := (&Instance{UUID: replyOwner}).SendKeysN("1")
	require.Error(t, err)
	assert.Zero(t, n)
}
