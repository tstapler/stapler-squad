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

// sessionUIURL builds the `/?session=<id>` deep link for baseURL, or "" when
// baseURL is empty or not an http(s) URL.
func sessionUIURL(baseURL, sessionID string) string {
	if baseURL == "" || sessionID == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	u.Path = "/"
	u.RawQuery = url.Values{"session": {sessionID}}.Encode()
	u.Fragment = ""
	return u.String()
}
