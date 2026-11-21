package server

import (
	githubpkg "github.com/tstapler/stapler-squad/github"
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// The concrete types handed to servermcp.NewHTTPHandler and RunServer must keep
// satisfying the consumer-side ports server/mcp defines for them.
var (
	_ servermcp.TerminalScrollback = (*scrollback.ScrollbackManager)(nil)
	_ servermcp.AutoReopener       = (*backlogsvc.BacklogService)(nil)
	_ servermcp.UserPRSource       = (*githubpkg.UserPRCache)(nil)
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
