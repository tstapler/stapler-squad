package ssqcli

import (
	"github.com/spf13/cobra"
)

// alias maps a short command under `ssq sessions` onto one RPC; positional
// names the request field filled from the first argument.
type alias struct {
	use        string
	short      string
	procedure  string
	positional string
}

const sessionSvc = "/session.v1.SessionService/"

func sessionAliases() []alias {
	return []alias{
		{use: "list", short: "List sessions (ListSessions)", procedure: sessionSvc + "ListSessions"},
		{use: "get <id>", short: "Show one session (GetSession)", procedure: sessionSvc + "GetSession", positional: "id"},
		{use: "restart <id>", short: "Restart a session (RestartSession)", procedure: sessionSvc + "RestartSession", positional: "id"},
		{use: "delete <id>", short: "Delete a session (DeleteSession)", procedure: sessionSvc + "DeleteSession", positional: "id"},
		{use: "ack <id>", short: "Acknowledge a session in the review queue (AcknowledgeSession)", procedure: sessionSvc + "AcknowledgeSession", positional: "id"},
		{use: "diff <id>", short: "Show a session's diff (GetSessionDiff)", procedure: sessionSvc + "GetSessionDiff", positional: "id"},
	}
}

func aliasGroup(env Env, g *globals, byProcedure map[string]Method) *cobra.Command {
	grp := groupCommand("sessions", "Friendly aliases for the most-used session RPCs")
	for _, a := range sessionAliases() {
		m, ok := byProcedure[a.procedure]
		if !ok {
			continue
		}
		grp.AddCommand(methodCommand(env, g, m, cmdSpec{use: a.use, short: a.short, positional: a.positional}))
	}
	return grp
}

// AliasProcedures lists the RPCs the aliases target, for tests.
func AliasProcedures() []string {
	out := make([]string, 0, len(sessionAliases()))
	for _, a := range sessionAliases() {
		out = append(out, a.procedure)
	}
	return out
}
