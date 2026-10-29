package prwatch

import "strings"

// LoginSource reports the operator's cached login. ok is false until a
// non-empty login has resolved (fail closed, D7).
type LoginSource interface {
	Cached() (login string, ok bool)
}

// FilterVerdict is the actor filter's decision for one event.
type FilterVerdict int

const (
	// VerdictSteer: the event passes the actor filters.
	VerdictSteer FilterVerdict = iota
	// VerdictDrop: ack and count FilterResult.Drop.
	VerdictDrop
	// VerdictHold: leave unacked, count FilterResult.Hold.
	VerdictHold
)

// FilterResult is the verdict plus the actor class. Class is empty for CI.
type FilterResult struct {
	Verdict FilterVerdict
	Drop    DropReason
	Hold    HoldReason
	Class   ActorClass
}

// ActorFilter applies the D7 actor rules to comment, review_comment and
// review events. It is pure: config plus the cached login, no network call
// (ADR-001 decision 4). It never logs or retains an actor login.
type ActorFilter struct {
	mode           HumanFeedbackMode
	allow          map[string]struct{}
	botAllow       map[string]struct{}
	ignoreBots     bool
	login          LoginSource
	effectivelyOff bool
}

// NewActorFilter builds the filter from Settings. An unknown mode is off.
func NewActorFilter(s Settings, login LoginSource) *ActorFilter {
	f := &ActorFilter{
		mode:       s.HumanFeedbackMode,
		allow:      lowerSet(s.ActorAllowlist),
		botAllow:   lowerSet(s.BotActorAllowlist),
		ignoreBots: s.IgnoreBotActors,
		login:      login,
	}
	switch f.mode {
	case ModeAll:
	case ModeAllowlist:
		f.effectivelyOff = len(f.allow) == 0
	default:
		f.effectivelyOff = true
	}
	return f
}

// EffectiveCIOnly reports that human-feedback steers are off (mode off or an
// empty allowlist), for the one-time CI-only notice and the status surface.
func (f *ActorFilter) EffectiveCIOnly() bool { return f.effectivelyOff }

// Evaluate decides one event. CI never depends on the actor.
func (f *ActorFilter) Evaluate(kind Kind, actor string) FilterResult {
	if kind.Class() == ClassCI {
		return FilterResult{Verdict: VerdictSteer}
	}
	login, ok := f.login.Cached()
	if !ok {
		return FilterResult{Verdict: VerdictHold, Hold: HoldLoginUnresolved, Class: f.class(actor, "")}
	}
	class := f.class(actor, login)
	key := strings.ToLower(actor)

	switch {
	case class == ActorSelf:
		return FilterResult{Verdict: VerdictDrop, Drop: DropSelfActor, Class: class}
	case f.effectivelyOff:
		return FilterResult{Verdict: VerdictDrop, Drop: DropHumanFeedbackOff, Class: class}
	case isBot(actor) && f.ignoreBots && !inSet(f.botAllow, key):
		return FilterResult{Verdict: VerdictDrop, Drop: DropBotActor, Class: class}
	case f.mode == ModeAllowlist && !inSet(f.allow, key) && !inSet(f.botAllow, key):
		return FilterResult{Verdict: VerdictDrop, Drop: DropActorNotAllowlist, Class: class}
	}
	return FilterResult{Verdict: VerdictSteer, Class: class}
}

// class computes actor_class. Precedence: self, allowlisted, bot, other. An
// allowlisted bot is allowlisted, not bot (ADR-001; validation R43). A
// missing actor is other. login "" means unresolved, so self is not decided.
func (f *ActorFilter) class(actor, login string) ActorClass {
	if actor == "" {
		return ActorOther
	}
	key := strings.ToLower(actor)
	switch {
	case login != "" && strings.EqualFold(actor, login):
		return ActorSelf
	case inSet(f.allow, key), inSet(f.botAllow, key):
		return ActorAllowlisted
	case isBot(actor):
		return ActorBot
	}
	return ActorOther
}

func isBot(actor string) bool { return strings.HasSuffix(strings.ToLower(actor), "[bot]") }

func inSet(m map[string]struct{}, key string) bool {
	_, ok := m[key]
	return ok
}

func lowerSet(in []string) map[string]struct{} {
	m := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			m[s] = struct{}{}
		}
	}
	return m
}
