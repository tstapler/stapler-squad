package config

import "testing"

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
	if c.BackgroundEffort() != "" {
		t.Error("unset effort must be empty")
	}
	for in, want := range map[string]string{"High": "high", "xhigh": "xhigh", "bogus; rm": "", "": ""} {
		c.BackgroundModels.Effort = in
		if got := c.BackgroundEffort(); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
