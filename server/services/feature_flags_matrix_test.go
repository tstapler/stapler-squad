package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/config"
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
	allPass := flagEvidence{s1HookReplyPass: true, s6TierBPass: true}
	noEvidence := flagEvidence{}

	t.Run("background_activity_feed follows global only, default ON", func(t *testing.T) {
		t.Parallel()
		assert.True(t, resolveEffectiveFlag(&config.Config{}, noEvidence, backgroundActivityFeedFlagName, ""))
		assert.False(t, resolveEffectiveFlag(matrixCfg(backgroundActivityFeedFlagName, tv(false), nil), noEvidence, backgroundActivityFeedFlagName, ""))
		assert.True(t, resolveEffectiveFlag(matrixCfg(backgroundActivityFeedFlagName, tv(true), nil), noEvidence, backgroundActivityFeedFlagName, ""))
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
							cfg := matrixCfg(askUserQuestionStructuredFlagName, g, map[string]tri{
								scopeStageRender: r, scopeStageAnswer: a, scopeStageHookReply: h,
							})
							ev := flagEvidence{s1HookReplyPass: s1}
							wantRender := inherit(g, r)
							wantAnswer := inherit(g, a) && wantRender
							wantHook := inherit(g, h) && wantAnswer && s1
							label := fmt.Sprintf("g=%v r=%v a=%v h=%v s1=%v", deref(g), deref(r), deref(a), deref(h), s1)
							assert.Equal(t, wantRender, resolveEffectiveFlag(cfg, ev, askUserQuestionStructuredFlagName, scopeStageRender), "render "+label)
							assert.Equal(t, wantAnswer, resolveEffectiveFlag(cfg, ev, askUserQuestionStructuredFlagName, scopeStageAnswer), "answer "+label)
							assert.Equal(t, wantHook, resolveEffectiveFlag(cfg, ev, askUserQuestionStructuredFlagName, scopeStageHookReply), "hook "+label)
						}
					}
				}
			}
		}
	})

	t.Run("askuserquestion_structured named cells", func(t *testing.T) {
		t.Parallel()
		n := askUserQuestionStructuredFlagName
		// Scoped true under global false renders only (stage 1 ships this way).
		cfg := matrixCfg(n, tv(false), map[string]tri{scopeStageRender: tv(true)})
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, scopeStageRender))
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, scopeStageAnswer))
		// Answer enabled while render is off is ignored.
		cfg = matrixCfg(n, tv(false), map[string]tri{scopeStageAnswer: tv(true)})
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, scopeStageAnswer))
		// Scoped false under global true is a kill switch for that stage.
		cfg = matrixCfg(n, tv(true), map[string]tri{scopeStageAnswer: tv(false)})
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, scopeStageRender))
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, scopeStageAnswer))
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, scopeStageHookReply))
		// Hook reply stays closed without S1 even with every scope on.
		cfg = matrixCfg(n, tv(true), nil)
		assert.False(t, resolveEffectiveFlag(cfg, noEvidence, n, scopeStageHookReply))
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, scopeStageHookReply))
	})

	t.Run("notification_inline_actions action, remote and tierB", func(t *testing.T) {
		t.Parallel()
		n := notificationInlineActionsFlagName
		deny, approve := scopePrefixAction+"DENY", scopePrefixAction+"APPROVE"
		cfg := matrixCfg(n, tv(true), map[string]tri{deny: tv(false)})
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, deny))
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, approve))
		// OPEN is always on, even with the flag globally off.
		assert.True(t, resolveEffectiveFlag(matrixCfg(n, tv(false), nil), allPass, n, scopePrefixAction+"OPEN"))
		// Scoped true under global false enables exactly that action.
		cfg = matrixCfg(n, tv(false), map[string]tri{approve: tv(true)})
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, approve))
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, deny))
		// Remote: scoped false blocks; absent inherits global.
		assert.False(t, resolveEffectiveFlag(matrixCfg(n, tv(true), map[string]tri{scopeChannelRemote: tv(false)}), allPass, n, scopeChannelRemote))
		assert.True(t, resolveEffectiveFlag(matrixCfg(n, tv(true), nil), allPass, n, scopeChannelRemote))
		// Tier B: needs remote and S6; tierB without remote is ignored.
		for _, remote := range []tri{nil, tv(true), tv(false)} {
			for _, tier := range []tri{nil, tv(true), tv(false)} {
				for _, s6 := range []bool{false, true} {
					cfg := matrixCfg(n, tv(false), map[string]tri{scopeChannelRemote: remote, scopeChannelTierB: tier})
					want := tier != nil && *tier && remote != nil && *remote && s6
					got := resolveEffectiveFlag(cfg, flagEvidence{s6TierBPass: s6}, n, scopeChannelTierB)
					assert.Equal(t, want, got, "remote=%v tierB=%v s6=%v", deref(remote), deref(tier), s6)
				}
			}
		}
	})

	t.Run("agent_wake_triggers dispatcher, trigger and tool", func(t *testing.T) {
		t.Parallel()
		n := agentWakeTriggersFlagName
		at, cron := scopePrefixTrigger+"at", scopePrefixTrigger+"cron"
		sched, help := scopePrefixTool+"schedule_wake", scopePrefixTool+"request_help"
		// Defaults: nothing runs.
		assert.False(t, resolveEffectiveFlag(&config.Config{}, allPass, n, "dispatcher"))
		// A scoped true under global false runs the dispatcher for that type only.
		cfg := matrixCfg(n, tv(false), map[string]tri{at: tv(true), sched: tv(true)})
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, "dispatcher"))
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, at))
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, cron), "schedule_wake(cron) is flag_off")
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, sched))
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, help))
		// A tool scope alone does not start the dispatcher.
		cfg = matrixCfg(n, tv(false), map[string]tri{sched: tv(true)})
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, "dispatcher"))
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, sched), "tool:* without the dispatcher")
		// A trigger scope explicitly false does not start it either.
		cfg = matrixCfg(n, tv(false), map[string]tri{at: tv(false)})
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, at))
		// Global true: everything follows it, a scoped false is a kill switch.
		cfg = matrixCfg(n, tv(true), map[string]tri{cron: tv(false)})
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, at))
		assert.False(t, resolveEffectiveFlag(cfg, allPass, n, cron))
		assert.True(t, resolveEffectiveFlag(cfg, allPass, n, help))
	})

	t.Run("unknown flag or decision point fails closed", func(t *testing.T) {
		t.Parallel()
		assert.False(t, resolveEffectiveFlag(matrixCfg(askUserQuestionStructuredFlagName, tv(true), nil), allPass, askUserQuestionStructuredFlagName, "stage:bogus"))
		assert.False(t, resolveEffectiveFlag(matrixCfg("nope", tv(true), nil), allPass, "nope", "x"))
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
		route answerRoute
	}{
		{"hidden live_keystroke", routeLiveKeystrokeHidden},
		{"visible live_keystroke", routeLiveKeystrokeVisible},
		{"durable record", routeDurableRecord},
		{"hook_reply", routeHookReply},
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
									FeatureFlags: map[string]bool{hiddenSessionReplyFlagName: hiddenReply},
									FeatureFlagScopes: map[string]map[string]bool{
										notificationInlineActionsFlagName: {scopePrefixAction + actionKindAnswerQuestion: action},
										askUserQuestionStructuredFlagName: {scopeStageRender: render, scopeStageAnswer: answer, scopeStageHookReply: hook},
									},
								}
								var want bool
								switch rt.route {
								case routeLiveKeystrokeHidden:
									want = action && hiddenReply
								case routeLiveKeystrokeVisible, routeDurableRecord:
									want = action && answer && render
								case routeHookReply:
									want = action && hook && answer && render && s1
								}
								got := resolveAnswerQuestion(cfg, flagEvidence{s1HookReplyPass: s1}, rt.route)
								assert.Equal(t, want, got, label)
							}
						}
					}
				}
			}
		}
	}
}

func TestFlagMatrix_ShouldKeepSpikeGatedCellsClosed_WhenSpikeResultsNotRecorded(t *testing.T) {
	t.Parallel()
	assert.False(t, spikeS1HookReplyPassed, "flip only when S1 PASS is recorded in research/spike-S1.md")
	assert.False(t, spikeS6TierBPassed, "flip only when S6 PASS is recorded")
	assert.Equal(t, "This action is turned off. Open session.", flagOffMessage)
}
