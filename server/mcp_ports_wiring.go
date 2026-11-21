package server

import (
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services/backlogsvc"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// The concrete types handed to servermcp.NewHTTPHandler and RunServer must keep
// satisfying the consumer-side ports server/mcp defines for them.
var (
	_ servermcp.TerminalScrollback = (*scrollback.ScrollbackManager)(nil)
	_ servermcp.AutoReopener       = (*backlogsvc.BacklogService)(nil)
)
