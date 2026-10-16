package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
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

// programIDs returns the program IDs svc currently knows about, live (not
// cached). Returns nil if svc is nil or the call errs -- a missing enum is
// non-fatal, matching the hardcoded-enum behavior this replaces.
func programIDs(ctx context.Context, svc *services.SessionService) []string {
	if svc == nil {
		return nil
	}
	resp, err := svc.ListProgramsConfig(ctx, connect.NewRequest(&sessionv1.ListProgramsConfigRequest{}))
	if err != nil {
		log.Warn("programIDs: ListProgramsConfig failed, omitting enum/warning", "err", err)
		return nil
	}
	ids := make([]string, 0, len(resp.Msg.Programs))
	for _, p := range resp.Msg.Programs {
		ids = append(ids, p.Id)
	}
	slices.Sort(ids)
	return ids
}

// programSchemaOptions builds the program property's schema options -- the
// single source of truth eliminating the two-file hand-copied
// Enum("claude", "aider") the two tools previously accumulated independently.
// Runs once at tool-registration time, so its enum is a startup snapshot.
func programSchemaOptions(svc *services.SessionService) []mcpgo.PropertyOption {
	opts := []mcpgo.PropertyOption{mcpgo.Description(programDescription)}
	if ids := programIDs(context.Background(), svc); len(ids) > 0 {
		opts = append(opts, mcpgo.Enum(ids...))
	}
	return opts
}

// programWarningFor returns a non-fatal warning when program isn't among the
// programs svc currently knows about (a live check -- unlike
// programSchemaOptions' registration-time enum), or "" when program is
// recognized or the known list is unavailable.
func programWarningFor(ctx context.Context, svc *services.SessionService, program string) string {
	known := programIDs(ctx, svc)
	if len(known) == 0 || slices.Contains(known, program) {
		return ""
	}
	return fmt.Sprintf(
		"program %q is not among the programs currently registered (%s) — the session will still be created, but if this is a typo it will fail silently as shell output in the session's tmux pane. Check the Program Configurations UI, or ask whether list_programs is available in this deployment for a live list.",
		program, strings.Join(known, ", "))
}
