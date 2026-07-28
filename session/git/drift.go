package git

import (
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// DefaultBranchDriftThreshold is how many commits a work session's branch can fall
// behind main before EnsureBranchSyncedWithMain treats it as drifted and attempts a
// proactive resync before review. See BUG-044: a branch left to drift unbounded
// across a multi-day item lifecycle eventually produces a review/PR diff dominated
// by unrelated upstream commits rather than the item's own work, which review then
// (correctly, given what it's shown) reports as unrelated — misdiagnosing branch
// staleness as bad work. 50 commits is comfortably past "a few days of normal repo
// activity" while still catching drift well before it reaches the hundreds-of-commits
// scale that made backlog item 693c2700's diff unreviewable (289 commits behind).
const DefaultBranchDriftThreshold = 50

// SteeringBranchDriftThreshold is the lower drift threshold used by the PostToolUse
// steering hook (server/services/hook_receiver_drift.go) that fires after every git
// commit/push inside an autonomous backlog work session. Deliberately smaller than
// DefaultBranchDriftThreshold: that threshold governs when review itself blocks and
// is calibrated to "comfortably past normal multi-day activity"; this one governs
// when the *agent still doing the work* gets nudged, so it can self-correct well
// before drift ever reaches review-blocking scale. 20 is roughly a day or so of
// normal upstream activity — enough headroom that this doesn't fire on ordinary
// same-session commits, while leaving a wide margin before DefaultBranchDriftThreshold
// (50) so the agent has room to act on the nudge before review would block. See
// BUG-044.
const SteeringBranchDriftThreshold = 20

// EnsureBranchSyncedWithMain checks how far worktreePath's checked-out branch has
// drifted behind origin/mainBranch (via BehindOriginMain, always freshly fetched)
// and, once past driftThreshold commits, proactively merges main in and pushes the
// result — the same sync backlog_service_triage.go's syncPRBranchWithMain performs
// reactively after a PR-fix cycle, offered here as a precondition any review caller
// can run before trusting a diff computed against this worktree (BUG-044 suggested
// fix direction #1: "make the main-sync a precondition of review, not a best-effort
// side effect of the fix-retry path").
//
// ok=true means the branch is fine to review as-is — either it wasn't drifted past
// driftThreshold, or a sync just resolved it cleanly (merged and pushed, or found
// already up to date). ok=false means the caller must not proceed to review yet;
// blockedSummary explains why in language written for both a human operator and a
// follow-up fix session's prompt context, naming the exact conflicted files (or push
// failure) rather than leaving the cause to be inferred from a confusing diff later.
//
// Fails OPEN on any error determining or acting on drift (bad repo, fetch failure,
// merge error unrelated to a real conflict): returns ok=true so a broken detector
// never itself blocks review — matches every other best-effort git check in this
// codebase (see syncPRBranchWithMain's "never blocks the spawn" doc comment for the
// identical rationale). branchName is used only to build the operator-facing/fix-session
// message; the merge and push themselves act on whatever is currently checked out at
// worktreePath.
func EnsureBranchSyncedWithMain(worktreePath, branchName, mainBranch string, driftThreshold int) (ok bool, blockedSummary string) {
	if worktreePath == "" {
		return true, ""
	}

	behind, err := BehindOriginMain(worktreePath, mainBranch)
	if err != nil {
		log.WarningLog.Printf("[EnsureBranchSyncedWithMain] BehindOriginMain %s: %v — proceeding without a drift check", worktreePath, err)
		return true, ""
	}
	if behind < driftThreshold {
		return true, ""
	}

	log.InfoLog.Printf("[EnsureBranchSyncedWithMain] branch=%s worktree=%s is %d commits behind origin/%s (threshold %d) — attempting proactive sync before review",
		branchName, worktreePath, behind, mainBranch, driftThreshold)

	result, mergeErr := MergeMainIntoWorktree(worktreePath, mainBranch)
	if mergeErr != nil {
		log.WarningLog.Printf("[EnsureBranchSyncedWithMain] merge %s into branch=%s: %v — proceeding without sync", mainBranch, branchName, mergeErr)
		return true, ""
	}

	switch {
	case result.Conflicted:
		return false, fmt.Sprintf(
			"Review blocked: this branch (%s) is %d commits behind %s and merging the latest %s in produced conflicts in:\n- %s\n\n"+
				"The merge was aborted so the worktree is clean. Resolve these conflicts against %s before this item can be reviewed — "+
				"reviewing the branch as-is would show a diff dominated by unrelated upstream drift instead of this item's actual work (see BUG-044).",
			branchName, behind, mainBranch, mainBranch, strings.Join(result.ConflictedFiles, "\n- "), mainBranch)
	case result.Merged:
		g := NewGitWorktreeFromStorage("", worktreePath, "", branchName, "")
		if pushErr := g.PushBranch(); pushErr != nil {
			return false, fmt.Sprintf(
				"Review blocked: this branch (%s) was %d commits behind %s. Merged the latest %s in locally, but could not push it (%v). "+
					"Push the merge before this item can be reviewed: `git -C %s push origin %s`.",
				branchName, behind, mainBranch, mainBranch, pushErr, worktreePath, branchName)
		}
		log.InfoLog.Printf("[EnsureBranchSyncedWithMain] branch=%s: synced and pushed %s (was %d commits behind)", branchName, mainBranch, behind)
		return true, ""
	default: // UpToDate — shouldn't normally happen given behind >= threshold > 0, but harmless.
		return true, ""
	}
}
