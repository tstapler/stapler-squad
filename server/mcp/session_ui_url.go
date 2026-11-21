package mcp

import (
	"net/url"
	"os"
	"strings"
)

// publicURLEnv names the externally reachable base URL of this instance's web UI
// (e.g. a Dev Workspace app URL). Unset means tools return no UI link.
const publicURLEnv = "STAPLER_SQUAD_PUBLIC_URL"

func publicURLFromEnv() string { return strings.TrimSpace(os.Getenv(publicURLEnv)) }

// sessionUIURL builds the `<base path>/?session=<id>` deep link for baseURL, or ""
// when baseURL is empty, not an http(s) URL, or carries userinfo (credentials
// must never be echoed into an MCP result).
func sessionUIURL(baseURL, sessionID string) string {
	if baseURL == "" || sessionID == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/"
	u.RawPath = ""
	u.RawQuery = url.Values{"session": {sessionID}}.Encode()
	u.Fragment = ""
	return u.String()
}
