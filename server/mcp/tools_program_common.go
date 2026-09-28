package mcp

import (
	"context"
	"sort"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services"
)

// defaultProgramID is create_session/create_session_for_pr's fallback when
// program is omitted — unchanged behavior, now named once instead of
// duplicated as a "claude" literal in both tools.
const defaultProgramID = "claude"

// programDescription is deliberately hedged about list_programs: this repo's
// program-param backlog fix (Phase 1) may ship without the additive
// list_programs tool (Phase 2), and naming a tool that might not exist would
// leave a calling agent following broken guidance.
const programDescription = "Program to run (default: claude). Accepts any registered program ID. Common built-ins: claude, aider, pi, opencode, gemini, agy, bash. Custom programs added via the Program Configurations UI are also accepted. The enum below reflects programs known at server startup and may not include a program added since — check the Program Configurations UI, or ask whether a list_programs tool is available in this deployment, for the live, authoritative list."

// programIDs returns the sorted list of program IDs svc knows about at the
// time of the call (built-ins plus any registered custom programs). Returns
// nil when svc is nil (the documented githubHandlers.svc-may-be-nil case) or
// when ListProgramsConfig errs -- a missing enum is safe, not fatal, matching
// the hardcoded-enum behavior this replaces.
func programIDs(svc *services.SessionService) []string {
	if svc == nil {
		return nil
	}
	resp, err := svc.ListProgramsConfig(context.Background(), connect.NewRequest(&sessionv1.ListProgramsConfigRequest{}))
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(resp.Msg.Programs))
	for _, p := range resp.Msg.Programs {
		ids = append(ids, p.Id)
	}
	sort.Strings(ids)
	return ids
}

// programSchemaOptions builds the program property's schema options for both
// create_session and create_session_for_pr -- the single source of truth
// eliminating the two-file hand-copied Enum("claude", "aider") drift the two
// tools previously accumulated independently.
func programSchemaOptions(svc *services.SessionService) []mcpgo.PropertyOption {
	opts := []mcpgo.PropertyOption{mcpgo.Description(programDescription)}
	if ids := programIDs(svc); len(ids) > 0 {
		opts = append(opts, mcpgo.Enum(ids...))
	}
	return opts
}
