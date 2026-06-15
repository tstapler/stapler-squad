package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

// BrowserCDPStream describes the CDP stream HTTP handler for embedded browser sessions.
var BrowserCDPStream = featureregistry.Feature{
	ID:          "browser-cdp-stream",
	Title:       "CDP Stream",
	Description: "Streams Chrome DevTools Protocol events for a session's embedded browser.",
	RPCIDs:      []string{"browser:cdp-stream"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

// BrowserProxy describes the VNC proxy HTTP handler for embedded browser sessions.
var BrowserProxy = featureregistry.Feature{
	ID:          "browser-proxy",
	Title:       "Browser Proxy",
	Description: "Proxies VNC connections to the embedded browser for a session.",
	RPCIDs:      []string{"browser:proxy"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(BrowserCDPStream)
	featureregistry.Register(BrowserProxy)
}
