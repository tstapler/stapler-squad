package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// WindowFocus describes the FocusWindow RPC that brings the app window to the foreground.
var WindowFocus = featureregistry.Feature{
	ID:          "window-focus",
	Title:       "Focus Window",
	Description: "Brings the application window to the foreground on the host platform.",
	RPCIDs:      []string{"window:focus"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(WindowFocus)
}
