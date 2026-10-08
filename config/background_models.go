package config

import (
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// HeadlessPoolDefaultModel is the pool-wide model for calls that name neither a model
// nor a pinned feature, so they never fall through to the account default.
const HeadlessPoolDefaultModel = "haiku"

const maxBackgroundModelLength = 128

// validModelName admits aliases ("sonnet"), family refs ("family:sonnet"), full IDs and
// context suffixes ("sonnet[1m]"); it rejects whitespace and a leading "-" (argv injection).
var validModelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:\[\]-]*$`)

// usableModel returns m when it is safe to forward as a --model value; otherwise it
// warns and returns "" so the caller falls back to the built-in default.
func usableModel(kind, key, m string) string {
	if m == "" {
		return ""
	}
	if !ValidBackgroundModelName(m) {
		log.Warn("ignoring invalid background model; using built-in default", "kind", kind, "key", key, "value", m)
		return ""
	}
	return m
}

// BackgroundModelsConfig pins the model (and effort) for unattended LLM work so it
// does not fall through to the account default (typically the most expensive model).
// Read live via LoadConfig, so edits to config.json apply without a restart.
type BackgroundModelsConfig struct {
	// Features maps a headless feature key (e.g. "session-completion-summary") to a
	// model alias or ID passed as --model to the headless pool.
	Features map[string]string `json:"features,omitempty"`
	// Stages maps a backlog stage role ("work" or "review"; triage uses headless_triage_model) to a model alias
	// ("sonnet") or ID, used when the item's pipeline mode does not pin one.
	Stages map[string]string `json:"stages,omitempty"`
	// Effort is the --effort level for background work sessions; empty = unset.
	Effort string `json:"effort,omitempty"`
}

// backgroundFeatureDefaults: haiku for short extraction/summary calls, sonnet where a wrong
// answer costs a rework loop or an unsafe approval and for open-ended prompts. Opus is never
// a default. Single source for the defaults and the panel's key list. See
// docs/reference/background-model-defaults.md.
func backgroundFeatureDefaults() []struct{ key, model string } {
	return []struct{ key, model string }{
		{"session-completion-summary", "haiku"}, {"handoff-summary", "haiku"},
		{"backlog-intent-parse", "haiku"}, {"pr-description", "haiku"},
		{"commit-message", "haiku"}, {"summarize", "haiku"}, {"acceptance-criteria", "haiku"},
		{"unfinished-work-summary", "haiku"}, {"session-tagging", "haiku"},
		{"autonomous_fix", "sonnet"}, {"autonomous_approval", "sonnet"},
		{"review", "sonnet"}, {"triage", "sonnet"}, {"custom", "sonnet"},
		{"rules-generation", "sonnet"}, {"instance-resume", "sonnet"},
	}
}

func backgroundFeatureModelDefault(feature string) string {
	for _, d := range backgroundFeatureDefaults() {
		if d.key == feature {
			return d.model
		}
	}
	return ""
}

func backgroundStageModelDefault(role string) string {
	switch role {
	case "work", "review":
		return "sonnet"
	}
	return ""
}

// isValidEffortLevel mirrors `claude --help` (--effort <level>) for claude 2.1.290.
func isValidEffortLevel(e string) bool {
	switch e {
	case "low", "medium", "high", "xhigh", "max":
		return true
	}
	return false
}

// BackgroundFeatureModel returns the pinned model for a headless feature key:
// configured override, else the built-in default, else "" (account default).
func (c *Config) BackgroundFeatureModel(feature string) string {
	if c != nil {
		if m := usableModel("feature", feature, strings.TrimSpace(c.BackgroundModels.Features[feature])); m != "" {
			return m
		}
	}
	return backgroundFeatureModelDefault(feature)
}

// BackgroundStageModel returns the default model for a backlog stage role as a
// stored executor value: a bare family alias becomes "family:<alias>" so it
// resolves through the same model-family map as pipeline-mode executors.
func (c *Config) BackgroundStageModel(role string) string {
	m := ""
	if c != nil {
		m = usableModel("stage", role, strings.TrimSpace(c.BackgroundModels.Stages[role]))
	}
	if m == "" {
		m = backgroundStageModelDefault(role)
	}
	switch m {
	case "opus", "sonnet", "haiku":
		return "family:" + m
	}
	return m
}

// BackgroundEffort returns the configured effort level, or "" when unset or not a
// level the claude CLI accepts (never forward an unvalidated value to argv).
func (c *Config) BackgroundEffort() string {
	if c == nil {
		return ""
	}
	e := strings.ToLower(strings.TrimSpace(c.BackgroundModels.Effort))
	if e == "" {
		return ""
	}
	if !isValidEffortLevel(e) {
		log.Warn("ignoring invalid background effort level; leaving unset", "value", e)
		return ""
	}
	return e
}

// BackgroundFeatureKeys lists the headless feature keys with a built-in default.
func BackgroundFeatureKeys() []string {
	defs := backgroundFeatureDefaults()
	keys := make([]string, len(defs))
	for i, d := range defs {
		keys[i] = d.key
	}
	return keys
}

// BackgroundStageRoles lists the backlog stage roles that take a default pin.
func BackgroundStageRoles() []string { return []string{"work", "review"} }

// BackgroundEffortLevels lists the accepted --effort values.
func BackgroundEffortLevels() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

// BackgroundFeatureDefault exposes the built-in default for a feature key ("" if none).
func BackgroundFeatureDefault(feature string) string { return backgroundFeatureModelDefault(feature) }

// BackgroundStageDefault exposes the built-in default for a stage role ("" if none).
func BackgroundStageDefault(role string) string { return backgroundStageModelDefault(role) }

// ValidBackgroundModelName reports whether m is safe to store as a model override.
func ValidBackgroundModelName(m string) bool {
	return len(m) <= maxBackgroundModelLength && validModelName.MatchString(m)
}

// ValidBackgroundEffort reports whether e is an accepted effort level.
func ValidBackgroundEffort(e string) bool { return isValidEffortLevel(e) }
