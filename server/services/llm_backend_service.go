package services

import (
	"context"
	"sort"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session/headless"
)

var _ sessionv1connect.LLMBackendServiceHandler = (*LLMBackendService)(nil)

// LLMBackendService reads and edits the live headless backend selection.
type LLMBackendService struct {
	selector *headless.Selector
}

// NewLLMBackendService creates the service. selector may be nil (status list empty).
func NewLLMBackendService(selector *headless.Selector) *LLMBackendService {
	return &LLMBackendService{selector: selector}
}

// BackendSettingsFromConfig converts persisted config to selector settings.
func BackendSettingsFromConfig(c config.LLMBackendsConfig) headless.BackendSettings {
	return headless.BackendSettings{
		Default:           c.Default,
		PerFeature:        c.PerFeature,
		ConsoletteBaseURL: c.ConsoletteBaseURL,
		AnthropicBaseURL:  c.AnthropicBaseURL,
		ModelMaps:         c.ModelMaps,
	}
}

// +api: GetLLMBackendSettings
func (s *LLMBackendService) GetLLMBackendSettings(
	_ context.Context, _ *connect.Request[sessionv1.GetLLMBackendSettingsRequest],
) (*connect.Response[sessionv1.GetLLMBackendSettingsResponse], error) {
	resp := &sessionv1.GetLLMBackendSettingsResponse{
		Settings:    settingsToProto(config.LiveLLMBackends()),
		FeatureKeys: headless.OverridableFeatureKeys(),
	}
	if s.selector != nil {
		for _, b := range s.selector.Backends() {
			c := b.Capabilities()
			resp.Backends = append(resp.Backends, &sessionv1.LLMBackendStatus{
				Name: b.Name(), Available: b.Available(),
				SupportsResume: c.Resume, SupportsSystemPrompt: c.SystemPrompt,
				SupportsToolRestriction: c.ToolRestriction, SupportsWorkdir: c.WorkDir,
			})
		}
	}
	return connect.NewResponse(resp), nil
}

// +api: UpdateLLMBackendSettings
func (s *LLMBackendService) UpdateLLMBackendSettings(
	_ context.Context, req *connect.Request[sessionv1.UpdateLLMBackendSettingsRequest],
) (*connect.Response[sessionv1.UpdateLLMBackendSettingsResponse], error) {
	next := settingsFromProto(req.Msg.GetSettings())
	if err := BackendSettingsFromConfig(next).Validate(); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := config.UpdateLLMBackends(next); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Read back what was stored rather than echoing the request.
	return connect.NewResponse(&sessionv1.UpdateLLMBackendSettingsResponse{
		Settings: settingsToProto(config.LiveLLMBackends()),
	}), nil
}

func settingsToProto(c config.LLMBackendsConfig) *sessionv1.LLMBackendSettings {
	out := &sessionv1.LLMBackendSettings{
		DefaultBackend:    c.Default,
		PerFeature:        c.PerFeature,
		ConsoletteBaseUrl: c.ConsoletteBaseURL,
		AnthropicBaseUrl:  c.AnthropicBaseURL,
	}
	names := make([]string, 0, len(c.ModelMaps))
	for n := range c.ModelMaps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		out.ModelMaps = append(out.ModelMaps, &sessionv1.LLMBackendModelMap{Backend: n, Aliases: c.ModelMaps[n]})
	}
	return out
}

func settingsFromProto(p *sessionv1.LLMBackendSettings) config.LLMBackendsConfig {
	out := config.LLMBackendsConfig{
		Default:           p.GetDefaultBackend(),
		PerFeature:        p.GetPerFeature(),
		ConsoletteBaseURL: p.GetConsoletteBaseUrl(),
		AnthropicBaseURL:  p.GetAnthropicBaseUrl(),
	}
	for _, m := range p.GetModelMaps() {
		if out.ModelMaps == nil {
			out.ModelMaps = map[string]map[string]string{}
		}
		out.ModelMaps[m.GetBackend()] = m.GetAliases()
	}
	return out
}
