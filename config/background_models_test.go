package config

import (
	"strings"
	"testing"
)

func TestBackgroundFeatureModel_Defaults(t *testing.T) {
	t.Parallel()
	c := &Config{}
	want := map[string]string{
		"session-completion-summary": "haiku", "handoff-summary": "haiku", "backlog-intent-parse": "haiku",
		"pr-description": "haiku", "autonomous_fix": "sonnet", "autonomous_approval": "sonnet", "unknown": "",
	}
	for k, v := range want {
		if got := c.BackgroundFeatureModel(k); got != v {
			t.Errorf("%s: got %q want %q", k, got, v)
		}
	}
	c.BackgroundModels.Features = map[string]string{"handoff-summary": " sonnet "}
	if got := c.BackgroundFeatureModel("handoff-summary"); got != "sonnet" {
		t.Errorf("override: got %q", got)
	}
	if got := (*Config)(nil).BackgroundFeatureModel("pr-description"); got != "haiku" {
		t.Errorf("nil config: got %q", got)
	}
}

func TestBackgroundStageModel_AliasesAndOverride(t *testing.T) {
	t.Parallel()
	c := &Config{}
	for _, role := range []string{"work", "review"} {
		if got := c.BackgroundStageModel(role); got != "family:sonnet" {
			t.Errorf("%s default: got %q", role, got)
		}
	}
	c.BackgroundModels.Stages = map[string]string{"review": "haiku", "work": "claude-opus-4-8"}
	if got := c.BackgroundStageModel("review"); got != "family:haiku" {
		t.Errorf("review: got %q", got)
	}
	if got := c.BackgroundStageModel("triage"); got != "" {
		t.Errorf("triage must not be pinned here: got %q", got)
	}
	if got := c.BackgroundStageModel("work"); got != "claude-opus-4-8" {
		t.Errorf("work: got %q", got)
	}
}

func TestBackgroundEffort_ValidatesLevels(t *testing.T) {
	t.Parallel()
	c := &Config{}
	if c.BackgroundEffort() != "medium" {
		t.Error("unset effort must default to medium")
	}
	for in, want := range map[string]string{"High": "high", "xhigh": "xhigh", "bogus; rm": "medium", "": "medium", "off": ""} {
		c.BackgroundModels.Effort = in
		if got := c.BackgroundEffort(); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestBackgroundModels_InvalidValuesFallBackToDefault(t *testing.T) {
	c := &Config{BackgroundModels: BackgroundModelsConfig{
		Features: map[string]string{"handoff-summary": "--dangerously-skip-permissions", "summarize": "has space"},
		Stages:   map[string]string{"work": "-x"},
	}}
	if got := c.BackgroundFeatureModel("handoff-summary"); got != "haiku" {
		t.Errorf("flag-like feature model = %q, want haiku", got)
	}
	if got := c.BackgroundFeatureModel("summarize"); got != "haiku" {
		t.Errorf("spaced feature model = %q, want haiku", got)
	}
	if got := c.BackgroundStageModel("work"); got != "family:sonnet" {
		t.Errorf("flag-like stage model = %q, want family:sonnet", got)
	}
}

func TestBackgroundModels_NoDefaultIsOpus(t *testing.T) {
	var c *Config
	for _, f := range []string{"session-completion-summary", "handoff-summary", "backlog-intent-parse", "pr-description", "autonomous_fix", "autonomous_approval", "unknown"} {
		if strings.Contains(c.BackgroundFeatureModel(f), "opus") {
			t.Errorf("feature %q defaults to opus", f)
		}
	}
	for _, r := range []string{"work", "review"} {
		if strings.Contains(c.BackgroundStageModel(r), "opus") {
			t.Errorf("stage %q defaults to opus", r)
		}
	}
	if strings.Contains(HeadlessPoolDefaultModel, "opus") || HeadlessPoolDefaultModel == "" {
		t.Errorf("HeadlessPoolDefaultModel = %q", HeadlessPoolDefaultModel)
	}
}
