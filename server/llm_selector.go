package server

import (
	"path/filepath"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session/headless"
)

// buildLLMSelector registers every backend and wires live settings from cfg.
// Backends are always registered; unavailable ones are skipped per call by
// Selector.Resolve, so installing agy/gemini later needs no restart.
func buildLLMSelector(pool *headless.Pool) *headless.Selector {
	settings := func() headless.BackendSettings {
		return services.BackendSettingsFromConfig(config.LiveLLMBackends())
	}
	sel := headless.NewSelector(settings)
	sel.Register(headless.NewClaudeBackend(pool))
	sel.Register(headless.NewConsoletteBackend(settings, headless.PoolConfig{MaxCallsPerSession: 25, MaxConcurrentSessions: 5}))
	for _, spec := range headless.KnownCLIBackendSpecs {
		sel.Register(headless.NewCLIBackend(spec))
	}
	if dir, err := config.GetConfigDir(); err == nil {
		sel.SetFallbackRecorder(headless.NewFileFallbackRecorder(filepath.Join(dir, "llm_backend_fallbacks.jsonl")))
	} else {
		log.Warn("llm backend fallbacks will not be persisted: config dir unavailable", "err", err)
	}
	headless.SetDefaultSelector(sel)
	return sel
}
