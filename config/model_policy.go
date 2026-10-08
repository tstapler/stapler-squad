package config

import (
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// Model-policy keys: one per background LLM feature, plus the shared effort level.
const (
	ModelPolicyWork                = "work"
	ModelPolicyReview              = "review"
	ModelPolicyCompletionNarrative = "completion_narrative"
	ModelPolicyHandoffSummary      = "handoff_summary"
	ModelPolicyIntentParse         = "intent_parse"
	ModelPolicyPRDescription       = "pr_description"
	ModelPolicyBackgroundEffort    = "background_effort"
)

// ModelPolicyNone opts a feature out of the pin (the account default applies).
const ModelPolicyNone = "none"

// modelPolicyDefaults holds the built-in values. Nothing here may resolve to Opus: Opus is
// opt-in only (TestModelPolicyDefaults_NeverOpus).
var modelPolicyDefaults = map[string]string{
	ModelPolicyWork:                "family:sonnet",
	ModelPolicyReview:              "family:sonnet",
	ModelPolicyCompletionNarrative: "family:haiku",
	ModelPolicyHandoffSummary:      "family:haiku",
	ModelPolicyIntentParse:         "family:haiku",
	ModelPolicyPRDescription:       "family:haiku",
	ModelPolicyBackgroundEffort:    "medium",
}

// validEfforts are the levels `claude --effort` accepts.
var validEfforts = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true, "max": true}

// ModelPolicyKeys lists every known key, in display order.
func ModelPolicyKeys() []string {
	return []string{
		ModelPolicyWork, ModelPolicyReview, ModelPolicyCompletionNarrative,
		ModelPolicyHandoffSummary, ModelPolicyIntentParse, ModelPolicyPRDescription,
		ModelPolicyBackgroundEffort,
	}
}

// ModelPolicyDefault returns the built-in value for key ("" for unknown keys).
func ModelPolicyDefault(key string) string { return modelPolicyDefaults[key] }

// ValidateModelPolicyValue reports why value is unacceptable for key, or "" when valid.
// An empty value is valid and means "use the built-in default".
func ValidateModelPolicyValue(key, value string) string {
	if _, ok := modelPolicyDefaults[key]; !ok {
		return "unknown model policy key " + key
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if key == ModelPolicyBackgroundEffort {
		if value != ModelPolicyNone && !validEfforts[value] {
			return "effort must be one of low, medium, high, xhigh, max, none"
		}
		return ""
	}
	if len(value) > 128 || strings.ContainsAny(value, " \t\n") {
		return "model must be a single token of at most 128 characters"
	}
	return ""
}

// ModelPolicyValue returns the effective raw value for key ("family:<alias>", a concrete
// ID, or an effort level), "" when opted out via "none". An invalid configured value falls
// back to the built-in default and logs a warning. Nil-safe.
func (c *Config) ModelPolicyValue(key string) string {
	def := modelPolicyDefaults[key]
	if c == nil {
		return def
	}
	v := strings.TrimSpace(c.ModelPolicy[key])
	if v == "" {
		return def
	}
	if reason := ValidateModelPolicyValue(key, v); reason != "" {
		log.Warn("invalid model_policy value, using built-in default", "key", key, "value", v, "reason", reason)
		return def
	}
	if v == ModelPolicyNone {
		return ""
	}
	return v
}
