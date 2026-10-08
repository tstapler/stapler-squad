package server

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/headless"
)

// headlessFeaturePolicyKeys maps pool features to their config.ModelPolicy key. Features not
// listed here (review, triage, autonomous fix, ...) pass their own model or use the pool default.
var headlessFeaturePolicyKeys = map[headless.FeatureKey]string{
	headless.FeatureKeySessionCompletionSummary: config.ModelPolicyCompletionNarrative,
	headless.FeatureKeyHandoffSummary:           config.ModelPolicyHandoffSummary,
	headless.FeatureKeyBacklogIntentParse:       config.ModelPolicyIntentParse,
	headless.FeatureKeyPRDescription:            config.ModelPolicyPRDescription,
}

// headlessPoolDefaultModel is the pool-wide model for calls that name none and have no
// per-feature policy: Sonnet, never the account default (which may be Opus).
const headlessPoolDefaultModel = "family:sonnet"

// headlessPoolModelConfig fills in the pool's model pinning. Per-feature models and effort
// are re-read from config on every call so edits apply without a restart.
func headlessPoolModelConfig(cfg headless.PoolConfig) headless.PoolConfig {
	families := workflows.DefaultModelFamilies()
	cfg.DefaultModel, _ = session.ResolveModel(families, headlessPoolDefaultModel)
	cfg.ModelForFeature = func(key headless.FeatureKey) string {
		policyKey, ok := headlessFeaturePolicyKeys[key]
		if !ok {
			return ""
		}
		return session.ResolveFeatureModel(config.LoadConfig(), families, policyKey, "claude")
	}
	cfg.Effort = func() string { return config.LoadConfig().ModelPolicyValue(config.ModelPolicyBackgroundEffort) }
	return cfg
}
