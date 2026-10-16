package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// maxTaggingClassifierFallbacks caps the fallback hierarchy length: every entry is one
// sequential LLM attempt on failure, so an unbounded list turns one bad primary into an
// unbounded spend event. Five is generous (primary + five fallbacks covers every realistic
// paid-plus-proxy setup) while keeping worst-case attempts per classification at six.
const maxTaggingClassifierFallbacks = 5

// maxTaggingClassifierModelLength caps a single model name: --model values are short
// identifiers ("haiku", "sonnet"); anything longer is a paste error, not a model.
const maxTaggingClassifierModelLength = 128

// taggingClassifierConfigToProto renders the persisted config through the same OrDefault
// accessors the poller boots from, so Get always reports the effective hierarchy —
// including env overrides — never raw possibly-blank storage.
func taggingClassifierConfigToProto(model string, fallbacks []string) *sessionv1.TaggingClassifierConfigProto {
	return &sessionv1.TaggingClassifierConfigProto{
		Model:          model,
		FallbackModels: fallbacks,
	}
}

// TaggingClassifierModelHierarchy returns the effective hierarchy the poller boots from:
// config file first, STAPLER_SQUAD_TAGGING_MODEL / STAPLER_SQUAD_TAGGING_FALLBACK_MODELS
// (comma-separated) overriding when set — same kill-switch-via-env precedent as
// STAPLER_SQUAD_DISABLE_DETECTOR_PLUGINS. Env wins so a free-proxy operator can point the
// fleet at a proxy model without editing config.json on every host. Exported for
// server/dependencies.go's poller construction (same package cannot be used there —
// dependencies.go is package server, the poller wiring lives here in services).
func TaggingClassifierModelHierarchy(cfg *config.Config) (string, []string) {
	model := strings.TrimSpace(envOrEmpty("STAPLER_SQUAD_TAGGING_MODEL"))
	fallbacks := splitEnvList(envOrEmpty("STAPLER_SQUAD_TAGGING_FALLBACK_MODELS"))
	if model == "" && cfg != nil {
		model = cfg.TaggingClassifierModelOrDefault()
	}
	if len(fallbacks) == 0 && cfg != nil {
		fallbacks = cfg.TaggingClassifierFallbacks()
	}
	if model == "" {
		model = "haiku"
	}
	return model, fallbacks
}

func envOrEmpty(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// splitEnvList splits a comma-separated env value, dropping blanks.
func splitEnvList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// GetTaggingClassifierConfig returns the effective LLM model hierarchy for session-tag
// classification.
// +api: tagging-classifier:get-config
func (s *SessionService) GetTaggingClassifierConfig(
	_ context.Context,
	_ *connect.Request[sessionv1.GetTaggingClassifierConfigRequest],
) (*connect.Response[sessionv1.GetTaggingClassifierConfigResponse], error) {
	model, fallbacks := TaggingClassifierModelHierarchy(config.LoadConfig())
	return connect.NewResponse(&sessionv1.GetTaggingClassifierConfigResponse{
		Config:            taggingClassifierConfigToProto(model, fallbacks),
		EnvOverrideActive: envOrEmpty("STAPLER_SQUAD_TAGGING_MODEL") != "" || envOrEmpty("STAPLER_SQUAD_TAGGING_FALLBACK_MODELS") != "",
	}), nil
}

// UpdateTaggingClassifierConfig validates, persists, and hot-applies a new model hierarchy.
// An empty model resets to the server default ("haiku"). Rejects blank-after-trim entries,
// overlong names, and hierarchies longer than maxTaggingClassifierFallbacks.
// +api: tagging-classifier:update-config
func (s *SessionService) UpdateTaggingClassifierConfig(
	_ context.Context,
	req *connect.Request[sessionv1.UpdateTaggingClassifierConfigRequest],
) (*connect.Response[sessionv1.UpdateTaggingClassifierConfigResponse], error) {
	in := req.Msg.GetConfig()
	if in == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("config is required"))
	}
	model := strings.TrimSpace(in.GetModel())
	for _, m := range append([]string{model}, in.GetFallbackModels()...) {
		if len(m) > maxTaggingClassifierModelLength {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("model name %q exceeds %d characters", m, maxTaggingClassifierModelLength))
		}
	}
	var fallbacks []string
	seen := map[string]bool{}
	primary := model
	if primary == "" {
		primary = "haiku"
	}
	for _, m := range in.GetFallbackModels() {
		if m = strings.TrimSpace(m); m == "" || m == primary || seen[m] {
			continue
		}
		seen[m] = true
		fallbacks = append(fallbacks, m)
	}
	if len(fallbacks) > maxTaggingClassifierFallbacks {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("%d fallback models exceeds the limit of %d", len(fallbacks), maxTaggingClassifierFallbacks))
	}

	cfg := config.LoadConfig()
	cfg.TaggingClassifier.Model = model
	cfg.TaggingClassifier.FallbackModels = fallbacks
	if err := config.SaveConfig(cfg); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("save tagging classifier config: %w", err))
	}
	// Env overrides win over the saved config, so the poller and response report the
	// effective hierarchy, not what was just saved.
	effModel, effFallbacks := TaggingClassifierModelHierarchy(cfg)
	if s.sessionTagPoller != nil {
		s.sessionTagPoller.SetModelConfig(effModel, effFallbacks)
	}
	log.Info("tagging classifier config updated", "model", effModel, "fallbacks", effFallbacks)
	return connect.NewResponse(&sessionv1.UpdateTaggingClassifierConfigResponse{
		Config: taggingClassifierConfigToProto(effModel, effFallbacks),
	}), nil
}

// ReclassifySessionTags synchronously re-runs the LLM classifier for one session, bypassing
// the poller's classify-once gate. Successfully applied sessions are never reclassified
// automatically — this RPC is the explicit user action that does. Fails NotFound when no
// monitored session matches, Unimplemented when the headless pool (and therefore the
// poller) is unavailable.
// +api: tagging-classifier:reclassify-session
func (s *SessionService) ReclassifySessionTags(
	ctx context.Context,
	req *connect.Request[sessionv1.ReclassifySessionTagsRequest],
) (*connect.Response[sessionv1.ReclassifySessionTagsResponse], error) {
	id := strings.TrimSpace(req.Msg.GetSessionId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if s.sessionTagPoller == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("tag classification is not available on this server (no headless pool)"))
	}
	if err := s.sessionTagPoller.ClassifyNow(ctx, id); err != nil {
		code := connect.CodeNotFound
		if errors.Is(err, session.ErrClassificationDegraded) {
			code = connect.CodeUnavailable
		}
		return nil, connect.NewError(code, err)
	}
	var tags []string
	for _, inst := range s.sessionTagPoller.Instances() {
		if inst.MatchesID(id) {
			tags = inst.GetTags()
			break
		}
	}
	return connect.NewResponse(&sessionv1.ReclassifySessionTagsResponse{Tags: tags}), nil
}
