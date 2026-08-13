package github

import "strings"

// PRPriority is a derived single-enum priority computed from compound PR state.
type PRPriority string

const (
	PRPriorityBlocking  PRPriority = "blocking"   // changes requested or CI failing
	PRPriorityReady     PRPriority = "ready"      // approved + CI passing
	PRPriorityPending   PRPriority = "pending"    // awaiting review or checks running
	PRPriorityDraft     PRPriority = "draft"      // PR is a draft
	PRPriorityComplete  PRPriority = "complete"   // PR is merged or closed
	PRPriorityNoPR      PRPriority = "no_pr"      // no PR found for branch
	PRPriorityAuthError PRPriority = "auth_error" // gh CLI not authenticated
	PRPriorityError     PRPriority = "error"      // transient error fetching status
)

// DerivePRPriority computes a single priority enum from compound PR state.
// Returns PRPriorityNoPR if info is nil.
func DerivePRPriority(info *PRInfo) PRPriority {
	if info == nil {
		return PRPriorityNoPR
	}

	state := strings.ToLower(info.State)

	// Terminal states take highest precedence
	if state == "merged" || state == "closed" {
		return PRPriorityComplete
	}

	// Draft is lower priority than terminal but higher than blocking
	if info.IsDraft {
		return PRPriorityDraft
	}

	// Blocking: reviewer has requested changes
	if info.ChangesRequestedCount > 0 {
		return PRPriorityBlocking
	}

	// Blocking: CI check has failed
	switch info.CheckConclusion {
	case "failure", "action_required":
		return PRPriorityBlocking
	}

	// Ready: has approvals and CI is passing (or no CI configured)
	if info.ApprovedCount > 0 {
		switch info.CheckConclusion {
		case "success", "":
			return PRPriorityReady
		}
	}

	// Pending: checks are still running
	if info.CheckStatus == "in_progress" || info.CheckConclusion == "pending" {
		return PRPriorityPending
	}

	return PRPriorityPending
}

// IsTerminal returns true if the priority indicates a terminal PR state.
// Terminal sessions should not be polled at normal frequency.
func IsTerminal(priority PRPriority) bool {
	return priority == PRPriorityComplete
}
