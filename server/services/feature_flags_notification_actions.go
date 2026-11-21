package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session/domain"
)

// feature_flags_notification_actions.go registers the four rollout flags of the
// notification-actions-and-agent-messaging project (ADR-008) and holds the one
// caller-side helper that applies the dependency order over the existing
// config.GetFeatureFlagScoped resolver.

const (
	backgroundActivityFeedFlagName    = "background_activity_feed"
	notificationInlineActionsFlagName = "notification_inline_actions"
	askUserQuestionStructuredFlagName = "askuserquestion_structured"
	agentWakeTriggersFlagName         = "agent_wake_triggers"
)

// Decision points (scopes) of the flags. Prefixes are the scope vocabulary.
const (
	scopeStageRender    = "stage:render"
	scopeStageAnswer    = "stage:answer"
	scopeStageHookReply = "stage:hook_reply"
	scopeChannelRemote  = "channel:remote"
	scopeChannelTierB   = "channel:remote:tierB"

	scopePrefixAction  = "action:"
	scopePrefixTrigger = "trigger:"
	scopePrefixTool    = "tool:"

	actionKindAnswerQuestion = "ANSWER_QUESTION"
)

// The nominal flag defaults, also the registry defaultValue entries.
const backgroundActivityFeedDefault = true

// MCP tools and inline action kinds accepted as scopes (plan Data Model).
var (
	wakeToolScopes        = []string{"schedule_wake", "list_wakes", "cancel_wake", "request_help", "request_owner_action"}
	inlineActionKindNames = []string{"APPROVE", "DENY", actionKindAnswerQuestion, "BACKLOG_DECISION", "ANSWER_GUIDANCE", "CANCEL_WAKE", "CANCEL_PEER_HELP"}
)

func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}

func notificationInlineActionScopes() []string {
	return append([]string{scopeChannelRemote, scopeChannelTierB}, prefixed(scopePrefixAction, inlineActionKindNames)...)
}

func askUserQuestionStructuredScopes() []string {
	return []string{scopeStageRender, scopeStageAnswer, scopeStageHookReply}
}

// wakeTriggerTypeScopes derives the trigger scope names from the domain's closed
// set, so a new wake trigger type gets its `trigger:<name>` scope automatically.
func wakeTriggerTypeScopes() []string {
	types := domain.AllWakeTriggerTypes()
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return names
}

func agentWakeTriggerScopes() []string {
	return append(prefixed(scopePrefixTrigger, wakeTriggerTypeScopes()), prefixed(scopePrefixTool, wakeToolScopes)...)
}

// Spike evidence compiled in until it is recorded (ADR-008): a gate that needs a
// spike result stays closed while the constant is false.
const (
	// spikeS1HookReplyPassed gates stage:hook_reply; flip when S1 PASS is recorded.
	spikeS1HookReplyPassed = false
	// spikeS6TierBPassed gates channel:remote:tierB: S6 PASS (authenticated remote chain,
	// research/spike-S6.md) plus the Story 2.8 remote-chain test. The scope itself stays opt-in.
	spikeS6TierBPassed = true
)

// flagReader resolves flag decisions against an injected registry. A nil
// registry is a programming error: registry() panics rather than guessing defaults.
type flagReader struct{ reg *featureflags.Registry }

func (r flagReader) registry() *featureflags.Registry {
	if r.reg == nil {
		panic("services: flagReader has no registry; inject one from the composition root")
	}
	return r.reg
}

// enabled resolves name against cfg with the injected registry's default.
func (r flagReader) enabled(cfg *config.Config, name string) bool {
	return r.registry().Enabled(cfg, name)
}

// liveFlag is a flag's live global value. The zero value is OFF (fail closed).
type liveFlag struct{ on atomic.Bool }

// liveFlagController applies a global flip in-process, without a restart.
type liveFlagController struct{ flag *liveFlag }

func (c liveFlagController) Enable(context.Context) error { c.flag.on.Store(true); return nil }
func (c liveFlagController) Disable() error               { c.flag.on.Store(false); return nil }
func (c liveFlagController) IsEnabled() bool              { return c.flag != nil && c.flag.on.Load() }

// One controller type per flag (following replyFlagController) so a wired flag
// is identifiable in a type switch and in logs.
type (
	backgroundActivityFeedFlagController    struct{ liveFlagController }
	notificationInlineActionsFlagController struct{ liveFlagController }
	askUserQuestionStructuredFlagController struct{ liveFlagController }
	agentWakeTriggersFlagController         struct{ liveFlagController }
)

// notificationActionsFlagAuditPolicy: turning a flag on loosens (it opens a new
// path, or re-enables a kill-switched one), so the flip is audit-fsynced and
// refused while the sink is down; turning it off is persisted first and queued.
var notificationActionsFlagAuditPolicy = featureflags.FlagAuditPolicy{
	Loosening: func(enabled bool) bool { return enabled },
}

// notificationActionsFlagAuditPolicies is merged into the service's audit wiring.
func notificationActionsFlagAuditPolicies() map[string]featureflags.FlagAuditPolicy {
	return map[string]featureflags.FlagAuditPolicy{
		backgroundActivityFeedFlagName:    notificationActionsFlagAuditPolicy,
		notificationInlineActionsFlagName: notificationActionsFlagAuditPolicy,
		askUserQuestionStructuredFlagName: notificationActionsFlagAuditPolicy,
		agentWakeTriggersFlagName:         notificationActionsFlagAuditPolicy,
	}
}

const backgroundActivityFeedOffDetail = "Background activity feed is OFF (legacy history-only selector); turning it back ON is audit-fsynced and refused while the audit sink is down"

// startupFlagValue reads a persisted global value strictly: no config file is
// the default, an explicit value is that value, and an unreadable config is OFF.
func startupFlagValue(name string, def bool) bool {
	dir, err := config.GetConfigDir()
	if err != nil {
		return false
	}
	cfg, err := config.LoadConfigFromPath(filepath.Join(dir, config.ConfigFileName))
	switch {
	case err == nil:
		return cfg.GetFeatureFlagWithDefault(name, def)
	case errors.Is(err, os.ErrNotExist):
		return def
	default:
		return false
	}
}

// wireNotificationActionFlags connects the four flags to live controllers and
// their status_detail. Decision points read the config at decision time via
// effectiveFlag, so a scoped flip needs no controller.
func wireNotificationActionFlags(ff *FeatureFlagService) {
	reg := ff.reg()
	wire := func(name string, ctrl func(liveFlagController) featureflags.FeatureController, offDetail string) {
		f := &liveFlag{}
		f.on.Store(startupFlagValue(name, reg.Default(name)))
		ff.SetFeatureController(name, ctrl(liveFlagController{flag: f}))
		ff.AddStatusDetailSource(name, func() string {
			if f.on.Load() {
				return ""
			}
			return offDetail
		})
	}
	wire(backgroundActivityFeedFlagName, func(c liveFlagController) featureflags.FeatureController {
		return backgroundActivityFeedFlagController{c}
	}, backgroundActivityFeedOffDetail)
	wire(notificationInlineActionsFlagName, func(c liveFlagController) featureflags.FeatureController {
		return notificationInlineActionsFlagController{c}
	}, "Inline notification actions are OFF globally; per-action scopes can still enable a decision point")
	wire(askUserQuestionStructuredFlagName, func(c liveFlagController) featureflags.FeatureController {
		return askUserQuestionStructuredFlagController{c}
	}, "Structured AskUserQuestion is OFF globally; per-stage scopes can still enable a stage")
	wire(agentWakeTriggersFlagName, func(c liveFlagController) featureflags.FeatureController { return agentWakeTriggersFlagController{c} }, "Agent wake triggers are OFF globally; per-trigger scopes can still enable a trigger type")
}

// flagOffMessage is the operator copy for the FLAG_OFF action outcome (UX-AC55,
// UX-AC57); the web counterpart is flagOffMessage in useFeatureFlagValue.ts.
const flagOffMessage = "This action is turned off. Open session."

// answerRoute is where an ANSWER_QUESTION write goes; it picks the route gate.
type answerRoute int

const (
	// routeLiveKeystrokeHidden is a live_keystroke write into a hidden session.
	routeLiveKeystrokeHidden answerRoute = iota
	// routeLiveKeystrokeVisible is a live_keystroke write into a visible session.
	routeLiveKeystrokeVisible
	// routeDurableRecord is a stored answer (resume, stopped session, agent_mcp row).
	routeDurableRecord
	// routeHookReply is a hook_reply write.
	routeHookReply
)

// flagEvidence carries the inputs besides config the matrix needs.
type flagEvidence struct {
	s1HookReplyPass bool
	s6TierBPass     bool
}

var compiledFlagEvidence = flagEvidence{s1HookReplyPass: spikeS1HookReplyPassed, s6TierBPass: spikeS6TierBPassed}

// effectiveFlag resolves one decision point of name against the live config,
// applying the dependency order of the plan's flag matrix (ADR-008).
func (r flagReader) effectiveFlag(name, decisionPoint string) bool {
	return r.resolveEffectiveFlag(config.LoadConfig(), compiledFlagEvidence, name, decisionPoint)
}

// effectiveAnswerQuestion resolves the three-flag ANSWER_QUESTION composition.
func (r flagReader) effectiveAnswerQuestion(route answerRoute) bool {
	return r.resolveAnswerQuestion(config.LoadConfig(), compiledFlagEvidence, route)
}

func (r flagReader) scoped(cfg *config.Config, name, scope string) bool {
	return r.registry().EnabledScoped(cfg, name, scope)
}

// resolveEffectiveFlag is effectiveFlag over an injected config and evidence.
// An unknown decision point of a known flag resolves false (fail closed).
func (r flagReader) resolveEffectiveFlag(cfg *config.Config, ev flagEvidence, name, dp string) bool {
	switch name {
	case backgroundActivityFeedFlagName:
		return cfg.GetFeatureFlagWithDefault(name, backgroundActivityFeedDefault)
	case askUserQuestionStructuredFlagName:
		return r.resolveStage(cfg, ev, dp)
	case notificationInlineActionsFlagName:
		switch {
		case dp == scopePrefixAction+"OPEN":
			return true
		case dp == scopeChannelTierB:
			return r.scoped(cfg, name, dp) && r.scoped(cfg, name, scopeChannelRemote) && ev.s6TierBPass
		case dp == scopeChannelRemote, strings.HasPrefix(dp, scopePrefixAction):
			return r.scoped(cfg, name, dp)
		}
	case agentWakeTriggersFlagName:
		if dp == "dispatcher" {
			return r.dispatcherRunning(cfg)
		}
		if strings.HasPrefix(dp, scopePrefixTrigger) || strings.HasPrefix(dp, scopePrefixTool) {
			return r.scoped(cfg, name, dp) && r.dispatcherRunning(cfg)
		}
	}
	return false
}

func (r flagReader) resolveStage(cfg *config.Config, ev flagEvidence, dp string) bool {
	const name = askUserQuestionStructuredFlagName
	switch dp {
	case scopeStageRender:
		return r.scoped(cfg, name, scopeStageRender)
	case scopeStageAnswer:
		return r.scoped(cfg, name, scopeStageAnswer) && r.resolveStage(cfg, ev, scopeStageRender)
	case scopeStageHookReply:
		return r.scoped(cfg, name, scopeStageHookReply) && r.resolveStage(cfg, ev, scopeStageAnswer) && ev.s1HookReplyPass
	}
	return false
}

// dispatcherRunning: the global value, or any trigger:<type> explicitly true.
func (r flagReader) dispatcherRunning(cfg *config.Config) bool {
	const name = agentWakeTriggersFlagName
	if r.registry().Enabled(cfg, name) {
		return true
	}
	for scope, v := range cfg.FeatureFlagScopeOverrides(name) {
		if v && strings.HasPrefix(scope, scopePrefixTrigger) {
			return true
		}
	}
	return false
}

func (r flagReader) resolveAnswerQuestion(cfg *config.Config, ev flagEvidence, route answerRoute) bool {
	if !r.resolveEffectiveFlag(cfg, ev, notificationInlineActionsFlagName, scopePrefixAction+actionKindAnswerQuestion) {
		return false
	}
	switch route {
	case routeLiveKeystrokeHidden:
		return r.registry().Enabled(cfg, hiddenSessionReplyFlagName)
	case routeLiveKeystrokeVisible, routeDurableRecord:
		return r.resolveStage(cfg, ev, scopeStageAnswer)
	case routeHookReply:
		return r.resolveStage(cfg, ev, scopeStageHookReply)
	}
	return false
}

// wakeFlagListener is told after every agent_wake_triggers flip (global or
// scoped) so the wake dispatcher starts, stops or re-checks held rows at once
// instead of at the next restart. The server sets it while it runs.
var wakeFlagListener atomic.Pointer[func()]

// SetWakeFlagListener registers fn; nil clears it.
func SetWakeFlagListener(fn func()) {
	if fn == nil {
		wakeFlagListener.Store(nil)
		return
	}
	wakeFlagListener.Store(&fn)
}

func notifyWakeFlagChanged() {
	if fn := wakeFlagListener.Load(); fn != nil {
		(*fn)()
	}
}

func (c agentWakeTriggersFlagController) Enable(ctx context.Context) error {
	err := c.liveFlagController.Enable(ctx)
	notifyWakeFlagChanged()
	return err
}

func (c agentWakeTriggersFlagController) Disable() error {
	err := c.liveFlagController.Disable()
	notifyWakeFlagChanged()
	return err
}

// ApplyScope makes a trigger:<type> flip live; the flag is already persisted.
func (c agentWakeTriggersFlagController) ApplyScope(string, bool) error {
	notifyWakeFlagChanged()
	return nil
}
