package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// TerminalRender describes the terminal streaming RPC and its UI surface.
var TerminalRender = featureregistry.Feature{
	ID:          "terminal-render",
	Title:       "Terminal Rendering",
	Description: "Streams terminal output to the browser with RAF batching.",
	RPCIDs:      []string{"session:stream-terminal"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(TerminalRender)
}
