package config

import (
	"sync"
	"sync/atomic"
)

// LLMBackendsConfig is the persisted backend selection for headless LLM calls.
// Field semantics mirror headless.BackendSettings (kept separate so config does
// not import session/headless).
type LLMBackendsConfig struct {
	// Default is the backend for features without an override; empty = claude.
	Default string `json:"default,omitempty"`
	// PerFeature maps a headless feature key to a backend name.
	PerFeature map[string]string `json:"per_feature,omitempty"`
	// ConsoletteBaseURL overrides the default http://127.0.0.1:47000 router URL.
	ConsoletteBaseURL string `json:"consolette_base_url,omitempty"`
	// AnthropicBaseURL overrides https://api.anthropic.com for the HTTP
	// AnthropicAIClient (rules generation). It never affects the capacity
	// monitor's probe, which always targets api.anthropic.com.
	AnthropicBaseURL string `json:"anthropic_base_url,omitempty"`
	// ModelMaps maps backend -> Claude model alias -> backend-specific model ID.
	ModelMaps map[string]map[string]string `json:"model_maps,omitempty"`
}

// liveLLMBackends is the process-wide current selection. LoadConfig re-reads
// disk on every call, so live readers (the headless Selector) cannot hold a
// *Config; they read this atomic instead, seeded once from disk and replaced by
// UpdateLLMBackends.
var (
	liveLLMBackends atomic.Pointer[LLMBackendsConfig] //nolint:gochecknoglobals // single live selection shared by RPC and selector
	liveLLMInitMu   sync.Mutex                        //nolint:gochecknoglobals // serializes first load and updates
)

// LiveLLMBackends returns the current backend selection (a deep copy), loading
// it from config.json on first use.
func LiveLLMBackends() LLMBackendsConfig {
	if p := liveLLMBackends.Load(); p != nil {
		return p.clone()
	}
	liveLLMInitMu.Lock()
	defer liveLLMInitMu.Unlock()
	if p := liveLLMBackends.Load(); p != nil {
		return p.clone()
	}
	v := LoadConfig().LLMBackends
	liveLLMBackends.Store(&v)
	return v.clone()
}

// UpdateLLMBackends persists v to config.json and makes it live immediately.
// The in-memory selection changes only after the save succeeds.
func UpdateLLMBackends(v LLMBackendsConfig) error {
	liveLLMInitMu.Lock()
	defer liveLLMInitMu.Unlock()
	cfg := LoadConfig()
	cfg.LLMBackends = v
	if err := SaveConfig(cfg); err != nil {
		return err
	}
	stored := v.clone()
	liveLLMBackends.Store(&stored)
	return nil
}

// ResetLiveLLMBackendsForTest forgets the cached selection so the next read reloads from disk.
func ResetLiveLLMBackendsForTest() { liveLLMBackends.Store(nil) }

func (c LLMBackendsConfig) clone() LLMBackendsConfig {
	out := c
	out.PerFeature = copyStringMap(c.PerFeature)
	if c.ModelMaps != nil {
		out.ModelMaps = make(map[string]map[string]string, len(c.ModelMaps))
		for k, v := range c.ModelMaps {
			out.ModelMaps[k] = copyStringMap(v)
		}
	}
	return out
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
