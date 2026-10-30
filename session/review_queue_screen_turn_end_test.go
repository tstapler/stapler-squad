package session

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func screenPoller(t *testing.T, content map[string]string, opts DeterminerOptions) *ReviewQueuePoller {
	t.Helper()
	p := NewReviewQueuePoller(NewReviewQueue(), newFakeStatusProvider(map[string]InstanceStatusInfo{}), nil)
	p.contentProvider = newFakeContentProvider(content)
	p.SetOptionsSource(func() (DeterminerOptions, RuleOptions) { return opts, RuleOptions{} })
	return p
}

func readPane(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/panes/" + name)
	require.NoError(t, err)
	return string(b)
}

func programInstance(title, program string) *Instance {
	inst := startedInstance(title, withCreatedAgo(time.Hour), withAges(time.Second, time.Second))
	inst.Program = program
	inst.SyncAtomicTimestamps()
	return inst
}

func TestScreenStage_Should_RecordFinishedFromScreen_When_AiderIdleAndFlagOn(t *testing.T) {
	t.Parallel()
	p := screenPoller(t, map[string]string{"a": readPane(t, "aider/idle_prose_question.txt")}, DeterminerOptions{ProseQuestion: true})
	inst := programInstance("a", "aider")

	p.checkSession(inst, nil)

	te := inst.Snapshot().TurnEnded
	assert.Equal(t, TurnEndSourceScreen, te.Source)
	assert.Equal(t, TurnEndKindFinished, te.Kind)
	assert.Contains(t, te.Excerpt, "Do you want me to move it?")
}

func TestScreenStage_Should_RecordClaudeQuestion_When_NoHookEverFired(t *testing.T) {
	t.Parallel()
	p := screenPoller(t, map[string]string{"c": readPane(t, "claude/idle_prose_question.txt")}, DeterminerOptions{ProseQuestion: true})
	inst := programInstance("c", "claude")

	p.checkSession(inst, nil)

	te := inst.Snapshot().TurnEnded
	assert.Equal(t, TurnEndKindQuestion, te.Kind)
	assert.Equal(t, TurnEndSourceScreen, te.Source)
}

func TestScreenStage_Should_DoNothing_When_FlagOff(t *testing.T) {
	t.Parallel()
	p := screenPoller(t, map[string]string{"a": readPane(t, "aider/idle_prose_question.txt")}, DeterminerOptions{})
	inst := programInstance("a", "aider")

	p.checkSession(inst, nil)

	assert.True(t, inst.Snapshot().TurnEnded.IsZero())
}

func TestScreenStage_Should_NotOverwriteHookTurnEnd(t *testing.T) {
	t.Parallel()
	p := screenPoller(t, map[string]string{"c": readPane(t, "claude/idle_finished.txt")}, DeterminerOptions{ProseQuestion: true})
	inst := programInstance("c", "claude")
	inst.RecordTurnEnd("Which one do you want?", TurnEndSourceHook)
	before := inst.Snapshot().TurnEnded

	p.checkSession(inst, nil)

	assert.Equal(t, before, inst.Snapshot().TurnEnded)
}

func TestScreenStage_Should_RecordOnce_When_ScreenUnchangedAcrossPolls(t *testing.T) {
	t.Parallel()
	p := screenPoller(t, map[string]string{"a": readPane(t, "aider/idle_prose_question.txt")}, DeterminerOptions{ProseQuestion: true})
	inst := programInstance("a", "aider")

	p.checkSession(inst, nil)
	first := inst.Snapshot().TurnEnded
	p.checkSession(inst, nil)

	assert.Equal(t, first, inst.Snapshot().TurnEnded)
}

func TestScreenStage_Should_SkipHiddenAndArchivedSessions(t *testing.T) {
	t.Parallel()
	opts := DeterminerOptions{ProseQuestion: true}
	pane := readPane(t, "aider/idle_prose_question.txt")
	p := screenPoller(t, map[string]string{"h": pane, "x": pane}, opts)

	hidden := programInstance("h", "aider")
	hidden.Hidden = true
	archived := programInstance("x", "aider")
	now := time.Now()
	archived.ArchivedAt = &now
	for _, inst := range []*Instance{hidden, archived} {
		inst.SyncAtomicTimestamps()
		p.recordScreenTurnEnd(inst, inst.Snapshot(), pane, InstanceStatusInfo{}, opts)
		assert.True(t, inst.Snapshot().TurnEnded.IsZero(), inst.Title)
	}
}

func TestScreenStage_Should_NotRecordForNoneRowPrograms(t *testing.T) {
	t.Parallel()
	for _, program := range []string{"gemini", "opencode", "pi"} {
		p := screenPoller(t, map[string]string{"g": readPane(t, program+"/idle_finished.txt")}, DeterminerOptions{ProseQuestion: true})
		inst := programInstance("g", program)
		p.checkSession(inst, nil)
		assert.True(t, inst.Snapshot().TurnEnded.IsZero(), program)
		assert.True(t, CannotDetect(program), program)
	}
}

func TestScreenStage_Should_UpgradeAmbiguousFinishedToQuestion_When_TiebreakSaysQuestion(t *testing.T) {
	t.Parallel()
	opts := DeterminerOptions{ProseQuestion: true, ProseQuestionLLMTiebreak: true}
	pane := "Does this layout work for the dashboard?\n\n> \n? for shortcuts\n"
	p := screenPoller(t, map[string]string{"c": pane}, opts)
	// Ambiguity needs a Medium Finished: a non-directed question.
	pane = "Why does this layout matter?\n\n> \n? for shortcuts\n"
	p.contentProvider = newFakeContentProvider(map[string]string{"c": pane})
	tb := &fakeTiebreaker{class: TurnEndClassQuestion}
	p.SetTurnEndTiebreaker(tb)
	inst := programInstance("c", "claude")

	p.checkSession(inst, nil)

	require.Eventually(t, func() bool { return inst.Snapshot().TurnEnded.Kind == TurnEndKindQuestion }, time.Second, 5*time.Millisecond)
	assert.EqualValues(t, 1, tb.calls.Load())
}

func TestScreenStage_Should_StayFinished_When_TiebreakFlagOffOrFails(t *testing.T) {
	t.Parallel()
	pane := "Why does this layout matter?\n\n> \n? for shortcuts\n"

	off := screenPoller(t, map[string]string{"c": pane}, DeterminerOptions{ProseQuestion: true})
	tbOff := &fakeTiebreaker{class: TurnEndClassQuestion}
	off.SetTurnEndTiebreaker(tbOff)
	instOff := programInstance("c", "claude")
	off.checkSession(instOff, nil)
	assert.Equal(t, TurnEndKindFinished, instOff.Snapshot().TurnEnded.Kind)
	assert.Zero(t, tbOff.calls.Load(), "flag off must not call the tiebreaker")

	on := screenPoller(t, map[string]string{"c": pane}, DeterminerOptions{ProseQuestion: true, ProseQuestionLLMTiebreak: true})
	tbFail := &fakeTiebreaker{err: errTiebreakTimeout{}}
	on.SetTurnEndTiebreaker(tbFail)
	instOn := programInstance("c", "claude")
	on.checkSession(instOn, nil)
	require.Eventually(t, func() bool { return tbFail.calls.Load() == 1 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, TurnEndKindFinished, instOn.Snapshot().TurnEnded.Kind)
}

type errTiebreakTimeout struct{}

func (errTiebreakTimeout) Error() string { return "deadline exceeded" }
