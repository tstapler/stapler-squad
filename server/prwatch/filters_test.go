package prwatch

import "testing"

type staticLogin struct {
	login string
	ok    bool
}

func (s staticLogin) Cached() (string, bool) { return s.login, s.ok }

var operator = staticLogin{login: "Operator", ok: true}

func settings(mode HumanFeedbackMode, allow, botAllow []string, ignoreBots bool) Settings {
	return Settings{HumanFeedbackMode: mode, ActorAllowlist: allow, BotActorAllowlist: botAllow, IgnoreBotActors: ignoreBots}
}

func TestFilters_should_DropSelfAuthoredEvent_When_ActorEqualsOperatorLoginCaseInsensitively(t *testing.T) {
	f := NewActorFilter(settings(ModeAll, nil, nil, true), operator)
	for _, kind := range []Kind{KindComment, KindReviewComment, KindReview} {
		for _, actor := range []string{"operator", "OPERATOR", "Operator"} {
			r := f.Evaluate(kind, actor)
			if r.Verdict != VerdictDrop || r.Drop != DropSelfActor || r.Class != ActorSelf {
				t.Fatalf("%s/%s: got %+v", kind, actor, r)
			}
		}
	}
}

func TestFilters_should_DropBotButKeepAllowlistedBot_When_CopilotBotInBotActorAllowlist(t *testing.T) {
	f := NewActorFilter(settings(ModeAll, nil, []string{"copilot[bot]"}, true), operator)
	if r := f.Evaluate(KindComment, "dependabot[bot]"); r.Drop != DropBotActor {
		t.Fatalf("dependabot: %+v", r)
	}
	if r := f.Evaluate(KindComment, "Copilot[bot]"); r.Verdict != VerdictSteer {
		t.Fatalf("copilot: %+v", r)
	}
	f = NewActorFilter(settings(ModeAll, nil, []string{"copilot[bot]"}, false), operator)
	if r := f.Evaluate(KindComment, "dependabot[bot]"); r.Verdict != VerdictSteer {
		t.Fatalf("ignore_bot_actors false should keep dependabot: %+v", r)
	}
}

func TestFilters_should_SteerAnyNonSelfNonBotActor_When_ModeIsAll(t *testing.T) {
	f := NewActorFilter(settings(ModeAll, []string{"someone"}, []string{"copilot[bot]"}, true), operator)
	for _, actor := range []string{"stranger", "collab", "copilot[bot]", ""} {
		if r := f.Evaluate(KindReview, actor); r.Verdict != VerdictSteer {
			t.Fatalf("%q: %+v", actor, r)
		}
	}
	if r := f.Evaluate(KindReview, "operator"); r.Drop != DropSelfActor {
		t.Fatalf("self: %+v", r)
	}
	if r := f.Evaluate(KindReview, "ci[bot]"); r.Drop != DropBotActor {
		t.Fatalf("bot: %+v", r)
	}
}

func TestFilters_should_SteerOnlyListedLoginsAndAllowlistedBots_When_ModeIsAllowlist(t *testing.T) {
	f := NewActorFilter(settings(ModeAllowlist, []string{"Teammate", "operator"}, []string{"copilot[bot]"}, true), operator)
	if r := f.Evaluate(KindComment, "teammate"); r.Verdict != VerdictSteer || r.Class != ActorAllowlisted {
		t.Fatalf("teammate: %+v", r)
	}
	if r := f.Evaluate(KindComment, "stranger"); r.Drop != DropActorNotAllowlist {
		t.Fatalf("stranger: %+v", r)
	}
	if r := f.Evaluate(KindComment, ""); r.Drop != DropActorNotAllowlist {
		t.Fatalf("missing actor: %+v", r)
	}
	if r := f.Evaluate(KindComment, "copilot[bot]"); r.Verdict != VerdictSteer {
		t.Fatalf("copilot: %+v", r)
	}
	if r := f.Evaluate(KindComment, "operator"); r.Drop != DropSelfActor {
		t.Fatalf("own login listed: %+v", r)
	}
	g := NewActorFilter(settings(ModeAllowlist, []string{"teammate"}, nil, true), operator)
	if r := g.Evaluate(KindComment, "copilot[bot]"); r.Drop != DropBotActor {
		t.Fatalf("bot not in bot allowlist: %+v", r)
	}
}

func TestFilters_should_DropHumanFeedbackButKeepCI_When_ModeIsOffOrAllowlistIsEmpty(t *testing.T) {
	for _, s := range []Settings{
		settings(ModeOff, []string{"teammate"}, nil, true),
		settings(ModeAllowlist, nil, []string{"copilot[bot]"}, true),
		settings("bogus", nil, nil, true),
	} {
		f := NewActorFilter(s, operator)
		if !f.EffectiveCIOnly() {
			t.Fatalf("%q should be CI-only", s.HumanFeedbackMode)
		}
		for _, kind := range []Kind{KindComment, KindReviewComment, KindReview} {
			if r := f.Evaluate(kind, "teammate"); r.Drop != DropHumanFeedbackOff {
				t.Fatalf("%s: %+v", kind, r)
			}
		}
		if r := f.Evaluate(KindComment, "operator"); r.Drop != DropSelfActor {
			t.Fatalf("self precedes off: %+v", r)
		}
		if r := f.Evaluate(KindCI, "teammate"); r.Verdict != VerdictSteer {
			t.Fatalf("ci: %+v", r)
		}
	}
}

func TestFilters_should_ClassifyActorWithPrecedenceSelfAllowlistedBotOther_When_EventsArrive(t *testing.T) {
	f := NewActorFilter(settings(ModeAll, []string{"operator", "friend"}, []string{"copilot[bot]"}, true), operator)
	cases := map[string]ActorClass{
		"operator":        ActorSelf,
		"friend":          ActorAllowlisted,
		"copilot[bot]":    ActorAllowlisted,
		"dependabot[bot]": ActorBot,
		"stranger":        ActorOther,
		"":                ActorOther,
	}
	for actor, want := range cases {
		if got := f.Evaluate(KindComment, actor).Class; got != want {
			t.Errorf("%q: class %q, want %q", actor, got, want)
		}
	}
	if got := f.Evaluate(KindCI, "stranger").Class; got != "" {
		t.Errorf("ci class %q, want empty", got)
	}
}

func TestFilters_should_AckPolicyDropsAndCountEachReason_When_SelfBotApprovedEventsArrive(t *testing.T) {
	f := NewActorFilter(settings(ModeAllowlist, []string{"teammate"}, nil, true), operator)
	got := map[DropReason]bool{}
	for _, actor := range []string{"operator", "x[bot]", "stranger"} {
		got[f.Evaluate(KindComment, actor).Drop] = true
	}
	for _, want := range []DropReason{DropSelfActor, DropBotActor, DropActorNotAllowlist} {
		if !got[want] {
			t.Errorf("reason %q never produced", want)
		}
	}
}

func TestFilters_should_MakeNoNetworkCall_When_FilteringActors(t *testing.T) {
	// The filter takes only Settings and a cached LoginSource: no port that
	// can reach the network is injected, so a lookup cannot happen.
	f := NewActorFilter(settings(ModeAll, nil, nil, true), operator)
	if r := f.Evaluate(KindComment, "stranger"); r.Verdict != VerdictSteer {
		t.Fatalf("%+v", r)
	}
}
