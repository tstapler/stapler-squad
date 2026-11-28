package flagsvc

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
	BackgroundActivityFeedFlagName    = "background_activity_feed"
	NotificationInlineActionsFlagName = "notification_inline_actions"
	AskUserQuestionStructuredFlagName = "askuserquestion_structured"
	AgentWakeTriggersFlagName         = "agent_wake_triggers"
)

// Decision points (scopes) of the flags. Prefixes are the scope vocabulary.
const (
	ScopeStageRender    = "stage:render"
	ScopeStageAnswer    = "stage:answer"
	ScopeStageHookReply = "stage:hook_reply"
	ScopeChannelRemote  = "channel:remote"
	ScopeChannelTierB   = "channel:remote:tierB"

	ScopePrefixAction  = "action:"
	ScopePrefixTrigger = "trigger:"
	ScopePrefixTool    = "tool:"

	ActionKindAnswerQuestion = "ANSWER_QUESTION"
)

// The nominal flag defaults, also the registry defaultValue entries.
const backgroundActivityFeedDefault = true

// MCP tools and inline action kinds accepted as scopes (plan Data Model).
var (
	wakeToolScopes        = []string{"schedule_wake", "list_wakes", "cancel_wake", "request_help", "request_owner_action"}
	inlineActionKindNames = []string{"APPROVE", "DENY", ActionKindAnswerQuestion, "BACKLOG_DECISION", "ANSWER_GUIDANCE", "CANCEL_WAKE", "CANCEL_PEER_HELP"}
)

func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}

func notificationInlineActionScopes() []string {
	return append([]string{ScopeChannelRemote, ScopeChannelTierB}, prefixed(ScopePrefixAction, inlineActionKindNames)...)
}

func askUserQuestionStructuredScopes() []string {
	return []string{ScopeStageRender, ScopeStageAnswer, ScopeStageHookReply}
}

// agentWakeTriggerScopes derives the trigger scopes from the domain's closed set,
// so a new wake trigger type gets its `trigger:<name>` scope automatically.
func agentWakeTriggerScopes() []string {
	return append(prefixed(ScopePrefixTrigger, domain.AllWakeTriggerTypeNames()), prefixed(ScopePrefixTool, wakeToolScopes)...)
}

// Spike evidence compiled in until it is recorded (ADR-008): a gate that needs a
// spike result stays closed while the constant is false.
const (
	// SpikeS1HookReplyPassed gates stage:hook_reply; flip when S1 PASS is recorded.
	SpikeS1HookReplyPassed = false
	// SpikeS6TierBPassed gates channel:remote:tierB: S6 PASS (authenticated remote chain,
	// research/spike-S6.md) plus the Story 2.8 remote-chain test. The scope itself stays opt-in.
	SpikeS6TierBPassed = true
)

// FlagReader resolves flag decisions against an injected registry. A nil
// registry is a programming error: registry() panics rather than guessing defaults.
type FlagReader struct{ Reg *featureflags.Registry }

func (r FlagReader) registry() *featureflags.Registry {
	if r.Reg == nil {
		panic("services: flagReader has no registry; inject one from the composition root")
	}
	return r.Reg
}

// Enabled resolves name against cfg with the injected registry's default.
func (r FlagReader) Enabled(cfg *config.Config, name string) bool {
	return r.registry().Enabled(cfg, name)
}

// SddDefaultPipelineEnabled reads the flag with its registered default, so an
// unset flag resolves the same way the Features page reports it.
func SddDefaultPipelineEnabled(reg *featureflags.Registry) bool {
	return FlagReader{Reg: reg}.Enabled(config.LoadConfig(), featureflags.SddDefaultPipelineFlagName)
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

// NotificationActionsFlagAuditPolicies is merged into the service's audit wiring.
func NotificationActionsFlagAuditPolicies() map[string]featureflags.FlagAuditPolicy {
	return map[string]featureflags.FlagAuditPolicy{
		BackgroundActivityFeedFlagName:    notificationActionsFlagAuditPolicy,
		NotificationInlineActionsFlagName: notificationActionsFlagAuditPolicy,
		AskUserQuestionStructuredFlagName: notificationActionsFlagAuditPolicy,
		AgentWakeTriggersFlagName:         notificationActionsFlagAuditPolicy,
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

// WireNotificationActionFlags connects the four flags to live controllers and
// their status_detail. Decision points read the config at decision time via
// effectiveFlag, so a scoped flip needs no controller.
func WireNotificationActionFlags(ff *FeatureFlagService) {
	reg := ff.Registry()
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
	wire(BackgroundActivityFeedFlagName, func(c liveFlagController) featureflags.FeatureController {
		return backgroundActivityFeedFlagController{c}
	}, backgroundActivityFeedOffDetail)
	wire(NotificationInlineActionsFlagName, func(c liveFlagController) featureflags.FeatureController {
		return notificationInlineActionsFlagController{c}
	}, "Inline notification actions are OFF globally; per-action scopes can still enable a decision point")
	wire(AskUserQuestionStructuredFlagName, func(c liveFlagController) featureflags.FeatureController {
		return askUserQuestionStructuredFlagController{c}
	}, "Structured AskUserQuestion is OFF globally; per-stage scopes can still enable a stage")
	wire(AgentWakeTriggersFlagName, func(c liveFlagController) featureflags.FeatureController { return agentWakeTriggersFlagController{c} }, "Agent wake triggers are OFF globally; per-trigger scopes can still enable a trigger type")
}

// FlagOffMessage is the operator copy for the FLAG_OFF action outcome (UX-AC55,
// UX-AC57); the web counterpart is flagOffMessage in useFeatureFlagValue.ts.
const FlagOffMessage = "This action is turned off. Open session."

// AnswerRoute is where an ANSWER_QUESTION write goes; it picks the route gate.
type AnswerRoute int

const (
	// routeLiveKeystrokeHidden is a live_keystroke write into a hidden session.
	RouteLiveKeystrokeHidden AnswerRoute = iota
	// routeLiveKeystrokeVisible is a live_keystroke write into a visible session.
	RouteLiveKeystrokeVisible
	// routeDurableRecord is a stored answer (resume, stopped session, agent_mcp row).
	RouteDurableRecord
	// routeHookReply is a hook_reply write.
	RouteHookReply
)

// FlagEvidence carries the inputs besides config the matrix needs.
type FlagEvidence struct {
	S1HookReplyPass bool
	S6TierBPass     bool
}

var CompiledFlagEvidence = FlagEvidence{S1HookReplyPass: SpikeS1HookReplyPassed, S6TierBPass: SpikeS6TierBPassed}

// EffectiveFlag resolves one decision point of name against the live config,
// applying the dependency order of the plan's flag matrix (ADR-008).
func (r FlagReader) EffectiveFlag(name, decisionPoint string) bool {
	return r.ResolveEffectiveFlag(config.LoadConfig(), CompiledFlagEvidence, name, decisionPoint)
}

// EffectiveAnswerQuestion resolves the three-flag ANSWER_QUESTION composition.
func (r FlagReader) EffectiveAnswerQuestion(route AnswerRoute) bool {
	return r.ResolveAnswerQuestion(config.LoadConfig(), CompiledFlagEvidence, route)
}

func (r FlagReader) scoped(cfg *config.Config, name, scope string) bool {
	return r.registry().EnabledScoped(cfg, name, scope)
}

// ResolveEffectiveFlag is effectiveFlag over an injected config and evidence.
// An unknown decision point of a known flag resolves false (fail closed).
func (r FlagReader) ResolveEffectiveFlag(cfg *config.Config, ev FlagEvidence, name, dp string) bool {
	switch name {
	case BackgroundActivityFeedFlagName:
		return cfg.GetFeatureFlagWithDefault(name, backgroundActivityFeedDefault)
	case AskUserQuestionStructuredFlagName:
		return r.resolveStage(cfg, ev, dp)
	case NotificationInlineActionsFlagName:
		switch {
		case dp == ScopePrefixAction+"OPEN":
			return true
		case dp == ScopeChannelTierB:
			return r.scoped(cfg, name, dp) && r.scoped(cfg, name, ScopeChannelRemote) && ev.S6TierBPass
		case dp == ScopeChannelRemote, strings.HasPrefix(dp, ScopePrefixAction):
			return r.scoped(cfg, name, dp)
		}
	case AgentWakeTriggersFlagName:
		if dp == "dispatcher" {
			return r.dispatcherRunning(cfg)
		}
		if strings.HasPrefix(dp, ScopePrefixTrigger) || strings.HasPrefix(dp, ScopePrefixTool) {
			return r.scoped(cfg, name, dp) && r.dispatcherRunning(cfg)
		}
	}
	return false
}

func (r FlagReader) resolveStage(cfg *config.Config, ev FlagEvidence, dp string) bool {
	const name = AskUserQuestionStructuredFlagName
	switch dp {
	case ScopeStageRender:
		return r.scoped(cfg, name, ScopeStageRender)
	case ScopeStageAnswer:
		return r.scoped(cfg, name, ScopeStageAnswer) && r.resolveStage(cfg, ev, ScopeStageRender)
	case ScopeStageHookReply:
		return r.scoped(cfg, name, ScopeStageHookReply) && r.resolveStage(cfg, ev, ScopeStageAnswer) && ev.S1HookReplyPass
	}
	return false
}

// dispatcherRunning: the global value, or any trigger:<type> explicitly true.
func (r FlagReader) dispatcherRunning(cfg *config.Config) bool {
	const name = AgentWakeTriggersFlagName
	if r.registry().Enabled(cfg, name) {
		return true
	}
	for scope, v := range cfg.FeatureFlagScopeOverrides(name) {
		if v && strings.HasPrefix(scope, ScopePrefixTrigger) {
			return true
		}
	}
	return false
}

func (r FlagReader) ResolveAnswerQuestion(cfg *config.Config, ev FlagEvidence, route AnswerRoute) bool {
	if !r.ResolveEffectiveFlag(cfg, ev, NotificationInlineActionsFlagName, ScopePrefixAction+ActionKindAnswerQuestion) {
		return false
	}
	switch route {
	case RouteLiveKeystrokeHidden:
		return r.registry().Enabled(cfg, featureflags.HiddenSessionReplyFlagName)
	case RouteLiveKeystrokeVisible, RouteDurableRecord:
		return r.resolveStage(cfg, ev, ScopeStageAnswer)
	case RouteHookReply:
		return r.resolveStage(cfg, ev, ScopeStageHookReply)
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
