package config

import (
	"fmt"
	"strings"
)

// FeatureInboxPRWatch gates the PR-watch inbox consumer (default off).
const FeatureInboxPRWatch = "inbox_pr_watch"

// Human feedback modes (ADR-001 decision 4).
const (
	HumanFeedbackAll       = "all"
	HumanFeedbackAllowlist = "allowlist"
	HumanFeedbackOff       = "off"
)

// DefaultHumanFeedbackMode is the shipped default. It is the only line to
// change (to HumanFeedbackAllowlist) if owner decision 2 is rejected.
const DefaultHumanFeedbackMode = HumanFeedbackAll

// DefaultInboxPollIntervalSeconds is the shipped poll cadence. It is the only
// line to change (to 60) if owner decision 1 (ADR-001 decision 3) is rejected.
const DefaultInboxPollIntervalSeconds = 5

const (
	DefaultInboxURL                 = "http://127.0.0.1:8789"
	DefaultInboxTokenPath           = "~/.local/state/gh-signal/inbox.token"
	DefaultReconcileIntervalSeconds = 60
	MinReconcileIntervalSeconds     = 10
	DefaultCoverageRecheckMinutes   = 10
	DefaultMinSteerIntervalSeconds  = 300
	MaxMinSteerIntervalSeconds      = 1800
	DefaultHeldNoticeAfterMinutes   = 60
	DefaultHeldAgeCeilingHours      = 24
	DefaultStateNotifyGraceSeconds  = 120
	DefaultLeaderRetrySeconds       = 30
	DefaultFollowupObserveSeconds   = 120
	DefaultStrangerNoticeThreshold  = 3
	minInboxPollIntervalSeconds     = 1
	minCoverageRecheckMinutes       = 1
)

// InboxPRWatchConfig is the raw, operator-written half of the settings.
// Pointer fields distinguish "absent" from an explicit 0 or false.
type InboxPRWatchConfig struct {
	URL                     string   `json:"url,omitempty"`
	TokenPath               string   `json:"token_path,omitempty"`
	PollIntervalSeconds     int      `json:"poll_interval_seconds,omitempty"`
	ReconcileIntervalSecs   int      `json:"reconcile_interval_seconds,omitempty"`
	CoverageRecheckMinutes  int      `json:"coverage_recheck_minutes,omitempty"`
	MinSteerIntervalSeconds *int     `json:"min_steer_interval_seconds,omitempty"`
	HeldNoticeAfterMinutes  int      `json:"held_notice_after_minutes,omitempty"`
	HeldAgeCeilingHours     int      `json:"held_age_ceiling_hours,omitempty"`
	StateNotifyGraceSeconds int      `json:"state_notify_grace_seconds,omitempty"`
	LeaderRetrySeconds      int      `json:"leader_retry_seconds,omitempty"`
	FollowupObserveSeconds  int      `json:"followup_observe_seconds,omitempty"`
	IgnoreRepos             []string `json:"ignore_repos,omitempty"`
	IgnoreBotActors         *bool    `json:"ignore_bot_actors,omitempty"`
	BotActorAllowlist       []string `json:"bot_actor_allowlist,omitempty"`
	HumanFeedbackMode       string   `json:"human_feedback_mode,omitempty"`
	ActorAllowlist          []string `json:"actor_allowlist,omitempty"`
	StrangerNoticeThreshold int      `json:"stranger_notice_threshold,omitempty"`
}

// InboxPRWatchSettings is the effective configuration after defaults and clamps.
type InboxPRWatchSettings struct {
	URL                     string
	TokenPath               string
	PollIntervalSeconds     int
	ReconcileIntervalSecs   int
	CoverageRecheckMinutes  int
	MinSteerIntervalSeconds int
	HeldNoticeAfterMinutes  int
	HeldAgeCeilingHours     int
	StateNotifyGraceSeconds int
	LeaderRetrySeconds      int
	FollowupObserveSeconds  int
	IgnoreRepos             []string
	IgnoreBotActors         bool
	BotActorAllowlist       []string
	// HumanFeedbackMode is the configured mode after validation (invalid -> off).
	HumanFeedbackMode string
	// EffectiveHumanFeedbackMode is what the gate applies: allowlist with an
	// empty list is off.
	EffectiveHumanFeedbackMode string
	ActorAllowlist             []string
	StrangerNoticeThreshold    int
}

// InboxPRWatchCorrection records one fallback, clamp or ignored key. Each is
// logged as one WARN at load and listed under "Settings" in the status surface.
type InboxPRWatchCorrection struct {
	Key       string
	Value     string
	Effective string
}

func (c InboxPRWatchCorrection) String() string {
	return fmt.Sprintf("%s=%s -> %s", c.Key, c.Value, c.Effective)
}

// InboxPRWatchConfigKeys returns every key under "inbox_pr_watch" (the operator
// doc completeness test checks the doc against this list).
func InboxPRWatchConfigKeys() []string {
	return []string{
		"url", "token_path", "poll_interval_seconds", "reconcile_interval_seconds",
		"coverage_recheck_minutes", "min_steer_interval_seconds",
		"held_notice_after_minutes", "held_age_ceiling_hours",
		"state_notify_grace_seconds", "leader_retry_seconds",
		"followup_observe_seconds", "ignore_repos", "ignore_bot_actors",
		"bot_actor_allowlist", "human_feedback_mode", "actor_allowlist",
		"stranger_notice_threshold",
	}
}

// ResolveInboxPRWatch applies defaults and clamps to raw and lists every
// correction. It is pure and nil-safe.
func ResolveInboxPRWatch(raw *InboxPRWatchConfig) (InboxPRWatchSettings, []InboxPRWatchCorrection) {
	if raw == nil {
		raw = &InboxPRWatchConfig{}
	}
	var corr []InboxPRWatchCorrection
	note := func(key string, value any, effective any) {
		corr = append(corr, InboxPRWatchCorrection{Key: key, Value: fmt.Sprint(value), Effective: fmt.Sprint(effective)})
	}
	// positive: zero is "unset" (silent default); negative falls back with a note.
	positive := func(key string, v, def, floor int) int {
		switch {
		case v == 0:
			return def
		case v < 0:
			note(key, v, def)
			return def
		case v < floor:
			note(key, v, floor)
			return floor
		}
		return v
	}

	s := InboxPRWatchSettings{
		URL:                     raw.URL,
		TokenPath:               raw.TokenPath,
		PollIntervalSeconds:     positive("poll_interval_seconds", raw.PollIntervalSeconds, DefaultInboxPollIntervalSeconds, minInboxPollIntervalSeconds),
		ReconcileIntervalSecs:   positive("reconcile_interval_seconds", raw.ReconcileIntervalSecs, DefaultReconcileIntervalSeconds, MinReconcileIntervalSeconds),
		CoverageRecheckMinutes:  positive("coverage_recheck_minutes", raw.CoverageRecheckMinutes, DefaultCoverageRecheckMinutes, minCoverageRecheckMinutes),
		HeldNoticeAfterMinutes:  positive("held_notice_after_minutes", raw.HeldNoticeAfterMinutes, DefaultHeldNoticeAfterMinutes, 1),
		HeldAgeCeilingHours:     positive("held_age_ceiling_hours", raw.HeldAgeCeilingHours, DefaultHeldAgeCeilingHours, 1),
		StateNotifyGraceSeconds: positive("state_notify_grace_seconds", raw.StateNotifyGraceSeconds, DefaultStateNotifyGraceSeconds, 1),
		LeaderRetrySeconds:      positive("leader_retry_seconds", raw.LeaderRetrySeconds, DefaultLeaderRetrySeconds, 1),
		FollowupObserveSeconds:  positive("followup_observe_seconds", raw.FollowupObserveSeconds, DefaultFollowupObserveSeconds, 1),
		StrangerNoticeThreshold: positive("stranger_notice_threshold", raw.StrangerNoticeThreshold, DefaultStrangerNoticeThreshold, 1),
		IgnoreBotActors:         true,
		BotActorAllowlist:       cleanLogins(raw.BotActorAllowlist),
		ActorAllowlist:          cleanLogins(raw.ActorAllowlist),
	}
	if s.URL == "" {
		s.URL = DefaultInboxURL
	}
	if s.TokenPath == "" {
		s.TokenPath = DefaultInboxTokenPath
	}
	if raw.IgnoreBotActors != nil {
		s.IgnoreBotActors = *raw.IgnoreBotActors
	}

	s.MinSteerIntervalSeconds = DefaultMinSteerIntervalSeconds
	if p := raw.MinSteerIntervalSeconds; p != nil {
		switch {
		case *p < 0:
			note("min_steer_interval_seconds", *p, DefaultMinSteerIntervalSeconds)
		case *p > MaxMinSteerIntervalSeconds:
			note("min_steer_interval_seconds", *p, MaxMinSteerIntervalSeconds)
			s.MinSteerIntervalSeconds = MaxMinSteerIntervalSeconds
		default:
			s.MinSteerIntervalSeconds = *p // 0 honored: coalescing off
		}
	}

	seen := map[string]bool{}
	for _, r := range raw.IgnoreRepos {
		n := strings.ToLower(strings.TrimSpace(r))
		if !validRepoSlug(n) {
			note("ignore_repos", fmt.Sprintf("%q", r), "dropped")
			continue
		}
		if !seen[n] {
			seen[n] = true
			s.IgnoreRepos = append(s.IgnoreRepos, n)
		}
	}

	mode := strings.ToLower(strings.TrimSpace(raw.HumanFeedbackMode))
	switch mode {
	case "":
		mode = DefaultHumanFeedbackMode
	case HumanFeedbackAll, HumanFeedbackAllowlist, HumanFeedbackOff:
	default:
		note("human_feedback_mode", fmt.Sprintf("%q", raw.HumanFeedbackMode), HumanFeedbackOff)
		mode = HumanFeedbackOff
	}
	s.HumanFeedbackMode = mode
	s.EffectiveHumanFeedbackMode = mode
	switch {
	case mode == HumanFeedbackAllowlist && len(s.ActorAllowlist) == 0:
		note("actor_allowlist", "[]", "mode allowlist behaves as off")
		s.EffectiveHumanFeedbackMode = HumanFeedbackOff
	case mode == HumanFeedbackAll && len(s.ActorAllowlist) > 0:
		note("actor_allowlist", fmt.Sprintf("%d entries", len(s.ActorAllowlist)), "ignored in mode all")
	}
	return s, corr
}

// InboxPRWatchSettings resolves the effective settings, nil-safe.
func (c *Config) InboxPRWatchSettings() (InboxPRWatchSettings, []InboxPRWatchCorrection) {
	if c == nil {
		return ResolveInboxPRWatch(nil)
	}
	return ResolveInboxPRWatch(&c.InboxPRWatch)
}

// LogInboxPRWatchCorrections emits exactly one WARN per correction via warn.
func LogInboxPRWatchCorrections(corr []InboxPRWatchCorrection, warn func(msg string, kv ...any)) {
	for _, c := range corr {
		warn("inbox_pr_watch config corrected", "key", c.Key, "value", c.Value, "effective", c.Effective)
	}
}

// AllowlistContainsLogin reports whether the operator's own login is in the
// allowlist (no effect, self is dropped first); callers warn once it resolves.
func (s InboxPRWatchSettings) AllowlistContainsLogin(login string) bool {
	for _, l := range s.ActorAllowlist {
		if strings.EqualFold(l, login) {
			return true
		}
	}
	return false
}

func cleanLogins(in []string) []string {
	var out []string
	for _, l := range in {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func validRepoSlug(s string) bool {
	owner, name, ok := strings.Cut(s, "/")
	return ok && owner != "" && name != "" && !strings.ContainsAny(s[len(owner)+1:], "/") &&
		!strings.ContainsAny(s, " \t\n")
}
