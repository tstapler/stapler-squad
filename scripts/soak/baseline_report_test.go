package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func t0() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) }

func notif(session string, typ int32, ago time.Duration) Notification {
	return Notification{SessionID: session, NotificationType: typ, CreatedAt: t0().Add(-ago)}
}

func TestBaselineReport_Should_ComputeDetectedButMissedRateWithWindowAndSampleSize_When_FixtureHistoryReplayed(t *testing.T) {
	viewed := t0().Add(-48*time.Hour + 10*time.Minute)
	touches := map[string]Touch{
		"a": {Program: "claude", LastViewed: viewed},
		"b": {Program: "claude"},
		"c": {Program: "aider"},
	}
	recs := []Notification{
		notif("a", typeInputRequired, 48*time.Hour),  // viewed within 30m: touched
		notif("b", typeApprovalNeeded, 24*time.Hour), // missed
		notif("c", typeError, 2*time.Hour),           // missed
		notif("a", 4, time.Hour),                     // not needs-operator
	}
	rep := ComputeDetectedMissed(recs, touches, t0(), 30, 30*time.Minute)
	if rep.Total.Episodes != 3 || rep.Total.Missed != 2 {
		t.Fatalf("total = %+v", rep.Total)
	}
	if got := rep.PerProgram["claude"]; got.Episodes != 2 || got.Missed != 1 {
		t.Fatalf("claude = %+v", got)
	}
	if rep.PerProgram["aider"].Missed != 1 {
		t.Fatalf("aider = %+v", rep.PerProgram["aider"])
	}
}

func TestBaselineReport_Should_StateActualCoveredWindowAndLimitations_When_HistoryShorterThan30Days(t *testing.T) {
	recs := []Notification{notif("a", typeError, 5*24*time.Hour), notif("a", typeError, time.Hour)}
	rep := ComputeDetectedMissed(recs, nil, t0(), 30, 30*time.Minute)
	if rep.CoveredDays > 5.1 || rep.CoveredDays < 4.9 {
		t.Fatalf("covered days = %v", rep.CoveredDays)
	}
	var buf bytes.Buffer
	renderDetected(&buf, rep)
	out := buf.String()
	if !strings.Contains(out, "not the requested 30 days") || !strings.Contains(out, "arbitrary") {
		t.Fatalf("missing honest window/limitations:\n%s", out)
	}
	if !strings.Contains(out, "unknown (session deleted)") {
		t.Fatalf("expected unknown program bucket:\n%s", out)
	}
}

func TestBaselineReport_Should_CountUndetectedQuestionSampleSeparately_When_LabelledTranscriptsProvided(t *testing.T) {
	csvIn := "id,mtime,last_assistant_text,is_question,touched_within_30m\n" +
		"1,x,t,yes,no\n2,x,t,yes,yes\n3,x,t,no,no\n4,x,t,,\n"
	labelled, err := LoadLabels(strings.NewReader(csvIn))
	if err != nil {
		t.Fatal(err)
	}
	s := SummarizeSample(labelled)
	if s.N != 3 || s.Questions != 2 || s.QuestionsUntouched != 1 || s.Pending {
		t.Fatalf("sample = %+v", s)
	}
	if !SummarizeSample(nil).Pending {
		t.Fatal("empty sample must be pending")
	}
}

func TestBaselineReport_Should_SwitchToNoIncreasePlusRecallTarget_When_B1Below5Percent(t *testing.T) {
	if !strings.Contains(SelectTarget(0.03).Description, "recall") {
		t.Fatal("B1 < 5% must use the no-increase plus recall target")
	}
	if strings.Contains(SelectTarget(0.20).Description, "recall") {
		t.Fatal("B1 >= 5% must use the relative target")
	}
}

func TestBaselineReport_Should_AveragePageViewsOverTrailing14CompleteDays(t *testing.T) {
	perDay := map[string]int{"2026-10-23": 14, "2026-10-10": 14, "2026-10-24": 1000, "2026-10-09": 1000}
	u := ComputeU1(perDay, t0(), 14)
	if u.Total != 28 || u.Mean != 2 {
		t.Fatalf("u1 = %+v", u)
	}
}

func TestBaselineReport_Should_ExtractLastAssistantText(t *testing.T) {
	in := `{"type":"assistant","message":{"content":[{"type":"text","text":"first"}]}}
{"type":"user","message":{"content":"x"}}
{"type":"assistant","message":{"content":[{"type":"thinking"},{"type":"text","text":"Shall I proceed?"}]}}
`
	if got := lastAssistantText(strings.NewReader(in)); got != "Shall I proceed?" {
		t.Fatalf("got %q", got)
	}
}
