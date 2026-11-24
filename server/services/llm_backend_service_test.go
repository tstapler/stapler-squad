package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/anthropicwire"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/headless"
)

func isolateLLMConfig(t *testing.T) {
	t.Helper()
	svctest.WithIsolatedStateDir(t)
	config.ResetLiveLLMBackendsForTest()
	t.Cleanup(config.ResetLiveLLMBackendsForTest)
}

func TestLLMBackendServiceUpdateIsLiveAndValidated(t *testing.T) {
	isolateLLMConfig(t)
	svc := NewLLMBackendService(nil)

	bad := &sessionv1.UpdateLLMBackendSettingsRequest{Settings: &sessionv1.LLMBackendSettings{DefaultBackend: "nope"}}
	if _, err := svc.UpdateLLMBackendSettings(context.Background(), connect.NewRequest(bad)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unknown backend must be rejected, got %v", err)
	}
	badFeature := &sessionv1.UpdateLLMBackendSettingsRequest{Settings: &sessionv1.LLMBackendSettings{PerFeature: map[string]string{"bogus": "agy"}}}
	if _, err := svc.UpdateLLMBackendSettings(context.Background(), connect.NewRequest(badFeature)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unknown feature must be rejected, got %v", err)
	}

	sel := headless.NewSelector(func() headless.BackendSettings { return BackendSettingsFromConfig(config.LiveLLMBackends()) })
	if got := sel.Requested(headless.FeatureKeySessionTagging); got != headless.BackendClaude {
		t.Fatalf("before update = %q", got)
	}
	ok := &sessionv1.UpdateLLMBackendSettingsRequest{Settings: &sessionv1.LLMBackendSettings{
		PerFeature: map[string]string{"session-tagging": "consolette"},
		ModelMaps:  []*sessionv1.LLMBackendModelMap{{Backend: "consolette", Aliases: map[string]string{"haiku": "free-small"}}},
	}}
	resp, err := svc.UpdateLLMBackendSettings(context.Background(), connect.NewRequest(ok))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.Settings.PerFeature["session-tagging"] != "consolette" {
		t.Errorf("read-back = %+v", resp.Msg.Settings)
	}
	if got := sel.Requested(headless.FeatureKeySessionTagging); got != headless.BackendConsolette {
		t.Fatalf("selector must see the edit without restart, got %q", got)
	}

	// Persisted: a cold reload from disk sees it.
	config.ResetLiveLLMBackendsForTest()
	if got := config.LiveLLMBackends().PerFeature["session-tagging"]; got != "consolette" {
		t.Errorf("not persisted, got %q", got)
	}
}

type stubAIClient struct{ called bool }

func (s *stubAIClient) Complete(context.Context, string, string) (string, error) {
	s.called = true
	return "fallback", nil
}

func TestRulesAIClientRoutesByFeatureSetting(t *testing.T) {
	isolateLLMConfig(t)
	t.Cleanup(func() { headless.SetDefaultSelector(nil) })
	fb := &stubAIClient{}
	c := anthropicwire.WrapRulesAIClient(fb)
	if anthropicwire.WrapRulesAIClient(nil) != nil {
		t.Fatal("nil fallback must stay nil")
	}

	headless.SetDefaultSelector(nil)
	if out, _ := c.Complete(context.Background(), "s", "u"); out != "fallback" || !fb.called {
		t.Fatal("no selector: original chain")
	}

	sel := headless.NewSelector(func() headless.BackendSettings { return BackendSettingsFromConfig(config.LiveLLMBackends()) })
	headless.SetDefaultSelector(sel)
	fb.called = false
	if out, _ := c.Complete(context.Background(), "s", "u"); out != "fallback" || !fb.called {
		t.Fatal("default claude: original chain")
	}

	if err := config.UpdateLLMBackends(config.LLMBackendsConfig{PerFeature: map[string]string{"rules-generation": "agy"}}); err != nil {
		t.Fatal(err)
	}
	fb.called = false
	_, err := c.Complete(context.Background(), "s", "u")
	if fb.called || err == nil {
		t.Fatalf("overridden feature must go through selector (no backends registered -> ErrNoBackend); called=%v err=%v", fb.called, err)
	}
}

func TestLLMBackendServiceGetReturnsDefaults(t *testing.T) {
	isolateLLMConfig(t)
	svc := NewLLMBackendService(nil)
	resp, err := svc.GetLLMBackendSettings(context.Background(), connect.NewRequest(&sessionv1.GetLLMBackendSettingsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.Settings == nil {
		t.Fatal("settings must be present")
	}
}
