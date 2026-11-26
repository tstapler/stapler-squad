package server

import (
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/forwardsync"
)

// programCLIFlagProbeGate and githubProgressCommentsGate read their flag through the session service's
// injected registry on every call, so a Settings toggle still applies with no restart.
func programCLIFlagProbeGate(sessions *services.SessionService) func() bool {
	return func() bool { return flagsvc.ProgramCLIFlagProbeEnabled(sessions.InjectedFlagRegistry()) }
}

func githubProgressCommentsGate(sessions *services.SessionService) func() bool {
	return func() bool { return forwardsync.GitHubIssueProgressCommentsEnabled(sessions.InjectedFlagRegistry()) }
}

// startProgressSyncSubscriber is a seam so a test can capture the gate wireDepsIntoServer hands the subscriber.
var startProgressSyncSubscriber = forwardsync.StartBacklogGitHubProgressSyncSubscriber
