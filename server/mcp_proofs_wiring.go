package server

import (
	"context"
	"net/http"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// liveInstanceFinder is the slice of SessionService the MCP identity
// middleware needs to prove a verified caller still has a session.
type liveInstanceFinder interface {
	FindLiveInstance(id string) *session.Instance
}

// newMCPIdentityMiddleware wraps the /mcp handler so a request whose
// X-Stapler-Session-Proof verifies for a live session carries a verified
// identity (ADR-007 rule 0). Verification reads the process-wide McpProofs at
// request time, so a request before wiring (or with proofs unavailable) is
// simply unverified.
func newMCPIdentityMiddleware(next http.Handler, sessions liveInstanceFinder) http.Handler {
	verify := func(sessionUUID, proof string) bool {
		p := services.DefaultMcpProofs()
		return p != nil && p.Verify(sessionUUID, proof)
	}
	exists := func(sessionUUID string) bool { return sessions.FindLiveInstance(sessionUUID) != nil }
	return servermcp.NewIdentityMiddleware(next, verify, exists)
}

// wireMCPProofs installs the per-session MCP credential (ADR-007 rule 0): a
// startup self-check, the proof provider session launches use, a sweep of
// files whose session is gone, and deletion of a session's files when it is
// deleted. If the self-check fails nothing is installed, so every session
// stays unverified (fail closed) and the log says why.
func wireMCPProofs(ctx context.Context, deps *ServerDependencies, goBackground func(func())) {
	keys := services.DefaultHookProofs()
	if keys == nil {
		log.Warn("mcp credential disabled: hook proof secret unavailable; identity-gated tools will refuse every session")
		return
	}
	dir, err := config.GetConfigDir()
	if err != nil {
		log.Warn("mcp credential disabled: config dir unavailable", "err", err)
		return
	}
	proofs := services.NewMcpProofs(keys, dir)
	if err := proofs.SelfCheck(); err != nil {
		log.Error("mcp credential self-check FAILED; identity-gated tools will refuse every session", "err", err)
		return
	}
	log.Info("mcp credential self-check ok")
	services.SetDefaultMcpProofs(proofs)
	session.SetMCPProofProvider(proofs.Provider())

	known := make(map[string]struct{}, len(deps.Instances))
	for _, inst := range deps.Instances {
		known[inst.UUID] = struct{}{}
	}
	if n := proofs.SweepStale(func(id string) bool {
		_, ok := known[id]
		return ok || (deps.SessionService != nil && deps.SessionService.FindLiveInstance(id) != nil)
	}); n > 0 {
		log.Info("mcp credential: removed stale proof files", "files", n)
	}
	if deps.EventBus != nil {
		watchSessionDeletions(ctx, deps.EventBus, proofs, goBackground)
	}
}

// watchSessionDeletions deletes a session's proof files when it is deleted, so
// its still-HMAC-valid proof stops being usable from disk.
func watchSessionDeletions(ctx context.Context, bus *events.EventBus, proofs *services.McpProofs, goBackground func(func())) {
	ch, _ := bus.Subscribe(ctx)
	goBackground(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				removeProofsOnDelete(proofs, ev)
			}
		}
	})
}

func removeProofsOnDelete(proofs *services.McpProofs, ev *events.Event) {
	if ev != nil && ev.Type == events.EventSessionDeleted {
		proofs.RemoveFiles(ev.SessionID)
	}
}
