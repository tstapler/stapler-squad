// Package terminalstate holds the terminal-state predicates the backlog and UI layers share:
// whether a backlog status is terminal, whether a terminal stream must be read-only, and the
// settled-key tracker that stops safety-net sweeps re-reading finished items.
package terminalstate

import "github.com/tstapler/stapler-squad/session/domain"

// terminalStatusChecker, when non-nil, augments IsTerminalStatus so an
// operator-configured custom stage marked IsTerminal:true is recognized
// alongside the two built-in terminal statuses. Epic 2.3's
// ConfiguredWorkflowEngine wires this (via SetTerminalStatusChecker) once a
// StageDefinition.IsTerminal flag exists; nil (the default, and every
// current production/test wiring) means only the built-in constants below
// count. See the "BacklogStatus becomes the open stage-slug type" decision
// (project_plans/backlog-custom-workflow-stages/implementation/plan.md,
// Epic 2.1).
var terminalStatusChecker func(domain.BacklogStatus) bool

// SetTerminalStatusChecker wires a ConfiguredWorkflowEngine-backed terminal
// lookup into IsTerminalStatus. Pass nil to revert to the built-in-only
// fallback (e.g. in tests).
func SetTerminalStatusChecker(checker func(domain.BacklogStatus) bool) {
	terminalStatusChecker = checker
}

// IsTerminalStatus reports whether status represents a finished item that
// nothing should keep monitoring, waiting on, or auto-reopening — the two
// built-in terminal statuses (done, archived), plus, once
// SetTerminalStatusChecker is wired, any operator-configured custom stage
// marked IsTerminal. Replaces the repeated `status == BacklogStatusDone ||
// status == BacklogStatusArchived` literal that used to appear at every call
// site (Epic 2.1, Story 2.1.3).
func IsTerminalStatus(status domain.BacklogStatus) bool {
	if status == domain.BacklogStatusDone || status == domain.BacklogStatusArchived {
		return true
	}
	return terminalStatusChecker != nil && terminalStatusChecker(status)
}
