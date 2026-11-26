package domain

// TerminalStatusStrings returns the built-in terminal statuses as strings,
// for ent query-builder call sites (StatusNotIn and similar) that need a
// literal list rather than a single-status predicate. Deliberately
// built-in-only: filtering a query against a dynamically configured
// custom-terminal stage set needs the stage registry, which isn't wired at
// the repository layer — out of scope for Epic 2.1 (see terminalstate.IsTerminalStatus's
// doc comment for the single-status equivalent, which custom stages do get).
func TerminalStatusStrings() []string {
	return []string{string(BacklogStatusDone), string(BacklogStatusArchived)}
}
