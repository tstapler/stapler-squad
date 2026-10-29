package prwatch

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	uuidA = "aaaaaaaa-1111"
	uuidB = "bbbbbbbb-2222"
	uuidC = "cccccccc-3333"
)

func newTestTracker() (*StatusTracker, *fakeClock, *CounterSet) {
	clock := newFakeClock()
	cs := NewCounterSet()
	tr := NewStatusTracker(clock, cs)
	tr.SetEnabled(true, 4)
	tr.SetLeaderHeld(true)
	return tr, clock, cs
}

func TestStatus_should_ReportFlagInboxTokenWatchedSkippedHeldAndLastSteer_When_ScenarioRuns(t *testing.T) {
	tr, clock, _ := newTestTracker()
	tr.SetLoginResolved(true)
	tr.SetInbox(InboxStateUnreachable, "connect_refused")
	tr.SetSessions([]SessionRef{
		{UUID: uuidA, Title: "fix auth", Repo: "o/a", PR: 1, State: StateLive},
		{UUID: uuidB, Title: "old", Repo: "o/b", PR: 2, State: StateSuspended},
		{UUID: uuidC, Title: "no pr", SkipReason: SkipNoPR},
	})
	tr.SetSubscribed(uuidA, true)
	tr.NoteEvent(uuidA, clock.Now())
	tr.SetHeld([]HeldItem{{SessionUUID: uuidB, Repo: "o/b", PR: 2, Kind: KindComment, Reason: HoldBusy, Since: clock.Now()}})
	clock.Advance(90 * time.Second)
	tr.Record(Disposition{At: clock.Now(), Kind: KindCI, Repo: "o/a", PR: 1, SessionUUID: uuidA, Disposition: DispositionSteered})

	st := tr.Snapshot(&fakeCoalescer{})
	if !st.Enabled || !st.Polls || !st.LeaderHeld || !st.LoginResolved {
		t.Fatalf("flag/leader/login wrong: %+v", st)
	}
	if st.InboxState != InboxStateUnreachable || st.InboxErrorClass != "connect_refused" || st.InboxErrorAt == nil {
		t.Fatalf("inbox state wrong: %+v", st)
	}
	if len(st.Watched) != 2 || len(st.Skipped) != 1 || st.Skipped[0].Reason != SkipNoPR {
		t.Fatalf("watched/skipped wrong: %+v / %+v", st.Watched, st.Skipped)
	}
	if len(st.Held) != 1 || st.Held[0].AgeSeconds != 90 || st.Held[0].Reason != HoldBusy {
		t.Fatalf("held wrong: %+v", st.Held)
	}
	if st.LastSteer == nil || st.LastSteer.Repo != "o/a" || st.LastSteer.ShortUUID != "aaaaaaaa" || st.LastSteer.ActorClass != "ci" {
		t.Fatalf("last steer wrong: %+v", st.LastSteer)
	}
	if st.Counts.BaselineCount != 4 || st.FlagOnAt == nil {
		t.Fatalf("baseline not recorded at flag-on: %+v", st.Counts)
	}
	// One next-steer value per session: A is clear, B has a hold.
	for _, w := range st.Watched {
		if w.Repo == "o/a" && w.NextSteer != NextSteerNow {
			t.Fatalf("A next = %s, want now", w.NextSteer)
		}
		if w.Repo == "o/b" && w.NextSteer != NextSteerWaiting {
			t.Fatalf("B next = %s, want waiting", w.NextSteer)
		}
	}
	// Baseline is fixed at first flag-on.
	tr.SetEnabled(false, 0)
	tr.SetEnabled(true, 99)
	if got := tr.Snapshot(nil).Counts.BaselineCount; got != 4 {
		t.Fatalf("baseline changed to %d", got)
	}
}

func TestStatus_should_ReportEverySkipReason_When_SessionsAreIneligible(t *testing.T) {
	tr, _, _ := newTestTracker()
	var refs []SessionRef
	for i, r := range AllSkipReasons() {
		refs = append(refs, SessionRef{UUID: "uuid-skip-" + string(rune('a'+i)), SkipReason: r})
	}
	tr.SetSessions(refs)
	st := tr.Snapshot(nil)
	if len(st.Skipped) != len(AllSkipReasons()) || st.Counts.Skipped != len(AllSkipReasons()) {
		t.Fatalf("skipped = %d, want %d", len(st.Skipped), len(AllSkipReasons()))
	}
	sum := 0
	for _, r := range AllSkipReasons() {
		if st.Counts.SkippedByReason[string(r)] != 1 {
			t.Errorf("reason %s missing", r)
		}
		sum += st.Counts.SkippedByReason[string(r)]
	}
	if sum != st.Counts.Skipped {
		t.Fatalf("breakdown %d != total %d", sum, st.Counts.Skipped)
	}
}

func TestStatus_should_ContainNoTokenPRTextOrActor_When_Serialized(t *testing.T) {
	tr, clock, _ := newTestTracker()
	tr.SetSettings(Settings{
		HumanFeedbackMode: ModeAllowlist, ActorAllowlist: []string{"hostile-actor"},
		BotActorAllowlist: []string{"evil[bot]"},
	}, nil)
	tr.SetSessions([]SessionRef{{UUID: uuidA, Title: "t", Repo: "o/a", PR: 1, State: StateLive}})
	tr.Record(Disposition{At: clock.Now(), Kind: KindComment, Repo: "o/a", PR: 1, SessionUUID: uuidA,
		ActorClass: ActorOther, Disposition: DispositionSteered})
	raw, _ := json.Marshal(tr.Snapshot(nil))
	for _, banned := range []string{"hostile-actor", "evil[bot]", "SENTINEL-TOKEN", "HOSTILE SUMMARY"} {
		if strings.Contains(string(raw), banned) {
			t.Fatalf("status leaked %q: %s", banned, raw)
		}
	}
	for _, b := range tr.ring.Entries() {
		raw, _ := json.Marshal(b)
		if strings.Contains(string(raw), "hostile-actor") {
			t.Fatal("ring buffer leaked actor")
		}
	}
}

func TestStatus_should_ShowEffectiveSettingsAndEveryCorrectedOrIgnoredKey_When_ConfigWasAdjusted(t *testing.T) {
	tr, _, _ := newTestTracker()
	tr.SetSettings(Settings{HumanFeedbackMode: ModeAllowlist, PollInterval: 5 * time.Second, MinSteerInterval: time.Minute},
		[]SettingCorrection{
			{Key: "poll_interval_seconds", Bad: "0", Effective: "5"},
			{Key: "actor_allowlist", Bad: "set", Effective: "ignored"},
		})
	st := tr.Snapshot(nil)
	if !st.Settings.CIOnly || !strings.Contains(st.Settings.ModeDescription, "CI failures only") {
		t.Fatalf("empty allowlist must read as CI-only: %+v", st.Settings)
	}
	if len(st.Settings.Corrections) != 2 || st.Settings.Corrections[0].Bad != "0" || st.Settings.Corrections[0].Effective != "5" {
		t.Fatalf("corrections wrong: %+v", st.Settings.Corrections)
	}
	tr.SetSettings(Settings{HumanFeedbackMode: ModeAll}, nil)
	clean := tr.Snapshot(nil).Settings
	if len(clean.Corrections) != 0 || clean.CIOnly {
		t.Fatalf("clean config wrong: %+v", clean)
	}
}

func TestStatus_should_ShowPositiveFirstRunLine_When_SessionsAreWatched(t *testing.T) {
	tr, _, _ := newTestTracker()
	if got := tr.Snapshot(nil).FirstRunLine; got != "Not watching any PRs." {
		t.Fatalf("empty line = %q", got)
	}
	tr.SetSessions([]SessionRef{
		{UUID: uuidA, Repo: "o/a", PR: 1, State: StateLive},
		{UUID: uuidB, Repo: "o/b", PR: 2, State: StateLive},
		{UUID: uuidC, Repo: "o/c", PR: 3, State: StateSuspended},
	})
	tr.SetCoverage("o/a", CoverageStateCovered, "")
	tr.SetCoverage("o/b", CoverageStateCovered, "")
	tr.SetCoverage("o/c", CoverageStateUnavailable, "no admin")
	want := "Watching 3 PRs in 3 repositories; 1 repository not covered."
	if got := tr.Snapshot(nil).FirstRunLine; got != want {
		t.Fatalf("line = %q, want %q", got, want)
	}
}

func TestStatus_should_ShowSanitizedTitleBesideShortUUIDOnStatusOnly_When_TitleHostile(t *testing.T) {
	tr, _, _ := newTestTracker()
	hostile := "\x1b[31mred\x1b[0m\nline two\r\x07" + strings.Repeat("x", 200)
	tr.SetSessions([]SessionRef{{UUID: "0123456789abcdef", Title: hostile, Repo: "o/a", PR: 1, State: StateLive}})
	w := tr.Snapshot(nil).Watched[0]
	if w.ShortUUID != "01234567" {
		t.Fatalf("short uuid = %q", w.ShortUUID)
	}
	if len([]rune(w.Title)) != 40 || !strings.HasPrefix(w.Title, "red line two x") {
		t.Fatalf("title = %q", w.Title)
	}
	for _, r := range w.Title {
		if r < 0x20 || r == 0x7f {
			t.Fatalf("control char in title %q", w.Title)
		}
	}
}

func TestStatus_should_ReportPerRepoCoverageState_When_CoveredUnavailableAndUnknown(t *testing.T) {
	tr, _, _ := newTestTracker()
	tr.SetSessions([]SessionRef{
		{UUID: uuidA, Repo: "o/a", PR: 1, State: StateLive},
		{UUID: uuidB, Repo: "o/b", PR: 2, State: StateLive},
	})
	tr.SetCoverage("o/a", CoverageStateCovered, "")
	tr.SetCoverage("o/b", CoverageStateUnavailable, "token lacks \x1b[1madmin")
	st := tr.Snapshot(nil)
	if st.Repos[0].Coverage != CoverageStateCovered || st.Repos[1].Coverage != CoverageStateUnavailable {
		t.Fatalf("repos = %+v", st.Repos)
	}
	if strings.Contains(st.Repos[1].Reason, "\x1b") || st.Repos[1].Reason != "token lacks admin" {
		t.Fatalf("reason = %q", st.Repos[1].Reason)
	}
	if st.CoverageFraction == nil || *st.CoverageFraction != 0.5 {
		t.Fatalf("fraction = %v", st.CoverageFraction)
	}
	// Per-session markers sum to the watched total.
	sum := 0
	for _, n := range st.Counts.CoverageMarkers {
		sum += n
	}
	if sum != st.Counts.Watched {
		t.Fatalf("markers %d != watched %d", sum, st.Counts.Watched)
	}

	tr.SetSessions([]SessionRef{{UUID: uuidA, Repo: "o/a", PR: 1, State: StateLive}, {UUID: uuidB, Repo: "o/never", PR: 2, State: StateLive}})
	st = tr.Snapshot(nil)
	if st.CoverageFraction != nil {
		t.Fatal("fraction must be not computable with an unknown repo")
	}
	if st.Watched[1].Coverage != CoverageStateUnknown || !strings.Contains(st.FirstRunLine, "not yet confirmed") {
		t.Fatalf("unknown handling wrong: %+v / %q", st.Watched, st.FirstRunLine)
	}
}

func TestStatus_should_ReportNextEligibleSteerTimePerSession_When_WindowOpenOrClosed(t *testing.T) {
	tr, clock, _ := newTestTracker()
	t0 := clock.Now()
	closes := t0.Add(5 * time.Minute)
	hf := CoalesceKey{SessionUUID: uuidA, Repo: "o/a", PR: 1, Class: ClassHumanFeedback}
	co := &fakeCoalescer{next: map[CoalesceKey]time.Time{hf: closes}}
	tr.SetSessions([]SessionRef{
		{UUID: uuidA, Repo: "o/a", PR: 1, State: StateLive},
		{UUID: uuidB, Repo: "o/b", PR: 2, State: StateLive},
	})
	tr.SetHeld([]HeldItem{{SessionUUID: uuidA, Repo: "o/a", PR: 1, Kind: KindComment, Reason: HoldWindowBatching, Since: t0}})

	st := tr.Snapshot(co)
	a, b := st.Watched[0], st.Watched[1]
	if a.NextSteer != NextSteerAt || a.NextSteerAt == nil || !a.NextSteerAt.Equal(closes) {
		t.Fatalf("A = %+v", a)
	}
	if b.NextSteer != NextSteerNow || b.NextSteerAt != nil {
		t.Fatalf("B = %+v", b)
	}
	if st.Held[0].WindowClosesAt == nil || !st.Held[0].WindowClosesAt.Equal(closes) {
		t.Fatalf("held window = %+v", st.Held[0])
	}

	clock.Advance(6 * time.Minute) // window passed; item still held -> waiting, not "now"
	a = tr.Snapshot(co).Watched[0]
	if a.NextSteer != NextSteerWaiting {
		t.Fatalf("A after window = %+v", a)
	}
	tr.SetHeld(nil)
	if a = tr.Snapshot(co).Watched[0]; a.NextSteer != NextSteerNow {
		t.Fatalf("A with nothing held = %+v", a)
	}
}

func TestStatus_should_CountStrangerSteersByActorClass_And_NeverContainALogin_When_EventsDisposed(t *testing.T) {
	tr, clock, _ := newTestTracker()
	classes := []ActorClass{ActorAllowlisted, ActorOther, ActorOther, ActorBot, "", ActorSelf}
	for i, c := range classes {
		tr.Record(Disposition{At: clock.Now(), Kind: KindComment, Repo: "o/a", PR: i, ActorClass: c, Disposition: DispositionSteered})
	}
	tr.Record(Disposition{At: clock.Now(), Disposition: DispositionDropped, Reason: string(DropBotActor)})
	st := tr.Snapshot(nil)
	if st.Counts.SteersByClass["other"] != 2 || st.Counts.SteersByClass["ci"] != 1 || st.Counts.SteersByClass["bot"] != 1 {
		t.Fatalf("by class = %v", st.Counts.SteersByClass)
	}
	sum := 0
	for _, n := range st.Counts.SteersByClass {
		sum += n
	}
	if sum != st.Counts.Steers || sum != len(classes) {
		t.Fatalf("class sum %d, total %d, want %d", sum, st.Counts.Steers, len(classes))
	}
	if len(st.Recent) != len(classes)+1 {
		t.Fatalf("recent = %d", len(st.Recent))
	}
}

func TestStatus_should_ExposeCountersIncludingCatchupOverrunAndDuplicate_When_Incremented(t *testing.T) {
	tr, _, cs := newTestTracker()
	cs.Inc(MetricCatchupSteers, nil)
	cs.Inc(MetricTickOverrun, nil)
	cs.Inc(MetricTickOverrun, nil)
	cs.Inc(MetricDuplicateSharedGuard, nil)
	cs.Inc(MetricCISteerInsideFeedbackWindow, nil)
	c := tr.Snapshot(nil).Counts
	if c.CatchupSteers != 1 || c.TickOverruns != 2 || c.SharedGuardDupes != 1 || c.CISteersInWindow != 1 {
		t.Fatalf("counts = %+v", c)
	}
}

func TestMetrics_should_CountPerLabelSetAndTotalAndBeConcurrencySafe_When_Incremented(t *testing.T) {
	cs := NewCounterSet()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs.Inc(MetricDropped, map[string]string{"drop_reason": string(DropSelfActor)})
			cs.Inc(MetricDropped, map[string]string{"drop_reason": string(DropBotActor)})
		}()
	}
	wg.Wait()
	if got := cs.Value(MetricDropped, map[string]string{"drop_reason": "self_actor"}); got != 50 {
		t.Fatalf("self_actor = %d", got)
	}
	if got := cs.Total(MetricDropped); got != 100 {
		t.Fatalf("total = %d", got)
	}
	if cs.Total(MetricDropped+"x") != 0 {
		t.Fatal("prefix leak")
	}
	if CounterKey("n", map[string]string{"b": "2", "a": "1"}) != "n{a=1,b=2}" {
		t.Fatal("key not sorted")
	}
}
