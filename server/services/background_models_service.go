package services

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
)

func backgroundModelsToProto(c config.BackgroundModelsConfig) *sessionv1.BackgroundModelsProto {
	return &sessionv1.BackgroundModelsProto{Features: c.Features, Stages: c.Stages, Effort: c.Effort}
}

// GetBackgroundModels returns the model/effort overrides plus the built-in defaults.
// +api: background-models:get
func (s *SessionService) GetBackgroundModels(
	_ context.Context,
	_ *connect.Request[sessionv1.GetBackgroundModelsRequest],
) (*connect.Response[sessionv1.GetBackgroundModelsResponse], error) {
	resp := &sessionv1.GetBackgroundModelsResponse{
		Settings:        backgroundModelsToProto(config.LoadConfig().BackgroundModels),
		FeatureDefaults: map[string]string{},
		StageDefaults:   map[string]string{},
		EffortLevels:    config.BackgroundEffortLevels,
	}
	for _, k := range config.BackgroundFeatureKeys {
		resp.FeatureDefaults[k] = config.BackgroundFeatureDefault(k)
	}
	for _, r := range config.BackgroundStageRoles {
		resp.StageDefaults[r] = config.BackgroundStageDefault(r)
	}
	return connect.NewResponse(resp), nil
}

// cleanModelOverrides trims, drops blanks, and rejects unknown keys or unsafe names.
func cleanModelOverrides(kind string, in map[string]string, allowed []string) (map[string]string, error) {
	var out map[string]string
	for k, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		known := false
		for _, a := range allowed {
			known = known || a == k
		}
		if !known {
			return nil, fmt.Errorf("unknown %s %q", kind, k)
		}
		if !config.ValidBackgroundModelName(v) {
			return nil, fmt.Errorf("invalid model %q for %s %q", v, kind, k)
		}
		if out == nil {
			out = map[string]string{}
		}
		out[k] = v
	}
	return out, nil
}

// UpdateBackgroundModels validates and persists the overrides; reads are live, so the
// next headless call or work spawn uses them without a restart.
// +api: background-models:update
func (s *SessionService) UpdateBackgroundModels(
	_ context.Context,
	req *connect.Request[sessionv1.UpdateBackgroundModelsRequest],
) (*connect.Response[sessionv1.UpdateBackgroundModelsResponse], error) {
	in := req.Msg.GetSettings()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("settings is required"))
	}
	features, err := cleanModelOverrides("feature", in.GetFeatures(), config.BackgroundFeatureKeys)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	stages, err := cleanModelOverrides("stage", in.GetStages(), config.BackgroundStageRoles)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	effort := strings.ToLower(strings.TrimSpace(in.GetEffort()))
	if effort != "" && !config.ValidBackgroundEffort(effort) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid effort %q", effort))
	}

	cfg := config.LoadConfig()
	cfg.BackgroundModels = config.BackgroundModelsConfig{Features: features, Stages: stages, Effort: effort}
	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save background models: %w", err))
	}
	log.Info("background models updated", "features", features, "stages", stages, "effort", effort)
	return connect.NewResponse(&sessionv1.UpdateBackgroundModelsResponse{
		Settings: backgroundModelsToProto(cfg.BackgroundModels),
	}), nil
}
