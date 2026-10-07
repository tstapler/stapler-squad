package config

import "strings"

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

// backgroundFeatureModelDefaults: haiku for short extraction/summary calls, sonnet
// where a wrong answer costs a rework loop or an unsafe approval. Opus is never a
// default. See docs/reference/background-model-defaults.md.
func backgroundFeatureModelDefault(feature string) string {
	switch feature {
	case "session-completion-summary", "handoff-summary", "backlog-intent-parse",
		"pr-description", "commit-message", "summarize", "acceptance-criteria":
		return "haiku"
	case "autonomous_fix", "autonomous_approval":
		return "sonnet"
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
		if m := strings.TrimSpace(c.BackgroundModels.Features[feature]); m != "" {
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
		m = strings.TrimSpace(c.BackgroundModels.Stages[role])
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
	if isValidEffortLevel(e) {
		return e
	}
	return ""
}
