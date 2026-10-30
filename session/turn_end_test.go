package session

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lockCheckingFake struct {
	inst  *Instance
	mu    sync.Mutex
	calls int
	// lockFree records whether i.mu could be write-locked inside the callback.
	lockFree bool
}

func (f *lockCheckingFake) observe() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lockFree = f.inst.mu.TryLock()
	if f.lockFree {
		f.inst.mu.Unlock()
	}
}

func (f *lockCheckingFake) PersistTurnEnded(string, TurnEnded) error { f.observe(); return nil }

type fakeRecorder struct {
	noopSessionEventRecorder
	lockCheckingFake
}

func (f *fakeRecorder) RecordTurnEnded(string, TurnEnded) { f.observe() }

func TestRecordTurnEnd_Should_SetSnapshotFactAndNotPanic_When_InstanceIsBareLiteral(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "bare", Status: Running}

	inst.RecordTurnEnd("finished", TurnEndSourceHook)

	assert.Equal(t, "finished", inst.Snapshot().TurnEnded.Excerpt)
	assert.Equal(t, TurnEndSourceHook, inst.Snapshot().TurnEnded.Source)
}

func TestRecordTurnEnd_Should_SetSnapshotFactAndNotPanic_When_InstanceFromInstanceData(t *testing.T) {
	t.Parallel()
	inst, err := FromInstanceDataDeferred(InstanceData{Title: "restored", Path: t.TempDir(), Status: Paused, Program: "claude"})
	require.NoError(t, err)

	inst.RecordTurnEnd("finished", TurnEndSourceScreen)

	assert.Equal(t, "finished", inst.Snapshot().TurnEnded.Excerpt)
}

func TestRecordTurnEnd_Should_CallSinkAndRecorderOnceAfterReleasingLock_When_CollaboratorsSet(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "wired", Status: Running}
	sink := &lockCheckingFake{inst: inst}
	rec := &fakeRecorder{lockCheckingFake: lockCheckingFake{inst: inst}}
	inst.SetTurnEndSink(sink)
	inst.SetEventRecorder(rec)

	inst.RecordTurnEnd("done", TurnEndSourceHook)

	assert.Equal(t, 1, sink.calls)
	assert.True(t, sink.lockFree, "sink ran under i.mu")
	assert.Equal(t, 1, rec.calls)
	assert.True(t, rec.lockFree, "recorder ran under i.mu")
}

func TestRecordTurnEnd_Should_SkipSinkAndRecorder_When_ActorStopped(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "stopped", Status: Running}
	sink := &lockCheckingFake{inst: inst}
	rec := &fakeRecorder{lockCheckingFake: lockCheckingFake{inst: inst}}
	inst.SetTurnEndSink(sink)
	inst.SetEventRecorder(rec)
	NewLiveInstance(inst).Stop()

	inst.RecordTurnEnd("late", TurnEndSourceHook)

	assert.Equal(t, 0, sink.calls)
	assert.Equal(t, 0, rec.calls)
}

func TestSetTurnEndSink_Should_KeepNoopDefault_When_TypedNilPassed(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "typednil", Status: Running}
	var sink *lockCheckingFake
	inst.SetTurnEndSink(sink)

	inst.RecordTurnEnd("done", TurnEndSourceHook)

	assert.Equal(t, "done", inst.Snapshot().TurnEnded.Excerpt)
}

func TestFromHook_Should_BuildStableEpisodeAndBoundedScrubbedExcerpt_When_TextIsLongAndHasSecrets(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 29, 14, 2, 11, 0, time.UTC)
	long := strings.Repeat("filler ", 100) + "\x1b[31mset token=abc123secret\x1b[0m then: Which approach, A or B?\x00"

	a := FromHook("demo", long, at, TurnEndKindQuestion, TurnEndConfidenceHigh)
	b := FromHook("demo", long, at, TurnEndKindQuestion, TurnEndConfidenceHigh)

	assert.Equal(t, a, b, "same inputs must give a stable value")
	assert.NotEmpty(t, a.Episode)
	assert.NotEqual(t, a.Episode, FromHook("other", long, at, TurnEndKindQuestion, TurnEndConfidenceHigh).Episode)
	assert.NotEqual(t, a.Episode, FromHook("demo", long, at.Add(time.Nanosecond), TurnEndKindQuestion, TurnEndConfidenceHigh).Episode)
	assert.LessOrEqual(t, utf8.RuneCountInString(a.Excerpt), MaxTurnEndExcerptChars)
	assert.True(t, strings.HasSuffix(a.Excerpt, "Which approach, A or B?"), a.Excerpt)
	assert.NotContains(t, a.Excerpt, "abc123secret")
	assert.NotContains(t, a.Excerpt, "\x1b")
	assert.NotContains(t, a.Excerpt, "\x00")
	assert.Equal(t, TurnEndSourceHook, a.Source)
	assert.False(t, a.IsZero())
}

func TestScrubExcerpt_Should_StayBoundedAndValidUTF8_When_InputIsHugeMultibyte(t *testing.T) {
	t.Parallel()
	got := scrubExcerpt(strings.Repeat("é世 ", 200000))

	assert.True(t, utf8.ValidString(got))
	assert.LessOrEqual(t, utf8.RuneCountInString(got), MaxTurnEndExcerptChars)
}

func TestFromScreenAndFromTranscript_Should_SetTheirSource(t *testing.T) {
	t.Parallel()
	at := time.Now()
	assert.Equal(t, TurnEndSourceScreen, FromScreen("s", "x", at, TurnEndKindFinished, TurnEndConfidenceLow).Source)
	assert.Equal(t, TurnEndSourceTranscript, FromTranscript("s", "x", at, TurnEndKindFinished, TurnEndConfidenceLow).Source)
}

func TestRecordTurnEnd_Should_RecordQuestionFromHookAndAdvanceActivity_When_TextEndsInQuestion(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "demo", Status: Running}
	before := time.Now()

	inst.RecordTurnEnd("Should I use Postgres or SQLite?", TurnEndSourceHook)

	got := inst.Snapshot().TurnEnded
	assert.Equal(t, TurnEndKindQuestion, got.Kind)
	assert.Equal(t, TurnEndSourceHook, got.Source)
	assert.Equal(t, TurnEndConfidenceHigh, got.Confidence)
	assert.NotEmpty(t, got.Episode)
	assert.LessOrEqual(t, utf8.RuneCountInString(got.Excerpt), MaxTurnEndExcerptChars)
	assert.False(t, inst.Snapshot().LastMeaningfulChangeAt.Before(before))
	assert.Equal(t, got.At, inst.Snapshot().LastMeaningfulChangeAt)
}

func TestRecordTurnEnd_Should_ClassifyNonQuestionAsFinished_When_NoTrailingQuestionMark(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "demo", Status: Running}

	inst.RecordTurnEnd("All done.", TurnEndSourceScreen)

	assert.Equal(t, TurnEndKindFinished, inst.Snapshot().TurnEnded.Kind)
	assert.Equal(t, TurnEndSourceScreen, inst.Snapshot().TurnEnded.Source)
}

type blockingSink struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingSink) PersistTurnEnded(string, TurnEnded) error {
	close(b.entered)
	<-b.release
	return errors.New("disk full")
}

func TestRecordTurnEnd_Should_KeepSnapshotReadableAndNotBlockMarkViewed_When_SinkBlocks(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "slow", Status: Running}
	sink := &blockingSink{entered: make(chan struct{}), release: make(chan struct{})}
	inst.SetTurnEndSink(sink)
	done := make(chan struct{})
	go func() { defer close(done); inst.RecordTurnEnd("Pick one?", TurnEndSourceHook) }()

	select {
	case <-sink.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("sink never reached")
	}
	readers := make(chan struct{})
	go func() {
		defer close(readers)
		assert.Equal(t, TurnEndKindQuestion, inst.Snapshot().TurnEnded.Kind)
		inst.MarkViewed()
	}()
	select {
	case <-readers:
	case <-time.After(5 * time.Second):
		t.Fatal("Snapshot/MarkViewed blocked behind the persisting sink")
	}
	close(sink.release)
	<-done
	assert.Equal(t, "Pick one?", inst.Snapshot().TurnEnded.Excerpt, "sink error must not drop the fact")
}

func TestRecordTurnEnd_Should_RecordFactWithoutChangingStatus_When_SessionArchived(t *testing.T) {
	t.Parallel()
	archivedAt := time.Now().Add(-time.Hour)
	inst := &Instance{Title: "old", Status: Paused, ArchivedAt: &archivedAt}

	inst.RecordTurnEnd("Anything else?", TurnEndSourceHook)

	snap := inst.Snapshot()
	assert.Equal(t, TurnEndKindQuestion, snap.TurnEnded.Kind)
	assert.Equal(t, Paused, snap.Status, "a turn end must not revive an archived session")
	assert.NotNil(t, snap.ArchivedAt)
}

// Instance.TurnEnded is written only by the setter and read via Snapshot() elsewhere.
func TestTurnEndedField_Should_NotBeReadOutsideSetterAndSnapshot_When_SourceScanned(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`\b(?:inst|i|s\.inst)\.TurnEnded\b`)
	allowed := map[string]bool{"instance_actor_setters.go": true, "instance_snapshot.go": true, "instance.go": true}
	for _, dir := range []string{".", "../server"} {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || allowed[d.Name()] {
				return nil
			}
			b, rerr := os.ReadFile(p)
			require.NoError(t, rerr)
			if loc := re.Find(b); loc != nil {
				t.Errorf("%s reads the raw TurnEnded field (%q); use Snapshot().TurnEnded", p, loc)
			}
			return nil
		})
	}
}
