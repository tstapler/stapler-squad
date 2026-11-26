package server

import (
	"net/http"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/backlogmisc"
	"github.com/tstapler/stapler-squad/server/services/debugseed"
	"github.com/tstapler/stapler-squad/server/services/guidance"
)

// registerE2EDebugRoutes registers the backlog and guidance debug endpoints ONLY for the e2e test server
// (STAPLER_SQUAD_INSTANCE=e2e-local) — lets the Playwright suite seed
// BacklogStuckState rows and queued backlog items directly, bypassing the
// reconciler's real thresholds and the real WIP-cap spawn flow. Never
// registered outside that instance.
func registerE2EDebugRoutes(mux *http.ServeMux, getenv func(string) string, deps *ServerDependencies) {
	if getenv("STAPLER_SQUAD_INSTANCE") == "e2e-local" && deps.Storage != nil {
		backlogSeedHandler := debugseed.NewBacklogDebugSeedHandler(deps.Storage)
		backlogSeedHandler.RegisterRoutes(mux)
		log.Info("Registered backlog debug seed handlers at /api/debug/backlog/seed-stuck, /api/debug/backlog/seed-queued, /api/debug/backlog/seed-headless-triage-session, /api/debug/backlog/seed-work-item-session, and /api/debug/backlog/seed-work-session-with-worktree (e2e-local only)")

		// Registered for project_plans/backlog-event-driven-updates's Playwright
		// e2e layer — lets tests mutate a backlog item directly through the
		// storage layer (create/transition/update/archive/delete), simulating a
		// second actor (reconciler, another tab) without walking a real
		// TransitionBacklogItemStatus RPC through its engine/gate checks.
		backlogMutateHandler := backlogmisc.NewBacklogDebugMutateHandler(deps.Storage)
		backlogMutateHandler.RegisterRoutes(mux)
		log.Info("Registered backlog debug mutate handlers at /api/debug/backlog/mutate-* (e2e-local only)")

		if deps.SessionService != nil {
			guidance.NewGuidanceDebugSeedHandler(deps.SessionService.GuidanceSeedPorts(), deps.Storage).RegisterRoutes(mux)
			log.Info("Registered guidance debug seed handler at /api/debug/guidance/seed-question (e2e-local only)")
		}
	}
}
