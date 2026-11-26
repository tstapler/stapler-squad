package flagsvc

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

// tri is an explicit value that may be absent (nil inherits).
type tri *bool

func tv(b bool) tri { return &b }

func matrixCfg(name string, global tri, scopes map[string]tri) *config.Config {
	cfg := &config.Config{}
	if global != nil {
		cfg.FeatureFlags = map[string]bool{name: *global}
	}
	for s, v := range scopes {
		if v == nil {
			continue
		}
		if cfg.FeatureFlagScopes == nil {
			cfg.FeatureFlagScopes = map[string]map[string]bool{}
		}
		if cfg.FeatureFlagScopes[name] == nil {
			cfg.FeatureFlagScopes[name] = map[string]bool{}
		}
		cfg.FeatureFlagScopes[name][s] = *v
	}
	return cfg
}

func TestFlagMatrix_ShouldResolveEveryCell_WhenGlobalAndScopesPermuted(t *testing.T) {
	t.Parallel()
	allPass := FlagEvidence{S1HookReplyPass: true, S6TierBPass: true}
	noEvidence := FlagEvidence{}

	t.Run("background_activity_feed follows global only, default ON", func(t *testing.T) {
		t.Parallel()
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(&config.Config{}, noEvidence, BackgroundActivityFeedFlagName, ""))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(matrixCfg(BackgroundActivityFeedFlagName, tv(false), nil), noEvidence, BackgroundActivityFeedFlagName, ""))
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(matrixCfg(BackgroundActivityFeedFlagName, tv(true), nil), noEvidence, BackgroundActivityFeedFlagName, ""))
	})

	t.Run("askuserquestion_structured stage ladder over every permutation", func(t *testing.T) {
		t.Parallel()
		vals := []tri{nil, tv(true), tv(false)}
		globals := []tri{nil, tv(true), tv(false)}
		inherit := func(g, s tri) bool { // scoped explicit, else global explicit, else default OFF
			if s != nil {
				return *s
			}
			return g != nil && *g
		}
		for _, g := range globals {
			for _, r := range vals {
				for _, a := range vals {
					for _, h := range vals {
						for _, s1 := range []bool{false, true} {
							cfg := matrixCfg(AskUserQuestionStructuredFlagName, g, map[string]tri{
								ScopeStageRender: r, ScopeStageAnswer: a, ScopeStageHookReply: h,
							})
							ev := FlagEvidence{S1HookReplyPass: s1}
							wantRender := inherit(g, r)
							wantAnswer := inherit(g, a) && wantRender
							wantHook := inherit(g, h) && wantAnswer && s1
							label := fmt.Sprintf("g=%v r=%v a=%v h=%v s1=%v", deref(g), deref(r), deref(a), deref(h), s1)
							assert.Equal(t, wantRender, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, ev, AskUserQuestionStructuredFlagName, ScopeStageRender), "render "+label)
							assert.Equal(t, wantAnswer, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, ev, AskUserQuestionStructuredFlagName, ScopeStageAnswer), "answer "+label)
							assert.Equal(t, wantHook, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, ev, AskUserQuestionStructuredFlagName, ScopeStageHookReply), "hook "+label)
						}
					}
				}
			}
		}
	})

	t.Run("askuserquestion_structured named cells", func(t *testing.T) {
		t.Parallel()
		n := AskUserQuestionStructuredFlagName
		// Scoped true under global false renders only (stage 1 ships this way).
		cfg := matrixCfg(n, tv(false), map[string]tri{ScopeStageRender: tv(true)})
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, ScopeStageRender))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, ScopeStageAnswer))
		// Answer enabled while render is off is ignored.
		cfg = matrixCfg(n, tv(false), map[string]tri{ScopeStageAnswer: tv(true)})
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, ScopeStageAnswer))
		// Scoped false under global true is a kill switch for that stage.
		cfg = matrixCfg(n, tv(true), map[string]tri{ScopeStageAnswer: tv(false)})
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, ScopeStageRender))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, ScopeStageAnswer))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, ScopeStageHookReply))
		// Hook reply stays closed without S1 even with every scope on.
		cfg = matrixCfg(n, tv(true), nil)
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, noEvidence, n, ScopeStageHookReply))
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, ScopeStageHookReply))
	})

	t.Run("notification_inline_actions action, remote and tierB", func(t *testing.T) {
		t.Parallel()
		n := NotificationInlineActionsFlagName
		deny, approve := ScopePrefixAction+"DENY", ScopePrefixAction+"APPROVE"
		cfg := matrixCfg(n, tv(true), map[string]tri{deny: tv(false)})
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, deny))
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, approve))
		// OPEN is always on, even with the flag globally off.
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(matrixCfg(n, tv(false), nil), allPass, n, ScopePrefixAction+"OPEN"))
		// Scoped true under global false enables exactly that action.
		cfg = matrixCfg(n, tv(false), map[string]tri{approve: tv(true)})
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, approve))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, deny))
		// Remote: scoped false blocks; absent inherits global.
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(matrixCfg(n, tv(true), map[string]tri{ScopeChannelRemote: tv(false)}), allPass, n, ScopeChannelRemote))
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(matrixCfg(n, tv(true), nil), allPass, n, ScopeChannelRemote))
		// Tier B: needs remote and S6; tierB without remote is ignored.
		for _, remote := range []tri{nil, tv(true), tv(false)} {
			for _, tier := range []tri{nil, tv(true), tv(false)} {
				for _, s6 := range []bool{false, true} {
					cfg := matrixCfg(n, tv(false), map[string]tri{ScopeChannelRemote: remote, ScopeChannelTierB: tier})
					want := tier != nil && *tier && remote != nil && *remote && s6
					got := FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, FlagEvidence{S6TierBPass: s6}, n, ScopeChannelTierB)
					assert.Equal(t, want, got, "remote=%v tierB=%v s6=%v", deref(remote), deref(tier), s6)
				}
			}
		}
	})

	t.Run("agent_wake_triggers dispatcher, trigger and tool", func(t *testing.T) {
		t.Parallel()
		n := AgentWakeTriggersFlagName
		at, cron := ScopePrefixTrigger+"at", ScopePrefixTrigger+"cron"
		sched, help := ScopePrefixTool+"schedule_wake", ScopePrefixTool+"request_help"
		// Defaults: nothing runs.
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(&config.Config{}, allPass, n, "dispatcher"))
		// A scoped true under global false runs the dispatcher for that type only.
		cfg := matrixCfg(n, tv(false), map[string]tri{at: tv(true), sched: tv(true)})
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, "dispatcher"))
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, at))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, cron), "schedule_wake(cron) is flag_off")
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, sched))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, help))
		// A tool scope alone does not start the dispatcher.
		cfg = matrixCfg(n, tv(false), map[string]tri{sched: tv(true)})
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, "dispatcher"))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, sched), "tool:* without the dispatcher")
		// A trigger scope explicitly false does not start it either.
		cfg = matrixCfg(n, tv(false), map[string]tri{at: tv(false)})
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, at))
		// Global true: everything follows it, a scoped false is a kill switch.
		cfg = matrixCfg(n, tv(true), map[string]tri{cron: tv(false)})
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, at))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, cron))
		assert.True(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(cfg, allPass, n, help))
	})

	t.Run("unknown flag or decision point fails closed", func(t *testing.T) {
		t.Parallel()
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(matrixCfg(AskUserQuestionStructuredFlagName, tv(true), nil), allPass, AskUserQuestionStructuredFlagName, "stage:bogus"))
		assert.False(t, FlagReader{testFlagRegistry()}.ResolveEffectiveFlag(matrixCfg("nope", tv(true), nil), allPass, "nope", "x"))
	})
}

func deref(v tri) string {
	if v == nil {
		return "unset"
	}
	return fmt.Sprint(*v)
}

func TestFlagMatrix_ShouldResolveAnswerQuestionComposition_ForActionRouteClassHiddenRenderAnswerPermutations(t *testing.T) {
	t.Parallel()
	routes := []struct {
		name  string
		route AnswerRoute
	}{
		{"hidden live_keystroke", RouteLiveKeystrokeHidden},
		{"visible live_keystroke", RouteLiveKeystrokeVisible},
		{"durable record", RouteDurableRecord},
		{"hook_reply", RouteHookReply},
	}
	bools := []bool{false, true}
	for _, rt := range routes {
		for _, action := range bools {
			for _, hiddenReply := range bools {
				for _, render := range bools {
					for _, answer := range bools {
						for _, hook := range bools {
							for _, s1 := range bools {
								label := fmt.Sprintf("%s action=%v hidden_session_reply=%v render=%v answer=%v hook=%v s1=%v",
									rt.name, action, hiddenReply, render, answer, hook, s1)
								cfg := &config.Config{
									FeatureFlags: map[string]bool{featureflags.HiddenSessionReplyFlagName: hiddenReply},
									FeatureFlagScopes: map[string]map[string]bool{
										NotificationInlineActionsFlagName: {ScopePrefixAction + ActionKindAnswerQuestion: action},
										AskUserQuestionStructuredFlagName: {ScopeStageRender: render, ScopeStageAnswer: answer, ScopeStageHookReply: hook},
									},
								}
								var want bool
								switch rt.route {
								case RouteLiveKeystrokeHidden:
									want = action && hiddenReply
								case RouteLiveKeystrokeVisible, RouteDurableRecord:
									want = action && answer && render
								case RouteHookReply:
									want = action && hook && answer && render && s1
								}
								got := FlagReader{testFlagRegistry()}.ResolveAnswerQuestion(cfg, FlagEvidence{S1HookReplyPass: s1}, rt.route)
								assert.Equal(t, want, got, label)
							}
						}
					}
				}
			}
		}
	}
}

func TestFlagMatrix_ShouldKeepSpikeGatedCellsClosed_WhenSpikeResultsNotRecorded_ExceptS6(t *testing.T) {
	t.Parallel()
	assert.False(t, SpikeS1HookReplyPassed, "flip only when S1 PASS is recorded in research/spike-S1.md")
	assert.True(t, SpikeS6TierBPassed, "S6 PASS recorded; channel:remote:tierB stays an operator opt-in scope")
	assert.Equal(t, "This action is turned off. Open session.", FlagOffMessage)
}
