package server

import (
	"net"
	"sync"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// prBaseURLSources are the inputs resolvePRBodyBaseURL chooses between.
type prBaseURLSources struct {
	configured     string // Slack.DashboardBaseURL
	remoteHTTPSURL string
	listenAddr     string // main listener, host:port
	hostnames      []string
}

// resolvePRBodyBaseURL picks the base URL for the PR-body footer link so the
// owner can open the creating instance from any of their machines: the
// configured Slack.DashboardBaseURL, else the remote-access HTTPS origin, else
// the first advertised hostname on the main listener's port when that listener
// is bound beyond loopback. It returns "" when none is available; loopback
// addresses are never returned.
func resolvePRBodyBaseURL(src prBaseURLSources) string {
	if u := session.NonLoopbackBaseURL(src.configured); u != "" {
		return u
	}
	if u := session.NonLoopbackBaseURL(src.remoteHTTPSURL); u != "" {
		return u
	}
	if len(src.hostnames) == 0 {
		return ""
	}
	listenHost, port, err := net.SplitHostPort(src.listenAddr)
	if err != nil {
		return ""
	}
	// A loopback-only listener is unreachable from another machine under any hostname.
	if ip := net.ParseIP(listenHost); listenHost == "localhost" || (ip != nil && ip.IsLoopback()) {
		return ""
	}
	return session.NonLoopbackBaseURL("http://" + net.JoinHostPort(src.hostnames[0], port))
}

// newHostRefResolver returns a resolver for this instance's host ID and first
// advertised hostname. The identity is loaded (or minted) on first use and
// cached; a load failure yields an empty host ID rather than failing the PR.
func newHostRefResolver(configDir string, hostnames func() []string) func() (session.HostID, string) {
	var (
		once sync.Once
		id   session.HostID
	)
	return func() (session.HostID, string) {
		once.Do(func() {
			identity, err := session.LoadOrCreateHostIdentity(configDir)
			if err != nil {
				log.Warn("pr_footer.host_identity_unavailable", "err", err)
				return
			}
			id = identity.ID
		})
		name := ""
		if hs := hostnames(); len(hs) > 0 {
			name = hs[0]
		}
		return id, name
	}
}
