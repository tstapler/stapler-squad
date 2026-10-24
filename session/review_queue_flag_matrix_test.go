package session

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// matrixFlags mirrors the six attention-inbox flags (server/services.AttentionFlags).
// In PR A only IdleClockOutput and AckBypassFix change behaviour; ProseQuestionDetection
// reaches Determine as an inert option and SessionEventLog only decides whether a recorder
// is wired. ProseQuestionLLMTiebreak and InboxConsolidation have no session-package seam yet.
type matrixFlags struct {
	IdleClockOutput          bool
	ProseQuestionDetection   bool
	SessionEventLog          bool
	AckBypassFix             bool
	ProseQuestionLLMTiebreak bool
	InboxConsolidation       bool
}

func (f matrixFlags) options() (DeterminerOptions, RuleOptions) {
	return DeterminerOptions{IdleClockOutput: f.IdleClockOutput, ProseQuestion: f.ProseQuestionDetection},
		RuleOptions{AckBypassFix: f.AckBypassFix}
}

// matrixStage is one flip of the flags followed by the scripted sequence, with the
// outcomes it must produce. wantProseQuestionItem is the column PR B extends: it is
// false in every row until prose_question detection exists to raise that item.
type matrixStage struct {
	flags                 matrixFlags
	wantNeedsHumanKept    bool // InputRequired Medium survives an ack newer than the output
	wantIdleItem          bool // output-age idle item raised
	wantProseQuestionItem bool
}

type matrixRow struct {
	name   string
	stages []matrixStage
}

// matrixWant names the two PR A behaviours a flag set can change; all else must stay off.
type matrixWant struct{ humanKept, idleItem bool }

var (
	wantNothing = matrixWant{}
	wantHuman   = matrixWant{humanKept: true}
	wantIdle    = matrixWant{idleItem: true}
	wantBoth    = matrixWant{humanKept: true, idleItem: true}
)

// stageOf spells every expectation out at the call site rather than deriving it from the flags.
func stageOf(f matrixFlags, w matrixWant) matrixStage {
	return matrixStage{flags: f, wantNeedsHumanKept: w.humanKept, wantIdleItem: w.idleItem}
}

func matrixRows() []matrixRow {
	idle := matrixFlags{IdleClockOutput: true}
	idleDetect := matrixFlags{IdleClockOutput: true, ProseQuestionDetection: true}
	idleDetectBypass := matrixFlags{IdleClockOutput: true, ProseQuestionDetection: true, AckBypassFix: true}
	bypass := matrixFlags{AckBypassFix: true}
	bypassDetect := matrixFlags{AckBypassFix: true, ProseQuestionDetection: true}
	detect := matrixFlags{ProseQuestionDetection: true}
	detectLog := matrixFlags{ProseQuestionDetection: true, SessionEventLog: true}

	return []matrixRow{
		{"order1_idleClock_then_detection_then_ackBypass", []matrixStage{
			stageOf(idle, wantIdle), stageOf(idleDetect, wantIdle), stageOf(idleDetectBypass, wantBoth),
		}},
		{"order2_ackBypass_before_detection", []matrixStage{
			stageOf(bypass, wantHuman), stageOf(bypassDetect, wantHuman),
		}},
		{"order3_detection_alone_ackBypass_off", []matrixStage{
			stageOf(detect, wantNothing),
		}},
		{"order4_detection_with_sessionEventLog_on_then_off", []matrixStage{
			stageOf(detectLog, wantNothing), stageOf(detect, wantNothing),
		}},
		{"alone_all_flags_off", []matrixStage{stageOf(matrixFlags{}, wantNothing)}},
		{"alone_idleClockOutput", []matrixStage{stageOf(idle, wantIdle)}},
		{"alone_proseQuestionDetection", []matrixStage{stageOf(detect, wantNothing)}},
		{"alone_sessionEventLog", []matrixStage{stageOf(matrixFlags{SessionEventLog: true}, wantNothing)}},
		{"alone_ackBypassFix", []matrixStage{stageOf(bypass, wantHuman)}},
		{"alone_proseQuestionLLMTiebreak", []matrixStage{stageOf(matrixFlags{ProseQuestionLLMTiebreak: true}, wantNothing)}},
		{"alone_inboxConsolidation", []matrixStage{stageOf(matrixFlags{InboxConsolidation: true}, wantNothing)}},
	}
}

// raisedRecorder records AttentionRaised titles; every other event is a no-op.
type raisedRecorder struct {
	noopSessionEventRecorder
	mu     sync.Mutex
	raised []string
}

func (r *raisedRecorder) RecordAttentionRaised(title string, _ AttentionReason) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.raised = append(r.raised, title)
}

func (r *raisedRecorder) raisedFor(title string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, t := range r.raised {
		if t == title {
			n++
		}
	}
	return n
}

const matrixProsePrompt = "I found two ways to fix this.\nWhich approach would you like me to take?\n"

// matrixInstance is a started session whose output and update times are offsets from now.
func matrixInstance(title string, updatedAgo, outputAgo time.Duration) *Instance {
	now := time.Now()
	inst := &Instance{Title: title, UUID: "uuid-" + title, Status: Running}
	inst.started.Store(true)
	inst.CreatedAt = now.Add(-time.Hour)
	inst.UpdatedAt = now.Add(-updatedAgo)
	inst.LastMeaningfulOutput = now.Add(-outputAgo)
	inst.SyncAtomicTimestamps()
	return inst
}

func openItemsFor(p *ReviewQueuePoller, title string) int {
	n := 0
	for _, it := range p.queue.List() {
		if it.SessionID == title {
			n++
		}
	}
	return n
}

// matrixHarness is one live poller whose flags are flipped between stages.
type matrixHarness struct {
	poller  *ReviewQueuePoller
	flags   matrixFlags
	status  *fakeStatusProvider
	content *fakeContentProvider
	rec     *raisedRecorder
}

func newMatrixHarness() *matrixHarness {
	h := &matrixHarness{
		status:  newFakeStatusProvider(map[string]InstanceStatusInfo{}),
		content: newFakeContentProvider(map[string]string{}),
		rec:     &raisedRecorder{},
	}
	h.poller = NewReviewQueuePoller(NewReviewQueue(), h.status, nil)
	h.poller.contentProvider = h.content
	h.poller.statusDeterminer = idleClockDeterminer()
	h.poller.SetOptionsSource(func() (DeterminerOptions, RuleOptions) { return h.flags.options() })
	return h
}

// run flips the flags, then drives one needs-human session (raise, then ack after output),
// one output-idle session and one prose-question session.
func (h *matrixHarness) run(t *testing.T, idx int, st matrixStage) {
	t.Helper()
	h.flags = st.flags
	if st.flags.SessionEventLog {
		h.poller.SetEventRecorder(h.rec)
	} else {
		h.poller.SetEventRecorder(nil)
	}

	human := fmt.Sprintf("human-%d", idx)
	idleTitle := fmt.Sprintf("idle-%d", idx)
	prose := fmt.Sprintf("prose-%d", idx)
	h.status.statuses[human] = InstanceStatusInfo{IsControllerActive: true, ClaudeStatus: detection.StatusInputRequired}
	h.content.content[prose] = matrixProsePrompt

	humanInst := matrixInstance(human, 0, 60*time.Second)
	h.poller.checkSession(humanInst, nil)
	if got := openItemsFor(h.poller, human); got != 1 {
		t.Fatalf("stage %d: needs-human items before ack = %d, want 1", idx, got)
	}
	// A fresh Instance carries the ack: direct field writes do not republish an existing snapshot.
	ackedInst := matrixInstance(human, 0, 60*time.Second)
	ackedInst.LastAcknowledged = time.Now().Add(-30 * time.Second)
	ackedInst.SyncAtomicTimestamps()
	h.poller.checkSession(ackedInst, nil)
	h.poller.checkSession(matrixInstance(idleTitle, 2*time.Second, 10*time.Minute), nil)
	h.poller.checkSession(matrixInstance(prose, 0, time.Second), nil)

	for _, title := range []string{human, idleTitle, prose} {
		if got := openItemsFor(h.poller, title); got > 1 {
			t.Errorf("stage %d: %s has %d open items, want at most 1", idx, title, got)
		}
	}
	h.assertStage(t, idx, st, human, idleTitle, prose)
}

func (h *matrixHarness) assertStage(t *testing.T, idx int, st matrixStage, human, idleTitle, prose string) {
	t.Helper()
	if got := openItemsFor(h.poller, human) == 1; got != st.wantNeedsHumanKept {
		t.Errorf("stage %d flags %+v: needs-human kept after ack = %v, want %v", idx, st.flags, got, st.wantNeedsHumanKept)
	}
	if got := openItemsFor(h.poller, idleTitle) == 1; got != st.wantIdleItem {
		t.Errorf("stage %d flags %+v: idle item = %v, want %v", idx, st.flags, got, st.wantIdleItem)
	}
	if got := openItemsFor(h.poller, prose) == 1; got != st.wantProseQuestionItem {
		t.Errorf("stage %d flags %+v: prose-question item = %v, want %v (PR A: detection is inert)", idx, st.flags, got, st.wantProseQuestionItem)
	}
	if got := h.rec.raisedFor(human) > 0; got != st.flags.SessionEventLog {
		t.Errorf("stage %d: attention events recorded = %v, want %v (session_event_log)", idx, got, st.flags.SessionEventLog)
	}
}

func TestFlagMatrix_Should_KeepOneItemPerSessionNoBuriedProseQuestionAndNeedsHumanBadge_When_FourOrdersAndEachFlagAlone(t *testing.T) {
	t.Parallel()
	for _, row := range matrixRows() {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			h := newMatrixHarness()
			for i, st := range row.stages {
				h.run(t, i, st)
			}
		})
	}
}

// With every flag off, flags that are inert in PR A must not change a single Determine
// outcome over the committed corpus (the goldens themselves are checked by
// TestDetermineGolden_*, which runs with DeterminerOptions{}).
func TestFlagMatrix_Should_MatchAllOffOutcome_When_InertFlagsOnOverCorpus(t *testing.T) {
	t.Parallel()
	clock := goldenClock{}
	detector := detection.NewStatusDetector()
	determiner := NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig())
	for _, c := range corpusCases(t) {
		data, err := os.ReadFile(c.fixturePath)
		if err != nil {
			t.Fatal(err)
		}
		raw := string(data)
		off := determiner.Determine(clock.instance(c.name), raw, InstanceStatusInfo{}, detector, DeterminerOptions{})
		on := determiner.Determine(clock.instance(c.name), raw, InstanceStatusInfo{}, detector, DeterminerOptions{ProseQuestion: true})
		if outcomeOf(off) != outcomeOf(on) {
			t.Errorf("%s/%s: ProseQuestion changed outcome %+v -> %+v", c.program, c.name, outcomeOf(off), outcomeOf(on))
		}
	}
}
