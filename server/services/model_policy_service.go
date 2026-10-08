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

func modelPolicyEffective(cfg *config.Config) map[string]string {
	out := make(map[string]string)
	for _, k := range config.ModelPolicyKeys() {
		v := cfg.ModelPolicyValue(k)
		if v == "" {
			v = config.ModelPolicyNone
		}
		out[k] = v
	}
	return out
}

func modelPolicyDefaults() map[string]string {
	out := make(map[string]string)
	for _, k := range config.ModelPolicyKeys() {
		out[k] = config.ModelPolicyDefault(k)
	}
	return out
}

// GetModelPolicy returns the effective per-feature model/effort settings.
// +api: model-policy:get
func (s *SessionService) GetModelPolicy(
	_ context.Context,
	_ *connect.Request[sessionv1.GetModelPolicyRequest],
) (*connect.Response[sessionv1.GetModelPolicyResponse], error) {
	return connect.NewResponse(&sessionv1.GetModelPolicyResponse{
		Effective: &sessionv1.ModelPolicyProto{Values: modelPolicyEffective(config.LoadConfig())},
		Defaults:  &sessionv1.ModelPolicyProto{Values: modelPolicyDefaults()},
	}), nil
}

// UpdateModelPolicy validates and persists the given keys; readers reload config per call, so
// the change applies without a restart.
// +api: model-policy:update
func (s *SessionService) UpdateModelPolicy(
	_ context.Context,
	req *connect.Request[sessionv1.UpdateModelPolicyRequest],
) (*connect.Response[sessionv1.UpdateModelPolicyResponse], error) {
	in := req.Msg.GetPolicy().GetValues()
	for k, v := range in {
		if reason := config.ValidateModelPolicyValue(k, v); reason != "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s: %s", k, reason))
		}
	}
	cfg := config.LoadConfig()
	if cfg.ModelPolicy == nil {
		cfg.ModelPolicy = make(map[string]string)
	}
	for k, v := range in {
		if v = strings.TrimSpace(v); v == "" {
			delete(cfg.ModelPolicy, k)
		} else {
			cfg.ModelPolicy[k] = v
		}
	}
	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save model policy: %w", err))
	}
	log.Info("model policy updated", "keys", len(in))
	return connect.NewResponse(&sessionv1.UpdateModelPolicyResponse{
		Effective: &sessionv1.ModelPolicyProto{Values: modelPolicyEffective(cfg)},
	}), nil
}
