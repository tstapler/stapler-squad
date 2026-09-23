package session

// terminalStatusChecker, when non-nil, augments IsTerminalStatus so an
// operator-configured custom stage marked IsTerminal:true is recognized
// alongside the two built-in terminal statuses. Epic 2.3's
// ConfiguredWorkflowEngine wires this (via SetTerminalStatusChecker) once a
// StageDefinition.IsTerminal flag exists; nil (the default, and every
// current production/test wiring) means only the built-in constants below
// count. See the "BacklogStatus becomes the open stage-slug type" decision
// (project_plans/backlog-custom-workflow-stages/implementation/plan.md,
// Epic 2.1).
var terminalStatusChecker func(BacklogStatus) bool

// SetTerminalStatusChecker wires a ConfiguredWorkflowEngine-backed terminal
// lookup into IsTerminalStatus. Pass nil to revert to the built-in-only
// fallback (e.g. in tests).
func SetTerminalStatusChecker(checker func(BacklogStatus) bool) {
	terminalStatusChecker = checker
}

// IsTerminalStatus reports whether status represents a finished item that
// nothing should keep monitoring, waiting on, or auto-reopening — the two
// built-in terminal statuses (done, archived), plus, once
// SetTerminalStatusChecker is wired, any operator-configured custom stage
// marked IsTerminal. Replaces the repeated `status == BacklogStatusDone ||
// status == BacklogStatusArchived` literal that used to appear at every call
// site (Epic 2.1, Story 2.1.3).
func IsTerminalStatus(status BacklogStatus) bool {
	if status == BacklogStatusDone || status == BacklogStatusArchived {
		return true
	}
	return terminalStatusChecker != nil && terminalStatusChecker(status)
}

// TerminalStatusStrings returns the built-in terminal statuses as strings,
// for ent query-builder call sites (StatusNotIn and similar) that need a
// literal list rather than a single-status predicate. Deliberately
// built-in-only: filtering a query against a dynamically configured
// custom-terminal stage set needs the stage registry, which isn't wired at
// the repository layer — out of scope for Epic 2.1 (see IsTerminalStatus's
// doc comment for the single-status equivalent, which custom stages do get).
func TerminalStatusStrings() []string {
	return []string{string(BacklogStatusDone), string(BacklogStatusArchived)}
}
