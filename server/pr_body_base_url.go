package server

import "github.com/tstapler/stapler-squad/session"

// resolvePRBodyBaseURL picks the dashboard base URL embedded in backlog PR
// bodies: the configured Slack.DashboardBaseURL, else the remote-access HTTPS
// origin, else "" (the PR body then omits its backlog link). Loopback and
// otherwise unusable URLs are skipped, never published.
func resolvePRBodyBaseURL(configured, remoteHTTPSURL string) string {
	if u := session.ReviewerReachableBaseURL(configured); u != "" {
		return u
	}
	return session.ReviewerReachableBaseURL(remoteHTTPSURL)
}
