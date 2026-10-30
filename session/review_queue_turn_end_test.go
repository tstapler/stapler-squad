package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

const turnEndGoldenPath = "testdata/review_queue/turn_end.golden.json"

func turnEndInstance(title string, te TurnEnded, changeAt time.Time) *Instance {
	return startedInstance(title, func(inst *Instance, now time.Time) {
		inst.LastMeaningfulOutput = now.Add(-time.Second)
		inst.TurnEnded = te
		inst.LastMeaningfulChangeAt = changeAt
	})
}

func turnEndAt(kind TurnEndKind, ago time.Duration) TurnEnded {
	te := FromHook("demo", "Which approach do you prefer?", time.Now().Add(-ago), kind, TurnEndConfidenceHigh)
	return te
}

func determineWith(inst *Instance, info InstanceStatusInfo, opts DeterminerOptions) DetectionResult {
	return NewDefaultStatusDeterminer(DefaultReviewQueuePollerConfig()).
		Determine(inst, "", info, detection.NewStatusDetector(), opts)
}

var (
	waitingController = InstanceStatusInfo{IsControllerActive: true, IdleState: detection.IdleStateInfo{State: detection.IdleStateWaiting}}
	successController = InstanceStatusInfo{IsControllerActive: true, ClaudeStatus: detection.StatusSuccess, IdleState: detection.IdleStateInfo{State: detection.IdleStateWaiting}}
	approvalCtl       = InstanceStatusInfo{IsControllerActive: true, ClaudeStatus: detection.StatusNeedsApproval, IdleState: detection.IdleStateInfo{State: detection.IdleStateWaiting}}
	on                = DeterminerOptions{ProseQuestion: true}
)

func TestDetermine_Should_IgnoreTurnEnded_When_ProseQuestionOptionOff(t *testing.T) {
	t.Parallel()
	for _, kind := range []TurnEndKind{TurnEndKindQuestion, TurnEndKindFinished, TurnEndKindError, TurnEndKindUnknown} {
		for name, info := range map[string]InstanceStatusInfo{"waiting": waitingController, "success": successController, "approval": approvalCtl} {
			with := determineWith(turnEndInstance("demo", turnEndAt(kind, time.Minute), time.Time{}), info, DeterminerOptions{})
			without := determineWith(turnEndInstance("demo", TurnEnded{}, time.Time{}), info, DeterminerOptions{})
			if with != without {
				t.Errorf("%v/%s: option off changed the result: %+v vs %+v", kind, name, with, without)
			}
		}
	}
}

func TestDetermine_Should_MapTurnEndKinds_When_OptionOn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		kind       TurnEndKind
		info       InstanceStatusInfo
		wantAction DetectionAction
		wantReason AttentionReason
		wantPrio   Priority
	}{
		{"question", TurnEndKindQuestion, waitingController, DetectionActionAdd, ReasonProseQuestion, PriorityMedium},
		{"finished", TurnEndKindFinished, waitingController, DetectionActionAdd, ReasonAwaitingDirection, PriorityLow},
		{"finished_replaces_task_complete", TurnEndKindFinished, successController, DetectionActionAdd, ReasonAwaitingDirection, PriorityLow},
		{"error", TurnEndKindError, waitingController, DetectionActionAdd, ReasonErrorState, PriorityUrgent},
		{"approval_keeps_precedence_over_question", TurnEndKindQuestion, approvalCtl, DetectionActionAdd, ReasonApprovalPending, PriorityHigh},
		{"approval_keeps_precedence_over_finished", TurnEndKindFinished, approvalCtl, DetectionActionAdd, ReasonApprovalPending, PriorityHigh},
		{"approval_keeps_precedence_over_error", TurnEndKindError, approvalCtl, DetectionActionAdd, ReasonApprovalPending, PriorityHigh},
		{"unknown_is_no_signal", TurnEndKindUnknown, waitingController, DetectionActionSkip, "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			te := turnEndAt(tc.kind, time.Minute)
			got := determineWith(turnEndInstance("demo", te, time.Time{}), tc.info, on)
			if got.Action != tc.wantAction || got.Reason != tc.wantReason || got.Priority != tc.wantPrio {
				t.Fatalf("got action=%v reason=%v prio=%v, want %v %v %v", got.Action, got.Reason, got.Priority, tc.wantAction, tc.wantReason, tc.wantPrio)
			}
			if tc.wantReason == ReasonProseQuestion && (got.Context != te.Excerpt || got.Episode != te.Episode) {
				t.Errorf("context/episode = %q/%q, want %q/%q", got.Context, got.Episode, te.Excerpt, te.Episode)
			}
		})
	}
}

func TestDetermine_Should_EqualNoTurnEndResult_When_KindIsUnknown(t *testing.T) {
	t.Parallel()
	for name, info := range map[string]InstanceStatusInfo{"waiting": waitingController, "success": successController, "approval": approvalCtl} {
		with := determineWith(turnEndInstance("demo", turnEndAt(TurnEndKindUnknown, time.Minute), time.Time{}), info, on)
		without := determineWith(turnEndInstance("demo", TurnEnded{}, time.Time{}), info, on)
		if with != without {
			t.Errorf("%s: Unknown kind changed the result: %+v vs %+v", name, with, without)
		}
	}
}

func TestDetermine_Should_DropAwaitingDirection_When_NewTurnStartedOrTTLPassed(t *testing.T) {
	t.Parallel()
	endedAt := time.Now().Add(-time.Hour)
	tests := []struct {
		name     string
		ago      time.Duration
		changeAt time.Time
		want     AttentionReason
	}{
		{"open_episode", time.Hour, time.Time{}, ReasonAwaitingDirection},
		{"output_after_turn_end_is_a_new_turn", time.Hour, endedAt.Add(time.Minute), ""},
		{"output_before_turn_end_is_same_turn", time.Hour, endedAt.Add(-time.Minute), ReasonAwaitingDirection},
		{"untouched_for_24h_auto_clears", 24*time.Hour + time.Minute, time.Time{}, ""},
		{"just_under_24h_stays", 23 * time.Hour, time.Time{}, ReasonAwaitingDirection},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := determineWith(turnEndInstance("demo", turnEndAt(TurnEndKindFinished, tc.ago), tc.changeAt), waitingController, on)
			if got.Reason != tc.want {
				t.Errorf("reason = %q, want %q", got.Reason, tc.want)
			}
		})
	}
}

func TestSignalForTurnEnd_Should_CoverEveryTurnEndKind(t *testing.T) {
	t.Parallel()
	kinds := 0
	for k := TurnEndKind(0); ; k++ {
		if k != TurnEndKindUnknown && k.String() == "unknown" {
			break
		}
		kinds++
		_, ok := signalForTurnEnd(TurnEnded{Kind: k, Excerpt: "x"})
		if ok == (k == TurnEndKindUnknown) {
			t.Errorf("kind %v: signal ok=%v; only Unknown may be no-signal", k, ok)
		}
	}
	if kinds != 4 {
		t.Errorf("TurnEndKind has %d named kinds, signalForTurnEnd was written for 4: add the new kind to its switch", kinds)
	}
}

func TestDetermine_Should_MatchTurnEndGolden(t *testing.T) {
	t.Parallel()
	type row struct {
		Case string           `json:"case"`
		Off  determineOutcome `json:"option_off"`
		On   determineOutcome `json:"option_on"`
	}
	infos := []struct {
		name string
		info InstanceStatusInfo
	}{{"waiting", waitingController}, {"task_complete", successController}, {"approval", approvalCtl}}
	var rows []row
	for _, in := range infos {
		for _, kind := range []TurnEndKind{TurnEndKindQuestion, TurnEndKindFinished, TurnEndKindError, TurnEndKindUnknown} {
			// Context carries the excerpt, which is fixed; Episode is excluded from the outcome.
			mk := func() *Instance { return turnEndInstance("demo", turnEndAt(kind, time.Minute), time.Time{}) }
			rows = append(rows, row{
				Case: in.name + "/" + kind.String(),
				Off:  outcomeOf(determineWith(mk(), in.info, DeterminerOptions{})),
				On:   outcomeOf(determineWith(mk(), in.info, on)),
			})
		}
	}
	got, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.FromSlash(turnEndGoldenPath)
	if *updateDetermineGoldens {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", path, err)
	}
	if string(want) != string(got) {
		t.Errorf("golden mismatch\n--- want\n%s\n--- got\n%s", want, got)
	}
}
