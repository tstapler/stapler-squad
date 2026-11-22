package server

import (
	githubpkg "github.com/tstapler/stapler-squad/github"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// The concrete types handed to servermcp.NewHTTPHandler and RunServer must keep
// satisfying the consumer-side ports server/mcp defines for them.
var (
	_ servermcp.TerminalScrollback = (*scrollback.ScrollbackManager)(nil)
	_ servermcp.AutoReopener       = (*backlogsvc.BacklogService)(nil)
	_ servermcp.UserPRSource       = (*githubpkg.UserPRCache)(nil)
	_ servermcp.BacklogRPC         = (*backlogsvc.BacklogService)(nil)
	_ servermcp.SessionRPC         = mcpSessionAdapter{}
	_ servermcp.HelpAnswerer       = (*services.GuidanceRequestService)(nil)
)

// mcpAutoReopener returns a nil interface for a nil service: boxing the nil
// pointer would make the handler's `autoReopener != nil` guard true and panic.
func mcpAutoReopener(svc *backlogsvc.BacklogService) servermcp.AutoReopener {
	if svc == nil {
		return nil
	}
	return svc
}

// mcpUserPRSource returns a nil interface for a nil cache: boxing the nil
// pointer would defeat the GitHub module's absent-source guard and panic.
func mcpUserPRSource(cache *githubpkg.UserPRCache) servermcp.UserPRSource {
	if cache == nil {
		return nil
	}
	return cache
}

// mcpBacklogRPC returns a nil interface for a nil service: boxing the nil
// pointer would defeat the handlers' `backlogSvc == nil` guards and panic.
func mcpBacklogRPC(svc *backlogsvc.BacklogService) servermcp.BacklogRPC {
	if svc == nil {
		return nil
	}
	return svc
}

// mcpSessionAdapter is *services.SessionService plus the two package-level hook
// helpers server/mcp calls, so that package names neither HookName nor the
// helpers (and so not server/services).
type mcpSessionAdapter struct{ *services.SessionService }

func (mcpSessionAdapter) InjectHooksConfig(rootDir, sessionTitle string, hooks []string) error {
	names := make([]services.HookName, len(hooks))
	for i, h := range hooks {
		names[i] = services.HookName(h)
	}
	return services.InjectHooksConfig(rootDir, sessionTitle, names)
}

func (mcpSessionAdapter) RefreshInstanceHookProof(inst *instance.Instance) {
	services.RefreshInstanceHookProof(inst)
}

// mcpSessionRPC returns a nil interface for a nil service: boxing the nil
// pointer inside the adapter would defeat every handler's `svc == nil` guard.
func mcpSessionRPC(svc *services.SessionService) servermcp.SessionRPC {
	if svc == nil {
		return nil
	}
	return mcpSessionAdapter{svc}
}
