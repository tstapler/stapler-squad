package server

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/taggingrules"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/summaries"
	"github.com/tstapler/stapler-squad/session/tagpoller"
)

// llmWiring is the headless LLM stack: pool, backend selector and client. client is an
// untyped nil when the claude binary is missing so downstream `== nil` guards keep working.
type llmWiring struct {
	pool     *headless.Pool
	selector *headless.Selector
	client   headless.PoolClient
}

// buildHeadlessLLM constructs the headless pool early so the lifecycle listener can take
// it by constructor. Non-fatal when the claude binary is not found.
func buildHeadlessLLM(sessionService *services.SessionService) llmWiring {
	// Construct the headless LLM pool early so the lifecycle listener can receive it
	// via constructor (eliminating the post-construction wiring race). Non-fatal if
	// the claude binary is not found.
	var headlessPool *headless.Pool
	{
		p, poolErr := headless.NewPool(headless.PoolConfig{
			MaxCallsPerSession:    25,
			MaxConcurrentSessions: 5,
			DefaultModel:          config.HeadlessPoolDefaultModel,
			// Fresh load per call so background_models edits apply live.
			FeatureModel: func(k headless.FeatureKey) string {
				return config.LoadConfig().BackgroundFeatureModel(string(k))
			},
		})
		if poolErr != nil {
			log.Warn("headless pool disabled: claude binary not found", "err", poolErr)
		} else {
			headlessPool = p
			headless.SetDefaultPool(p)
			sessionService.SetHeadlessPool(p)
			log.Info("headless LLM pool initialized")
		}
	}

	// Backend selector: routes headless calls by live config (llm_backends), with
	// claude as the always-registered fallback. llmClient stays an untyped nil
	// when claude is missing so downstream `== nil` guards keep working.
	llmSelector := buildLLMSelector(headlessPool)
	var llmClient headless.PoolClient
	if headlessPool != nil {
		llmClient = &headless.SelectingClient{Selector: llmSelector}
		sessionService.SetHeadlessClient(llmClient)
	}

	return llmWiring{pool: headlessPool, selector: llmSelector, client: llmClient}
}

// buildTagPoller builds the LLM-fallback tag classifier; nil without a headless pool.
func buildTagPoller(cfg *config.Config, llm llmWiring, sessionService *services.SessionService) *tagpoller.SessionTagClassificationPoller {
	headlessPool, llmClient := llm.pool, llm.client
	// SessionTagClassificationPoller (session-classifier-pipeline Epic 4.4) — LLM fallback
	// classification for sessions the sync TaggingEngine can't confidently tag. Guarded by
	// headlessPool != nil, same "disable by not registering" pattern as every other
	// headlessPool-dependent component in this function: no claude binary means every session
	// simply never gets an LLM-derived tag, sync rules still work unaffected (ADR-001).
	var sessionTagPoller *tagpoller.SessionTagClassificationPoller
	if headlessPool != nil {
		tagPollerCfg := tagpoller.DefaultSessionTagPollerConfig()
		tagModel, tagFallbacks := taggingrules.TaggingClassifierModelHierarchy(cfg)
		tagPollerCfg.Model = tagModel
		tagPollerCfg.FallbackModels = tagFallbacks
		sessionTagPoller = tagpoller.NewSessionTagClassificationPollerWithConfig(llmClient, sessionService.GetTaggingEngine(), tagPollerCfg)
		// Wire into SessionService so every session-creation path (CreateSession,
		// CreateDirectorySession, CreateWorktreeSession, ForkSession) registers new
		// sessions with the poller too, not just the boot-time instance list set via
		// SetInstances below. Without this, any session created after server startup
		// was invisible to LLM-fallback tag classification (sync-rule tagging via
		// reclassifyTagsLocked still worked, but the LLM fallback never saw it).
		sessionService.SetSessionTagPoller(sessionTagPoller)
	}

	return sessionTagPoller
}

// buildSummaryGenerators builds the session and handoff summary generators; each is nil
// when storage is not ent-backed.
func buildSummaryGenerators(svc *ServiceDeps, llmClient headless.PoolClient) (*summaries.SessionSummaryGenerator, *summaries.HandoffSummaryGenerator) {
	storage, sessionService := svc.Storage, svc.SessionService
	// SessionSummaryGenerator — constructed here (not deferred to server.go) so it
	// can be wired to every instance in the loop just below, mirroring
	// backlogLifecycleListener's setup. Its NotificationDecisionLister/TokenStore
	// dependencies aren't ready yet at this point in startup (NotificationHistoryStore
	// is built later, in server.go's RunServer; the token store further below in this
	// function) — wired in later via SetNotificationLister/SetTokenStore, the same
	// "Set* called long after construction" ordering constraint SetHeadlessPool used
	// to have (see the comment above backlogLifecycleListener). Nil when storage isn't
	// ent-backed.
	var sessionSummaryGenerator *summaries.SessionSummaryGenerator
	if entClient := storage.GetEntClient(); entClient != nil {
		sessionSummaryGenerator = summaries.NewSessionSummaryGenerator(
			entClient,
			llmClient,
			nil, // NotificationDecisionLister — wired later via SetNotificationLister
			nil, // TokenStoreReader — wired later via SetTokenStore
			&reviewQueueLookupAdapter{storage: storage},
		)
		sessionService.SetSessionSummaryGenerator(sessionSummaryGenerator)
	} else {
		log.Warn("session summary generation unavailable: storage is not ent-backed")
	}

	// HandoffSummaryGenerator (Story 2.2.1) — same construction/nil-guard shape
	// as sessionSummaryGenerator just above. No SetSessionSummaryGenerator-style
	// wiring onto sessionService needed: unlike SessionSummaryGenerator, nothing
	// in the live-instance lifecycle dispatches handoff-summary generation —
	// it's only triggered on demand via HandoffSummaryService's RPC handlers.
	var handoffSummaryGenerator *summaries.HandoffSummaryGenerator
	if entClient := storage.GetEntClient(); entClient != nil {
		handoffSummaryGenerator = summaries.NewHandoffSummaryGenerator(entClient, llmClient)
	} else {
		log.Warn("handoff summary generation unavailable: storage is not ent-backed")
	}

	return sessionSummaryGenerator, handoffSummaryGenerator
}
