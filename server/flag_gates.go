package server

import "github.com/tstapler/stapler-squad/server/services"

// programCLIFlagProbeGate and githubProgressCommentsGate read their flag through the session service's
// injected registry on every call, so a Settings toggle still applies with no restart.
func programCLIFlagProbeGate(sessions *services.SessionService) func() bool {
	return func() bool { return services.ProgramCLIFlagProbeEnabled(sessions.InjectedFlagRegistry()) }
}

func githubProgressCommentsGate(sessions *services.SessionService) func() bool {
	return func() bool { return services.GitHubIssueProgressCommentsEnabled(sessions.InjectedFlagRegistry()) }
}
