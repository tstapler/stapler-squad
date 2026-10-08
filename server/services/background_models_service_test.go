package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestUpdateBackgroundModels_ValidatesAndPersists(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	s := &SessionService{}
	upd := func(p *sessionv1.BackgroundModelsProto) error {
		_, err := s.UpdateBackgroundModels(context.Background(),
			connect.NewRequest(&sessionv1.UpdateBackgroundModelsRequest{Settings: p}))
		return err
	}

	for name, bad := range map[string]*sessionv1.BackgroundModelsProto{
		"unknown feature": {Features: map[string]string{"nope": "haiku"}},
		"flag-like model": {Features: map[string]string{"handoff-summary": "--x"}},
		"bad stage":       {Stages: map[string]string{"triage": "haiku"}},
		"bad effort":      {Effort: "turbo"},
	} {
		if err := upd(bad); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: err = %v, want InvalidArgument", name, err)
		}
	}

	if err := upd(&sessionv1.BackgroundModelsProto{
		Features: map[string]string{"handoff-summary": " sonnet ", "summarize": ""},
		Stages:   map[string]string{"review": "haiku"},
		Effort:   "Medium",
	}); err != nil {
		t.Fatal(err)
	}
	cfg := config.LoadConfig()
	if got := cfg.BackgroundFeatureModel("handoff-summary"); got != "sonnet" {
		t.Errorf("handoff-summary = %q, want sonnet (live, no restart)", got)
	}
	if got := cfg.BackgroundFeatureModel("summarize"); got != "haiku" {
		t.Errorf("blank override should fall to default, got %q", got)
	}
	if got := cfg.BackgroundStageModel("review"); got != "family:haiku" {
		t.Errorf("review = %q", got)
	}
	if got := cfg.BackgroundEffort(); got != "medium" {
		t.Errorf("effort = %q", got)
	}

	resp, err := s.GetBackgroundModels(context.Background(), connect.NewRequest(&sessionv1.GetBackgroundModelsRequest{}))
	if err != nil || resp.Msg.FeatureDefaults["handoff-summary"] != "haiku" || resp.Msg.StageDefaults["work"] != "sonnet" {
		t.Errorf("get = %v, %v", resp, err)
	}
}
