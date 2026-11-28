package domain

// SessionSpawnOptions bundles the session-creation fields shared by
// CreateDirectorySession and CreateWorktreeSession (Fowler's Introduce
// Parameter Object), replacing their previous 8/9-parameter positional lists
// (syntax-rules-go long-parameter-list).
type SessionSpawnOptions struct {
	Title   string
	Prompt  string
	Tags    []string
	OneShot bool
	Hidden  bool
	// ProgramOverride, when non-empty, replaces config.ResolveDefaults' resolved.Program
	// as the spawned Instance's InstanceOptions.Program — set before session.NewInstance/
	// instance.Start(true), never via a post-hoc SwitchProgram/Restart (see Epic 2.4's
	// design note). Leave "" for callers unaffected by per-stage program overrides.
	ProgramOverride string
	// AllowedTools restricts the dispatched session's MCP tool surface (see
	// sessioncreate.DiagnosticSessionAllowedTools's doc comment); "" preserves the default,
	// unrestricted behavior.
	AllowedTools string
}
